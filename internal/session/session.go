package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Entry struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
	CWD  string `json:"cwd"`
}

type Session struct {
	Entries []Entry `json:"entries"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish")
}

func sessionPath() string {
	return filepath.Join(configDir(), "session.json")
}

func Save(entries []Entry) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(Session{Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), data, 0o644)
}

func Load() ([]Entry, error) {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return nil, nil
	}
	var s Session
	err = json.Unmarshal(data, &s)
	return s.Entries, err
}
