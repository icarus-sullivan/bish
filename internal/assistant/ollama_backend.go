// ollamaBackend drives a local Ollama model (e.g. Gemma) through a
// tool-calling agent loop, emitting the same NDJSON message shapes as
// cliBackend (assistant.go equivalent for the `claude` CLI) so
// AssistantPanel.svelte renders it identically without any frontend changes.
//
// Unlike cliBackend, which offloads tool execution and permission gating to
// the `claude` binary itself, there is no external agent here — this file
// *is* the agent: it owns the conversation history, decides when to call a
// tool, executes it, and feeds the result back to the model.
package assistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/config"
)

// requestTimeout bounds a single streamed generation on modest local
// hardware; raise further if a bigger model needs it. It's enforced via
// per-request context (not client.Timeout) so Interrupt/Stop can cut a
// request short before the deadline too.
const requestTimeout = 10 * time.Minute

type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type ollamaMsg struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	Images    []string         `json:"images,omitempty"` // base64, no data-URI prefix — Ollama's vision-model input
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaSession struct {
	mu        sync.Mutex
	root      string
	mode      string
	history   []ollamaMsg
	hasVision bool               // fixed at Start from the backend's capability fetch
	cancel    context.CancelFunc // cancels the in-flight chat request, if any
	seq       int                // guards cancel against a stale goroutine clearing a newer request's cancel func
}

type ollamaBackend struct {
	mu       sync.Mutex
	sessions map[string]*ollamaSession
	next     int
	emit     func(event string, data ...interface{})
	baseURL  string
	model    string
	client   *http.Client

	capsOnce sync.Once
	caps     []string // model capabilities from /api/show, e.g. ["completion","tools","vision"]
}

func newOllamaBackend(emit func(string, ...interface{}), cfg config.AssistantConfig) *ollamaBackend {
	return &ollamaBackend{
		sessions: make(map[string]*ollamaSession),
		emit:     emit,
		baseURL:  strings.TrimRight(cfg.OllamaURL, "/"),
		model:    cfg.OllamaModel,
		client:   &http.Client{}, // timeout enforced per-request via requestTimeout, see chat()
	}
}

// capabilities fetches and caches the configured model's capabilities on
// first use (not at backend construction, so a slow/unreachable Ollama
// server doesn't delay app startup — only the first session Start). A
// failed fetch (old Ollama without this field, wrong URL, etc.) caches an
// empty result rather than retrying every call, and callers treat "unknown"
// the same as "assume the conservative default."
func (b *ollamaBackend) capabilities() []string {
	b.capsOnce.Do(func() {
		if caps, err := GetCapabilities(b.baseURL, b.model); err == nil {
			b.caps = caps
		}
	})
	return b.caps
}

func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// systemPrompt is built from what this specific model actually declares —
// a qwen3 model with only completion/tools/thinking gets a different prompt
// than a gemma model that also has vision, rather than a one-size-fits-all
// description that oversells or undersells what the model can do.
func systemPrompt(root string, caps []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a coding assistant embedded in the bish IDE, working in the project rooted at %s.\n", root)
	b.WriteString("Use the read_file, list_files, and search_files tools to explore the project, and write_file / " +
		"run_shell to make changes, run tests, or run commands. All tool paths are relative to the project root.\n")
	if hasCapability(caps, "vision") {
		b.WriteString("You have vision: read_file also lets you view image files directly, and gives a single " +
			"still-frame preview of video files (no motion).\n")
	} else {
		b.WriteString("You do not have vision in this session — you cannot view images or videos; read_file only " +
			"handles text files for you.\n")
	}
	// No Ollama /api/chat request field carries audio today, regardless of
	// whether the model architecture itself supports it — so this is stated
	// unconditionally rather than keyed off any "audio" capability tag.
	b.WriteString("You do not have audio input available in this environment.\n")
	if hasCapability(caps, "thinking") {
		b.WriteString("Extended thinking/reasoning is available to you for this model.\n")
	}
	b.WriteString("Keep text responses concise.")
	return b.String()
}

