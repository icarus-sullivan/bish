package commandcenter

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func definitionPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".bish", "command-center.json")
}

func loadDefinition(projectRoot string) (*Definition, error) {
	def := &Definition{}
	data, err := os.ReadFile(definitionPath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return def, nil
		}
		return def, err
	}
	err = json.Unmarshal(data, def)
	return def, err
}

func saveDefinition(projectRoot string, def *Definition) error {
	p := definitionPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func stateConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "projects")
}

func statePath(projectRoot string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(projectRoot)))
	return filepath.Join(stateConfigDir(), hash+"-cc.json")
}

func loadState(projectRoot string) (*State, error) {
	st := &State{Targets: map[string]*Target{}}
	data, err := os.ReadFile(statePath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return st, err
	}
	if st.Targets == nil {
		st.Targets = map[string]*Target{}
	}
	return st, nil
}

func saveState(projectRoot string, st *State) error {
	if err := os.MkdirAll(stateConfigDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(projectRoot), data, 0o644)
}

// defaultTarget starts a repo off on its main checkout with no services
// ticked (the user picks which to run — unlike command-center, a freshly
// discovered repo has no services defined yet to default-select).
func defaultTarget(r *Repo) *Target {
	return &Target{Mode: "main", Path: r.Path, Branch: r.MainBranch, Env: map[string]string{}}
}
