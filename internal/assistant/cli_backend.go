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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

	// pendingMu guards the two maps below — both are touched from readLoop
	// and from Wails-bound callers concurrently.
	pendingMu sync.Mutex
	// replies routes control_responses back to the harness-side caller that
	// sent the matching control_request (see cliBackend.control).
	replies map[string]chan controlReply
	// asks remembers every open can_use_tool request: the original tool input
	// and the CLI's own permission_suggestions. A permission answer may only
	// pick suggestions by index from here — the frontend can never mint its
	// own allow-rule — and only AskUserQuestion/ExitPlanMode may have their
	// input rewritten (answers / an edited plan), so an approved Bash command
	// is always exactly the command the user was shown.
	asks map[string]pendingAsk

	initOnce sync.Once
	initDone chan struct{}   // closed when the initialize handshake answers (or the process dies)
	initResp json.RawMessage // initialize's response: commands, models, agents, account
}

type controlReply struct {
	ok   bool
	body json.RawMessage
	err  string
}

type pendingAsk struct {
	toolName    string
	input       json.RawMessage
	suggestions []json.RawMessage
}

// StartOptions is everything a conversation can be spawned with. Every
// field is validated before it reaches the CLI's argv (see validate).
type StartOptions struct {
	PermissionMode string `json:"permissionMode"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`   // "" | low | medium | high | xhigh | max
	Thinking       string `json:"thinking"` // "" (CLI default) | adaptive | disabled
	Resume         string `json:"resume"`   // session UUID to resume
	ResumeAt       string `json:"resumeAt"` // message UUID to truncate the resumed history at
	Fork           bool   `json:"fork"`     // resume into a new session id instead of appending
}

