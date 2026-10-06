package codex

import "testing"

func TestSanitizeParams(t *testing.T) {
	root := "/proj"
	if _, err := sanitizeParams("fs/writeFile", map[string]any{}, root); err == nil {
		t.Fatal("fs/writeFile must be refused")
	}
	p, err := sanitizeParams("thread/start", map[string]any{
		"cwd": "/etc", "sandbox": "workspace-write", "approvalPolicy": "on-request",
		"config": map[string]any{"x": 1}, "developerInstructions": "evil", "model": "gpt-6",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if p["cwd"] != root || p["config"] != nil || p["developerInstructions"] != nil || p["model"] != "gpt-6" {
		t.Fatalf("thread/start not sanitized: %v", p)
	}
	if _, err := sanitizeParams("thread/start", map[string]any{"sandbox": "yolo"}, root); err == nil {
		t.Fatal("bad sandbox accepted")
	}
	if _, err := sanitizeParams("thread/start", map[string]any{"model": "--flag x"}, root); err == nil {
		t.Fatal("bad model accepted")
	}
	if _, err := sanitizeParams("turn/start", map[string]any{
		"threadId": "t1", "input": []any{map[string]any{"type": "mention", "name": "x", "path": "/etc/passwd"}},
	}, root); err == nil {
		t.Fatal("mention outside root accepted")
	}
	if _, err := sanitizeParams("turn/start", map[string]any{
		"threadId": "t1", "input": []any{map[string]any{"type": "localImage", "path": "/tmp/a.sh"}},
	}, root); err == nil {
		t.Fatal("non-image localImage accepted")
	}
	p, err = sanitizeParams("turn/start", map[string]any{
		"threadId": "t1", "input": []any{map[string]any{"type": "text", "text": "hi"}},
		"sandboxPolicy": map[string]any{"type": "workspaceWrite", "writableRoots": []any{"/"}},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if sp := p["sandboxPolicy"].(map[string]any); sp["writableRoots"] != nil {
		t.Fatal("writableRoots must be dropped")
	}
}

func TestSanitizeResponse(t *testing.T) {
	if _, err := sanitizeResponse("item/commandExecution/requestApproval", map[string]any{"decision": "acceptWithExecpolicyAmendment"}); err == nil {
		t.Fatal("amendment decision accepted")
	}
	r, err := sanitizeResponse("item/fileChange/requestApproval", map[string]any{"decision": "accept", "extra": true})
	if err != nil || r["extra"] != nil {
		t.Fatalf("bad: %v %v", r, err)
	}
	if _, err := sanitizeResponse("account/chatgptAuthTokens/refresh", map[string]any{}); err == nil {
		t.Fatal("auth refresh must not be answerable")
	}
}
