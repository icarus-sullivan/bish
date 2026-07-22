// User-defined keybindings: map a combo to any registered command id. Additive
// on top of the built-in shortcuts; persisted client-side. Combos use the same
// syntax as keybinds.ts: "mod+shift+k", "alt+g", "f5" (mod = ⌘ or Ctrl).
import { get, writable } from 'svelte/store'
import { registerKeybind } from './keybinds'
import { listCommands } from './commands'
import { featureOn } from './features'

const KEY = 'bish.keybinds'

function load(): Record<string, string> {
  try { return JSON.parse(localStorage.getItem(KEY) || '{}') } catch { return {} }
}

// commandId -> combo
export const customKeybinds = writable<Record<string, string>>(load())
customKeybinds.subscribe(v => localStorage.setItem(KEY, JSON.stringify(v)))

let offs: (() => void)[] = []

// (Re)register all user bindings. Safe to call repeatedly.
export function applyCustomKeybinds() {
  offs.forEach(o => o())
  offs = []
  if (!featureOn('customKeybinds')) return
  for (const [id, combo] of Object.entries(get(customKeybinds))) {
    if (!combo.trim()) continue
    offs.push(registerKeybind({
      combo: combo.trim().toLowerCase(),
      handler: (e) => {
        const cmd = listCommands().find(c => c.id === id)
        if (cmd) { e.preventDefault(); cmd.run() }
      },
    }))
  }
}
