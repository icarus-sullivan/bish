// Provider adapters for the shared agent panel shell (components/agent).
// Each says how to create a conversation, list/open past ones, and which
// knobs its settings menu exposes.
import { AssistantListSessions, CodexStart, CodexCall, CodexStop } from '../wails'
import { Conversation, type PermissionMode } from './conversation.svelte'
import { CodexConversation } from '../codex/conversation.svelte'
import type { AgentConversation, ModeDef } from './agent'
import { CLAUDE_MODES } from './conversation.svelte'
import { CODEX_MODES } from '../codex/conversation.svelte'

export interface HistoryEntry { id: string; title: string; firstPrompt: string; gitBranch: string; modified: number; size: number }

export interface PanelPrefs { mode: string; model: string; effort: string; thinking: string }

export interface Provider {
  id: 'claude' | 'codex'
  name: string
  glyph: string
  /** modes allowed as the sticky default for new conversations */
  defaultModes: ModeDef[]
  defaultMode: string
  efforts: { value: string; label: string }[]
  thinking: boolean
  create(root: string, prefs: PanelPrefs): AgentConversation
  listHistory(root: string): Promise<HistoryEntry[]>
  open(conv: AgentConversation, id: string, title: string): Promise<void>
  fork?(src: AgentConversation, at: any, root: string, prefs: PanelPrefs): AgentConversation
  dispose(): void
}

export const claudeProvider: Provider = {
  id: 'claude',
  name: 'Claude Code',
  glyph: '✻',
  defaultModes: CLAUDE_MODES.filter(m => ['plan', 'manual', 'acceptEdits'].includes(m.id)),
  defaultMode: 'plan',
  efforts: [
    { value: '', label: 'Default' }, { value: 'low', label: 'Low' }, { value: 'medium', label: 'Medium' },
    { value: 'high', label: 'High' }, { value: 'xhigh', label: 'Extra high' }, { value: 'max', label: 'Max' },
  ],
  thinking: true,
  create(root, p) {
    return new Conversation(root, { mode: p.mode as PermissionMode, model: p.model, effort: p.effort, thinking: p.thinking })
  },
  async listHistory(root) { return (await AssistantListSessions(root)) ?? [] },
  async open(conv, id, title) { await (conv as Conversation).loadTranscript(id, title) },
  fork(src, u, root, p) {
    const c = new Conversation(root, { mode: src.mode as PermissionMode, model: src.model, effort: src.effort, thinking: p.thinking })
    c.forkFrom(src as Conversation, u)
    return c
  },
  dispose() {},
}

// thread/list needs a live app-server; one shared, lazily started process
// serves the History view for the whole Codex panel
let browser: { root: string; handle: Promise<string> } | null = null
export function browserHandle(root: string): Promise<string> {
  if (!browser || browser.root !== root) {
    if (browser) browser.handle.then(h => CodexStop(h)).catch(() => {})
    browser = { root, handle: CodexStart(root) }
    browser.handle.catch(() => { browser = null })
  }
  return browser.handle
}

export const codexProvider: Provider = {
  id: 'codex',
  name: 'Codex',
  glyph: '❯_',
  defaultModes: CODEX_MODES.filter(m => !m.danger),
  defaultMode: 'auto',
  efforts: [
    { value: '', label: 'Default' }, { value: 'minimal', label: 'Minimal' }, { value: 'low', label: 'Low' },
    { value: 'medium', label: 'Medium' }, { value: 'high', label: 'High' }, { value: 'xhigh', label: 'Extra high' },
  ],
  thinking: false,
  create(root, p) { return new CodexConversation(root, { mode: p.mode, model: p.model, effort: p.effort }) },
  async listHistory(root) {
    const h = await browserHandle(root)
    const r = JSON.parse(await CodexCall(h, 'thread/list', JSON.stringify({ limit: 100 })))
    const ms = (t: number) => (t && t < 1e12 ? t * 1000 : t)
    return (r?.data ?? []).filter((t: any) => !t.ephemeral).map((t: any) => ({
      id: t.id,
      title: (t.name || t.preview || 'Untitled').replace(/\s+/g, ' ').slice(0, 140),
      firstPrompt: t.preview ?? '',
      gitBranch: t.gitInfo?.branch ?? '',
      modified: ms(t.recencyAt ?? t.updatedAt ?? t.createdAt ?? 0),
      size: 0,
    }))
  },
  async open(conv, id, title) { await (conv as CodexConversation).loadThread(id, title) },
  dispose() {
    if (browser) browser.handle.then(h => CodexStop(h)).catch(() => {})
    browser = null
  },
}
