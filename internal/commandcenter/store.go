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
	normalizeDefinition(def)
	return def, err
}

// normalizeDefinition replaces nil slices with empty ones on every repo —
// a Go nil slice marshals to JSON null, and the frontend indexes these
// fields (.length, .includes) without a null guard, so a null here crashes
// the Svelte render entirely and silently strands the UI on stale state.
// Applies to repos loaded from an existing file (json.Unmarshal populates
// zero-value nil slices for JSON null / absent keys) as well as freshly
// discovered ones.
func normalizeDefinition(def *Definition) {
	for _, r := range def.Repos {
		if r.DependsOn == nil {
			r.DependsOn = []string{}
		}
		if r.Link == nil {
			r.Link = []string{}
		}
		if r.Copy == nil {
			r.Copy = []string{}
		}
		if r.Steps == nil {
			r.Steps = []*Step{}
		}
		if r.Services == nil {
			r.Services = []*Service{}
		}
	}
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
	for _, t := range st.Targets {
		if t.Services == nil {
			t.Services = []string{}
		}
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
	return &Target{Mode: "main", Path: r.Path, Branch: r.MainBranch, Services: []string{}, Env: map[string]string{}}
}
