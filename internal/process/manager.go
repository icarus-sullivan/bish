package process

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/logs"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusCrashed Status = "crashed"
)

type Process struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	Name      string    `json:"name"`
	Cmd       string    `json:"cmd"`
	CWD       string    `json:"cwd"`
	StartTime time.Time `json:"start_time"`
	Ports     []int     `json:"ports"`
	CPUPct    float64   `json:"cpu_pct"`
	MemMB     float64   `json:"mem_mb"`
	Status    Status    `json:"status"`
	ExitCode  int       `json:"exit_code"`

	cmd    *exec.Cmd
	Log    *logs.LogBuffer `json:"-"`
}

func (p *Process) Uptime() string {
	if p.Status != StatusRunning {
		return "-"
	}
	d := time.Since(p.StartTime).Round(time.Second)
	if d.Hours() >= 1 {
		return fmt.Sprintf("%.0fh%.0fm", d.Hours(), d.Minutes()-60*float64(int(d.Hours())))
	}
	return fmt.Sprintf("%.0fm%.0fs", d.Minutes(), d.Seconds()-60*float64(int(d.Minutes())))
}

type Manager struct {
	mu        sync.Mutex
	Processes []*Process
}

func New() *Manager {
	return &Manager{}
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish")
}

func (m *Manager) Add(cmdStr, cwd, name string) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	if name == "" {
		fields := strings.Fields(cmdStr)
		if len(fields) > 0 {
			name = filepath.Base(fields[0])
		} else {
			name = cmdStr
		}
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-c", cmdStr)
	cmd.Dir = cwd

	lb := logs.NewBuffer()
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, err
	}
	pw.Close()

	p := &Process{
		ID:        id,
		PID:       cmd.Process.Pid,
		Name:      name,
		Cmd:       cmdStr,
		CWD:       cwd,
		StartTime: time.Now(),
		Status:    StatusRunning,
		cmd:       cmd,
		Log:       lb,
	}
	m.Processes = append(m.Processes, p)

	go func() {
		buf := make([]byte, 4096)
		var line []byte
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				for _, b := range buf[:n] {
					if b == '\n' {
						lb.Write(string(line))
						line = line[:0]
					} else {
						line = append(line, b)
					}
				}
			}
			if err != nil {
				break
			}
		}
		pr.Close()
	}()

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		p.Status = StatusStopped
		if err != nil {
			p.Status = StatusCrashed
			if cmd.ProcessState != nil {
				p.ExitCode = cmd.ProcessState.ExitCode()
			}
		}
	}()

	return p, nil
}

func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.find(id)
	if p == nil {
		return fmt.Errorf("process %s not found", id)
	}
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

func (m *Manager) Restart(id string) error {
	m.mu.Lock()
	p := m.find(id)
	if p == nil {
		m.mu.Unlock()
		return fmt.Errorf("process %s not found", id)
	}
	cmdStr := p.Cmd
	cwd := p.CWD
	name := p.Name
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill() //nolint
	}
	// remove old
	for i, pp := range m.Processes {
		if pp.ID == id {
			m.Processes = append(m.Processes[:i], m.Processes[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	_, err := m.Add(cmdStr, cwd, name)
	return err
}

func (m *Manager) Refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.Processes {
		if p.Status != StatusRunning {
			continue
		}
		p.Ports = DetectPorts(p.PID)
		p.CPUPct, p.MemMB = pidStats(p.PID)
	}
}

func pidStats(pid int) (cpu float64, memMB float64) {
	out, err := exec.Command("ps", "-o", "%cpu,rss", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return 0, 0
	}
	cpu, _ = strconv.ParseFloat(fields[0], 64)
	rss, _ := strconv.ParseFloat(fields[1], 64)
	memMB = rss / 1024
	return
}

func (m *Manager) KillAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.Processes {
		if p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Kill() //nolint
		}
	}
}

func (m *Manager) List() []*Process {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*Process, len(m.Processes))
	copy(result, m.Processes)
	return result
}

func (m *Manager) FindByID(id string) *Process {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.find(id)
}

func (m *Manager) find(id string) *Process {
	for _, p := range m.Processes {
		if p.ID == id {
			return p
		}
	}
	return nil
}

type diskProcess struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Cmd       string    `json:"cmd"`
	CWD       string    `json:"cwd"`
	StartTime time.Time `json:"start_time"`
	Status    Status    `json:"status"`
}

func (m *Manager) SaveToDisk() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var records []diskProcess
	for _, p := range m.Processes {
		records = append(records, diskProcess{
			ID:        p.ID,
			Name:      p.Name,
			Cmd:       p.Cmd,
			CWD:       p.CWD,
			StartTime: p.StartTime,
			Status:    p.Status,
		})
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir(), "processes.json"), data, 0o644)
}

func (m *Manager) LoadFromDisk() error {
	data, err := os.ReadFile(filepath.Join(configDir(), "processes.json"))
	if err != nil {
		return nil // not an error if missing
	}
	var records []diskProcess
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range records {
		m.Processes = append(m.Processes, &Process{
			ID:        r.ID,
			Name:      r.Name,
			Cmd:       r.Cmd,
			CWD:       r.CWD,
			StartTime: r.StartTime,
			Status:    StatusStopped, // can't know if still running across restarts
			Log:       logs.NewBuffer(),
		})
	}
	return nil
}