var (
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// model aliases ("opus", "sonnet[1m]") and full ids ("claude-opus-5-5");
	// must not start with '-' so it can never be read as a flag
	modelRe         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\[\]/-]{0,127}$`)
	allowedEffort   = map[string]bool{"": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}
	allowedThinking = map[string]bool{"": true, "adaptive": true, "disabled": true}
)

func (o *StartOptions) validate() error {
	if !allowedModes[o.PermissionMode] {
		return fmt.Errorf("assistant: invalid permission mode %q", o.PermissionMode)
	}
	if o.Model == "default" {
		o.Model = ""
	}
	if o.Model != "" && !modelRe.MatchString(o.Model) {
		return fmt.Errorf("assistant: invalid model %q", o.Model)
	}
	if !allowedEffort[o.Effort] {
		return fmt.Errorf("assistant: invalid effort %q", o.Effort)
	}
	if !allowedThinking[o.Thinking] {
		return fmt.Errorf("assistant: invalid thinking mode %q", o.Thinking)
	}
	if o.Resume != "" && !uuidRe.MatchString(o.Resume) {
		return fmt.Errorf("assistant: invalid session id")
	}
	if o.ResumeAt != "" && (o.Resume == "" || !uuidRe.MatchString(o.ResumeAt)) {
		return fmt.Errorf("assistant: invalid resume point")
	}
	if o.Fork && o.Resume == "" {
		return fmt.Errorf("assistant: fork needs a session to resume")
	}
	return nil
}

// allowedControl is the closed set of harness-initiated control requests the
// frontend may send through Control. Anything that writes settings files,
// authenticates, or reconfigures MCP servers wholesale stays off this list.
var allowedControl = map[string]bool{
	"set_model":               true,
	"set_max_thinking_tokens": true,
	"get_context_usage":       true,
	"mcp_status":              true,
	"mcp_reconnect":           true,
	"mcp_toggle":              true,
	"rewind_files":            true,
	"generate_session_title":  true,
	"stop_task":               true,
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
// at root and returns an opaque session handle.
func (b *cliBackend) Start(root, permissionMode string) (string, error) {
	return b.StartWithOptions(root, StartOptions{PermissionMode: permissionMode})
}

// StartWithOptions spawns a `claude` process for one conversation. The
// process lives for the whole conversation — mode switches, model switches,
// plan approval, and interrupts are all handled in place over the control
// protocol, so the handle never has to be re-keyed by a kill+respawn.
// It returns once the initialize handshake has answered (or timed out), so
// SessionInfo is populated by the time the caller subscribes to events.
func (b *cliBackend) StartWithOptions(root string, o StartOptions) (string, error) {
	if err := o.validate(); err != nil {
		return "", err
	}
	if err := checkRoot(root); err != nil {
		return "", err
	}
	cmd, stdin, stdout, stderr, err := spawn(root, o)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	id := fmt.Sprintf("a%d", b.next)
	b.next++
	s := &cliSession{
		cmd: cmd, stdin: stdin, stderr: stderr, root: root, mode: o.PermissionMode,
		replies: make(map[string]chan controlReply), asks: make(map[string]pendingAsk),
		initDone: make(chan struct{}),
	}
	b.sessions[id] = s
	b.mu.Unlock()
	go b.readLoop(id, s, stdout)
	// Handshake: tells the CLI this harness can answer permission prompts
	// interactively over this stdio. Without it, tools that need approval —
	// ExitPlanMode included — are unusable: the CLI has nowhere to send the
	// ask, so it never exposes the tool, and the model falls back to
	// describing what it would do (e.g. writing a plan to a scratch file)
	// instead of actually calling it.
	go func() {
		body, err := b.control(s, "initialize", nil, 60*time.Second)
		if err == nil {
			s.initResp = body
		}
		s.initOnce.Do(func() { close(s.initDone) })
	}()
	select {
	case <-s.initDone:
	case <-time.After(30 * time.Second):
	}
	return id, nil
}

// checkRoot refuses to spawn anywhere but an existing absolute directory —
// the CLI's working directory is its trust boundary for file access.
func checkRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("assistant: project root must be an absolute path")
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("assistant: project root %q is not a directory", root)
	}
	return nil
}

// SessionInfo returns the initialize handshake's response (slash commands,
// models, agents, account) as raw JSON, or "{}" if it never answered.
func (b *cliBackend) SessionInfo(id string) (string, error) {
	s := b.session(id)
	if s == nil {
		return "", fmt.Errorf("assistant: no session %q", id)
	}
	if len(s.initResp) == 0 {
		return "{}", nil
	}
	return string(s.initResp), nil
}

// Control sends one whitelisted control_request and waits for its answer.
// argsJSON is an object merged into the request body next to subtype.
func (b *cliBackend) Control(id, subtype, argsJSON string) (string, error) {
	if !allowedControl[subtype] {
		return "", fmt.Errorf("assistant: control request %q not allowed", subtype)
	}
	s := b.session(id)
	if s == nil {
		return "", fmt.Errorf("assistant: no session %q", id)
	}
	var extra map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &extra); err != nil {
			return "", fmt.Errorf("assistant: bad control args: %w", err)
		}
		delete(extra, "subtype")
	}
	if subtype == "set_model" {
		if m, _ := extra["model"].(string); m != "" && m != "default" && !modelRe.MatchString(m) {
			return "", fmt.Errorf("assistant: invalid model %q", m)
		}
	}
	body, err := b.control(s, subtype, extra, 2*time.Minute)
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "null", nil
	}
	return string(body), nil
}

// Send writes one stream-json user turn to the session's stdin.
func (b *cliBackend) Send(id, text string) error {
	return b.SendWithImages(id, text, nil)
}

// maxInlineImage caps one inline image; the API rejects larger ones.
const maxInlineImage = 5 << 20

var imageMediaTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp",
}

// SendWithImages writes one user turn with each image attached as an inline
// base64 block, read now — so a dropped screenshot reaches the model even if
// its file is gone by the time the model would have opened it, and without
// a Read permission prompt for a path outside the project. Unreadable or
// unsupported files are skipped (their paths are still listed in the text).
func (b *cliBackend) SendWithImages(id, text string, imagePaths []string) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	content := []map[string]any{{"type": "text", "text": text}}
	for _, p := range imagePaths {
		mt, ok := imageMediaTypes[strings.ToLower(filepath.Ext(p))]
		if !ok {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 || len(data) > maxInlineImage {
			continue
		}
		content = append(content, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": mt,
				"data":       base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	return s.write(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	})
}

// RespondPermission answers a pending `can_use_tool` control_request — the
// plan card's Approve/Reject buttons, and any other tool the CLI paused on
// to ask, route through here. requestID is the control_request's own id, as
// captured off the stream when the ask arrived (see readLoop).
func (b *cliBackend) RespondPermission(id, requestID string, allow bool, message string) error {
	return b.RespondPermissionEx(id, requestID, allow, message, "", nil, false)
}

// rewritableInput lists the tools whose input the user's answer legitimately
// changes: AskUserQuestion carries the chosen answers back in its input, and
// an approved plan may be edited before approval.
var rewritableInput = map[string]bool{"AskUserQuestion": true, "ExitPlanMode": true}

// RespondPermissionEx is RespondPermission with the rest of the decision:
// updatedInputJSON (honored only for rewritableInput tools), suggestionIdx
// (indices into the CLI's own permission_suggestions for this ask — "always
// allow"), and interrupt (deny and stop the turn).
func (b *cliBackend) RespondPermissionEx(id, requestID string, allow bool, message, updatedInputJSON string, suggestionIdx []int, interrupt bool) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	s.pendingMu.Lock()
	ask, ok := s.asks[requestID]
	delete(s.asks, requestID)
	s.pendingMu.Unlock()
	if !ok {
		return fmt.Errorf("assistant: no pending permission request %q", requestID)
	}
	var decision map[string]any
	if allow {
		input := ask.input
		if updatedInputJSON != "" && rewritableInput[ask.toolName] {
			if !json.Valid([]byte(updatedInputJSON)) {
				return fmt.Errorf("assistant: updated input is not valid JSON")
			}
			input = json.RawMessage(updatedInputJSON)
		}
		decision = map[string]any{"behavior": "allow"}
		if len(input) > 0 {
			decision["updatedInput"] = input
		}
		var perms []json.RawMessage
		for _, i := range suggestionIdx {
			if i >= 0 && i < len(ask.suggestions) {
				perms = append(perms, ask.suggestions[i])
			}
		}
		if len(perms) > 0 {
			decision["updatedPermissions"] = perms
		}
	} else {
		if message == "" {
			message = "The user rejected this."
		}
		decision = map[string]any{"behavior": "deny", "message": message}
		if interrupt {
			decision["interrupt"] = true
		}
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
	_, err := b.control(s, "interrupt", nil, 0)
	return err
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
	_, err := b.control(s, "set_permission_mode", map[string]any{"mode": wireMode(newMode)}, 0)
	return err
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

// control sends a harness-initiated control_request on s's stdin. extra is
// merged into the request body alongside subtype. With wait > 0 it blocks
// until the CLI's matching control_response (or the timeout / process exit)
// and returns the response body; with wait == 0 it's fire-and-forget.
func (b *cliBackend) control(s *cliSession, subtype string, extra map[string]any, wait time.Duration) (json.RawMessage, error) {
	req := map[string]any{"subtype": subtype}
	for k, v := range extra {
		req[k] = v
	}
	reqID := fmt.Sprintf("bish-%d", atomic.AddInt64(&b.reqSeq, 1))
	var ch chan controlReply
	if wait > 0 {
		ch = make(chan controlReply, 1)
		s.pendingMu.Lock()
		s.replies[reqID] = ch
		s.pendingMu.Unlock()
		defer func() {
			s.pendingMu.Lock()
			delete(s.replies, reqID)
			s.pendingMu.Unlock()
		}()
	}
	if err := s.write(map[string]any{
		"type":       "control_request",
		"request_id": reqID,
		"request":    req,
	}); err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, nil
	}
	select {
	case r := <-ch:
		if !r.ok {
			if r.err == "" {
				r.err = "request failed"
			}
			return nil, fmt.Errorf("assistant: %s: %s", subtype, r.err)
		}
		return r.body, nil
	case <-time.After(wait):
		return nil, fmt.Errorf("assistant: %s timed out", subtype)
	}
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
func spawn(root string, o StartOptions) (*exec.Cmd, io.WriteCloser, io.Reader, *capBuf, error) {
	args := []string{
		"-p",
		"--verbose", // required for --output-format stream-json in print mode
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--include-partial-messages",
		"--replay-user-messages",
		"--permission-mode", o.PermissionMode,
	}
	if o.PermissionMode == "bypassPermissions" {
		args = append(args, "--allow-dangerously-skip-permissions")
	}
	// every value below passed validate(); the "=" form keeps a value from
	// ever being parsed as a separate flag
	if o.Model != "" {
		args = append(args, "--model="+o.Model)
	}
	if o.Effort != "" {
		args = append(args, "--effort="+o.Effort)
	}
	if o.Thinking != "" {
		args = append(args, "--thinking="+o.Thinking)
	}
	if o.Resume != "" {
		args = append(args, "--resume="+o.Resume)
		if o.ResumeAt != "" {
			args = append(args, "--resume-session-at="+o.ResumeAt)
		}
		if o.Fork {
			args = append(args, "--fork-session")
		}
	}
	// dropped files are stashed outside the project (see app.StashDropped);
	// let the model Read them without a per-file permission prompt
	if home, err := os.UserHomeDir(); err == nil {
		drops := filepath.Join(home, ".config", "bish", "drops")
		if os.MkdirAll(drops, 0o755) == nil {
			args = append(args, "--add-dir", drops)
		}
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = root
	// file checkpointing backs the panel's "restore code to here" — the CLI
	// snapshots files before each edit, keyed by user message uuid
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING=true")
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

// controlProbe is the subset of an inbound control_request /
// control_response this backend acts on — other inbound requests
// (mcp_message, hook_callback, ...) are ignored: bish registers no hooks and
// runs no SDK-side MCP servers, so the CLI has no reason to send them here.
type controlProbe struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype            string            `json:"subtype"`
		ToolName           string            `json:"tool_name"`
		Input              json.RawMessage   `json:"input"`
		ToolUseID          string            `json:"tool_use_id"`
		Title              string            `json:"title"`
		Suggestions        []json.RawMessage `json:"permission_suggestions"`
		BlockedPath        string            `json:"blocked_path"`
		DecisionReason     string            `json:"decision_reason"`
		DecisionReasonType string            `json:"decision_reason_type"`
		SuppressAlways     bool              `json:"suppress_always_allow_rule"`
		DefaultToNo        bool              `json:"default_to_no"`
		McpServer          json.RawMessage   `json:"mcp_server"`
	} `json:"request"`
	Response struct {
		Subtype   string          `json:"subtype"`
		RequestID string          `json:"request_id"`
		Response  json.RawMessage `json:"response"`
		Error     string          `json:"error"`
	} `json:"response"`
}

// readLoop forwards each conversation NDJSON line as an assistant:msg:<id>
// event. control_request/control_response envelopes are protocol plumbing,
// not conversation content: a `can_use_tool` ask is translated into a
// synthetic "permission_request" message the frontend renders as an
// approve/reject card; a control_response is routed to whichever harness
// call is waiting on it; everything else is consumed here.
func (b *cliBackend) readLoop(id string, s *cliSession, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	for scanner.Scan() {
		line := scanner.Bytes()
		var probe controlProbe
		if json.Unmarshal(line, &probe) == nil {
			switch probe.Type {
			case "control_request":
				if probe.Request.Subtype == "can_use_tool" {
					r := probe.Request
					s.pendingMu.Lock()
					s.asks[probe.RequestID] = pendingAsk{toolName: r.ToolName, input: r.Input, suggestions: r.Suggestions}
					s.pendingMu.Unlock()
					data, err := json.Marshal(map[string]any{
						"type":                 "permission_request",
						"request_id":           probe.RequestID,
						"tool_use_id":          r.ToolUseID,
						"tool_name":            r.ToolName,
						"input":                r.Input,
						"title":                r.Title,
						"suggestions":          r.Suggestions,
						"blocked_path":         r.BlockedPath,
						"decision_reason":      r.DecisionReason,
						"decision_reason_type": r.DecisionReasonType,
						"suppress_always":      r.SuppressAlways,
						"default_to_no":        r.DefaultToNo,
						"mcp_server":           r.McpServer,
					})
					if err == nil {
						b.emit("assistant:msg:"+id, string(data))
					}
				}
				continue
			case "control_cancel_request":
				// the CLI withdrew an ask (e.g. the turn was interrupted)
				s.pendingMu.Lock()
				delete(s.asks, probe.RequestID)
				s.pendingMu.Unlock()
				if data, err := json.Marshal(map[string]any{"type": "permission_cancel", "request_id": probe.RequestID}); err == nil {
					b.emit("assistant:msg:"+id, string(data))
				}
				continue
			case "keep_alive":
				continue
			case "control_response":
				r := probe.Response
				s.pendingMu.Lock()
				ch := s.replies[r.RequestID]
				s.pendingMu.Unlock()
				if ch != nil {
					select {
					case ch <- controlReply{ok: r.Subtype == "success", body: r.Response, err: r.Error}:
					default:
					}
				}
				continue
			}
		}
		b.emit("assistant:msg:"+id, string(line))
	}
	s.cmd.Wait() //nolint
	s.initOnce.Do(func() { close(s.initDone) })
	s.pendingMu.Lock()
	for _, ch := range s.replies {
		select {
		case ch <- controlReply{err: "assistant process exited"}:
		default:
		}
	}
	s.pendingMu.Unlock()
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
