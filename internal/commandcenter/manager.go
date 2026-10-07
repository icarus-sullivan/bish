package commandcenter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/health"
	"github.com/csullivan/bish/internal/process"
)

// Manager orchestrates repo discovery, worktree/branch resolution, and
// service launch for the currently open project, on top of the shared
// *process.Manager (so launched services show up in the normal Processes
// panel too).
type Manager struct {
	mu          sync.Mutex
	mgr         *process.Manager
	projectRoot string
	def         *Definition
	state       *State
	// tracked maps "<env>|<repoID>|prestart" / "<env>|<repoID>|<service>" to
	// the current process.Manager process ID, since Add mints its own IDs
	// rather than taking a caller-supplied deterministic one.
	tracked map[string]string
	// health holds each service's readiness phase, keyed like tracked. An
	// entry can exist before its process does (waiting on deps/prestart).
	health map[string]*svcHealth
	// launches cancels a repo's in-flight start pipeline ("<env>|<repoID>").
	launches map[string]context.CancelFunc
	// monitors cancels a service's readiness probe loop (tracked key).
	monitors map[string]context.CancelFunc
	features map[string]bool
	drift    map[string][]string
	infraUp  map[string]bool // repoID -> shared compose brought up this session
}

type svcHealth struct {
	phase  string
	detail string
	ready  bool
	// procID is the process this phase describes; "" while the service is
	// still waiting on deps/prestart (tracked may hold a previous run's ID).
	procID string
}

const (
	phaseStarting    = "starting"
	phaseWaitingDeps = "waiting-deps"
	phaseReady       = "ready"
	phaseUnhealthy   = "unhealthy"
	phaseStopped     = "stopped"
)

func New(mgr *process.Manager) *Manager {
	return &Manager{
		mgr:      mgr,
		def:      &Definition{},
		state:    normalizeState(&State{}),
		tracked:  map[string]string{},
		health:   map[string]*svcHealth{},
		launches: map[string]context.CancelFunc{},
		monitors: map[string]context.CancelFunc{},
		features: map[string]bool{},
		drift:    map[string][]string{},
		infraUp:  map[string]bool{},
	}
}

func key(env, repoID, name string) string { return env + "|" + repoID + "|" + name }

// Load (re)loads the Definition/State for projectRoot and discovers any git
// repos among projectRoot + extraRoots not already known. Pass "" to clear
// (no project open).
func (m *Manager) Load(projectRoot string, extraRoots []string) error {
	m.mu.Lock()
	for _, cancel := range m.launches {
		cancel()
	}
	for _, cancel := range m.monitors {
		cancel()
	}
	m.launches = map[string]context.CancelFunc{}
	m.monitors = map[string]context.CancelFunc{}
	m.health = map[string]*svcHealth{}
	m.tracked = map[string]string{}
	m.drift = map[string][]string{}
	m.infraUp = map[string]bool{}
	m.mu.Unlock()
	if projectRoot == "" {
		m.mu.Lock()
		m.projectRoot = ""
		m.def = &Definition{}
		m.state = normalizeState(&State{})
		m.mu.Unlock()
		return nil
	}
	def, err := loadDefinition(projectRoot)
	if err != nil {
		return err
	}
	st, err := loadState(projectRoot)
	if err != nil {
		return err
	}
	roots := append([]string{projectRoot}, extraRoots...)
	discoverRepos(def, roots)
	fillTargets(def, st)
	m.mu.Lock()
	m.projectRoot = projectRoot
	m.def = def
	m.state = st
	m.mu.Unlock()
	if err := saveDefinition(projectRoot, def); err != nil {
		return err
	}
	if err := saveState(projectRoot, st); err != nil {
		return err
	}
	go m.reconcile()
	return nil
}

// fillTargets gives every env a target for every repo: the default env
// starts a new repo on its main checkout, other envs start it off (the main
// checkout belongs to the default env).
func fillTargets(def *Definition, st *State) {
	for _, e := range st.Envs {
		for _, r := range def.Repos {
			if e.Targets[r.ID] != nil {
				continue
			}
			t := defaultTarget(r)
			if e.Name != defaultEnv {
				t.Mode = "off"
			}
			e.Targets[r.ID] = t
		}
	}
}

