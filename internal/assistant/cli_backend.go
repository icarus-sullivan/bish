// cliBackend spawns the `claude` CLI in headless streaming mode and speaks
// its bidirectional control protocol over the same stdio: a plain
// "user"/"assistant"/"result" stream for the conversation, plus
// "control_request"/"control_response" envelopes for everything that isn't
// a conversation turn — the startup handshake, permission asks (including
// ExitPlanMode), live mode switches, and interrupts. Undocumented in
// `claude --help`; reverse-engineered from the `@anthropic-ai/claude-agent-sdk`
// npm package, which drives the same CLI binary the same way.
package assistant

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// maxLine caps a single NDJSON line (a plan can be one long line); a session
// producing more is killed rather than buffered.
const maxLine = 4 << 20

var allowedModes = map[string]bool{
	"plan": true, "acceptEdits": true, "auto": true,
	"bypassPermissions": true, "manual": true, "dontAsk": true,
}

type cliSession struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stderr  *capBuf
	writeMu sync.Mutex
	root    string
	mode    string // permission mode this process is currently running under
	stopped bool   // set before a deliberate kill so the exit isn't reported as a crash
}

// write sends one NDJSON line (a user turn, control_request, or
// control_response) to the live process's stdin.
func (s *cliSession) write(v map[string]any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = fmt.Fprintf(s.stdin, "%s\n", line)
	return err
}

// capBuf keeps only the last limit bytes written — enough to explain why a
// crashed process died without letting a chatty CLI grow this unbounded.
type capBuf struct {
	buf   []byte
	limit int
}

func (c *capBuf) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p...)
	if len(c.buf) > c.limit {
		c.buf = c.buf[len(c.buf)-c.limit:]
	}
	return len(p), nil
}

type cliBackend struct {
	mu       sync.Mutex
	sessions map[string]*cliSession
	next     int
	reqSeq   int64
	emit     func(event string, data ...interface{})
}

func newCLIBackend(emit func(string, ...interface{})) *cliBackend {
	return &cliBackend{sessions: make(map[string]*cliSession), emit: emit}
}

// Start spawns a plan-mode (or other permissionMode) `claude` process rooted
// at root and returns an opaque session handle. The process lives for the
// whole conversation — mode switches, plan approval, and interrupts are all
// handled in place over the control protocol, so the handle never has to be
// re-keyed by a kill+respawn.
func (b *cliBackend) Start(root, permissionMode string) (string, error) {
	if !allowedModes[permissionMode] {
		return "", fmt.Errorf("assistant: invalid permission mode %q", permissionMode)
	}
	cmd, stdin, stdout, stderr, err := spawn(root, permissionMode)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	id := fmt.Sprintf("a%d", b.next)
	b.next++
	s := &cliSession{cmd: cmd, stdin: stdin, stderr: stderr, root: root, mode: permissionMode}
	b.sessions[id] = s
	b.mu.Unlock()
	go b.readLoop(id, s, stdout)
	// Handshake: tells the CLI this harness can answer permission prompts
	// interactively over this stdio. Without it, tools that need approval —
	// ExitPlanMode included — are unusable: the CLI has nowhere to send the
	// ask, so it never exposes the tool, and the model falls back to
	// describing what it would do (e.g. writing a plan to a scratch file)
	// instead of actually calling it.
	b.controlRequest(s, "initialize", nil)
	return id, nil
}

// Send writes one stream-json user turn to the session's stdin.
func (b *cliBackend) Send(id, text string) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	return s.write(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	})
}

// RespondPermission answers a pending `can_use_tool` control_request — the
// plan card's Approve/Reject buttons, and any other tool the CLI paused on
// to ask, route through here. requestID is the control_request's own id, as
// captured off the stream when the ask arrived (see readLoop).
func (b *cliBackend) RespondPermission(id, requestID string, allow bool, message string) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	var decision map[string]any
	if allow {
		decision = map[string]any{"behavior": "allow"}
	} else {
		if message == "" {
			message = "The user rejected this."
		}
		decision = map[string]any{"behavior": "deny", "message": message}
	}
	return s.write(map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response":   decision,
		},
	})
}

// Interrupt stops the in-flight turn without killing the process — the
// control protocol's own interrupt request, answered in place.
func (b *cliBackend) Interrupt(id string) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	return b.controlRequest(s, "interrupt", nil)
}

