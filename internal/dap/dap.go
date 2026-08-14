// Package dap drives a single delve DAP session (`dlv dap`) end to end:
// spawn, TCP handshake, initialize/launch, breakpoints, stepping, and
// stack/variable resolution. Unlike internal/lsp (a dumb stdio relay — the
// LSP protocol lives in the frontend's @codemirror/lsp-client), the DAP
// protocol lives entirely here: there's no equivalent frontend DAP client
// library installed, and doing the request/response/event bookkeeping in Go
// means the frontend only ever renders an already-resolved State struct.
package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const requestTimeout = 10 * time.Second

type Frame struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

type Variable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// State is the fully-resolved snapshot pushed to the frontend on every
// pause/run/exit — Problems-panel-style: heavy lifting happens once here
// instead of being repeated in JS.
type State struct {
	Status    string     `json:"status"` // "running" | "paused" | "terminated" | "error"
	Frames    []Frame    `json:"frames,omitempty"`
	Variables []Variable `json:"variables,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type wireMsg struct {
	Seq        int             `json:"seq"`
	Type       string          `json:"type"`
	Command    string          `json:"command,omitempty"`
	Event      string          `json:"event,omitempty"`
	Success    bool            `json:"success,omitempty"`
	RequestSeq int             `json:"request_seq,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	Arguments  interface{}     `json:"arguments,omitempty"`
	Message    string          `json:"message,omitempty"`
}

type Manager struct {
	mu   sync.Mutex
	sess *session
	emit func(event string, data ...interface{})
}

func NewManager(emit func(string, ...interface{})) *Manager {
	return &Manager{emit: emit}
}

type session struct {
	cmd      *exec.Cmd
	conn     net.Conn
	seq      atomic.Int32
	mu       sync.Mutex
	pending  map[int]chan wireMsg
	initEvt  chan struct{}
	initOnce sync.Once
	threadID atomic.Int32
	stopped  atomic.Bool // set by Manager.Stop so readLoop's EOF isn't reported as a crash
}

// Start spawns `dlv dap` rooted at root, runs the initialize/launch
// handshake with the given starting breakpoints (path → 1-based lines), and
// blocks until the program is confirmed running (or the handshake fails —
// most commonly a build error, surfaced in the returned error).
func (m *Manager) Start(root string, breakpoints map[string][]int) error {
	m.mu.Lock()
	if m.sess != nil {
		m.mu.Unlock()
		return fmt.Errorf("dap: a debug session is already running")
	}
	m.mu.Unlock()

	if _, err := exec.LookPath("dlv"); err != nil {
		return fmt.Errorf("dap: delve not installed (go install github.com/go-delve/delve/cmd/dlv@latest)")
	}

	cmd := exec.Command("dlv", "dap", "--listen=127.0.0.1:0")
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}

	br := bufio.NewReader(stdout)
	addr, err := readListenAddr(br)
	if err != nil {
		cmd.Process.Kill() //nolint
		return err
	}
	go io.Copy(io.Discard, br) //nolint // drain dlv's own logs so its stdout pipe never blocks

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		cmd.Process.Kill() //nolint
		return fmt.Errorf("dap: connecting to dlv: %w", err)
	}

	s := &session{
		cmd:     cmd,
		conn:    conn,
		pending: make(map[int]chan wireMsg),
		initEvt: make(chan struct{}),
	}
	m.mu.Lock()
	m.sess = s
	m.mu.Unlock()
	go m.readLoop(s)

	if err := m.handshake(s, root, breakpoints); err != nil {
		m.Stop()
		return err
	}
	return nil
}

func readListenAddr(br *bufio.Reader) (string, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("dap: dlv exited before it started listening: %w", err)
		}
		if _, addr, ok := strings.Cut(line, "DAP server listening at: "); ok {
			return strings.TrimSpace(addr), nil
		}
	}
	return "", fmt.Errorf("dap: timed out waiting for dlv to start listening")
}