func (b *ollamaBackend) session(id string) *ollamaSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessions[id]
}

func (b *ollamaBackend) Start(root, permissionMode string) (string, error) {
	if !allowedModes[permissionMode] {
		return "", fmt.Errorf("assistant: invalid permission mode %q", permissionMode)
	}
	caps := b.capabilities()
	b.mu.Lock()
	id := fmt.Sprintf("o%d", b.next)
	b.next++
	b.sessions[id] = &ollamaSession{
		root:      root,
		mode:      permissionMode,
		history:   []ollamaMsg{{Role: "system", Content: systemPrompt(root, caps)}},
		hasVision: hasCapability(caps, "vision"),
	}
	b.mu.Unlock()
	return id, nil
}

func (b *ollamaBackend) Send(id, text string) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	s.mu.Lock()
	s.history = append(s.history, ollamaMsg{Role: "user", Content: text})
	s.mu.Unlock()
	go b.runTurn(id, s)
	return nil
}

// RespondPermission unblocks a session that paused on a plan card: on
// approval it lets the model know it may now use mutating tools and resumes
// the loop, mirroring cliBackend's real control-protocol response but
// in-process rather than over stdio. requestID is unused here — bish's own
// tool loop has no real control_request ids to echo. Rejection is left to
// the frontend (it just hides the card); there is no pending call on this
// backend that needs unblocking the way the CLI's does.
func (b *ollamaBackend) RespondPermission(id, requestID string, allow bool, message string) error {
	if !allow {
		return nil
	}
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	s.mu.Lock()
	s.mode = "acceptEdits"
	s.history = append(s.history, ollamaMsg{Role: "user", Content: "The plan is approved — proceed with the changes."})
	s.mu.Unlock()
	go b.runTurn(id, s)
	return nil
}

