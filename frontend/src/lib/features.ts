import { writable, get } from 'svelte/store'

// ─── Feature toggles ────────────────────────────────────────────────────────
// Every optional/perf-sensitive feature registers one row here. Gate the
// feature's code with featureOn(id) so that OFF = zero cost (no extension
// mounted, no server spawned, no poll running) — not just hidden UI.
// Adding a new feature = one row below + a featureOn() guard at its mount.

export interface FeatureDef {
  id: string
  label: string
  hint: string
  default: boolean
  section: 'editor' | 'terminal'
}

export const FEATURES: FeatureDef[] = [
  { id: 'lsp', section: 'editor', default: true,
    label: 'Code intelligence (LSP)',
    hint: 'Completion, diagnostics, hover, go-to-definition, format-on-save. The heaviest editor feature — turn off for max performance on low-end machines.' },
  { id: 'gitBlame', section: 'editor', default: true,
    label: 'Inline git blame',
    hint: 'Author + commit annotation at the cursor line. Spawns git per file.' },
  { id: 'gitGutter', section: 'editor', default: true,
    label: 'Git change gutter',
    hint: 'Added/modified/deleted line bars in the gutter vs git HEAD. Spawns git per file.' },
  { id: 'gitDiff', section: 'editor', default: true,
    label: 'Diff on click (Git panel)',
    hint: 'Clicking a changed file in the Git panel opens its diff. Off = opens the file directly.' },
  { id: 'matchCount', section: 'editor', default: true,
    label: 'Search match count',
    hint: 'Show “3 / 17” next to the in-file search arrows.' },
  { id: 'commandPalette', section: 'editor', default: true,
    label: 'Command palette (⌘⇧P)',
    hint: 'Fuzzy action runner. Off = the ⌘⇧P shortcut is disabled.' },
  { id: 'snippets', section: 'editor', default: true,
    label: 'Code snippets',
    hint: 'Language snippets in autocomplete (log, fn, iferr, def…). Off = no snippet completions.' },
  { id: 'qwenComplete', section: 'editor', default: false,
    label: 'Local code completion (Qwen2.5-Coder)',
    hint: 'Lightweight local model spawned by the Go backend for inline boilerplate suggestions in the autocomplete popup. Off by default — needs llama-server + a downloaded model.' },
  { id: 'debugger', section: 'editor', default: false,
    label: 'Debugger (Go)',
    hint: 'Breakpoint gutter + Debug panel driving `dlv dap`. Off by default — needs delve installed. Go projects only for now.' },
  { id: 'tests', section: 'editor', default: true,
    label: 'Test Explorer (Go)',
    hint: 'Run-test gutter triangle in _test.go files + Tests sidebar panel, via `go test`. Go projects only for now.' },
  { id: 'extensions', section: 'editor', default: true,
    label: 'Extensions panel',
    hint: 'Loads local extensions from ~/.bish/extensions (each runs in its own Web Worker — see the "Extensions" section in Settings to enable/disable one).' },
  { id: 'extensionTopbar', section: 'editor', default: true,
    label: 'Extension icons in tab bar',
    hint: 'Each extension panel gets its own icon right of the tabs, opening that extension alone in its own dock beside the editor. Off = extension panels live in the sidebar strip instead.' },
  { id: 'languageExtensions', section: 'editor', default: true,
    label: 'Languages panel',
    hint: 'Per-language server/formatter install status and overrides (custom binary path, disable formatting, indent style). Off = panel hidden; language support itself is unaffected.' },
  { id: 'outline', section: 'editor', default: true,
    label: 'Symbol outline panel',
    hint: 'Sidebar panel listing functions/classes/types in the current file. Off = panel hidden.' },
  { id: 'problems', section: 'editor', default: true,
    label: 'Problems panel',
    hint: 'Sidebar panel aggregating LSP errors/warnings across the whole project, including files you haven\'t opened. Off = panel hidden.' },
  { id: 'keyboardShortcuts', section: 'editor', default: true,
    label: 'Tab/terminal shortcuts',
    hint: 'New terminal (⌘⇧T), close tab (⌘W), cycle tabs (⌘⇧[ / ⌘⇧]). Off = these shortcuts are disabled.' },
  { id: 'customKeybinds', section: 'editor', default: true,
    label: 'Custom keybindings',
    hint: 'Honor the command combos set below. Off = your custom bindings are ignored.' },
  { id: 'assistant', section: 'editor', default: false,
    label: 'AI panel (Claude Code / Codex)',
    hint: 'Agent chat for the claude and codex CLIs, switchable and able to run side by side: streaming, conversation tabs, history/resume, inline diffs, permission prompts, plan approval, todos, checkpoints (Claude), MCP status. Each tab spawns its CLI on its first message — off by default since it costs money.' },
  { id: 'dictation', section: 'editor', default: true,
    label: 'Voice dictation',
    hint: 'Microphone button in the AI panel composer — speech is recognized offline in the app (Vosk) and streamed into the prompt as you talk. Costs nothing until the first click, which loads the bundled speech model.' },
  { id: 'commandCenter', section: 'editor', default: true,
    label: 'Command Center panel',
    hint: 'Per-project service launcher: repos auto-discovered from this workspace\'s git folders, with branch/worktree checkout, pre-start steps, and cross-repo dependsOn ordering.' },
  { id: 'harness', section: 'editor', default: false,
    label: 'Dev harness (Command Center)',
    hint: 'Master switch for the harness extensions to Command Center below. Off = today\'s Command Center exactly: no detection, no envs, no databases, no step cache, no extra probes.' },
  { id: 'harnessDetect', section: 'editor', default: false,
    label: 'Harness: detect services',
    hint: '"Detect services" button on repo cards — proposes services/steps from package.json, nx, compose, Makefile, Procfile, go.mod, Cargo, Rails, Python. Nothing is written until you apply.' },
  { id: 'harnessEnvs', section: 'editor', default: false,
    label: 'Harness: ephemeral environments',
    hint: 'Named envs in the Command Center header — each its own worktrees and port offset, runnable side by side. Off = a single "default" env.' },
  { id: 'harnessDb', section: 'editor', default: false,
    label: 'Harness: per-env databases',
    hint: 'Runs a repo\'s declared db create/drop commands per env (template clone or compose project). Off = no database command ever runs.' },
  { id: 'harnessPreview', section: 'editor', default: true,
    label: 'Harness: Preview tab',
    hint: 'Port badges open the app in an in-app Preview tab (iframe) instead of the system browser. Inert until clicked.' },
  { id: 'harnessPreviewAuto', section: 'editor', default: false,
    label: 'Harness: auto-open Preview when ready',
    hint: 'Open a Preview tab the moment a Command Center service with a port becomes ready. Off by default — five apps becoming ready would open five tabs.' },
  { id: 'harnessCache', section: 'editor', default: false,
    label: 'Harness: step cache',
    hint: 'Skip install/codegen steps whose cacheInputs (lockfiles etc.) are unchanged since their last successful run. Off = no stat walk, every step runs.' },
  { id: 'harnessHealth', section: 'editor', default: false,
    label: 'Harness: rich health checks',
    hint: 'HTTP / log-match / command readiness probes per service. The plain port check is always on.' },
  { id: 'copyOnSelect', section: 'terminal', default: false,
    label: 'Copy on select',
    hint: 'Selecting text in the terminal copies it to the clipboard automatically.' },
  { id: 'terminalWebgl', section: 'terminal', default: true,
    label: 'GPU terminal renderer',
    hint: 'WebGL rendering (faster on capable GPUs). Disable on VMs / old GPUs if the terminal glitches or lags.' },
  { id: 'terminalBlocks', section: 'terminal', default: true,
    label: 'Command blocks',
    hint: 'A colored marker per command (bash/zsh only) — click for Copy Command, Copy Output, Rerun. Red on nonzero exit.' },
]

const defaults = (): Record<string, boolean> =>
  Object.fromEntries(FEATURES.map(f => [f.id, f.default]))

export const features = writable<Record<string, boolean>>(defaults())

// Merge saved config overrides onto registry defaults (missing key = default).
export function loadFeatures(saved: Record<string, boolean> | null | undefined) {
  features.set({ ...defaults(), ...(saved ?? {}) })
}

// Synchronous check for use at editor/terminal build time (not reactive).
export function featureOn(id: string): boolean {
  const v = get(features)[id]
  if (v !== undefined) return v
  return FEATURES.find(f => f.id === id)?.default ?? true
}
