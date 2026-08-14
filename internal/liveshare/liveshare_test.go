package liveshare

import (
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialTestGuest(t *testing.T, shareURL string) (*websocket.Conn, func()) {
	t.Helper()
	u, err := url.Parse(shareURL)
	if err != nil {
		t.Fatalf("bad share URL %q: %v", shareURL, err)
	}
	// localIP() may return a real LAN address the test sandbox can't dial —
	// the server always listens on 0.0.0.0, so localhost reaches it too.
	wsURL := "ws://localhost:" + u.Port() + u.Path + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", wsURL, err)
	}
	return conn, func() { conn.Close() } //nolint
}

func TestLiveShareBroadcastAndPermissionGating(t *testing.T) {
	var mu sync.Mutex
	var events []string
	m := NewManager(func(event string, data ...interface{}) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	var written [][]byte
	var writeMu sync.Mutex
	shareURL, err := m.Start("t1", func(b []byte) error {
		writeMu.Lock()
		cp := append([]byte(nil), b...)
		written = append(written, cp)
		writeMu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !m.IsSharing("t1") {
		t.Fatal("IsSharing(t1) = false after Start")
	}

	conn, closeConn := dialTestGuest(t, shareURL)
	defer closeConn()

	// server -> guest: host output reaches the guest as a binary frame
	m.Broadcast("t1", []byte("hello guest\r\n"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint
	mt, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("guest ReadMessage: %v", err)
	}
	if mt != websocket.BinaryMessage || string(data) != "hello guest\r\n" {
		t.Fatalf("guest got (type=%d) %q, want binary %q", mt, data, "hello guest\r\n")
	}

	// guest -> host while read-only (default): must NOT reach the PTY writer
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ls\n")); err != nil {
		t.Fatalf("guest write: %v", err)
	}
	time.Sleep(150 * time.Millisecond) // let the server's read loop process it
	writeMu.Lock()
	gotWrites := len(written)
	writeMu.Unlock()
	if gotWrites != 0 {
		t.Fatalf("read-only guest's input reached the PTY writer: %v", written)
	}

	// find the guest id the server assigned, then promote it
	guests := m.Guests("t1")
	if len(guests) != 1 {
		t.Fatalf("Guests(t1) = %+v, want exactly one", guests)
	}
	if guests[0].CanType {
		t.Fatal("guest defaulted to can-type=true, want read-only by default")
	}
	if err := m.SetGuestPermission("t1", guests[0].ID, true); err != nil {
		t.Fatalf("SetGuestPermission: %v", err)
	}

	// the permission flip is itself pushed to the guest as a control message
	mt, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("guest ReadMessage (permission): %v", err)
	}
	if mt != websocket.TextMessage || !strings.Contains(string(data), `"canType":true`) {
		t.Fatalf("permission message = (type=%d) %q", mt, data)
	}

	// guest -> host once promoted: must reach the PTY writer
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ls\n")); err != nil {
		t.Fatalf("guest write: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	writeMu.Lock()
	defer writeMu.Unlock()
	if len(written) != 1 || string(written[0]) != "ls\n" {
		t.Fatalf("written = %v, want exactly one write of %q", written, "ls\n")
	}
}

func TestLiveShareStartIsIdempotent(t *testing.T) {
	m := NewManager(func(string, ...interface{}) {})
	defer m.StopAll()
	u1, err := m.Start("t1", func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	u2, err := m.Start("t1", func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if u1 != u2 {
		t.Fatalf("Start called twice for the same terminal returned different links: %q vs %q", u1, u2)
	}
}

func TestLiveShareBroadcastNoOpWhenNotSharing(t *testing.T) {
	m := NewManager(func(string, ...interface{}) {})
	m.Broadcast("nonexistent", []byte("x")) // must not panic
}

// TestEditShareAlwaysRelays covers the one real correctness difference
// between the two session kinds: a read-only guest's bytes must still reach
// the host for an edit session (Yjs sync/awareness has to flow both ways
// for a read-only guest to see live content at all), whereas a read-only
// terminal guest's keystrokes must be dropped (already covered above).
func TestEditShareAlwaysRelays(t *testing.T) {
	m := NewManager(func(string, ...interface{}) {})
	defer m.StopAll()

	var received [][]byte
	var mu sync.Mutex
	shareURL, err := m.StartEdit("/tmp/shared.go", func(b []byte) error {
		mu.Lock()
		received = append(received, append([]byte(nil), b...))
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("StartEdit: %v", err)
	}
	if !m.IsEditSharing("/tmp/shared.go") {
		t.Fatal("IsEditSharing = false after StartEdit")
	}

	conn, closeConn := dialTestGuest(t, shareURL)
	defer closeConn()

	// still read-only (default) — must reach the host anyway for edit sessions
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0, 1, 2}); err != nil {
		t.Fatalf("guest write: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	got := len(received)
	mu.Unlock()
	if got != 1 {
		t.Fatalf("read-only edit-session guest bytes did not reach the host relay: got %d messages, want 1", got)
	}

	// a terminal session keyed "/tmp/shared.go" and an edit session keyed the
	// same path must not collide in the session map
	if _, err := m.Start("/tmp/shared.go", func([]byte) error { return nil }); err != nil {
		t.Fatalf("Start (terminal, same id as edit path): %v", err)
	}
	if !m.IsSharing("/tmp/shared.go") || !m.IsEditSharing("/tmp/shared.go") {
		t.Fatal("terminal and edit sessions with the same id both stopped being tracked — key collision")
	}
}
