import type { EditorView } from '@codemirror/view'
import { StreamLanguage } from '@codemirror/language'
import { xml as xmlMode } from '@codemirror/legacy-modes/mode/xml'
import { indentInfo, replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = async (view: EditorView) => {
  const { size, useTabs } = indentInfo(view)
  const { default: format } = await import('xml-formatter')
  replaceAll(view, format(view.state.doc.toString(), { indentation: useTabs ? '\t' : ' '.repeat(size) }))
}

export const grammar: LanguageModule['grammar'] = () => StreamLanguage.define(xmlMode)
