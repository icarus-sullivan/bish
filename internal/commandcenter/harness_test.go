package commandcenter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/csullivan/bish/internal/process"
)

func TestLegacyStateMigratesToDefaultEnv(t *testing.T) {
	var st State
	if err := json.Unmarshal([]byte(`{"targets":{"feather":{"mode":"main","path":"/x","branch":"main","services":["api"],"env":null}}}`), &st); err != nil {
		t.Fatal(err)
	}
	normalizeState(&st)
	if len(st.Envs) != 1 || st.Envs[0].Name != defaultEnv || st.Active != defaultEnv || st.Envs[0].PortOffset != 0 {
		t.Fatalf("envs = %+v active=%q", st.Envs, st.Active)
	}
	tg := st.Envs[0].Targets["feather"]
	if tg == nil || tg.Services[0] != "api" || tg.Env == nil {
		t.Fatalf("target = %+v", tg)
	}
	b, _ := json.Marshal(&st)
	var top map[string]json.RawMessage
	json.Unmarshal(b, &top) //nolint
	if _, ok := top["targets"]; ok {
		t.Errorf("legacy targets written back: %s", b)
	}
}

func TestEffectivePortAndServiceCmd(t *testing.T) {
	e := &Env{PortOffset: 100}
	svc := &Service{Cmd: "pnpm nx serve harmony", Port: 3000, PortArgs: "--port={{port}}", PortEnv: "PORT"}
	if p := effectivePort(svc, e); p != 3100 {
		t.Fatal(p)
	}
	if p := effectivePort(&Service{}, e); p != 0 {
		t.Fatal(p)
	}
	if c := serviceCmd(svc, 3100); c != "export PORT=3100; pnpm nx serve harmony --port=3100" {
		t.Fatal(c)
	}
	if c := serviceCmd(&Service{Cmd: "x"}, 0); c != "x" {
		t.Fatal(c)
	}
}

func TestCheckPortableRefusesHardcodedPort(t *testing.T) {
	def := &Definition{Repos: []*Repo{{ID: "nuna", Name: "nuna", Services: []*Service{
		{Name: "harmony", Port: 3000},
		{Name: "arrow", Port: 3001, PortArgs: "--port={{port}}"},
	}}}}
	e := &Env{Name: "b", PortOffset: 100, Targets: map[string]*Target{"nuna": {Mode: "worktree", Services: []string{"harmony", "arrow"}}}}
	err := checkPortable(def, e, nil)
	if err == nil || !strings.Contains(err.Error(), "nuna/harmony") || strings.Contains(err.Error(), "arrow") {
		t.Fatalf("err = %v", err)
	}
	e.PortOffset = 0
	if err := checkPortable(def, e, nil); err != nil {
		t.Fatalf("offset 0 must always be allowed: %v", err)
	}
}

func TestDropRefusesNonGeneratedName(t *testing.T) {
	m := New(nil)
	r := &Repo{Name: "feather", DB: DefaultDBSpec("template")}
	for _, name := range []string{"feather", "postgres", "bish_; rm -rf /", ""} {
		if err := m.dropDB(r, &Env{Name: "x", DBName: name}); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("%q: err = %v", name, err)
		}
	}
}

func TestEnvVarsInjectsPortsAndDB(t *testing.T) {
	m := New(nil)
	m.SetFeatures(map[string]bool{FeatHarness: true, FeatEnvs: true, FeatDB: true})
	feather := &Repo{ID: "feather", Services: []*Service{{Name: "api", Port: 5000}}, Env: map[string]string{"X": "{{env}}"},
		DB: &DBSpec{URLEnv: "PGDATABASE", URL: "{{db}}"}}
	nuna := &Repo{ID: "nuna", Services: []*Service{{Name: "harmony", Port: 3000}}}
	def := &Definition{Repos: []*Repo{feather, nuna}}
	e := &Env{Name: "b", PortOffset: 200, DBName: "bish_b", Targets: map[string]*Target{
		"feather": {Mode: "worktree"}, "nuna": {Mode: "worktree"},
	}}
	env := m.envVars(def, e, nuna, e.Targets["nuna"])
	if env["FEATHER_API_PORT"] != "5200" || env["FEATHER_API_URL"] != "http://localhost:5200" || env["NUNA_HARMONY_PORT"] != "3200" {
		t.Fatalf("env = %v", env)
	}
	fenv := m.envVars(def, e, feather, e.Targets["feather"])
	if fenv["PGDATABASE"] != "bish_b" || fenv["X"] != "b" {
		t.Fatalf("feather env = %v", fenv)
	}
	// flags off: exactly the repo/target env, nothing injected
	m.SetFeatures(nil)
	if off := m.envVars(def, e, feather, e.Targets["feather"]); len(off) != 1 || off["X"] != "{{env}}" {
		t.Fatalf("flag-off env = %v", off)
	}
}

func TestStepCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte("a"), 0o644)
	install := &Step{Name: "install", Cmd: "pnpm install", Kind: "install", Default: true, CacheInputs: []string{"pnpm-lock.yaml", "**/package.json"}}
	migrate := &Step{Name: "migrate", Cmd: "pnpm migrate", Kind: "migrate", Default: true, CacheInputs: []string{"pnpm-lock.yaml"}}
	r := &Repo{Path: "/elsewhere", Steps: []*Step{install, migrate}}
	tg := &Target{}

	c := newStepCacher(dir)
	cmd, _, ran := prestartCmd(r, tg, dir, c)
	if cmd != "pnpm install && pnpm migrate" || len(ran) != 1 || ran[0] != install {
		t.Fatalf("first run: cmd=%q ran=%v", cmd, ran)
	}
	c.record(install)

	cmd, skipped, _ := prestartCmd(r, tg, dir, newStepCacher(dir))
	if !strings.Contains(cmd, "skipped (cached)") || !strings.Contains(cmd, "pnpm migrate") || strings.Contains(cmd, "pnpm install &&") {
		t.Fatalf("cached run: %q", cmd)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped = %v", skipped)
	}

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte("bb"), 0o644)
	if cmd, _, _ := prestartCmd(r, tg, dir, newStepCacher(dir)); !strings.HasPrefix(cmd, "pnpm install") {
		t.Fatalf("lockfile change didn't invalidate: %q", cmd)
	}
	// cache off (nil cacher): every step runs
	if cmd, _, _ := prestartCmd(r, tg, dir, nil); cmd != "pnpm install && pnpm migrate" {
		t.Fatalf("cache off: %q", cmd)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Two envs of a dependent pair run side by side; stopping one leaves the
// other running, and the dependent only starts once its dependency is ready.
func TestStartPipelineAcrossEnvs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	pm := process.New()
	m := New(pm)
	m.SetFeatures(map[string]bool{FeatHarness: true, FeatEnvs: true})
	api := &Repo{ID: "api", Name: "api", Path: root, Services: []*Service{{Name: "srv", Cmd: "sleep 30"}}}
	web := &Repo{ID: "web", Name: "web", Path: root, DependsOn: []string{"api"}, Services: []*Service{{Name: "ui", Cmd: "sleep 30"}}}
	m.projectRoot = root
	m.def = &Definition{Repos: []*Repo{api, web}}
	main := func(svc string) *Target {
		return &Target{Mode: "main", Services: []string{svc}, Env: map[string]string{}}
	}
	m.state = normalizeState(&State{})
	m.state.env(defaultEnv).Targets = map[string]*Target{"api": main("srv"), "web": main("ui")}
	m.state.Envs = append(m.state.Envs, &Env{Name: "b", Targets: map[string]*Target{"api": main("srv"), "web": main("ui")}})
	defer pm.KillAll()

	for _, env := range []string{defaultEnv, "b"} {
		if err := m.StartEnv(env); err != nil {
			t.Fatal(err)
		}
	}
	ready := func(env, repo, svc string) bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		h := m.health[key(env, repo, svc)]
		return h != nil && h.ready
	}
	for _, env := range []string{defaultEnv, "b"} {
		waitFor(t, env+" ready", func() bool { return ready(env, "api", "srv") && ready(env, "web", "ui") })
	}
	if err := m.SetActiveEnv("b"); err != nil {
		t.Fatal(err)
	}
	snap := m.Snapshot()
	if snap.Running[defaultEnv] != 2 || snap.Running["b"] != 2 {
		t.Fatalf("running = %v", snap.Running)
	}
	if st := snap.Statuses["web|ui"]; st == nil || st.Phase != phaseReady {
		t.Fatalf("active-env status = %+v", st)
	}

	m.StopEnv("b") //nolint
	waitFor(t, "b stopped", func() bool { return m.Snapshot().Running["b"] == 0 })
	if n := m.Snapshot().Running[defaultEnv]; n != 2 {
		t.Fatalf("stopping b touched default: running=%d", n)
	}
}
