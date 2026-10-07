package commandcenter

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/csullivan/bish/internal/health"
)

// effectivePort is a service's port in env — the one function the probe,
// overrides, port badges, and Preview all go through.
func effectivePort(svc *Service, env *Env) int {
	if svc.Port <= 0 {
		return 0
	}
	if env == nil {
		return svc.Port
	}
	return svc.Port + env.PortOffset
}

var envVarUnsafe = regexp.MustCompile(`[^A-Za-z0-9]+`)

// envVarName turns "feather", "api" into "FEATHER_API".
func envVarName(parts ...string) string {
	for i, p := range parts {
		parts[i] = strings.Trim(envVarUnsafe.ReplaceAllString(strings.ToUpper(p), "_"), "_")
	}
	return strings.Join(parts, "_")
}

// subst fills the template variables shared by DB commands and env values.
func subst(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

func dbVars(r *Repo, e *Env) map[string]string {
	vars := map[string]string{"env": e.Name, "db": e.DBName}
	if r.DB != nil {
		vars["file"] = r.DB.File
		vars["template"] = r.DB.Template
		if r.DB.Port > 0 {
			vars["port"] = strconv.Itoa(r.DB.Port)
		}
	}
	return vars
}

// envVars is the env written to r's overrides file for a start in e: the
// repo's and target's own vars plus harnessVars.
func (m *Manager) envVars(def *Definition, e *Env, r *Repo, t *Target) map[string]string {
	env := mergeEnv(r.Env, t.Env, m.harnessVars(def, e, r))
	m.mu.Lock()
	on := m.featureOnLocked(FeatEnvs) || m.featureOnLocked(FeatDB)
	m.mu.Unlock()
	if on {
		vars := dbVars(r, e)
		for k, v := range env {
			env[k] = subst(v, vars)
		}
	}
	return env
}

// harnessVars is what the harness injects for a start of r in e: every on
// service's effective port/URL in the env (so cross-repo callers follow the
// offset) and the env's database URL. Empty with the harness flags off.
func (m *Manager) harnessVars(def *Definition, e *Env, r *Repo) map[string]string {
	m.mu.Lock()
	envsOn := m.featureOnLocked(FeatEnvs)
	dbOn := m.featureOnLocked(FeatDB)
	m.mu.Unlock()
	env := map[string]string{}
	if envsOn {
		for _, rr := range def.Repos {
			rt := e.Targets[rr.ID]
			if rt == nil || rt.Mode == "off" {
				continue
			}
			for _, svc := range rr.Services {
				if p := effectivePort(svc, e); p > 0 {
					env[envVarName(rr.ID, svc.Name, "PORT")] = strconv.Itoa(p)
					env[envVarName(rr.ID, svc.Name, "URL")] = "http://localhost:" + strconv.Itoa(p)
				}
			}
		}
	}
	if dbOn && r.DB != nil && e.DBName != "" && r.DB.URLEnv != "" {
		env[r.DB.URLEnv] = subst(r.DB.URL, dbVars(r, e))
	}
	return env
}

// exportPrefix exports vars ahead of a shell command, so harness-injected
// values reach the process even when the repo has no overrides file.
func exportPrefix(vars map[string]string) string {
	if len(vars) == 0 {
		return ""
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString("export " + k + "=" + shellQuote(vars[k]) + "; ")
	}
	return sb.String()
}

// ListEnvs returns every env, default first.
func (m *Manager) ListEnvs() []*Env {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*Env{}, m.state.Envs...)
}

// SetActiveEnv picks which env the repo cards and Start/Stop all act on.
func (m *Manager) SetActiveEnv(name string) error {
	m.mu.Lock()
	if name != defaultEnv && !m.featureOnLocked(FeatEnvs) {
		m.mu.Unlock()
		return fmt.Errorf("environments are turned off")
	}
	if m.state.env(name) == nil {
		m.mu.Unlock()
		return fmt.Errorf("env %q not found", name)
	}
	m.state.Active = name
	root, st := m.projectRoot, m.state
	m.mu.Unlock()
	return saveState(root, st)
}

