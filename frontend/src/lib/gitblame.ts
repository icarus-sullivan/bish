// Inline git blame: GitLens-style annotation at the end of the cursor's line.
// ponytail: no dirty-buffer line mapping — annotation hides while the doc is
// modified and returns after the next save.
import { EditorView, Decoration, ViewPlugin, WidgetType } from '@codemirror/view'
import type { DecorationSet, ViewUpdate } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'
import type { Extension } from '@codemirror/state'
import { GitBlame } from './wails'
import type { BlameLine } from './wails'

const setBlame = StateEffect.define<{ line: number; text: string } | null>()
const refreshEffect = StateEffect.define<null>()

export function refreshBlame(view: EditorView) {
  view.dispatch({ effects: refreshEffect.of(null) })
}

class BlameWidget extends WidgetType {
  constructor(readonly text: string) { super() }
  eq(other: BlameWidget) { return other.text === this.text }
  toDOM() {
    const el = document.createElement('span')
    el.className = 'cm-blame-widget'
    el.textContent = this.text
    return el
  }
}

const blameField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) {
      if (e.is(setBlame)) {
        if (!e.value || e.value.line > tr.state.doc.lines) return Decoration.none
        const line = tr.state.doc.line(e.value.line)
        return Decoration.set([
          Decoration.widget({ widget: new BlameWidget(e.value.text), side: 1 }).range(line.to),
        ])
      }
    }
    if (tr.docChanged) return Decoration.none
    return deco.map(tr.changes)
  },
  provide: f => EditorView.decorations.from(f),
})

function relTime(unix: number): string {
  const s = Math.max(0, Date.now() / 1000 - unix)
  if (s < 60) return 'just now'
  const m = s / 60, h = m / 60, d = h / 24, mo = d / 30, y = d / 365
  const f = (n: number, u: string) => `${Math.floor(n)} ${u}${Math.floor(n) === 1 ? '' : 's'} ago`
  if (m < 60) return f(m, 'minute')
  if (h < 24) return f(h, 'hour')
  if (d < 30) return f(d, 'day')
  if (mo < 12) return f(mo, 'month')
  return f(y, 'year')
}

function format(b: BlameLine): string {
  if (/^0+$/.test(b.sha)) return 'You · Uncommitted changes'
  return `${b.author}, ${relTime(b.time)} · ${b.summary}`
}

const blameTheme = EditorView.baseTheme({
  '.cm-blame-widget': {
    color: 'color-mix(in srgb, var(--muted) 75%, transparent)',
    fontStyle: 'italic',
    paddingLeft: '2em',
    pointerEvents: 'none',
  },
})

export function gitBlame(path: string): Extension {
  let blame: BlameLine[] | null = null
  let stale = false

  const plugin = ViewPlugin.define(view => {
    let shown = ''
    let destroyed = false

    function show() {
      const ln = view.state.doc.lineAt(view.state.selection.main.head).number
      const b = !stale && blame ? blame[ln - 1] : null
      const text = b ? format(b) : ''
      const key = text ? `${ln}:${text}` : ''
      if (key === shown) return
      shown = key
      // can't dispatch inside an update cycle
      queueMicrotask(() => {
        if (destroyed) return
        view.dispatch({ effects: setBlame.of(text ? { line: ln, text } : null) })
      })
    }

    function fetchBlame() {
      GitBlame(path).then(b => {
        if (destroyed) return
        blame = b && b.length ? b : null
        stale = false
        show()
      }).catch(() => {})
    }

    fetchBlame()
    return {
      update(u: ViewUpdate) {
        if (u.transactions.some(tr => tr.effects.some(e => e.is(refreshEffect)))) {
          fetchBlame()
          return
        }
        if (u.docChanged) stale = true
        if (u.docChanged || u.selectionSet) show()
      },
      destroy() { destroyed = true },
    }
  })

  return [blameField, plugin, blameTheme]
}