func (m *Manager) handshake(s *session, root string, breakpoints map[string][]int) error {
	if _, err := s.request("initialize", map[string]interface{}{
		"clientID": "bish", "adapterID": "go", "pathFormat": "path",
		"linesStartAt1": true, "columnsStartAt1": true, "supportsVariableType": true,
	}); err != nil {
		return err
	}

	launchCh := s.requestAsync("launch", map[string]interface{}{
		"request": "launch", "mode": "debug", "program": root, "stopOnEntry": false,
	})

	select {
	case <-s.initEvt:
	case <-time.After(requestTimeout):
		return fmt.Errorf("dap: adapter never sent 'initialized'")
	}

	for path, lines := range breakpoints {
		if len(lines) == 0 {
			continue
		}
		if _, err := s.request("setBreakpoints", setBreakpointsArgs(path, lines)); err != nil {
			return fmt.Errorf("dap: setBreakpoints %s: %w", path, err)
		}
	}
	if _, err := s.request("configurationDone", struct{}{}); err != nil {
		return fmt.Errorf("dap: configurationDone: %w", err)
	}

	select {
	case resp := <-launchCh:
		if !resp.Success {
			return fmt.Errorf("dap: launch failed: %s", resp.Message)
		}
	case <-time.After(15 * time.Second):
		return fmt.Errorf("dap: launch timed out (build error?)")
	}
	return nil
}

type sourceBreakpoint struct {
	Line int `json:"line"`
}

func setBreakpointsArgs(path string, lines []int) map[string]interface{} {
	bps := make([]sourceBreakpoint, len(lines))
	for i, l := range lines {
		bps[i] = sourceBreakpoint{Line: l}
	}
	return map[string]interface{}{
		"source":      map[string]string{"path": path},
		"breakpoints": bps,
	}
}

// SetBreakpoints pushes an updated breakpoint set for one file to the
// running session (DAP replaces the whole set for that file each call). A
// no-op when no session is active — the frontend passes its full breakpoint
// map to Start next time instead.
func (m *Manager) SetBreakpoints(path string, lines []int) error {
	s := m.active()
	if s == nil {
		return nil
	}
	_, err := s.request("setBreakpoints", setBreakpointsArgs(path, lines))
	return err
}

func (m *Manager) Continue() error  { return m.threadCmd("continue") }
func (m *Manager) StepOver() error  { return m.threadCmd("next") }
func (m *Manager) StepIn() error    { return m.threadCmd("stepIn") }
func (m *Manager) StepOut() error   { return m.threadCmd("stepOut") }

func (m *Manager) threadCmd(command string) error {
	s := m.active()
	if s == nil {
		return fmt.Errorf("dap: no active session")
	}
	if command == "continue" {
		m.emit("dap:state", State{Status: "running"})
	}
	_, err := s.request(command, map[string]interface{}{"threadId": int(s.threadID.Load())})
	return err
}

func (m *Manager) Stop() {
	m.mu.Lock()
	s := m.sess
	m.sess = nil
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.stopped.Store(true)
	func() {
		defer func() { recover() }() //nolint // conn/session may already be tearing down
		s.request("disconnect", map[string]interface{}{"terminateDebuggee": true})
	}()
	s.conn.Close()
	s.cmd.Process.Kill() //nolint
}

func (m *Manager) active() *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sess
}

func (s *session) nextSeq() int { return int(s.seq.Add(1)) }

func (s *session) writeMsg(msg wireMsg) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = fmt.Fprintf(s.conn, "Content-Length: %d\r\n\r\n%s", len(b), b)
	return err
}

// requestAsync sends a request and returns a channel that receives its
// response whenever it arrives — for requests (like "launch") whose
// response is intentionally delayed behind other requests we still need to
// send in the meantime.
func (s *session) requestAsync(command string, args interface{}) chan wireMsg {
	seq := s.nextSeq()
	ch := make(chan wireMsg, 1)
	s.mu.Lock()
	s.pending[seq] = ch
	s.mu.Unlock()
	if err := s.writeMsg(wireMsg{Seq: seq, Type: "request", Command: command, Arguments: args}); err != nil {
		ch <- wireMsg{Message: err.Error()}
	}
	return ch
}

func (s *session) request(command string, args interface{}) (wireMsg, error) {
	ch := s.requestAsync(command, args)
	select {
	case resp := <-ch:
		if !resp.Success {
			return resp, fmt.Errorf("%s: %s", command, resp.Message)
		}
		return resp, nil
	case <-time.After(requestTimeout):
		return wireMsg{}, fmt.Errorf("%s timed out", command)
	}
}

