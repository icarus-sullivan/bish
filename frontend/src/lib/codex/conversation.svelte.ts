// One Codex conversation (one Codex panel tab): owns a `codex app-server`
// process and maps its JSON-RPC thread/turn/item events onto the same item
// model the Claude panel renders, so ChatView/ToolCard serve both.
import { on, CodexStart, CodexCall, CodexRespond, CodexStop } from '../wails'
import { pendingExternalReload } from '../stores'
import { renderMarkdown, stripAnsi, basename } from '../claude/format'
import type {
  Item, ToolItem, UserItem, TextItem, ThinkingItem, NoteItem, Todo, ContextUsage, SendOptions, Ask,
} from '../claude/conversation.svelte'
import type { AgentCaps, AgentConversation, CommandDef, ModeDef } from '../claude/agent'

export const CODEX_MODES: (ModeDef & { sandbox: string; approval: string; policy: any })[] = [
  { id: 'read-only', label: 'Read only', desc: 'Reads files and answers; asks before any edit or command',
    sandbox: 'read-only', approval: 'on-request', policy: { type: 'readOnly' } },
  { id: 'auto', label: 'Auto', desc: 'Edits and runs commands inside the workspace; asks to go beyond it',
    sandbox: 'workspace-write', approval: 'on-request', policy: { type: 'workspaceWrite' } },
  { id: 'full-access', label: 'Full access', desc: 'No sandbox and no approvals — dangerous', danger: true,
    sandbox: 'danger-full-access', approval: 'never', policy: { type: 'dangerFullAccess' } },
]

export interface CodexPrefs { mode: string; model: string; effort: string }

let keySeq = 0
let itemSeq = 0
const nid = () => 'k' + (itemSeq++).toString(36)

export class CodexConversation implements AgentConversation {
  readonly key = 'x' + (keySeq++)
  readonly caps: AgentCaps = { provider: 'codex', editRewind: true, restoreCode: false, fork: false, contextBreakdown: false }
  readonly modes = CODEX_MODES
  readonly modeCycle = ['read-only', 'auto']
  readonly terminalOnly = ['/login', '/logout', '/approvals', '/status']
  title = $state('New conversation')
  items = $state<Item[]>([])
  busy = $state(false)
  starting = $state(false)
  handle = $state<string | null>(null)
  sessionId = $state<string | null>(null) // Codex thread id
  mode = $state('auto')
  model = $state('default')
  effort = $state('')
  activeModel = $state('')
  info = $state<any>(null)
  todos = $state<Todo[]>([])
  context = $state<ContextUsage | null>(null)
  costUsd = $state(0)
  status = $state('')
  unread = $state(false)
  onTitle?: () => void

  private root: string
  private turnId: string | null = null
  private resumeId: string | null = null
  private threadReady = false
  private offs: (() => void)[] = []
  private byItem = new Map<string, Item>()
  private renderQueued = new Set<TextItem>()
  private renderRaf = 0
  private turnStartedAt = 0

  constructor(root: string, prefs: CodexPrefs) {
    this.root = root
    this.mode = prefs.mode
    this.model = prefs.model
    this.effort = prefs.effort
  }

  get projectRoot() { return this.root }
  get pendingAsks(): ToolItem[] {
    return this.items.filter((i): i is ToolItem => i.kind === 'tool' && i.ask?.state === 'pending')
  }
  get resumable() { return !!this.resumeId }
  get commands(): CommandDef[] {
    return [{ name: '/compact', description: 'Summarize the conversation to free up context' }]
  }

  private add<T extends Item>(item: T): T {
    this.items.push(item)
    return this.items[this.items.length - 1] as T
  }
  note(text: string, tone: NoteItem['tone'] = 'info') {
    this.add({ kind: 'note', id: nid(), tone, text })
  }

  // ─── process / thread lifecycle ──────────────────────────────────────────

