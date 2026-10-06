// One Claude conversation (one panel tab): owns its `claude` subprocess
// handle and turns the CLI's stream-json events into a renderable item list.
// The reducer (handle) is shared by live events and by transcript replay
// when a past session is reopened from History.
import {
  on, AssistantStartWithOptions, AssistantSessionInfo, AssistantControl, AssistantSend, AssistantSendWithImages,
  AssistantRespondPermissionEx, AssistantStop, AssistantInterrupt, AssistantSwitchMode, AssistantLoadTranscript,
} from '../wails'
import { pendingExternalReload } from '../stores'
import { renderMarkdown, resultText, stripAnsi, FILE_EDIT_TOOLS } from './format'
import { SLASH_COMMANDS } from '../slashCommands'
import type { AgentCaps, AgentConversation, CommandDef, ModeDef } from './agent'

export const CLAUDE_MODES: ModeDef[] = [
  { id: 'manual', label: 'Ask before edits', desc: 'Claude asks before every edit and command' },
  { id: 'acceptEdits', label: 'Edit automatically', desc: 'File edits apply without asking; commands still ask' },
  { id: 'plan', label: 'Plan mode', desc: 'Read-only exploration, then a plan for you to approve' },
  { id: 'auto', label: 'Auto', desc: 'A classifier approves safe actions and asks about risky ones' },
  { id: 'dontAsk', label: "Don't ask", desc: 'Anything not pre-approved is denied instead of asking' },
  { id: 'bypassPermissions', label: 'Bypass permissions', desc: 'Everything runs with no checks — dangerous', danger: true },
]

export type PermissionMode = 'plan' | 'acceptEdits' | 'auto' | 'bypassPermissions' | 'manual' | 'dontAsk'

export interface Ask {
  requestId: string
  suggestions: any[]
  reason: string
  reasonType: string
  blockedPath: string
  suppressAlways: boolean
  defaultToNo: boolean
  title: string
  state: 'pending' | 'allowed' | 'denied' | 'cancelled'
  note?: string
}

export interface ToolResult { text: string; isError: boolean }

export interface UserItem { kind: 'user'; id: string; text: string; images: string[]; files: string[]; uuid?: string; prevAssistantUuid?: string | null; turnId?: string }
export interface TextItem { kind: 'text'; id: string; md: string; html: string; streaming: boolean; msgId?: string }
export interface ThinkingItem { kind: 'thinking'; id: string; text: string; streaming: boolean; msgId?: string }
export interface ToolItem {
  kind: 'tool'; id: string; toolUseId: string; name: string; input: any; partialJson: string
  status: 'streaming' | 'running' | 'done' | 'error' | 'denied'
  result?: ToolResult; ask?: Ask; children: ToolItem[]; childText: string
}
export interface NoteItem { kind: 'note'; id: string; tone: 'info' | 'error' | 'compact' | 'command'; text: string }
export interface ResultItem { kind: 'result'; id: string; durationMs: number; costUsd: number; turns: number; isError: boolean }
export type Item = UserItem | TextItem | ThinkingItem | ToolItem | NoteItem | ResultItem

export interface Todo { content: string; status: 'pending' | 'in_progress' | 'completed'; activeForm?: string }
export interface ContextUsage { total: number; max: number; pct: number; categories: { name: string; tokens: number; color: string }[] }

export interface StartPrefs { mode: PermissionMode; model: string; effort: string; thinking: string }

export interface SendOptions { text: string; context: string; images: string[]; files: string[] }

let keySeq = 0
let itemSeq = 0
const nid = () => 'i' + (itemSeq++).toString(36)

