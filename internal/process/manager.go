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
	"github.com/csullivan/bish/internal/shellenv"
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

	cmd      *exec.Cmd
	Log      *logs.LogBuffer `json:"-"`
	stopping bool            // set by Stop() right before killing, so the exit-watch goroutine reports "stopped" not "crashed"
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

	p := &Process{ID: id, Name: name, Log: logs.NewBuffer()}
	if err := m.spawnLocked(p, cmdStr, cwd); err != nil {
		return nil, err
	}
	m.Processes = append(m.Processes, p)
	return p, nil
}

// spawnLocked starts cmdStr in cwd and wires it into p (stdout/stderr piped
// into p.Log, an exit-watch goroutine to update p.Status/ExitCode). Caller
// must hold m.mu. p need not be in m.Processes yet (Add appends it right
// after; Restart swaps it into an existing slot right after) — by the time
// any goroutine started here can acquire m.mu, that placement has already
// happened, so the exit-watch goroutine's m.find(id) == p check below is
// race-free.
func (m *Manager) spawnLocked(p *Process, cmdStr, cwd string) error {
	// -l so .zprofile (brew shellenv etc.) applies to background commands.
	cmd := exec.Command(shellenv.DefaultShell(), "-l", "-c", cmdStr)
	cmd.Dir = cwd

	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}
	pw.Close()

	p.cmd = cmd
	p.PID = cmd.Process.Pid
	p.Cmd = cmdStr
	p.CWD = cwd
	p.StartTime = time.Now()
	p.Status = StatusRunning
	p.ExitCode = 0
	lb := p.Log
	id := p.ID

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
		waitErr := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		// a later Restart already replaced this process's slot with a new
		// *Process (possibly reusing this same id) — this run's exit status
		// is stale, not the current one's, so don't report it.
		if m.find(id) != p {
			return
		}
		if p.stopping {
			p.Status = StatusStopped
			return
		}
		p.Status = StatusStopped
		if waitErr != nil {
			p.Status = StatusCrashed
			if cmd.ProcessState != nil {
				p.ExitCode = cmd.ProcessState.ExitCode()
			}
		}
	}()

	return nil
}

// Stop kills the process but leaves its row in place (Status becomes
// "stopped", not removed) so it can be restarted later via Restart.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.find(id)
	if p == nil {
		return fmt.Errorf("process %s not found", id)
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.stopping = true
	return p.cmd.Process.Kill()
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, p := range m.Processes {
		if p.ID == id {
			if p.cmd != nil && p.cmd.Process != nil {
				p.cmd.Process.Kill() //nolint
			}
			m.Processes = append(m.Processes[:i], m.Processes[i+1:]...)
			return
		}
	}
}

// Restart (re)spawns id in place: same id and same slice slot (no
// reordering, and the frontend's log tab — keyed by id — stays the same
// tab), but a fresh log buffer, so the reused tab shows only the new run's
// output instead of appending onto the previous one. A new *Process is used
// rather than mutating the old one in place so spawnLocked's stale-exit-watch
// guard (pointer identity via m.find(id) == p) works unchanged.
func (m *Manager) Restart(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := -1
	var old *Process
	for i, pp := range m.Processes {
		if pp.ID == id {
			idx, old = i, pp
			break
		}
	}
	if old == nil {
		return fmt.Errorf("process %s not found", id)
	}
	if old.cmd != nil && old.cmd.Process != nil {
		old.cmd.Process.Kill() //nolint
	}
	np := &Process{ID: old.ID, Name: old.Name, Log: logs.NewBuffer()}
	if err := m.spawnLocked(np, old.Cmd, old.CWD); err != nil {
		return err
	}
	m.Processes[idx] = np
	return nil
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
