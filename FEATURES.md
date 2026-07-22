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
- [ ] Non-macOS support (heavy `_darwin.go` / `open`/`dscl`/`ps` reliance)
