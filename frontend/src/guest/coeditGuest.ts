// Entry point bundled (via esbuild, see frontend/package.json's
// build:guest script) into internal/liveshare/assets/coedit.js — the guest
// side of Live Share editor co-editing (PM_ASKS.md #12.b). Runs in a plain
// browser with no bish install, same posture as phase 1's vendored xterm.js
// guest page. Language detection + highlight palette are shared with the
// host editor (frontend/src/lib/codeLang.ts) so a shared file looks the
// same here as it does in bish itself.
import * as Y from 'yjs'
import { EditorState, Compartment } from '@codemirror/state'
import { EditorView, basicSetup } from 'codemirror'
import { yCollab } from 'y-codemirror.next'
import { RelayProvider } from '../lib/yjsRelay'
import { langFor, highlightFor } from '../lib/codeLang'

interface MountOpts {
  path?: string
  onStatus: (text: string, disconnected?: boolean) => void
}

// matches guest.html's terminal theme (Catppuccin Mocha) — this static page
// can't read the host's live theme, so it hardcodes the same fallback.
const baseTheme = EditorView.theme({
  '&': { height: '100%', fontSize: '13px', backgroundColor: '#1e1e2e', color: '#cdd6f4' },
  '.cm-scroller': { fontFamily: '"SF Mono", Menlo, Monaco, "Courier New", monospace', lineHeight: '1.6' },
  '.cm-content': { caretColor: '#cba6f7' },
  '.cm-focused': { outline: 'none' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#cba6f7', borderLeftWidth: '2px' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': { background: '#313244 !important' },
  '.cm-activeLine': { backgroundColor: '#26263a' },
  '.cm-activeLineGutter': { backgroundColor: '#26263a' },
  '.cm-gutters': { background: '#181825', color: '#6c7086', border: 'none' },
}, { dark: true })

function mount(container: HTMLElement, wsUrl: string, opts: MountOpts) {
  const doc = new Y.Doc()
  const ytext = doc.getText('content')

  const ws = new WebSocket(wsUrl)
  ws.binaryType = 'arraybuffer'

  const provider = new RelayProvider(doc, (data) => {
    if (ws.readyState === WebSocket.OPEN) ws.send(data as BufferSource)
  })
  provider.setLocalUser('Guest', '#f5c2e7')

  const editableCompartment = new Compartment()
  let canType = false

  const view = new EditorView({
    state: EditorState.create({
      doc: '',
      extensions: [
        basicSetup,
        baseTheme,
        highlightFor('catppuccin'),
        opts.path ? langFor(opts.path) : [],
        editableCompartment.of(EditorView.editable.of(false)),
        yCollab(ytext, provider.awareness),
      ],
    }),
    parent: container,
  })

  ws.onopen = () => {
    opts.onStatus('Connected — read-only')
    provider.requestSync()
  }
  ws.onclose = () => opts.onStatus('Disconnected', true)
  ws.onerror = () => opts.onStatus('Connection error', true)

  ws.onmessage = (ev) => {
    if (typeof ev.data === 'string') {
      let msg: any
      try { msg = JSON.parse(ev.data) } catch { return }
      if (msg.type === 'permission') {
        canType = !!msg.canType
        view.dispatch({ effects: editableCompartment.reconfigure(EditorView.editable.of(canType)) })
        opts.onStatus('Connected — ' + (canType ? 'you can type' : 'read-only'))
      }
      return
    }
    provider.receive(new Uint8Array(ev.data))
  }
}

;(window as any).BishCoedit = { mount }