// SwitchMode changes the live permission mode for id in place. Permission
// mode used to be treated as fixed at process spawn time (forcing a
// kill+--resume to change it); the control protocol's set_permission_mode
// request changes it on the running process instead — the same process
// that's mid-conversation, and mid-ExitPlanMode-ask, keeps running.
func (b *cliBackend) SwitchMode(id, newMode string) error {
	if !allowedModes[newMode] {
		return fmt.Errorf("assistant: invalid permission mode %q", newMode)
	}
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	s.mode = newMode
	return b.controlRequest(s, "set_permission_mode", map[string]any{"mode": wireMode(newMode)})
}

// wireMode translates bish's permission-mode vocabulary to the control
// protocol's. The `claude` CLI's --permission-mode flag accepts "manual" as
// an alias for its internal "default" mode (confirmed via --help); the
// control protocol's set_permission_mode request documents only "default",
// not the alias, so translate explicitly rather than assume the same
// leniency applies over the wire.
func wireMode(mode string) string {
	if mode == "manual" {
		return "default"
	}
	return mode
}

func (b *cliBackend) session(id string) *cliSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessions[id]
}

// controlRequest sends a harness-initiated control_request (initialize,
// set_permission_mode, interrupt) on s's stdin. extra is merged into the
// request body alongside subtype; nil for subtypes that take no fields.
func (b *cliBackend) controlRequest(s *cliSession, subtype string, extra map[string]any) error {
	req := map[string]any{"subtype": subtype}
	for k, v := range extra {
		req[k] = v
	}
	reqID := fmt.Sprintf("bish-%d", atomic.AddInt64(&b.reqSeq, 1))
	return s.write(map[string]any{
		"type":       "control_request",
		"request_id": reqID,
		"request":    req,
	})
}

func (b *cliBackend) Stop(id string) {
	b.mu.Lock()
	s := b.sessions[id]
	delete(b.sessions, id)
	b.mu.Unlock()
	if s != nil {
		b.killLocked(s)
	}
}

func (b *cliBackend) StopAll() {
	b.mu.Lock()
	sessions := b.sessions
	b.sessions = make(map[string]*cliSession)
	b.mu.Unlock()
	for _, s := range sessions {
		b.killLocked(s)
	}
}

func (b *cliBackend) killLocked(s *cliSession) {
	s.stopped = true
	s.stdin.Close()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill() //nolint
	}
}

// spawn starts `claude` in headless NDJSON mode with stream-json on both
// sides of stdio — the same shape the control protocol rides on.
func spawn(root, permissionMode string) (*exec.Cmd, io.WriteCloser, io.Reader, *capBuf, error) {
	args := []string{
		"-p",
		"--verbose", // required for --output-format stream-json in print mode
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--include-partial-messages",
		"--replay-user-messages",
		"--permission-mode", permissionMode,
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = root
	stderr := &capBuf{limit: 4 << 10}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	return cmd, stdin, stdout, stderr, nil
}

// controlRequestProbe is the subset of an inbound control_request this
// backend acts on — everything else (mcp_message, hook_callback,
// request_user_dialog, ...) is ignored: bish registers no hooks, hosts no
// dialogs, and runs no SDK-side MCP servers, so the CLI has no reason to
// send them here.
type controlRequestProbe struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype   string          `json:"subtype"`
		ToolName  string          `json:"tool_name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Title     string          `json:"title"`
	} `json:"request"`
}

// readLoop forwards each conversation NDJSON line as an assistant:msg:<id>
// event. control_request/control_response envelopes are protocol plumbing,
// not conversation content: a `can_use_tool` ask is translated into a
// synthetic "permission_request" message the frontend renders as an
// approve/reject card; every other control message is consumed here and
// never forwarded.
func (b *cliBackend) readLoop(id string, s *cliSession, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		var probe controlRequestProbe
		if json.Unmarshal(line, &probe) == nil {
			switch probe.Type {
			case "control_request":
				if probe.Request.Subtype == "can_use_tool" {
					data, err := json.Marshal(map[string]any{
						"type":        "permission_request",
						"request_id":  probe.RequestID,
						"tool_use_id": probe.Request.ToolUseID,
						"tool_name":   probe.Request.ToolName,
						"input":       probe.Request.Input,
						"title":       probe.Request.Title,
					})
					if err == nil {
						b.emit("assistant:msg:"+id, string(data))
					}
				}
				continue
			case "control_response":
				continue // ack for our own initialize/set_permission_mode/interrupt — nothing to correlate
			}
		}
		b.emit("assistant:msg:"+id, string(line))
	}
	s.cmd.Wait() //nolint
	b.mu.Lock()
	crashed := !s.stopped && b.sessions[id] == s
	if crashed {
		delete(b.sessions, id)
	}
	b.mu.Unlock()
	if crashed {
		b.emit("assistant:exit:"+id, strings.TrimSpace(string(s.stderr.buf)))
	}
}
