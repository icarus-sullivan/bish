import { StreamLanguage } from '@codemirror/language'
import { rust as rustMode } from '@codemirror/legacy-modes/mode/rust'
import type { LanguageModule } from '../languageExtensions'

export const grammar: LanguageModule['grammar'] = () => StreamLanguage.define(rustMode)