  async ensureProcess(): Promise<string> {
    if (this.handle) return this.handle
    this.starting = true
    try {
      const h = await CodexStart(this.root)
      this.handle = h
      this.offs.push(on(`codex:msg:${h}`, (raw: string) => {
        let msg: any
        try { msg = JSON.parse(raw) } catch { return }
        this.ingest(msg)
      }))
      this.offs.push(on(`codex:exit:${h}`, (stderr: string) => {
        this.detach()
        this.busy = false
        this.cancelAsks()
        this.note(stripAnsi(stderr) || 'Codex exited unexpectedly.', 'error')
      }))
      this.loadModels(h)
      return h
    } finally {
      this.starting = false
    }
  }

  private async loadModels(h: string) {
    try {
      const r = JSON.parse(await CodexCall(h, 'model/list', '{}'))
      const models = (r?.data ?? []).filter((m: any) => !m.hidden).map((m: any) => ({
        value: m.model ?? m.id, displayName: m.displayName ?? m.model, description: m.description ?? '',
        efforts: (m.supportedReasoningEfforts ?? []).map((e: any) => e.reasoningEffort),
        isDefault: !!m.isDefault,
      }))
      this.info = { ...(this.info ?? {}), models }
    } catch { /* older app-server without model/list */ }
  }

  private modeDef() { return CODEX_MODES.find(m => m.id === this.mode) ?? CODEX_MODES[1] }

  private async ensureThread(): Promise<string> {
    const h = await this.ensureProcess()
    if (this.threadReady && this.sessionId) return this.sessionId
    const m = this.modeDef()
    const base: any = { sandbox: m.sandbox, approvalPolicy: m.approval }
    if (this.model !== 'default') base.model = this.model
    let r: any
    if (this.resumeId) {
      r = JSON.parse(await CodexCall(h, 'thread/resume', JSON.stringify({ ...base, threadId: this.resumeId })))
      this.resumeId = null
    } else {
      r = JSON.parse(await CodexCall(h, 'thread/start', JSON.stringify(base)))
    }
    this.sessionId = r?.thread?.id ?? this.sessionId
    if (r?.model) this.activeModel = r.model
    this.threadReady = true
    this.onTitle?.()
    return this.sessionId!
  }

  private detach() {
    this.offs.forEach(o => o()); this.offs = []
    this.handle = null
    this.threadReady = false
    this.turnId = null
    if (this.sessionId) this.resumeId = this.sessionId
  }

  private cancelAsks() { for (const t of this.pendingAsks) t.ask!.state = 'cancelled' }

  stop() {
    if (this.handle) CodexStop(this.handle)
    this.detach()
    this.busy = false
    this.cancelAsks()
  }

  dispose() {
    if (this.handle) CodexStop(this.handle)
    this.offs.forEach(o => o()); this.offs = []
    this.handle = null
    cancelAnimationFrame(this.renderRaf)
  }

  // ─── user actions ────────────────────────────────────────────────────────

  async send(o: SendOptions) {
    const text = o.text.trim()
    if (!text) return
    if (!this.items.some(i => i.kind === 'user') && this.title === 'New conversation') {
      this.title = text.replace(/\s+/g, ' ').slice(0, 60)
      this.onTitle?.()
    }
    const u = this.add<UserItem>({ kind: 'user', id: nid(), text, images: o.images, files: o.files })
    const steering = this.busy && !!this.turnId
    this.busy = true
    try {
      const thread = await this.ensureThread()
      const input: any[] = [{ type: 'text', text: o.context + text }]
      for (const p of o.images) input.push({ type: 'localImage', path: p })
      if (steering) {
        await CodexCall(this.handle!, 'turn/steer', JSON.stringify({ threadId: thread, input, expectedTurnId: this.turnId }))
        u.turnId = this.turnId ?? undefined
        return
      }
      const m = this.modeDef()
      const params: any = { threadId: thread, input, approvalPolicy: m.approval, sandboxPolicy: m.policy }
      if (this.model !== 'default') params.model = this.model
      if (this.effort) params.effort = this.effort
      const r = JSON.parse(await CodexCall(this.handle!, 'turn/start', JSON.stringify(params)))
      if (r?.turn?.id) { this.turnId = r.turn.id; u.turnId = r.turn.id }
      this.turnStartedAt = Date.now()
    } catch (e) {
      this.note(String(e), 'error')
      this.busy = false
    }
  }

