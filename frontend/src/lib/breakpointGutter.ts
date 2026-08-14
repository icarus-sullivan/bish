// Click-to-toggle breakpoint gutter. Line set is mirrored in from the
// dap.ts `breakpoints` store (shared with the Debug panel) via a StateField,
// same wiring shape as gitGutter's diff field.
import { gutter, GutterMarker, ViewPlugin } from '@codemirror/view'
import { EditorView } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'
import type { Extension } from '@codemirror/state'
import { breakpoints, toggleBreakpoint } from './dap'

const setLines = StateEffect.define<Set<number>>()

const linesField = StateField.define<Set<number>>({
  create: () => new Set(),
  update(val, tr) {
    for (const e of tr.effects) if (e.is(setLines)) return e.value
    return val
  },
})

class BreakpointMarker extends GutterMarker {
  toDOM() {
    const d = document.createElement('div')
    d.className = 'cm-breakpoint-dot'
    return d
  }
}
const marker = new BreakpointMarker()

const bpTheme = EditorView.baseTheme({
  '.cm-breakpoint-gutter': { width: '14px', cursor: 'pointer' },
  '.cm-breakpoint-dot': {
    width: '8px', height: '8px', borderRadius: '50%',
    background: 'var(--error)', margin: '4px auto 0',
  },
})

export function breakpointGutter(path: string): Extension {
  const bpGutter = gutter({
    class: 'cm-breakpoint-gutter',
    lineMarker(view, line) {
      const set = view.state.field(linesField)
      if (set.size === 0) return null
      const ln = view.state.doc.lineAt(line.from).number
      return set.has(ln) ? marker : null
    },
    lineMarkerChange(update) {
      return update.transactions.some(tr => tr.effects.some(e => e.is(setLines)))
    },
    domEventHandlers: {
      click(view, line) {
        toggleBreakpoint(path, view.state.doc.lineAt(line.from).number)
        return true
      },
    },
  })
  const plugin = ViewPlugin.define(view => {
    // subscribe fires synchronously with the current value — defer that
    // first dispatch a tick so it doesn't land mid-construction of the view
    let first = true
    const unsub = breakpoints.subscribe(m => {
      const lines = m.get(path) ?? new Set()
      if (first) { first = false; queueMicrotask(() => view.dispatch({ effects: setLines.of(lines) })) }
      else view.dispatch({ effects: setLines.of(lines) })
    })
    return { destroy: unsub }
  })
  return [linesField, bpGutter, plugin, bpTheme]
}
