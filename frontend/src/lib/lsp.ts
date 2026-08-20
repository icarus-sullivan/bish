// LSP client over the Wails transport. The Go side (internal/lsp) only
// spawns servers and frames stdio; @codemirror/lsp-client does the protocol.
// Editors mount instantly on the v1 autoimport fallback and upgrade in place
// (Compartment reconfigure) once the server is up.
import { Compartment, type Extension } from '@codemirror/state'
import { ViewPlugin, EditorView } from '@codemirror/view'
import {
  LSPClient, LSPPlugin, jumpToDefinition, languageServerSupport, serverDiagnostics, type Transport,
} from '@codemirror/lsp-client'
import { getIndentUnit, indentUnit } from '@codemirror/language'
import { writable, get } from 'svelte/store'
import { LSPStart, LSPSend, LSPStop, LSPInstalled, LSPInstall, on } from './wails'
import { openFileTab } from './stores'
import type { IntelKind } from './codeintel'
import { languageIdFor, loadLanguageExtensions } from './languageExtensions'
import { recordDiagnostics, clearDiagnostics } from './diagnostics'
import { toast } from './toast'

const IDLE_SHUTDOWN_MS = 5 * 60_000

function wailsTransport(lang: IntelKind): Transport {
  const handlers = new Set<(v: string) => void>()
  on('lsp:msg:' + lang, (msg: string) => handlers.forEach(h => h(msg)))
  return {
    send: (msg: string) => { LSPSend(lang, msg).catch(() => {}) },
    subscribe: h => { handlers.add(h) },
    unsubscribe: h => { handlers.delete(h) },
  }
}

// path→view registry so displayFile can await a freshly opened tab's editor
const viewsByPath = new Map<string, EditorView>()
export function registerEditorView(path: string, view: EditorView) {
  viewsByPath.set(path, view)
}
export function unregisterEditorView(path: string, view: EditorView) {
  if (viewsByPath.get(path) === view) viewsByPath.delete(path)
}

