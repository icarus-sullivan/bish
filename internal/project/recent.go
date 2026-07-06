package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type RecentEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func recentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "recent_projects.json")
}

func LoadRecent() ([]*RecentEntry, error) {
	data, err := os.ReadFile(recentPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []*RecentEntry
	err = json.Unmarshal(data, &entries)
	return entries, err
}

func AddRecent(path string) error {
	entries, _ := LoadRecent()
	// dedup — remove existing entry for this path
	out := entries[:0]
	for _, e := range entries {
		if e.Path != path {
			out = append(out, e)
		}
	}
	// prepend
	entry := &RecentEntry{Path: path, Name: filepath.Base(path)}
	out = append([]*RecentEntry{entry}, out...)
	// keep top 10
	if len(out) > 10 {
		out = out[:10]
	}
	dir := filepath.Dir(recentPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(recentPath(), data, 0o644)
}
