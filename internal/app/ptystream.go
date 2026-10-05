package app

import (
	"sync"
	"time"
	"unicode/utf8"

	bishpty "github.com/csullivan/bish/internal/pty"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// ptyCoalesce batches PTY output so escape sequences are rarely split
	// across EventsEmit calls and a flood of tiny reads becomes one event.
	ptyCoalesce = 5 * time.Millisecond
	// ptyFlushAt flushes early once this much output is pending.
	ptyFlushAt = 64 * 1024
	// ptyBacklogMax is how much recent output each terminal keeps for replay
	// to a (re)mounting xterm (see attachPTY).
	ptyBacklogMax = 512 * 1024
)

// ptyStream is one terminal's output fan-out: live events plus a bounded
// backlog. mu serializes flushes against attach so a replay never
// interleaves with (or duplicates) a live event.
type ptyStream struct {
	mu      sync.Mutex
	backlog []byte
}

var (
	ptyStreamsMu sync.Mutex
	ptyStreams   = map[string]*ptyStream{}
)

func streamFor(id string) *ptyStream {
	ptyStreamsMu.Lock()
	defer ptyStreamsMu.Unlock()
	s, ok := ptyStreams[id]
	if !ok {
		s = &ptyStream{}
		ptyStreams[id] = s
	}
	return s
}

func dropStream(id string) {
	ptyStreamsMu.Lock()
	delete(ptyStreams, id)
	ptyStreamsMu.Unlock()
}

// appendBacklog keeps roughly the last ptyBacklogMax bytes, cutting at a
// line boundary (and never mid-rune) so the replay starts on clean output.
// Trimming is amortized — the buffer may grow to 2× before one copy cuts it
// back — so a chatty terminal doesn't memmove 512KB on every 5ms flush.
func (s *ptyStream) appendBacklog(b []byte) {
	s.backlog = append(s.backlog, b...)
	if len(s.backlog) <= 2*ptyBacklogMax {
		return
	}
	cut := len(s.backlog) - ptyBacklogMax
	for i := cut; i < len(s.backlog) && i < cut+4096; i++ {
		if s.backlog[i] == '\n' {
			cut = i + 1
			break
		}
	}
	for cut < len(s.backlog) && !utf8.RuneStart(s.backlog[cut]) {
		cut++
	}
	s.backlog = append(s.backlog[:0:0], s.backlog[cut:]...)
}

// incompleteTail returns how many trailing bytes of b form an unfinished
// UTF-8 sequence. Emitting those would make Wails' JSON encoding replace
// them with U+FFFD, garbling multibyte glyphs (box drawing, emoji, CJK)
// whenever a read boundary splits one.
func incompleteTail(b []byte) int {
	for i := 1; i <= utf8.UTFMax-1 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < utf8.RuneSelf {
			return 0 // ASCII: nothing pending
		}
		if utf8.RuneStart(c) {
			if utf8.FullRune(b[len(b)-i:]) {
				return 0
			}
			return i
		}
	}
	return 0
}

// attachPTY handles the frontend's "pty:attach" event: replay the backlog on
// pty:backlog:<id>. The xterm subscribes to live data first and ignores it
// until this replay lands — anything flushed before the snapshot is in the
// backlog, anything after arrives after it — so a terminal mounted late
// (session restore, hidden tab, closed-and-reopened main tab) starts with
// its full recent output instead of a blank screen.
func (a *App) attachPTY(id string) {
	s := streamFor(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime.EventsEmit(a.ctx, "pty:backlog:"+id, string(s.backlog))
}

func (a *App) readPTYLoopFor(id string, p *bishpty.PTY) {
	dataEvent, exitEvent := "pty:data", "pty:exit"
	if id != "main" {
		dataEvent = "pty:data:" + id
		exitEvent = "pty:exit:" + id
	}
	s := streamFor(id)

	ch := make(chan []byte, 512)

	// reader: push raw chunks into channel as fast as the PTY produces them
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				tmp := make([]byte, n)
				copy(tmp, buf[:n])
				ch <- tmp
			}
			if err != nil {
				close(ch)
				return
			}
		}
	}()

	var pending []byte
	// final=false holds back a split trailing rune for the next flush
	flush := func(final bool) {
		n := len(pending)
		if !final {
			n -= incompleteTail(pending)
		}
		if n <= 0 {
			return
		}
		out := string(pending[:n]) // copies — pending's backing array gets reused below
		s.mu.Lock()
		s.appendBacklog(pending[:n])
		runtime.EventsEmit(a.ctx, dataEvent, out)
		s.mu.Unlock()
		a.liveShare.Broadcast(id, []byte(out)) // no-op unless this terminal is currently shared
		pending = append(pending[:0], pending[n:]...)
	}

	// timer armed only while output is pending — no idle wakeups
	var timer *time.Timer
	var timerC <-chan time.Time
	held := false // last timer flush held back a split rune
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				if timer != nil {
					timer.Stop()
				}
				flush(true)
				s.mu.Lock()
				s.appendBacklog([]byte("\r\n\x1b[2m[process exited]\x1b[0m\r\n"))
				runtime.EventsEmit(a.ctx, exitEvent)
				s.mu.Unlock()
				a.liveShare.Stop(id)
				return
			}
			pending = append(pending, data...)
			if len(pending) >= ptyFlushAt {
				flush(false)
			}
			if len(pending) > 0 && timer == nil {
				timer = time.NewTimer(ptyCoalesce)
				timerC = timer.C
			}
		case <-timerC:
			timer, timerC = nil, nil
			// a rune still incomplete after a whole extra tick is genuinely
			// malformed output — emit it rather than holding it forever
			flush(held)
			held = len(pending) > 0
			if held {
				timer = time.NewTimer(ptyCoalesce)
				timerC = timer.C
			}
		}
	}
}