function uriToPath(uri: string): string {
  return decodeURIComponent(uri.replace(/^file:\/\//, ''))
}
function pathToUri(path: string): string {
  return 'file://' + encodeURI(path)
}

// Cross-file jump: open the tab, then wait for its FileViewer to mount.
async function displayFileByTab(uri: string): Promise<EditorView | null> {
  const path = uriToPath(uri)
  openFileTab(path, true)
  const deadline = Date.now() + 2000
  while (Date.now() < deadline) {
    const view = viewsByPath.get(path)
    if (view) return view
    await new Promise(r => setTimeout(r, 50))
  }
  return null // ponytail: silent no-op beats a hang
}

interface Entry {
  client: LSPClient
  root: string
  attached: number
  idleTimer?: ReturnType<typeof setTimeout>
}
const clients = new Map<IntelKind, Entry>()

// live editors, so a crashed server degrades them back to the fallback, and
// a just-installed server can upgrade them back up (see retryAttach)
interface Attachment {
  kind: IntelKind
  view: EditorView
  comp: Compartment
  fallback: Extension
  path: string
  root: string
  held: Entry | null
  dead: boolean
}
const attachments = new Set<Attachment>()

// One server per (language, project) install can be missing/broken; per-kind
// status the UI reads to offer an install prompt. Absent = fine/unknown.
export type ServerStatus =
  | { status: 'missing' }
  | { status: 'installing'; output: string[] }
  | { status: 'error'; message: string }
export const serverStatus = writable<Partial<Record<IntelKind, ServerStatus>>>({})

function dropClient(lang: IntelKind) {
  const e = clients.get(lang)
  if (!e) return
  clearTimeout(e.idleTimer)
  clients.delete(lang)
  try { e.client.disconnect() } catch { /* transport already gone */ }
  for (const a of attachments) {
    if (a.kind === lang) {
      a.held = null
      a.view.dispatch({ effects: a.comp.reconfigure(a.fallback) })
    }
  }
}

function applyEntry(att: Attachment, entry: Entry) {
  att.held = entry
  retain(att.kind, entry)
  registerEditorView(att.path, att.view)
  att.view.dispatch({
    effects: att.comp.reconfigure(
      languageServerSupport(entry.client, pathToUri(att.path), languageIdFor(att.path))),
  })
}

function tryAttach(att: Attachment) {
  if (att.held || att.dead) return
  ensureClient(att.kind, att.root).then(entry => {
    if (!entry || att.dead || att.held) return
    applyEntry(att, entry)
  })
}

// Re-attempt attaching every editor still on the fallback for `kind` —
// called once a server install finishes so open tabs upgrade without a
// reload. No-op for editors already on full LSP support.
export function retryAttach(kind: IntelKind) {
  for (const att of attachments) if (att.kind === kind) tryAttach(att)
}

// Ask the backend to install `kind`'s language server (go install / pnpm add
// -g, see internal/lsp), streaming its output into serverStatus for the UI,
// then upgrades any editors that were waiting on it.
function installToastId(kind: IntelKind) {
  return `lsp-install-${kind}`
}

export async function installServer(kind: IntelKind) {
  const id = installToastId(kind)
  serverStatus.update(s => ({ ...s, [kind]: { status: 'installing', output: [] } }))
  toast.loading(`Installing ${kind} language server…`, { id, duration: Infinity })
  const off = on('lsp:install-output:' + kind, (line: string) => {
    serverStatus.update(s => {
      const cur = s[kind]
      if (!cur || cur.status !== 'installing') return s
      return { ...s, [kind]: { status: 'installing', output: [...cur.output, line] } }
    })
    toast.loading(`Installing ${kind} language server…`, { id, description: line, duration: Infinity })
  })
  try {
    await LSPInstall(kind)
    off()
    serverStatus.update(s => {
      const next = { ...s }
      delete next[kind]
      return next
    })
    toast.success(`${kind} language server installed`, { id, description: undefined, duration: 3000 })
    retryAttach(kind)
    loadLanguageExtensions() // refresh the Languages panel's install-status pill
  } catch (err: any) {
    off()
    const message = String(err?.message ?? err)
    serverStatus.update(s => ({ ...s, [kind]: { status: 'error', message } }))
    toast.error(`Install failed`, {
      id, description: message, duration: Infinity,
      action: { label: 'Retry', onClick: () => installServer(kind) },
    })
  }
}

on('project:change', () => {
  // backend already killed the servers; drop stale protocol state
  for (const lang of [...clients.keys()]) dropClient(lang)
  clearDiagnostics()
})

// lsp:down:<lang> listeners used to be wired for a hardcoded 4-language
// list at module load; languages are a runtime-fetched registry now, so
// each is wired lazily, once, the first time ensureClient actually needs it.
const downWired = new Set<string>()
function wireDown(lang: IntelKind) {
  if (downWired.has(lang)) return
  downWired.add(lang)
  on('lsp:down:' + lang, () => dropClient(lang))
}

async function ensureClient(lang: IntelKind, root: string): Promise<Entry | null> {
  wireDown(lang)
  const existing = clients.get(lang)
  if (existing && existing.root === root) return existing
  if (existing) dropClient(lang)
  const ok = await LSPStart(lang, root).catch(() => false)
  if (!ok) {
    // don't clobber an install in progress (or its result) with 'missing'
    if (get(serverStatus)[lang]?.status !== 'installing') {
      const installed = await LSPInstalled(lang).catch(() => true)
      if (!installed && get(serverStatus)[lang]?.status !== 'missing') {
        serverStatus.update(s => ({ ...s, [lang]: { status: 'missing' } }))
        toast(`No ${lang} language server found`, {
          id: installToastId(lang), duration: Infinity,
          action: { label: 'Install', onClick: () => installServer(lang) },
        })
      }
    }
    return null
  }
  toast.dismiss(installToastId(lang))
  serverStatus.update(s => {
    if (!(lang in s)) return s
    const next = { ...s }
    delete next[lang]
    return next
  })
  // races between two editors opening at once: second await wins the check above
  const again = clients.get(lang)
  if (again && again.root === root) return again
  const client = new LSPClient({
    rootUri: pathToUri(root),
    extensions: [serverDiagnostics()],
    notificationHandlers: {
      // fires before serverDiagnostics()'s own handler (extensions are tried
      // after top-level config handlers) — always record for the Problems
      // panel, then fall through (return false) so the open editor's gutter
      // still gets updated as before.
      'textDocument/publishDiagnostics': (_client, params) => {
        recordDiagnostics(params.uri, params.diagnostics)
        return false
      },
    },
  })
  const origDisplay = client.workspace.displayFile.bind(client.workspace)
  client.workspace.displayFile = async (uri: string) =>
    (await origDisplay(uri)) ?? displayFileByTab(uri)
  client.connect(wailsTransport(lang))
  const entry: Entry = { client, root, attached: 0 }
  clients.set(lang, entry)
  return entry
}

function retain(lang: IntelKind, entry: Entry) {
  entry.attached++
  clearTimeout(entry.idleTimer)
}
function release(lang: IntelKind, entry: Entry) {
  entry.attached--
  if (entry.attached > 0) return
  clearTimeout(entry.idleTimer)
  entry.idleTimer = setTimeout(() => {
    if (entry.attached === 0 && clients.get(lang) === entry) {
      dropClient(lang)
      LSPStop(lang).catch(() => {})
    }
  }, IDLE_SHUTDOWN_MS)
}

// Awaitable document formatting. The lib's formatDocument command applies
// edits async with no completion signal, which would race format-then-write
// on save. No LSP attached (server not installed, or still connecting) →
// throws rather than silently no-opping, so callers can surface *why*
// nothing happened instead of leaving the user staring at an unformatted file.
export async function lspFormat(view: EditorView): Promise<void> {
  const plugin = LSPPlugin.get(view)
  if (!plugin) throw new Error('No language server attached for this file')
  // Not every server implements formatting (pyright notably doesn't — it's a
  // type checker, not a formatter). Asking anyway gets back a generic JSON-RPC
  // "Unhandled method" error; check the advertised capability first so the
  // failure at least says why.
  if (!plugin.client.serverCapabilities?.documentFormattingProvider) {
    throw new Error('This language server does not support formatting')
  }
  plugin.client.sync()
  let edits: any[] | null
  try {
    edits = await (plugin.client as any).request('textDocument/formatting', {
      textDocument: { uri: plugin.uri },
      options: {
        tabSize: getIndentUnit(view.state),
        insertSpaces: view.state.facet(indentUnit).indexOf('\t') < 0,
      },
    })
  } catch (err: any) {
    throw new Error(`Format request failed: ${err?.message ?? err}`)
  }
  if (!edits?.length) return
  view.dispatch({
    changes: edits.map(e => ({
      from: plugin.fromPosition(e.range.start),
      to: plugin.fromPosition(e.range.end),
      insert: e.newText,
    })),
  })
}

// Cmd/Ctrl+click jumps to the definition of the symbol under the pointer
// (imports resolve to the target file). Single event handler, no hover
// tracking, so it costs nothing until a modifier-click actually happens.
// No-ops silently when the LSP isn't attached yet (fallback mode).
const modClickToDefinition = EditorView.domEventHandlers({
  mousedown(event, view) {
    if (!(event.metaKey || event.ctrlKey) || event.button !== 0) return false
    const pos = view.posAtCoords({ x: event.clientX, y: event.clientY })
    if (pos == null) return false
    event.preventDefault()
    view.dispatch({ selection: { anchor: pos } })
    jumpToDefinition(view)
    return true
  },
})

// Returns an extension that starts as `fallback` (v1 autoimport) and swaps
// itself for full LSP support once the server for `lang` is connected.
export function lspOrFallback(path: string, root: string, kind: IntelKind, fallback: Extension): Extension {
  const comp = new Compartment()
  const attach = ViewPlugin.define(view => {
    const att: Attachment = { kind, view, comp, fallback, path, root, held: null, dead: false }
    attachments.add(att)
    tryAttach(att)
    return {
      destroy() {
        att.dead = true
        attachments.delete(att)
        unregisterEditorView(path, view)
        if (att.held) release(kind, att.held)
      },
    }
  })
  return [comp.of(fallback), attach, modClickToDefinition]
}
