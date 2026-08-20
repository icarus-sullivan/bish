// RFC4180 normalize: re-quote only fields that need it (contain a comma,
// quote, or newline), and settle line endings — no external dependency,
// CSV "formatting" doesn't need a real parser/AST like the other builtins.
import type { EditorView } from '@codemirror/view'
import { replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

function parseRows(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = [], field = '', inQuotes = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (inQuotes) {
      if (c === '"') {
        if (text[i + 1] === '"') { field += '"'; i++ }
        else inQuotes = false
      } else field += c
    } else if (c === '"') {
      inQuotes = true
    } else if (c === ',') {
      row.push(field); field = ''
    } else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++
      row.push(field); field = ''
      rows.push(row); row = []
    } else {
      field += c
    }
  }
  if (field !== '' || row.length) { row.push(field); rows.push(row) }
  return rows
}

function quoteField(f: string): string {
  return /[",\n]/.test(f) ? '"' + f.replace(/"/g, '""') + '"' : f
}

export const formatter: LanguageModule['formatter'] = (view: EditorView) => {
  const rows = parseRows(view.state.doc.toString())
  replaceAll(view, rows.map(r => r.map(quoteField).join(',')).join('\n') + '\n')
}
