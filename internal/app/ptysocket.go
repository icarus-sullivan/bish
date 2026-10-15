package app

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// ptySubQueue is how many flushed chunks a socket may fall behind before it
// is dropped (the client reconnects and replays the backlog). At up to
// ptyFlushAt bytes per chunk that's far more than a healthy webview lags.
const ptySubQueue = 1024

// ptySub is one websocket-attached xterm. A nil chunk on ch means the
// process exited; ch closing without one means the subscriber was dropped.
type ptySub struct{ ch chan []byte }

// publish fans one flushed chunk out to every socket. Callers hold s.mu.
// chunk is shared read-only between subscribers.
func (s *ptyStream) publish(chunk []byte) {
	for sub := range s.subs {
		select {
		case sub.ch <- chunk:
		default: // hopelessly behind — drop it rather than stall the PTY
			delete(s.subs, sub)
			close(sub.ch)
		}
	}
}

// closeSubs tells every socket the process exited. Callers hold s.mu.
func (s *ptyStream) closeSubs() {
	for sub := range s.subs {
		select {
		case sub.ch <- nil:
		default:
		}
		delete(s.subs, sub)
		close(sub.ch)
	}
}

func (s *ptyStream) unsubscribe(sub *ptySub) {
	s.mu.Lock()
	if _, ok := s.subs[sub]; ok {
		delete(s.subs, sub)
		close(sub.ch)
	}
	s.mu.Unlock()
}

var ptyUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 64 * 1024,
	// the page is served from wails://, never the socket's own origin; the
	// media server's token gates access instead
	CheckOrigin: func(*http.Request) bool { return true },
}

// PTYSocketHandler serves /pty?id=<terminal id> on the loopback media
// server: a binary websocket that replaces the pty:data / WritePTY Wails
// round trips, which JSON-encoded every byte of output and every keystroke.
//
// Server → client: the first binary frame is the backlog replay (possibly
// empty), then live output chunks; a text frame "exit" means the process
// exited, "exited" means it had already exited before this attach (the
// backlog already ends with the exit notice). Client → server: any frame is
// raw input for the PTY.
//
// Not an App method on purpose — Wails would try to bind it.
func PTYSocketHandler(a *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if a.ptyFor(id) == nil {
			http.NotFound(w, r)
			return
		}
		conn, err := ptyUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		s := streamFor(id)
		sub := &ptySub{ch: make(chan []byte, ptySubQueue)}
		s.mu.Lock()
		backlog := append([]byte(nil), s.backlog...)
		exited := s.exited
		if !exited {
			s.subs[sub] = struct{}{}
		}
		s.mu.Unlock()

		// writer: owns all writes to conn
		go func() {
			defer conn.Close()
			write := func(kind int, b []byte) bool {
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint
				return conn.WriteMessage(kind, b) == nil
			}
			if !write(websocket.BinaryMessage, backlog) {
				return
			}
			if exited {
				write(websocket.TextMessage, []byte("exited"))
				return
			}
			for chunk := range sub.ch {
				if chunk == nil {
					write(websocket.TextMessage, []byte("exit"))
					return
				}
				if !write(websocket.BinaryMessage, chunk) {
					return
				}
			}
		}()

		// reader: keystrokes / pastes straight into the PTY
		defer s.unsubscribe(sub)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if p := a.ptyFor(id); p != nil {
				p.Write(data) //nolint
			}
		}
	})
}
