package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func sessionPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "session.json")
}

// LoadSession returns the project paths that were open when bish last quit.
func LoadSession() ([]string, error) {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	err = json.Unmarshal(data, &paths)
	return paths, err
}

// ponytail: no file lock — concurrent close of two windows at once can drop
// one entry. Rare and low-stakes (worst case a window isn't restored); add
// flock if it ever bites.
func saveSession(paths []string) error {
	dir := filepath.Dir(sessionPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), data, 0o644)
}

// AddToSession records path as an open project window, so it's restored on
// the next cold start if the window is still open when bish quits.
func AddToSession(path string) error {
	paths, _ := LoadSession()
	for _, p := range paths {
		if p == path {
			return nil
		}
	}
	return saveSession(append(paths, path))
}

// RemoveFromSession drops path — called when its window is closed
// individually (not via app Quit), so it won't be restored.
func RemoveFromSession(path string) error {
	paths, _ := LoadSession()
	out := paths[:0]
	for _, p := range paths {
		if p != path {
			out = append(out, p)
		}
	}
	return saveSession(out)
}
