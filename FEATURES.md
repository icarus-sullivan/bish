# bish — IDE Feature Progress

Living checklist. Mark `[x]` when done, `[ ]` when not. Every feature must be
toggleable in Settings (`frontend/src/lib/features.ts` registry + `featureOn()`
guard, OFF = zero cost). See `.claude/plans/` for the full gap analysis.

## Foundation
- [x] Feature-toggle framework (registry, config persistence, Settings UI)
- [x] In-file search match count ("3 / 17")

## Tier 1 — high value, fits architecture
- [x] Git change gutter (added/modified/deleted bars vs HEAD)
- [x] Git diff view (unified colored diff tab; click a file in Git panel)
- [x] Git write ops (stage / unstage / commit / branch switch) in GitPanel
- [x] Action command palette (⌘⇧P — runs commands, not just files)
- [x] Terminal keyboard shortcuts (new/close/next terminal, clear, rename tab, font zoom)
- [x] Symbol outline (sidebar panel, click-to-jump) — breadcrumbs still pending

## Tier 2 — high value, larger lift
- [ ] Split editor panes / side-by-side + diff-as-tab
- [ ] Split terminals (panes within a tab)
- [ ] Rename symbol + code actions UI (LSP data already there)
- [ ] Terminal command decorations / jump-between-prompts (exit-code marks)
- [x] Customizable keybindings (bind any command to a combo; Settings → Keyboard)

## Tier 3 — nice to have
- [x] Snippets (@codemirror/autocomplete native)
- [ ] Minimap
- [ ] Sticky scroll
- [ ] Standalone linting (no LSP required)
- [x] bash shell integration (PROMPT_COMMAND + DEBUG trap; cwd/title/w/gallery)
- [x] Copy-on-select (terminal; opt-in toggle) — paste sanitization still pending
- [ ] Configurable ANSI palette (16 colors hardcoded today)

## Cross-cutting / strategic
- [x] AI layer (native Assistant panel: claude subprocess, plan-mode preview, editor context) — inline completions / terminal command suggestions still pending
- [x] Claude panel parity with the VS Code extension: streaming text/thinking, conversation tabs, past-conversation history + resume, fork/edit-and-rewind, file checkpoints (restore code), inline Edit/Write diffs, rich permission prompts ("always allow" from CLI suggestions), plan approval modes, AskUserQuestion, todo tracker, subagent nesting, @-file mentions, dynamic slash commands, model/effort/thinking switching, context meter, MCP status, ⌘Esc focus / ⌘⌥K add selection
- [x] Codex panel (codex app-server JSON-RPC) beside Claude in the AI panel — switchable, both run concurrently: streaming, tabs, thread history/resume, edit-rewind (thread/revert), command/patch approvals, user-input questions, plan → todos, token meter, model/effort, MCP status, /compact
- [x] Voice dictation in the AI panel composer (Claude + Codex): mic button → offline speech recognition in the webview (vosk-browser, bundled small en-US model) → words stream into the prompt live as you talk
- [ ] Non-macOS support (heavy `_darwin.go` / `open`/`dscl`/`ps` reliance)
- [x] Per-extension sidebar panel icons (each contributed panel gets its own gutter icon, not just the aggregate Extensions panel)
- [x] Extension icons in the tab bar (right-aligned; click opens that extension alone in its own dock beside the editor). Manifest panel `icon` = tabler name, http(s) URL, base64 / data: URI, or local image file (svg/png/jpg/gif/webp/ico). Toggle: `extensionTopbar` — off = icons back in the sidebar strip

## Harness (see HARNESS_PLAN.md)
- [x] Readiness in Go (status phases; rich probes toggle: `harnessHealth`)
- [x] Env auto-detection — "Detect services" (toggle: `harnessDetect`)
- [x] Preview tab — in-app iframe for launched web UIs (toggle: `harnessPreview`; auto-open: `harnessPreviewAuto`)
- [x] Ephemeral environments — side-by-side envs with port offsets (toggle: `harnessEnvs`)
- [x] Database lifecycle — per-env template/compose databases (toggle: `harnessDb`)
- [x] Step caching — content-hash skip for install/codegen (toggle: `harnessCache`)
