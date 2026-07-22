// Git change gutter: colored bars marking added/modified/deleted lines vs the
// index/HEAD, like VSCode. ponytail: no dirty-buffer line mapping — bars stay at
// their fetched line numbers until the next save refresh (same tradeoff as
// gitblame.ts).
import { EditorView, gutter, GutterMarker, ViewPlugin } from '@codemirror/view'
import type { ViewUpdate } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'
import type { Extension } from '@codemirror/state'
import { GitDiff } from './wails'

const setDiff = StateEffect.define<Map<number, string>>()
const refreshEffect = StateEffect.define<null>()

export function refreshDiff(view: EditorView) {
  view.dispatch({ effects: refreshEffect.of(null) })
}

const diffField = StateField.define<Map<number, string>>({
  create: () => new Map(),
  update(val, tr) {
    for (const e of tr.effects) if (e.is(setDiff)) return e.value
    return val
  },
})

class ChangeMarker extends GutterMarker {
  constructor(readonly type: string) { super() }
  eq(o: ChangeMarker) { return o.type === this.type }
  toDOM() {
    const d = document.createElement('div')
    d.className = 'cm-diff-bar cm-diff-' + this.type
    return d
  }
}
const markers: Record<string, ChangeMarker> = {
  added: new ChangeMarker('added'),
  modified: new ChangeMarker('modified'),
  deleted: new ChangeMarker('deleted'),
}

const diffGutter = gutter({
  class: 'cm-diff-gutter',
  lineMarker(view, line) {
    const map = view.state.field(diffField)
    if (map.size === 0) return null
    const ln = view.state.doc.lineAt(line.from).number
    const type = map.get(ln)
    return type ? markers[type] : null
  },
  lineMarkerChange(update) {
    return update.transactions.some(tr => tr.effects.some(e => e.is(setDiff)))
  },
})

const diffTheme = EditorView.baseTheme({
  '.cm-diff-gutter': { width: '3px' },
  '.cm-diff-bar': { width: '3px', height: '100%', boxSizing: 'border-box' },
  '.cm-diff-added': { background: 'var(--success)' },
  '.cm-diff-modified': { background: 'var(--accent)' },
  // deletion: a wedge at the top border of the line below the removed block
  '.cm-diff-deleted': {
    background: 'transparent',
    height: '0',
    borderTop: '4px solid var(--error)',
  },
})

export function gitGutter(path: string): Extension {
  const plugin = ViewPlugin.define(view => {
    let destroyed = false
    function fetch() {
      GitDiff(path).then(lines => {
        if (destroyed) return
        const map = new Map<number, string>()
        for (const d of lines ?? []) {
          const ln = d.type === 'deleted' ? Math.max(1, d.line) : d.line
          // added/modified win over a deletion marker on the same line
          if (!map.has(ln) || map.get(ln) === 'deleted') map.set(ln, d.type)
        }
        view.dispatch({ effects: setDiff.of(map) })
      }).catch(() => {})
    }
    fetch()
    return {
      update(u: ViewUpdate) {
        if (u.transactions.some(tr => tr.effects.some(e => e.is(refreshEffect)))) fetch()
      },
      destroy() { destroyed = true },
    }
  })
  return [diffField, diffGutter, plugin, diffTheme]
}
