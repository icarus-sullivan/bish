# PM + Marketing Asks — Remaining Gaps

Follow-up to `PM_ASKS.md` (12 competitor feature gaps, all shipped or explicitly scoped). This pass looks at two different questions: **could someone who isn't already reading the source code discover, trust, and install this thing** (marketing/GTM), and **what product gaps remain that the first pass didn't touch** (PM). Every item below is grounded in something actually checked in this repo — not generic advice.

Same format as before: gap, why it matters, a concrete plan, acceptance criteria, effort. Ordered by leverage within each section, not strict priority.

Shipped and removed from this doc: opt-in telemetry, merge-conflict resolution UI, desktop notifications, a user-facing snippet manager, custom theme creation, first-run onboarding, a keyboard shortcuts overlay, Saved Commands in Settings Sync, and an accessibility pass on modal dialogs.

---

# Part 1 — Marketing / GTM (9)

## 1. No README

**Gap:** There is no `README.md` anywhere in the repo. `git remote -v` points at a real GitHub repo (`icarus-sullivan/bish`) — anyone who lands on it via a link, search, or `git clone` sees a bare file tree with no explanation of what bish is, what it does, or why it's different from VS Code/Cursor/Warp.

**Why it matters:** This is the single highest-leverage item in this whole doc. A README is the actual product page for an open-source project — it's what gets skimmed in the 5 seconds before someone clicks away or stars the repo.

