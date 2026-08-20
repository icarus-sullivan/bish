// Format-document router. Single entry point FileViewer calls for both
// "Format Document" and format-on-save — routes through, in order:
//   1. a builtin zero-install formatter (json/yaml/sql/xml/html/markdown/
//      csv/toml — see frontend/src/lib/languages/<id>.ts)
//   2. a dedicated external formatter (ruff, prettier, rustfmt, ... — see
//      internal/langext) if one is declared and installed
//   3. the attached LSP server's documentFormattingProvider, same as before
//   4. if a dedicated formatter is declared but not installed, prompt to
//      install it specifically (not the intelligence server) — this is the
//      fix for servers like pyright that never implement formatting at all
import type { EditorView } from '@codemirror/view'
import { lspFormat } from './lsp'
import { defFor, loadLanguage } from './languageExtensions'
import { FormatWithExtension } from './wails'
import { installFormatter } from './formatterInstall'
import { replaceAll } from './formatUtil'

export async function formatDocument(view: EditorView, path: string): Promise<void> {
  const def = defFor(path)
  if (!def) {
    await lspFormat(view)
    return
  }

  if (def.builtinFormatter) {
    const mod = await loadLanguage(def.id)
    if (mod.formatter) {
      await mod.formatter(view)
      return
    }
  }

  if (def.formatter) {
    if (def.formatterInstalled) {
      const text = await FormatWithExtension(def.id, path, view.state.doc.toString())
      replaceAll(view, text)
      return
    }
    try {
      await lspFormat(view)
      return
    } catch {
      installFormatter(def.id, def.name)
      throw new Error(`No formatter installed for ${def.name} — install started`)
    }
  }

  await lspFormat(view)
}
