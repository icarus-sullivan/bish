<script lang="ts">
  import { get } from 'svelte/store'
  import {
    IconPlus, IconSlash, IconAt, IconPlayerStopFilled, IconArrowUp, IconX, IconEye, IconEyeOff,
    IconFile, IconPhoto, IconChevronDown, IconCpu, IconCheck,
  } from '@tabler/icons-svelte'
  import { AssistantPickFiles, StashDropped, GetAllFiles } from '../../lib/wails'
  import { tabs, activeTabId, activeSelection } from '../../lib/stores'
  import { fuzzyMatch } from '../../lib/fuzzy'
    import { basename, relPath, fmtTokens } from '../../lib/claude/format'
  import type { AgentConversation } from '../../lib/claude/agent'

  let { conv, onModel, onPanelCommand }: {
    conv: AgentConversation
    onModel: () => void
    // panel-handled slash commands (/clear, /model, /resume…); true = consumed
    onPanelCommand: (name: string, args: string) => boolean
  } = $props()

  const IMAGE_EXT = /\.(png|jpe?g|gif|webp)$/i
  const CTX_KEY = 'bish.claude.includeContext'
  const HIST_KEY = 'bish.claude.promptHistory'

  let input = $state('')
  let attached = $state<string[]>([])
  let includeContext = $state(localStorage.getItem(CTX_KEY) !== '0')
  let textareaEl: HTMLTextAreaElement
  let pendingDrop: Promise<void> = Promise.resolve()
  let modeMenu = $state(false)

  const root = $derived(conv.projectRoot)

  // ─── editor context ──────────────────────────────────────────────────────
  const sel = $derived($activeSelection)
  const activeFile = $derived.by(() => {
    const t = $tabs.find(t => t.id === $activeTabId)
    return t && t.type === 'file' && t.path && t.path !== '__new__' ? t.path : null
  })
  const ctxLabel = $derived.by(() => {
    if (sel?.text && sel.path) {
      const end = sel.line + Math.max(0, (sel.lines ?? 1) - 1)
      return `${basename(sel.path)}:${sel.line}${end > sel.line ? '-' + end : ''}`
    }
    return activeFile ? basename(activeFile) : ''
  })

  function buildContext(files: string[]): string {
    const parts: string[] = []
    if (includeContext) {
      if (activeFile) parts.push(`Active file: ${activeFile}`)
      if (sel?.text && sel.path) parts.push(`Selected text (${sel.path}:${sel.line}):\n\`\`\`\n${sel.text}\n\`\`\``)
    }
    if (files.length) parts.push(`Attached files:\n${files.map(p => '- ' + p).join('\n')}`)
    return parts.length ? parts.join('\n\n') + '\n\n---\n\n' : ''
  }

  // ─── @-mentions ──────────────────────────────────────────────────────────
  let allFiles = $state.raw<string[]>([])
  let filesLoadedAt = 0
  let fileSet = new Set<string>()
  async function loadFiles() {
    if (Date.now() - filesLoadedAt < 30_000 || !root) return
    filesLoadedAt = Date.now()
    allFiles = (await GetAllFiles(root).catch(() => [])) ?? []
    fileSet = new Set(allFiles)
    mentionTick++
  }
  let mentionTick = $state(0)
  let caret = $state(0)
  const mentionQuery = $derived.by(() => {
    const m = /(^|\s)@([^\s@]*)$/.exec(input.slice(0, caret))
    return m ? m[2] : null
  })
  let mentionDismissed = $state(false)
  const mentionOpen = $derived(mentionQuery !== null && !mentionDismissed)
  const mentionResults = $derived.by(() => {
    mentionTick
    if (!mentionOpen) return []
    const q = mentionQuery!
    const rels = allFiles.map(f => relPath(f, root))
    if (!q) return rels.slice(0, 30)
    return rels
      .map(r => ({ r, m: fuzzyMatch(q, r) }))
      .filter(x => x.m)
      .sort((a, b) => b.m!.score - a.m!.score)
      .slice(0, 30)
      .map(x => x.r)
  })
  $effect(() => { if (mentionOpen) loadFiles() })

  function applyMention(rel: string) {
    const before = input.slice(0, caret).replace(/@([^\s@]*)$/, '@' + rel + ' ')
    input = before + input.slice(caret)
    requestAnimationFrame(() => { textareaEl.focus(); textareaEl.selectionStart = textareaEl.selectionEnd = caret = before.length })
  }

  function mentionedFiles(text: string): string[] {
    const out: string[] = []
    for (const m of text.matchAll(/(?:^|\s)@(\S+)/g)) {
      const p = m[1].replace(/#L\d+(-L?\d+)?$/, '')
      const abs = p.startsWith('/') ? p : root + '/' + p
      if (fileSet.has(abs) && !out.includes(abs)) out.push(abs)
    }
    return out
  }

  // ─── slash commands ──────────────────────────────────────────────────────
  const PANEL_COMMANDS = [
    { name: '/clear', description: 'Start a new conversation in this tab' },
    { name: '/new', description: 'Open a new conversation tab' },
    { name: '/model', description: 'Switch the model' },
    { name: '/resume', description: 'Browse and resume past conversations' },
    { name: '/mcp', description: 'Show MCP server status' },
    { name: '/context', description: 'Show context window usage' },
  ]
  const slashCommands = $derived.by(() => {
    const seen = new Set<string>()
    const out: { name: string; description: string; hint?: string }[] = []
    for (const c of [...PANEL_COMMANDS, ...conv.commands]) {
      if (seen.has(c.name)) continue
      seen.add(c.name); out.push(c)
    }
    return out
  })
  let slashDismissed = $state(false)
  let menuIdx = $state(0)
  const slashOpen = $derived(!slashDismissed && input.startsWith('/') && !/\s/.test(input))
  const slashResults = $derived.by(() => {
    if (!slashOpen) return []
    const q = input.slice(1)
    if (!q) return slashCommands
    return slashCommands
      .map(c => ({ c, m: fuzzyMatch(q, c.name.slice(1)) }))
      .filter(r => r.m)
      .sort((a, b) => b.m!.score - a.m!.score)
      .map(r => r.c)
  })
  const menuItems = $derived(slashOpen ? slashResults.length : mentionOpen ? mentionResults.length : 0)
  $effect(() => { slashResults; mentionResults; menuIdx = 0 })

  function applySlash(c: { name: string }) {
    slashDismissed = true
    if (PANEL_COMMANDS.some(p => p.name === c.name) && onPanelCommand(c.name, '')) { input = ''; return }
    input = c.name + ' '
    textareaEl?.focus()
  }

  // ─── send ────────────────────────────────────────────────────────────────
  let history: string[] = (() => { try { return JSON.parse(localStorage.getItem(HIST_KEY) || '[]') } catch { return [] } })()
  let histIdx = -1

  export async function send() {
    const text = input.trim()
    if (!text) return
    const [cmd, ...rest] = text.split(/\s+/)
    if (cmd.startsWith('/') && (onPanelCommand(cmd, rest.join(' ')) || conv.runCommand(cmd, rest.join(' ')))) { input = ''; return }
    if (conv.terminalOnly.includes(cmd)) {
      conv.note(`${cmd} needs an interactive terminal — run the CLI in the Terminal panel for it.`)
      input = ''
      return
    }
    await pendingDrop
    history = [text, ...history.filter(h => h !== text)].slice(0, 50)
    try { localStorage.setItem(HIST_KEY, JSON.stringify(history)) } catch {}
    histIdx = -1
    const images = attached.filter(p => IMAGE_EXT.test(p))
    const files = [...attached.filter(p => !IMAGE_EXT.test(p)), ...mentionedFiles(text)]
    const context = cmd.startsWith('/') ? '' : buildContext(files)
    input = ''
    attached = []
    slashDismissed = false
    await conv.send({ text, context, images, files })
  }

  export function focus() { textareaEl?.focus() }
  export function insert(text: string) {
    input = input ? input.replace(/\s*$/, ' ') + text : text
    requestAnimationFrame(() => { textareaEl?.focus(); textareaEl.selectionStart = textareaEl.selectionEnd = input.length })
  }
  export function setText(text: string) { input = text; requestAnimationFrame(() => textareaEl?.focus()) }
  export function attach(paths: string[]) {
    pendingDrop = pendingDrop.then(async () => {
      const stashed = await StashDropped(paths).catch(() => paths)
      for (const p of stashed) if (!attached.includes(p)) attached.push(p)
    })
  }

  async function pickFiles() {
    const paths = await AssistantPickFiles().catch(() => [])
    for (const p of paths ?? []) if (!attached.includes(p)) attached.push(p)
  }

  function cycleMode() {
    const i = conv.modeCycle.indexOf(conv.mode)
    selectMode(conv.modeCycle[(i + 1) % conv.modeCycle.length])
  }
  function selectMode(m: string) {
    modeMenu = false
    const def = conv.modes.find(x => x.id === m)
    if (def?.danger && conv.mode !== m &&
        !confirm(`"${def.label}" lets the agent run any command and edit any file without asking.\n\nOnly use this in a sandbox or a throwaway checkout. Continue?`)) return
    conv.setMode(m)
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.isComposing) return
    if (menuItems) {
      if (e.key === 'ArrowDown') { e.preventDefault(); menuIdx = Math.min(menuIdx + 1, menuItems - 1); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); menuIdx = Math.max(menuIdx - 1, 0); return }
      if (e.key === 'Escape') { e.preventDefault(); slashDismissed = true; mentionDismissed = true; return }
      if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey) {
        e.preventDefault()
        if (slashOpen) applySlash(slashResults[menuIdx]); else applyMention(mentionResults[menuIdx])
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); return }
    if (e.key === 'Tab' && e.shiftKey) { e.preventDefault(); cycleMode(); return }
    if (e.key === 'Escape' && conv.busy) { e.preventDefault(); conv.interrupt(); return }
    if (e.key === 'ArrowUp' && (!input || histIdx >= 0) && history.length) {
      e.preventDefault(); histIdx = Math.min(histIdx + 1, history.length - 1); input = history[histIdx]; return
    }
    if (e.key === 'ArrowDown' && histIdx >= 0) {
      e.preventDefault(); histIdx--; input = histIdx >= 0 ? history[histIdx] : ''
    }
  }
  function onInput() {
    caret = textareaEl.selectionStart
    if (!input) { slashDismissed = false; histIdx = -1 }
    mentionDismissed = false
  }

  // drag the composer's top edge; double-click resets to auto-grow
  const H_KEY = 'bish.assistant.composerHeight'
  let height = $state<number | null>((() => { const n = Number(localStorage.getItem(H_KEY)); return n > 0 ? n : null })())
  function startResize(e: MouseEvent) {
    e.preventDefault()
    const startY = e.clientY, startH = textareaEl.offsetHeight
    const move = (ev: MouseEvent) => { height = Math.round(Math.max(40, Math.min(500, startH + startY - ev.clientY))) }
    const up = () => {
      window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up)
      document.body.style.cursor = ''
      try { if (height) localStorage.setItem(H_KEY, String(height)) } catch {}
    }
    document.body.style.cursor = 'ns-resize'
    window.addEventListener('mousemove', move); window.addEventListener('mouseup', up)
  }
  function resetHeight() { height = null; try { localStorage.removeItem(H_KEY) } catch {} }

  const modeInfo = $derived(conv.modes.find(m => m.id === conv.mode) ?? conv.modes[0])
  const ctxPct = $derived(conv.context ? Math.min(100, conv.context.pct) : 0)
  const modelLabel = $derived(conv.model === 'default' ? (conv.activeModel ? conv.activeModel.replace(/^claude-/, '') : 'Default') : conv.model)
