// Package lsp spawns language servers and pipes framed JSON-RPC between
// them and the frontend. It is not an LSP client — the protocol lives in
// the frontend (@codemirror/lsp-client); Go only frames stdin/stdout.
package lsp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/config"
	"github.com/csullivan/bish/internal/langext"
)

// maxMessage caps a single Content-Length body; a server sending more is
// killed rather than buffered (old-device memory guard).
const maxMessage = 16 << 20

// maxServers caps concurrent language servers. ponytail: LRU-evict the
// least-recently-used server past 2; raise if trilingual projects hurt.
const maxServers = 2

type server struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	writeMu  sync.Mutex
	root     string
	lastSend time.Time
	stopped  bool // set by Stop/evict so the exit isn't counted as a crash
}

type Manager struct {
	mu        sync.Mutex
	servers   map[string]*server
	fails     map[string]int
	lastFail  map[string]time.Time
	emit      func(event string, data ...interface{})
	overrides map[string]config.LanguageOverride
}

func NewManager(emit func(string, ...interface{})) *Manager {
	return &Manager{
		servers:  make(map[string]*server),
		fails:    make(map[string]int),
		lastFail: make(map[string]time.Time),
		emit:     emit,
	}
}

// SetOverrides installs the user's per-language customizations (custom
// binary path/args, or disabling a server outright) — see
// internal/config.LanguageOverride. Called from App.Startup/SaveConfig,
// same pattern as completion.Manager.SetConfig.
func (m *Manager) SetOverrides(o map[string]config.LanguageOverride) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrides = o
}

// candidatesLocked returns the argv's to try for lang's server, honoring a
// user override (custom path wins outright, disable yields none).
func (m *Manager) candidatesLocked(lang string) [][]string {
	if ov, ok := m.overrides[lang]; ok {
		if ov.DisableServer {
			return nil
		}
		if ov.ServerPath != "" {
			return [][]string{append([]string{ov.ServerPath}, ov.ServerArgs...)}
		}
	}
	def, ok := langext.Get(lang)
	if !ok || def.Server == nil {
		return nil
	}
	return def.Server.Candidates
}

// Start lazily spawns the server for lang rooted at root. Returns false when
// no server is installed, LSP is disabled, or the lang is in crash backoff.
// Idempotent per (lang, root); a different root kills and respawns.
func (m *Manager) Start(lang, root string) bool {
	if os.Getenv("BISH_NO_LSP") != "" {
		return false
	}
	langext.FixPath()
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.servers[lang]; s != nil {
		if s.root == root {
			return true
		}
		m.stopLocked(lang)
	}
	if m.fails[lang] >= 3 && time.Since(m.lastFail[lang]) < time.Minute {
		return false
	}
	if time.Since(m.lastFail[lang]) >= time.Minute {
		m.fails[lang] = 0
	}
	var argv []string
	for _, c := range m.candidatesLocked(lang) {
		if _, err := exec.LookPath(c[0]); err == nil {
			argv = c
			break
		}
	}
	if argv == nil {
		return false
	}
	if len(m.servers) >= maxServers {
		m.evictLRULocked()
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	cmd.Stderr = io.Discard // gopls is chatty; never buffer logs
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	s := &server{cmd: cmd, stdin: stdin, root: root, lastSend: time.Now()}
	m.servers[lang] = s
	go m.readLoop(lang, s, stdout)
	return true
}

// Installed reports whether a server binary for lang is already on PATH,
// without spawning anything — lets the frontend tell "not installed" apart
// from other Start failures (e.g. crash backoff) before offering to install.
func (m *Manager) Installed(lang string) bool {
	langext.FixPath()
	m.mu.Lock()
	candidates := m.candidatesLocked(lang)
	m.mu.Unlock()
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err == nil {
			return true
		}
	}
	return false
}

// Install runs the package-manager command that installs lang's server,
// streaming each output line as an lsp:install-output:<lang> event so the
// frontend can show progress. The installer itself (go/pnpm) must already be
// on PATH.
func (m *Manager) Install(lang string) error {
	def, ok := langext.Get(lang)
	if !ok || def.Server == nil || len(def.Server.Install) == 0 {
		return fmt.Errorf("lsp: no installer for %s", lang)
	}
	langext.FixPath()
	return langext.RunInstaller(def.Server.Install, func(line string) {
		m.emit("lsp:install-output:"+lang, line)
	})
}

// Send frames msg with a Content-Length header and writes it to the server.
func (m *Manager) Send(lang, msg string) error {
	m.mu.Lock()
	s := m.servers[lang]
	if s != nil {
		s.lastSend = time.Now()
	}
	m.mu.Unlock()
	if s == nil {
		return fmt.Errorf("lsp: no server for %s", lang)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := fmt.Fprintf(s.stdin, "Content-Length: %d\r\n\r\n%s", len(msg), msg)
	return err
}

func (m *Manager) Stop(lang string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(lang)
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for lang := range m.servers {
		m.stopLocked(lang)
	}
}

func (m *Manager) stopLocked(lang string) {
	s := m.servers[lang]
	if s == nil {
		return
	}
	s.stopped = true
	delete(m.servers, lang)
	s.stdin.Close()
	s.cmd.Process.Kill() //nolint
}

func (m *Manager) evictLRULocked() {
	var lru string
	var oldest time.Time
	for lang, s := range m.servers {
		if lru == "" || s.lastSend.Before(oldest) {
			lru, oldest = lang, s.lastSend
		}
	}
	if lru != "" {
		m.stopLocked(lru)
	}
}

// readLoop parses Content-Length-framed messages off stdout and emits each
// as an lsp:msg:<lang> event. On exit it reaps the process and reports
// crashes (not deliberate stops) via lsp:down:<lang>.
func (m *Manager) readLoop(lang string, s *server, stdout io.Reader) {
	err := readFrames(stdout, func(body []byte) {
		m.emit("lsp:msg:"+lang, string(body))
	})
	s.cmd.Wait() //nolint
	m.mu.Lock()
	crashed := !s.stopped
	if crashed {
		if m.servers[lang] == s {
			delete(m.servers, lang)
		}
		m.fails[lang]++
		m.lastFail[lang] = time.Now()
	}
	m.mu.Unlock()
	_ = err
	if crashed {
		m.emit("lsp:down:" + lang)
	}
}

// readFrames reads Content-Length-framed JSON-RPC messages from r, calling
// onMsg with each body. Returns on EOF, malformed framing, or oversize body.
func readFrames(r io.Reader, onMsg func([]byte)) error {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		length := -1
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil {
					return fmt.Errorf("lsp: bad Content-Length %q", v)
				}
				length = n
			}
		}
		if length < 0 {
			return fmt.Errorf("lsp: missing Content-Length")
		}
		if length > maxMessage {
			return fmt.Errorf("lsp: message too large (%d)", length)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(br, body); err != nil {
			return err
		}
		onMsg(body)
	}
}
