// Wails v2 generated bindings — do not call window.go directly
export { EventsOn as on, EventsOff as off, EventsEmit as emit } from '../../wailsjs/runtime/runtime'

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
  ReadFile, ReadFileChunk, WriteFile, WriteFileChecked, StatMtime, ConfirmFileConflict,
  OpenProject, CloseProject, GetProjectRoot, GetAllFiles, GetCWD, GetStartupFile,
  OpenRemoteProject, IsRemoteProject,
  NewWindow, SaveNewFile,
  GetProjectCommands, GetRecentProjects, OpenRecentProject, DeleteProjectCommand, RunProjectCommand, AddProjectCommand, RenameProjectCommand,
  GetTasks, RunTask,
  AddWorkspaceRoot, RemoveWorkspaceRoot, GetWorkspaceRoots, GitStatusForRoots, GitWorktrees, SwitchWorktree,
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
  AssistantStart, AssistantSend, AssistantSendWithImages, AssistantRespondPermission, AssistantStop, AssistantInterrupt, AssistantSwitchMode, AssistantPickFiles,
  AssistantStartWithOptions, AssistantSessionInfo, AssistantControl, AssistantRespondPermissionEx, AssistantListSessions, AssistantLoadTranscript,
  CodexInstalled, CodexEnsureInstalled, CodexStart, CodexCall, CodexRespond, CodexStop,
  OllamaListModels,
  CompletionSuggest, ConfirmDiscardChanges,
  ReadFileBase64,
  RefreshTree, CollapseAllTree,
  GitBlame, GitStatus, GitDiff, GitDiffText,
  GitStage, GitUnstage, GitCommit, GitBranches, GitCheckout,
  GitCommitForRoot, GitBranchesForRoot, GitCheckoutForRoot,
  FileOutline,
  FileTests, GetGoTests, RunGoTest,
  RefreshCommandCenterRepo, StartCommandCenterRepo, StartCommandCenterService, StartAllCommandCenter,
  StopCommandCenterRepo, StopAllCommandCenter,
} from '../../wailsjs/go/app/App'

import { GetMediaBase } from '../../wailsjs/go/app/App'
import {
  GetCommandCenterSnapshot as _GetCommandCenterSnapshot,
  SaveCommandCenterDefinition as _SaveCommandCenterDefinition,
  SetCommandCenterTarget as _SetCommandCenterTarget,
  GetCommandCenterBranches as _GetCommandCenterBranches,
  DetectRepoEnv as _DetectRepoEnv,
  ApplyRepoProposal as _ApplyRepoProposal,
  CreateEnv as _CreateEnv,
  DefaultCommandCenterDBSpec as _DefaultCommandCenterDBSpec,
} from '../../wailsjs/go/app/App'
export { SetActiveEnv, StartEnv, StopEnv, DestroyEnv, ClearStepCache, PreviewFrameBlocked } from '../../wailsjs/go/app/App'

// The generated .d.ts types these four with the wailsjs/go/models.ts classes
// (which require a convertValues method nothing at runtime actually
// provides — Wails never constructs those classes for you, `.d.ts` typing
// is TS-only). Thin wrappers here keep our plain CC* interfaces (below) as
// the types components actually use, same as Process/SavedCommand/etc. above.
export function GetCommandCenterSnapshot(): Promise<CCSnapshot> { return _GetCommandCenterSnapshot() as unknown as Promise<CCSnapshot> }
export function SaveCommandCenterDefinition(def: CCDefinition): Promise<void> { return _SaveCommandCenterDefinition(def as any) }
export function SetCommandCenterTarget(repoId: string, target: CCTarget): Promise<void> { return _SetCommandCenterTarget(repoId, target as any) }
export function GetCommandCenterBranches(repoId: string): Promise<CCBranchInfo[]> { return _GetCommandCenterBranches(repoId) as unknown as Promise<CCBranchInfo[]> }
export function DetectRepoEnv(repoId: string): Promise<CCProposal> { return _DetectRepoEnv(repoId) as unknown as Promise<CCProposal> }
export function ApplyRepoProposal(repoId: string, p: CCProposal): Promise<void> { return _ApplyRepoProposal(repoId, p as any) }
export function CreateEnv(name: string, branch: string): Promise<CCEnv> { return _CreateEnv(name, branch) as unknown as Promise<CCEnv> }
export function DefaultCommandCenterDBSpec(mode: string): Promise<CCDBSpec> { return _DefaultCommandCenterDBSpec(mode) as unknown as Promise<CCDBSpec> }

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
export interface ExtContribution { id: string; title: string; key?: string; icon?: string; iconSvg?: string; iconSrc?: string }
export interface Extension {
  name: string; main: string; dir: string; script: string; enabled: boolean
  commands?: ExtContribution[]; panels?: ExtContribution[]
}
export interface GitStatusDTO { branch: string; files: GitFileStatus[] }
export interface GitWorktree { path: string; branch: string; head: string; bare: boolean; locked: boolean }
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

// -- Command Center --
export interface CCStep {
  name: string; cmd: string; default: boolean
  destructive?: boolean; supersedes?: string[]
  kind?: string; cacheInputs?: string[]; stateful?: boolean
}
export interface CCHealth { http?: string; status?: number; logMatch?: string; cmd?: string; timeoutSec?: number }
export interface CCService {
  name: string; cmd: string; port: number
  health?: CCHealth | null; portEnv?: string; portArgs?: string
}
export interface CCDBSpec {
  mode: 'template' | 'compose'; file?: string; template?: string; port?: number
  create: string; drop: string; ready: string; urlEnv: string; url: string
}
export interface CCRepo {
  id: string; name: string; path: string; mainBranch: string
  // Go nil slices marshal to JSON null, not [] — always null-check these
  // (r.dependsOn ?? []) rather than assuming the array itself exists.
  dependsOn: string[] | null; worktreeIn: string; prefix: string
  overrides: string; setup: string; link: string[] | null; copy: string[] | null
  env: Record<string, string>; steps: CCStep[] | null; services: CCService[] | null
  compose?: string; db?: CCDBSpec | null
}
export interface CCDefinition { repos: CCRepo[] }
export interface CCTarget {
  mode: 'main' | 'worktree' | 'off'
  path: string; branch: string
  services: string[] | null; steps?: Record<string, boolean>; env: Record<string, string>
}
export interface CCEnv {
  name: string; branch: string; portOffset: number; dbName?: string
  targets: Record<string, CCTarget>; createdAt: string
}
// targets mirrors the active env's targets
export interface CCState { targets: Record<string, CCTarget>; envs: CCEnv[]; active: string }
export interface CCBranchInfo { name: string; path?: string; main?: boolean; remote?: boolean }
export interface CCServiceStatus {
  key: string; processId: string; pid: number
  status: 'running' | 'stopped' | 'crashed' | ''; ports: number[] | null
  ready: boolean; phase: '' | 'starting' | 'waiting-deps' | 'ready' | 'unhealthy' | 'stopped'
  detail: string; port: number
}
export interface CCSnapshot {
  definition: CCDefinition; state: CCState; statuses: Record<string, CCServiceStatus>
  running: Record<string, number>; drift: Record<string, string[]>
}
export interface CCCandidate {
  name: string; cmd: string; port?: number; portEnv?: string; portArgs?: string
  kind: string; destructive?: boolean; supersedes: string[]
  source: string; confidence: number; accept: boolean
}
export interface CCProposal {
  repoId: string; toolchain: string; services: CCCandidate[]; steps: CCCandidate[]
  compose?: string; notes: string[]
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
