package project

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Task is a project-defined, git-shareable run button — distinct from Cmd,
// which is per-user history saved under ~/.config/bish. Checking
// .bish/tasks.json into a repo gives every teammate the same buttons.
type Task struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

type taskFile struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
}

// LoadTasks reads <root>/.bish/tasks.json, if present. IDs are derived from
// content (name+command), not stored, so edits to the file are always the
// source of truth — no UI-side add/rename/delete to keep in sync.
func LoadTasks(root string) []*Task {
	data, err := os.ReadFile(filepath.Join(root, ".bish", "tasks.json"))
	if err != nil {
		return nil
	}
	var raw []taskFile
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	tasks := make([]*Task, 0, len(raw))
	for _, r := range raw {
		if r.Command == "" {
			continue
		}
		cwd := r.Cwd
		if cwd == "" {
			cwd = root
		} else if !filepath.IsAbs(cwd) {
			cwd = filepath.Join(root, cwd)
		}
		name := r.Name
		if name == "" {
			name = r.Command
		}
		tasks = append(tasks, &Task{
			ID:      fmt.Sprintf("%x", md5.Sum([]byte(r.Name+"\x00"+r.Command))),
			Name:    name,
			Command: r.Command,
			Cwd:     cwd,
		})
	}
	return tasks
}
