import type { EditorView } from '@codemirror/view'
import { parseDocument } from 'yaml'
import { indentInfo, replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = (view: EditorView) => {
  const { size } = indentInfo(view)
  const doc = parseDocument(view.state.doc.toString())
  replaceAll(view, doc.toString({ indent: size }))
}
