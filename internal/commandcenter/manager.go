package commandcenter

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// tracked maps "<repoID>|prestart" / "<repoID>|<service>" to the current
	// process.Manager process ID, since Add mints its own IDs rather than
	// taking a caller-supplied deterministic one.
	tracked map[string]string
}

func New(mgr *process.Manager) *Manager {
	return &Manager{
		mgr:     mgr,
		def:     &Definition{},
		state:   &State{Targets: map[string]*Target{}},
		tracked: map[string]string{},
	}
}

// Load (re)loads the Definition/State for projectRoot and discovers any git
// repos among projectRoot + extraRoots not already known. Pass "" to clear
// (no project open).
func (m *Manager) Load(projectRoot string, extraRoots []string) error {
	if projectRoot == "" {
		m.mu.Lock()
		m.projectRoot = ""
		m.def = &Definition{}
		m.state = &State{Targets: map[string]*Target{}}
		m.tracked = map[string]string{}
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
	for _, r := range def.Repos {
		if st.Targets[r.ID] == nil {
			st.Targets[r.ID] = defaultTarget(r)
		}
	}
	m.mu.Lock()
	m.projectRoot = projectRoot
	m.def = def
	m.state = st
	m.tracked = map[string]string{}
	m.mu.Unlock()
	if err := saveDefinition(projectRoot, def); err != nil {
		return err
	}
	return saveState(projectRoot, st)
}

// Snapshot is the combined definition/state/live-status view pushed to the
// frontend.
func (m *Manager) Snapshot() *Snapshot {
	m.mu.Lock()
	def, st := m.def, m.state
	tracked := make(map[string]string, len(m.tracked))
	for k, v := range m.tracked {
		tracked[k] = v
	}
	m.mu.Unlock()

	byID := make(map[string]*process.Process)
	for _, p := range m.mgr.List() {
		byID[p.ID] = p
	}
	statuses := make(map[string]*ServiceStatus, len(tracked))
	for key, id := range tracked {
		p := byID[id]
		if p == nil {
			continue
		}
		statuses[key] = &ServiceStatus{Key: key, ProcessID: p.ID, PID: p.PID, Status: string(p.Status), Ports: p.Ports}
	}
	return &Snapshot{Definition: def, State: st, Statuses: statuses}
}

func (m *Manager) SaveDefinition(def *Definition) error {
	m.mu.Lock()
	root := m.projectRoot
	if root == "" {
		m.mu.Unlock()
		return fmt.Errorf("no project open")
	}
	m.def = def
	for _, r := range def.Repos {
		if m.state.Targets[r.ID] == nil {
			m.state.Targets[r.ID] = defaultTarget(r)
		}
	}
	st := m.state
	m.mu.Unlock()
	if err := saveState(root, st); err != nil {
		return err
	}
	return saveDefinition(root, def)
}

func (m *Manager) SetTarget(repoID string, t *Target) error {
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
	m.state.Targets[repoID] = t
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

func (m *Manager) StartAll() error {
	m.mu.Lock()
	def, st := m.def, m.state
	m.mu.Unlock()
	var errs []string
	for _, id := range startOrder(def, st) {
		t := st.Targets[id]
		if t == nil || t.Mode == "off" {
			continue
		}
		if err := m.startRepo(id, nil); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	def := m.def
	m.mu.Unlock()
	for _, r := range def.Repos {
		m.StopRepo(r.ID) //nolint
	}
}

func (m *Manager) StartRepo(repoID string) error { return m.startRepo(repoID, nil) }

func (m *Manager) StartService(repoID, service string) error {
	return m.startRepo(repoID, []string{service})
}

func (m *Manager) StopRepo(repoID string) error {
	m.stopServices(repoID, nil)
	m.mu.Lock()
	prestartID := m.tracked[repoID+"|prestart"]
	m.mu.Unlock()
	if prestartID != "" {
		m.mgr.Stop(prestartID) //nolint
	}
	return nil
}

func (m *Manager) startRepo(repoID string, only []string) error {
	m.mu.Lock()
	r := m.def.repo(repoID)
	t := m.state.Targets[repoID]
	def, st, root := m.def, m.state, m.projectRoot
	m.mu.Unlock()
	if root == "" {
		return fmt.Errorf("no project open")
	}
	if r == nil {
		return fmt.Errorf("repo %s not found", repoID)
	}
	if t == nil || t.Mode == "off" || t.Mode == "" {
		return fmt.Errorf("%s is off — pick a checkout mode first", r.Name)
	}

	dir, err := m.resolveDir(r, t)
	if err != nil {
		return err
	}
	saveState(root, st) //nolint — best effort; persists any worktree path resolved above

	env := mergeEnv(r.Env, t.Env)
	linkShared(r, dir)
	copyShared(r, dir)
	if err := writeOverrides(dir, r.Overrides, env); err != nil {
		return fmt.Errorf("%s: write %s: %w", r.Name, r.Overrides, err)
	}

	// Replace what this start covers: just the named services on a targeted
	// start, all of the repo's on a full one — a single-service restart
	// shouldn't kill its siblings.
	m.stopServices(repoID, only)

	pre, _ := prestartCmd(r, t, dir)
	gated := false
	if wait := depWaitCmd(def, st, r); wait != "" {
		gated = true
		if pre == "" {
			pre = wait
		} else {
			pre = wait + " && " + pre
		}
	}

	if pre != "" {
		proc, err := m.mgr.Add(pre, dir, r.Name+" · prestart"+branchSuffix(dir))
		if err != nil {
			return err
		}
		m.trackSet(repoID, "prestart", proc.ID)
		if gated {
			go m.watchDeps(proc.ID, r.DependsOn)
		}
		go m.awaitPrestart(proc.ID, r, t, dir, env, only)
		return nil
	}
	return m.startServices(r, t, dir, env, only)
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

// awaitPrestart polls the prestart process (spawned via the shared
// process.Manager, which has no completion-callback API) until it stops
// running, then boots services on success.
func (m *Manager) awaitPrestart(id string, r *Repo, t *Target, dir string, env map[string]string, only []string) {
	for {
		time.Sleep(300 * time.Millisecond)
		p := m.mgr.FindByID(id)
		if p == nil || p.Status != process.StatusRunning {
			break
		}
	}
	p := m.mgr.FindByID(id)
	if p == nil || p.Status != process.StatusStopped {
		return // crashed, killed, or missing — services never start; visible on the prestart process itself
	}
	// Re-read the target: prestart can take minutes, and a save in the
	// meantime replaces this pointer.
	m.mu.Lock()
	ct := m.state.Targets[r.ID]
	m.mu.Unlock()
	if ct == nil {
		ct = t
	}
	m.startServices(r, ct, dir, mergeEnv(r.Env, ct.Env), only) //nolint
}

// watchDeps fails a gated prestart the moment a dependency it's waiting on
// dies, so a dependency that never binds its port surfaces in seconds
// instead of after the full depWaitSeconds timeout.
func (m *Manager) watchDeps(prestartID string, deps []string) {
	for {
		time.Sleep(2 * time.Second)
		p := m.mgr.FindByID(prestartID)
		if p == nil || p.Status != process.StatusRunning {
			return
		}
		for _, dep := range deps {
			for _, svcID := range m.trackedServiceIDs(dep) {
				sp := m.mgr.FindByID(svcID)
				if sp != nil && sp.Status != process.StatusRunning {
					m.mgr.Stop(prestartID) //nolint
					return
				}
			}
		}
	}
}

func (m *Manager) startServices(r *Repo, t *Target, dir string, env map[string]string, only []string) error {
	list := t.Services
	if len(only) > 0 {
		list = only
	}
	if len(list) == 0 {
		return fmt.Errorf("no services ticked for %s", r.Name)
	}
	var firstErr error
	at := branchSuffix(dir)
	for _, svcName := range list {
		svc := r.service(svcName)
		if svc == nil {
			continue
		}
		killPort(svc.Port)
		proc, err := m.mgr.Add(svc.Cmd, dir, r.Name+" · "+svcName+at)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		m.trackSet(r.ID, svcName, proc.ID)
	}
	return firstErr
}

func (m *Manager) stopServices(repoID string, only []string) {
	prefix := repoID + "|"
	m.mu.Lock()
	var toStop []string
	for k, id := range m.tracked {
		if !strings.HasPrefix(k, prefix) || k == prefix+"prestart" {
			continue
		}
		svc := strings.TrimPrefix(k, prefix)
		if len(only) > 0 && !contains(only, svc) {
			continue
		}
		toStop = append(toStop, id)
	}
	m.mu.Unlock()
	for _, id := range toStop {
		m.mgr.Stop(id) //nolint
	}
}

func (m *Manager) trackSet(repoID, key, id string) {
	m.mu.Lock()
	m.tracked[repoID+"|"+key] = id
	m.mu.Unlock()
}

func (m *Manager) trackedServiceIDs(repoID string) []string {
	prefix := repoID + "|"
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for k, id := range m.tracked {
		if strings.HasPrefix(k, prefix) && k != prefix+"prestart" {
			ids = append(ids, id)
		}
	}
	return ids
}

// killPort clears any stray process already bound to port before a service
// starts, so a crashed previous run doesn't block the new one with
// EADDRINUSE.
func killPort(port int) {
	if port <= 0 {
		return
	}
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", port)).Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 1 {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			proc.Kill() //nolint
		}
	}
}