// Interrupt cancels the in-flight chat request for id, if any — runTurn's
// context.Canceled error unwinds it and emits an "Interrupted." result. The
// session's history is untouched (nothing is appended for a failed round),
// so the next Send just picks up the conversation where it left off.
func (b *ollamaBackend) Interrupt(id string) error {
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (b *ollamaBackend) SwitchMode(id, newMode string) error {
	if !allowedModes[newMode] {
		return fmt.Errorf("assistant: invalid permission mode %q", newMode)
	}
	s := b.session(id)
	if s == nil {
		return fmt.Errorf("assistant: no session %q", id)
	}
	s.mu.Lock()
	s.mode = newMode
	s.mu.Unlock()
	return nil
}

func (b *ollamaBackend) Stop(id string) {
	b.mu.Lock()
	s := b.sessions[id]
	delete(b.sessions, id)
	b.mu.Unlock()
	cancelSession(s)
}

func (b *ollamaBackend) StopAll() {
	b.mu.Lock()
	sessions := b.sessions
	b.sessions = make(map[string]*ollamaSession)
	b.mu.Unlock()
	for _, s := range sessions {
		cancelSession(s)
	}
}

func cancelSession(s *ollamaSession) {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// runTurn is the agent loop: ask the model, run whatever tools it calls,
// feed results back, repeat until it answers with plain text. In "plan"
// mode the first mutating tool call in a turn short-circuits into a plan
// card instead of executing, and the loop stops until ApprovePlan resumes it.
func (b *ollamaBackend) runTurn(id string, s *ollamaSession) {
	for {
		s.mu.Lock()
		history := append([]ollamaMsg(nil), s.history...)
		mode := s.mode
		root := s.root
		hasVision := s.hasVision
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		s.mu.Lock()
		s.seq++
		mySeq := s.seq
		s.cancel = cancel
		s.mu.Unlock()

		reply, err := b.chat(ctx, history)

		s.mu.Lock()
		if s.seq == mySeq {
			s.cancel = nil
		}
		s.mu.Unlock()
		cancel()

		if err != nil {
			if errors.Is(err, context.Canceled) {
				b.emitResult(id, true, "Interrupted.")
			} else {
				b.emitResult(id, true, err.Error())
			}
			return
		}

		s.mu.Lock()
		s.history = append(s.history, reply)
		s.mu.Unlock()

		if strings.TrimSpace(reply.Content) != "" {
			b.emitText(id, reply.Content)
		}

		if len(reply.ToolCalls) == 0 {
			b.emitResult(id, false, "")
			return
		}

		for _, tc := range reply.ToolCalls {
			name := tc.Function.Name
			args := tc.Function.Arguments

			if mode == "plan" && isMutating(name) {
				b.emitPlan(id, planSummary(reply.Content, name, args))
				s.mu.Lock()
				s.history = append(s.history, ollamaMsg{
					Role:    "tool",
					Content: "Not executed yet — awaiting user approval of the plan.",
				})
				s.mu.Unlock()
				return
			}

			b.emitToolUse(id, name, args)
			result, err := runTool(root, name, args, hasVision)
			tm := ollamaMsg{Role: "tool"}
			switch {
			case err != nil:
				tm.Content = "error: " + err.Error()
			case result.Image != "":
				tm.Content = "Image attached."
				tm.Images = []string{result.Image}
			default:
				tm.Content = result.Text
			}
			s.mu.Lock()
			s.history = append(s.history, tm)
			s.mu.Unlock()
		}
	}
}

func planSummary(precedingText, toolName string, args map[string]any) string {
	var b strings.Builder
	if strings.TrimSpace(precedingText) != "" {
		b.WriteString(precedingText)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Proposed action: `%s`\n\n", toolName)
	for k, v := range args {
		if k == "content" {
			fmt.Fprintf(&b, "- %s: (new file contents, %d chars)\n", k, len(fmt.Sprint(v)))
			continue
		}
		fmt.Fprintf(&b, "- %s: %v\n", k, v)
	}
	return b.String()
}

type ollamaChatChunk struct {
	Message struct {
		Content   string           `json:"content"`
		ToolCalls []ollamaToolCall `json:"tool_calls"`
	} `json:"message"`
}

// chat sends one full conversation to Ollama's native chat endpoint and
// accumulates the streamed response into a single assistant message. ctx
// cancellation aborts the request and unblocks any in-progress body read
// (wired to Interrupt/Stop via the session's cancel func).
func (b *ollamaBackend) chat(ctx context.Context, history []ollamaMsg) (ollamaMsg, error) {
	reqBody := map[string]any{
		"model":    b.model,
		"messages": history,
		"tools":    toolSchemas(),
		"stream":   true,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return ollamaMsg{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return ollamaMsg{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return ollamaMsg{}, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ollamaMsg{}, fmt.Errorf("ollama: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var content strings.Builder
	var toolCalls []ollamaToolCall
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var chunk ollamaChatChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}
		content.WriteString(chunk.Message.Content)
		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = chunk.Message.ToolCalls
		}
	}
	if err := scanner.Err(); err != nil {
		return ollamaMsg{}, err
	}
	return ollamaMsg{Role: "assistant", Content: content.String(), ToolCalls: toolCalls}, nil
}

func (b *ollamaBackend) emitLine(id string, v map[string]any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.emit("assistant:msg:"+id, string(data))
}

func (b *ollamaBackend) emitText(id, text string) {
	b.emitLine(id, map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": text}}},
	})
}

func (b *ollamaBackend) emitToolUse(id, name string, args map[string]any) {
	b.emitLine(id, map[string]any{
		"type": "assistant",
		"message": map[string]any{"content": []map[string]any{
			{"type": "tool_use", "name": name, "input": args},
		}},
	})
}

func (b *ollamaBackend) emitPlan(id, plan string) {
	b.emitLine(id, map[string]any{
		"type": "assistant",
		"message": map[string]any{"content": []map[string]any{
			{"type": "tool_use", "name": "ExitPlanMode", "input": map[string]any{"plan": plan}},
		}},
	})
}

func (b *ollamaBackend) emitResult(id string, isErr bool, result string) {
	b.emitLine(id, map[string]any{"type": "result", "is_error": isErr, "result": result})
}
