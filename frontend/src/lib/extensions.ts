// Extension host: each enabled extension's entry script runs in its own Web
// Worker — a real browser sandbox (no DOM, no filesystem/network access)
// rather than a hand-rolled JS VM. The only API surface is postMessage:
// run a contributed command, push sanitized HTML for a contributed panel,
// or ask for the active file path. Commands/panels themselves are declared
// statically in the manifest (see internal/extensions), so they're known —
// and show up in the Command Palette — before the worker even starts.
import { writable, get } from 'svelte/store'
import DOMPurify from 'dompurify'
import { GetExtensions, SetExtensionEnabled, UninstallExtension, InstallExtensionFromZip, InstallExtensionFromDirectory } from './wails'
import type { Extension } from './wails'
import { registerCommand } from './commands'
import { registerKeybind } from './keybinds'
import { tabs, activeTabId, pendingFormatDocument, activeRightPanel } from './stores'

export const loadedExtensions = writable<Extension[]>([])
// `${extensionName}:${panelId}` -> sanitized HTML
export const extensionPanelHTML = writable<Map<string, string>>(new Map())
// `${extensionName}:${panelId}` -> a status-style dropdown next to that
// panel's input box. Absent (no entry) means the panel shows no dropdown —
// most extensions never call setPanelSelect and get the plain text input.
export interface PanelSelect { options: { value: string; label: string }[]; value: string }
export const extensionPanelSelect = writable<Map<string, PanelSelect>>(new Map())

const workers = new Map<string, Worker>()
const unregisterFns = new Map<string, (() => void)[]>()

function activeFilePath(): string {
  const t = get(tabs).find(t => t.id === get(activeTabId))
  return t && t.type === 'file' && t.path && t.path !== '__new__' ? t.path : ''
}

// Namespaced localStorage key for a worker's persisted secret (tokens etc).
// A worker has no storage of its own, so getSecret/setSecret relay through
// here — this is browser-side storage, not bish's Go config/backend.
function secretKey(extName: string, key: string): string {
  return `bish.ext.${extName}.secret.${key}`
}

// Forwards a panel's submitted input to that extension's worker, if running.
export function sendPanelInput(extName: string, panelId: string, value: string) {
  workers.get(extName)?.postMessage({ type: 'input', panelId, value })
}

// Forwards a panel's dropdown selection to that extension's worker.
export function sendPanelSelect(extName: string, panelId: string, value: string) {
  workers.get(extName)?.postMessage({ type: 'select', panelId, value })
}

// Runs one of the extension's manifest-declared commands directly (e.g. a
// panel's refresh icon), same message a Command Palette invocation sends.
export function runExtensionCommand(extName: string, commandId: string) {
  workers.get(extName)?.postMessage({ type: 'command', id: commandId })
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
    } else if (msg.type === 'setPanelSelect' && typeof msg.panelId === 'string') {
      extensionPanelSelect.update(m => {
        const next = new Map(m)
        const options = Array.isArray(msg.options)
          ? msg.options.filter((o: any) => o && typeof o.value === 'string' && typeof o.label === 'string')
          : []
        next.set(`${ext.name}:${msg.panelId}`, { options, value: String(msg.value ?? '') })
        return next
      })
    } else if (msg.type === 'getActiveFilePath' && msg.reqId != null) {
      worker.postMessage({ type: 'reply', reqId: msg.reqId, value: activeFilePath() })
    } else if (msg.type === 'getSecret' && msg.reqId != null && typeof msg.key === 'string') {
      worker.postMessage({ type: 'reply', reqId: msg.reqId, value: localStorage.getItem(secretKey(ext.name, msg.key)) })
    } else if (msg.type === 'setSecret' && typeof msg.key === 'string') {
      localStorage.setItem(secretKey(ext.name, msg.key), String(msg.value ?? ''))
    } else if (msg.type === 'formatActiveDocument') {
      const path = activeFilePath()
      if (path) pendingFormatDocument.set(path)
    }
  }

  const offs = unregisterFns.get(ext.name) ?? []
  for (const c of ext.commands ?? []) {
    const run = () => worker.postMessage({ type: 'command', id: c.id })
    offs.push(registerCommand({ id: `ext.${ext.name}.${c.id}`, title: c.title, key: c.key, run }))
    // manifest-declared default keybind — fires the moment the extension is
    // enabled, no separate Settings > Keybindings step (contrast with
    // user-defined keybinds in keymap.ts, which are opt-in per command)
    if (c.key) offs.push(registerKeybind({ combo: c.key, handler: (e) => { e.preventDefault(); run() } }))
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
  extensionPanelSelect.update(m => {
    const next = new Map(m)
    for (const k of [...next.keys()]) if (k.startsWith(name + ':')) next.delete(k)
    return next
  })
}

// Both open a native picker (zip file / folder), install under
// ~/.bish/extensions/<picked name>, and reload the list so the new
// extension's worker starts immediately. Return "" if the user cancelled
// the picker; throw on install failure (missing manifest, bad name, etc.).
export async function installExtensionFromZip(): Promise<string> {
  const name = await InstallExtensionFromZip()
  if (name) await loadExtensions()
  return name
}

export async function installExtensionFromDirectory(): Promise<string> {
  const name = await InstallExtensionFromDirectory()
  if (name) await loadExtensions()
  return name
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

// Deletes the extension's directory under ~/.bish/extensions and drops it
// from the list — unlike disabling, this can't be undone from Settings.
// Throws on failure (bad name, permission error, etc.) — the directory is
// only dropped from the list once the backend confirms it's actually gone,
// otherwise a swallowed error would leave it on disk while the UI claims
// it's uninstalled, and it'd reappear the next time extensions reload.
export async function uninstallExtension(name: string) {
  stopWorker(name)
  await UninstallExtension(name)
  loadedExtensions.update(list => list.filter(e => e.name !== name))
  // if the panel we just deleted was showing, fall back so the sidebar
  // doesn't point at a panel that no longer exists
  if (get(activeRightPanel).startsWith(`ext:${name}:`)) {
    activeRightPanel.set('files')
  }
}
