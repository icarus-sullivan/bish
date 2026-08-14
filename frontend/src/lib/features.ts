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
    label: 'AI Assistant panel',
    hint: 'Sidebar chat that drives the claude CLI: plan-mode preview, editor context, approve/reject. Spawns a claude subprocess on first use — off by default since it costs money.' },
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
