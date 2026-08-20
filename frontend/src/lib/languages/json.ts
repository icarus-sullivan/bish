import type { EditorView } from '@codemirror/view'
import { getIndentUnit } from '@codemirror/language'
import { replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = (view: EditorView) => {
  const size = getIndentUnit(view.state)
  replaceAll(view, JSON.stringify(JSON.parse(view.state.doc.toString()), null, size) + '\n')
}
