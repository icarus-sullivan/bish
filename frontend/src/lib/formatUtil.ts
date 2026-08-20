// Small shared helpers for the builtin (zero-install) per-language
// formatters under frontend/src/lib/languages/ — every builtin formatter
// does "compute new text, replace the whole document" with the same
// indent-detection and no-op-if-unchanged logic, so it lives once here
// instead of once per language module.
import type { EditorView } from '@codemirror/view'
import { getIndentUnit, indentUnit } from '@codemirror/language'

export function indentInfo(view: EditorView) {
  return {
    size: getIndentUnit(view.state),
    useTabs: view.state.facet(indentUnit).indexOf('\t') >= 0,
  }
}

export function replaceAll(view: EditorView, text: string) {
  if (text === view.state.doc.toString()) return
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })
}
