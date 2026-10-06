<script lang="ts">
  import {
    IconCopy, IconCheck, IconPencil, IconRestore, IconGitFork, IconBrain, IconChevronRight, IconChevronDown,
    IconArrowDown, IconListCheck, IconCircle, IconCircleCheck, IconCircleDot, IconHistory, IconLoader2, IconPhoto, IconFile,
  } from '@tabler/icons-svelte'
  import type { UserItem, Item, ToolItem } from '../../lib/claude/conversation.svelte'
  import type { AgentConversation } from '../../lib/claude/agent'
  import type { SessionSummary } from './types'
  import { fmtCost, fmtDuration, timeAgo, basename } from '../../lib/claude/format'
  import Markdown from './Markdown.svelte'
  import ToolCard from './ToolCard.svelte'
  import Composer from './Composer.svelte'

  let { conv, recent = [], onResume, onFork, onModel, onPanelCommand }: {
    conv: AgentConversation
    recent?: SessionSummary[]
    onResume: (s: SessionSummary) => void
    onFork: (u: UserItem) => void
    onModel: () => void
    onPanelCommand: (name: string, args: string) => boolean
  } = $props()

  let composer: Composer
  export function focus() { composer?.focus() }
  export function insert(text: string) { composer?.insert(text) }
  export function attach(paths: string[]) { composer?.attach(paths) }

  const root = $derived(conv.projectRoot)

  // consecutive non-user items of one turn thread together under a rail
  const groups = $derived.by(() => {
    const out: { key: string; user?: UserItem; items: Item[] }[] = []
    for (const it of conv.items) {
      if (it.kind === 'user') out.push({ key: it.id, user: it, items: [] })
      else if (out.length) out[out.length - 1].items.push(it)
      else out.push({ key: it.id, items: [it] })
    }
    return out
  })

  // ─── user message actions ────────────────────────────────────────────────
  let copied = $state<string | null>(null)
  function copy(id: string, text: string) {
    navigator.clipboard.writeText(text)
    copied = id
    setTimeout(() => { if (copied === id) copied = null }, 1200)
  }
  async function edit(u: UserItem) {
    if (conv.busy) await conv.interrupt()
    const text = u.text
    await conv.rewindConversation(u)
    composer?.setText(text)
  }
  let restoreFor = $state<{ u: UserItem; files: string[]; error?: string; loading: boolean } | null>(null)
  async function askRestore(u: UserItem) {
    restoreFor = { u, files: [], loading: true }
    const r = await conv.previewRewind(u)
    restoreFor = { u, files: r.files, error: r.canRewind ? undefined : (r.error || 'Nothing to restore.'), loading: false }
  }
  async function doRestore(alsoConversation: boolean) {
    if (!restoreFor) return
    const u = restoreFor.u
    restoreFor = null
    const ok = await conv.rewindFiles(u)
    if (ok && alsoConversation) await edit(u)
  }

  // ─── thinking blocks ─────────────────────────────────────────────────────
  let openThinking = $state<Record<string, boolean>>({})

  // ─── todos ───────────────────────────────────────────────────────────────
  let todosOpen = $state(true)
  const todoActive = $derived(conv.todos.some(t => t.status !== 'completed'))
  const todoDone = $derived(conv.todos.filter(t => t.status === 'completed').length)
  const currentTodo = $derived(conv.todos.find(t => t.status === 'in_progress'))

  // ─── scrolling ───────────────────────────────────────────────────────────
  let scroller: HTMLDivElement
  let stick = $state(true)
  function onScroll() { stick = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 60 }
  function toBottom() { scroller.scrollTop = scroller.scrollHeight; stick = true }
  $effect(() => {
    // re-run on any content change; tick after DOM update
    conv.items.length
    const last = conv.items[conv.items.length - 1]
    if (last?.kind === 'text') last.html
    if (last?.kind === 'thinking') last.text
    conv.busy
    if (stick && scroller) queueMicrotask(() => { scroller.scrollTop = scroller.scrollHeight })
  })

  const waitingOnUser = $derived(conv.pendingAsks.length > 0)