export class Conversation implements AgentConversation {
  readonly key = 'c' + (keySeq++)
  readonly caps: AgentCaps = { provider: 'claude', editRewind: true, restoreCode: true, fork: true, contextBreakdown: true }
  readonly modes = CLAUDE_MODES
  readonly modeCycle = ['manual', 'acceptEdits', 'plan']
  title = $state('New conversation')
  items = $state<Item[]>([])
  busy = $state(false)
  starting = $state(false)
  handle = $state<string | null>(null)       // Go-side process handle
  sessionId = $state<string | null>(null)    // the CLI's own session uuid (persisted transcript)
  mode = $state<PermissionMode>('plan')
  model = $state('default')
  effort = $state('')
  thinking = $state('')
  activeModel = $state('')                   // resolved model id the CLI reported
  info = $state<any>(null)                   // initialize response (commands, models, agents, account)
  init = $state<any>(null)                   // system/init message (tools, mcp servers, slash commands)
  todos = $state<Todo[]>([])
  context = $state<ContextUsage | null>(null)
  costUsd = $state(0)
  status = $state('')                        // transient line: compacting, retrying…
  unread = $state(false)
  // set when this tab was opened from History / forked and has no process yet
  private resume: { id: string; at?: string; fork?: boolean } | null = null
  private offs: (() => void)[] = []
  private tools = new Map<string, ToolItem>()
  private streamBlocks = new Map<number, TextItem | ThinkingItem | ToolItem>()
  private streamMsgId = ''
  private turnHasText = false
  private lastAssistantUuid: string | null = null
  private renderQueued = new Set<TextItem>()
  private renderRaf = 0
  private root: string
  onTitle?: () => void

  constructor(root: string, prefs: StartPrefs) {
    this.root = root
    this.mode = prefs.mode
    this.model = prefs.model
    this.effort = prefs.effort
    this.thinking = prefs.thinking
  }

  get projectRoot() { return this.root }
  get pendingAsks(): ToolItem[] {
    return this.items.filter((i): i is ToolItem => i.kind === 'tool' && i.ask?.state === 'pending')
  }
  get resumable() { return !!this.resume }

  /** the CLI's own commands (from the handshake / init), plus well-known fallbacks */
  get commands(): CommandDef[] {
    const seen = new Set<string>()
    const out: CommandDef[] = []
    const push = (c: CommandDef) => { if (!seen.has(c.name)) { seen.add(c.name); out.push(c) } }
    for (const c of this.info?.commands ?? []) push({ name: '/' + c.name, description: c.description ?? '', hint: c.argumentHint })
    for (const n of this.init?.slash_commands ?? []) push({ name: '/' + n, description: '' })
    SLASH_COMMANDS.filter(c => !c.terminalOnly).forEach(push)
    return out
  }
  get terminalOnly(): string[] {
    const live = new Set((this.info?.commands ?? []).map((c: any) => '/' + c.name))
    return SLASH_COMMANDS.filter(c => c.terminalOnly && !live.has(c.name)).map(c => c.name)
  }
  runCommand(_name: string, _args: string): boolean { return false }

  private add<T extends Item>(item: T): T {
    this.items.push(item)
    return this.items[this.items.length - 1] as T // the reactive proxy, not the raw object
  }
  note(text: string, tone: NoteItem['tone'] = 'info') {
    this.add({ kind: 'note', id: nid(), tone, text })
  }

  // ─── process lifecycle ───────────────────────────────────────────────────

  private async ensureProcess(): Promise<string> {
    if (this.handle) return this.handle
    this.starting = true
    try {
      const opts = {
        permissionMode: this.mode,
        model: this.model === 'default' ? '' : this.model,
        effort: this.effort,
        thinking: this.thinking,
        resume: this.resume?.id ?? '',
        resumeAt: this.resume?.at ?? '',
        fork: !!this.resume?.fork,
      }
      const h = await AssistantStartWithOptions(this.root, JSON.stringify(opts))
      this.resume = null
      this.handle = h
      this.offs.push(on(`assistant:msg:${h}`, (raw: string) => {
        let msg: any
        try { msg = JSON.parse(raw) } catch { return }
        this.ingest(msg, false)
      }))
      this.offs.push(on(`assistant:exit:${h}`, (stderr: string) => {
        this.detach()
        this.busy = false
        this.failPendingAsks()
        this.note(stripAnsi(stderr) || 'Claude exited unexpectedly.', 'error')
        // keep the conversation resumable: the next message respawns into it
        if (this.sessionId) this.resume = { id: this.sessionId }
      }))
      AssistantSessionInfo(h).then(s => { try { this.info = JSON.parse(s) } catch {} }).catch(() => {})
      return h
    } finally {
      this.starting = false
    }
  }

