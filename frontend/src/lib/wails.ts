// Wails v2 generated bindings — do not call window.go directly
export { EventsOn as on, EventsOff as off } from '../../wailsjs/runtime/runtime'

// Re-export all bound methods so components can import by name
export {
  GetProcesses, KillProcess, RestartProcess, StopProcess, GetProcessLogs,
  GetCommands, RunCommand, DeleteCommand, RenameCommand, AddCommand,
  GetTreeNodes, ToggleTreeNode, RevealInTree, CdToPath,
  FSNewFile, FSNewFolder, FSRename, FSDelete, FSDeletePaths, FSCopyPath, FSRevealInFinder, FSMove, FSDuplicate, StashDropped,
  WritePTY, ResizePTY,
  GetGalleryImages, GetCurrentGalleryPath, IsVideo,
  GetTheme, GetConfig, SaveConfig,
  ExportSettingsFile, ImportSettingsFile,
  GetExtensions, SetExtensionEnabled, UninstallExtension, InstallExtensionFromZip, InstallExtensionFromDirectory,
  ReadFile, ReadFileChunk, WriteFile,
  OpenProject, CloseProject, GetProjectRoot, GetAllFiles, GetCWD, GetStartupFile,
  OpenRemoteProject, IsRemoteProject,
  NewWindow, SaveNewFile,
  GetProjectCommands, GetRecentProjects, OpenRecentProject, DeleteProjectCommand, RunProjectCommand, AddProjectCommand, RenameProjectCommand,
  GetTasks, RunTask,
  AddWorkspaceRoot, RemoveWorkspaceRoot, GetWorkspaceRoots, GitStatusForRoots,
  GetProjectUI, SaveProjectUI,
  NewTerminal, CloseTerminal, WritePTYTab, ResizePTYTab,
  StartLiveShare, StopLiveShare, IsLiveSharing, GetLiveShareGuests, SetLiveShareGuestPermission,
  StartEditShare, StopEditShare, IsEditSharing, GetEditShareGuests, SetEditShareGuestPermission, EditShareBroadcast,
  SearchInFiles, ReplaceInFiles,
  GetProjectSymbols,
  LSPStart, LSPSend, LSPStop, LSPInstalled, LSPInstall,
  ListLanguageExtensions, FormatterInstalled, FormatterInstall, FormatWithExtension,
  GetLanguageOverride, SetLanguageOverride,
  DebugStart, DebugSetBreakpoints, DebugContinue, DebugStepOver, DebugStepIn, DebugStepOut, DebugStop,
  AssistantStart, AssistantSend, AssistantRespondPermission, AssistantStop, AssistantInterrupt, AssistantSwitchMode, AssistantPickFiles,
  OllamaListModels,
  CompletionSuggest, ConfirmDiscardChanges,
  ReadFileBase64,
  RefreshTree, CollapseAllTree,
  GitBlame, GitStatus, GitDiff, GitDiffText,
  GitStage, GitUnstage, GitCommit, GitBranches, GitCheckout,
  GitCommitForRoot, GitBranchesForRoot, GitCheckoutForRoot,
  FileOutline,
  FileTests, GetGoTests, RunGoTest,
} from '../../wailsjs/go/app/App'

import { GetMediaBase } from '../../wailsjs/go/app/App'

// Videos must stream over real HTTP (WKWebView can't play media through the
// wails:// scheme). Base is fetched once at startup; '' falls back to the
// in-webview route.
let mediaBase = ''
export async function initMediaBase() {
  mediaBase = await GetMediaBase().catch(() => '')
}
export function mediaUrl(path: string): string {
  const enc = encodeURIComponent(path)
  return mediaBase ? mediaBase + enc : `/localfile?path=${enc}`
}

// Types
export interface Process {
  id: string; pid: number; name: string; cmd: string; cwd: string
  start_time: any; ports: number[]; cpu_pct: number; mem_mb: number
  status: 'running' | 'stopped' | 'crashed'; exit_code: number
}
export interface SavedCommand { id: string; name: string; cwd: string; command: string }
export interface ProjectCmd { id: string; name?: string; command: string; directory: string }
export interface Task { id: string; name: string; command: string; cwd: string }
export interface RecentEntry { path: string; name: string }
export interface SearchResultDTO { file: string; line: number; col: number; text: string }
export interface SymbolInfo { name: string; kind: string; file: string; importPath: string; pkg: string }
export interface BlameLine { sha: string; author: string; time: number; summary: string }
export interface GitFileStatus { status: string; path: string }
export interface DiffLine { line: number; type: 'added' | 'modified' | 'deleted' }
export interface OutlineSym { name: string; kind: string; line: number; depth: number }
export interface GoTest { name: string; file: string; line: number; pkg: string }
export interface ExtContribution { id: string; title: string; key?: string; icon?: string }
export interface Extension {
  name: string; main: string; dir: string; script: string; enabled: boolean
  commands?: ExtContribution[]; panels?: ExtContribution[]
}
export interface GitStatusDTO { branch: string; files: GitFileStatus[] }
export interface ProcessDef { candidates: string[][]; install?: string[]; installHint?: string }
export interface LanguageOverride {
  server_path?: string; server_args?: string[]; disable_server?: boolean
  formatter_path?: string; formatter_args?: string[]; disable_formatter?: boolean
  format_on_save?: boolean; indent_size?: number; indent_style?: string
}
export interface LanguageExtensionDTO {
  id: string; name: string; extensions: string[]
  languageIds?: Record<string, string>
  server?: ProcessDef; formatter?: ProcessDef; builtinFormatter?: boolean
  serverInstalled: boolean; formatterInstalled: boolean
  override: LanguageOverride
}
export interface LiveShareGuest { id: string; canType: boolean }
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
