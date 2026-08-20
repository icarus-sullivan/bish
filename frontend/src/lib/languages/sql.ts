import type { EditorView } from '@codemirror/view'
import { format as formatSql } from 'sql-formatter'
import { indentInfo, replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = (view: EditorView) => {
  const { size, useTabs } = indentInfo(view)
  replaceAll(view, formatSql(view.state.doc.toString(), { tabWidth: size, useTabs }))
}