var envNameOK = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

// CreateEnv adds an env on branch: every repo the active env runs gets a
// worktree target on branch with the same ticked services/steps, a fresh
// port offset, and (with harnessDb) its own database.
func (m *Manager) CreateEnv(name, branch string) (*Env, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	branch = strings.TrimSpace(branch)
	m.mu.Lock()
	if !m.featureOnLocked(FeatEnvs) {
		m.mu.Unlock()
		return nil, fmt.Errorf("environments are turned off (Settings → Harness: ephemeral environments)")
	}
	dbOn := m.featureOnLocked(FeatDB)
	root, def, st := m.projectRoot, m.def, m.state
	src := m.activeEnvLocked()
	m.mu.Unlock()
	if root == "" {
		return nil, fmt.Errorf("no project open")
	}
	if !envNameOK.MatchString(name) {
		return nil, fmt.Errorf("env name must be lowercase letters, digits and dashes")
	}
	if st.env(name) != nil {
		return nil, fmt.Errorf("env %q already exists", name)
	}
	if branch == "" {
		return nil, fmt.Errorf("pick a branch for env %q", name)
	}
	e := &Env{
		Name:      name,
		Branch:    branch,
		DBName:    "bish_" + strings.ReplaceAll(name, "-", "_"),
		Targets:   map[string]*Target{},
		CreatedAt: time.Now(),
	}
	for _, r := range def.Repos {
		t := &Target{Mode: "off", Branch: branch, Services: []string{}, Env: map[string]string{}}
		if s := src.Targets[r.ID]; s != nil && s.Mode != "off" {
			t.Mode = "worktree"
			t.Services = append([]string{}, s.Services...)
			if s.Steps != nil {
				t.Steps = map[string]bool{}
				for k, v := range s.Steps {
					t.Steps[k] = v
				}
			}
			for k, v := range s.Env {
				t.Env[k] = v
			}
			if r.MainBranch == branch {
				return nil, fmt.Errorf("%s: %s is the main checkout's branch — git can't check it out twice; pick another branch", r.Name, branch)
			}
			for _, other := range st.Envs {
				if ot := other.Targets[r.ID]; ot != nil && ot.Mode != "off" && ot.Branch == branch {
					return nil, fmt.Errorf("%s: branch %s is already checked out by env %q", r.Name, branch, other.Name)
				}
			}
		}
		e.Targets[r.ID] = t
	}
	e.PortOffset = m.allocateOffset(def, st)

	if dbOn {
		for _, r := range def.Repos {
			if r.DB == nil || e.Targets[r.ID].Mode == "off" || strings.TrimSpace(r.DB.Create) == "" {
				continue
			}
			cmd := subst(r.DB.Create, dbVars(r, e))
			if err := m.runOnce(context.Background(), cmd, r.Path, r.Name+" · db create ["+name+"]"); err != nil {
				return nil, fmt.Errorf("%s: create database %s: %w", r.Name, e.DBName, err)
			}
		}
	}

	m.mu.Lock()
	m.state.Envs = append(m.state.Envs, e)
	m.state.Active = e.Name
	m.mu.Unlock()
	return e, saveState(root, st)
}

// allocateOffset scans upward in steps of 100 for an offset no other env
// uses and at which none of the project's declared ports is already bound
// (feather's spinup.py strategy, generalized).
func (m *Manager) allocateOffset(def *Definition, st *State) int {
	used := map[int]bool{}
	for _, e := range st.Envs {
		used[e.PortOffset] = true
	}
	for off := 100; off < 100*100; off += 100 {
		if used[off] {
			continue
		}
		free := true
		for _, r := range def.Repos {
			for _, svc := range r.Services {
				if svc.Port > 0 && health.Dial(svc.Port+off) == nil {
					free = false
				}
			}
		}
		if free {
			return off
		}
	}
	return 100 * 100
}

