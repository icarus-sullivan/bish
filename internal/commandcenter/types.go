// Package commandcenter is a per-project, git-scoped service launcher: define
// repos (auto-discovered from the project's git roots) with launchable
// services, pre-start steps, and cross-repo dependsOn ordering, then start
// them against either a repo's main checkout or a branch worktree.
package commandcenter

import (
	"time"

	"github.com/csullivan/bish/internal/health"
)

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
	// Kind is the detector's classification ("install"|"migrate"|"codegen"|
	// "reset"|"build"|"infra"); only "install"/"codegen" are cacheable.
	Kind string `json:"kind,omitempty"`
	// CacheInputs are globs (rel to the checkout) whose stat fingerprint lets
	// a successful run be skipped next time; empty = never cached.
	CacheInputs []string `json:"cacheInputs,omitempty"`
	// Stateful marks a step whose effect lives outside the repo (DB, queue) —
	// never cached, since unchanged inputs don't mean the effect still exists.
	Stateful bool `json:"stateful,omitempty"`
}

// Service is one launchable process inside a repo.
type Service struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
	Port int    `json:"port"`
	// Health declares what "ready" means; nil = a TCP dial on the effective
	// port. The richer probe kinds only run with the harnessHealth flag on.
	Health *health.Spec `json:"health,omitempty"`
	// PortEnv / PortArgs are how a non-zero env port offset reaches the
	// process: PortEnv is exported as the effective port, PortArgs (with
	// {{port}} substituted) is appended to Cmd. A service with neither can
	// only run at offset 0.
	PortEnv  string `json:"portEnv,omitempty"`
	PortArgs string `json:"portArgs,omitempty"`
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

	// Compose is a shared-infra compose file (rel to Path), brought up once
	// per project — not once per env — before any of this repo's services.
	Compose string `json:"compose,omitempty"`
	// DB declares how to make and unmake this repo's per-env database.
	DB *DBSpec `json:"db,omitempty"`
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

// DBSpec declares how to make and unmake a repo's per-env database. Every
// command runs through the shell with {{db}}, {{port}}, {{env}}, {{file}} and
// {{template}} substituted. Mode "template" clones a database on a shared
// server; "compose" brings up a stack scoped by project name.
type DBSpec struct {
	Mode     string `json:"mode"`               // "template" | "compose"
	File     string `json:"file,omitempty"`     // compose mode: compose file, rel to the repo
	Template string `json:"template,omitempty"` // template mode: source database to clone
	Port     int    `json:"port,omitempty"`     // {{port}} in commands/URL
	Create   string `json:"create"`
	Drop     string `json:"drop"`
	Ready    string `json:"ready"`  // exit 0 = usable
	URLEnv   string `json:"urlEnv"` // "DATABASE_URL", or "PGDATABASE"
	URL      string `json:"url"`    // "postgres://localhost:{{port}}/{{db}}", or just "{{db}}"
}

// Env is one named, independently-startable set of checkouts.
type Env struct {
	Name       string             `json:"name"`
	Branch     string             `json:"branch"`           // default branch for every repo's worktree
	PortOffset int                `json:"portOffset"`       // added to each Service.Port
	DBName     string             `json:"dbName,omitempty"` // generated, "bish_<env>"
	Targets    map[string]*Target `json:"targets"`          // keyed by Repo.ID
	CreatedAt  time.Time          `json:"createdAt"`
}

const defaultEnv = "default"

// State is per-user runtime selection — which checkout/branch and which
// services/steps are ticked for each repo, per env. Not git-shared.
type State struct {
	// Targets is the legacy pre-Env single selection: read on load to build
	// the "default" env, never written again after migration.
	Targets map[string]*Target `json:"targets,omitempty"`
	Envs    []*Env             `json:"envs"`
	Active  string             `json:"active"` // Env.Name
}

func (s *State) env(name string) *Env {
	for _, e := range s.Envs {
		if e.Name == name {
			return e
		}
	}
	return nil
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
	Ready     bool   `json:"ready"`
	Phase     string `json:"phase"`  // "starting" | "waiting-deps" | "ready" | "unhealthy" | "stopped"
	Detail    string `json:"detail"` // "waiting for feather :5010 — http 503"
	Port      int    `json:"port"`   // effective port in the active env (0 = none)
}

// Snapshot is the combined view pushed to the frontend on "cc:update".
// State.Targets mirrors the active env's targets, so the repo cards read
// the same shape regardless of how many envs exist.
type Snapshot struct {
	Definition *Definition               `json:"definition"`
	State      *State                    `json:"state"`
	Statuses   map[string]*ServiceStatus `json:"statuses"` // active env only, keyed "<repoID>|<service>"
	Running    map[string]int            `json:"running"`  // env name -> live tracked processes
	Drift      map[string][]string       `json:"drift"`    // env name -> reconcile warnings
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
