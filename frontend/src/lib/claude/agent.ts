// What ChatView / Composer / ToolCard need from a conversation, so the same
// UI renders both the Claude (lib/claude) and Codex (lib/codex) backends.
import type { Item, ToolItem, UserItem, Todo, ContextUsage, SendOptions } from './conversation.svelte'

export interface ModeDef { id: string; label: string; desc: string; danger?: boolean }
export interface CommandDef { name: string; description: string; hint?: string }

export interface AgentCaps {
  provider: 'claude' | 'codex'
  editRewind: boolean     // "edit" a past message (truncate + continue)
  restoreCode: boolean    // file checkpoints
  fork: boolean           // fork into a new tab from a message
  contextBreakdown: boolean
}

export interface AgentConversation {
  readonly key: string
  readonly caps: AgentCaps
  readonly modes: ModeDef[]
  readonly modeCycle: string[]
  title: string
  items: Item[]
  busy: boolean
  starting: boolean
  handle: string | null
  sessionId: string | null
  mode: string
  model: string
  effort: string
  activeModel: string
  info: any
  todos: Todo[]
  context: ContextUsage | null
  costUsd: number
  status: string
  unread: boolean
  onTitle?: () => void
  readonly projectRoot: string
  readonly pendingAsks: ToolItem[]
  readonly resumable: boolean
  readonly commands: CommandDef[]
  readonly terminalOnly: string[]
  note(text: string, tone?: 'info' | 'error' | 'compact' | 'command'): void
  send(o: SendOptions): Promise<void>
  interrupt(): Promise<void>
  setMode(mode: string): Promise<void>
  setModel(model: string): Promise<void>
  setSpawnOption(k: 'effort' | 'thinking', v: string): Promise<void>
  respond(t: ToolItem, allow: boolean, opts?: { message?: string; updatedInput?: any; suggestionIdx?: number[]; interrupt?: boolean }): Promise<void>
  refreshContext(): Promise<void>
  previewRewind(u: UserItem): Promise<{ canRewind: boolean; files: string[]; error?: string }>
  rewindFiles(u: UserItem): Promise<boolean>
  rewindConversation(u: UserItem): Promise<void>
  mcpStatus(): Promise<any[]>
  mcpReconnect(name: string): Promise<void>
  mcpToggle(name: string, enabled: boolean): Promise<void>
  /** provider-specific slash command; true = handled */
  runCommand(name: string, args: string): boolean
  stop(): void
  dispose(): void
}