// readLoop parses Content-Length-framed DAP messages off the TCP connection,
// routing responses to their waiting request() caller and events to their
// handler. Runs until the connection closes (deliberate Stop or a crash).
func (m *Manager) readLoop(s *session) {
	err := readFrames(s.conn, func(body []byte) {
		var msg wireMsg
		if json.Unmarshal(body, &msg) != nil {
			return
		}
		switch msg.Type {
		case "response":
			s.mu.Lock()
			ch := s.pending[msg.RequestSeq]
			delete(s.pending, msg.RequestSeq)
			s.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
		case "event":
			m.handleEvent(s, msg)
		}
	})
	_ = err
	if !s.stopped.Load() {
		m.mu.Lock()
		if m.sess == s {
			m.sess = nil
		}
		m.mu.Unlock()
		m.emit("dap:state", State{Status: "terminated"})
	}
}

func (m *Manager) handleEvent(s *session, msg wireMsg) {
	switch msg.Event {
	case "initialized":
		s.initOnce.Do(func() { close(s.initEvt) })
	case "stopped":
		var body struct {
			ThreadID int `json:"threadId"`
		}
		json.Unmarshal(msg.Body, &body) //nolint
		s.threadID.Store(int32(body.ThreadID))
		go m.resolveStop(s, body.ThreadID)
	case "output":
		var body struct {
			Output string `json:"output"`
		}
		json.Unmarshal(msg.Body, &body) //nolint
		if body.Output != "" {
			m.emit("dap:output", body.Output)
		}
	case "terminated", "exited":
		m.emit("dap:state", State{Status: "terminated"})
	}
}

// resolveStop is where the "do it once in Go" payoff is: stack trace, top
// frame's scopes, and each scope's variables, flattened into one State the
// frontend drops straight into the DOM.
func (m *Manager) resolveStop(s *session, threadID int) {
	stResp, err := s.request("stackTrace", map[string]interface{}{
		"threadId": threadID, "startFrame": 0, "levels": 20,
	})
	if err != nil {
		m.emit("dap:state", State{Status: "error", Error: err.Error()})
		return
	}
	var st struct {
		StackFrames []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Line   int    `json:"line"`
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		} `json:"stackFrames"`
	}
	json.Unmarshal(stResp.Body, &st) //nolint

	frames := make([]Frame, 0, len(st.StackFrames))
	for _, f := range st.StackFrames {
		frames = append(frames, Frame{ID: f.ID, Name: f.Name, Path: f.Source.Path, Line: f.Line})
	}

	var vars []Variable
	if len(st.StackFrames) > 0 {
		scResp, err := s.request("scopes", map[string]interface{}{"frameId": st.StackFrames[0].ID})
		if err == nil {
			var sc struct {
				Scopes []struct {
					Name               string `json:"name"`
					VariablesReference int    `json:"variablesReference"`
				} `json:"scopes"`
			}
			json.Unmarshal(scResp.Body, &sc) //nolint
			for _, scope := range sc.Scopes {
				// ponytail: Locals/Arguments only — Globals/Registers is VS
				// Code's default-collapsed noise too, add a toggle if asked.
				if scope.VariablesReference == 0 || (scope.Name != "Locals" && scope.Name != "Arguments") {
					continue
				}
				vResp, err := s.request("variables", map[string]interface{}{"variablesReference": scope.VariablesReference})
				if err != nil {
					continue
				}
				var vb struct {
					Variables []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
						Type  string `json:"type"`
					} `json:"variables"`
				}
				json.Unmarshal(vResp.Body, &vb) //nolint
				for _, v := range vb.Variables {
					vars = append(vars, Variable{Name: v.Name, Value: v.Value, Type: v.Type})
				}
			}
		}
	}
	m.emit("dap:state", State{Status: "paused", Frames: frames, Variables: vars})
}

// readFrames reads Content-Length-framed JSON messages from r, calling onMsg
// with each body. Same framing as internal/lsp (DAP reuses LSP's transport
// envelope) — duplicated rather than shared since dap.go reads from a TCP
// conn per session, not a per-language stdio pipe, and owns the protocol
// state machine LSP deliberately doesn't.
func readFrames(r io.Reader, onMsg func([]byte)) error {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		length := -1
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return fmt.Errorf("dap: bad Content-Length %q", v)
				}
				length = n
			}
		}
		if length < 0 {
			return fmt.Errorf("dap: missing Content-Length")
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(br, body); err != nil {
			return err
		}
		onMsg(body)
	}
}