  private detach() {
    this.offs.forEach(o => o()); this.offs = []
    this.handle = null
  }

  private failPendingAsks() {
    for (const t of this.pendingAsks) t.ask!.state = 'cancelled'
  }

  /** Ends the process; the conversation stays visible and resumable. */
  stop() {
    if (this.handle) AssistantStop(this.handle)
    this.detach()
    this.busy = false
    this.failPendingAsks()
    if (this.sessionId) this.resume = { id: this.sessionId }
  }

  dispose() {
    if (this.handle) AssistantStop(this.handle)
    this.detach()
    cancelAnimationFrame(this.renderRaf)
  }

  /** Restart the process in place (same session) so spawn-time options apply. */
  async restart() {
    if (!this.handle) return
    const sid = this.sessionId
    if (this.busy) await this.interrupt()
    if (this.handle) AssistantStop(this.handle)
    this.detach()
    if (sid) this.resume = { id: sid }
  }

  // ─── user actions ────────────────────────────────────────────────────────

  async send(o: SendOptions) {
    const text = o.text.trim()
    if (!text) return
    if (this.items.filter(i => i.kind === 'user').length === 0 && this.title === 'New conversation') {
      this.title = text.replace(/\s+/g, ' ').slice(0, 60)
      this.onTitle?.()
    }
    this.add({
      kind: 'user', id: nid(), text, images: o.images, files: o.files,
      prevAssistantUuid: this.lastAssistantUuid,
    })
    this.busy = true
    this.turnHasText = false
    try {
      const h = await this.ensureProcess()
      const body = o.context + text
      if (o.images.length) await AssistantSendWithImages(h, body, o.images)
      else await AssistantSend(h, body)
    } catch (e) {
      this.note(String(e), 'error')
      this.busy = false
    }
  }

  async interrupt() {
    if (!this.handle) return
    try {
      await AssistantInterrupt(this.handle)
    } catch (e) {
      this.note(String(e), 'error')
    }
    this.busy = false
    this.failPendingAsks()
    this.status = ''
  }

  async setMode(mode: string) {
    const prev = this.mode
    this.mode = mode as PermissionMode
    if (!this.handle) return
    // bypass needs its opt-in flag at spawn time: respawn into the same session
    if (mode === 'bypassPermissions' && prev !== mode) { await this.restart(); return }
    try { await AssistantSwitchMode(this.handle, mode) }
    catch (e) { this.note(String(e), 'error') }
  }

  async setModel(model: string) {
    this.model = model
    if (!this.handle) return
    try {
      await AssistantControl(this.handle, 'set_model', JSON.stringify({ model: model === 'default' ? 'default' : model }))
      this.note(`Model set to ${model}.`)
    } catch (e) { this.note(String(e), 'error') }
  }

  /** effort / thinking are spawn-time flags: change and respawn into the same session. */
  async setSpawnOption(k: 'effort' | 'thinking', v: string) {
    this[k] = v
    if (this.handle) {
      await this.restart()
      this.note(`${k === 'effort' ? 'Effort' : 'Thinking'} set to ${v || 'default'} — applies from the next message.`)
    }
  }

  async respond(t: ToolItem, allow: boolean, opts: { message?: string; updatedInput?: any; suggestionIdx?: number[]; interrupt?: boolean } = {}) {
    if (!t.ask || t.ask.state !== 'pending' || !this.handle) return
    t.ask.state = allow ? 'allowed' : 'denied'
    if (!allow) { t.status = 'denied'; t.ask.note = opts.message }
    this.busy = !opts.interrupt
    try {
      await AssistantRespondPermissionEx(
        this.handle, t.ask.requestId, allow, opts.message ?? '',
        opts.updatedInput ? JSON.stringify(opts.updatedInput) : '',
        opts.suggestionIdx ?? [], !!opts.interrupt,
      )
    } catch (e) {
      this.note(String(e), 'error')
    }
  }

