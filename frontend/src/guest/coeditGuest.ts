// Entry point bundled (via esbuild, see frontend/package.json's
// build:guest script) into internal/liveshare/assets/coedit.js — the guest
// side of Live Share editor co-editing (PM_ASKS.md #12.b). Runs in a plain
// browser with no bish install, same posture as phase 1's vendored xterm.js
// guest page. Deliberately plain-text only (no language modes/highlighting)
// — this is a pairing view, not a full editor.
import * as Y from 'yjs'
import { EditorState, Compartment } from '@codemirror/state'
import { EditorView, basicSetup } from 'codemirror'
import { yCollab } from 'y-codemirror.next'
import { RelayProvider } from '../lib/yjsRelay'

interface MountOpts {
  onStatus: (text: string, disconnected?: boolean) => void
}

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
