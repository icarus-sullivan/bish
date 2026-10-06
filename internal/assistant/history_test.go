package assistant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartOptionsValidate(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	ok := []StartOptions{
		{PermissionMode: "plan"},
		{PermissionMode: "acceptEdits", Model: "opus", Effort: "high", Thinking: "adaptive"},
		{PermissionMode: "manual", Model: "sonnet[1m]"},
		{PermissionMode: "plan", Model: "default"},
		{PermissionMode: "plan", Resume: id, ResumeAt: id, Fork: true},
	}
	for _, o := range ok {
		if err := o.validate(); err != nil {
			t.Errorf("%+v: unexpected error %v", o, err)
		}
	}
	bad := []StartOptions{
		{PermissionMode: "yolo"},
		{PermissionMode: "plan", Model: "--dangerously-skip-permissions"},
		{PermissionMode: "plan", Model: "opus --add-dir /"},
		{PermissionMode: "plan", Effort: "extreme"},
		{PermissionMode: "plan", Thinking: "on"},
		{PermissionMode: "plan", Resume: "../../etc/passwd"},
		{PermissionMode: "plan", ResumeAt: id},
		{PermissionMode: "plan", Fork: true},
	}
	for _, o := range bad {
		if err := o.validate(); err == nil {
			t.Errorf("%+v: expected validation error", o)
		}
	}
}

func TestControlWhitelist(t *testing.T) {
	b := newCLIBackend(func(string, ...interface{}) {})
	for _, sub := range []string{"update_settings", "apply_flag_settings", "claude_authenticate", "mcp_set_servers", "initialize"} {
		if _, err := b.Control("a0", sub, ""); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("%s: expected not-allowed error, got %v", sub, err)
		}
	}
}

func TestListAndLoadSessions(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	root := t.TempDir()
	dir := filepath.Join(cfg, "projects", nonAlnum.ReplaceAllString(root, "-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const id = "123e4567-e89b-12d3-a456-426614174000"
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"Active file: x.go\n\n---\n\nfix the bug"},"uuid":"u1"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]},"uuid":"a1"}`,
		`{"type":"user","isSidechain":true,"message":{"role":"user","content":"sub"}}`,
		`{"type":"custom-title","customTitle":"My title","sessionId":"` + id + `"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-uuid names are ignored
	os.WriteFile(filepath.Join(dir, "notes.jsonl"), []byte(lines), 0o644)

	got, err := ListSessions(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != id || got[0].Title != "My title" || got[0].FirstPrompt != "fix the bug" {
		t.Fatalf("unexpected sessions: %+v", got)
	}

	tr, err := LoadTranscript(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tr, `"sub"`) || strings.Contains(tr, "custom-title") || !strings.Contains(tr, `"done"`) {
		t.Fatalf("transcript filtering wrong: %s", tr)
	}
	if _, err := LoadTranscript(root, "../"+id); err == nil {
		t.Fatal("expected invalid id error")
	}
	if _, err := ListSessions("relative/path"); err == nil {
		t.Fatal("expected error for relative root")
	}
}
