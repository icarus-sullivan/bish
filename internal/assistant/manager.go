// Package assistant drives the Assistant panel's backing agent — either the
// `claude` CLI subprocess (cliBackend) or a local Ollama model with its own
// tool-calling loop (ollamaBackend). Manager is a thin, provider-agnostic
// facade: internal/app only ever talks to Manager, never to a concrete
// backend, so switching providers doesn't touch the Wails-bound surface.
package assistant

import (
	"sync"

	"github.com/csullivan/bish/internal/config"
)

// Backend is one Assistant panel provider. A session id returned by Start is
// only ever valid against the backend that created it — Manager never
// migrates a live session across a provider swap (SetConfig stops the old
// backend's sessions outright instead).
type Backend interface {
	Start(root, permissionMode string) (string, error)
	Send(id, text string) error
	RespondPermission(id, requestID string, allow bool, message string) error
	Interrupt(id string) error
	SwitchMode(id, newMode string) error
	Stop(id string)
	StopAll()
}

type Manager struct {
	mu      sync.Mutex
	backend Backend
	emit    func(event string, data ...interface{})
}

func NewManager(emit func(string, ...interface{}), cfg config.AssistantConfig) *Manager {
	m := &Manager{emit: emit}
	m.backend = newBackend(emit, cfg)
	return m
}

func newBackend(emit func(string, ...interface{}), cfg config.AssistantConfig) Backend {
	if cfg.Provider == "ollama" {
		return newOllamaBackend(emit, cfg)
	}
	return newCLIBackend(emit)
}

// SetConfig swaps the active backend when the provider (or its settings)
// changes. Sessions already running on the old backend are stopped — mode
// switches already tear down and respawn the live process, so this is
// consistent with the existing "changing settings ends the in-flight
// conversation" behavior rather than a new rule.
func (m *Manager) SetConfig(cfg config.AssistantConfig) {
	m.mu.Lock()
	old := m.backend
	m.backend = newBackend(m.emit, cfg)
	m.mu.Unlock()
	old.StopAll()
}

func (m *Manager) current() Backend {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.backend
}

func (m *Manager) Start(root, permissionMode string) (string, error) {
	return m.current().Start(root, permissionMode)
}

func (m *Manager) Send(id, text string) error {
	return m.current().Send(id, text)
}

func (m *Manager) RespondPermission(id, requestID string, allow bool, message string) error {
	return m.current().RespondPermission(id, requestID, allow, message)
}

func (m *Manager) Interrupt(id string) error {
	return m.current().Interrupt(id)
}

func (m *Manager) SwitchMode(id, newMode string) error {
	return m.current().SwitchMode(id, newMode)
}

func (m *Manager) Stop(id string) {
	m.current().Stop(id)
}

func (m *Manager) StopAll() {
	m.current().StopAll()
}