  async refreshContext() {
    if (!this.handle) return
    try {
      const r = JSON.parse(await AssistantControl(this.handle, 'get_context_usage', ''))
      if (r && typeof r.totalTokens === 'number') {
        this.context = {
          total: r.totalTokens, max: r.maxTokens, pct: r.percentage,
          categories: (r.categories ?? []).filter((c: any) => c.tokens > 0)
            .map((c: any) => ({ name: c.name, tokens: c.tokens, color: c.color })),
        }
      }
    } catch { /* older CLI without get_context_usage — keep the usage-based estimate */ }
  }

  async mcpStatus(): Promise<any[]> {
    if (!this.handle) return this.init?.mcp_servers ?? []
    try { return JSON.parse(await AssistantControl(this.handle, 'mcp_status', '')).mcpServers ?? [] }
    catch { return this.init?.mcp_servers ?? [] }
  }

  async mcpReconnect(name: string) {
    if (!this.handle) return
    try { await AssistantControl(this.handle, 'mcp_reconnect', JSON.stringify({ serverName: name })) }
    catch (e) { this.note(String(e), 'error') }
  }

  async mcpToggle(name: string, enabled: boolean) {
    if (!this.handle) return
    try { await AssistantControl(this.handle, 'mcp_toggle', JSON.stringify({ serverName: name, enabled })) }
    catch (e) { this.note(String(e), 'error') }
  }

  /** Files the CLI would restore for a rewind to before `u` (dry run). */
  async previewRewind(u: UserItem): Promise<{ canRewind: boolean; files: string[]; error?: string }> {
    if (!u.uuid) return { canRewind: false, files: [], error: 'No checkpoint for this message.' }
    if (!this.handle) await this.ensureProcess()
    try {
      const r = JSON.parse(await AssistantControl(this.handle!, 'rewind_files', JSON.stringify({ user_message_id: u.uuid, dry_run: true })))
      return { canRewind: r?.canRewind ?? true, files: r?.filesChanged ?? [], error: r?.error }
    } catch (e) {
      return { canRewind: false, files: [], error: String(e) }
    }
  }

  async rewindFiles(u: UserItem): Promise<boolean> {
    if (!u.uuid) return false
    if (!this.handle) await this.ensureProcess()
    try {
      const r = JSON.parse(await AssistantControl(this.handle!, 'rewind_files', JSON.stringify({ user_message_id: u.uuid })))
      if (r && r.canRewind === false) { this.note(r.error || 'Could not restore files.', 'error'); return false }
      for (const f of r?.filesChanged ?? []) pendingExternalReload.set(f)
      this.note(`Restored ${r?.filesChanged?.length ?? 0} file(s) to before that message.`)
      return true
    } catch (e) {
      this.note(String(e), 'error')
      return false
    }
  }

  /**
   * Drop `u` and everything after it, and continue from just before it in a
   * forked session (the original stays intact in History).
   */
  async rewindConversation(u: UserItem) {
    const idx = this.items.findIndex(i => i.id === u.id)
    if (idx < 0) return
    const sid = this.sessionId
    if (this.handle) { AssistantStop(this.handle); this.detach() }
    this.busy = false
    this.items.splice(idx)
    this.tools.clear()
    this.lastAssistantUuid = u.prevAssistantUuid ?? null
    this.resume = sid && u.prevAssistantUuid ? { id: sid, at: u.prevAssistantUuid, fork: true } : null
    if (!this.resume) { this.sessionId = null; this.costUsd = 0; this.context = null }
    this.todos = []
  }

  /** Re-render a past session from its transcript; the next message resumes it. */
  async loadTranscript(sessionId: string, title: string) {
    this.title = title || 'Conversation'
    this.sessionId = sessionId
    this.resume = { id: sessionId }
    const raw = await AssistantLoadTranscript(this.root, sessionId)
    const lines: any[] = JSON.parse(raw)
    for (const l of lines) this.ingest(l, true)
    this.busy = false
    this.flushRenders()
  }

