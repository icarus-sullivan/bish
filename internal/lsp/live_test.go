package lsp

import (
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

// TestLiveGopls exercises the full pipe against a real gopls: spawn,
// initialize handshake, shutdown. Skipped when gopls isn't installed.
func TestLiveGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	msgs := make(chan string, 16)
	m := NewManager(func(event string, data ...interface{}) {
		if event == "lsp:msg:go" && len(data) == 1 {
			msgs <- data[0].(string)
		}
	})
	defer m.StopAll()
	if !m.Start("go", t.TempDir()) {
		t.Fatal("Start returned false")
	}
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":null,"capabilities":{}}}`
	if err := m.Send("go", init); err != nil {
		t.Fatalf("Send: %v", err)
	}
	deadline := time.After(15 * time.Second)
	for {
		select {
		case raw := <-msgs:
			var resp struct {
				ID     any             `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal([]byte(raw), &resp); err != nil {
				t.Fatalf("bad JSON from server: %v\n%s", err, raw)
			}
			if resp.ID != nil && resp.Result != nil {
				if len(resp.Result) == 0 {
					t.Fatal("empty initialize result")
				}
				return // handshake worked
			}
			// server-initiated notification before the response; keep reading
		case <-deadline:
			t.Fatal("no initialize response within 15s")
		}
	}
}
