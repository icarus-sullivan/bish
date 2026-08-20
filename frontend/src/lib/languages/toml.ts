import type { EditorView } from '@codemirror/view'
import { StreamLanguage } from '@codemirror/language'
import { toml as tomlMode } from '@codemirror/legacy-modes/mode/toml'
import { replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = async (view: EditorView) => {
  const { parse, stringify } = await import('smol-toml')
  replaceAll(view, stringify(parse(view.state.doc.toString())))
}

export const grammar: LanguageModule['grammar'] = () => StreamLanguage.define(tomlMode)