// activeEnvLocked is the env the UI and the un-named Start/Stop calls act
// on. With envs off it's always the default env. Caller holds m.mu.
func (m *Manager) activeEnvLocked() *Env {
	if m.featureOnLocked(FeatEnvs) {
		if e := m.state.env(m.state.Active); e != nil {
			return e
		}
	}
	return m.state.env(defaultEnv)
}

func (m *Manager) activeEnvName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeEnvLocked().Name
}

// Snapshot is the combined definition/state/live-status view pushed to the
// frontend.
func (m *Manager) Snapshot() *Snapshot {
	m.mu.Lock()
	def, st := m.def, m.state
	active := m.activeEnvLocked()
	tracked := make(map[string]string, len(m.tracked))
	for k, v := range m.tracked {
		tracked[k] = v
	}
	hs := make(map[string]svcHealth, len(m.health))
	for k, v := range m.health {
		hs[k] = *v
	}
	drift := make(map[string][]string, len(m.drift))
	for k, v := range m.drift {
		drift[k] = v
	}
	m.mu.Unlock()

	byID := make(map[string]*process.Process)
	for _, p := range m.mgr.List() {
		byID[p.ID] = p
	}
	running := map[string]int{}
	statuses := map[string]*ServiceStatus{}
	prefix := active.Name + "|"
	for k, id := range tracked {
		p := byID[id]
		if p == nil {
			continue
		}
		env := k[:strings.Index(k, "|")]
		if p.Status == process.StatusRunning {
			running[env]++
		}
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		ss := &ServiceStatus{Key: strings.TrimPrefix(k, prefix), ProcessID: p.ID, PID: p.PID, Status: string(p.Status), Ports: p.Ports}
		h, ok := hs[k]
		if ok && h.procID != p.ID {
			// a new start is pending; the tracked process is the previous
			// run's — keep its logs reachable but show the pending phase
			ss.Status, ss.PID, ss.Ports = "", 0, nil
		}
		if ok {
			ss.Phase, ss.Detail, ss.Ready = h.phase, h.detail, h.ready
		}
		if ss.Status != "" && p.Status != process.StatusRunning {
			ss.Phase, ss.Ready = phaseStopped, false
		}
		statuses[ss.Key] = ss
	}
	// services still waiting on deps/prestart have a phase but no process yet
	for k, h := range hs {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		sk := strings.TrimPrefix(k, prefix)
		if _, ok := statuses[sk]; !ok {
			statuses[sk] = &ServiceStatus{Key: sk, Phase: h.phase, Detail: h.detail, Ready: h.ready}
		}
	}
	for sk, ss := range statuses {
		repoID, svcName, _ := strings.Cut(sk, "|")
		if r := def.repo(repoID); r != nil {
			if svc := r.service(svcName); svc != nil {
				ss.Port = effectivePort(svc, active)
			}
		}
	}
	view := &State{Targets: active.Targets, Envs: st.Envs, Active: active.Name}
	return &Snapshot{Definition: def, State: view, Statuses: statuses, Running: running, Drift: drift}
}

func (m *Manager) SaveDefinition(def *Definition) error {
	normalizeDefinition(def)
	m.mu.Lock()
	root := m.projectRoot
	if root == "" {
		m.mu.Unlock()
		return fmt.Errorf("no project open")
	}
	m.def = def
	fillTargets(def, m.state)
	st := m.state
	m.mu.Unlock()
	if err := saveState(root, st); err != nil {
		return err
	}
	return saveDefinition(root, def)
}

// SetTarget replaces a repo's target in the active env.
func (m *Manager) SetTarget(repoID string, t *Target) error {
	if t == nil {
		return fmt.Errorf("no target")
	}
	normalizeTarget(t)
	m.mu.Lock()
	root := m.projectRoot
	if root == "" {
		m.mu.Unlock()
		return fmt.Errorf("no project open")
	}
	if m.def.repo(repoID) == nil {
		m.mu.Unlock()
		return fmt.Errorf("repo %s not found", repoID)
	}
	m.activeEnvLocked().Targets[repoID] = t
	st := m.state
	m.mu.Unlock()
	return saveState(root, st)
}

