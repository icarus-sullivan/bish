// LSP client over the Wails transport. The Go side (internal/lsp) only
// spawns servers and frames stdio; @codemirror/lsp-client does the protocol.
// Editors mount instantly on the v1 autoimport fallback and upgrade in place
// (Compartment reconfigure) once the server is up.
import { Compartment, type Extension } from '@codemirror/state'
import { ViewPlugin, EditorView } from '@codemirror/view'
import {
  LSPClient, languageServerSupport, serverDiagnostics, type Transport,
} from '@codemirror/lsp-client'
import { LSPStart, LSPSend, LSPStop, on } from './wails'
import { openFileTab } from './stores'
import type { IntelKind } from './codeintel'

const IDLE_SHUTDOWN_MS = 5 * 60_000

function languageIdFor(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  switch (ext) {
    case 'go': return 'go'
    case 'py': return 'python'
    case 'ts': return 'typescript'
    case 'tsx': return 'typescriptreact'
    case 'jsx': return 'javascriptreact'
    default: return 'javascript'
  }
}

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

// live editors, so a crashed server degrades them back to the fallback
interface Attachment {
  kind: IntelKind
  view: EditorView
  comp: Compartment
  fallback: Extension
}
const attachments = new Set<Attachment>()

function dropClient(lang: IntelKind) {
  const e = clients.get(lang)
  if (!e) return
  clearTimeout(e.idleTimer)
  clients.delete(lang)
  try { e.client.disconnect() } catch { /* transport already gone */ }
  for (const a of attachments) {
    if (a.kind === lang) {
      a.view.dispatch({ effects: a.comp.reconfigure(a.fallback) })
    }
  }
}

for (const lang of ['go', 'js', 'py'] as IntelKind[]) {
  on('lsp:down:' + lang, () => dropClient(lang))
}
on('project:change', () => {
  // backend already killed the servers; drop stale protocol state
  for (const lang of [...clients.keys()]) dropClient(lang)
})

async function ensureClient(lang: IntelKind, root: string): Promise<Entry | null> {
  const existing = clients.get(lang)
  if (existing && existing.root === root) return existing
  if (existing) dropClient(lang)
  const ok = await LSPStart(lang, root).catch(() => false)
  if (!ok) return null
  // races between two editors opening at once: second await wins the check above
  const again = clients.get(lang)
  if (again && again.root === root) return again
  const client = new LSPClient({
    rootUri: pathToUri(root),
    extensions: [serverDiagnostics()],
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

// Returns an extension that starts as `fallback` (v1 autoimport) and swaps
// itself for full LSP support once the server for `lang` is connected.
export function lspOrFallback(path: string, root: string, kind: IntelKind, fallback: Extension): Extension {
  const comp = new Compartment()
  const attach = ViewPlugin.define(view => {
    let dead = false
    let held: Entry | null = null
    const att: Attachment = { kind, view, comp, fallback }
    ensureClient(kind, root).then(entry => {
      if (!entry || dead) return
      held = entry
      retain(kind, entry)
      registerEditorView(path, view)
      attachments.add(att)
      view.dispatch({
        effects: comp.reconfigure(
          languageServerSupport(entry.client, pathToUri(path), languageIdFor(path))),
      })
    })
    return {
      destroy() {
        dead = true
        attachments.delete(att)
        unregisterEditorView(path, view)
        if (held) release(kind, held)
      },
    }
  })
  return [comp.of(fallback), attach]
}
