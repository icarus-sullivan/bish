// Host-side session manager for Live Share editor co-editing (phase 2,
// PM_ASKS.md #12.b). One Y.Doc per shared file, relayed through Go's
// edit-share WebSocket (see internal/liveshare) via yjsRelay's transport-
// agnostic provider. FileViewer.svelte splices coEditExtension(path) into
// its CodeMirror instance (via a Compartment) whenever editSharePath
// matches the file it's showing.
import * as Y from 'yjs'
import { yCollab } from 'y-codemirror.next'
import type { Extension } from '@codemirror/state'
import { writable, get } from 'svelte/store'
import { RelayProvider } from './yjsRelay'
import { StartEditShare, StopEditShare, GetEditShareGuests, EditShareBroadcast, on } from './wails'
import type { LiveShareGuest } from './wails'

interface CoEditSession { doc: Y.Doc; ytext: Y.Text; provider: RelayProvider }

const sessions = new Map<string, CoEditSession>() // path -> session
export const editSharePath = writable<string | null>(null)
// path -> connected guests, pushed live over 'editshare:guests'
export const editShareGuests = writable<Map<string, LiveShareGuest[]>>(new Map())

let listening = false
function ensureListener() {
  if (listening) return
  listening = true
  on('editshare:data', (payload: { path: string; data: string }) => {
    sessions.get(payload.path)?.provider.receive(base64ToBytes(payload.data))
  })
  on('editshare:guests', (update: { path: string; guests: LiveShareGuest[] }) => {
    editShareGuests.update(m => {
      if (!sessions.has(update.path)) return m
      const next = new Map(m)
      next.set(update.path, update.guests)
      return next
    })
  })
}

function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}
function bytesToBase64(bytes: Uint8Array): string {
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin)
}

// Starts (or returns the existing) share for path, seeding the Y.Text from
// the file's current buffer content the first time. Returns the guest link.
export async function startCoEdit(path: string, initialContent: string): Promise<string> {
  ensureListener()
  if (!sessions.has(path)) {
    const doc = new Y.Doc()
    const ytext = doc.getText('content')
    if (initialContent) ytext.insert(0, initialContent)
    const provider = new RelayProvider(doc, (data) => {
      EditShareBroadcast(path, bytesToBase64(data)).catch(() => {})
    })
    provider.setLocalUser('Host', '#cba6f7')
    sessions.set(path, { doc, ytext, provider })
  }
  const url = await StartEditShare(path)
  const guests = (await GetEditShareGuests(path).catch(() => [])) ?? []
  editShareGuests.update(m => new Map(m).set(path, guests))
  editSharePath.set(path)
  return url
}

export function stopCoEdit(path: string) {
  sessions.get(path)?.provider.destroy()
  sessions.delete(path)
  StopEditShare(path).catch(() => {})
  editShareGuests.update(m => { const next = new Map(m); next.delete(path); return next })
  if (get(editSharePath) === path) editSharePath.set(null)
}

export function isCoEditing(path: string): boolean {
  return sessions.has(path)
}

export function coEditExtension(path: string): Extension | undefined {
  const s = sessions.get(path)
  return s ? yCollab(s.ytext, s.provider.awareness) : undefined
}
