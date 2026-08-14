// Extension host: each enabled extension's entry script runs in its own Web
// Worker — a real browser sandbox (no DOM, no filesystem/network access)
// rather than a hand-rolled JS VM. The only API surface is postMessage:
// run a contributed command, push sanitized HTML for a contributed panel,
// or ask for the active file path. Commands/panels themselves are declared
// statically in the manifest (see internal/extensions), so they're known —
// and show up in the Command Palette — before the worker even starts.
import { writable, get } from 'svelte/store'
import DOMPurify from 'dompurify'
import { GetExtensions, SetExtensionEnabled } from './wails'
import type { Extension } from './wails'
import { registerCommand } from './commands'
import { tabs, activeTabId } from './stores'

export const loadedExtensions = writable<Extension[]>([])
// `${extensionName}:${panelId}` -> sanitized HTML
export const extensionPanelHTML = writable<Map<string, string>>(new Map())

const workers = new Map<string, Worker>()
const unregisterFns = new Map<string, (() => void)[]>()

function activeFilePath(): string {
  const t = get(tabs).find(t => t.id === get(activeTabId))
  return t && t.type === 'file' && t.path && t.path !== '__new__' ? t.path : ''
}

function startWorker(ext: Extension) {
  if (workers.has(ext.name)) return
  let worker: Worker
  try {
    const blob = new Blob([ext.script], { type: 'application/javascript' })
    worker = new Worker(URL.createObjectURL(blob))
  } catch {
    return
  }
  workers.set(ext.name, worker)

  worker.onmessage = (e: MessageEvent) => {
    const msg = e.data
    if (!msg || typeof msg !== 'object') return
    if (msg.type === 'setPanelHTML' && typeof msg.panelId === 'string') {
      extensionPanelHTML.update(m => {
        const next = new Map(m)
        next.set(`${ext.name}:${msg.panelId}`, DOMPurify.sanitize(String(msg.html ?? '')))
        return next
      })
    } else if (msg.type === 'getActiveFilePath' && msg.reqId != null) {
      worker.postMessage({ type: 'reply', reqId: msg.reqId, value: activeFilePath() })
    }
  }

  const offs = unregisterFns.get(ext.name) ?? []
  for (const c of ext.commands ?? []) {
    offs.push(registerCommand({
      id: `ext.${ext.name}.${c.id}`,
      title: c.title,
      run: () => worker.postMessage({ type: 'command', id: c.id }),
    }))
  }
  unregisterFns.set(ext.name, offs)
}

function stopWorker(name: string) {
  workers.get(name)?.terminate()
  workers.delete(name)
  for (const off of unregisterFns.get(name) ?? []) off()
  unregisterFns.delete(name)
  extensionPanelHTML.update(m => {
    const next = new Map(m)
    for (const k of [...next.keys()]) if (k.startsWith(name + ':')) next.delete(k)
    return next
  })
}

export async function loadExtensions() {
  const list = (await GetExtensions().catch(() => [])) as Extension[]
  loadedExtensions.set(list)
  for (const ext of list) if (ext.enabled) startWorker(ext)
}

export function setExtensionEnabled(name: string, enabled: boolean) {
  loadedExtensions.update(list => list.map(e => e.name === name ? { ...e, enabled } : e))
  SetExtensionEnabled(name, enabled).catch(() => {})
  if (enabled) {
    const ext = get(loadedExtensions).find(e => e.name === name)
    if (ext) startWorker(ext)
  } else {
    stopWorker(name)
  }
}
