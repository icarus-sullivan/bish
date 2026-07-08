import { waitForWails, on, GetProcesses, GetCommands, GetTreeNodes, GetTheme, GetGalleryImages, GetCWD,
         GetProjectRoot, GetProjectCommands, GetProjectUI, SaveProjectUI } from './wails'
import {
  processes, commands, treeNodes, cwd,
  galleryMode, galleryImages, theme, projectRoot,
  showPalette, projectCommands, openFileTab,
  showLeft, showRight, leftWidth, rightWidth, processHeight,
  tabs, activeTabId, isMediaPath
} from './stores'
import { get } from 'svelte/store'

export async function initEvents() {
  await waitForWails()

  // Load initial data
  const [procs, cmds, nodes, t, initialCwd, root, pcmds] = await Promise.all([
    GetProcesses().catch(() => []),
    GetCommands().catch(() => []),
    GetTreeNodes().catch(() => []),
    GetTheme().catch(() => null),
    GetCWD().catch(() => ''),
    // fetched explicitly: the backend may open the project (session restore /
    // --project) before our event listeners exist, so the events alone can be missed
    GetProjectRoot().catch(() => ''),
    GetProjectCommands().catch(() => []),
  ])

  if (procs) processes.set(procs as any)
  if (cmds) commands.set(cmds as any)
  if (nodes) treeNodes.set(nodes as any)
  if (t) { theme.set(t as any); applyTheme(t as any) }
  if (initialCwd) cwd.set(initialCwd as string)
  if (root) {
    projectRoot.set(root as string)
    projectCommands.set((pcmds as any) ?? [])
    await loadProjectUI()
  }

  // Wire backend → store events
  on('processes:update', (procs) => processes.set(procs))
  on('commands:update', (cmds) => commands.set(cmds))
  on('tree:update', (nodes) => treeNodes.set(nodes))
  on('cwd:change', (newCwd) => cwd.set(newCwd))
  on('theme:update', (t) => { theme.set(t); applyTheme(t) })
  on('project:change', (root: string) => {
    const prev = get(projectRoot)
    projectRoot.set(root)
    if (root && root !== prev) loadProjectUI()
  })
  on('project:commands', (cmds: any) => projectCommands.set(cmds ?? []))

  const uiStores: { subscribe: (fn: (v: any) => void) => unknown }[] =
    [showLeft, showRight, leftWidth, rightWidth, processHeight, tabs, activeTabId]
  uiStores.forEach(s => s.subscribe(scheduleSaveUI))
  on('file:new', () => openFileTab('__new__'))
  on('palette:open', () => showPalette.set(true))
  on('gallery:open', async (dirPath: string) => {
    const imgs = await GetGalleryImages(dirPath).catch(() => [])
    if (imgs && imgs.length > 0) {
      galleryImages.set(imgs)
      galleryMode.set(true)
    }
  })
}

// ─── per-project UI state (panel sizes/visibility, open tabs) ────────────────

let applyingUI = false
let saveTimer: ReturnType<typeof setTimeout> | undefined

async function loadProjectUI() {
  applyingUI = true
  try {
    const ui: any = await GetProjectUI().catch(() => null)
    // swap out the previous project's file/media tabs
    tabs.update(ts => ts.filter(t => t.type === 'terminal' || t.type === 'logs'))
    activeTabId.set('main')
    if (!ui) return
    if (ui.left_width) leftWidth.set(ui.left_width)
    if (ui.right_width) rightWidth.set(ui.right_width)
    if (ui.process_height) processHeight.set(ui.process_height)
    if (ui.show_left != null) showLeft.set(ui.show_left)
    if (ui.show_right != null) showRight.set(ui.show_right)
    for (const t of ui.tabs ?? []) {
      // forceText when a media-extension path was open as a text tab
      openFileTab(t.path, t.type === 'file' && isMediaPath(t.path))
    }
    if (ui.active_tab && get(tabs).some(t => t.id === ui.active_tab)) {
      activeTabId.set(ui.active_tab)
    }
  } finally {
    applyingUI = false
  }
}

function scheduleSaveUI() {
  if (applyingUI || !get(projectRoot)) return
  clearTimeout(saveTimer)
  saveTimer = setTimeout(saveProjectUI, 400)
}

function saveProjectUI() {
  if (!get(projectRoot)) return
  const openTabs = get(tabs)
    .filter(t => (t.type === 'file' || t.type === 'media') && t.path && t.path !== '__new__')
    .map(t => ({ type: t.type, path: t.path! }))
  SaveProjectUI({
    left_width: get(leftWidth),
    right_width: get(rightWidth),
    process_height: get(processHeight),
    show_left: get(showLeft),
    show_right: get(showRight),
    tabs: openTabs,
    active_tab: get(activeTabId),
  } as any).catch(() => {})
}

function applyTheme(t: any) {
  if (!t) return
  const root = document.documentElement
  Object.entries(t).forEach(([k, v]) => {
    root.style.setProperty(`--${camelToKebab(k)}`, v as string)
  })
}

function camelToKebab(s: string) {
  return s.replace(/([A-Z])/g, '-$1').toLowerCase()
}
