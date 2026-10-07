# Harness Plan — Command Center → AI-first dev harness

## What this is

bish already ships **Command Center** (`internal/commandcenter/`, `frontend/src/components/CommandCenter.svelte`): a per-project, git-scoped multi-repo service launcher with worktree checkout, pre-start steps, cross-repo `dependsOn` ordering, env-override file generation, and live status cross-referenced against `process.Manager`.

That's roughly 70% of the bones of a real dev harness. This document covers the other 30%: making it **work on an unseen project without hand-configuration**, letting **two branches of the same repo run side by side**, giving it **databases**, and putting the **web UIs it launches inside the app** instead of punting to the system browser.

This is an extension plan, not a rewrite. There is no new "harness" subsystem — the work lands as two small new Go packages plus additive fields on the existing Command Center types. Everything new is gated per the `FEATURES.md` contract: a row in `frontend/src/lib/features.ts` and a `featureOn()` guard at the mount, where OFF means no goroutine, no container, no poll — not just hidden UI.

Format matches `PM_ASKS_2.md`: gap, why it matters, plan, acceptance criteria, effort. Ordered by **value per effort**, which is also the build order.

### The six areas

| # | Area | Effort | Why this position |
|---|---|---|---|
| 1 | [Readiness in Go](#1-readiness-in-go) | S/M | Deletes fragile code, makes status honest, and is a prerequisite for 3 and 4. Build first. |
| 2 | [Env auto-detection](#2-env-auto-detection) | M | The actual "works on any project" unlock. Today a fresh project shows an empty card. |
| 3 | [Preview tab](#3-preview-tab) | S | One tab type, one component, zero new Go. Highest visible payoff per line. |
| 4 | [Ephemeral environments](#4-ephemeral-environments) | L | The headline feature, but it's the big model change and wants 1 and 2 landed first. |
| 5 | [Database lifecycle](#5-database-lifecycle) | S | Small *because* it's declared commands. Only meaningful once 4 exists. |
| 6 | [Step caching](#6-step-caching) | S | Narrow, low value, real footgun. Last, or never. |

### Progress

Code complete; `go test ./internal/{health,envdetect,commandcenter}` passes and the frontend builds. Acceptance criteria against real feather/nuna still need a manual run (all harness flags except Preview default off — turn on "Dev harness" + sub-flags in Settings).

- [x] 1. Readiness in Go — `internal/health` (TCP dial ungated; HTTP/LogMatch/Cmd behind `harnessHealth`); `depWaitCmd`/`watchDeps`/`nc` gone; phases + amber/red dots; Health rows in the Add/Edit Service dialog
- [x] 2. Env auto-detection — `internal/envdetect` (node/nx incl. inferred plugin targets, compose, Makefile, Procfile, python/django, go, cargo, rails, `.env*` port hints); `DetectRepoEnv`/`ApplyRepoProposal`; wand button + review modal
- [x] 3. Preview tab — `Preview.svelte` (kept mounted across tab switches, sandboxed without top-navigation); port badges → Preview with an external-open button beside; auto-open behind `harnessPreviewAuto`. Deviation: one small Go method, `PreviewFrameBlocked`, checks `X-Frame-Options`/`frame-ancestors` — a refused frame fires no error event in WKWebView, so the explicit failure state can't be detected client-side
- [x] 4. Ephemeral environments — `Env` + legacy-state migration, `effectivePort`, `PortEnv`/`PortArgs`, `<REPO>_<SVC>_PORT/_URL` injection (overrides file *and* exported into prestart/service commands), offset allocation, env-scoped port kill, reconcile-and-warn drift, hardcoded-port refusal, env select + new/destroy dialogs
- [x] 5. Database lifecycle — `DBSpec` with generated template/compose defaults (`DefaultCommandCenterDBSpec`), create on `CreateEnv`, `^bish_` guarded drop on `DestroyEnv` (default off + confirm), URL injection before prestart, `Ready` gates prestart; `Repo.Compose` shared infra brought up once per session
- [x] 6. Step caching — stat-hash per checkout under `~/.config/bish/cache/steps/`, install/codegen only, never destructive/stateful, visible `skipped (cached)` line, per-step "cache" chip, ⇧-click Start = ignore cache (`ClearStepCache`)
- [x] Feature flags — `features.ts` rows (+ `harnessPreviewAuto`), Go-side `Manager.SetFeatures`/`FeatureOn` fed from config on startup and every SaveConfig, `FEATURES.md` "Harness" heading
- [x] Incidental cleanups — `process.KillPort` is the single implementation; `normalizeDefinition`/`normalizeState`/`normalizeCC` cover the new DTOs

Known, not addressed here: `process.Manager.List`/`FindByID` hand out live `*Process` pointers whose `Status` the exit goroutine writes under the manager lock — `go test -race` flags it (pre-existing; Command Center read it the same way before).

### Decisions already settled

1. **Database isolation: both modes, declared per repo.** `template` (per-env database on a shared server, `CREATE DATABASE … TEMPLATE`) or `compose` (own stack scoped by `COMPOSE_PROJECT_NAME`). bish links **no Docker SDK** either way — both are declared shell commands run through the existing `process.Manager`.
2. **Caching: local content-hash skip only.** No remote, shared, or chunked cache. Where a repo has its own remote cache (Nx Cloud), reuse it rather than reinventing it.
3. **Web UIs: in-app Preview tab (iframe)**, with `BrowserOpenURL` kept as the always-works fallback for servers that refuse framing.
4. **Flags: a master `harness` row plus sub-flags**, mirroring how `features.ts` already splits `gitBlame` / `gitGutter` / `gitDiff`.

### Validation targets

Two real workloads are used throughout to keep this honest. Neither gets a line of bish-side special-casing.

- **feather** (`~/Desktop/repo/feather`) — pnpm + Nx 22, API on `:5000`, shared infra via `docker/docker-compose.yml` (Postgres 17/PostGIS, PostgREST `:3010`, RabbitMQ `:5672`/`:15672`, OpenSearch `:9200`, stripe-mock `:8420`, optional Keycloak `:8480`), `pnpm db:reset` = schema drop → migrate → 59 YAML fixtures → post-hooks, env from AWS SSM, DB credentials out-of-band in `~/.pg_service.conf`.
- **nuna** (`~/Desktop/repo/nuna`) — 5 Vite apps on `:3000`–`:3004` (harmony, arrow, ashoka, partner, employer), Storybook on `:6006`, no database of its own, `.env` auto-generated from SSM, talks to feather via `VITE_API_ENDPOINT`.

feather already has `worktree-ops/spinup.py` / `spindown.py` doing per-worktree databases on a shared Postgres with upward port scanning from per-app bases. That script is proof the shape works — and it's exactly the thing bish should generalize so every project gets it for free.

---

# 1. Readiness in Go

**Gap:** `depWaitCmd` (`internal/commandcenter/launch.go:59`) synthesizes a shell string — `for i in $(seq 1 600); do nc -z 127.0.0.1 N || nc -z ::1 N; …` — and prepends it to the prestart command. It's the only readiness signal in the system, and its own doc comment admits it: *"a port-listening + settle probe, not a real health check."*

**Why it matters:** Three separate consequences, all of which bite the rest of this plan.

- **The UI can't see it.** Readiness lives inside one opaque prestart process, so the status dot has exactly two states. A service stuck waiting 600 seconds on a dependency looks identical to one that's booting normally.
- **It can't read logs.** `logs.LogBuffer` is Go-side only. Tools that print `ready in 412ms` before their port means anything — and tools that bind a port long before they serve — can't be probed correctly from a shell loop.
- **It depends on `nc`.** Plus the `::1`-vs-`127.0.0.1` dual-probe exists only because a shell probe can't do what one `net.DialTimeout` does.

Readiness is also load-bearing for ephemeral environments (which need the *effective* port, not the declared one) and for Preview auto-open. Fixing it first makes both cheap.

**Plan:**

New package `internal/health` (~120 lines). One `Probe` function, polled at 500ms from a goroutine owned by `commandcenter.Manager`.

```go
// Health declares what "ready" means for a service. The zero value is a TCP
// dial on the service's effective port — i.e. exactly today's behavior.
type Health struct {
	HTTP       string `json:"http,omitempty"`       // "/health", or a full URL; {{port}} substituted
	Status     int    `json:"status,omitempty"`     // expected code; 0 = any 2xx/3xx
	LogMatch   string `json:"logMatch,omitempty"`   // regexp matched against the service's logs.LogBuffer
	Cmd        string `json:"cmd,omitempty"`        // exit 0 = ready
	TimeoutSec int    `json:"timeoutSec,omitempty"` // 0 = 600, matching today's depWaitSeconds
}

// Service gains:
Health *Health `json:"health,omitempty"`
```

```go
// ServiceStatus gains the three fields that make the UI honest.
Ready  bool   `json:"ready"`
Phase  string `json:"phase"`  // "starting" | "waiting-deps" | "ready" | "unhealthy" | "stopped"
Detail string `json:"detail"` // "waiting for feather :5010 — http 503"
```

1. `Probe(ctx, spec, port, buf *logs.LogBuffer) error`. Probe kinds compose: if `HTTP` is set, hit it; if `LogMatch` is set, scan `buf`; if `Cmd` is set, run it; otherwise dial the port.
2. Delete `depWaitCmd`. `startRepo` gates on a Go wait instead of prepending a generated string to prestart. The `nc` dependency, the dual-stack hack, and the `seq`/`sleep` string all go away.
3. `watchDeps` (`manager.go:335`) collapses into the same goroutine — it already polls every 2s to kill a gated prestart when a dependency dies.
4. **A timed-out probe starts the service anyway**, with `Phase:"unhealthy"`. Degrade, don't block: a bad probe must never turn a previously-working start into a permanent hang.
5. No new Wails methods. `Phase` / `Detail` / `Ready` ride the existing `cc:update` snapshot emitted from `refreshLoop`.
6. UI: the existing status dot gains amber (`starting`, `waiting-deps`) and red (`unhealthy`), with `Detail` as the `title`. A Health row in the Add/Edit Service dialog.

**Flag tension, stated for the record:** this is the one item that *replaces* existing behavior rather than adding to it. Gating the whole thing means maintaining the old shell-generated path forever. So the plain TCP dial replaces `depWaitCmd` **ungated** — it is a pure refactor, strictly better, and costs nothing extra — and only the richer probe types (`HTTP`, `LogMatch`, `Cmd`) sit behind `harnessHealth`.

**Acceptance criteria:** Starting feather with nuna depending on it shows nuna in `waiting-deps` with a human-readable `Detail`, flipping to `ready` only once `GET :5000/health` returns 200 — not merely once the port is bound. No `nc` on `PATH` is required. A service with a deliberately wrong health path still starts, and shows `unhealthy` rather than hanging the panel.

**Effort:** S/M (net negative lines in `launch.go`)

---

# 2. Env auto-detection

**Gap:** `discoverRepos` (`internal/commandcenter/discover.go:14`) seeds a newly found repo with `Steps: []*Step{}` and `Services: []*Service{}` — empty. Its comment is explicit that it seeds *"never from a hardcoded name/path"*, which is the right instinct, but the result is that opening any project for the first time shows a card with a name, a branch picker, and nothing to run. `defaultTarget` likewise ticks no services, because there are none to tick.

**Why it matters:** This is the entire first-run experience, and it's the difference between "a launcher for projects I've already configured" and "a harness that works on any project." Every piece of information needed is sitting in files in the repo: `package.json` scripts, Nx project targets, compose services, Makefile targets, Procfile lines. For nuna specifically, the five apps and their ports are fully determined by `nx.json` plus five `vite.config.ts` files — a user should not be typing them in by hand.

**Plan:**

New package `internal/envdetect`. Pure functions, zero writes. It returns a **reviewable proposal with provenance** — it never silently writes `.bish/command-center.json`.

```go
// Candidate is one proposed Service or Step, carrying the file it came from so
// the user can judge it instead of trusting it.
type Candidate struct {
	Name        string   `json:"name"`
	Cmd         string   `json:"cmd"`
	Port        int      `json:"port,omitempty"`
	Kind        string   `json:"kind"` // "service"|"install"|"migrate"|"codegen"|"reset"|"build"|"infra"
	Destructive bool     `json:"destructive,omitempty"`
	Supersedes  []string `json:"supersedes"`
	Source      string   `json:"source"`     // "package.json#scripts.dev", "docker/docker-compose.yml#postgres"
	Confidence  int      `json:"confidence"` // 0-100; drives whether the row is pre-ticked
	Accept      bool     `json:"accept"`
}

type Proposal struct {
	RepoID    string       `json:"repoId"`
	Toolchain string       `json:"toolchain"` // "pnpm@10.33.0 node@24.18.0" — displayed, never executed
	Services  []*Candidate `json:"services"`
	Steps     []*Candidate `json:"steps"`
	Compose   string       `json:"compose,omitempty"`
	Notes     []string     `json:"notes"`
}
```

Detectors, each contributing candidates:

| Source | Yields |
|---|---|
| `package.json#packageManager`, `.nvmrc`, `.tool-versions` | `Toolchain` string only — display and cache key, never executed |
| `package.json#scripts` | name pattern → kind. `dev\|start\|serve\|storybook` → service; `install` → install; `migrat*` → migrate; `codegen\|generate` → codegen; `*reset\|db:drop\|db:nuke` → reset, with `Destructive: true` and `Supersedes` pointing at the migrate candidates |
| `nx.json` present → `apps/*/project.json#targets` | one service per `serve`/`dev` target, `Cmd: pnpm nx serve <project>`, `Name` = project name. **This is what makes nuna's five apps appear in one click.** |
| `docker-compose*.yml` → `services.*.ports` | one `Kind:"infra"` **Step** (`docker compose -f <f> up -d`) plus `Notes` listing each container and port. Deliberately *not* one Service per container — shared infra starts once per project, not once per env |
| `Makefile` (`^[a-z][a-z0-9_-]*:`, skipping `.PHONY` and pattern rules) | services for `dev\|run\|serve`, steps otherwise, low confidence |
| `Procfile` | one service per line; `web:` → service |
| `pyproject.toml` / `poetry.lock` / `requirements.txt` | install step; `manage.py` present → `runserver` service + `migrate` step |
| `go.mod` | `go run ./cmd/<x>` service per `cmd/*` directory |
| `Cargo.toml` | `cargo run` service, `cargo build` step |
| `Gemfile` + `config/application.rb` | `bin/rails s` service, `db:migrate` / `db:reset` steps |
| `.env.example` / `.env.*` keys matching `(?i)port$` | `PortEnv` hints for the matching service |

Port extraction, in order: `--port[= ]([0-9]+)` in the script value, then `PORT=` in `vite.config.*` / `next.config.*` / `.env*`, then compose `ports:`. A miss yields `Port: 0` plus a Note. **Never guess a port.**

- Wails: `DetectRepoEnv(repoID string) (*envdetect.Proposal, error)` and `ApplyRepoProposal(repoID string, p *envdetect.Proposal) error`. Apply filters on `Accept` and merges into the `Definition` **by name** — it never deletes or overwrites an entry the user already has.
- UI: a repo card with no services shows a single "Detect services" button (`.hdr-btn`, `size={13}`, per `CLAUDE.md`). Clicking opens a modal listing each candidate as `[x] name · cmd · :port · from package.json#scripts.dev`. Apply writes `.bish/command-center.json` through the existing `SaveCommandCenterDefinition`.
- Nothing is pre-ticked below confidence 70. Detection **never** runs automatically on discovery — it's a button, because a wrong proposal applied silently is worse than an empty card.

**Biggest risk:** a plausible-looking but wrong command gets applied and the user blames bish for a broken start. The mitigation is provenance on every row — `Source` is not decoration, it's the thing that makes the proposal reviewable rather than magic.

**Acceptance criteria:** Opening a workspace containing feather and nuna for the first time, then clicking "Detect services" on each, produces a runnable configuration with no typing: nuna's five Vite apps with their ports and Storybook, feather's `pnpm start` on `:5000` plus install / migrate / codegen / `db:reset` steps with `db:reset` marked destructive and superseding migrate, and feather's compose file as an infra step. A scratch Go module and a scratch Rails app each produce at least one correct service.

**Effort:** M (pure functions, trivially unit-testable, no runtime risk)

---

# 3. Preview tab

**Gap:** There is no `<iframe>` anywhere in the frontend — the only occurrence of the string is in DOMPurify's `FORBID_TAGS` in `lib/claude/format.ts`. The sole affordance for a launched web UI is the Command Center port badge calling `BrowserOpenURL(http://localhost:N)`, which hands off to the system browser and out of the app.

**Why it matters:** The harness launches web UIs; it should be able to show them. Context-switching to a browser window breaks the loop of *change code → see it_ that the whole panel exists to serve, and it's the one place where an AI-driven edit can't be visually verified without leaving the tool.

**Plan:**

Zero new Go. `index.html` sets no CSP meta and Wails sets no `frame-src`, so a `wails://wails.localhost` page framing `http://localhost:3101` loads today.

1. `frontend/src/lib/stores.ts`: add `'preview'` to `Tab['type']` and a `url?: string` field. Add `openPreviewTab(url, label)` mirroring the existing `openLogsTab`.
2. One `{:else if tab.type === 'preview'}` branch in `App.svelte`'s tab-content block.
3. New `frontend/src/components/Preview.svelte`: a URL `<input>`, a reload `.hdr-btn`, a width `<select>` (Responsive / 390 / 768 / 1280 — `appearance:none` plus the absolutely-positioned `IconChevronDown`, per `CLAUDE.md`), the `<iframe>`, and an "open in browser" button.
4. Reload must **re-create the element** — wrap the iframe in `{#key reloadNonce}`. `iframe.contentWindow.location.reload()` throws cross-origin, and reassigning `src` only appends history entries.
5. Port badges in Command Center open a Preview tab instead of the system browser, keeping an explicit external-open button beside them.
6. Optional auto-open when a service reaches `Ready` with `Port > 0`, behind its own toggle, **default off** — five nuna apps becoming ready would otherwise open five tabs.

**Constraints, stated here rather than discovered later:**

- **Cross-origin means no introspection.** No console capture, no `window.onerror`, no DOM access, no route or scroll sync. Anything that reads inside the frame is off the table.
- **Some servers refuse framing outright.** `X-Frame-Options: DENY` or a `frame-ancestors` CSP blocks the frame entirely. Vite and Next don't by default; Rails and some auth middleware do. This needs an explicit failure state — detect via `onerror` plus a blank-frame timeout, and surface "this server refuses framing — open externally." A silent blank box reads as "bish is broken."
- **No devtools for frame content.** Wails' inspector targets the host page. This is the main reason the external-open button is permanent, not transitional.
- **HMR works unproxied.** Vite's client dials `location.host`, which inside the frame is the dev server itself. This is precisely why there must be no proxy.
- **Storage and cookies are shared with the whole webview.** Good for staying logged in across reloads, but WKWebView's cross-site cookie policy breaks redirect-based IdP flows (Stytch, Keycloak) inside a frame. Those open externally.

**Cut as speculative:** console and error capture via a loopback proxy that rewrites HTML to inject a `postMessage` shim. That's an MITM HTML rewriter that breaks on streaming SSR, compressed responses, and WebSocket upgrades — to re-provide what devtools already does. Revisit only if someone asks twice. Also cut: device chrome, screenshots, and a preview history stack.

**Acceptance criteria:** Starting nuna's harmony app and clicking its port badge opens an interactive Preview tab — clicks, forms, and navigation all work, HMR still hot-reloads on a source edit, and the width selector reflows the app. Pointing a Preview tab at a server that sends `X-Frame-Options: DENY` shows an explicit message and a working "open in browser" button, not an empty frame.

**Effort:** S

---

# 4. Ephemeral environments

**Gap:** Two structural facts prevent running two environments at once. `State.Targets` is `map[string]*Target` — exactly one target per repo — so selecting a branch for feather replaces whatever was selected before. And `Service.Port` is a single fixed `int`, so even if two targets existed, both would try to bind the same port. `killPort` running before every service start actively enforces this: starting env B kills env A's server.

**Why it matters:** This is the headline capability and the reason the plan exists. Reviewing a teammate's branch while your own work keeps running, or having an agent iterate on a fix in an isolated environment while you keep developing, both require N concurrent environments. Worktree support — the hard part — is already built (`addWorktree`, `linkShared`, `copyShared`); what's missing is the model that lets more than one be live.

**Plan:**

`Definition` changes are **additive only**, because it's git-shared and an older bish must still read it:

```go
// Service gains the two fields that make a port offset honest.
PortEnv  string `json:"portEnv,omitempty"`  // env var the service reads its port from, e.g. "PORT"
PortArgs string `json:"portArgs,omitempty"` // appended to Cmd with {{port}} substituted, e.g. "--port {{port}}"

// Repo gains
Compose string  `json:"compose,omitempty"` // shared-infra compose file, rel to Path; started once per project
DB      *DBSpec `json:"db,omitempty"`      // see area 5
```

`State` is per-user (`~/.config/bish/projects/<md5>-cc.json`), so migration is cheap:

```go
// Env is one named, independently-startable set of checkouts.
type Env struct {
	Name       string             `json:"name"`
	Branch     string             `json:"branch"`           // default branch for every repo's worktree
	PortOffset int                `json:"portOffset"`       // added to each Service.Port
	DBName     string             `json:"dbName,omitempty"` // generated, "bish_<env>"
	Targets    map[string]*Target `json:"targets"`          // keyed by Repo.ID — Target itself is UNCHANGED
	CreatedAt  time.Time          `json:"createdAt"`
}

type State struct {
	// Targets is the legacy pre-Env single selection: read on load to build the
	// "default" env, never written again after migration.
	Targets map[string]*Target `json:"targets"`
	Envs    []*Env             `json:"envs"`
	Active  string             `json:"active"` // Env.Name
}
```

1. **Backward compatibility is one branch in `loadState`:** if `len(Envs) == 0 && len(Targets) > 0`, synthesize `Envs = [{Name: "default", PortOffset: 0, Targets: Targets}]` and `Active = "default"`. Everything downstream reads `st.active().Targets`, so `Manager`'s exported method signatures are untouched. Leaving `Target` itself unchanged is what keeps this diff tractable.
2. **One port function**, used by the health probe, `writeOverrides`, the port badge, and Preview:
   ```go
   func effectivePort(svc *Service, env *Env) int {
       if svc.Port <= 0 { return 0 }
       return svc.Port + env.PortOffset
   }
   ```
3. The offset is applied three ways: substituting `{{port}}` into `PortArgs` and appending it to `Cmd`; setting `PortEnv=<port>` in the env map; and injecting `<REPOID>_<SVC>_PORT` plus the service's full URL into **every** repo's `Overrides` file, so cross-repo callers follow — this is what makes nuna's `VITE_API_ENDPOINT` point at the right feather.
4. Offset allocation: scan upward in steps of 100, probing every service's `svc.Port + offset` with `net.DialTimeout`. Same strategy as feather's `spinup.py`, generalized.
5. `killPort` becomes env-scoped — it may only kill the effective port of the env being started, never a sibling's.
6. Reconcile on load: `git worktree list` per repo plus a dial per service. A drifted env (worktree deleted by hand, port stolen by something else) is **marked with a warning**, not silently repaired.

**Fail loud, honestly:** a service that hardcodes its port ignores the offset. nuna does exactly this — `apps/harmony/vite.config.ts:28` and friends set `server.port` literally. So `StartEnv` **refuses** to start a non-zero-offset env containing a service with `Port > 0` and neither `PortEnv` nor `PortArgs` set, with an error naming the service and the fix. Binding the wrong port and silently corrupting the sibling env is far worse than a clear refusal. The detector emits a Note for precisely this case.

- Wails: `ListEnvs() []*Env`, `CreateEnv(name, branch string) (*Env, error)` (allocates offset and `DBName`), `SetActiveEnv(name string) error`, `StartEnv(name string) error`, `StopEnv(name string) error`, `DestroyEnv(name string, dropDB, removeWorktrees bool) error`.
- UI: one env `<select>` in the Command Center header (`appearance:none` + absolute `IconChevronDown`) listing envs plus "New env…", with a `+` / trash `.hdr-btn` pair. Repo cards render unchanged, now reading the active env's targets; port badges show the effective port.

**Biggest risk:** env state drifting from what's actually on disk. Reconcile-and-warn on load is the answer; attempting automatic repair of a half-deleted worktree is how you lose someone's uncommitted work.

**Acceptance criteria:** Two envs on different feather+nuna branches run simultaneously — distinct worktrees, distinct effective ports, distinct databases — and nuna in env B talks to feather in env B. Stopping env B leaves env A fully running. An existing `.bish/command-center.json` and its `<md5>-cc.json` state file load unchanged and appear as a "default" env with offset 0. Creating an env whose repos contain a hardcoded-port service fails with a message naming that service.

**Effort:** L

---

# 5. Database lifecycle

**Gap:** Zero references to docker, containers, or databases anywhere in bish's Go or Svelte source. An ephemeral environment without an isolated database isn't isolated — both envs migrate and seed the same schema, and `db:reset` in one destroys the other's data.

**Why it matters:** For any app with a database, the database *is* the environment. feather's `pnpm db:reset` drops the schema, runs migrations, and loads 59 YAML fixture files; two envs sharing that is not two envs.

**Plan:**

Declared commands with template variables, honoring both chosen modes. Roughly 40 lines of Go: no database drivers, no Docker SDK. `compose` mode simply means the commands happen to be `docker compose` invocations scoped by `COMPOSE_PROJECT_NAME`.

```go
// DBSpec declares how to make and unmake this repo's per-env database. Every
// command runs through the existing process.Manager with {{db}}, {{port}},
// {{env}}, {{file}} and {{template}} substituted.
type DBSpec struct {
	Mode     string `json:"mode"`               // "template" | "compose"
	File     string `json:"file,omitempty"`     // compose mode: compose file, rel to the repo
	Template string `json:"template,omitempty"` // template mode: source database to clone
	Create   string `json:"create"`
	Drop     string `json:"drop"`
	Ready    string `json:"ready"`  // exit 0 = usable
	URLEnv   string `json:"urlEnv"` // "DATABASE_URL", or "PGDATABASE"
	URL      string `json:"url"`    // "postgres://localhost:{{port}}/{{db}}", or just "{{db}}"
}
```

Each mode ships a **generated default** the user can edit, so choosing `template` on a Postgres repo is a one-click decision rather than three commands to write:

| Mode | `Create` | `Drop` | `Ready` |
|---|---|---|---|
| `template` | `createdb -T {{template}} {{db}}` | `dropdb --if-exists {{db}}` | `pg_isready -q` |
| `compose` | `docker compose -p {{env}} -f {{file}} up -d` | `docker compose -p {{env}} -f {{file}} down -v` | `docker compose -p {{env}} -f {{file}} ps --status running -q` |

1. `CreateEnv` runs `Create`. `DestroyEnv(dropDB: true)` runs `Drop`.
2. `StartEnv` injects `URLEnv=URL` into the overrides file **before** prestart, so the repo's own `migrate` / `reset` steps hit that env's database with no bish-side knowledge of how they work.
3. Shared infra (`Repo.Compose`) is a normal idempotent `infra` Step (`up -d`), started once per project rather than once per env — this is where feather's RabbitMQ, OpenSearch, and stripe-mock live. Only the *database* is per-env.
4. `Ready` participates in the area-1 probe, so prestart waits for the database rather than racing it.

**Non-negotiable guard:** `Drop` runs only when `DBName` matches the generated `^bish_` prefix. It never runs against a name the user typed. `dropDB` defaults to **false** with an explicit confirmation, mirroring the existing `Destructive` step confirm in `CommandCenter.svelte`. There is no "drop all envs" button, and there won't be one.

**Biggest risk:** dropping the wrong database. The prefix guard plus default-false is the entire mitigation, and it's worth more than any amount of cleverness elsewhere.

**Cut as speculative:** linking a Docker SDK or any container API; per-env containers for anything other than a database; snapshot/restore beyond what `CREATE DATABASE … TEMPLATE` gives for free.

**Acceptance criteria:** Creating an env on a `template`-mode repo produces `bish_<env>` cloned from the template in seconds, and the env's services connect to it with no manual `DATABASE_URL` editing. `pnpm db:reset` inside env A leaves env B's data untouched. Destroying an env with the drop box ticked removes only `bish_<env>`; with it unticked, the database survives for a fast restart. A `DBSpec` whose `DBName` doesn't match `^bish_` refuses to drop.

**Effort:** S

---

# 6. Step caching

**Gap:** Every start re-runs its enabled pre-start steps. `prestartCmd` has two skip heuristics — `Supersedes` (a `db:reset` covers `migrate`) and the symlinked-`node_modules` install skip (`hasLink`) — but nothing notices that a lockfile hasn't changed since the last successful install.

**Why it matters:** It doesn't, much. This is the lowest-value item on the list and the most dangerous, and the plan should say so plainly.

The original framing was to borrow Nx Cloud's strategy and "only update the latest chunks." That doesn't transfer: Nx Cloud's value is a *shared, networked, team* cache, and bish is a single-user desktop app with no cache server and no team. There is nothing to chunk. What survives the translation is worth about twenty lines, and where a repo already has a remote cache — both feather and nuna have Nx Cloud configured — bish should simply let that do its job rather than wrapping it.

**Plan:**

```go
// Step gains
CacheInputs []string `json:"cacheInputs"`        // globs rel to the repo dir; empty = never cached
Stateful    bool     `json:"stateful,omitempty"` // effect lives outside the repo (DB, queue) — never cached
```

1. Hash = sha256 of the step name, the step command, the resolved `Toolchain` string, and for each glob match, sorted `(relpath, size, mtimeUnixNano)`. **Stat, not content** — the point is to never walk `node_modules`.
2. Store per-repo-path, per-user: `~/.config/bish/cache/steps/<md5(repoPath)>/<stepName>.json` holding `{hash, at, exitCode}`. Skip only when the hash matches **and** the recorded `exitCode == 0`.
3. Wails: `ClearStepCache(repoID string) error`. Nothing else.

**Where this is dangerous — the part that must not be softened:** a hash hit means *the inputs did not change*, not *the effect is still present*. `migrate` is the canonical trap. The repo files are byte-identical, but the target database may be brand new, dropped by a sibling env, or sitting at a different revision. Caching it would silently boot a service against an unmigrated schema, and the failure would surface somewhere far away as a confusing runtime error. Therefore:

- `Destructive` steps: never cached.
- `Stateful` steps: never cached in v1. (If this is ever revisited, the key must include `envName` and `dbName` — and even then it cannot see an out-of-band `dropdb`.)
- v1 caches `Kind` of `install` and `codegen` **only**, and only when the user explicitly ticks `cacheInputs` on the step.
- A visible `skipped (cached)` line in the prestart log, plus a per-start "ignore cache" modifier. **Never a silent skip.**

For feather and nuna this reduces to `cacheInputs: ["pnpm-lock.yaml", "package.json", "**/package.json"]` on the install step — which is the actual 90% win, since Nx Cloud already covers codegen and build.

**Cut as speculative:** remote, shared, or chunked cache transfer; artifact upload/download; cache eviction policy; caching any stateful or destructive step.

**Acceptance criteria:** Starting the same env twice with no source changes skips install with a visible `skipped (cached)` line and no measurable time cost. Touching `pnpm-lock.yaml` re-runs it. A `migrate` or `db:reset` step runs every time regardless of cache state. "Ignore cache" forces a full run.

**Effort:** S

---

# Feature flags

New rows for `frontend/src/lib/features.ts`, section `'editor'`. Per the registry's own contract, each needs a `featureOn()` guard where OFF means no goroutine, no container, and no poll — not hidden UI.

| id | Default | Guard site |
|---|---|---|
| `harness` | off | Master. Gates the harness UI in `CommandCenter.svelte` and every harness goroutine in `commandcenter.Manager`. OFF = today's Command Center exactly. |
| `harnessDetect` | off | The "Detect services" button; `DetectRepoEnv` returns early. |
| `harnessEnvs` | off | The env `<select>` and `+`/trash controls; `loadState` still migrates, but `StartEnv`/`CreateEnv` reject. OFF = single "default" env, i.e. current behavior. |
| `harnessDb` | off | `DBSpec` execution. OFF = no `Create`/`Drop` ever runs. |
| `harnessPreview` | **on** | `openPreviewTab`; port badges fall back to `BrowserOpenURL`. On because it's inert until clicked. |
| `harnessCache` | off | Hash computation and the skip decision. OFF = no stat walk, every step runs. |
| `harnessHealth` | off | The richer probe types (`HTTP`, `LogMatch`, `Cmd`). The plain TCP dial is **not** gated — see area 1. |

Add the corresponding unchecked rows to `FEATURES.md` under a new "Harness" heading in the same change, so the roadmap and this document don't diverge on day one.

# Package layout

`internal/app/app.go` is already 2517 lines, and the established pattern is new subsystems landing as sibling files in the same package (`git.go`, `symbols.go`, `tests.go`, `outline.go`).

- **New:** `internal/envdetect/` (area 2), `internal/health/` (area 1).
- **Extended:** `internal/commandcenter/` gets `Env`, `DBSpec`, and the port math — they're inseparable from `Target` and the worktree logic, and splitting them out would mean exporting most of the package.
- **New sibling:** all new Wails methods land in `internal/app/harness.go`. None get appended to `app.go`.
- **Frontend:** one new `Preview.svelte`; `CommandCenter.svelte` grows the env header and the detect modal; `stores.ts` gains the `preview` tab type.

# Two incidental cleanups

1. **`killPort` is duplicated verbatim** in `internal/process/ports.go` and `internal/commandcenter/manager.go:448`. Area 4 has to make it env-aware anyway, so collapse to one exported function at the same time rather than teaching both copies the same new rule.
2. **Every new DTO needs normalizing on both sides.** Go nil slices marshal to JSON `null`, and the Svelte side indexes these arrays unguarded — a `null` crashes the render and silently strands the UI on stale state. This is why `normalizeDefinition` (Go) and `normalizeCC` (TS) exist. `Proposal.Services`, `Proposal.Steps`, `Proposal.Notes`, `Env.Targets`, `Step.CacheInputs`, and `Health` all need the same treatment.

# Cut list

Stated once, in one place, so none of it creeps back in:

- A Docker SDK or any container API dependency. Compose is shelled out to, or not used.
- Per-env containers for anything but a database.
- Console and error capture in Preview via an injecting loopback proxy.
- Preview device chrome, screenshots, and history stack.
- Remote, shared, or chunked cache transfer; cache eviction policy.
- Caching any stateful or destructive step.
- Env presets, templates, or a shareable environment marketplace.
- Any configuration DSL beyond shell strings plus `{{port}}` / `{{db}}` / `{{env}}` / `{{file}}` / `{{template}}` substitution.
- Auto-offsetting services that can't accept a port. Fail loud instead.

# End state

Opening a workspace with two unfamiliar repos, clicking "Detect services" on each, and clicking "New env" twice gives two complete, isolated environments — distinct worktrees, offset ports, separate databases, shared infra started once — each app one click from an interactive Preview tab, with honest per-service readiness in the status dots. No YAML written by hand, nothing specific to any one company's stack anywhere in bish.
