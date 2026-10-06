// Package assistant drives the Assistant panel's backing agent — either the
// `claude` CLI subprocess (cliBackend) or a local Ollama model with its own
// tool-calling loop (ollamaBackend). Manager is a thin, provider-agnostic
// facade: internal/app only ever talks to Manager, never to a concrete
// backend, so switching providers doesn't touch the Wails-bound surface.
package assistant

import (
	"fmt"
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

// optionsStarter / controller / permissionResponder are implemented by the
// `claude` CLI backend only. Ollama sessions fall back to the base Backend
// behavior (or a clear "not supported" error) through the methods below.
type optionsStarter interface {
	StartWithOptions(root string, o StartOptions) (string, error)
}

type controller interface {
	SessionInfo(id string) (string, error)
	Control(id, subtype, argsJSON string) (string, error)
}

type permissionResponder interface {
	RespondPermissionEx(id, requestID string, allow bool, message, updatedInputJSON string, suggestionIdx []int, interrupt bool) error
}

func (m *Manager) StartWithOptions(root string, o StartOptions) (string, error) {
	b := m.current()
	if st, ok := b.(optionsStarter); ok {
		return st.StartWithOptions(root, o)
	}
	return b.Start(root, o.PermissionMode)
}

func (m *Manager) SessionInfo(id string) (string, error) {
	if c, ok := m.current().(controller); ok {
		return c.SessionInfo(id)
	}
	return "{}", nil
}

func (m *Manager) Control(id, subtype, argsJSON string) (string, error) {
	if c, ok := m.current().(controller); ok {
		return c.Control(id, subtype, argsJSON)
	}
	return "", fmt.Errorf("assistant: %s is not supported by this provider", subtype)
}

func (m *Manager) RespondPermissionEx(id, requestID string, allow bool, message, updatedInputJSON string, suggestionIdx []int, interrupt bool) error {
	b := m.current()
	if pr, ok := b.(permissionResponder); ok {
		return pr.RespondPermissionEx(id, requestID, allow, message, updatedInputJSON, suggestionIdx, interrupt)
	}
	return b.RespondPermission(id, requestID, allow, message)
}

func (m *Manager) Send(id, text string) error {
	return m.current().Send(id, text)
}

// imageSender is implemented by backends that can take images inline with a
// user turn (the claude CLI); others just get the text.
type imageSender interface {
	SendWithImages(id, text string, imagePaths []string) error
}

// SendWithImages sends text plus the given image files as inline image
// blocks when the backend supports it, else falls back to plain Send.
func (m *Manager) SendWithImages(id, text string, imagePaths []string) error {
	b := m.current()
	if is, ok := b.(imageSender); ok && len(imagePaths) > 0 {
		return is.SendWithImages(id, text, imagePaths)
	}
	return b.Send(id, text)
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
