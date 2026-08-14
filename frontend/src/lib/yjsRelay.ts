// Transport-agnostic Yjs sync provider for Live Share's editor co-editing
// (phase 2, PM_ASKS.md #12.b). bish's Go backend is a dumb relay — it
// broadcasts opaque binary blobs between the host and connected guests
// (see internal/liveshare's edit-session path) without understanding Yjs at
// all. This class implements the wire protocol on both ends of that relay:
// the host frontend uses it directly, and the guest's vendored bundle
// (esbuild output embedded as internal/liveshare/assets/coedit.js) imports
// this exact same module so both sides speak identical bytes.
//
// ponytail: star topology only — every message is broadcast to every
// connected peer (never point-to-point), which is wasteful with 3+ guests
// but correct (Yjs updates are idempotent) and is exactly what a pairing
// feature needs for the realistic 1-host+1-guest case. Add point-to-point
// sync-step replies if this grows multi-guest usage.
import * as Y from 'yjs'
import * as syncProtocol from 'y-protocols/sync'
import * as awarenessProtocol from 'y-protocols/awareness'
import * as encoding from 'lib0/encoding'
import * as decoding from 'lib0/decoding'

const MSG_SYNC = 0
const MSG_AWARENESS = 1

export class RelayProvider {
  doc: Y.Doc
  awareness: awarenessProtocol.Awareness
  private send: (data: Uint8Array) => void

  constructor(doc: Y.Doc, send: (data: Uint8Array) => void) {
    this.doc = doc
    this.send = send
    this.awareness = new awarenessProtocol.Awareness(doc)

    // any local or remote-applied change gets rebroadcast to every peer —
    // simplest correct hub behavior, see file doc comment
    doc.on('update', (update: Uint8Array) => {
      const enc = encoding.createEncoder()
      encoding.writeVarUint(enc, MSG_SYNC)
      syncProtocol.writeUpdate(enc, update)
      this.send(encoding.toUint8Array(enc))
    })

    this.awareness.on('update', ({ added, updated, removed }: { added: number[]; updated: number[]; removed: number[] }) => {
      const changed = added.concat(updated, removed)
      const enc = encoding.createEncoder()
      encoding.writeVarUint(enc, MSG_AWARENESS)
      encoding.writeVarUint8Array(enc, awarenessProtocol.encodeAwarenessUpdate(this.awareness, changed))
      this.send(encoding.toUint8Array(enc))
    })
  }

  setLocalUser(name: string, color: string) {
    this.awareness.setLocalStateField('user', { name, color })
  }

  // Kick off sync by asking the other side for whatever this doc is missing
  // (an empty state vector on first join = "send me everything").
  requestSync() {
    const enc = encoding.createEncoder()
    encoding.writeVarUint(enc, MSG_SYNC)
    syncProtocol.writeSyncStep1(enc, this.doc)
    this.send(encoding.toUint8Array(enc))
  }

  receive(data: Uint8Array) {
    const dec = decoding.createDecoder(data)
    const type = decoding.readVarUint(dec)
    if (type === MSG_SYNC) {
      const enc = encoding.createEncoder()
      encoding.writeVarUint(enc, MSG_SYNC)
      syncProtocol.readSyncMessage(dec, enc, this.doc, this)
      if (encoding.length(enc) > 1) this.send(encoding.toUint8Array(enc)) // non-empty reply (e.g. step2)
    } else if (type === MSG_AWARENESS) {
      awarenessProtocol.applyAwarenessUpdate(this.awareness, decoding.readVarUint8Array(dec), this)
    }
  }

  destroy() {
    awarenessProtocol.removeAwarenessStates(this.awareness, [this.doc.clientID], 'destroy')
  }
}