</script>

<div class="composer">
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="resize" role="separator" aria-orientation="horizontal" tabindex="-1"
       title="Drag to resize · double-click to reset" onmousedown={startResize} ondblclick={resetHeight}></div>

  {#if slashOpen || mentionOpen}
    <div class="menu" role="listbox">
      {#if slashOpen}
        {#each slashResults as c, i (c.name)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div class="row" class:active={i === menuIdx} role="option" aria-selected={i === menuIdx} tabindex="-1"
               onclick={() => applySlash(c)} onmouseenter={() => menuIdx = i}>
            <span class="name">{c.name}</span>{#if c.hint}<span class="hint">{c.hint}</span>{/if}
            <span class="desc">{c.description}</span>
          </div>
        {:else}<div class="empty">No matching commands</div>{/each}
      {:else}
        {#each mentionResults as r, i (r)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div class="row" class:active={i === menuIdx} role="option" aria-selected={i === menuIdx} tabindex="-1"
               onclick={() => applyMention(r)} onmouseenter={() => menuIdx = i}>
            <IconFile size={12} /><span class="name">{basename(r)}</span><span class="desc">{r}</span>
          </div>
        {:else}<div class="empty">{allFiles.length ? 'No matching files' : 'Loading files…'}</div>{/each}
      {/if}
    </div>
  {/if}

  <div class="box" class:focusable={true}>
    {#if attached.length || ctxLabel}
      <div class="chips">
        {#if ctxLabel}
          <button class="chip ctx" class:off={!includeContext} title={includeContext ? 'Editor context is sent with your message — click to exclude' : 'Editor context excluded — click to include'}
                  onclick={() => { includeContext = !includeContext; try { localStorage.setItem(CTX_KEY, includeContext ? '1' : '0') } catch {} }}>
            {#if includeContext}<IconEye size={11} />{:else}<IconEyeOff size={11} />{/if}
            {ctxLabel}{#if sel?.text}<span class="sub">selection</span>{/if}
          </button>
        {/if}
        {#each attached as p (p)}
          <span class="chip" title={p}>
            {#if IMAGE_EXT.test(p)}<IconPhoto size={11} />{:else}<IconFile size={11} />{/if}
            {basename(p)}
            <button class="x" aria-label="Remove" onclick={() => attached = attached.filter(f => f !== p)}><IconX size={10} /></button>
          </span>
        {/each}
      </div>
    {/if}
    <textarea
      bind:this={textareaEl} bind:value={input}
      placeholder={conv.busy ? 'Queue another message…' : conv.items.length ? 'Reply…' : `Ask ${conv.caps.provider === 'codex' ? 'Codex' : 'Claude'} to build, fix, or explain…  (@ files, / commands)`}
      rows={2} class:sized={height !== null} style:height={height !== null ? height + 'px' : null}
      onkeydown={onKeydown} oninput={onInput} onclick={() => caret = textareaEl.selectionStart}
      onkeyup={() => caret = textareaEl.selectionStart}
    ></textarea>
    <div class="bar">
      <div class="left">
        <button class="ib" onclick={pickFiles} title="Attach files or images"><IconPlus size={15} /></button>
        <button class="ib" onclick={() => { insert('@'); mentionDismissed = false; caret = input.length }} title="Mention a file"><IconAt size={15} /></button>
        <button class="ib" onclick={() => { if (!input.startsWith('/')) input = '/' + input; slashDismissed = false; textareaEl.focus() }} title="Slash commands"><IconSlash size={15} /></button>
        <div class="mode-wrap">
          <button class="pill" class:danger={!!modeInfo.danger} class:plan={conv.mode === 'plan' || conv.mode === 'read-only'}
                  onclick={() => modeMenu = !modeMenu} title="Permission mode (⇧Tab to cycle)">
            {modeInfo.label} <IconChevronDown size={10} />
          </button>
          {#if modeMenu}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="scrim" onclick={() => modeMenu = false}></div>
            <div class="mode-menu">
              {#each conv.modes as m (m.id)}
                <button class="mode-row" class:active={m.id === conv.mode} class:danger={m.danger} onclick={() => selectMode(m.id)}>
                  <span class="mode-check">{#if m.id === conv.mode}<IconCheck size={12} />{/if}</span>
                  <span class="mode-text"><span class="mode-label">{m.label}</span><span class="mode-desc">{m.desc}</span></span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      </div>
      <div class="right">
        {#if conv.context}
          <span class="meter" title={`Context: ${fmtTokens(conv.context.total)} / ${fmtTokens(conv.context.max)} tokens (${Math.round(ctxPct)}%)`}>
            <svg width="14" height="14" viewBox="0 0 16 16">
              <circle cx="8" cy="8" r="6" fill="none" stroke="var(--border)" stroke-width="2.5" />
              <circle cx="8" cy="8" r="6" fill="none" stroke-width="2.5" stroke-linecap="round"
                      stroke={ctxPct > 85 ? 'var(--error)' : ctxPct > 65 ? 'var(--warning)' : 'var(--accent)'}
                      stroke-dasharray={`${(ctxPct / 100) * 37.7} 37.7`} transform="rotate(-90 8 8)" />
            </svg>
            {Math.round(ctxPct)}%
          </span>
        {/if}
        <button class="pill model" onclick={onModel} title="Switch model"><IconCpu size={11} /> {modelLabel}</button>
        {#if conv.busy}
          <button class="send stop" onclick={() => conv.interrupt()} title="Stop (Esc)"><IconPlayerStopFilled size={14} /></button>
        {:else}
          <button class="send" disabled={!input.trim()} onclick={send} title="Send (Enter)"><IconArrowUp size={15} /></button>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .composer { position: relative; padding: 6px 10px 10px; flex-shrink: 0; }
  .resize { position: absolute; top: -3px; left: 0; right: 0; height: 6px; cursor: ns-resize; z-index: 2; }
  .resize:hover { background: color-mix(in srgb, var(--accent) 40%, transparent); }
  .box {
    border: 1px solid var(--border); border-radius: 8px; background: var(--background);
    transition: border-color 0.1s;
  }
  .box:focus-within { border-color: var(--accent); }
  textarea {
    display: block; width: 100%; resize: none; background: none; border: none; outline: none;
    color: var(--foreground); font-size: 12.5px; line-height: 1.5; padding: 8px 10px 4px; font-family: inherit;
    box-sizing: border-box; field-sizing: content; min-height: 44px; max-height: 240px; overflow-y: auto;
  }
  textarea.sized { field-sizing: fixed; max-height: none; }
  .chips { display: flex; flex-wrap: wrap; gap: 4px; padding: 6px 8px 0; }
  .chip {
    display: inline-flex; align-items: center; gap: 4px; font-size: 10.5px; color: var(--foreground);
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 4px; padding: 1px 5px;
    max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: inherit;
  }
  button.chip { cursor: pointer; }
  .chip.ctx.off { color: var(--muted); text-decoration: line-through; }
  .chip .sub { color: var(--muted); }
  .x { display: flex; background: none; border: none; color: var(--muted); cursor: pointer; padding: 0; }
  .x:hover { color: var(--foreground); }
  .bar { display: flex; align-items: center; justify-content: space-between; padding: 2px 6px 5px; gap: 6px; }
  .left, .right { display: flex; align-items: center; gap: 3px; min-width: 0; }
  .ib {
    display: flex; align-items: center; justify-content: center; background: none; border: none;
    color: var(--muted); cursor: pointer; padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .ib:hover { color: var(--foreground); background: var(--bg-hover); }
  .pill {
    display: flex; align-items: center; gap: 4px; white-space: nowrap; max-width: 150px; overflow: hidden;
    background: none; border: 1px solid transparent; border-radius: 4px; color: var(--muted);
    font-size: 11px; padding: 2px 6px; cursor: pointer; font-family: inherit;
  }
  .pill:hover { color: var(--foreground); background: var(--bg-hover); }
  .pill.plan { color: var(--accent); }
  .pill.danger { color: var(--error); border-color: color-mix(in srgb, var(--error) 50%, transparent); }
  .meter { display: flex; align-items: center; gap: 3px; font-size: 10.5px; color: var(--muted); padding: 0 4px; }
  .send {
    display: flex; align-items: center; justify-content: center; width: 24px; height: 24px; flex-shrink: 0;
    border-radius: 6px; border: none; cursor: pointer; background: var(--accent); color: var(--background);
  }
  .send:disabled { opacity: 0.35; cursor: default; }
  .send.stop { background: var(--foreground); }

  .mode-wrap { position: relative; }
  .scrim { position: fixed; inset: 0; z-index: 20; }
  .mode-menu {
    position: absolute; bottom: calc(100% + 6px); left: 0; z-index: 21; width: 270px;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 7px; padding: 4px;
    box-shadow: 0 8px 24px color-mix(in srgb, #000 40%, transparent);
  }
  .mode-row {
    display: flex; gap: 6px; width: 100%; text-align: left; background: none; border: none; border-radius: 4px;
    padding: 5px 6px; cursor: pointer; color: var(--foreground); font-family: inherit;
  }
  .mode-row:hover { background: var(--bg-hover); }
  .mode-row.danger .mode-label { color: var(--error); }
  .mode-check { width: 12px; flex-shrink: 0; color: var(--accent); padding-top: 1px; }
  .mode-text { display: flex; flex-direction: column; gap: 1px; }
  .mode-label { font-size: 12px; font-weight: 600; }
  .mode-desc { font-size: 10.5px; color: var(--muted); }

  .menu {
    position: absolute; left: 10px; right: 10px; bottom: calc(100% - 2px); z-index: 10;
    max-height: 240px; overflow-y: auto; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 7px; padding: 4px;
    box-shadow: 0 8px 24px color-mix(in srgb, #000 40%, transparent);
  }
  .row { display: flex; align-items: center; gap: 7px; padding: 4px 7px; border-radius: 4px; cursor: pointer; font-size: 12px; color: var(--muted); }
  .row.active { background: var(--bg-selected); }
  .row .name { font-family: "SF Mono", Menlo, monospace; color: var(--foreground); flex-shrink: 0; }
  .row .hint { font-size: 10.5px; color: var(--muted); flex-shrink: 0; }
  .row .desc { font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .empty { padding: 8px; font-size: 11px; color: var(--muted); text-align: center; }
</style>
