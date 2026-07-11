// Centralized global keyboard shortcut registry — the single place app-wide
// (window-scoped) combos are defined, so components don't each install their
// own competing keydown listener.
export interface Keybind {
  combo: string                      // 'mod+p', 'mod+shift+f', 'escape', 'arrowright', 's'
  handler: (e: KeyboardEvent) => void
  when?: () => boolean               // optional extra guard
}

const registry = new Set<Keybind>()
let installed = false

function comboOf(e: KeyboardEvent): string {
  const parts: string[] = []
  if (e.metaKey || e.ctrlKey) parts.push('mod')
  if (e.shiftKey) parts.push('shift')
  if (e.altKey) parts.push('alt')
  parts.push(e.key.toLowerCase())
  return parts.join('+')
}

const EDITABLE_SEL = 'input, textarea, select, [contenteditable], .cm-editor, .xterm'

function onKeydown(e: KeyboardEvent) {
  const combo = comboOf(e)
  // reverse insertion order: later-mounted (topmost overlay) wins first match
  for (const kb of [...registry].reverse()) {
    if (kb.combo !== combo) continue
    if (kb.when && !kb.when()) continue
    // bare-key combos (Enter, Escape, 's', arrows) respect editable fields;
    // mod-combos (Cmd+S, Cmd+Shift+V, Cmd+T, ...) must fire from inside them
    if (!combo.startsWith('mod+')) {
      const t = e.target as HTMLElement | null
      if (t?.closest?.(EDITABLE_SEL) || t?.isContentEditable) continue
    }
    kb.handler(e)
    return
  }
}

function ensureInstalled() {
  if (installed) return
  window.addEventListener('keydown', onKeydown)
  installed = true
}

export function registerKeybind(kb: Keybind): () => void {
  ensureInstalled()
  registry.add(kb)
  return () => registry.delete(kb)
}
