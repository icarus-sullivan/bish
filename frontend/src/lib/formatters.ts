// Built-in, LSP-free formatting for file types no configured language server
// formats. gopls/pyright/typescript-language-server/svelte-language-server
// all speak textDocument/formatting for their own language, but nothing
// bish attaches handles SQL/JSON/YAML (pyright, notably, doesn't implement
// formatting for Python either — that's a server-capability gap, not
// something fixable from here). These run locally instead of round-tripping
// through a server that doesn't exist.
import { EditorView } from '@codemirror/view'
import { getIndentUnit, indentUnit } from '@codemirror/language'
import { format as formatSql } from 'sql-formatter'
import { parseDocument } from 'yaml'
import { lspFormat } from './lsp'

function extOf(path: string): string {
  return path.split('.').pop()?.toLowerCase() ?? ''
}

function indentInfo(view: EditorView) {
  return {
    size: getIndentUnit(view.state),
    useTabs: view.state.facet(indentUnit).indexOf('\t') >= 0,
  }
}

function replaceAll(view: EditorView, text: string) {
  if (text === view.state.doc.toString()) return
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })
}

const builtins: Record<string, (view: EditorView) => void> = {
  json(view) {
    const { size } = indentInfo(view)
    replaceAll(view, JSON.stringify(JSON.parse(view.state.doc.toString()), null, size) + '\n')
  },
  yaml(view) {
    const { size } = indentInfo(view)
    const doc = parseDocument(view.state.doc.toString())
    replaceAll(view, doc.toString({ indent: size }))
  },
  sql(view) {
    const { size, useTabs } = indentInfo(view)
    replaceAll(view, formatSql(view.state.doc.toString(), { tabWidth: size, useTabs }))
  },
}
builtins.yml = builtins.yaml

// Single entry point FileViewer calls for both "Format Document" and
// format-on-save: a built-in formatter when the extension has one,
// otherwise the attached language server (throws if none is attached).
export async function formatDocument(view: EditorView, path: string): Promise<void> {
  const builtin = builtins[extOf(path)]
  if (builtin) {
    builtin(view)
    return
  }
  await lspFormat(view)
}
