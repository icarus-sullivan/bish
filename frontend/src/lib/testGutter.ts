// Test gutter: a play triangle next to `func TestXxx(t *testing.T)`
// declarations in a Go _test.go file. Click runs just that test through the
// same process-manager mechanism as Task Runner; the triangle recolors once
// goTests.ts resolves pass/fail. Same wiring shape as gitGutter/breakpointGutter.
import { EditorView, gutter, GutterMarker, ViewPlugin } from '@codemirror/view'
import type { ViewUpdate } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'
import type { Extension } from '@codemirror/state'
import { FileTests } from './wails'
import type { GoTest } from './wails'
import { runGoTest, testStatus, statusFor } from './goTests'

const setTests = StateEffect.define<GoTest[]>()
// re-parse from disk (a save may have added/removed a test func)
const refreshEffect = StateEffect.define<null>()
// repaint only — a test's pass/fail status changed, testsField is still valid
const repaintEffect = StateEffect.define<null>()

export function refreshTests(view: EditorView) {
  view.dispatch({ effects: refreshEffect.of(null) })
}

const testsField = StateField.define<GoTest[]>({
  create: () => [],
  update(val, tr) {
    for (const e of tr.effects) if (e.is(setTests)) return e.value
    return val
  },
})

class TestMarker extends GutterMarker {
  constructor(readonly status: string) { super() }
  eq(o: TestMarker) { return o.status === this.status }
  toDOM() {
    const d = document.createElement('div')
    d.className = 'cm-test-play cm-test-' + this.status
    d.title =
      this.status === 'running' ? 'Running…' :
      this.status === 'passed' ? 'Passed — click to re-run' :
      this.status === 'failed' ? 'Failed — click to re-run' : 'Run test'
    return d
  }
}

const markerCache = new Map<string, TestMarker>()
function markerFor(status: string): TestMarker {
  let m = markerCache.get(status)
  if (!m) { m = new TestMarker(status); markerCache.set(status, m) }
  return m
}

const testGutterExt = gutter({
  class: 'cm-test-gutter',
  lineMarker(view, line) {
    const tests = view.state.field(testsField)
    if (tests.length === 0) return null
    const ln = view.state.doc.lineAt(line.from).number
    const t = tests.find(t => t.line === ln)
    return t ? markerFor(statusFor(t.pkg, t.name)) : null
  },
  lineMarkerChange(update) {
    return update.transactions.some(tr => tr.effects.some(e => e.is(setTests) || e.is(repaintEffect)))
  },
  domEventHandlers: {
    click(view, line) {
      const tests = view.state.field(testsField)
      const ln = view.state.doc.lineAt(line.from).number
      const t = tests.find(t => t.line === ln)
      if (t) runGoTest(t.pkg, t.name)
      return true
    },
  },
})

const testTheme = EditorView.baseTheme({
  '.cm-test-gutter': { width: '14px', cursor: 'pointer' },
  '.cm-test-play': {
    width: 0, height: 0, margin: '5px auto 0',
    borderTop: '4px solid transparent', borderBottom: '4px solid transparent',
    borderLeft: '6px solid var(--muted)',
  },
  '.cm-test-running': { borderLeftColor: 'var(--warning)' },
  '.cm-test-passed': { borderLeftColor: 'var(--success)' },
  '.cm-test-failed': { borderLeftColor: 'var(--error)' },
})

export function testGutter(path: string): Extension {
  const plugin = ViewPlugin.define(view => {
    let destroyed = false
    function fetch() {
      FileTests(path).then(tests => {
        if (!destroyed) view.dispatch({ effects: setTests.of(tests ?? []) })
      }).catch(() => {})
    }
    fetch()
    // any test's status changing (in this file or elsewhere) should
    // repaint this gutter — defer the subscribe's synchronous first
    // callback so it doesn't dispatch mid-construction of the view
    let first = true
    const unsub = testStatus.subscribe(() => {
      if (destroyed) return
      if (first) { first = false; return }
      queueMicrotask(() => { if (!destroyed) view.dispatch({ effects: repaintEffect.of(null) }) })
    })
    return {
      update(u: ViewUpdate) {
        if (u.transactions.some(tr => tr.effects.some(e => e.is(refreshEffect)))) fetch()
      },
      destroy() { destroyed = true; unsub() },
    }
  })
  return [testsField, testGutterExt, plugin, testTheme]
}