  async interrupt() {
    if (!this.handle || !this.sessionId || !this.turnId) { this.busy = false; return }
    try {
      await CodexCall(this.handle, 'turn/interrupt', JSON.stringify({ threadId: this.sessionId, turnId: this.turnId }))
    } catch (e) {
      this.note(String(e), 'error')
    }
    this.cancelAsks()
  }

  async setMode(mode: string) {
    this.mode = mode
    if (this.handle) this.note(`Mode set to ${this.modeDef().label} — applies from the next message.`)
  }

  async setModel(model: string) {
    this.model = model
    if (this.handle) this.note(`Model set to ${model} — applies from the next message.`)
  }

  async setSpawnOption(k: 'effort' | 'thinking', v: string) {
    if (k === 'effort') this.effort = v
  }

  async respond(t: ToolItem, allow: boolean, opts: { message?: string; updatedInput?: any; suggestionIdx?: number[]; interrupt?: boolean } = {}) {
    if (!t.ask || t.ask.state !== 'pending' || !this.handle) return
    t.ask.state = allow ? 'allowed' : 'denied'
    let result: any
    if (t.name === 'AskUserQuestion') {
      const answers: Record<string, { answers: string[] }> = {}
      if (allow) {
        for (const q of t.input?.questions ?? []) {
          const a: string = opts.updatedInput?.answers?.[q.question] ?? ''
          answers[q._id] = { answers: a ? a.split(', ').filter(Boolean) : [] }
        }
      }
      result = { answers }
    } else {
      result = { decision: allow ? (opts.suggestionIdx?.length ? 'acceptForSession' : 'accept') : (opts.interrupt ? 'cancel' : 'decline') }
      if (!allow) { t.status = 'denied'; t.ask.note = opts.message }
    }
    try {
      await CodexRespond(this.handle, t.ask.requestId, JSON.stringify(result))
    } catch (e) {
      this.note(String(e), 'error')
    }
    // Codex declines carry no message — send the user's note as a follow-up
    if (!allow && opts.message && !opts.interrupt) this.send({ text: opts.message, context: '', images: [], files: [] })
  }

  async refreshContext() { /* pushed via thread/tokenUsage/updated */ }

  async previewRewind() { return { canRewind: false, files: [], error: 'Codex has no file checkpoints.' } }
  async rewindFiles() { return false }

  async rewindConversation(u: UserItem) {
    const idx = this.items.findIndex(i => i.id === u.id)
    if (idx < 0) return
    if (this.busy) await this.interrupt()
    if (this.sessionId && u.turnId) {
      try {
        await this.ensureProcess()
        if (!this.threadReady) await this.ensureThread()
        await CodexCall(this.handle!, 'thread/revert', JSON.stringify({ threadId: this.sessionId, beforeTurnId: u.turnId }))
      } catch (e) {
        this.note(`Couldn't rewind: ${e}`, 'error')
        return
      }
    }
    this.items.splice(idx)
    this.busy = false
  }

  async mcpStatus(): Promise<any[]> {
    try {
      const h = await this.ensureProcess()
      const r = JSON.parse(await CodexCall(h, 'mcpServerStatus/list', JSON.stringify(this.sessionId ? { threadId: this.sessionId } : {})))
      return (r?.data ?? []).map((s: any) => {
        const rs = s.runtimeStatus
        const status = typeof rs === 'string' ? rs : rs?.status ?? rs?.type ?? (s.toolsError ? 'failed' : 'connected')
        return { name: s.name, status: String(status).replace(/^./, (c: string) => c.toLowerCase()), error: s.toolsError, serverInfo: s.serverInfo, readOnly: true }
      })
    } catch (e) {
      this.note(String(e), 'error')
      return []
    }
  }
  async mcpReconnect() { this.note('Reconnecting MCP servers is not supported for Codex yet.') }
  async mcpToggle() { this.note('Enable/disable MCP servers in ~/.codex/config.toml for Codex.') }