  /** Clone `src` up to (not including) user item `u` into this conversation as a fork. */
  forkFrom(src: Conversation, u: UserItem) {
    const idx = src.items.findIndex(i => i.id === u.id)
    this.items = $state.snapshot(src.items.slice(0, idx)) as Item[]
    for (const it of this.items) if (it.kind === 'tool') { this.tools.set(it.toolUseId, it); if (it.ask?.state === 'pending') it.ask.state = 'cancelled' }
    this.title = src.title + ' (fork)'
    this.lastAssistantUuid = u.prevAssistantUuid ?? null
    if (src.sessionId && u.prevAssistantUuid) this.resume = { id: src.sessionId, at: u.prevAssistantUuid, fork: true }
  }

  // ─── stream reducer ──────────────────────────────────────────────────────

  private queueRender(t: TextItem) {
    this.renderQueued.add(t)
    if (this.renderRaf) return
    this.renderRaf = requestAnimationFrame(() => this.flushRenders())
  }
  private flushRenders() {
    this.renderRaf = 0
    for (const t of this.renderQueued) t.html = renderMarkdown(t.md)
    this.renderQueued.clear()
  }

  private newTool(id: string, name: string, input: any, status: ToolItem['status']): ToolItem {
    return { kind: 'tool', id: nid(), toolUseId: id, name, input, partialJson: '', status, children: [], childText: '' }
  }

  private onToolInput(t: ToolItem) {
    if (t.name === 'TodoWrite' && Array.isArray(t.input?.todos)) this.todos = t.input.todos
  }

  ingest(msg: any, replay: boolean) {
    switch (msg.type) {
      case 'stream_event': return this.onStreamEvent(msg)
      case 'assistant': return this.onAssistant(msg, replay)
      case 'user': return this.onUser(msg, replay)
      case 'system': return this.onSystem(msg, replay)
      case 'result': return this.onResult(msg)
      case 'permission_request': return this.onPermission(msg)
      case 'permission_cancel': {
        for (const t of this.pendingAsks) if (t.ask!.requestId === msg.request_id) t.ask!.state = 'cancelled'
        return
      }
      case 'tool_progress': {
        const t = this.tools.get(msg.tool_use_id)
        if (t && t.status === 'running') t.childText = `${Math.round(msg.elapsed_time_seconds ?? 0)}s…`
        return
      }
    }
  }

  private onStreamEvent(msg: any) {
    if (msg.parent_tool_use_id) return // subagent internals render from full messages
    const ev = msg.event
    switch (ev?.type) {
      case 'message_start':
        this.streamMsgId = ev.message?.id ?? ''
        this.streamBlocks.clear()
        break
      case 'content_block_start': {
        const cb = ev.content_block
        if (cb?.type === 'text') {
          this.streamBlocks.set(ev.index, this.add({ kind: 'text', id: nid(), md: cb.text ?? '', html: '', streaming: true, msgId: this.streamMsgId }))
        } else if (cb?.type === 'thinking') {
          this.streamBlocks.set(ev.index, this.add({ kind: 'thinking', id: nid(), text: cb.thinking ?? '', streaming: true, msgId: this.streamMsgId }))
        } else if (cb?.type === 'tool_use' || cb?.type === 'server_tool_use') {
          const t = this.add(this.newTool(cb.id, cb.name, {}, 'streaming'))
          this.tools.set(cb.id, t)
          this.streamBlocks.set(ev.index, t)
        }
        break
      }
      case 'content_block_delta': {
        const it = this.streamBlocks.get(ev.index)
        const d = ev.delta
        if (!it || !d) break
        if (it.kind === 'text' && d.type === 'text_delta') { it.md += d.text; this.turnHasText = true; this.queueRender(it) }
        else if (it.kind === 'thinking' && d.type === 'thinking_delta') it.text += d.thinking
        else if (it.kind === 'tool' && d.type === 'input_json_delta') it.partialJson += d.partial_json
        break
      }
      case 'content_block_stop': {
        const it = this.streamBlocks.get(ev.index)
        if (it && it.kind !== 'tool') it.streaming = false
        break
      }
    }
  }

