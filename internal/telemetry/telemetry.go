// Package telemetry is opt-in, feature-name-only usage counting — no file
// paths, no content, no PII. Off by default; even switched on, nothing
// leaves the machine unless an endpoint is also configured (see
// config.TelemetryConfig's doc comment for why that's two separate gates).
package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type Manager struct {
	mu       sync.Mutex
	counts   map[string]int
	enabled  bool
	endpoint string
	client   *http.Client
}

func NewManager() *Manager {
	return &Manager{counts: map[string]int{}, client: &http.Client{Timeout: 10 * time.Second}}
}

func (m *Manager) SetConfig(enabled bool, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
	m.endpoint = endpoint
}

// Count increments event's counter — a no-op (not even an in-memory
// accumulation) when telemetry is off, so disabling it isn't just "don't
// send," it's "don't do any of the related work at all."
func (m *Manager) Count(event string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled {
		return
	}
	m.counts[event]++
}

// Flush sends accumulated counts to the configured endpoint and resets
// them — a no-op whenever disabled, no endpoint is set, or there's nothing
// to send, all checked before any network code runs.
func (m *Manager) Flush() {
	m.mu.Lock()
	if !m.enabled || m.endpoint == "" || len(m.counts) == 0 {
		m.mu.Unlock()
		return
	}
	payload := m.counts
	m.counts = map[string]int{}
	endpoint := m.endpoint
	client := m.client
	m.mu.Unlock()

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close() //nolint
	}
	// best-effort: a failed send just means those counts are lost, not retried —
	// this is aggregate usage counting, not an event log worth queuing/retrying
}

// StartLoop flushes once a day until ctx is cancelled. Call Flush directly
// once more at shutdown for a final best-effort send of that day's counts.
func (m *Manager) StartLoop(done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				m.Flush()
			}
		}
	}()
}
