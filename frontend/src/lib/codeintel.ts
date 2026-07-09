import type { Extension } from '@codemirror/state'
import { LanguageSupport } from '@codemirror/language'
import { autoImportSource } from './autoimport'
import { lspOrFallback } from './lsp'

export type IntelKind = 'go' | 'js' | 'py' | 'svelte'

export function intelKindFor(path: string): IntelKind | null {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (ext === 'go') return 'go'
  if (['js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx'].includes(ext)) return 'js'
  if (ext === 'py') return 'py'
  if (ext === 'svelte') return 'svelte' // own kind: the js server must not didOpen .svelte
  return null
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
