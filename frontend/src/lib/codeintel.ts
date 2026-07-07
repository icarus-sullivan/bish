import type { Extension } from '@codemirror/state'
import { LanguageSupport } from '@codemirror/language'
import { autoImportSource } from './autoimport'

export type IntelKind = 'go' | 'js' | 'py'

export function intelKindFor(path: string): IntelKind | null {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (ext === 'go') return 'go'
  if (['js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx'].includes(ext)) return 'js'
  if (ext === 'py') return 'py'
  return null
}

// Single seam for editor intelligence. v1 wires the heuristic project-symbol
// auto-import; a future LSP integration (@codemirror/lsp-client over a Wails
// transport) swaps its extensions in here without touching FileViewer.
export function codeIntel(filePath: string, root: string, lang: unknown, kind: IntelKind | null): Extension[] {
  if (!kind || !root || !(lang instanceof LanguageSupport)) return []
  return [lang.language.data.of({ autocomplete: autoImportSource(filePath, root, kind) })]
}