  runCommand(name: string): boolean {
    if (name === '/compact') {
      if (!this.handle || !this.sessionId) { this.note('Nothing to compact yet.'); return true }
      this.status = 'Compacting conversation…'
      CodexCall(this.handle, 'thread/compact/start', JSON.stringify({ threadId: this.sessionId }))
        .catch(e => { this.status = ''; this.note(String(e), 'error') })
      return true
    }
    return false
  }

  /** Re-render a past thread; the next message resumes it. */
  async loadThread(threadId: string, title: string) {
    this.title = title || 'Conversation'
    this.sessionId = threadId
    this.resumeId = threadId
    const h = await this.ensureProcess()
    const r = JSON.parse(await CodexCall(h, 'thread/read', JSON.stringify({ threadId, includeTurns: true })))
    for (const turn of r?.thread?.turns ?? []) {
      for (const it of turn.items ?? []) {
        if (it.type === 'userMessage') {
          const text = (it.content ?? []).filter((c: any) => c.type === 'text').map((c: any) => c.text).join('\n')
          const sep = text.lastIndexOf('\n\n---\n\n')
          const shown = sep >= 0 && /^(Active file|Selected|Attached)/.test(text) ? text.slice(sep + 7) : text
          this.add({ kind: 'user', id: nid(), text: shown, images: [], files: [], turnId: turn.id })
        } else {
          this.itemStarted(it, true)
          this.itemCompleted(it, true)
        }
      }
      if (turn.status === 'failed' && turn.error?.message) this.note(turn.error.message, 'error')
    }
    this.flushRenders()
  }

  // ─── event reducer ───────────────────────────────────────────────────────

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

  private tool(id: string, name: string, input: any): ToolItem {
    const t = this.add<ToolItem>({
      kind: 'tool', id: nid(), toolUseId: id, name, input, partialJson: '', status: 'running', children: [], childText: '',
    })
    this.byItem.set(id, t)
    return t
  }

  private itemStarted(it: any, replay = false) {
    if (!it?.id || this.byItem.has(it.id)) return
    switch (it.type) {
      case 'agentMessage':
      case 'plan': {
        const t = this.add<TextItem>({ kind: 'text', id: nid(), md: it.text ?? '', html: '', streaming: !replay })
        this.byItem.set(it.id, t)
        if (t.md) this.queueRender(t)
        break
      }
      case 'reasoning': {
        const th = this.add<ThinkingItem>({ kind: 'thinking', id: nid(), text: (it.summary ?? []).join('\n\n'), streaming: !replay })
        this.byItem.set(it.id, th)
        break
      }
      case 'commandExecution':
        this.tool(it.id, 'Bash', { command: it.command, cwd: it.cwd })
        break
      case 'fileChange':
        this.tool(it.id, 'apply_patch', { changes: it.changes ?? [] })
        break
      case 'mcpToolCall':
        this.tool(it.id, `mcp__${it.server}__${it.tool}`, it.arguments ?? {})
        break
      case 'dynamicToolCall':
        this.tool(it.id, it.tool ?? 'tool', it.arguments ?? {})
        break
      case 'webSearch':
        this.tool(it.id, 'WebSearch', { query: it.query ?? it.action?.query ?? '' })
        break
      case 'collabAgentToolCall':
        this.tool(it.id, 'Agent', { description: it.prompt ?? '', subagent_type: it.model ?? '' })
        break
      case 'imageView':
        this.note(`Viewed image ${basename(it.path ?? '')}`)
        this.byItem.set(it.id, this.items[this.items.length - 1])
        break
      case 'enteredReviewMode':
        this.note('Entered review mode.')
        this.byItem.set(it.id, this.items[this.items.length - 1])
        break
    }
  }

