import { waitForWails, on, GetProcesses, GetCommands, GetTreeNodes, GetTheme, GetGalleryImages, GetCWD,
         GetProjectRoot, GetProjectCommands, GetProjectUI, SaveProjectUI, GetConfig, GitStatus, initMediaBase } from './wails'
import {
  processes, commands, treeNodes, cwd,
  galleryMode, galleryImages, theme, projectRoot,
  showPalette, projectCommands, openFileTab,
  showRight, rightWidth,
  tabs, activeTabId, isMediaPath, activeRightPanel, persistPrefs, formatOnSave, gitBranch
} from './stores'
import { get } from 'svelte/store'
import { loadFeatures } from './features'
import { OnFileDrop } from '../../wailsjs/runtime/runtime'

export async function initEvents() {
  await waitForWails()
  await initMediaBase()

  // persistence prefs live in app config; missing = persist everything
  const cfg: any = await GetConfig().catch(() => null)
  if (cfg?.persist) persistPrefs.set(cfg.persist)
  formatOnSave.set(!!cfg?.format_on_save)
  loadFeatures(cfg?.features)

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

  // Single native file-drop router: OnFileDropOff() is global (one component's
  // cleanup would deregister everyone), so register once and hand the drop to
  // whatever sits under the cursor via a bubbling DOM event
  OnFileDrop((x: number, y: number, paths: string[]) => {
    if (!paths.length) return
    const el = document.elementFromPoint(x, y) ?? document.body
    el.dispatchEvent(new CustomEvent('bish:filedrop', { detail: { paths }, bubbles: true }))
  }, false)

  refreshGitBranch()

  // Wire backend → store events
  on('processes:update', (procs) => processes.set(procs))
  on('commands:update', (cmds) => commands.set(cmds))
  on('tree:update', (nodes) => { treeNodes.set(nodes); refreshGitBranch() })
  on('cwd:change', (newCwd) => { cwd.set(newCwd); refreshGitBranch() })
  on('theme:update', (t) => { theme.set(t); applyTheme(t) })
  on('project:change', (root: string) => {
    const prev = get(projectRoot)
    projectRoot.set(root)
    if (root && root !== prev) loadProjectUI()
    refreshGitBranch()
  })
  on('project:commands', (cmds: any) => projectCommands.set(cmds ?? []))

  const uiStores: { subscribe: (fn: (v: any) => void) => unknown }[] =
    [showRight, rightWidth, tabs, activeTabId, activeRightPanel]
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

function refreshGitBranch() {
  GitStatus().then((s: any) => gitBranch.set(s?.branch || null)).catch(() => gitBranch.set(null))
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
    activeTabId.set(get(tabs)[0]?.id ?? '')
    if (!ui) return
    const p = get(persistPrefs)
    if (p.panel_width && ui.right_width) rightWidth.set(ui.right_width)
    if (p.right_sidebar && ui.show_right != null) showRight.set(ui.show_right)
    if (p.right_panel && ui.right_panel) activeRightPanel.set(ui.right_panel)
    if (p.tabs) {
      for (const t of ui.tabs ?? []) {
        // forceText when a media-extension path was open as a text tab
        openFileTab(t.path, t.type === 'file' && isMediaPath(t.path))
      }
      if (ui.active_tab && get(tabs).some(t => t.id === ui.active_tab)) {
        activeTabId.set(ui.active_tab)
      }
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
  const p = get(persistPrefs)
  const ui: any = {}
  if (p.panel_width) ui.right_width = get(rightWidth)
  if (p.right_sidebar) ui.show_right = get(showRight)
  if (p.right_panel) ui.right_panel = get(activeRightPanel)
  if (p.tabs) {
    ui.tabs = get(tabs)
      .filter(t => (t.type === 'file' || t.type === 'media') && t.path && t.path !== '__new__')
      .map(t => ({ type: t.type, path: t.path! }))
    ui.active_tab = get(activeTabId)
  }
  SaveProjectUI(ui).catch(() => {})
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
