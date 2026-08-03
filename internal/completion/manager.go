// Package completion spawns and manages a local llama.cpp server
// (llama-server) running a small code model, used only for inline
// autocomplete suggestions in the editor. It is independent of the
// Assistant panel's model provider — that may point at a remote machine;
// this always runs on localhost as a subprocess of the Go backend.
package completion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/config"
)

// idleShutdown frees the running server's RSS after this long with no
// Suggest calls — mirrors frontend/src/lib/lsp.ts's IDLE_SHUTDOWN_MS for the
// same reason (nothing is editing code, don't keep paying for it).
const idleShutdown = 15 * time.Minute

// startTimeout bounds how long a freshly spawned server gets to answer
// /health before this attempt is treated as a failure.
const startTimeout = 5 * time.Second

// requestTimeout bounds a single /infill call — completions must stay fast
// enough to feel inline; a hung request shouldn't wedge the popup forever.
const requestTimeout = 5 * time.Second

type Manager struct {
	mu   sync.Mutex
	cfg  config.CompletionConfig
	cmd  *exec.Cmd
	port int

	fails    int
	lastFail time.Time

	idleTimer *time.Timer
	reqCancel context.CancelFunc

	client *http.Client
}

func NewManager() *Manager {
	return &Manager{client: &http.Client{}}
}

// SetConfig applies new settings, stopping any running server so the next
// Suggest call respawns with the new model/binary.
func (m *Manager) SetConfig(cfg config.CompletionConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	m.stopLocked()
	m.fails, m.lastFail = 0, time.Time{}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill() //nolint
	}
	m.cmd = nil
}

// Suggest completes the gap between prefix and suffix using the local
// model's fill-in-the-middle endpoint. It returns ("", nil) rather than an
// error for "no suggestion available right now" cases (disabled, no binary,
// no model, crash backoff, superseded by a newer keystroke) so callers can
// treat every non-error outcome uniformly.
func (m *Manager) Suggest(ctx context.Context, prefix, suffix string) (string, error) {
	port, ok := m.ensureStarted()
	if !ok {
		return "", nil
	}
	m.resetIdleTimer()

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	m.mu.Lock()
	if m.reqCancel != nil {
		m.reqCancel() // supersede any still-in-flight request from a prior keystroke
	}
	m.reqCancel = cancel
	m.mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"input_prefix": prefix,
		"input_suffix": suffix,
		"n_predict":    64,
		"temperature":  0.2,
	})
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/infill", port), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		if reqCtx.Err() != nil {
			return "", nil // canceled (superseded) or timed out — not a real error
		}
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	var out struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil
	}
	return out.Content, nil
}

// ensureStarted returns the port of a running, healthy server, spawning one
// if needed. ok is false when completion is disabled, misconfigured, or in
// crash backoff.
func (m *Manager) ensureStarted() (port int, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cfg.Enabled || m.cfg.ModelPath == "" {
		return 0, false
	}
	if m.cmd != nil {
		return m.port, true
	}
	if m.fails >= 3 && time.Since(m.lastFail) < time.Minute {
		return 0, false
	}
	if time.Since(m.lastFail) >= time.Minute {
		m.fails = 0
	}

	bin := m.cfg.ServerPath
	if bin == "" {
		bin = "llama-server"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return 0, false
	}

	p, err := freePort()
	if err != nil {
		return 0, false
	}

	cmd := exec.Command(bin, "-m", m.cfg.ModelPath, "-c", "32768",
		"--host", "127.0.0.1", "--port", strconv.Itoa(p))
	cmd.Stdout = io.Discard // llama-server logs are noisy; never buffer them
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		m.fails++
		m.lastFail = time.Now()
		return 0, false
	}
	m.cmd = cmd
	m.port = p
	go m.reap(cmd)

	if !waitHealthy(p, startTimeout) {
		cmd.Process.Kill() //nolint
		if m.cmd == cmd {
			m.cmd = nil
		}
		m.fails++
		m.lastFail = time.Now()
		return 0, false
	}
	return p, true
}

// reap waits for the server process to exit and clears it so the next
// Suggest call respawns — a crash is just another kind of "not started".
func (m *Manager) reap(cmd *exec.Cmd) {
	cmd.Wait() //nolint
	m.mu.Lock()
	if m.cmd == cmd {
		m.cmd = nil
		m.fails++
		m.lastFail = time.Now()
	}
	m.mu.Unlock()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitHealthy(port int, timeout time.Duration) bool {
	healthClient := &http.Client{Timeout: 300 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := healthClient.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (m *Manager) resetIdleTimer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idleTimer != nil {
		m.idleTimer.Stop()
	}
	m.idleTimer = time.AfterFunc(idleShutdown, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stopLocked()
	})
}
