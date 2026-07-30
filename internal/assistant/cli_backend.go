// cliBackend spawns the `claude` CLI in headless streaming mode and pipes
// newline-delimited JSON between it and the frontend. Each stdout line is
// already a discrete JSON message (unlike LSP's Content-Length framing), so
// the read loop is a plain bufio.Scanner.
package assistant

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
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
	mode    string // permission mode this process was spawned with
	cliID   string // claude's own session_id, captured off the stream; needed for --resume
	stopped bool   // set before a deliberate kill so the exit isn't reported as a crash
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
	emit     func(event string, data ...interface{})
}

func newCLIBackend(emit func(string, ...interface{})) *cliBackend {
	return &cliBackend{sessions: make(map[string]*cliSession), emit: emit}
}

// Start spawns a plan-mode (or other permissionMode) `claude` process rooted
// at root and returns an opaque session handle. The handle stays stable
// across ApprovePlan (which swaps the underlying process but keeps the id),
// so the frontend never has to re-key its UI state mid-conversation.
func (b *cliBackend) Start(root, permissionMode string) (string, error) {
	if !allowedModes[permissionMode] {
		return "", fmt.Errorf("assistant: invalid permission mode %q", permissionMode)
	}
	cmd, stdin, stdout, stderr, err := spawn(root, permissionMode, "", "")
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
	return id, nil
}

// Send writes one stream-json user turn to the session's stdin.
func (b *cliBackend) Send(id, text string) error {
	b.mu.Lock()
	s := b.sessions[id]
	b.mu.Unlock()
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	line, err := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	})
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = fmt.Fprintf(s.stdin, "%s\n", line)
	return err
}

// ApprovePlan kills the plan-mode process for id and replaces it in place
// with a --resume'd process in acceptEdits mode, told to proceed. The
// session id is unchanged so the frontend keeps talking to the same handle.
func (b *cliBackend) ApprovePlan(id string) error {
	b.mu.Lock()
	s := b.sessions[id]
	b.mu.Unlock()
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	if s.cliID == "" {
		return fmt.Errorf("assistant: session %q has no captured session id yet", id)
	}
	return b.resume(id, s, "acceptEdits", "proceed with the plan")
}

// Interrupt stops the in-flight turn for id. If claude's own session id has
// already been captured off the stream, it immediately respawns via
// --resume in the same permission mode (idle, waiting for the next Send) so
// conversation context survives; otherwise the session just ends and the
// next Send starts a fresh one.
func (b *cliBackend) Interrupt(id string) error {
	b.mu.Lock()
	s := b.sessions[id]
	b.mu.Unlock()
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	if s.cliID == "" {
		b.Stop(id)
		return nil
	}
	return b.resume(id, s, s.mode, "")
}

// SwitchMode changes the live permission mode for id: the running process
// (whatever it was doing) is killed and replaced via --resume in newMode.
// Permission mode is fixed at process spawn time — there is no way to
// change it without swapping the process, so this is the only way a
// mid-conversation mode change (the panel's mode pill) actually takes
// effect on the process the model is running in.
func (b *cliBackend) SwitchMode(id, newMode string) error {
	if !allowedModes[newMode] {
		return fmt.Errorf("assistant: invalid permission mode %q", newMode)
	}
	b.mu.Lock()
	s := b.sessions[id]
	b.mu.Unlock()
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	if s.cliID == "" {
		return fmt.Errorf("assistant: session %q has no captured session id yet", id)
	}
	return b.resume(id, s, newMode, "")
}

// resume kills s and replaces the session at id with a freshly --resume'd
// process in newMode — the shared "swap the live process, keep the
// conversation" step behind ApprovePlan, Interrupt, and SwitchMode.
func (b *cliBackend) resume(id string, s *cliSession, newMode, prompt string) error {
	root, cliID := s.root, s.cliID
	b.killLocked(s)
	cmd, stdin, stdout, stderr, err := spawn(root, newMode, cliID, prompt)
	if err != nil {
		b.mu.Lock()
		if b.sessions[id] == s {
			delete(b.sessions, id)
		}
		b.mu.Unlock()
		return err
	}
	ns := &cliSession{cmd: cmd, stdin: stdin, stderr: stderr, root: root, mode: newMode, cliID: cliID}
	b.mu.Lock()
	b.sessions[id] = ns
	b.mu.Unlock()
	go b.readLoop(id, ns, stdout)
	return nil
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

// spawn starts `claude` in headless NDJSON mode. resumeID, when non-empty,
// resumes an existing conversation instead of starting a fresh one. prompt,
// when non-empty, is sent as the initial turn (e.g. "proceed with the
// plan"); left empty, the process just waits idle for the first Send.
func spawn(root, permissionMode, resumeID, prompt string) (*exec.Cmd, io.WriteCloser, io.Reader, *capBuf, error) {
	args := []string{
		"-p",
		"--verbose", // required for --output-format stream-json in print mode
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--include-partial-messages",
		"--replay-user-messages",
		"--permission-mode", permissionMode,
	}
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
	if prompt != "" {
		args = append(args, prompt)
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

// readLoop forwards each NDJSON line as an assistant:msg:<id> event and
// opportunistically captures claude's own session_id (present on every line)
// so a later ApprovePlan can --resume this conversation.
func (b *cliBackend) readLoop(id string, s *cliSession, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		captureSessionID(s, line)
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

func captureSessionID(s *cliSession, line []byte) {
	if s.cliID != "" {
		return
	}
	var probe struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(line, &probe) == nil && probe.SessionID != "" {
		s.cliID = probe.SessionID
	}
}
