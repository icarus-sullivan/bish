import { waitForWails, on, GetProcesses, GetCommands, GetTreeNodes, GetTheme, GetGalleryImages, GetCWD } from './wails'
import {
  processes, commands, treeNodes, cwd,
  galleryMode, galleryImages, theme, projectRoot,
  showPalette, projectCommands, openFileTab
} from './stores'

export async function initEvents() {
  await waitForWails()

  // Load initial data
  const [procs, cmds, nodes, t, initialCwd] = await Promise.all([
    GetProcesses().catch(() => []),
    GetCommands().catch(() => []),
    GetTreeNodes().catch(() => []),
    GetTheme().catch(() => null),
    GetCWD().catch(() => ''),
  ])

  if (procs) processes.set(procs as any)
  if (cmds) commands.set(cmds as any)
  if (nodes) treeNodes.set(nodes as any)
  if (t) { theme.set(t as any); applyTheme(t as any) }
  if (initialCwd) cwd.set(initialCwd as string)

  // Wire backend → store events
  on('processes:update', (procs) => processes.set(procs))
  on('commands:update', (cmds) => commands.set(cmds))
  on('tree:update', (nodes) => treeNodes.set(nodes))
  on('cwd:change', (newCwd) => cwd.set(newCwd))
  on('theme:update', (t) => { theme.set(t); applyTheme(t) })
  on('project:change', (root: string) => projectRoot.set(root))
  on('project:commands', (cmds: any) => projectCommands.set(cmds ?? []))
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
