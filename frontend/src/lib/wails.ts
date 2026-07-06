// Wails v2 generated bindings — do not call window.go directly
export { EventsOn as on, EventsOff as off } from '../../wailsjs/runtime/runtime'

// Re-export all bound methods so components can import by name
export {
  GetProcesses, KillProcess, RestartProcess, GetProcessLogs,
  GetCommands, RunCommand, DeleteCommand, RenameCommand,
  GetTreeNodes, ToggleTreeNode, CdToPath,
  FSNewFile, FSNewFolder, FSRename, FSDelete, FSCopyPath, FSRevealInFinder,
  WritePTY, ResizePTY,
  GetGalleryImages, GetCurrentGalleryPath, IsVideo,
  GetTheme, GetConfig, SaveConfig,
  ReadFile, WriteFile,
  OpenProject, CloseProject, GetProjectRoot, GetAllFiles, GetCWD,
  NewWindow, SaveNewFile,
  GetProjectCommands, GetRecentProjects, OpenRecentProject, DeleteProjectCommand, RunProjectCommand,
  NewTerminal, CloseTerminal, WritePTYTab, ResizePTYTab,
  SearchInFiles, ReplaceInFiles,
  ReadFileBase64,
  RefreshTree, CollapseAllTree,
} from '../../wailsjs/go/app/App'

// Types
export interface Process {
  id: string; pid: number; name: string; cmd: string; cwd: string
  start_time: any; ports: number[]; cpu_pct: number; mem_mb: number
  status: 'running' | 'stopped' | 'crashed'; exit_code: number
}
export interface SavedCommand { id: string; name: string; cwd: string; command: string }
export interface ProjectCmd { id: string; command: string; directory: string }
export interface RecentEntry { path: string; name: string }
export interface SearchResultDTO { file: string; line: number; col: number; text: string }
export interface TreeNode {
  name: string; path: string; isDir: boolean; depth: number
  expanded: boolean; selected: boolean
}
export interface Theme {
  background: string; foreground: string; border: string; borderFocused: string
  accent: string; muted: string; success: string; error: string; warning: string
}

// Wait for Wails runtime to be injected (can be async in some launch paths)
export function waitForWails(): Promise<void> {
  return new Promise((resolve) => {
    if ((window as any).go?.app?.App) { resolve(); return }
    const t = setInterval(() => {
      if ((window as any).go?.app?.App) { clearInterval(t); resolve() }
    }, 50)
    setTimeout(() => { clearInterval(t); resolve() }, 10000)
  })
}
