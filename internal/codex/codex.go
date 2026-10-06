// Package codex drives the Codex panel: one `codex app-server` subprocess
// per conversation, spoken to over its newline-delimited JSON-RPC protocol
// on stdio (the same protocol the Codex IDE extensions use).
//
// The frontend never gets a raw pipe. It can call a closed set of methods
// (Call), and every thread/turn call has its parameters rebuilt here from a
// per-method allowlist: the working directory is pinned to the project root,
// sandbox/approval values must be known enums, and config overrides,
// instructions, and dynamic tools are dropped. Server→client requests
// (approvals, questions) are tracked by id, and only those may be answered.
package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxLine = 16 << 20

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session
	next     int
	emit     func(event string, data ...interface{})
	version  string
}

func NewManager(emit func(string, ...interface{}), version string) *Manager {
	return &Manager{sessions: make(map[string]*session), emit: emit, version: version}
}

type reply struct {
	result json.RawMessage
	err    string
}

type session struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stderr  *capBuf
	root    string
	writeMu sync.Mutex
	seq     int64
	stopped atomic.Bool

	mu      sync.Mutex
	replies map[string]chan reply
	// open server→client requests: JSON id (as raw text) → method
	asks map[string]string
}

type capBuf struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (c *capBuf) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > c.limit {
		c.buf = c.buf[len(c.buf)-c.limit:]
	}
	return len(p), nil
}

func (c *capBuf) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(string(c.buf))
}

func (s *session) write(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = fmt.Fprintf(s.stdin, "%s\n", line)
	return err
}

// Installed reports whether a runnable codex exists (own, bundled, or managed).
func Installed() bool { return Resolve() != "" }

// Start spawns `codex app-server` rooted at root, completes the initialize
// handshake, and returns an opaque handle.
func (m *Manager) Start(root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", fmt.Errorf("codex: project root must be an absolute path")
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return "", fmt.Errorf("codex: project root %q is not a directory", root)
	}
	bin := Resolve()
	if bin == "" {
		return "", fmt.Errorf("codex is not installed yet")
	}
	cmd := exec.Command(bin, "app-server")
	cmd.Dir = root
	stderr := &capBuf{limit: 8 << 10}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	s := &session{
		cmd: cmd, stdin: stdin, stderr: stderr, root: root,
		replies: make(map[string]chan reply), asks: make(map[string]string),
	}
	m.mu.Lock()
	id := fmt.Sprintf("x%d", m.next)
	m.next++
	m.sessions[id] = s
	m.mu.Unlock()
	go m.readLoop(id, s, stdout)

	_, err = s.call("initialize", map[string]any{
		"clientInfo": map[string]any{"name": "bish", "title": "bish", "version": m.version},
	}, 30*time.Second)
	if err != nil {
		m.Stop(id)
		if msg := stderr.String(); msg != "" {
			return "", fmt.Errorf("codex app-server failed to start: %s", msg)
		}
		return "", fmt.Errorf("codex app-server failed to start: %w", err)
	}
	if err := s.write(map[string]any{"method": "initialized"}); err != nil {
		m.Stop(id)
		return "", err
	}
	return id, nil
}

func (s *session) call(method string, params any, wait time.Duration) (json.RawMessage, error) {
	n := atomic.AddInt64(&s.seq, 1)
	key := fmt.Sprint(n)
	ch := make(chan reply, 1)
	s.mu.Lock()
	s.replies[key] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.replies, key)
		s.mu.Unlock()
	}()
	msg := map[string]any{"id": n, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := s.write(msg); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		if r.err != "" {
			return nil, fmt.Errorf("codex: %s: %s", method, r.err)
		}
		return r.result, nil
	case <-time.After(wait):
		return nil, fmt.Errorf("codex: %s timed out", method)
	}
}

func (m *Manager) session(id string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Call invokes one allowlisted app-server method. paramsJSON is a JSON
// object; it is filtered/pinned by sanitizeParams before it is sent.
func (m *Manager) Call(id, method, paramsJSON string) (string, error) {
	s := m.session(id)
	if s == nil {
		return "", fmt.Errorf("codex: no session %q", id)
	}
	var in map[string]any
	if paramsJSON != "" {
		if err := json.Unmarshal([]byte(paramsJSON), &in); err != nil {
			return "", fmt.Errorf("codex: bad params: %w", err)
		}
	}
	params, err := sanitizeParams(method, in, s.root)
	if err != nil {
		return "", err
	}
	wait := 60 * time.Second
	if method == "thread/list" || method == "thread/read" || method == "thread/resume" {
		wait = 2 * time.Minute
	}
	res, err := s.call(method, params, wait)
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "null", nil
	}
	return string(res), nil
}

// Respond answers an open server→client request (approval, user input).
// requestID is the request's JSON id exactly as it arrived.
func (m *Manager) Respond(id, requestID, resultJSON string) error {
	s := m.session(id)
	if s == nil {
		return fmt.Errorf("codex: no session %q", id)
	}
	s.mu.Lock()
	method, ok := s.asks[requestID]
	delete(s.asks, requestID)
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("codex: no pending request %s", requestID)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return fmt.Errorf("codex: bad response: %w", err)
	}
	clean, err := sanitizeResponse(method, result)
	if err != nil {
		return err
	}
	return s.write(map[string]any{"id": json.RawMessage(requestID), "result": clean})
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s != nil {
		s.kill()
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	all := m.sessions
	m.sessions = make(map[string]*session)
	m.mu.Unlock()
	for _, s := range all {
		s.kill()
	}
}

func (s *session) kill() {
	s.stopped.Store(true)
	s.stdin.Close()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill() //nolint
	}
}

type probe struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// answerable server requests are forwarded to the panel; everything else
// (auth token refresh, attestation, dynamic tools, MCP elicitation, …) is
// declined here so Codex never blocks on a request nobody will answer.
var answerable = map[string]bool{
	"item/commandExecution/requestApproval": true,
	"item/fileChange/requestApproval":       true,
	"item/tool/requestUserInput":            true,
}

func (m *Manager) readLoop(id string, s *session, stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		var p probe
		if json.Unmarshal(line, &p) != nil {
			continue
		}
		hasID := len(p.ID) > 0 && string(p.ID) != "null"
		switch {
		case hasID && p.Method == "" && (p.Result != nil || p.Error != nil):
			// response to one of our calls
			s.mu.Lock()
			ch := s.replies[strings.Trim(string(p.ID), `"`)]
			s.mu.Unlock()
			if ch != nil {
				r := reply{result: p.Result}
				if p.Error != nil {
					r.err = p.Error.Message
					if r.err == "" {
						r.err = "request failed"
					}
				}
				select {
				case ch <- r:
				default:
				}
			}
			continue
		case hasID && p.Method != "":
			if !answerable[p.Method] {
				s.write(map[string]any{"id": p.ID, "error": map[string]any{"code": -32601, "message": "not supported by this client"}}) //nolint
				continue
			}
			s.mu.Lock()
			s.asks[string(p.ID)] = p.Method
			s.mu.Unlock()
		}
		m.emit("codex:msg:"+id, string(line))
	}
	s.cmd.Wait() //nolint
	s.mu.Lock()
	for _, ch := range s.replies {
		select {
		case ch <- reply{err: "codex process exited"}:
		default:
		}
	}
	s.mu.Unlock()
	m.mu.Lock()
	crashed := !s.stopped.Load() && m.sessions[id] == s
	if crashed {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if crashed {
		m.emit("codex:exit:"+id, s.stderr.String())
	}
}
