// Package liveshare is bish's Live Share pairing feature over a plain
// link — no app install required on the guest's end. Two kinds of session
// share the same relay machinery (Session/guest below, one HTTP+WS server):
//
//   - "terminal" (phase 1): guest is a vendored xterm.js page; guest input
//     writes directly into the real PTY, same as the host typing.
//   - "edit" (phase 2, PM_ASKS.md #12.b): guest is a vendored CodeMirror +
//     Yjs page. Go doesn't understand the Yjs protocol at all — it's a dumb
//     relay of opaque binary blobs between the host frontend (which owns
//     the real Y.Doc) and connected guests, via runtime events in one
//     direction and BroadcastEdit in the other. See frontend/src/lib/coedit.ts
//     and yjsRelay.ts for the actual CRDT sync logic.
//
// Scope, stated plainly: LAN-only (the share link embeds the host's local
// IP; there's no relay or NAT traversal for guests off the local network).
package liveshare

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

//go:embed assets/xterm.js assets/xterm.css assets/guest.html assets/coedit.js assets/coedit.html
var assetsFS embed.FS

var upgrader = websocket.Upgrader{
	// the guest is a plain browser on another machine, not bish's own
	// webview — there's no origin to compare a share link against.
	CheckOrigin: func(r *http.Request) bool { return true },
}

type GuestInfo struct {
	ID      string `json:"id"`
	CanType bool   `json:"canType"`
}

type GuestsUpdate struct {
	TerminalID string      `json:"terminalId"`
	Guests     []GuestInfo `json:"guests"`
}

type EditGuestsUpdate struct {
	Path   string      `json:"path"`
	Guests []GuestInfo `json:"guests"`
}

// EditData is one relayed blob of Yjs protocol bytes from a guest, destined
// for the host frontend's Y.Doc — see App.EditShareBroadcast for the reverse
// direction (host -> guests).
type EditData struct {
	Path string `json:"path"`
	Data string `json:"data"` // base64
}

type guest struct {
	id      string
	conn    *websocket.Conn
	send    chan []byte
	canType bool
}

// Session is one shared resource — a terminal (Kind "terminal") or a file
// being co-edited (Kind "edit"). Same relay machinery either way; only the
// guest page served and the write() semantics differ.
type Session struct {
	Token      string
	TerminalID string // terminal sessions: the terminal id. edit sessions: the file path.
	Kind       string // "terminal" | "edit"
	write      func([]byte) error

	mu     sync.Mutex
	guests map[string]*guest
	nextID int
}

func newSession(kind, id string, write func([]byte) error) *Session {
	b := make([]byte, 16)
	rand.Read(b) //nolint // crypto/rand.Read never errors on a live system
	return &Session{
		Token:      hex.EncodeToString(b),
		TerminalID: id,
		Kind:       kind,
		write:      write,
		guests:     map[string]*guest{},
	}
}

// Broadcast relays host terminal output to every connected guest. Called
// from the same PTY read loop that already feeds the frontend, so guests
// see exactly what the host sees, in the same order.
func (s *Session) Broadcast(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.guests {
		select {
		case g.send <- data:
		default: // guest's outbound buffer is backed up — drop rather than block the host
		}
	}
}

func (s *Session) addGuest(conn *websocket.Conn) *guest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	g := &guest{id: fmt.Sprintf("g%d", s.nextID), conn: conn, send: make(chan []byte, 256)}
	s.guests[g.id] = g
	return g
}

func (s *Session) removeGuest(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.guests[id]; ok {
		close(g.send)
		delete(s.guests, id)
	}
}

func (s *Session) guestList() []GuestInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]GuestInfo, 0, len(s.guests))
	for _, g := range s.guests {
		out = append(out, GuestInfo{ID: g.id, CanType: g.canType})
	}
	return out
}

