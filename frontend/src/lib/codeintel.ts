import type { Extension } from '@codemirror/state'
import { LanguageSupport } from '@codemirror/language'
import { autoImportSource } from './autoimport'
import { lspOrFallback } from './lsp'
import { defFor } from './languageExtensions'

// Was a 4-value union (go/js/py/svelte); widened to string now that
// languages are a registry (internal/langext) rather than a hardcoded list.
// Callers that switch on specific literals (snippets.ts, autoimport.ts) are
// Record-lookup based already, so this loosening is source-compatible.
export type IntelKind = string

// A language only has an IntelKind (i.e. gets an LSP attachment) if its
// Definition declares a server — data/markup-only languages (json, xml, ...)
// return null here same as an unrecognized extension.
export function intelKindFor(path: string): IntelKind | null {
  const def = defFor(path)
  return def?.server ? def.id : null
}

// Single seam for editor intelligence. Editors mount with the heuristic
// project-symbol auto-import and upgrade in place to full LSP support
// (@codemirror/lsp-client over the Wails transport) when a server is
// installed; no server → the fallback simply stays.
export function codeIntel(filePath: string, root: string, lang: unknown, kind: IntelKind | null): Extension[] {
  if (!kind || !root || !(lang instanceof LanguageSupport)) return []
  const fallback = lang.language.data.of({ autocomplete: autoImportSource(filePath, root, kind) })
  return [lspOrFallback(filePath, root, kind, fallback)]
}
