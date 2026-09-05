import type { EditorView } from '@codemirror/view'
import { indentInfo, replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = (view: EditorView) => {
  const { size, useTabs } = indentInfo(view)
  const out = JSON.stringify(JSON.parse(view.state.doc.toString()), null, useTabs ? '\t' : size)
  replaceAll(view, out + '\n')
}