// setGuestPermission flips one guest's can-type flag and tells that guest's
// own page about it (so its UI/disableStdin updates immediately).
func (s *Session) setGuestPermission(id string, canType bool) bool {
	s.mu.Lock()
	g, ok := s.guests[id]
	if ok {
		g.canType = canType
	}
	s.mu.Unlock()
	if !ok {
		return false
	}
	b, _ := json.Marshal(map[string]any{"type": "permission", "canType": canType})
	select {
	case g.send <- b:
	default:
	}
	return true
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session // terminalID -> session
	byToken  map[string]*Session // token -> session
	server   *http.Server
	port     int
	emit     func(string, ...interface{})
}

func NewManager(emit func(string, ...interface{})) *Manager {
	return &Manager{sessions: map[string]*Session{}, byToken: map[string]*Session{}, emit: emit}
}

// Start begins sharing terminalID (idempotent — returns the existing link
// if already sharing), lazily starting the HTTP+WS server on first use
// across the whole app. write is called with each chunk of guest input;
// callers pass through to the real PTY's Write.
func (m *Manager) Start(terminalID string, write func([]byte) error) (string, error) {
	return m.startLocked(terminalID, terminalID, "terminal", write)
}

// StartEdit begins sharing path for co-editing (phase 2) — same idempotent/
// lazy-server semantics as Start. write receives each blob of guest-sent
// Yjs protocol bytes; the App layer forwards them to the host frontend's
// Y.Doc as a "editshare:data" event rather than writing anywhere itself.
func (m *Manager) StartEdit(path string, write func([]byte) error) (string, error) {
	return m.startLocked(editKey(path), path, "edit", write)
}

func (m *Manager) startLocked(key, id, kind string, write func([]byte) error) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[key]; ok {
		return m.urlForLocked(s.Token), nil
	}
	if m.server == nil {
		if err := m.startServerLocked(); err != nil {
			return "", err
		}
	}
	s := newSession(kind, id, write)
	m.sessions[key] = s
	m.byToken[s.Token] = s
	return m.urlForLocked(s.Token), nil
}

// Stop ends sharing terminalID and disconnects any connected guests.
func (m *Manager) Stop(terminalID string) { m.stopLocked(terminalID) }

// StopEdit ends sharing path for co-editing.
func (m *Manager) StopEdit(path string) { m.stopLocked(editKey(path)) }

func (m *Manager) stopLocked(key string) {
	m.mu.Lock()
	s, ok := m.sessions[key]
	if ok {
		delete(m.sessions, key)
		delete(m.byToken, s.Token)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	s.mu.Lock()
	for _, g := range s.guests {
		g.conn.Close() //nolint
	}
	s.mu.Unlock()
}

func editKey(path string) string { return "edit:" + path }

// StopAll tears down every session and the HTTP server — called on app shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.mu.Lock()
		for _, g := range s.guests {
			g.conn.Close() //nolint
		}
		s.mu.Unlock()
	}
	m.sessions = map[string]*Session{}
	m.byToken = map[string]*Session{}
	if m.server != nil {
		m.server.Close() //nolint
		m.server = nil
	}
}

// Broadcast is a no-op when terminalID isn't currently shared — the common
// case — so the PTY read loop can call this unconditionally on every chunk.
func (m *Manager) Broadcast(terminalID string, data []byte) {
	m.mu.Lock()
	s, ok := m.sessions[terminalID]
	m.mu.Unlock()
	if ok {
		s.Broadcast(data)
	}
}

// BroadcastEdit relays a Yjs update (or sync/awareness message) from the
// host frontend to every guest connected to path's co-editing session —
// same no-op-when-unshared contract as Broadcast.
func (m *Manager) BroadcastEdit(path string, data []byte) {
	m.mu.Lock()
	s, ok := m.sessions[editKey(path)]
	m.mu.Unlock()
	if ok {
		s.Broadcast(data)
	}
}

func (m *Manager) IsSharing(terminalID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[terminalID]
	return ok
}

func (m *Manager) IsEditSharing(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[editKey(path)]
	return ok
}