</script>

<div class="chat">
  <div class="scroll" bind:this={scroller} onscroll={onScroll}>
    {#if conv.items.length === 0}
      <div class="welcome">
        <div class="logo">{conv.caps.provider === 'codex' ? '❯_' : '✻'}</div>
        <div class="w-title">What should we work on?</div>
        <div class="w-tips">
          <span><kbd>@</kbd> mention files</span>
          <span><kbd>/</kbd> commands</span>
          <span><kbd>⇧Tab</kbd> permission mode</span>
          <span><kbd>Esc</kbd> stop</span>
        </div>
        {#if conv.resumable}
          <div class="hint">Send a message to continue this conversation.</div>
        {/if}
        {#if recent.length}
          <div class="recent">
            <div class="recent-h"><IconHistory size={12} /> Recent conversations</div>
            {#each recent.slice(0, 5) as s (s.id)}
              <button class="recent-row" onclick={() => onResume(s)}>
                <span class="r-title">{s.title}</span>
                <span class="r-time">{timeAgo(s.modified)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {#each groups as g (g.key)}
      {#if g.user}
        {@const u = g.user}
        <div class="user-wrap">
          <div class="user">
            <div class="user-text">{u.text}</div>
            {#if u.images.length || u.files.length}
              <div class="user-att">
                {#each u.images as p}<span><IconPhoto size={11} /> {basename(p)}</span>{/each}
                {#each u.files as p}<span><IconFile size={11} /> {basename(p)}</span>{/each}
              </div>
            {/if}
          </div>
          <div class="user-actions">
            <button title="Copy" onclick={() => copy(u.id, u.text)}>{#if copied === u.id}<IconCheck size={12} />{:else}<IconCopy size={12} />{/if}</button>
            {#if conv.caps.editRewind}
              <button title="Edit — rewind the conversation to here" onclick={() => edit(u)}><IconPencil size={12} /></button>
            {/if}
            {#if conv.caps.restoreCode && u.uuid}
              <button title="Restore code to before this message" onclick={() => askRestore(u)}><IconRestore size={12} /></button>
            {/if}
            {#if conv.caps.fork && conv.sessionId && u.prevAssistantUuid}
              <button title="Fork into a new tab from here" onclick={() => onFork(u)}><IconGitFork size={12} /></button>
            {/if}
          </div>
          {#if restoreFor?.u.id === u.id}
            <div class="restore">
              {#if restoreFor.loading}
                <IconLoader2 size={12} class="spin" /> Checking checkpoint…
              {:else if restoreFor.error}
                <span>{restoreFor.error}</span>
                <button onclick={() => restoreFor = null}>Close</button>
              {:else}
                <div>Restore {restoreFor.files.length} file{restoreFor.files.length === 1 ? '' : 's'} to before this message?</div>
                {#if restoreFor.files.length}<ul>{#each restoreFor.files.slice(0, 8) as f}<li>{basename(f)}</li>{/each}</ul>{/if}
                <div class="restore-actions">
                  <button class="primary" onclick={() => doRestore(false)}>Restore code</button>
                  <button onclick={() => doRestore(true)}>Restore code and conversation</button>
                  <button onclick={() => restoreFor = null}>Cancel</button>
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
      {#if g.items.length}
        <div class="turn">
          {#each g.items as it (it.id)}
            <div class="item {it.kind}">
              {#if it.kind === 'text'}
                <Markdown html={it.html || ''} {root} streaming={it.streaming} />
              {:else if it.kind === 'thinking'}
                <button class="thinking" onclick={() => openThinking[it.id] = !openThinking[it.id]}>
                  {#if openThinking[it.id]}<IconChevronDown size={11} />{:else}<IconChevronRight size={11} />{/if}
                  <IconBrain size={12} /> {it.streaming ? 'Thinking…' : 'Thought process'}
                </button>
                {#if openThinking[it.id] || it.streaming}
                  <div class="thinking-body" class:live={it.streaming}>{it.text}</div>
                {/if}
              {:else if it.kind === 'tool'}
                <ToolCard t={it as ToolItem} {conv} {root} />
              {:else if it.kind === 'note'}
                <div class="note {it.tone}">{it.text}</div>
              {:else if it.kind === 'result'}
                <div class="result">{fmtDuration(it.durationMs)}{#if it.costUsd} · {fmtCost(it.costUsd)} total{/if}</div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    {/each}

    {#if conv.busy && !waitingOnUser}
      <div class="working">
        <span class="spark">✻</span>
        {conv.status || (conv.starting ? 'Starting Claude…' : currentTodo?.activeForm || 'Working…')}
        <span class="esc">esc to interrupt</span>
      </div>
    {:else if conv.status}
      <div class="working">{conv.status}</div>
    {/if}
  </div>

  {#if !stick}
    <button class="jump" onclick={toBottom} title="Scroll to bottom"><IconArrowDown size={14} /></button>
  {/if}

  {#if conv.todos.length && todoActive}
    <div class="todo-bar">
      <button class="todo-head" onclick={() => todosOpen = !todosOpen}>
        {#if todosOpen}<IconChevronDown size={11} />{:else}<IconChevronRight size={11} />{/if}
        <IconListCheck size={12} /> Todos <span class="muted">{todoDone}/{conv.todos.length}</span>
        {#if !todosOpen && currentTodo}<span class="cur">{currentTodo.activeForm || currentTodo.content}</span>{/if}
      </button>
      {#if todosOpen}
        <ul>
          {#each conv.todos as td, i (i)}
            <li class={td.status}>
              {#if td.status === 'completed'}<IconCircleCheck size={12} />{:else if td.status === 'in_progress'}<IconCircleDot size={12} />{:else}<IconCircle size={12} />{/if}
              <span>{td.content}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  <Composer bind:this={composer} {conv} {onModel} {onPanelCommand} />
</div>

<style>
  .chat { position: relative; display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .scroll { flex: 1; overflow-y: auto; padding: 10px 12px 4px; display: flex; flex-direction: column; gap: 10px; }

  .welcome { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px 8px 12px; text-align: center; }
  .logo { font-size: 30px; color: var(--accent); line-height: 1; }
  .w-title { font-size: 14px; font-weight: 600; color: var(--foreground); }
  .w-tips { display: flex; flex-wrap: wrap; justify-content: center; gap: 4px 12px; font-size: 11px; color: var(--muted); }
  kbd {
    font-family: "SF Mono", Menlo, monospace; font-size: 10px; border: 1px solid var(--border);
    border-radius: 3px; padding: 0 4px; background: var(--bg-raised);
  }
  .hint { font-size: 11px; color: var(--muted); }
  .recent { width: 100%; margin-top: 14px; display: flex; flex-direction: column; gap: 2px; text-align: left; }
  .recent-h { display: flex; align-items: center; gap: 5px; font-size: 10.5px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.06em; padding: 0 6px 4px; }
  .recent-row {
    display: flex; justify-content: space-between; gap: 8px; background: none; border: none; border-radius: 4px;
    padding: 5px 6px; cursor: pointer; color: var(--foreground); font-size: 12px; text-align: left; font-family: inherit;
  }
  .recent-row:hover { background: var(--bg-hover); }
  .r-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .r-time { color: var(--muted); font-size: 11px; flex-shrink: 0; }

  .user-wrap { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; }
  .user {
    max-width: 92%; background: var(--bg-selected); border: 1px solid var(--border); border-radius: 8px;
    padding: 6px 10px; font-size: 12.5px; line-height: 1.5; color: var(--foreground);
  }
  .user-text { white-space: pre-wrap; overflow-wrap: anywhere; }
  .user-att { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; font-size: 10.5px; color: var(--muted); }
  .user-att span { display: inline-flex; align-items: center; gap: 3px; }
  .user-actions { display: flex; gap: 1px; opacity: 0; transition: opacity 0.1s; }
  .user-wrap:hover .user-actions, .user-actions:focus-within { opacity: 1; }
  .user-actions button {
    display: flex; background: none; border: none; color: var(--muted); cursor: pointer; padding: 3px 4px; border-radius: 3px;
  }
  .user-actions button:hover { color: var(--foreground); background: var(--bg-hover); }
  .restore {
    align-self: stretch; border: 1px solid var(--border); border-radius: 6px; background: var(--bg-raised);
    padding: 8px; font-size: 11.5px; color: var(--foreground); display: flex; flex-direction: column; gap: 6px;
  }
  .restore ul { margin: 0; padding-left: 16px; color: var(--muted); font-family: "SF Mono", Menlo, monospace; font-size: 10.5px; }
  .restore-actions { display: flex; flex-wrap: wrap; gap: 5px; }
  .restore button {
    background: var(--background); border: 1px solid var(--border); border-radius: 4px; color: var(--foreground);
    font-size: 11px; padding: 3px 8px; cursor: pointer; align-self: flex-start;
  }
  .restore button:hover { border-color: var(--accent); }
  .restore button.primary { border-color: var(--accent); }

  .turn { display: flex; flex-direction: column; gap: 8px; }
  .thinking {
    display: flex; align-items: center; gap: 5px; background: none; border: none; padding: 0;
    color: var(--muted); font-size: 11.5px; cursor: pointer; font-family: inherit;
  }
  .thinking:hover { color: var(--foreground); }
  .thinking-body {
    margin-top: 4px; padding: 4px 0 4px 10px; border-left: 2px solid var(--border);
    color: var(--muted); font-size: 11.5px; line-height: 1.5; white-space: pre-wrap; font-style: italic;
    max-height: 260px; overflow-y: auto;
  }
  .note { font-size: 11.5px; color: var(--muted); white-space: pre-wrap; overflow-wrap: anywhere; }
  .note.error {
    color: var(--error); background: color-mix(in srgb, var(--error) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--error) 40%, transparent); border-radius: 5px; padding: 6px 8px;
    font-family: "SF Mono", Menlo, monospace; font-size: 11px;
  }
  .note.compact { text-align: center; border-top: 1px dashed var(--border); padding-top: 6px; }
  .note.command {
    font-family: "SF Mono", Menlo, monospace; font-size: 11px; color: var(--foreground);
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 5px; padding: 6px 8px;
  }
  .result { font-size: 10.5px; color: var(--muted); opacity: 0.7; }

  .working { display: flex; align-items: center; gap: 6px; font-size: 11.5px; color: var(--muted); padding: 2px 0 6px; }
  .spark { color: var(--accent); display: inline-block; animation: spark 2s linear infinite; }
  @keyframes spark { to { transform: rotate(360deg); } }
  .esc { opacity: 0.6; font-size: 10.5px; }

  .jump {
    position: absolute; right: 16px; bottom: 120px; z-index: 5; display: flex; padding: 5px;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 50%; color: var(--muted); cursor: pointer;
    box-shadow: 0 2px 8px color-mix(in srgb, #000 30%, transparent);
  }
  .jump:hover { color: var(--foreground); }

  .todo-bar { margin: 0 10px; border: 1px solid var(--border); border-bottom: none; border-radius: 6px 6px 0 0; background: var(--bg-raised); flex-shrink: 0; }
  .todo-head {
    display: flex; align-items: center; gap: 5px; width: 100%; background: none; border: none; padding: 4px 8px;
    color: var(--foreground); font-size: 11.5px; cursor: pointer; font-family: inherit; text-align: left;
  }
  .muted { color: var(--muted); }
  .cur { color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .todo-bar ul { list-style: none; margin: 0; padding: 0 8px 6px 22px; display: flex; flex-direction: column; gap: 2px; max-height: 140px; overflow-y: auto; }
  .todo-bar li { display: flex; gap: 6px; align-items: flex-start; font-size: 11.5px; color: var(--foreground); }
  .todo-bar li :global(svg) { flex-shrink: 0; margin-top: 2px; color: var(--muted); }
  .todo-bar li.completed { color: var(--muted); text-decoration: line-through; }
  .todo-bar li.completed :global(svg) { color: var(--success); }
  .todo-bar li.in_progress :global(svg) { color: var(--accent); }
</style>
