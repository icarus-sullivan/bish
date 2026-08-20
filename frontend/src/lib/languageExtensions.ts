// Frontend-side registry for internal/langext's per-language definitions.
// Go owns the source of truth (server/formatter argv's, install commands —
// see ListLanguageExtensions in internal/app/app.go); this module fetches
// that list once at startup and is the single lookup point that replaces
// the old hardcoded ext→kind maps that used to live separately in
// codeintel.ts, lsp.ts, and (partially) FileViewer.svelte.
//
// Frontend-only concerns a Definition can't carry (a CodeMirror grammar, a
// custom settings component, a builtin zero-install formatter function)
// live in a same-id module under frontend/src/lib/languages/<id>.ts,
// dynamically imported via loadLanguage() below — a language only needs one
// if it has something to contribute beyond the Go-side defaults.
import { writable, get } from 'svelte/store'
import type { EditorView } from '@codemirror/view'
import type { Component } from 'svelte'
import { ListLanguageExtensions } from './wails'
import type { LanguageExtensionDTO } from './wails'

export const languageExtensions = writable<LanguageExtensionDTO[]>([])

// ext (leading dot, lowercase) → Definition, rebuilt whenever the registry
// loads/reloads.
let byExt = new Map<string, LanguageExtensionDTO>()

export async function loadLanguageExtensions() {
  const defs = await ListLanguageExtensions().catch(() => [])
  languageExtensions.set(defs)
  byExt = new Map()
  for (const d of defs) for (const ext of d.extensions ?? []) byExt.set(ext, d)
}

function extOf(path: string): string {
  const i = path.lastIndexOf('.')
  return i < 0 ? '' : path.slice(i).toLowerCase()
}

// The single ext→Definition lookup every other module should use instead of
// its own hardcoded switch.
export function defFor(path: string): LanguageExtensionDTO | null {
  return byExt.get(extOf(path)) ?? null
}

export function languageIdFor(path: string): string {
  const def = defFor(path)
  if (!def) return 'plaintext'
  return def.languageIds?.[extOf(path)] ?? def.id
}

// A language's optional frontend-only module — every export is optional, a
// language only ships what it needs beyond the Go-side defaults.
export interface LanguageModule {
  // Zero-install formatter (json/yaml/sql/xml/html/markdown/csv/toml) —
  // present iff the Definition has builtinFormatter: true.
  formatter?: (view: EditorView) => void | Promise<void>
  // CodeMirror grammar for a language not already statically imported in
  // FileViewer.svelte's langFor() — only needed for genuinely new languages.
  grammar?: () => any
  // Custom settings UI for the Languages panel; falls back to
  // DefaultLanguageSettings.svelte when absent. Gets the full DTO (not just
  // id) so a custom Settings component can still embed
  // DefaultLanguageSettings for the fields it doesn't want to reimplement.
  Settings?: Component<{ id: string; def: LanguageExtensionDTO }>
}

const cache = new Map<string, Promise<LanguageModule>>()

// Dynamic-imports frontend/src/lib/languages/<id>.ts. Not every language has
// one — a missing module is not an error, just "nothing extra to load."
export function loadLanguage(id: string): Promise<LanguageModule> {
  let p = cache.get(id)
  if (p) return p
  p = import(`./languages/${id}.ts`).catch(() => ({})) as Promise<LanguageModule>
  cache.set(id, p)
  return p
}

export function overrideFor(id: string): LanguageExtensionDTO['override'] {
  return get(languageExtensions).find(d => d.id === id)?.override ?? {}
}
