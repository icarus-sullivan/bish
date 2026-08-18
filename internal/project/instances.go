package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// instances.json is a live registry of every running bish window (process),
// independent of windows.json (which only tracks windows that have a
// project open — the empty-state welcome window never appears there). It
// exists so that when the Dock-icon-owning window closes, another still-open
// window can be found and promoted to take over Dock/menu-bar
// representation — see App.Shutdown (internal/app/app.go) and the native
// activation-policy handoff in promote_darwin.go.
func instancesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "instances.json")
}

func loadInstances() (map[int]bool, error) {
	data, err := os.ReadFile(instancesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[int]bool{}, nil
		}
		return nil, err
	}
	m := map[int]bool{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[int]bool{}, nil // corrupt file — start fresh rather than fail every launch
	}
	return m, nil
}

// ponytail: no file lock, same accepted risk as windows.go/session.go —
// worst case two windows launching/closing at once drop one registry entry,
// low-stakes (it only affects Dock-icon handoff, not correctness).
func saveInstances(m map[int]bool) error {
	dir := filepath.Dir(instancesPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(instancesPath(), data, 0o644)
}

// RegisterInstance records pid as a live bish window. Called once from
// Startup by every window (whether or not it holds the Dock icon).
func RegisterInstance(pid int) error {
	m, _ := loadInstances()
	m[pid] = true
	return saveInstances(m)
}

// UnregisterInstance drops pid's entry — called once from Shutdown.
func UnregisterInstance(pid int) error {
	m, _ := loadInstances()
	if _, ok := m[pid]; !ok {
		return nil
	}
	delete(m, pid)
	return saveInstances(m)
}

// PickPromotionTarget returns the pid of another live bish window, if any,
// excluding exclude. Dead entries (crashed, or exited without cleanly
// unregistering) are pruned along the way rather than trusted. Used by the
// Dock-icon-owning window's Shutdown to hand off Dock/menu-bar
// representation before it exits.
func PickPromotionTarget(exclude int) (int, bool) {
	m, _ := loadInstances()
	pruned := false
	target, found := 0, false
	for pid := range m {
		if pid == exclude {
			continue
		}
		if !processAlive(pid) {
			delete(m, pid)
			pruned = true
			continue
		}
		if !found {
			target, found = pid, true
		}
	}
	if pruned {
		saveInstances(m) //nolint
	}
	return target, found
}
