package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SavedCommand struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	CWD     string `json:"cwd"`
	Command string `json:"command"`
}

type Store struct {
	Commands []*SavedCommand
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish")
}

func storePath() string {
	return filepath.Join(configDir(), "commands.json")
}

func Load() (*Store, error) {
	s := &Store{}
	data, err := os.ReadFile(storePath())
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	err = json.Unmarshal(data, &s.Commands)
	return s, err
}

func (s *Store) Save() error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.Commands, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storePath(), data, 0o644)
}

func (s *Store) Add(name, cwd, command string) *SavedCommand {
	// deduplicate by command
	for _, c := range s.Commands {
		if c.Command == command {
			return c
		}
	}
	cmd := &SavedCommand{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:    name,
		CWD:     cwd,
		Command: command,
	}
	s.Commands = append(s.Commands, cmd)
	return cmd
}

func (s *Store) Delete(id string) {
	for i, c := range s.Commands {
		if c.ID == id {
			s.Commands = append(s.Commands[:i], s.Commands[i+1:]...)
			return
		}
	}
}

func (s *Store) Edit(id, name, command string) {
	for _, c := range s.Commands {
		if c.ID == id {
			if name != "" {
				c.Name = name
			}
			if command != "" {
				c.Command = command
			}
			return
		}
	}
}
