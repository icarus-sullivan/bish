import { writable } from 'svelte/store'
import { on } from './wails'

// Native Preview views (Preview.svelte, internal/app/browser.go) paint above
// every HTML element, so a modal, palette or context menu opened over one
// would render underneath it. Each such overlay holds a block while open
// and the native views hide until the count drops back to zero.
export const nativeViewBlockers = writable(0)

export function blockNativeViews(): () => void {
  nativeViewBlockers.update(n => n + 1)
  let released = false
  return () => {
    if (released) return
    released = true
    nativeViewBlockers.update(n => n - 1)
  }
}

// A focused native view swallows keystrokes, so it forwards ⌘-combos as
// "browser:key"; replay them where keybinds.ts listens. Registered once,
// however many Preview tabs are open.
let forwarding = false
export function forwardNativeViewKeys() {
  if (forwarding) return
  forwarding = true
  on('browser:key', (k: KeyboardEventInit) => {
    window.dispatchEvent(new KeyboardEvent('keydown', { ...k, bubbles: true, cancelable: true }))
  })
}
