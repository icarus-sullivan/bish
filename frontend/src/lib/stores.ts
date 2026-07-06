import { writable, get } from 'svelte/store'
import type { Process, SavedCommand, TreeNode, Theme, ProjectCmd } from './wails'

export type Pane = 'processes' | 'commands' | 'terminal' | 'tree'

export const focusedPane = writable<Pane>('terminal')
export const processes = writable<Process[]>([])
export const commands = writable<SavedCommand[]>([])
export const treeNodes = writable<TreeNode[]>([])
export const cwd = writable<string>('')
export const theme = writable<Theme | null>(null)

// gallery mode
export const galleryMode = writable<boolean>(false)
export const galleryImages = writable<string[]>([])
export const galleryIndex = writable<number>(0)

// selected items in panels (for keyboard nav)
export const selectedProcess = writable<string | null>(null)
export const selectedCommand = writable<string | null>(null)

// active theme name (for editor syntax highlighting)
export const currentThemeName = writable<string>('catppuccin')

// pinned project root (empty = follow CWD)
export const projectRoot = writable<string>('')

// project-scoped commands (populated when a project is open)
export const projectCommands = writable<ProjectCmd[]>([])

// command palette visibility
export const showPalette = writable<boolean>(false)

// global file search
export const showGlobalSearch = writable<boolean>(false)

// panel visibility
export const showLeft = writable<boolean>(true)
export const showRight = writable<boolean>(true)

// panel sizes (px)
export const leftWidth = writable<number>(220)
export const rightWidth = writable<number>(220)
export const processHeight = writable<number>(300)

// ─── Tab system ───────────────────────────────────────────────────────────────

export interface Tab {
  id: string
  type: 'terminal' | 'file' | 'logs' | 'media'
  label: string
  path?: string       // file + media tabs
  processId?: string  // logs tabs
}

const MEDIA_EXTS = new Set(['png','jpg','jpeg','gif','webp','bmp','tiff','tif','svg','ico',
                             'mp4','mov','webm','mkv','avi'])

export function isMediaPath(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return MEDIA_EXTS.has(ext)
}

export const tabs = writable<Tab[]>([{ id: 'main', type: 'terminal', label: 'Terminal' }])
export const activeTabId = writable<string>('main')

export function openFileTab(path: string, forceText = false) {
  if (path === '__new__') {
    const id = 'file:__new__:' + Date.now()
    tabs.update(ts => [...ts, { id, type: 'file', label: 'Untitled', path: '__new__' }])
    activeTabId.set(id)
    return
  }
  if (!forceText && isMediaPath(path)) {
    const existing = get(tabs).find(t => t.type === 'media' && t.path === path)
    if (existing) { activeTabId.set(existing.id); return }
    const id = 'media:' + path
    const label = path.split('/').pop() || path
    tabs.update(ts => [...ts, { id, type: 'media', label, path }])
    activeTabId.set(id)
    return
  }
  const existing = get(tabs).find(t => t.type === 'file' && t.path === path)
  if (existing) {
    activeTabId.set(existing.id)
    return
  }
  const id = 'file:' + path
  const label = path.split('/').pop() || path
  tabs.update(ts => [...ts, { id, type: 'file', label, path }])
  activeTabId.set(id)
}

export function openLogsTab(processId: string, label: string) {
  const existing = get(tabs).find(t => t.type === 'logs' && t.processId === processId)
  if (existing) {
    activeTabId.set(existing.id)
    return
  }
  const id = 'logs:' + processId
  tabs.update(ts => [...ts, { id, type: 'logs', label, processId }])
  activeTabId.set(id)
}

export function addTerminalTab(id: string) {
  const count = get(tabs).filter(t => t.type === 'terminal').length + 1
  const label = count === 1 ? 'Terminal' : `Terminal ${count}`
  tabs.update(ts => [...ts, { id, type: 'terminal', label }])
  activeTabId.set(id)
}

export function closeTab(id: string) {
  const current = get(tabs)
  if (id === 'main' && current.filter(t => t.type === 'terminal').length <= 1) return
  const idx = current.findIndex(t => t.id === id)
  if (idx === -1) return
  const newTabs = current.filter(t => t.id !== id)
  if (newTabs.length === 0) newTabs.push({ id: 'main', type: 'terminal', label: 'Terminal' })
  tabs.set(newTabs)
  if (get(activeTabId) === id) {
    const newIdx = Math.min(idx, newTabs.length - 1)
    activeTabId.set(newTabs[newIdx].id)
  }
}

// Returns non-main terminal tab IDs that need CloseTerminal() called by the caller.
function bulkClose(toRemove: Tab[]): string[] {
  const current = get(tabs)
  const removeIds = new Set(toRemove.map(t => t.id))

  // Never remove the main terminal if it would leave no terminals
  const remainingTerminals = current.filter(t => t.type === 'terminal' && !removeIds.has(t.id))
  if (remainingTerminals.length === 0) {
    // keep main terminal
    removeIds.delete('main')
  }

  const ptyClosed = toRemove
    .filter(t => t.type === 'terminal' && t.id !== 'main' && removeIds.has(t.id))
    .map(t => t.id)

  let newTabs = current.filter(t => !removeIds.has(t.id))
  if (newTabs.length === 0) newTabs = [{ id: 'main', type: 'terminal', label: 'Terminal' }]
  tabs.set(newTabs)

  const active = get(activeTabId)
  if (removeIds.has(active)) {
    activeTabId.set(newTabs[Math.min(0, newTabs.length - 1)].id)
  }

  return ptyClosed
}

export function closeTabsToRight(id: string): string[] {
  const current = get(tabs)
  const idx = current.findIndex(t => t.id === id)
  if (idx === -1) return []
  const removed = bulkClose(current.slice(idx + 1))
  activeTabId.set(id)
  return removed
}

export function closeTabsToLeft(id: string): string[] {
  const current = get(tabs)
  const idx = current.findIndex(t => t.id === id)
  if (idx === -1) return []
  const removed = bulkClose(current.slice(0, idx))
  activeTabId.set(id)
  return removed
}

export function closeOtherTabs(id: string): string[] {
  const current = get(tabs)
  const removed = bulkClose(current.filter(t => t.id !== id))
  activeTabId.set(id)
  return removed
}

export function closeAllTabs(): string[] {
  return bulkClose(get(tabs))
}

export function updateTabPath(tabId: string, newPath: string) {
  const label = newPath.split('/').pop() || newPath
  tabs.update(ts => ts.map(t =>
    t.id === tabId ? { ...t, path: newPath, label } : t
  ))
}