  private itemCompleted(it: any, replay = false) {
    if (!it?.id) return
    if (!this.byItem.has(it.id)) this.itemStarted(it, replay)
    const cur = this.byItem.get(it.id)
    switch (it.type) {
      case 'agentMessage':
      case 'plan': {
        const t = cur as TextItem
        if (typeof it.text === 'string') t.md = it.text
        t.streaming = false
        t.html = renderMarkdown(t.md)
        break
      }
      case 'reasoning': {
        const th = cur as ThinkingItem
        const full = (it.summary ?? []).join('\n\n') || (it.content ?? []).join('\n\n')
        if (full) th.text = full
        th.streaming = false
        if (!th.text) this.items.splice(this.items.indexOf(th), 1)
        break
      }
      case 'commandExecution': {
        const t = cur as ToolItem
        const out = stripAnsi(it.aggregatedOutput ?? t.result?.text ?? '')
        const failed = it.status === 'failed' || it.status === 'declined' || (typeof it.exitCode === 'number' && it.exitCode !== 0)
        t.result = { text: out + (typeof it.exitCode === 'number' && it.exitCode !== 0 ? `\n[exit ${it.exitCode}]` : ''), isError: failed }
        if (t.status !== 'denied') t.status = it.status === 'declined' ? 'denied' : failed ? 'error' : 'done'
        break
      }
      case 'fileChange': {
        const t = cur as ToolItem
        t.input = { changes: it.changes ?? t.input?.changes ?? [] }
        const ok = it.status === 'completed'
        if (t.status !== 'denied') t.status = it.status === 'declined' ? 'denied' : ok ? 'done' : it.status === 'failed' ? 'error' : 'done'
        if (it.status === 'failed') t.result = { text: 'Patch failed to apply.', isError: true }
        if (ok && !replay) for (const c of t.input.changes) if (c.path) pendingExternalReload.set(c.path.startsWith('/') ? c.path : this.root + '/' + c.path)
        break
      }
      case 'mcpToolCall': {
        const t = cur as ToolItem
        if (it.error) { t.result = { text: it.error.message ?? JSON.stringify(it.error), isError: true }; t.status = 'error' }
        else {
          const content = it.result?.content ?? []
          t.result = { text: content.map((c: any) => c.text ?? (c.type === 'image' ? '[image]' : '')).filter(Boolean).join('\n'), isError: false }
          t.status = 'done'
        }
        break
      }
      case 'dynamicToolCall': case 'webSearch': case 'collabAgentToolCall': {
        const t = cur as ToolItem
        t.status = it.success === false || it.status === 'failed' ? 'error' : 'done'
        break
      }
      case 'contextCompaction':
        this.note('Conversation compacted.', 'compact')
        this.status = ''
        break
      case 'exitedReviewMode':
        if (it.review) this.add({ kind: 'text', id: nid(), md: it.review, html: renderMarkdown(it.review), streaming: false })
        break
    }
  }

  private ask(requestId: any, reason = ''): Ask {
    return {
      requestId: JSON.stringify(requestId), suggestions: [], reason: stripAnsi(reason), reasonType: '', blockedPath: '',
      suppressAlways: false, defaultToNo: false, title: '', state: 'pending',
    }
  }

