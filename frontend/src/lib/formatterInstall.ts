// Formatter-specific install flow — mirrors installServer() in lsp.ts, but
// targets a language's dedicated formatter (internal/langext.FormatterManager)
// instead of its intelligence server. Kept separate because the two are
// genuinely independent installs (see formatDocument()'s router in
// formatters.ts): a language can be missing its formatter while its server
// is installed and working fine, or vice versa.
import { writable } from 'svelte/store'
import { FormatterInstall, on } from './wails'
import { toast } from './toast'
import { loadLanguageExtensions } from './languageExtensions'

export type FormatterStatus =
  | { status: 'installing'; output: string[] }
  | { status: 'error'; message: string }
export const formatterStatus = writable<Partial<Record<string, FormatterStatus>>>({})

function toastId(id: string) {
  return `formatter-install-${id}`
}

export async function installFormatter(id: string, label: string) {
  const tid = toastId(id)
  formatterStatus.update(s => ({ ...s, [id]: { status: 'installing', output: [] } }))
  toast.loading(`Installing ${label} formatter…`, { id: tid, duration: Infinity })
  const off = on('langext:formatter-install-output:' + id, (line: string) => {
    formatterStatus.update(s => {
      const cur = s[id]
      if (!cur || cur.status !== 'installing') return s
      return { ...s, [id]: { status: 'installing', output: [...cur.output, line] } }
    })
    toast.loading(`Installing ${label} formatter…`, { id: tid, description: line, duration: Infinity })
  })
  try {
    await FormatterInstall(id)
    off()
    formatterStatus.update(s => {
      const next = { ...s }
      delete next[id]
      return next
    })
    toast.success(`${label} formatter installed`, { id: tid, description: undefined, duration: 3000 })
    await loadLanguageExtensions() // refresh install-status pills
  } catch (err: any) {
    off()
    const message = String(err?.message ?? err)
    formatterStatus.update(s => ({ ...s, [id]: { status: 'error', message } }))
    toast.error('Install failed', {
      id: tid, description: message, duration: Infinity,
      action: { label: 'Retry', onClick: () => installFormatter(id, label) },
    })
  }
}
