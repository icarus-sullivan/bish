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
export const showActionPalette = writable<boolean>(false)

// global file search; searchScopeDir null = whole project, else scoped to that dir
export const showGlobalSearch = writable<boolean>(false)
export const searchScopeDir = writable<string | null>(null)

// jump target consumed by FileViewer after opening a file (line 1-based, col 0-based)
export const pendingGoto = writable<{ path: string; line: number; col: number } | null>(null)

// focus request consumed by FileViewer — set on every openFileTab so the
// editor grabs focus even when its tab was already active (no remount)
export const pendingFocus = writable<string | null>(null)

// always-current selection in the active editor (not consume-once like
// pendingGoto — readers just check "what's selected right now")
export interface EditorSelection {
  path: string; from: number; to: number; text: string; line: number; col: number
}
export const activeSelection = writable<EditorSelection | null>(null)

// panel visibility
export const showRight = writable<boolean>(true)

// active panel in the right sidebar (ids from lib/panels.ts)
export const activeRightPanel = writable<string>('files')

// which per-project UI state gets saved/restored (config.json `persist`,
// missing = all true)
export interface PersistPrefs {
  panel_width: boolean
  right_sidebar: boolean
  right_panel: boolean
  tabs: boolean
}
export const persistPrefs = writable<PersistPrefs>({
  panel_width: true, right_sidebar: true, right_panel: true, tabs: true,
})

// format buffer via LSP before writing on ⌘S (config.json format_on_save)
export const formatOnSave = writable<boolean>(false)

// panel sizes (px)
export const rightWidth = writable<number>(220)

// ─── Tab system ───────────────────────────────────────────────────────────────

export interface Tab {
  id: string
  type: 'terminal' | 'file' | 'logs' | 'media' | 'settings' | 'diff'
  label: string
  baseLabel?: string  // terminal tabs: original label, restored when title clears
  path?: string       // file + media tabs
  processId?: string  // logs tabs
  modified?: boolean  // file tabs: unsaved changes
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
  pendingFocus.set(path)
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

export function openDiffTab(path: string) {
  const id = 'diff:' + path
  const existing = get(tabs).find(t => t.id === id)
  if (existing) { activeTabId.set(id); return }
  const label = (path.split('/').pop() || path) + ' (diff)'
  tabs.update(ts => [...ts, { id, type: 'diff', label, path }])
  activeTabId.set(id)
}

export function openSettingsTab() {
  const existing = get(tabs).find(t => t.type === 'settings')
  if (existing) { activeTabId.set(existing.id); return }
  tabs.update(ts => [...ts, { id: 'settings', type: 'settings', label: 'Settings' }])
  activeTabId.set('settings')
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

// Reopen the main terminal tab. Its PTY stays alive in Go while the tab is
// closed, so this reattaches; only new output appears (like an app restart).
export function reopenMainTab() {
  if (!get(tabs).some(t => t.id === 'main')) {
    tabs.update(ts => [{ id: 'main', type: 'terminal', label: 'Terminal' } as Tab, ...ts])
  }
  activeTabId.set('main')
}

export function addTerminalTab(id: string) {
  const count = get(tabs).filter(t => t.type === 'terminal').length + 1
  const label = count === 1 ? 'Terminal' : `Terminal ${count}`
  tabs.update(ts => [...ts, { id, type: 'terminal', label }])
  activeTabId.set(id)
}

// terminal font size, persisted client-side (no Go round-trip). ⌘+ / ⌘- / ⌘0.
const savedFont = Number(localStorage.getItem('bish.termFontSize')) || 13
export const terminalFontSize = writable<number>(savedFont)
terminalFontSize.subscribe(v => localStorage.setItem('bish.termFontSize', String(v)))

export function cycleTab(dir: 1 | -1) {
  const ts = get(tabs)
  if (ts.length < 2) return
  const i = ts.findIndex(t => t.id === get(activeTabId))
  activeTabId.set(ts[(i + dir + ts.length) % ts.length].id)
}

export function setTabLabel(id: string, label: string) {
  const l = label.trim()
  if (!l) return
  tabs.update(ts => ts.map(t => t.id === id ? { ...t, label: l, baseLabel: l } : t))
}

export function setTabModified(id: string, modified: boolean) {
  // no-op when unchanged: FileViewer's load() runs inside an $effect and
  // calls this synchronously — an unconditional store emit re-renders the
  // tab list, re-triggering the effect → infinite loop (all clicks die)
  const t = get(tabs).find(t => t.id === id)
  if (!t || !!t.modified === modified) return
  tabs.update(ts => ts.map(t => t.id === id ? { ...t, modified } : t))
}

export function closeTab(id: string) {
  const current = get(tabs)
  const idx = current.findIndex(t => t.id === id)
  if (idx === -1) return
  if (current[idx].modified &&
      !window.confirm(`Discard unsaved changes to ${current[idx].label}?`)) return
  const newTabs = current.filter(t => t.id !== id)
  tabs.set(newTabs)
  if (get(activeTabId) === id) {
    const newIdx = Math.min(idx, newTabs.length - 1)
    activeTabId.set(newIdx >= 0 ? newTabs[newIdx].id : '')
  }
}

// Returns non-main terminal tab IDs that need CloseTerminal() called by the caller.
function bulkClose(toRemove: Tab[]): string[] {
  const current = get(tabs)
  const removeIds = new Set(toRemove.map(t => t.id))

  const ptyClosed = toRemove
    .filter(t => t.type === 'terminal' && t.id !== 'main' && removeIds.has(t.id))
    .map(t => t.id)

  const newTabs = current.filter(t => !removeIds.has(t.id))
  tabs.set(newTabs)

  const active = get(activeTabId)
  if (removeIds.has(active)) {
    activeTabId.set(newTabs.length > 0 ? newTabs[0].id : '')
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

export function reorderTabs(fromId: string, beforeId: string | null) {
  tabs.update(ts => {
    const fromIdx = ts.findIndex(t => t.id === fromId)
    if (fromIdx === -1) return ts
    const tab = ts[fromIdx]
    const rest = ts.filter(t => t.id !== fromId)
    if (beforeId === null) return [...rest, tab]
    const toIdx = rest.findIndex(t => t.id === beforeId)
    if (toIdx === -1) return [...rest, tab]
    return [...rest.slice(0, toIdx), tab, ...rest.slice(toIdx)]
  })
}

// terminal title from OSC escape (xterm onTitleChange) — running command shows
// as the tab label; empty title (shell back at prompt) restores the original
export function setTerminalTitle(id: string, title: string) {
  tabs.update(ts => ts.map(t => {
    if (t.id !== id || t.type !== 'terminal') return t
    const base = t.baseLabel ?? t.label
    return { ...t, baseLabel: base, label: title.trim() || base }
  }))
}

export function updateTabPath(tabId: string, newPath: string) {
  const label = newPath.split('/').pop() || newPath
  tabs.update(ts => ts.map(t =>
    t.id === tabId ? { ...t, path: newPath, label } : t
  ))
}