**Plan:**
1. Above-the-fold: one-sentence positioning ("a shell-first IDE — terminal and editor as equals, not editor-with-a-terminal-tab-bolted-on"), a screenshot or GIF, install instructions.
2. Feature highlights section pulling from what's actually shipped (AI assistant, git panel, LSP, debugger, multi-root, remote SSH, live share) — not aspirational.
3. Quickstart: clone → `make init` → `make dev`, and the two lines needed to install a built app (`make build && make install`).
4. Link to `FEATURES.md` for the full roadmap, and to `CONTRIBUTING.md` (item #7) once it exists.

**Acceptance criteria:** A person who has never seen bish before can read the README top-to-bottom in under a minute and know what it is, what makes it different, and how to try it.

**Effort:** S

---

## 2. No LICENSE

**Gap:** No `LICENSE` file. Legally, that means "all rights reserved" by default — nobody can safely fork, redistribute, or even use the code in a company setting without asking first, regardless of what the README might imply.

**Why it matters:** This silently blocks the exact audience most likely to adopt an open-source dev tool (other developers who'd fork it, package it, or vendor a piece of it). It also blocks inclusion in Homebrew, package indexes, and "awesome-lists" — most have a hard requirement for a detectable license.

**Plan:** Pick a license (MIT or Apache-2.0 are the standard choices for a dev-tool repo like this) and add the `LICENSE` file at the root; add the SPDX identifier to `go.mod`'s neighborhood or `package.json` if conventional for the ecosystem.

**Acceptance criteria:** GitHub's own UI shows a detected license badge on the repo page; `frontend/package.json`'s `license` field (currently unset) matches.

**Effort:** S (the decision might take longer than the change)

---

## 3. No distributable build — install requires the full dev toolchain

**Gap:** `Makefile`'s only install path is `make build && make install`, which needs Go, the `wails` CLI, and `pnpm` already set up locally. There's no signed/notarized `.dmg`, no Homebrew tap/cask, no GitHub Release with a downloadable binary, and no auto-update mechanism.

**Why it matters:** Every competitor named in `PM_ASKS.md` (VS Code, Cursor, Zed, Warp) is a double-click install. Requiring a full Go+Node dev environment just to *try* the app caps the addressable audience at "people who were already going to read the source anyway."

**Plan:**
1. Wire GitHub Actions (ties into item #7's CI ask) to build `make darwin` on tag push, produce a `.dmg`, and attach it to a GitHub Release.
2. Apple notarization (needs a paid Developer ID — a real cost/time tradeoff to flag explicitly, not hide).
3. A Homebrew cask (`brew install --cask bish`) once a release artifact exists to point it at.
4. Defer auto-update (Sparkle or similar) to a later pass — note it here so it isn't forgotten, but don't block the release pipeline on it.

**Acceptance criteria:** A user with zero dev tools installed can download a `.dmg` from a GitHub Release, drag it to Applications, and open it without a Gatekeeper "unidentified developer" block.

**Effort:** M (the CI+release pipeline is straightforward; notarization is the long pole — Apple Developer Program enrollment and cert setup)

---

## 4. No screenshots, GIF, or demo video anywhere in the repo

**Gap:** `icons/` contains exactly three files — the app icon and two logo marks. There is not a single screenshot of the actual editor, terminal, git panel, or AI assistant anywhere a prospective user could see it without cloning and building the app themselves.

**Why it matters:** For a visual product (an IDE), "show, don't tell" isn't optional — it's the primary way people evaluate whether the UI matches their taste before investing 10 minutes in a build. This blocks the README (#1), any landing page (#5), and any launch post (#9) simultaneously.

**Plan:**
1. Capture 4–6 screenshots covering: the main editor+terminal split, the Git panel with a diff open, the AI Assistant panel mid-conversation, the Debug panel with a breakpoint hit, and the multi-root file tree.
2. One 15–30s GIF or short screen recording showing the core loop: open a file, edit, run a task, see it in the terminal.
3. Store these in a `docs/` or `.github/` assets folder (GitHub renders images inline in Markdown directly from the repo) and reference them from the README.

**Acceptance criteria:** The README renders at least one screenshot and one GIF inline on GitHub's web UI, no external image host required.

**Effort:** S (mostly capture + light editing time, not engineering)

---

## 5. No landing page or product site

**Gap:** There's no website anywhere in the repo or referenced by it — the GitHub repo (once it has a README) is the *only* thing to link to.

**Why it matters:** A GitHub repo is a fine landing spot for developers who already read READMEs for fun, but a dedicated one-pager converts a much wider audience (a Twitter/X link preview, a Product Hunt "visit website" button, a Hacker News comment) — and it's the only place that can rank in search for "bish IDE" or "terminal-first code editor" independent of GitHub's own SEO.

**Plan:**
1. A single static page (could be GitHub Pages served straight from the repo — zero new hosting cost): hero section with the same positioning line as the README, the demo GIF from #4, a download link once #3 exists, and a feature grid.
2. Reuse the actual in-app dark/light theme colors and the icon marks already in `icons/` so it visually matches the product instead of looking like a generic template.
3. Wire basic OG/Twitter meta tags so link previews render the screenshot, not a blank card.

**Acceptance criteria:** Pasting the site URL into Slack/Twitter/iMessage shows a rich preview card with the product screenshot and one-line description.

**Effort:** M

---

## 6. No CHANGELOG

**Gap:** `release.config.yaml` implies a versioned release process exists (`version: 0.1.0`), but there's no `CHANGELOG.md` anywhere — a user updating the app (once #3's distribution exists) has no way to know what changed.

**Why it matters:** Especially relevant *because* of how much shipped in the last two `PM_ASKS.md` passes (debugger, remote dev, multi-root, live share, extensions...) — none of that is visible to an existing user unless they read git log. A changelog is also free marketing: "look how fast this is moving" is a genuine growth signal on its own.

**Plan:** Add `CHANGELOG.md` in Keep-a-Changelog format; backfill the last few versions from `git log`, then adopt a per-release-tag update as part of the same GitHub Actions release flow from #3 (a release without a changelog entry fails the workflow — cheap enforcement).

**Acceptance criteria:** Every tagged release has a corresponding `## [x.y.z]` section describing user-visible changes, not commit messages verbatim.

**Effort:** S

---

## 7. No CONTRIBUTING.md, issue/PR templates, or CI

**Gap:** No `.github/` directory at all — no issue templates, no PR template, and (confirmed by its absence) no GitHub Actions workflow running `go build`/`go vet`/`go test`/`pnpm run build` on pull requests. Every verification pass across the whole `PM_ASKS.md` build-out this session was run by hand, locally.

**Why it matters:** Two separate problems bundled into one gap: (a) outside contributors have zero guidance on how to get a dev environment running or what's expected of a PR, and (b) there's no safety net catching a regression before it merges — the exact kind of bug found and fixed live in `internal/process/ports.go` and `internal/liveshare` this session could ship silently without one.

**Plan:**
1. `.github/workflows/ci.yml`: on every PR, run `go build ./...`, `go vet ./...`, `go test ./...`, and `pnpm run build` in `frontend/`.
2. `CONTRIBUTING.md`: dev environment setup (mirrors `make init`), how to run tests, PR expectations.
3. `.github/ISSUE_TEMPLATE/` (bug report + feature request) and a minimal PR template checklist.

**Acceptance criteria:** A PR that breaks `go build` or `pnpm run build` shows a red CI check before a human ever has to notice.

**Effort:** S–M

---

## 8. Brand inconsistency: the CLI binary is named "bosh," not "bish"

**Gap:** `release.config.yaml` defines the app as `bish` but the CLI entry as `cli.name: bosh`. `Makefile`'s `install` target literally installs a binary called `bosh` into `~/.local/bin` on Linux. This is either a deliberate choice with no documented reason, or a copy-paste typo that's been shipping since — either way it's inconsistent with every other reference to the product (`bish.app`, `bish_icon.png`, the shell functions in `internal/pty/pty.go` all say "bish").

**Why it matters:** Brand names are load-bearing for word-of-mouth and search — "wait, is it bish or bosh?" is friction that costs recall and makes screenshots/docs/word-of-mouth inconsistent with each other.

**Plan:** Confirm with the maintainer whether "bosh" is intentional (e.g., avoiding a name collision with an existing `bish` CLI tool on some package index — worth actually checking) and either document the reasoning inline in `release.config.yaml` or rename `cli.name` to `bish` for consistency.

**Acceptance criteria:** Every user-facing surface (app name, CLI binary, docs, marketing copy) uses one name, or the divergence is explained in a comment where someone would actually see it before shipping a release.

**Effort:** S (the check-with-maintainer step matters more than the code change)

---

## 9. No community channel

**Gap:** Nothing in the repo points to a Discord, Slack, GitHub Discussions board, or any other place for users to ask questions, share configs, or show off an extension (relevant now that `examples/extensions/hello-world` exists — the Extension API from `PM_ASKS.md` #9 has literally nowhere for a user-built extension to be shared).

**Why it matters:** Every comparable tool (Zed, Warp, Cursor) has an active Discord that doubles as free support (deflects GitHub issues that are really questions) and a growth channel (people share their setups, which is itself marketing).

**Plan:** GitHub Discussions is the lowest-effort start (built into the repo, zero new infra) — enable it, seed it with a "Show your setup" and a "Extensions" category tied to #9's Extension API, and link it from the README.

**Acceptance criteria:** The README links to a live discussion/community space, and at least the "Show your setup" and "Extensions" categories exist.

**Effort:** S

---

# Part 2 — Product (2)

## 11. No in-app update checker

**Gap:** No code anywhere checks for a newer version or tells the user one exists. This is a direct consequence of #3 (no release pipeline to check *against*) but is worth calling out as its own gap since it's a separate piece of UI/UX even once #3 exists.

**User story:** "I've been on the same version for three months and had no idea a debugger, remote dev, and live share all shipped in the meantime."

**Plan:** Once #3's GitHub Releases exist, a simple periodic check (once per day, on startup) against the GitHub Releases API for a newer tag than the running `main.version` (already threaded through via ldflags in the `Makefile`), surfaced as a small non-blocking banner or Settings badge — not a forced update, just a signal, given #3 explicitly deferred true auto-update.

**Acceptance criteria:** Running an old build against a repo with a newer tagged release shows an update notice within one app launch, with no network call at all if the check is disabled.

**Effort:** S (once #3 exists — blocked on it, not concurrent)

---

## 12. No crash/error reporting

**Gap:** No Sentry/Bugsnag-equivalent, and no structured Go panic recovery + report path either. A Go-side panic or an uncaught frontend exception today is invisible to the maintainer unless a user notices, reproduces it, and manually files a GitHub issue with steps.

**Why it matters:** Every other item in this doc assumes stability the codebase hasn't independently verified — this session alone found and fixed a real concurrency bug (`internal/liveshare`) and a real platform-specific data bug (`internal/process/ports.go`'s `lsof` quirk) purely by choosing to write integration tests, not because anything surfaced them automatically. Those are the ones caught; production crashes with no reporting path are, definitionally, the ones that aren't.

**Plan:**
1. Go side: a top-level `recover()` in `main.go` around app startup and the goroutines in `internal/app` that already do the sensitive I/O (PTY loops, the liveshare/dap/lsp managers) — log to a local file at minimum, even before any remote reporting exists.
2. Frontend: a global `window.onerror`/`unhandledrejection` handler.
3. Opt-in only, same trust posture as the already-shipped opt-in telemetry (Settings > Privacy) — a crash report can contain a stack trace with a local file path in it, which is exactly the kind of thing this audience is sensitive about being sent without asking.

**Acceptance criteria:** A deliberately-triggered panic in a background goroutine no longer silently kills a feature with zero trace — at minimum it's in a local log file the user can find and attach to a bug report, opt-in remote reporting on top of that if enabled.

**Effort:** M

---

## Suggested sequencing

Free/near-free and highest-leverage first: **#1 README**, **#2 LICENSE**, **#6 CHANGELOG**, **#9 Community channel** — all S effort, and #1/#2 in particular are prerequisites for every other marketing item in this doc (nobody links to a landing page for a repo with no README).

Then the release pipeline, since #3/#4/#5/#11 all chain off it: **#3 Distributable build** → **#4 Screenshots/demo** → **#5 Landing page** → **#11 Update checker**.

Treat as its own scoped effort: **#12 Crash reporting** — touches enough of the app (Go panic recovery + a frontend error handler + a trust/opt-in decision) to warrant its own pass rather than a quick add-on.
