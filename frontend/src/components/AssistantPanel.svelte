<script lang="ts">
  import { get } from 'svelte/store'
  import { marked } from 'marked'
  import DOMPurify from 'dompurify'
  import {
    IconSparkles, IconPlus, IconPlayerStop, IconPlayerStopFilled, IconSendFilled, IconX, IconCheck, IconCode, IconSlash,
  } from '@tabler/icons-svelte'
  import {
    on, AssistantStart, AssistantSend, AssistantRespondPermission, AssistantStop, AssistantInterrupt, AssistantSwitchMode,
    AssistantPickFiles, StashDropped,
  } from '../lib/wails'
  import {
    projectRoot, cwd, tabs, activeTabId, activeSelection, pendingGoto, openFileTab, pendingExternalReload,
  } from '../lib/stores'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { fuzzyMatch } from '../lib/fuzzy'
  import { SLASH_COMMANDS } from '../lib/slashCommands'
  import SwitchModelDialog from './SwitchModelDialog.svelte'

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

  // Tool names (across both the `claude` CLI and bish's own Ollama tool loop)
  // that write file contents — used to auto-refresh an open tab afterward.
  const FILE_WRITE_TOOLS = new Set(['write_file', 'Write', 'Edit', 'MultiEdit', 'NotebookEdit'])

  interface ChatMsg {
    id: string
    turnId: number
    role: 'user' | 'assistant' | 'tool' | 'plan' | 'permission' | 'status' | 'error'
    text?: string
    html?: string
    toolName?: string
    toolPath?: string
    planDone?: 'approved' | 'rejected'
    // set on a 'plan' card as soon as its ExitPlanMode tool_use block
    // arrives, before the CLI's matching can_use_tool ask shows up
    toolUseId?: string
    // the pending can_use_tool control_request's own id — arrives slightly
    // after toolUseId, via a separate 'permission_request' event; Approve/
    // Reject can't answer the CLI until this is set
    requestId?: string
  }

  let messages = $state<ChatMsg[]>([])
  let turn = $state(0)
  let input = $state('')
  let attachedFiles = $state<string[]>([])
  let includeContext = $state(true)
  let busy = $state(false)
  let planPending = $state(false)
  // least-permissive by default every fresh session; only sticky if the user raised it
  let permissionMode = $state(localStorage.getItem(PERM_KEY) ?? 'plan')
  let showModelDialog = $state(false)
  // last alias picked via the dialog — the CLI never reports its active model
  // back to us, so this is "what we last told it," not an authoritative read
  const MODEL_KEY = 'bish.assistant.model'
  let currentModel = $state(localStorage.getItem(MODEL_KEY) ?? 'default')

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
      messages.push({ id: nextId(), turnId: turn, role: 'status', text: `Switched to "${MODE_LABELS[next]}".` })
    } catch (e) {
      messages.push({ id: nextId(), turnId: turn, role: 'error', text: `${e}` })
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
        id: nextId(), turnId: turn, role: 'error',
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
    if (!text) return
    if (text === '/model') {
      input = ''
      showModelDialog = true
      return
    }
    const cmd = SLASH_COMMANDS.find(c => c.terminalOnly && c.name === text.split(/\s+/)[0])
    turn += 1
    slashDismissed = false
    messages.push({ id: nextId(), turnId: turn, role: 'user', text })
    input = ''
    attachedFiles = []
    if (cmd) {
      messages.push({
        id: nextId(), turnId: turn, role: 'status',
        text: `${cmd.name} needs an interactive terminal session — run it in the Terminal panel instead.`,
      })
      return
    }
    const ctx = buildContext()
    busy = true
    try {
      const id = await ensureSession()
      await AssistantSend(id, ctx + text)
    } catch (e) {
      messages.push({ id: nextId(), turnId: turn, role: 'error', text: `${e}` })
      busy = false
    }
  }

  async function handleLine(raw: string) {
    let msg: any
    try { msg = JSON.parse(raw) } catch { return }
    if (msg.type === 'assistant') {
      for (const block of msg.message?.content ?? []) {
        if (block.type === 'text' && block.text) {
          messages.push({ id: nextId(), turnId: turn, role: 'assistant', html: await renderMd(block.text) })
        } else if (block.type === 'tool_use' && block.name === 'ExitPlanMode') {
          messages.push({
            id: nextId(), turnId: turn, role: 'plan', toolUseId: block.id,
            html: await renderMd(block.input?.plan ?? ''),
          })
          planPending = true
          busy = false
        } else if (block.type === 'tool_use') {
          const path = block.input?.file_path ?? block.input?.path ?? ''
          messages.push({ id: nextId(), turnId: turn, role: 'tool', toolName: block.name, toolPath: path })
          if (path && FILE_WRITE_TOOLS.has(block.name)) {
            // small settle delay — the CLI backend's tool_use announcement
            // isn't guaranteed to strictly follow the actual disk write
            setTimeout(() => pendingExternalReload.set(path), 300)
          }
        }
      }
    } else if (msg.type === 'permission_request') {
      // The CLI is blocked on a can_use_tool ask. If it matches a plan card
      // already on screen (ExitPlanMode), just attach the request id so
      // Approve/Reject can answer it. Otherwise it's some other tool the
      // current permission mode wants a human decision on — show a generic
      // approval card for it.
      const plan = [...messages].reverse().find(m => m.role === 'plan' && m.toolUseId === msg.tool_use_id && !m.requestId)
      if (plan) {
        plan.requestId = msg.request_id
        messages = messages
      } else {
        messages.push({
          id: nextId(), turnId: turn, role: 'permission',
          toolName: msg.tool_name, requestId: msg.request_id, text: msg.title,
        })
        busy = false
      }
    } else if (msg.type === 'result') {
      busy = false
      if (msg.is_error) messages.push({ id: nextId(), turnId: turn, role: 'error', text: msg.result ?? 'The assistant hit an error.' })
    }
  }

  // Answers the CLI's pending can_use_tool ask either way — approve or
  // reject both resolve it and let the model keep generating in the same
  // turn (a reject just tells it no and lets it react, e.g. revise the plan).
  async function respondPermission(m: ChatMsg, allow: boolean) {
    if (!sessionId || !m.requestId) return
    if (m.role === 'plan') planPending = false
    m.planDone = allow ? 'approved' : 'rejected'
    busy = true
    messages.push({
      id: nextId(), turnId: turn, role: 'status',
      text: allow ? 'Continuing…' : 'Rejected — waiting for the assistant…',
    })
    try {
      await AssistantRespondPermission(sessionId, m.requestId, allow, '')
    } catch (e) {
      messages.push({ id: nextId(), turnId: turn, role: 'error', text: `${e}` })
      busy = false
    }
  }

  // interrupts the in-flight turn but keeps the conversation (Go resumes
  // the session in place) — distinct from newSession(), which ends it
  async function stopTurn() {
    if (!sessionId || !busy) return
    busy = false
    try {
      await AssistantInterrupt(sessionId)
      messages.push({ id: nextId(), turnId: turn, role: 'status', text: 'Stopped.' })
    } catch (e) {
      messages.push({ id: nextId(), turnId: turn, role: 'error', text: `${e}` })
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
    if (!input.startsWith('/')) { input = '/' + input; slashDismissed = false }
    textareaEl?.focus()
  }

  // slash-command menu — open while `input` looks like an in-progress command
  // (starts with '/', no whitespace yet); Escape dismisses until input is cleared
  let slashDismissed = $state(false)
  let slashIdx = $state(0)
  const slashOpen = $derived(!slashDismissed && input.startsWith('/') && !/\s/.test(input))
  const slashResults = $derived.by(() => {
    if (!slashOpen) return []
    const q = input.slice(1)
    if (!q) return SLASH_COMMANDS
    return SLASH_COMMANDS
      .map(c => ({ c, m: fuzzyMatch(q, c.name.slice(1)) }))
      .filter(r => r.m)
      .sort((a, b) => b.m!.score - a.m!.score)
      .map(r => r.c)
  })
  $effect(() => { slashResults; slashIdx = 0 })

  function applySlash(cmd: { name: string }) {
    if (cmd.name === '/model') {
      input = ''
      slashDismissed = true
      showModelDialog = true
      return
    }
    input = cmd.name + ' '
    slashDismissed = true
    textareaEl?.focus()
  }

  // sends "/model <alias>" the same way any other slash command is sent —
  // the CLI resolves the alias to the right model for whatever provider
  // it's actually configured against (direct API, Bedrock, Vertex), so the
  // dialog never needs to know a provider-specific model ID
  async function selectModel(alias: string) {
    showModelDialog = false
    currentModel = alias
    localStorage.setItem(MODEL_KEY, alias)
    const text = `/model ${alias}`
    turn += 1
    messages.push({ id: nextId(), turnId: turn, role: 'user', text })
    busy = true
    try {
      const id = await ensureSession()
      await AssistantSend(id, text)
    } catch (e) {
      messages.push({ id: nextId(), turnId: turn, role: 'error', text: `${e}` })
      busy = false
    }
  }

  function onComposerInput() {
    if (!input) slashDismissed = false
  }

  function onKeydown(e: KeyboardEvent) {
    if (slashOpen && slashResults.length) {
      if (e.key === 'ArrowDown') { e.preventDefault(); slashIdx = Math.min(slashIdx + 1, slashResults.length - 1); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); slashIdx = Math.max(slashIdx - 1, 0); return }
      if (e.key === 'Escape') { e.preventDefault(); slashDismissed = true; return }
      if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey) { e.preventDefault(); applySlash(slashResults[slashIdx]); return }
    }
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); return }
    if (e.key === 'Tab' && e.shiftKey) { e.preventDefault(); cycleMode() }
  }

  function onMessagesClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest('a')
    if (!a) return
    const href = a.getAttribute('href')
    if (href && /^https?:\/\//i.test(href)) { e.preventDefault(); BrowserOpenURL(href) }
  }

  // group consecutive non-user messages from the same turn so the template
  // can thread them with a connecting line — user bubbles always stand alone
  const groups = $derived.by(() => {
    const out: { turnId: number; msgs: ChatMsg[] }[] = []
    for (const m of messages) {
      const last = out[out.length - 1]
      if (m.role !== 'user' && last && last.turnId === m.turnId && last.msgs[0].role !== 'user') {
        last.msgs.push(m)
      } else {
        out.push({ turnId: m.turnId, msgs: [m] })
      }
    }
    return out
  })

  let textareaEl: HTMLTextAreaElement

  let container: HTMLDivElement
  $effect(() => {
    const el = container
    if (!el) return
    el.addEventListener('bish:filedrop', onDrop)
    return () => el.removeEventListener('bish:filedrop', onDrop)
  })

  // autoscroll to new content, but don't yank the view if the user has
  // scrolled up to read scrollback
  let messagesEl: HTMLDivElement
  let stickToBottom = true

  function onMessagesScroll() {
    const el = messagesEl
    stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }

  $effect(() => {
    messages.length
    busy
    if (stickToBottom && messagesEl) messagesEl.scrollTop = messagesEl.scrollHeight
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

  {#snippet msgItem(m: ChatMsg)}
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
            <button class="approve" disabled={!m.requestId} onclick={() => respondPermission(m, true)}><IconCheck size={13} /> Approve</button>
            <button class="reject" disabled={!m.requestId} onclick={() => respondPermission(m, false)}><IconX size={13} /> Reject</button>
          </div>
        {:else}
          <div class="plan-status">{m.planDone === 'approved' ? 'Approved' : 'Rejected — keep refining below'}</div>
        {/if}
      </div>
    {:else if m.role === 'permission'}
      <div class="plan-card">
        <div class="plan-label">Permission</div>
        <div class="plan-body">{m.text || `Allow "${m.toolName}"?`}</div>
        {#if !m.planDone}
          <div class="plan-actions">
            <button class="approve" onclick={() => respondPermission(m, true)}><IconCheck size={13} /> Allow</button>
            <button class="reject" onclick={() => respondPermission(m, false)}><IconX size={13} /> Deny</button>
          </div>
        {:else}
          <div class="plan-status">{m.planDone === 'approved' ? 'Allowed' : 'Denied'}</div>
        {/if}
      </div>
    {:else if m.role === 'status'}
      <div class="status">{m.text}</div>
    {:else if m.role === 'error'}
      <div class="error">{m.text}</div>
    {/if}
  {/snippet}

  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="messages" bind:this={messagesEl} onscroll={onMessagesScroll} onclick={onMessagesClick}>
    {#if messages.length === 0}
      <div class="empty">Ask a question, or select code in the editor first for context.</div>
    {/if}
    {#each groups as g (g.msgs[0].id)}
      {#if g.msgs.length > 1}
        <div class="turn-group">
          {#each g.msgs as m (m.id)}
            <div class="turn-item"><span class="turn-dot"></span>{@render msgItem(m)}</div>
          {/each}
        </div>
      {:else}
        {@render msgItem(g.msgs[0])}
      {/if}
    {/each}
    {#if busy}
      <div class="thinking"><span class="dot"></span><span class="dot"></span><span class="dot"></span> Working…</div>
    {/if}
  </div>

  <div class="composer">
    {#if slashOpen}
      <div class="slash-menu">
        {#if slashResults.length > 0}
          {#each slashResults as c, i (c.name)}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <div
              class="slash-row"
              class:active={i === slashIdx}
              onclick={() => applySlash(c)}
              onmouseenter={() => slashIdx = i}
              role="option"
              aria-selected={i === slashIdx}
              tabindex="-1"
            >
              <span class="slash-name">{c.name}</span>
              <span class="slash-desc">{c.description}</span>
            </div>
          {/each}
        {:else}
          <div class="slash-empty">No matching commands</div>
        {/if}
      </div>
    {/if}
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
      oninput={onComposerInput}
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
        {/if}
        <button class="icon-send" disabled={!input.trim()} onclick={send} title={busy ? 'Send — injects without stopping the current turn' : 'Send'}><IconSendFilled size={16} /></button>
      </div>
    </div>
  </div>
</div>

{#if showModelDialog}
  <SwitchModelDialog current={currentModel} onSelect={selectModel} onClose={() => showModelDialog = false} />
{/if}

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

  .messages { flex: 1; overflow-y: auto; padding: 8px 12px; display: flex; flex-direction: column; gap: 8px; }

  .turn-group { position: relative; display: flex; flex-direction: column; gap: 8px; padding-left: 14px; }
  .turn-group::before {
    content: ''; position: absolute; left: 4px; top: 6px; bottom: 6px; width: 1px; background: var(--border);
  }
  .turn-item { position: relative; }
  .turn-dot {
    position: absolute; left: -10px; top: 7px; width: 5px; height: 5px;
    border-radius: 50%; background: var(--muted);
  }
  .empty { color: var(--muted); font-size: 12px; padding: 8px; }

  .bubble { font-size: 12px; line-height: 1.5; border-radius: 6px; padding: 6px 9px; max-width: 100%; }
  .bubble.user { background: var(--bg-selected); color: var(--foreground); align-self: flex-end; white-space: pre-wrap; }
  .bubble.assistant { background: var(--bg-raised); color: var(--foreground); }
  .bubble.assistant :global(p) { margin: 0 0 6px; }
  .bubble.assistant :global(p:last-child) { margin-bottom: 0; }
  .bubble.assistant :global(pre) { background: var(--background); padding: 6px; border-radius: 4px; overflow-x: auto; }
  .bubble.assistant :global(code) { font-family: "SF Mono", Menlo, monospace; font-size: 11px; }

  .bubble.assistant :global(h1), .bubble.assistant :global(h2), .bubble.assistant :global(h3),
  .bubble.assistant :global(h4), .bubble.assistant :global(h5), .bubble.assistant :global(h6),
  .plan-body :global(h1), .plan-body :global(h2), .plan-body :global(h3),
  .plan-body :global(h4), .plan-body :global(h5), .plan-body :global(h6) {
    margin: 8px 0 6px; font-size: 13px; font-weight: 600;
  }
  .bubble.assistant :global(h1:first-child), .bubble.assistant :global(h2:first-child), .bubble.assistant :global(h3:first-child),
  .bubble.assistant :global(h4:first-child), .bubble.assistant :global(h5:first-child), .bubble.assistant :global(h6:first-child),
  .plan-body :global(h1:first-child), .plan-body :global(h2:first-child), .plan-body :global(h3:first-child),
  .plan-body :global(h4:first-child), .plan-body :global(h5:first-child), .plan-body :global(h6:first-child) {
    margin-top: 0;
  }
  .bubble.assistant :global(ul), .bubble.assistant :global(ol) { margin: 0 0 6px; padding-left: 18px; }
  .bubble.assistant :global(li), .plan-body :global(li) { margin: 0 0 2px; }
  .bubble.assistant :global(blockquote), .plan-body :global(blockquote) {
    border-left: 2px solid var(--border); margin: 0 0 6px; padding-left: 9px; color: var(--muted);
  }
  .bubble.assistant :global(table), .plan-body :global(table) { border-collapse: collapse; margin: 0 0 6px; }
  .bubble.assistant :global(th), .bubble.assistant :global(td),
  .plan-body :global(th), .plan-body :global(td) {
    border: 1px solid var(--border); padding: 3px 6px;
  }
  .bubble.assistant :global(th), .plan-body :global(th) { font-weight: 600; }
  .bubble.assistant :global(hr), .plan-body :global(hr) { border: none; border-top: 1px solid var(--border); margin: 6px 0; }
  .bubble.assistant :global(a), .plan-body :global(a) { color: var(--accent); }
  .bubble.assistant :global(img), .plan-body :global(img) { max-width: 100%; }

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
  .plan-actions button:disabled { opacity: 0.4; cursor: default; }
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

  .composer { position: relative; border-top: 1px solid var(--border); padding: 6px 12px; flex-shrink: 0; }

  .slash-menu {
    position: absolute; left: 12px; right: 12px; bottom: 100%; margin-bottom: 4px;
    max-height: 220px; overflow-y: auto; background: var(--bg-raised);
    border: 1px solid var(--border-focused); border-radius: 6px;
    box-shadow: 0 8px 24px color-mix(in srgb, #000 40%, transparent);
    padding: 4px;
  }
  .slash-row {
    display: flex; align-items: baseline; gap: 8px; padding: 5px 8px;
    border-radius: 4px; cursor: pointer; font-size: 12px;
  }
  .slash-row.active { background: var(--bg-selected); }
  .slash-row:hover { background: var(--bg-hover); }
  .slash-name { font-family: "SF Mono", Menlo, monospace; color: var(--foreground); flex-shrink: 0; }
  .slash-desc { color: var(--muted); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .slash-empty { padding: 8px; font-size: 11px; color: var(--muted); text-align: center; }
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
