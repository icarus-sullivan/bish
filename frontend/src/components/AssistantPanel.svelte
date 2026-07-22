<script lang="ts">
  import { get } from 'svelte/store'
  import { marked } from 'marked'
  import DOMPurify from 'dompurify'
  import {
    IconSparkles, IconPlus, IconPlayerStop, IconPlayerStopFilled, IconSendFilled, IconX, IconCheck, IconCode, IconSlash,
  } from '@tabler/icons-svelte'
  import {
    on, AssistantStart, AssistantSend, AssistantApprovePlan, AssistantStop, AssistantInterrupt, AssistantSwitchMode,
    AssistantPickFiles, StashDropped,
  } from '../lib/wails'
  import {
    projectRoot, cwd, tabs, activeTabId, activeSelection, pendingGoto, openFileTab,
  } from '../lib/stores'

  const PERMISSION_MODES = ['plan', 'acceptEdits', 'auto', 'bypassPermissions', 'manual', 'dontAsk']
  const MODE_LABELS: Record<string, string> = {
    plan: 'Plan first',
    acceptEdits: 'Edit automatically',
    auto: 'Auto',
    bypassPermissions: 'Full access',
    manual: 'Manual approval',
    dontAsk: "Don't ask",
  }
  const PERM_KEY = 'bish.assistant.permissionMode'

  interface ChatMsg {
    id: string
    role: 'user' | 'assistant' | 'tool' | 'plan' | 'status' | 'error'
    text?: string
    html?: string
    toolName?: string
    toolPath?: string
    planDone?: 'approved' | 'rejected'
  }

  let messages = $state<ChatMsg[]>([])
  let input = $state('')
  let attachedFiles = $state<string[]>([])
  let includeContext = $state(true)
  let busy = $state(false)
  let planPending = $state(false)
  // least-permissive by default every fresh session; only sticky if the user raised it
  let permissionMode = $state(localStorage.getItem(PERM_KEY) ?? 'plan')

  let sessionId = $state<string | null>(null)
  let offMsg: (() => void) | null = null
  let offExit: (() => void) | null = null
  let seq = 0
  const nextId = () => 'm' + seq++

  // click the pill, or Shift+Tab in the composer — same as claude CLI's own cycle keybind.
  // Permission mode is fixed at process spawn time, so if a session is already
  // running this has to kill + --resume it in the new mode or the live process
  // just keeps enforcing whatever mode it started with.
  async function cycleMode() {
    const i = PERMISSION_MODES.indexOf(permissionMode)
    const next = PERMISSION_MODES[(i + 1) % PERMISSION_MODES.length]
    permissionMode = next
    localStorage.setItem(PERM_KEY, next)
    if (!sessionId) return
    busy = false
    planPending = false
    try {
      await AssistantSwitchMode(sessionId, next)
      messages.push({ id: nextId(), role: 'status', text: `Switched to "${MODE_LABELS[next]}".` })
    } catch (e) {
      messages.push({ id: nextId(), role: 'error', text: `${e}` })
    }
  }

  async function renderMd(md: string): Promise<string> {
    return DOMPurify.sanitize(await marked.parse(md))
  }

  // subprocess only ever spawns from here — never on mount, never eagerly
  async function ensureSession(): Promise<string> {
    if (sessionId) return sessionId
    const root = get(projectRoot) || get(cwd)
    const id = await AssistantStart(root, permissionMode)
    sessionId = id
    offMsg = on(`assistant:msg:${id}`, handleLine)
    offExit = on(`assistant:exit:${id}`, (stderr: string) => {
      busy = false
      planPending = false
      sessionId = null // let the next send() spawn a fresh process
      offMsg?.(); offExit?.()
      offMsg = offExit = null
      messages.push({
        id: nextId(), role: 'error',
        text: stderr || 'Assistant process exited unexpectedly.',
      })
    })
    return id
  }

  function activeFile(): { path: string } | null {
    const t = get(tabs).find(t => t.id === get(activeTabId))
    return t && t.type === 'file' && t.path && t.path !== '__new__' ? { path: t.path } : null
  }

  function buildContext(): string {
    if (!includeContext && attachedFiles.length === 0) return ''
    const parts: string[] = []
    if (includeContext) {
      const sel = get(activeSelection)
      const file = activeFile()
      if (file) parts.push(`Active file: ${file.path}`)
      if (sel && sel.text) parts.push(`Selected text (${sel.path}:${sel.line}):\n\`\`\`\n${sel.text}\n\`\`\``)
    }
    if (attachedFiles.length) parts.push(`Attached files:\n${attachedFiles.map(p => '- ' + p).join('\n')}`)
    return parts.length ? parts.join('\n\n') + '\n\n---\n\n' : ''
  }

  async function send() {
    const text = input.trim()
    if (!text || busy) return
    const ctx = buildContext()
    messages.push({ id: nextId(), role: 'user', text })
    input = ''
    attachedFiles = []
    busy = true
    try {
      const id = await ensureSession()
      await AssistantSend(id, ctx + text)
    } catch (e) {
      messages.push({ id: nextId(), role: 'error', text: `${e}` })
      busy = false
    }
  }

  async function handleLine(raw: string) {
    let msg: any
    try { msg = JSON.parse(raw) } catch { return }
    if (msg.type === 'assistant') {
      for (const block of msg.message?.content ?? []) {
        if (block.type === 'text' && block.text) {
          messages.push({ id: nextId(), role: 'assistant', html: await renderMd(block.text) })
        } else if (block.type === 'tool_use' && block.name === 'ExitPlanMode') {
          messages.push({ id: nextId(), role: 'plan', html: await renderMd(block.input?.plan ?? '') })
          planPending = true
          busy = false
        } else if (block.type === 'tool_use') {
          const path = block.input?.file_path ?? block.input?.path ?? ''
          messages.push({ id: nextId(), role: 'tool', toolName: block.name, toolPath: path })
        }
      }
    } else if (msg.type === 'result') {
      busy = false
      if (msg.is_error) messages.push({ id: nextId(), role: 'error', text: msg.result ?? 'The assistant hit an error.' })
    }
  }

  async function approvePlan(m: ChatMsg) {
    if (!sessionId) return
    planPending = false
    m.planDone = 'approved'
    busy = true
    messages.push({ id: nextId(), role: 'status', text: 'Executing approved plan…' })
    try {
      await AssistantApprovePlan(sessionId)
    } catch (e) {
      messages.push({ id: nextId(), role: 'error', text: `${e}` })
      busy = false
    }
  }

  function rejectPlan(m: ChatMsg) {
    planPending = false
    m.planDone = 'rejected'
  }

  // interrupts the in-flight turn but keeps the conversation (Go resumes
  // the session in place) — distinct from newSession(), which ends it
  async function stopTurn() {
    if (!sessionId || !busy) return
    busy = false
    try {
      await AssistantInterrupt(sessionId)
      messages.push({ id: nextId(), role: 'status', text: 'Stopped.' })
    } catch (e) {
      messages.push({ id: nextId(), role: 'error', text: `${e}` })
    }
  }

  function newSession() {
    if (sessionId) AssistantStop(sessionId)
    offMsg?.(); offExit?.()
    offMsg = offExit = null
    sessionId = null
    messages = []
    attachedFiles = []
    planPending = false
    busy = false
  }

  function jumpTo(path: string) {
    if (!path) return
    openFileTab(path)
    pendingGoto.set({ path, line: 1, col: 0 })
  }

  function removeAttachment(p: string) {
    attachedFiles = attachedFiles.filter(f => f !== p)
  }

  async function onDrop(e: Event) {
    const dropped: string[] = (e as CustomEvent).detail.paths
    const paths = await StashDropped(dropped).catch(() => dropped)
    for (const p of paths) if (!attachedFiles.includes(p)) attachedFiles.push(p)
  }

  async function pickFiles() {
    const paths = await AssistantPickFiles().catch(() => [])
    for (const p of paths) if (!attachedFiles.includes(p)) attachedFiles.push(p)
  }

  function insertSlash() {
    if (!input.startsWith('/')) input = '/' + input
    textareaEl?.focus()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); return }
    if (e.key === 'Tab' && e.shiftKey) { e.preventDefault(); cycleMode() }
  }

  let textareaEl: HTMLTextAreaElement

  let container: HTMLDivElement
  $effect(() => {
    const el = container
    if (!el) return
    el.addEventListener('bish:filedrop', onDrop)
    return () => el.removeEventListener('bish:filedrop', onDrop)
  })
