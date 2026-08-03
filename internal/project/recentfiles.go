package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RecentFile is a standalone file opened without a project context (e.g.
// `bish some/file.go` on the CLI) — distinct from RecentEntry, which tracks
// project directories.
type RecentFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func recentFilesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "recent_files.json")
}

func LoadRecentFiles() ([]*RecentFile, error) {
	data, err := os.ReadFile(recentFilesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []*RecentFile
	err = json.Unmarshal(data, &entries)
	return entries, err
}

func AddRecentFile(path string) error {
	entries, _ := LoadRecentFiles()
	// dedup — remove existing entry for this path
	out := entries[:0]
	for _, e := range entries {
		if e.Path != path {
			out = append(out, e)
		}
	}
	// prepend
	entry := &RecentFile{Path: path, Name: filepath.Base(path)}
	out = append([]*RecentFile{entry}, out...)
	// keep top 10
	if len(out) > 10 {
		out = out[:10]
	}
	dir := filepath.Dir(recentFilesPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(recentFilesPath(), data, 0o644)
}