  ingest(msg: any) {
    const p = msg.params ?? {}
    if (msg.id !== undefined && msg.method) return this.onRequest(msg.id, msg.method, p)
    switch (msg.method) {
      case 'turn/started':
        this.turnId = p.turn?.id ?? this.turnId
        this.busy = true
        break
      case 'item/started': this.itemStarted(p.item); break
      case 'item/completed': this.itemCompleted(p.item); break
      case 'item/agentMessage/delta':
      case 'item/plan/delta': {
        const t = this.byItem.get(p.itemId) as TextItem | undefined
        if (t?.kind === 'text') { t.md += p.delta ?? ''; this.queueRender(t) }
        break
      }
      case 'item/reasoning/summaryTextDelta':
      case 'item/reasoning/textDelta': {
        const th = this.byItem.get(p.itemId) as ThinkingItem | undefined
        if (th?.kind === 'thinking') th.text += p.delta ?? ''
        break
      }
      case 'item/reasoning/summaryPartAdded': {
        const th = this.byItem.get(p.itemId) as ThinkingItem | undefined
        if (th?.kind === 'thinking' && th.text) th.text += '\n\n'
        break
      }
      case 'item/commandExecution/outputDelta': {
        const t = this.byItem.get(p.itemId) as ToolItem | undefined
        if (t?.kind === 'tool') t.result = { text: (t.result?.text ?? '') + stripAnsi(p.delta ?? ''), isError: false }
        break
      }
      case 'item/fileChange/patchUpdated': {
        const t = this.byItem.get(p.itemId) as ToolItem | undefined
        if (t?.kind === 'tool' && Array.isArray(p.changes)) t.input = { changes: p.changes }
        break
      }
      case 'turn/plan/updated':
        this.todos = (p.plan ?? []).map((s: any) => ({
          content: s.step, status: s.status === 'inProgress' ? 'in_progress' : s.status === 'completed' ? 'completed' : 'pending',
        }))
        break
      case 'thread/tokenUsage/updated': {
        const u = p.tokenUsage
        const max = u?.modelContextWindow
        const used = u?.last?.totalTokens ?? 0
        if (max) this.context = { total: used, max, pct: (used / max) * 100, categories: [] }
        break
      }
      case 'turn/completed': {
        const turn = p.turn ?? {}
        this.busy = false
        this.status = ''
        this.turnId = null
        this.cancelAsks()
        for (const it of this.byItem.values()) if (it.kind === 'tool' && (it.status === 'running' || it.status === 'streaming')) it.status = 'done'
        if (turn.status === 'failed') this.note(turn.error?.message ?? 'Codex hit an error.', 'error')
        else if (turn.status === 'interrupted') this.note('Interrupted.')
        this.add({ kind: 'result', id: nid(), durationMs: turn.durationMs ?? (this.turnStartedAt ? Date.now() - this.turnStartedAt : 0), costUsd: 0, turns: 0, isError: turn.status === 'failed' })
        this.unread = true
        break
      }
      case 'error':
        if (p.willRetry) this.status = `${p.error?.message ?? 'Error'} — retrying…`
        else this.note(p.error?.message ?? 'Codex error.', 'error')
        break
      case 'thread/name/updated':
        if (p.threadName || p.name) { this.title = p.threadName ?? p.name; this.onTitle?.() }
        break
      case 'thread/compacted':
        this.status = ''
        break
      case 'warning': case 'configWarning': case 'deprecationNotice':
        if (p.message) this.note(stripAnsi(p.message))
        break
      case 'model/rerouted':
        if (p.toModel ?? p.model) this.activeModel = p.toModel ?? p.model
        break
      case 'serverRequest/resolved': {
        const rid = JSON.stringify(p.requestId)
        for (const t of this.pendingAsks) if (t.ask!.requestId === rid) t.ask!.state = 'cancelled'
        break
      }
    }
  }

  private onRequest(id: any, method: string, p: any) {
    this.unread = true
    if (method === 'item/commandExecution/requestApproval') {
      let t = this.byItem.get(p.itemId) as ToolItem | undefined
      if (t?.kind !== 'tool') t = this.tool(p.itemId ?? String(id), 'Bash', { command: p.command ?? '', cwd: p.cwd })
      if (p.command && !t.input?.command) t.input = { ...t.input, command: p.command }
      const a = this.ask(id, p.reason ?? '')
      const decisions: any[] | undefined = p.availableDecisions
      if (!decisions || decisions.includes('acceptForSession')) a.suggestions = [{ label: "don't ask again for this command this session" }]
      a.title = 'Run this command?'
      t.ask = a
    } else if (method === 'item/fileChange/requestApproval') {
      let t = this.byItem.get(p.itemId) as ToolItem | undefined
      if (t?.kind !== 'tool') t = this.tool(p.itemId ?? String(id), 'apply_patch', { changes: [] })
      const a = this.ask(id, p.reason ?? '')
      a.suggestions = [{ label: "don't ask again for edits this session" }]
      a.title = 'Apply these changes?'
      if (p.grantRoot) a.blockedPath = p.grantRoot
      t.ask = a
    } else if (method === 'item/tool/requestUserInput') {
      const questions = (p.questions ?? []).map((q: any) => ({
        _id: q.id, question: q.question, header: q.header, multiSelect: false,
        options: (q.options ?? []).map((o: any) => ({ label: o.label, description: o.description })),
      }))
      const t = this.tool(`ask-${JSON.stringify(id)}`, 'AskUserQuestion', { questions })
      t.ask = this.ask(id)
    }
  }
}