  private onAssistant(msg: any, replay: boolean) {
    if (msg.uuid && !msg.parent_tool_use_id) this.lastAssistantUuid = msg.uuid
    const blocks: any[] = msg.message?.content ?? []
    if (msg.parent_tool_use_id) {
      // subagent activity: nest its tool calls under the Agent card
      const parent = this.tools.get(msg.parent_tool_use_id)
      if (!parent) return
      for (const b of blocks) {
        if (b.type === 'tool_use') {
          const c = this.newTool(b.id, b.name, b.input ?? {}, 'running')
          parent.children.push(c)
          this.tools.set(b.id, parent.children[parent.children.length - 1])
        } else if (b.type === 'text' && b.text) {
          parent.childText = b.text.slice(0, 300)
        }
      }
      return
    }
    const msgId = msg.message?.id
    for (const b of blocks) {
      if (b.type === 'text') {
        if (!b.text) continue
        this.turnHasText = true
        // the streamed copy of this block (msgId is cleared once finalized)
        const live = msgId ? this.items.find((i): i is TextItem => i.kind === 'text' && i.msgId === msgId) : undefined
        if (live) { live.md = b.text; live.streaming = false; live.html = renderMarkdown(b.text); live.msgId = undefined }
        else this.add({ kind: 'text', id: nid(), md: b.text, html: renderMarkdown(b.text), streaming: false })
      } else if (b.type === 'thinking') {
        if (!b.thinking) continue
        const live = msgId ? this.items.find((i): i is ThinkingItem => i.kind === 'thinking' && i.msgId === msgId) : undefined
        if (live) { live.text = b.thinking; live.streaming = false; live.msgId = undefined }
        else this.add({ kind: 'thinking', id: nid(), text: b.thinking, streaming: false })
      } else if (b.type === 'tool_use' || b.type === 'server_tool_use') {
        let t = this.tools.get(b.id)
        if (!t) { t = this.add(this.newTool(b.id, b.name, b.input ?? {}, 'running')); this.tools.set(b.id, t) }
        t.input = b.input ?? {}
        t.partialJson = ''
        if (t.status === 'streaming') t.status = 'running'
        this.onToolInput(t)
      }
    }
    if (msg.error && !replay) this.note(`API error: ${msg.error}`, 'error')
  }

  private onUser(msg: any, replay: boolean) {
    const content = msg.message?.content
    if (Array.isArray(content)) {
      let hadResult = false
      for (const b of content) {
        if (b?.type !== 'tool_result') continue
        hadResult = true
        const t = this.tools.get(b.tool_use_id)
        if (!t) continue
        t.result = { text: resultText(b.content), isError: !!b.is_error }
        if (t.status !== 'denied') t.status = b.is_error ? 'error' : 'done'
        if (!b.is_error && FILE_EDIT_TOOLS.has(t.name) && !replay) {
          const p = t.input?.file_path ?? t.input?.notebook_path ?? t.input?.path
          if (p) pendingExternalReload.set(p)
        }
      }
      if (hadResult) return
    }
    if (msg.parent_tool_use_id || msg.isMeta || msg.isSynthetic || msg.isCompactSummary) return
    const text = typeof content === 'string' ? content
      : Array.isArray(content) ? content.filter((b: any) => b?.type === 'text').map((b: any) => b.text).join('\n') : ''
    // harness wrappers: slash-command echo and its local output
    const out = /<local-command-stdout>([\s\S]*?)<\/local-command-stdout>/.exec(text)
    if (out) { if (out[1].trim()) this.note(stripAnsi(out[1].trim()), 'command'); return }
    const cmd = /<command-name>([\s\S]*?)<\/command-name>/.exec(text)
    if (!replay) {
      // live echo of our own prompt (--replay-user-messages): capture its uuid,
      // the checkpoint key for rewind
      if (msg.isReplay && msg.uuid) {
        const u = [...this.items].reverse().find((i): i is UserItem => i.kind === 'user' && !i.uuid)
        if (u) u.uuid = msg.uuid
      }
      return
    }
    if (cmd) {
      const args = /<command-args>([\s\S]*?)<\/command-args>/.exec(text)?.[1] ?? ''
      this.add({ kind: 'user', id: nid(), text: `${cmd[1].trim()} ${args}`.trim(), images: [], files: [], uuid: msg.uuid, prevAssistantUuid: this.lastAssistantUuid })
      return
    }
    if (!text.trim() || text.trimStart().startsWith('<')) return
    // the panel prefixes editor context before a rule — show only the ask
    const sep = text.lastIndexOf('\n\n---\n\n')
    const shown = sep >= 0 && /^(Active file|Selected|Attached|Selection)/.test(text) ? text.slice(sep + 7) : text
    const images = Array.isArray(content) ? content.filter((b: any) => b?.type === 'image').map(() => 'image') : []
    this.add({ kind: 'user', id: nid(), text: shown, images, files: [], uuid: msg.uuid, prevAssistantUuid: this.lastAssistantUuid })
  }

