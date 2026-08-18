package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
)

// windows.json is a live registry of which open bish window (process) has
// which project root open — path -> pid. This is distinct from session.json
// (which paths to restore on the next cold start): this file only matters
// while bish is running, so Open Recent can focus an already-open project's
// window instead of either replacing the current window's project or
// blindly spawning a duplicate.
func windowsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "windows.json")
}

func loadWindows() (map[string]int, error) {
	data, err := os.ReadFile(windowsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]int{}, nil
		}
		return nil, err
	}
	m := map[string]int{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]int{}, nil // corrupt file — start fresh rather than fail every open
	}
	return m, nil
}

// ponytail: no file lock, same accepted risk as session.go — worst case two
// windows opening at once drop one registry entry, low-stakes.
func saveWindows(m map[string]int) error {
	dir := filepath.Dir(windowsPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(windowsPath(), data, 0o644)
}

// RegisterWindow records pid as the window with path open.
func RegisterWindow(path string, pid int) error {
	m, _ := loadWindows()
	m[path] = pid
	return saveWindows(m)
}

// UnregisterWindow drops path's entry — called when its window closes the
// project (switches away, closes it, or the process exits).
func UnregisterWindow(path string) error {
	m, _ := loadWindows()
	if _, ok := m[path]; !ok {
		return nil
	}
	delete(m, path)
	return saveWindows(m)
}

// FindWindowPID returns the pid of the window with path open, if any window
// claiming it is still actually alive. A dead entry (crashed process, or one
// that exited without cleanly unregistering) is pruned and reported as not
// found rather than trusted.
func FindWindowPID(path string) (int, bool) {
	m, _ := loadWindows()
	pid, ok := m[path]
	if !ok {
		return 0, false
	}
	if !processAlive(pid) {
		delete(m, path)
		saveWindows(m) //nolint
		return 0, false
	}
	return pid, true
}

// processAlive uses signal 0 — the standard unix idiom for "does this pid
// exist" without actually signaling it. Every bish window runs as the same
// user (spawned via os.Executable() by the current process), so a live
// process always signals successfully; no permission-denied case to handle.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