func (m *Manager) Guests(terminalID string) []GuestInfo {
	m.mu.Lock()
	s, ok := m.sessions[terminalID]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return s.guestList()
}

func (m *Manager) EditGuests(path string) []GuestInfo {
	m.mu.Lock()
	s, ok := m.sessions[editKey(path)]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return s.guestList()
}

func (m *Manager) SetGuestPermission(terminalID, guestID string, canType bool) error {
	m.mu.Lock()
	s, ok := m.sessions[terminalID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("not sharing that terminal")
	}
	if !s.setGuestPermission(guestID, canType) {
		return fmt.Errorf("guest not found")
	}
	m.emit("liveshare:guests", GuestsUpdate{TerminalID: terminalID, Guests: s.guestList()})
	return nil
}

func (m *Manager) SetEditGuestPermission(path, guestID string, canType bool) error {
	m.mu.Lock()
	s, ok := m.sessions[editKey(path)]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("not sharing that file")
	}
	if !s.setGuestPermission(guestID, canType) {
		return fmt.Errorf("guest not found")
	}
	m.emit("editshare:guests", EditGuestsUpdate{Path: path, Guests: s.guestList()})
	return nil
}

func (m *Manager) urlForLocked(token string) string {
	return fmt.Sprintf("http://%s:%d/live/%s", localIP(), m.port, token)
}

func (m *Manager) startServerLocked() error {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return err
	}
	m.port = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("GET /live/assets/xterm.js", serveAsset("assets/xterm.js", "application/javascript"))
	mux.HandleFunc("GET /live/assets/xterm.css", serveAsset("assets/xterm.css", "text/css"))
	mux.HandleFunc("GET /live/assets/coedit.js", serveAsset("assets/coedit.js", "application/javascript"))
	mux.HandleFunc("GET /live/{token}", m.handleGuestPage)
	mux.HandleFunc("GET /live/{token}/ws", m.handleWS)

	m.server = &http.Server{Handler: mux}
	go m.server.Serve(ln) //nolint
	return nil
}

func serveAsset(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(data) //nolint
	}
}

func (m *Manager) handleGuestPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	m.mu.Lock()
	s, ok := m.byToken[token]
	m.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	page := "assets/guest.html"
	if s.Kind == "edit" {
		page = "assets/coedit.html"
	}
	data, err := assetsFS.ReadFile(page)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data) //nolint
}

func (m *Manager) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	m.mu.Lock()
	s, ok := m.byToken[token]
	m.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close() //nolint

	emitGuests := func() {
		if s.Kind == "edit" {
			m.emit("editshare:guests", EditGuestsUpdate{Path: s.TerminalID, Guests: s.guestList()})
		} else {
			m.emit("liveshare:guests", GuestsUpdate{TerminalID: s.TerminalID, Guests: s.guestList()})
		}
	}

	g := s.addGuest(conn)
	emitGuests()
	defer func() {
		s.removeGuest(g.id)
		emitGuests()
	}()

	go func() {
		for data := range g.send {
			mt := websocket.BinaryMessage
			if len(data) > 0 && data[0] == '{' {
				mt = websocket.TextMessage // control messages are JSON objects
			}
			if conn.WriteMessage(mt, data) != nil {
				return
			}
		}
	}()

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
			continue
		}
		s.mu.Lock()
		can := g.canType
		s.mu.Unlock()
		// edit sessions always relay: Yjs sync/awareness messages must reach a
		// read-only guest too (that's how they see live content at all) — the
		// actual read-only enforcement is the guest's CodeMirror being
		// non-editable, not a byte-level gate here. Terminal sessions keep the
		// gate: relaying is literally typing into a live shell.
		if s.Kind == "edit" || can {
			s.write(data) //nolint // best-effort; a write failure just means this keystroke/update is lost
		}
	}
}

// localIP returns the first non-loopback IPv4 address — good enough for a
// LAN share link. Falls back to loopback (a link only the host itself could
// open) rather than failing outright if nothing else is found.
func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return "127.0.0.1"
}