func (m *Manager) Branches(repoID string) ([]BranchInfo, error) {
	m.mu.Lock()
	r := m.def.repo(repoID)
	m.mu.Unlock()
	if r == nil {
		return nil, fmt.Errorf("repo %s not found", repoID)
	}
	return branches(r.Path, r.MainBranch)
}

// RefreshRepo fetches the repo's main branch from origin, so the branch
// picker's remote-branch list and worktree base ref are current.
func (m *Manager) RefreshRepo(repoID string) error {
	m.mu.Lock()
	r := m.def.repo(repoID)
	m.mu.Unlock()
	if r == nil {
		return fmt.Errorf("repo %s not found", repoID)
	}
	_, err := git(r.Path, "fetch", "origin", r.MainBranch)
	return err
}

func (m *Manager) StartAll() error { return m.StartEnv(m.activeEnvName()) }

func (m *Manager) StopAll() { m.StopEnv(m.activeEnvName()) } //nolint

func (m *Manager) StartRepo(repoID string) error {
	return m.startRepo(m.activeEnvName(), repoID, nil)
}

func (m *Manager) StartService(repoID, service string) error {
	return m.startRepo(m.activeEnvName(), repoID, []string{service})
}

func (m *Manager) StopRepo(repoID string) error {
	m.stopRepo(m.activeEnvName(), repoID)
	return nil
}

