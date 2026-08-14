import { writable } from 'svelte/store'

// path → diagnostics for that file, aggregated across every LSP client.
// Independent of open editors: the Problems panel needs to show issues in
// files you haven't opened, not just the ones with a mounted gutter.
export interface DiagnosticEntry {
  line: number   // 1-based, matches pendingGoto.line
  col: number    // 0-based char offset, matches pendingGoto.col
  severity: 'error' | 'warning' | 'info' | 'hint'
  message: string
}

export const diagnostics = writable<Map<string, DiagnosticEntry[]>>(new Map())

function uriToPath(uri: string): string {
  return decodeURIComponent(uri.replace(/^file:\/\//, ''))
}

function toSeverity(sev: number | undefined): DiagnosticEntry['severity'] {
  return sev === 1 ? 'error' : sev === 2 ? 'warning' : sev === 3 ? 'info' : 'hint'
}

export function recordDiagnostics(uri: string, items: any[]) {
  const path = uriToPath(uri)
  diagnostics.update(m => {
    const next = new Map(m)
    if (!items.length) next.delete(path)
    else next.set(path, items.map(item => ({
      line: item.range.start.line + 1,
      col: item.range.start.character,
      severity: toSeverity(item.severity),
      message: item.message as string,
    })))
    return next
  })
}

export function clearDiagnostics() {
  diagnostics.set(new Map())
}
