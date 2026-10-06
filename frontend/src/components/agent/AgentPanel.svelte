<script lang="ts">
  // Agent panel shell (one per provider — Claude Code, Codex): conversation
  // tabs, history, MCP status, and per-panel settings. Each tab is an
  // AgentConversation with its own CLI subprocess, spawned lazily on that
  // tab's first message. Both providers' shells stay mounted side by side, so
  // conversations keep running while you look at the other one.
  import { onMount, untrack } from 'svelte'
  import { get } from 'svelte/store'
  import {
    IconPlus, IconHistory, IconX, IconSettings, IconPlug, IconArrowLeft, IconRefresh, IconChevronDown, IconLoader2,
  } from '@tabler/icons-svelte'
  import { projectRoot, cwd, activeSelection, activeRightPanel, showRight } from '../../lib/stores'
  import { registerCommand } from '../../lib/commands'
  import { registerKeybind } from '../../lib/keybinds'
  import type { UserItem } from '../../lib/claude/conversation.svelte'
  import type { AgentConversation } from '../../lib/claude/agent'
  import type { Provider, PanelPrefs } from '../../lib/claude/providers'
  import { relPath, fmtTokens, fmtCost } from '../../lib/claude/format'
  import type { SessionSummary } from '../claude/types'
  import ChatView from '../claude/ChatView.svelte'
  import HistoryView from '../claude/HistoryView.svelte'
  import SwitchModelDialog from '../SwitchModelDialog.svelte'

  let { provider, visible, onShow, onActivity }: {
    provider: Provider; visible: boolean; onShow: () => void
    onActivity?: (busy: boolean, asks: number) => void
  } = $props()
  // svelte-ignore state_referenced_locally
  const P = provider
  const isClaude = P.id === 'claude'

  // Claude keeps its original storage keys so existing prefs carry over
  const PERM_KEY = isClaude ? 'bish.assistant.permissionMode' : 'bish.codex.mode'
  const MODEL_KEY = isClaude ? 'bish.assistant.model' : 'bish.codex.model'
  const EFFORT_KEY = `bish.${P.id}.effort`
  const THINK_KEY = `bish.${P.id}.thinking`
  const TABS_KEY = (root: string) => `bish.${P.id}.tabs:` + root

  function ls(k: string, d: string) { try { return localStorage.getItem(k) ?? d } catch { return d } }
  function lsSet(k: string, v: string) { try { localStorage.setItem(k, v) } catch {} }

  // only the provider's safe modes can become the sticky default — a fresh
  // conversation never starts in bypass / full-access
  const SAFE_DEFAULTS = new Set(P.defaultModes.map(m => m.id))
  let defaultMode = $state(SAFE_DEFAULTS.has(ls(PERM_KEY, P.defaultMode)) ? ls(PERM_KEY, P.defaultMode) : P.defaultMode)
  let defaultModel = $state(ls(MODEL_KEY, 'default'))
  let effort = $state(ls(EFFORT_KEY, ''))
  let thinking = $state(ls(THINK_KEY, ''))

  const root = $derived($projectRoot || $cwd)
  function prefs(): PanelPrefs { return { mode: defaultMode, model: defaultModel, effort, thinking } }

  let convs = $state<AgentConversation[]>([])
  let activeKey = $state('')
  const active = $derived(convs.find(c => c.key === activeKey) ?? convs[0])
  let chatViews: Record<string, ChatView> = $state({})
  let view = $state<'chat' | 'history' | 'mcp'>('chat')
  let showModelDialog = $state(false)
  let settingsOpen = $state(false)

  // tabs opened from History/persistence render their transcript on first view
  const pendingLoad = new Map<string, { id: string; title: string }>()

  function makeConv(r = root): AgentConversation {
    const c = P.create(r, prefs())
    c.onTitle = persistTabs
    return c
  }

  function newTab(): AgentConversation {
    const c = makeConv()
    convs.push(c)
    activeKey = c.key
    view = 'chat'
    persistTabs()
    requestAnimationFrame(() => chatViews[c.key]?.focus())
    return c
  }

  function closeTab(c: AgentConversation) {
    if (c.busy && !confirm(`${P.name} is still working in this conversation. Stop it and close the tab?`)) return
    c.dispose()
    pendingLoad.delete(c.key)
    const i = convs.indexOf(c)
    convs.splice(i, 1)
    if (!convs.length) newTab()
    else if (activeKey === c.key) activeKey = convs[Math.max(0, i - 1)].key
    persistTabs()
  }

  function activate(c: AgentConversation) {
    activeKey = c.key
    c.unread = false
    view = 'chat'
    const p = pendingLoad.get(c.key)
    if (p) {
      pendingLoad.delete(c.key)
      P.open(c, p.id, p.title).catch(e => c.note(`Couldn't load conversation: ${e}`, 'error'))
    }
  }

  // ─── persistence: open tabs per project (session ids only) ───────────────
  function persistTabs() {
    if (!root) return
    const list = convs.filter(c => c.sessionId).map(c => ({ id: c.sessionId, title: c.title }))
    lsSet(TABS_KEY(root), JSON.stringify(list))
  }
  $effect(() => {
    // re-persist when any tab learns its session id
    convs.map(c => c.sessionId).join()
    persistTabs()
  })

  // report busy/waiting state up to the provider switcher
  $effect(() => {
    const busy = convs.some(c => c.busy)
    const asks = convs.reduce((n, c) => n + c.pendingAsks.length, 0)
    untrack(() => onActivity?.(busy, asks))
  })

  let restoredFor = ''
  $effect(() => {
    const r = root
    if (!r || restoredFor === r) return
    restoredFor = r
    untrack(() => restoreTabs(r))
  })
  function restoreTabs(r: string) {
    for (const c of convs) c.dispose()
    convs = []
    let saved: { id: string; title: string }[] = []
    try { saved = JSON.parse(ls(TABS_KEY(r), '[]')) } catch {}
    for (const s of saved.slice(0, 8)) {
      const c = makeConv(r)
      c.title = s.title
      pendingLoad.set(c.key, s)
      convs.push(c)
    }
    if (!convs.length) convs.push(makeConv(r))
    activate(convs[convs.length - 1])
    loadHistory()
  }

  // ─── history ─────────────────────────────────────────────────────────────
  let sessions = $state<SessionSummary[]>([])
  let historyLoading = $state(false)
  let historyError = $state('')
  async function loadHistory() {
    if (!root) return
    historyLoading = true
    historyError = ''
    try { sessions = await P.listHistory(root) }
    catch (e) { historyError = String(e) }
    finally { historyLoading = false }
  }
  function openHistory() { view = 'history'; loadHistory() }

  function resume(s: SessionSummary) {
    const open = convs.find(c => c.sessionId === s.id || pendingLoad.get(c.key)?.id === s.id)
    if (open) { activate(open); return }
    // reuse an untouched tab instead of piling up empties
    let c = active && active.items.length === 0 && !active.sessionId && !pendingLoad.has(active.key) ? active : null
    if (!c) { c = makeConv(); convs.push(c) }
    pendingLoad.set(c.key, { id: s.id, title: s.title })
    activate(c)
  }

  function fork(src: AgentConversation, u: UserItem) {
    if (!P.fork) return
    const c = P.fork(src, u, src.projectRoot, prefs())
    c.onTitle = persistTabs
    convs.push(c)
    activate(c)
  }

  // ─── settings ────────────────────────────────────────────────────────────
  function selectModel(m: string) {
    showModelDialog = false
    defaultModel = m
    lsSet(MODEL_KEY, m)
    active?.setModel(m)
  }
  function setEffort(v: string) { effort = v; lsSet(EFFORT_KEY, v); active?.setSpawnOption('effort', v) }
  function setThinking(v: string) { thinking = v; lsSet(THINK_KEY, v); active?.setSpawnOption('thinking', v) }
  function setDefaultMode(v: string) { defaultMode = v; lsSet(PERM_KEY, v) }

  // ─── MCP ─────────────────────────────────────────────────────────────────
  let mcpServers = $state<any[]>([])
  let mcpLoading = $state(false)
  async function openMcp() {
    view = 'mcp'
    mcpLoading = true
    mcpServers = active ? await active.mcpStatus() : []
    mcpLoading = false
  }

  // ─── panel-handled slash commands ────────────────────────────────────────
  function panelCommand(name: string, _args: string): boolean {
    const c = active
    switch (name) {
      case '/clear': {
        if (!c) return true
        const i = convs.indexOf(c)
        c.dispose()
        const n = makeConv()
        convs[i] = n
        activate(n)
        persistTabs()
        return true
      }
      case '/new': newTab(); return true
      case '/model': showModelDialog = true; return true
      case '/resume': openHistory(); return true
      case '/mcp': openMcp(); return true
      case '/context':
        if (!c?.handle) { c?.note('Context usage is available once the conversation has started.'); return true }
        c.refreshContext().then(() => {
          const u = c.context
          if (!u) { c.note('Context usage unavailable for this CLI version.'); return }
          const rows = u.categories.map(x => `  ${x.name.padEnd(22)} ${fmtTokens(x.tokens).padStart(7)}`).join('\n')
          c.note(`Context: ${fmtTokens(u.total)} / ${fmtTokens(u.max)} tokens (${Math.round(u.pct)}%)\n${rows}`, 'command')
        })
        return true
    }
    return false
  }

  // ─── editor integration ──────────────────────────────────────────────────
  function focusPanel() {
    onShow()
    showRight.set(true)
    activeRightPanel.set('assistant')
    view = 'chat'
    requestAnimationFrame(() => active && chatViews[active.key]?.focus())
  }
  function addSelection() {
    const sel = get(activeSelection)
    focusPanel()
    if (!sel?.path || !active) return
    const rel = relPath(sel.path, active.projectRoot)
    const end = sel.line + Math.max(0, (sel.lines ?? 1) - 1)
    requestAnimationFrame(() => chatViews[active.key]?.insert(`@${rel}${sel.text ? `#L${sel.line}${end > sel.line ? '-' + end : ''}` : ''} `))
  }

  onMount(() => {
    // ⌘Esc / ⌘⌥K belong to Claude (VS Code parity); ⌘⇧Esc / ⌘⌥J to Codex
    const focusKey = isClaude ? 'mod+escape' : 'mod+shift+escape'
    const selKeys = isClaude ? ['mod+alt+k', 'mod+alt+˚'] : ['mod+alt+j', 'mod+alt+∆'] // second = macOS ⌥-layout key
    const offs = [
      registerKeybind({ combo: focusKey, handler: (e) => { e.preventDefault(); focusPanel() } }),
      ...selKeys.map(combo => registerKeybind({ combo, handler: (e) => { e.preventDefault(); addSelection() } })),
    ]
    const label = isClaude ? 'Claude' : 'Codex'
    const cmds = [
      { id: `${P.id}.focus`, title: `${label}: Focus Chat`, run: focusPanel, key: focusKey },
      { id: `${P.id}.addSelection`, title: `${label}: Add Selection to Chat`, run: addSelection, key: selKeys[0] },
      { id: `${P.id}.new`, title: `${label}: New Conversation`, run: () => { focusPanel(); newTab() } },
      { id: `${P.id}.history`, title: `${label}: Past Conversations…`, run: () => { focusPanel(); openHistory() } },
      { id: `${P.id}.stop`, title: `${label}: Stop`, run: () => active?.interrupt(), when: () => !!active?.busy },
      { id: `${P.id}.model`, title: `${label}: Switch Model…`, run: () => { focusPanel(); showModelDialog = true } },
    ]
    for (const c of cmds) offs.push(registerCommand(c))
    return () => {
      offs.forEach(o => o())
      for (const c of convs) c.dispose()
      P.dispose()
    }
  })

  let container: HTMLDivElement
  $effect(() => {
    const el = container
    if (!el) return
    const onDrop = (e: Event) => {
      if (!visible) return
      const paths: string[] = (e as CustomEvent).detail.paths
      if (active) { view = 'chat'; chatViews[active.key]?.attach(paths) }
    }
    el.addEventListener('bish:filedrop', onDrop)
    return () => el.removeEventListener('bish:filedrop', onDrop)
  })

  const MCP_COLORS: Record<string, string> = { connected: 'var(--success)', failed: 'var(--error)', 'needs-auth': 'var(--warning)', pending: 'var(--muted)', disabled: 'var(--muted)' }
</script>

<div class="panel" bind:this={container}>
  <div class="header">
    <div class="tabs" role="tablist">
      {#each convs as c (c.key)}
        <div class="tab" class:active={c.key === active?.key && view === 'chat'} role="tab" tabindex="0"
             aria-selected={c.key === active?.key}
             onclick={() => activate(c)} onkeydown={(e) => { if (e.key === 'Enter') activate(c) }}
             onauxclick={(e) => { if (e.button === 1) closeTab(c) }} title={c.title}>
          {#if c.busy && !c.pendingAsks.length}<IconLoader2 size={10} class="spin" />
          {:else if c.pendingAsks.length}<span class="dot ask"></span>
          {:else if c.unread && c.key !== active?.key}<span class="dot"></span>{/if}
          <span class="tab-title">{c.title}</span>
          {#if convs.length > 1 || c.items.length}
            <button class="tab-x" aria-label="Close conversation" onclick={(e) => { e.stopPropagation(); closeTab(c) }}><IconX size={10} /></button>
          {/if}
        </div>
      {/each}
    </div>
    <div class="header-actions">
      <button class="hdr-btn" onclick={newTab} title="New conversation"><IconPlus size={13} /></button>
      <button class="hdr-btn" onclick={openHistory} title="Past conversations"><IconHistory size={13} /></button>
      <button class="hdr-btn" onclick={openMcp} title="MCP servers"><IconPlug size={13} /></button>
      <button class="hdr-btn" onclick={() => settingsOpen = !settingsOpen} title="{P.name} settings"><IconSettings size={13} /></button>
    </div>
    {#if settingsOpen}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="scrim" onclick={() => settingsOpen = false}></div>
      <div class="settings">
        <label>
          <span>Default mode for new chats</span>
          <span class="select-wrap">
            <select value={defaultMode} onchange={(e) => setDefaultMode((e.currentTarget as HTMLSelectElement).value)}>
              {#each P.defaultModes as m (m.id)}<option value={m.id}>{m.label}</option>{/each}
            </select>
            <IconChevronDown size={13} class="select-chevron" />
          </span>
        </label>
        <label>
          <span>Effort</span>
          <span class="select-wrap">
            <select value={effort} onchange={(e) => setEffort((e.currentTarget as HTMLSelectElement).value)}>
              {#each P.efforts as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
            </select>
            <IconChevronDown size={13} class="select-chevron" />
          </span>
        </label>
        {#if P.thinking}
        <label>
          <span>Extended thinking</span>
          <span class="select-wrap">
            <select value={thinking} onchange={(e) => setThinking((e.currentTarget as HTMLSelectElement).value)}>
              <option value="">Default</option>
              <option value="adaptive">Adaptive</option>
              <option value="disabled">Off</option>
            </select>
            <IconChevronDown size={13} class="select-chevron" />
          </span>
        </label>
        {/if}
        {#if active}
          <div class="facts">
            {#if active.activeModel}<div><span>Model</span>{active.activeModel}</div>{/if}
            {#if active.costUsd}<div><span>Session cost</span>{fmtCost(active.costUsd)}</div>{/if}
            {#if active.info?.account?.email}<div><span>Account</span>{active.info.account.email}</div>{/if}
            {#if (active as any).init?.claude_code_version}<div><span>CLI</span>v{(active as any).init.claude_code_version}</div>{/if}
            {#if active.sessionId}<div><span>Session</span><code>{active.sessionId.slice(0, 8)}</code></div>{/if}
          </div>
        {/if}
        <div class="note">{P.thinking ? 'Effort and thinking apply from the next message (the session restarts in place).' : 'Effort, model, and mode apply from the next message.'}</div>
      </div>
    {/if}
  </div>

  <div class="body">
    {#each convs as c (c.key)}
      <div class="view" style:display={view === 'chat' && c.key === active?.key ? 'flex' : 'none'}>
        <ChatView bind:this={chatViews[c.key]} conv={c} recent={sessions.filter(s => !convs.some(x => x.sessionId === s.id))}
                  onResume={resume} onFork={(u) => fork(c, u)} onModel={() => showModelDialog = true} onPanelCommand={panelCommand} />
      </div>
    {/each}
    {#if view === 'history'}
      <div class="view"><HistoryView {sessions} loading={historyLoading} error={historyError} onOpen={resume} onBack={() => view = 'chat'} /></div>
    {:else if view === 'mcp'}
      <div class="view col">
        <div class="sub-top">
          <button class="hdr-btn" onclick={() => view = 'chat'} title="Back"><IconArrowLeft size={13} /></button>
          <span class="sub-title">MCP servers</span>
          <button class="hdr-btn" onclick={openMcp} title="Refresh"><IconRefresh size={13} /></button>
        </div>
        <div class="mcp-list">
          {#if mcpLoading}
            <div class="empty"><IconLoader2 size={13} class="spin" /> Loading…</div>
          {:else if !mcpServers.length}
            <div class="empty">{active?.handle ? 'No MCP servers configured.' : 'Start the conversation to see its MCP servers.'}<br />
              Add servers with <code>{P.id} mcp add</code> in the terminal.</div>
          {/if}
          {#each mcpServers as s (s.name)}
            <div class="mcp-row">
              <span class="mcp-dot" style:background={MCP_COLORS[s.status] ?? 'var(--muted)'}></span>
              <div class="mcp-text">
                <span class="mcp-name">{s.name}</span>
                <span class="mcp-status">{s.status}{s.serverInfo?.version ? ` · v${s.serverInfo.version}` : ''}{s.error ? ` — ${s.error}` : ''}</span>
              </div>
              {#if active?.handle && !s.readOnly}
                {#if s.status === 'failed'}<button class="mini" onclick={async () => { await active.mcpReconnect(s.name); openMcp() }}>Reconnect</button>{/if}
                <button class="mini" onclick={async () => { await active.mcpToggle(s.name, s.status === 'disabled'); openMcp() }}>{s.status === 'disabled' ? 'Enable' : 'Disable'}</button>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</div>

{#if showModelDialog}
  <SwitchModelDialog current={active?.model ?? defaultModel}
    models={active?.info?.models?.length ? [{ value: 'default', displayName: 'Default', description: `${P.name}'s configured default` }, ...active.info.models.filter((m: any) => m.value !== 'default')] : (isClaude ? [] : [{ value: 'default', displayName: 'Default', description: 'Start the conversation to load the full model list' }])}
    onSelect={selectModel} onClose={() => showModelDialog = false} />
{/if}

<style>
  .panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .header {
    position: relative; display: flex; align-items: center; gap: 6px; padding: 0 8px 0 6px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border); color: var(--muted);
  }
  .tabs { display: flex; align-items: stretch; gap: 2px; min-width: 0; flex: 1; overflow-x: auto; scrollbar-width: none; height: 100%; }
  .tabs::-webkit-scrollbar { display: none; }
  .tab {
    display: flex; align-items: center; gap: 4px; padding: 0 4px 0 7px; font-size: 11px; color: var(--muted);
    cursor: pointer; max-width: 150px; flex-shrink: 0; border-bottom: 2px solid transparent; user-select: none;
  }
  .tab:hover { color: var(--foreground); }
  .tab.active { color: var(--foreground); border-bottom-color: var(--accent); }
  .tab-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tab-x { display: flex; background: none; border: none; color: var(--muted); cursor: pointer; padding: 2px; border-radius: 3px; opacity: 0; }
  .tab:hover .tab-x, .tab.active .tab-x { opacity: 1; }
  .tab-x:hover { color: var(--foreground); background: var(--bg-hover); }
  .dot { width: 6px; height: 6px; border-radius: 50%; background: var(--accent); flex-shrink: 0; }
  .dot.ask { background: var(--warning); }
  .header-actions { display: flex; align-items: center; gap: 2px; flex-shrink: 0; }
  .hdr-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    padding: 3px 4px;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .scrim { position: fixed; inset: 0; z-index: 30; }
  .settings {
    position: absolute; top: calc(100% + 4px); right: 8px; z-index: 31; width: 250px;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 8px; padding: 10px;
    box-shadow: 0 10px 30px color-mix(in srgb, #000 45%, transparent);
    display: flex; flex-direction: column; gap: 8px;
  }
  .settings label { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 11.5px; color: var(--foreground); }
  .facts { border-top: 1px solid var(--border); padding-top: 8px; display: flex; flex-direction: column; gap: 3px; font-size: 11px; color: var(--foreground); }
  .facts div { display: flex; justify-content: space-between; gap: 8px; }
  .facts span { color: var(--muted); }
  .facts div { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .settings .note { font-size: 10.5px; color: var(--muted); }

  .select-wrap { position: relative; display: inline-flex; align-items: center; }
  select {
    appearance: none; -webkit-appearance: none;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 11px; padding: 4px 26px 4px 8px; outline: none; cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  select:hover { background: var(--bg-hover); }
  select:focus { border-color: var(--accent); }
  option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) { position: absolute; right: 9px; color: var(--muted); pointer-events: none; }

  .body { flex: 1; min-height: 0; position: relative; }
  .view { position: absolute; inset: 0; display: flex; flex-direction: column; }
  .view.col { background: var(--background); }
  .sub-top { display: flex; align-items: center; gap: 6px; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .sub-title { flex: 1; font-size: 12px; font-weight: 600; color: var(--foreground); }
  .mcp-list { flex: 1; overflow-y: auto; padding: 6px 10px; display: flex; flex-direction: column; gap: 4px; }
  .mcp-row { display: flex; align-items: center; gap: 8px; padding: 6px; border-radius: 5px; border: 1px solid var(--border); background: var(--bg-raised); }
  .mcp-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
  .mcp-text { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .mcp-name { font-size: 12px; color: var(--foreground); font-weight: 600; }
  .mcp-status { font-size: 10.5px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mini {
    background: var(--background); border: 1px solid var(--border); border-radius: 4px; color: var(--foreground);
    font-size: 10.5px; padding: 2px 7px; cursor: pointer;
  }
  .mini:hover { border-color: var(--accent); }
  .empty { display: flex; flex-direction: column; align-items: center; gap: 6px; text-align: center; padding: 20px; font-size: 12px; color: var(--muted); }
</style>