// StartEnv starts every on repo in env, dependencies first.
func (m *Manager) StartEnv(name string) error {
	m.mu.Lock()
	def := m.def
	e := m.state.env(name)
	envsOn := m.featureOnLocked(FeatEnvs)
	m.mu.Unlock()
	if e == nil {
		return fmt.Errorf("env %q not found", name)
	}
	if name != defaultEnv && !envsOn {
		return fmt.Errorf("environments are turned off (Settings → Harness: ephemeral environments)")
	}
	if err := checkPortable(def, e, nil); err != nil {
		return err
	}
	var errs []string
	for _, id := range startOrder(def, e.Targets) {
		t := e.Targets[id]
		if t == nil || t.Mode == "off" {
			continue
		}
		if err := m.startRepo(name, id, nil); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// StopEnv stops every repo in env, leaving every other env running.
func (m *Manager) StopEnv(name string) error {
	m.mu.Lock()
	def := m.def
	m.mu.Unlock()
	for _, r := range def.Repos {
		m.stopRepo(name, r.ID)
	}
	return nil
}

// checkPortable refuses to run a service at a non-zero port offset when it
// has no way to receive the offset port — it would bind its declared port
// and collide with (or be killed as) the sibling env's copy. only limits
// the check to named services; nil = every ticked service in every on repo.
func checkPortable(def *Definition, e *Env, only map[string][]string) error {
	if e.PortOffset == 0 {
		return nil
	}
	var bad []string
	for _, r := range def.Repos {
		t := e.Targets[r.ID]
		if t == nil || t.Mode == "off" {
			continue
		}
		names := t.Services
		if only != nil {
			if names = only[r.ID]; names == nil {
				continue
			}
		}
		for _, n := range names {
			svc := r.service(n)
			if svc != nil && svc.Port > 0 && svc.PortEnv == "" && svc.PortArgs == "" {
				bad = append(bad, fmt.Sprintf("%s/%s (hardcoded :%d)", r.Name, svc.Name, svc.Port))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("env %q runs at port offset +%d, but %s can't take a different port — set portEnv (e.g. PORT) or portArgs (e.g. --port {{port}}) on the service", e.Name, e.PortOffset, strings.Join(bad, ", "))
	}
	return nil
}

func (m *Manager) startRepo(envName, repoID string, only []string) error {
	m.mu.Lock()
	r := m.def.repo(repoID)
	e := m.state.env(envName)
	def, st, root := m.def, m.state, m.projectRoot
	m.mu.Unlock()
	if root == "" {
		return fmt.Errorf("no project open")
	}
	if r == nil {
		return fmt.Errorf("repo %s not found", repoID)
	}
	if e == nil {
		return fmt.Errorf("env %q not found", envName)
	}
	t := e.Targets[repoID]
	if t == nil || t.Mode == "off" || t.Mode == "" {
		return fmt.Errorf("%s is off — pick a checkout mode first", r.Name)
	}
	wanted := wantedServices(r, t, only)
	if len(wanted) == 0 {
		return fmt.Errorf("no services ticked for %s", r.Name)
	}
	if err := checkPortable(def, e, map[string][]string{repoID: wanted}); err != nil {
		return err
	}

	dir, err := m.resolveDir(r, t)
	if err != nil {
		return err
	}
	saveState(root, st) //nolint — best effort; persists any worktree path resolved above

	env := m.envVars(def, e, r, t)
	linkShared(r, dir)
	copyShared(r, dir)
	if err := writeOverrides(dir, r.Overrides, env); err != nil {
		return fmt.Errorf("%s: write %s: %w", r.Name, r.Overrides, err)
	}

	// Replace what this start covers: just the named services on a targeted
	// start, all of the repo's on a full one — a single-service restart
	// shouldn't kill its siblings.
	m.stopServices(envName, repoID, only)

	ctx, cancel := context.WithCancel(context.Background())
	lk := envName + "|" + repoID
	m.mu.Lock()
	if prev := m.launches[lk]; prev != nil {
		prev()
	}
	m.launches[lk] = cancel
	gated := m.gatedLocked(def, e, r)
	for _, n := range wanted {
		phase, detail := phaseStarting, ""
		if gated {
			phase, detail = phaseWaitingDeps, "waiting for "+strings.Join(r.DependsOn, ", ")
		}
		m.health[key(envName, repoID, n)] = &svcHealth{phase: phase, detail: detail}
	}
	m.mu.Unlock()
	go m.launch(ctx, envName, r, dir, wanted)
	return nil
}

// wantedServices is the start set in the repo's defined service order, not
// toggle-click order.
func wantedServices(r *Repo, t *Target, only []string) []string {
	wanted := t.Services
	if len(only) > 0 {
		wanted = only
	}
	var list []string
	for _, svc := range r.Services {
		if contains(wanted, svc.Name) {
			list = append(list, svc.Name)
		}
	}
	return list
}

// gatedLocked reports whether r has an on dependency with a ticked service
// in env, i.e. whether its start has to wait on anything.
func (m *Manager) gatedLocked(def *Definition, e *Env, r *Repo) bool {
	for _, dep := range r.DependsOn {
		dt := e.Targets[dep]
		if dep != r.ID && dt != nil && dt.Mode != "off" && def.repo(dep) != nil && len(dt.Services) > 0 {
			return true
		}
	}
	return false
}

// launch is a repo's start pipeline, run off the caller's goroutine: shared
// infra → the env's database → dependencies ready → prestart → services.
// Any stage can be cancelled by a newer start or a stop of the same repo.
func (m *Manager) launch(ctx context.Context, envName string, r *Repo, dir string, wanted []string) {
	setAll := func(phase, detail string) {
		m.mu.Lock()
		for _, n := range wanted {
			if h := m.health[key(envName, r.ID, n)]; h != nil {
				h.phase, h.detail = phase, detail
			}
		}
		m.mu.Unlock()
	}

	if err := m.ensureInfra(ctx, envName, r); err != nil {
		if ctx.Err() != nil {
			return
		}
		setAll(phaseStarting, "infra: "+err.Error()) // degrade: shared infra may already be up some other way
	}
	if err := m.awaitDB(ctx, envName, r, dir, setAll); err != nil && ctx.Err() != nil {
		return
	}
	if !m.awaitDeps(ctx, envName, r, setAll) {
		return
	}

	m.mu.Lock()
	var cache *stepCacher
	if m.featureOnLocked(FeatCache) {
		cache = newStepCacher(dir)
	}
	def := m.def
	e := m.state.env(envName)
	var t *Target
	if e != nil {
		t = e.Targets[r.ID]
	}
	m.mu.Unlock()
	if t == nil {
		return
	}
	exports := exportPrefix(m.harnessVars(def, e, r))
	pre, _, ran := prestartCmd(r, t, dir, cache)
	if pre != "" {
		pre = exports + pre
		setAll(phaseStarting, "prestart running")
		proc, err := m.mgr.Add(pre, dir, r.Name+" · prestart"+branchSuffix(dir))
		if err != nil {
			setAll(phaseStopped, "prestart: "+err.Error())
			return
		}
		m.trackSet(key(envName, r.ID, "prestart"), proc.ID)
		ok, cancelled := m.awaitExit(ctx, proc.ID)
		if cancelled {
			return
		}
		if !ok {
			// crashed, killed, or missing — services never start; the
			// failure is visible on the prestart process itself
			setAll(phaseStopped, "prestart failed — see its output")
			return
		}
		if cache != nil {
			for _, st := range ran {
				cache.record(st)
			}
		}
	}

	// Re-read the target: prestart can take minutes, and a save in the
	// meantime replaces this pointer.
	m.mu.Lock()
	def = m.def
	e = m.state.env(envName)
	if e != nil {
		t = e.Targets[r.ID]
	}
	m.mu.Unlock()
	if e == nil || t == nil || ctx.Err() != nil {
		return
	}
	m.startServices(envName, def, e, r, t, dir, wanted)
}

// awaitExit polls a process (spawned via the shared process.Manager, which
// has no completion-callback API) until it stops running. ok = it exited
// cleanly.
func (m *Manager) awaitExit(ctx context.Context, id string) (ok, cancelled bool) {
	for {
		select {
		case <-ctx.Done():
			m.mgr.Stop(id) //nolint
			return false, true
		case <-time.After(300 * time.Millisecond):
		}
		p := m.mgr.FindByID(id)
		if p == nil {
			return false, false
		}
		if p.Status != process.StatusRunning {
			return p.Status == process.StatusStopped, false
		}
	}
}

// awaitDeps blocks until every on dependency's ticked services in the same
// env are ready. A dependency that dies while being waited on aborts the
// start (false); one that never becomes ready within its probe timeout is
// waited out and the start proceeds anyway — degrade, don't block.
func (m *Manager) awaitDeps(ctx context.Context, envName string, r *Repo, setAll func(phase, detail string)) bool {
	m.mu.Lock()
	def := m.def
	e := m.state.env(envName)
	rich := m.featureOnLocked(FeatHealth)
	m.mu.Unlock()
	if e == nil {
		return false
	}
	settle := false
	for _, dep := range r.DependsOn {
		dt, drp := e.Targets[dep], def.repo(dep)
		if dt == nil || drp == nil || dt.Mode == "off" || dep == r.ID {
			continue
		}
		for _, svcName := range dt.Services {
			svc := drp.service(svcName)
			if svc == nil {
				continue
			}
			port := effectivePort(svc, e)
			label := fmt.Sprintf("%s %s", drp.Name, svc.Name)
			if port > 0 {
				label += fmt.Sprintf(" :%d", port)
			}
			k := key(envName, dep, svcName)
			deadline := time.Now().Add(svc.Health.Timeout())
			for {
				if ctx.Err() != nil {
					return false
				}
				ready, dead, detail := m.depState(ctx, k, svc, port, rich)
				if ready {
					if !(rich && svc.Health.Rich()) && port > 0 {
						settle = true
					}
					break
				}
				if dead {
					setAll(phaseStopped, label+" exited — not starting")
					return false
				}
				if time.Now().After(deadline) {
					setAll(phaseWaitingDeps, label+" never became ready — starting anyway")
					break
				}
				msg := "waiting for " + label
				if detail != "" {
					msg += " — " + detail
				}
				setAll(phaseWaitingDeps, msg)
				select {
				case <-ctx.Done():
					return false
				case <-time.After(health.Interval):
				}
			}
		}
	}
	if settle {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(depSettle):
		}
	}
	return true
}

// depState reads a dependency's readiness: from its own monitor when bish
// launched it, else by probing it directly (started outside bish).
func (m *Manager) depState(ctx context.Context, k string, svc *Service, port int, rich bool) (ready, dead bool, detail string) {
	m.mu.Lock()
	h := m.health[k]
	var hc svcHealth
	if h != nil {
		hc = *h
	}
	m.mu.Unlock()
	if h != nil {
		if hc.procID != "" {
			if p := m.mgr.FindByID(hc.procID); p == nil || p.Status != process.StatusRunning {
				return false, true, ""
			}
		}
		if hc.phase == phaseStopped {
			return false, true, ""
		}
		return hc.ready, false, hc.detail
	}
	err := health.Probe(ctx, svc.Health, health.Target{Port: port}, rich)
	if err == nil {
		return true, false, ""
	}
	return false, false, err.Error()
}

func (m *Manager) startServices(envName string, def *Definition, e *Env, r *Repo, t *Target, dir string, wanted []string) {
	at := branchSuffix(dir)
	if e.Name != defaultEnv {
		at += " [" + e.Name + "]"
	}
	for _, svcName := range wanted {
		svc := r.service(svcName)
		if svc == nil {
			continue
		}
		port := effectivePort(svc, e)
		m.killPortFor(envName, port)
		k := key(envName, r.ID, svcName)
		proc, err := m.mgr.Add(exportPrefix(m.harnessVars(def, e, r))+serviceCmd(svc, port), dir, r.Name+" · "+svcName+at)
		if err != nil {
			m.setHealth(k, phaseStopped, err.Error(), false)
			continue
		}
		m.trackSet(k, proc.ID)
		m.mu.Lock()
		m.health[k] = &svcHealth{phase: phaseStarting, procID: proc.ID}
		m.mu.Unlock()
		ctx, cancel := context.WithCancel(context.Background())
		m.mu.Lock()
		if prev := m.monitors[k]; prev != nil {
			prev()
		}
		m.monitors[k] = cancel
		rich := m.featureOnLocked(FeatHealth)
		m.mu.Unlock()
		go m.monitor(ctx, k, svc, port, dir, proc.ID, rich)
	}
}

// serviceCmd is svc.Cmd with its effective port handed over: PortArgs
// appended with {{port}} substituted, PortEnv exported ahead of it.
func serviceCmd(svc *Service, port int) string {
	cmd := svc.Cmd
	if port <= 0 {
		return cmd
	}
	ps := fmt.Sprint(port)
	if svc.PortArgs != "" {
		cmd += " " + strings.ReplaceAll(svc.PortArgs, "{{port}}", ps)
	}
	if svc.PortEnv != "" {
		cmd = "export " + svc.PortEnv + "=" + ps + "; " + cmd
	}
	return cmd
}

// monitor probes a just-started service until it's ready, flipping it to
// unhealthy (but leaving it running) once the probe timeout passes, and
// stops once ready or once the process exits.
func (m *Manager) monitor(ctx context.Context, k string, svc *Service, port int, dir, procID string, rich bool) {
	deadline := time.Now().Add(svc.Health.Timeout())
	unhealthy := false
	for {
		p := m.mgr.FindByID(procID)
		if p == nil || p.Status != process.StatusRunning {
			m.setHealth(k, phaseStopped, "", false)
			return
		}
		err := health.Probe(ctx, svc.Health, health.Target{Port: port, Dir: dir, Log: p.Log}, rich)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			m.setHealth(k, phaseReady, "", true)
			return
		}
		if !unhealthy && time.Now().After(deadline) {
			unhealthy = true
		}
		if unhealthy {
			m.setHealth(k, phaseUnhealthy, "health check failing: "+err.Error(), false)
		} else {
			m.setHealth(k, phaseStarting, err.Error(), false)
		}
		wait := health.Interval
		if unhealthy {
			wait = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (m *Manager) setHealth(k, phase, detail string, ready bool) {
	m.mu.Lock()
	h := m.health[k]
	if h == nil {
		h = &svcHealth{}
		m.health[k] = h
	}
	h.phase, h.detail, h.ready = phase, detail, ready
	m.mu.Unlock()
}

// resolveDir picks (and, for worktree mode, creates) the checkout directory
// to run in, and for main-checkout mode makes sure it's on the requested
// branch — start owns the branch switch.
func (m *Manager) resolveDir(r *Repo, t *Target) (string, error) {
	switch t.Mode {
	case "worktree":
		if t.Branch == "" {
			return "", fmt.Errorf("%s: no branch selected", r.Name)
		}
		path, err := addWorktree(r, t.Branch, "")
		if err != nil {
			return "", err
		}
		t.Path = path
		return path, nil
	case "main":
		dir := r.Path
		if t.Branch != "" && currentBranch(dir) != t.Branch {
			if branchExists(dir, t.Branch) {
				if _, err := git(dir, "checkout", t.Branch); err != nil {
					return "", err
				}
			} else if _, err := git(dir, "checkout", "-b", t.Branch); err != nil {
				return "", err
			}
		}
		t.Path = dir
		return dir, nil
	default:
		return "", fmt.Errorf("%s is off", r.Name)
	}
}

func (m *Manager) stopRepo(envName, repoID string) {
	lk := envName + "|" + repoID
	pk := key(envName, repoID, "prestart")
	m.mu.Lock()
	if cancel := m.launches[lk]; cancel != nil {
		cancel()
		delete(m.launches, lk)
	}
	prestartID := m.tracked[pk]
	m.mu.Unlock()
	m.stopServices(envName, repoID, nil)
	if prestartID != "" {
		m.mgr.Stop(prestartID) //nolint
	}
}

func (m *Manager) stopServices(envName, repoID string, only []string) {
	prefix := envName + "|" + repoID + "|"
	m.mu.Lock()
	r := m.def.repo(repoID)
	e := m.state.env(envName)
	var toStop []string
	var ports []int
	for k, id := range m.tracked {
		if !strings.HasPrefix(k, prefix) || k == prefix+"prestart" {
			continue
		}
		svc := strings.TrimPrefix(k, prefix)
		if len(only) > 0 && !contains(only, svc) {
			continue
		}
		toStop = append(toStop, id)
		if r != nil && e != nil {
			if s := r.service(svc); s != nil {
				if p := effectivePort(s, e); p > 0 {
					ports = append(ports, p)
				}
			}
		}
	}
	for k, cancel := range m.monitors {
		if strings.HasPrefix(k, prefix) && (len(only) == 0 || contains(only, strings.TrimPrefix(k, prefix))) {
			cancel()
			delete(m.monitors, k)
		}
	}
	for k := range m.health {
		if strings.HasPrefix(k, prefix) && (len(only) == 0 || contains(only, strings.TrimPrefix(k, prefix))) {
			delete(m.health, k)
		}
	}
	m.mu.Unlock()
	for _, id := range toStop {
		m.mgr.Stop(id) //nolint
	}
	// killGroup's SIGKILL misses children that escaped the process group
	// (setsid, detached spawn — common in dev-server tooling), leaving them
	// bound to the port. lsof-sweep it directly, same as the pre-start guard.
	for _, port := range ports {
		m.killPortFor(envName, port)
	}
}

// killPortFor clears a stray listener on port before envName's service
// binds it — unless that port is a live service of a different env, which
// is never this env's to kill.
func (m *Manager) killPortFor(envName string, port int) {
	if port <= 0 {
		return
	}
	m.mu.Lock()
	for k, id := range m.tracked {
		env, rest, _ := strings.Cut(k, "|")
		if env == envName {
			continue
		}
		repoID, svcName, _ := strings.Cut(rest, "|")
		r, e := m.def.repo(repoID), m.state.env(env)
		if r == nil || e == nil {
			continue
		}
		svc := r.service(svcName)
		if svc == nil || effectivePort(svc, e) != port {
			continue
		}
		if p := m.mgr.FindByID(id); p != nil && p.Status == process.StatusRunning {
			m.mu.Unlock()
			return
		}
	}
	m.mu.Unlock()
	process.KillPort(port)
}

func (m *Manager) trackSet(k, id string) {
	m.mu.Lock()
	m.tracked[k] = id
	m.mu.Unlock()
}