  private onSystem(msg: any, replay: boolean) {
    switch (msg.subtype) {
      case 'init':
        this.init = msg
        if (msg.session_id) this.sessionId = msg.session_id
        if (msg.model) this.activeModel = msg.model
        break
      case 'compact_boundary': {
        const pre = msg.compact_metadata?.pre_tokens ?? msg.compactMetadata?.preTokens
        this.note(`Conversation compacted${pre ? ` (was ${Math.round(pre / 1000)}k tokens)` : ''}.`, 'compact')
        this.status = ''
        break
      }
      case 'status':
        if (!replay) this.status = msg.status === 'compacting' ? 'Compacting conversation…' : ''
        break
      case 'api_retry':
        if (!replay) this.status = `API error ${msg.error_status ?? ''} — retrying (${msg.attempt}/${msg.max_retries})…`
        break
      case 'local_command_output':
        if (msg.content) this.note(stripAnsi(msg.content), 'command')
        break
      case 'local_command':
        if (replay && typeof msg.content === 'string') {
          const out = /<local-command-stdout>([\s\S]*?)<\/local-command-stdout>/.exec(msg.content)
          if (out?.[1]?.trim()) this.note(stripAnsi(out[1].trim()), 'command')
        }
        break
    }
  }

  private onResult(msg: any) {
    this.busy = false
    this.status = ''
    for (const t of this.tools.values()) if (t.status === 'running' || t.status === 'streaming') t.status = 'done'
    if (typeof msg.total_cost_usd === 'number') this.costUsd = msg.total_cost_usd
    const isError = !!msg.is_error || (typeof msg.subtype === 'string' && msg.subtype.startsWith('error'))
    if (isError) {
      const why = msg.result || (Array.isArray(msg.errors) ? msg.errors.join('\n') : '') || msg.subtype || 'Claude hit an error.'
      this.note(stripAnsi(String(why)), 'error')
    } else if (!this.turnHasText && typeof msg.result === 'string' && msg.result.trim()) {
      // slash commands (/cost, /context, …) answer with only a result line
      this.add({ kind: 'text', id: nid(), md: msg.result, html: renderMarkdown(msg.result), streaming: false })
    }
    this.turnHasText = false
    this.add({ kind: 'result', id: nid(), durationMs: msg.duration_ms ?? 0, costUsd: msg.total_cost_usd ?? 0, turns: msg.num_turns ?? 0, isError })
    if (msg.session_id) this.sessionId = msg.session_id
    this.unread = true
    this.refreshContext()
  }

  private onPermission(msg: any) {
    const ask: Ask = {
      requestId: msg.request_id,
      suggestions: Array.isArray(msg.suggestions) ? msg.suggestions : [],
      reason: stripAnsi(msg.decision_reason ?? ''),
      reasonType: msg.decision_reason_type ?? '',
      blockedPath: msg.blocked_path ?? '',
      suppressAlways: !!msg.suppress_always,
      defaultToNo: !!msg.default_to_no,
      title: stripAnsi(msg.title ?? ''),
      state: 'pending',
    }
    let t = msg.tool_use_id ? this.tools.get(msg.tool_use_id) : undefined
    if (!t) {
      t = this.add(this.newTool(msg.tool_use_id || msg.request_id, msg.tool_name, msg.input ?? {}, 'running'))
      if (msg.tool_use_id) this.tools.set(msg.tool_use_id, t)
    }
    if (msg.input && Object.keys(t.input ?? {}).length === 0) t.input = msg.input
    t.ask = ask
    this.unread = true
  }
}