</script>

<div class="panel" bind:this={container}>
  <div class="header">
    <IconSparkles size={13} />
    <span class="header-label">Assistant</span>
    <div class="header-actions">
      {#if sessionId}
        <button class="hdr-btn" onclick={() => sessionId && AssistantStop(sessionId)} title="End session"><IconPlayerStop size={13} /></button>
      {/if}
      <button class="hdr-btn" onclick={newSession} title="New session"><IconPlus size={13} /></button>
    </div>
  </div>

  <div class="messages">
    {#if messages.length === 0}
      <div class="empty">Ask a question, or select code in the editor first for context.</div>
    {/if}
    {#each messages as m (m.id)}
      {#if m.role === 'user'}
        <div class="bubble user">{m.text}</div>
      {:else if m.role === 'assistant'}
        <div class="bubble assistant">{@html m.html}</div>
      {:else if m.role === 'tool'}
        <button class="tool-pill" disabled={!m.toolPath} onclick={() => jumpTo(m.toolPath!)}>
          <span class="tool-name">{m.toolName}</span>
          {#if m.toolPath}<span class="tool-path">{m.toolPath}</span>{/if}
        </button>
      {:else if m.role === 'plan'}
        <div class="plan-card">
          <div class="plan-label">Plan</div>
          <div class="plan-body">{@html m.html}</div>
          {#if !m.planDone}
            <div class="plan-actions">
              <button class="approve" onclick={() => approvePlan(m)}><IconCheck size={13} /> Approve</button>
              <button class="reject" onclick={() => rejectPlan(m)}><IconX size={13} /> Reject</button>
            </div>
          {:else}
            <div class="plan-status">{m.planDone === 'approved' ? 'Approved' : 'Rejected — keep refining below'}</div>
          {/if}
        </div>
      {:else if m.role === 'status'}
        <div class="status">{m.text}</div>
      {:else if m.role === 'error'}
        <div class="error">{m.text}</div>
      {/if}
    {/each}
    {#if busy}
      <div class="thinking"><span class="dot"></span><span class="dot"></span><span class="dot"></span> Working…</div>
    {/if}
  </div>

  <div class="composer">
    {#if attachedFiles.length || includeContext}
      <div class="chips">
        {#if includeContext}
          <button class="chip" onclick={() => includeContext = false} title="Stop attaching active file/selection">
            Context <IconX size={11} />
          </button>
        {/if}
        {#each attachedFiles as p}
          <button class="chip" onclick={() => removeAttachment(p)} title={p}>
            {p.split('/').pop()} <IconX size={11} />
          </button>
        {/each}
      </div>
    {/if}
    <textarea
      class="composer-input"
      placeholder="Ask the assistant…"
      bind:value={input}
      bind:this={textareaEl}
      onkeydown={onKeydown}
      rows={2}
    ></textarea>
    <div class="composer-actions">
      <div class="actions-left">
        <button class="icon-btn" onclick={pickFiles} title="Attach files"><IconPlus size={16} /></button>
        <button class="icon-btn" onclick={insertSlash} title="Slash command"><IconSlash size={16} /></button>
      </div>
      <div class="actions-right">
        <button class="mode-pill" onclick={cycleMode} title="Permission mode — click or Shift+Tab to cycle">
          <IconCode size={11} /> {MODE_LABELS[permissionMode]}
        </button>
        {#if busy}
          <button class="icon-send stop" onclick={stopTurn} title="Stop"><IconPlayerStopFilled size={16} /></button>
        {:else}
          <button class="icon-send" disabled={!input.trim()} onclick={send} title="Send"><IconSendFilled size={16} /></button>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .header {
    display: flex; align-items: center; gap: 6px; padding: 0 12px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border);
    color: var(--muted);
  }
  .header-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; color: var(--muted);
  }
  .header-actions { display: flex; align-items: center; gap: 4px; margin-left: auto; flex-shrink: 0; }
  .hdr-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .messages { flex: 1; overflow-y: auto; padding: 8px; display: flex; flex-direction: column; gap: 8px; }
  .empty { color: var(--muted); font-size: 12px; padding: 8px; }

  .bubble { font-size: 12px; line-height: 1.5; border-radius: 6px; padding: 6px 9px; max-width: 100%; }
  .bubble.user { background: var(--bg-selected); color: var(--foreground); align-self: flex-end; white-space: pre-wrap; }
  .bubble.assistant { background: var(--bg-raised); color: var(--foreground); }
  .bubble.assistant :global(p) { margin: 0 0 6px; }
  .bubble.assistant :global(p:last-child) { margin-bottom: 0; }
  .bubble.assistant :global(pre) { background: var(--background); padding: 6px; border-radius: 4px; overflow-x: auto; }
  .bubble.assistant :global(code) { font-family: "SF Mono", Menlo, monospace; font-size: 11px; }

  .tool-pill {
    display: flex; align-items: center; gap: 6px; align-self: flex-start;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 12px;
    padding: 2px 8px; font-size: 11px; color: var(--muted); cursor: pointer;
  }
  .tool-pill:disabled { cursor: default; }
  .tool-pill:not(:disabled):hover { color: var(--foreground); border-color: var(--accent); }
  .tool-name { font-weight: 600; }
  .tool-path { color: var(--muted); font-family: "SF Mono", Menlo, monospace; }

  .plan-card { border: 1px solid var(--accent); border-radius: 6px; overflow: hidden; }
  .plan-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase;
    color: var(--accent); background: color-mix(in srgb, var(--accent) 15%, transparent);
    padding: 4px 9px;
  }
  .plan-body { font-size: 12px; line-height: 1.5; padding: 8px 9px; }
  .plan-body :global(p) { margin: 0 0 6px; }
  .plan-body :global(ul), .plan-body :global(ol) { margin: 0 0 6px; padding-left: 18px; }
  .plan-actions { display: flex; gap: 6px; padding: 0 9px 9px; }
  .plan-actions button {
    display: flex; align-items: center; gap: 4px; font-size: 11px; border-radius: 4px;
    padding: 4px 8px; cursor: pointer; border: 1px solid var(--border); background: var(--bg-raised);
    color: var(--foreground);
  }
  .plan-actions .approve { border-color: var(--success); color: var(--success); }
  .plan-actions .reject { border-color: var(--error); color: var(--error); }
  .plan-status { font-size: 11px; color: var(--muted); padding: 0 9px 9px; }

  .status { font-size: 11px; color: var(--muted); align-self: center; }
  .thinking {
    display: flex; align-items: center; gap: 5px; align-self: flex-start;
    font-size: 11px; color: var(--muted); padding: 2px 2px;
  }
  .thinking .dot {
    width: 4px; height: 4px; border-radius: 50%; background: var(--muted);
    animation: thinking-pulse 1.1s ease-in-out infinite;
  }
  .thinking .dot:nth-child(2) { animation-delay: 0.15s; }
  .thinking .dot:nth-child(3) { animation-delay: 0.3s; }
  @keyframes thinking-pulse {
    0%, 60%, 100% { opacity: 0.25; }
    30% { opacity: 1; }
  }
  .error {
    font-size: 11px; color: var(--error); align-self: stretch; white-space: pre-wrap;
    font-family: "SF Mono", Menlo, monospace; background: var(--bg-raised);
    border: 1px solid var(--error); border-radius: 4px; padding: 6px 8px;
  }

  .composer { border-top: 1px solid var(--border); padding: 6px 8px; flex-shrink: 0; }
  .chips { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 6px; }
  .chip {
    display: flex; align-items: center; gap: 4px; font-size: 10px; color: var(--muted);
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 10px;
    padding: 2px 6px; cursor: pointer;
  }
  .chip:hover { color: var(--foreground); border-color: var(--accent); }

  .composer-input {
    width: 100%; resize: none; background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 12px; padding: 6px 8px; outline: none;
    font-family: inherit; box-sizing: border-box;
  }
  .composer-input:focus { border-color: var(--accent); }

  .composer-actions { display: flex; align-items: center; justify-content: space-between; margin-top: 6px; }
  .actions-left, .actions-right { display: flex; align-items: center; gap: 4px; }

  .icon-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .icon-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .mode-pill {
    display: flex; align-items: center; gap: 4px; white-space: nowrap;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 10px;
    color: var(--foreground); font-size: 10px; padding: 2px 7px; cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  .mode-pill:hover { border-color: var(--accent); background: var(--bg-hover); }

  .icon-send {
    display: flex; align-items: center; justify-content: center; flex-shrink: 0;
    background: none; border: none; cursor: pointer; color: var(--foreground); padding: 3px;
  }
  .icon-send:disabled { opacity: 0.35; cursor: default; }
  .icon-send.stop { color: var(--error); }
</style>
