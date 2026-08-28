// Package commandcenter is a per-project, git-scoped service launcher: define
// repos (auto-discovered from the project's git roots) with launchable
// services, pre-start steps, and cross-repo dependsOn ordering, then start
// them against either a repo's main checkout or a branch worktree.
package commandcenter

// Step is a pre-start command (install/migrate/codegen) run before a repo's
// services.
type Step struct {
	Name    string `json:"name"`
	Cmd     string `json:"cmd"`
	Default bool   `json:"default"`
	// Destructive marks a step that throws data away (db:reset). Never on by
	// default; the UI asks before ticking one.
	Destructive bool `json:"destructive,omitempty"`
	// Supersedes names steps this one makes redundant (db:reset already runs
	// migrations, so having both ticked would migrate twice).
	Supersedes []string `json:"supersedes,omitempty"`
}

// Service is one launchable process inside a repo.
type Service struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
	Port int    `json:"port"`
}

// Repo is a git checkout Command Center can launch services from and create
// worktrees against.
type Repo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Path is the shared checkout, kept on MainBranch.
	Path       string   `json:"path"`
	MainBranch string   `json:"mainBranch"`
	DependsOn  []string `json:"dependsOn"`  // other Repo.ID that must be listening first
	WorktreeIn string   `json:"worktreeIn"` // dir new worktrees are created in
	Prefix     string   `json:"prefix"`     // worktree dir name prefix
	Overrides  string   `json:"overrides"`  // env-override file written on start, relative to worktree
	Setup      string   `json:"setup"`      // one-off command for a fresh worktree
	Link       []string `json:"link"`       // globs symlinked from the main checkout into a worktree
	Copy       []string `json:"copy"`       // globs copied from the main checkout when missing

	Env      map[string]string `json:"env"`
	Steps    []*Step           `json:"steps"`
	Services []*Service        `json:"services"`
}

func (r *Repo) service(name string) *Service {
	for _, s := range r.Services {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (r *Repo) step(name string) *Step {
	for _, s := range r.Steps {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Target is which checkout of a repo to run, and how.
type Target struct {
	Mode string `json:"mode"` // "worktree" | "main" | "off"
	Path string `json:"path"`
	// Branch is the branch picked. For mode "main" it's what start makes sure
	// the shared checkout is on.
	Branch   string            `json:"branch"`
	Services []string          `json:"services"`
	Steps    map[string]bool   `json:"steps,omitempty"` // step name -> run it (missing = repo default)
	Env      map[string]string `json:"env"`
}

// Definition is the git-shareable repo/service/step config for a project —
// <projectRoot>/.bish/command-center.json.
type Definition struct {
	Repos []*Repo `json:"repos"`
}

func (d *Definition) repo(id string) *Repo {
	for _, r := range d.Repos {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// State is per-user runtime selection — which checkout/branch and which
// services/steps are ticked for each repo right now. Not git-shared.
type State struct {
	Targets map[string]*Target `json:"targets"` // keyed by Repo.ID
}

// WorktreeInfo is one entry from `git worktree list`.
type WorktreeInfo struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Main   bool   `json:"main"`
}

// BranchInfo is one selectable branch in the branch picker. Path is set when
// a worktree already holds it, Main when that worktree is the repo's main
// checkout, Remote when the branch only exists as origin/<name>.
type BranchInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Main   bool   `json:"main,omitempty"`
	Remote bool   `json:"remote,omitempty"`
}

// ServiceStatus is the live view of one tracked service or prestart process,
// cross-referenced against the process manager.
type ServiceStatus struct {
	Key       string `json:"key"` // "<repoID>|prestart" or "<repoID>|<service>"
	ProcessID string `json:"processId"`
	PID       int    `json:"pid"`
	Status    string `json:"status"`
	Ports     []int  `json:"ports"`
}

// Snapshot is the combined view pushed to the frontend on "cc:update".
type Snapshot struct {
	Definition *Definition               `json:"definition"`
	State      *State                    `json:"state"`
	Statuses   map[string]*ServiceStatus `json:"statuses"`
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