// DestroyEnv stops an env and forgets it. dropDB runs the repos' declared
// Drop (only ever against a generated bish_ name); removeWorktrees removes
// its worktrees — without --force, so uncommitted work makes it fail loud
// rather than vanish.
func (m *Manager) DestroyEnv(name string, dropDB, removeWorktrees bool) error {
	if name == defaultEnv {
		return fmt.Errorf("the default env can't be destroyed")
	}
	m.mu.Lock()
	e := m.state.env(name)
	root, def := m.projectRoot, m.def
	dbOn := m.featureOnLocked(FeatDB)
	m.mu.Unlock()
	if e == nil {
		return fmt.Errorf("env %q not found", name)
	}
	m.StopEnv(name) //nolint
	var errs []string
	if dropDB && dbOn {
		for _, r := range def.Repos {
			if r.DB == nil || strings.TrimSpace(r.DB.Drop) == "" {
				continue
			}
			if err := m.dropDB(r, e); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if removeWorktrees {
		for _, r := range def.Repos {
			t := e.Targets[r.ID]
			if t == nil || t.Mode != "worktree" || t.Path == "" || t.Path == r.Path || !dirExists(t.Path) {
				continue
			}
			if _, err := git(r.Path, "worktree", "remove", t.Path); err != nil {
				errs = append(errs, fmt.Sprintf("%s: worktree %s kept: %v", r.Name, t.Path, err))
			}
		}
	}
	m.mu.Lock()
	envs := m.state.Envs[:0]
	for _, x := range m.state.Envs {
		if x.Name != name {
			envs = append(envs, x)
		}
	}
	m.state.Envs = envs
	if m.state.Active == name {
		m.state.Active = defaultEnv
	}
	delete(m.drift, name)
	st := m.state
	m.mu.Unlock()
	if err := saveState(root, st); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

var generatedDB = regexp.MustCompile(`^bish_[a-z0-9_]+$`)

// dropDB runs r's Drop for e — the non-negotiable guard: only a generated
// bish_ name, never something a user typed.
func (m *Manager) dropDB(r *Repo, e *Env) error {
	if !generatedDB.MatchString(e.DBName) {
		return fmt.Errorf("%s: refusing to drop %q — only generated bish_ databases are ever dropped", r.Name, e.DBName)
	}
	cmd := subst(r.DB.Drop, dbVars(r, e))
	if err := m.runOnce(context.Background(), cmd, r.Path, r.Name+" · db drop ["+e.Name+"]"); err != nil {
		return fmt.Errorf("%s: drop %s: %w", r.Name, e.DBName, err)
	}
	return nil
}

// runOnce runs cmd through the process manager (so its output is visible
// in the Processes panel) and waits for it to finish.
func (m *Manager) runOnce(ctx context.Context, cmd, dir, name string) error {
	proc, err := m.mgr.Add(cmd, dir, name)
	if err != nil {
		return err
	}
	ok, cancelled := m.awaitExit(ctx, proc.ID)
	if cancelled {
		return ctx.Err()
	}
	if !ok {
		if p := m.mgr.FindByID(proc.ID); p != nil {
			return fmt.Errorf("exit %d", p.ExitCode)
		}
		return fmt.Errorf("failed")
	}
	return nil
}

// ensureInfra brings up r's shared compose stack once per project session
// — shared infra (queues, search, mocks) is started once, not once per env.
func (m *Manager) ensureInfra(ctx context.Context, envName string, r *Repo) error {
	m.mu.Lock()
	on := m.featureOnLocked(FeatHarness)
	done := m.infraUp[r.ID]
	m.mu.Unlock()
	if !on || r.Compose == "" || done {
		return nil
	}
	cmd := "docker compose -f " + shellQuote(r.Compose) + " up -d"
	if err := m.runOnce(ctx, cmd, r.Path, r.Name+" · infra"); err != nil {
		return err
	}
	m.mu.Lock()
	m.infraUp[r.ID] = true
	m.mu.Unlock()
	return nil
}

// awaitDB waits for r's env database to answer its Ready probe, so prestart
// migrations don't race a database that's still coming up.
func (m *Manager) awaitDB(ctx context.Context, envName string, r *Repo, dir string, setAll func(phase, detail string)) error {
	m.mu.Lock()
	on := m.featureOnLocked(FeatDB)
	e := m.state.env(envName)
	m.mu.Unlock()
	if !on || e == nil || e.DBName == "" || r.DB == nil || strings.TrimSpace(r.DB.Ready) == "" {
		return nil
	}
	spec := &health.Spec{Cmd: subst(r.DB.Ready, dbVars(r, e)), TimeoutSec: 120}
	err := health.Wait(ctx, spec, func() health.Target { return health.Target{Dir: dir} }, true, func(err error) {
		setAll(phaseStarting, "waiting for database "+e.DBName)
	})
	if err != nil && ctx.Err() == nil {
		setAll(phaseStarting, "database "+e.DBName+" not ready — starting anyway")
	}
	return err
}

// DefaultDBSpec is the editable starting point for a mode, so picking
// "template" on a Postgres repo is one click rather than three commands.
func DefaultDBSpec(mode string) *DBSpec {
	switch mode {
	case "compose":
		return &DBSpec{
			Mode:   "compose",
			File:   "docker-compose.yml",
			Create: "docker compose -p {{env}} -f {{file}} up -d",
			Drop:   "docker compose -p {{env}} -f {{file}} down -v",
			Ready:  "docker compose -p {{env}} -f {{file}} ps --status running -q | grep -q .",
			URLEnv: "DATABASE_URL",
			URL:    "postgres://localhost:{{port}}/{{db}}",
		}
	default:
		return &DBSpec{
			Mode:     "template",
			Template: "postgres",
			Create:   "createdb -T {{template}} {{db}}",
			Drop:     "dropdb --if-exists {{db}}",
			Ready:    "pg_isready -q",
			URLEnv:   "PGDATABASE",
			URL:      "{{db}}",
		}
	}
}

// reconcile checks each env against disk: a worktree that's gone, or a
// declared port that something else already holds, is reported as drift —
// never silently repaired (repairing a half-deleted worktree is how
// uncommitted work gets lost).
func (m *Manager) reconcile() {
	m.mu.Lock()
	def, st := m.def, m.state
	envs := append([]*Env{}, st.Envs...)
	m.mu.Unlock()
	wts := map[string]map[string]bool{}
	for _, r := range def.Repos {
		set := map[string]bool{}
		if list, err := worktrees(r.Path); err == nil {
			for _, w := range list {
				set[w.Path] = true
			}
		}
		wts[r.ID] = set
	}
	drift := map[string][]string{}
	for _, e := range envs {
		var warn []string
		for _, r := range def.Repos {
			t := e.Targets[r.ID]
			if t == nil || t.Mode != "worktree" || t.Path == "" {
				continue
			}
			if !dirExists(t.Path) {
				warn = append(warn, fmt.Sprintf("%s: worktree %s is gone", r.Name, t.Path))
			} else if !wts[r.ID][t.Path] {
				warn = append(warn, fmt.Sprintf("%s: %s is no longer a git worktree", r.Name, t.Path))
			}
		}
		if e.Name != defaultEnv {
			for _, r := range def.Repos {
				t := e.Targets[r.ID]
				if t == nil || t.Mode == "off" {
					continue
				}
				for _, n := range t.Services {
					if svc := r.service(n); svc != nil {
						if p := effectivePort(svc, e); p > 0 && health.Dial(p) == nil {
							warn = append(warn, fmt.Sprintf("%s %s: port %d is already in use", r.Name, n, p))
						}
					}
				}
			}
		}
		if len(warn) > 0 {
			drift[e.Name] = warn
		}
	}
	m.mu.Lock()
	if m.def == def {
		m.drift = drift
	}
	m.mu.Unlock()
}
