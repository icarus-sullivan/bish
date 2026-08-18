<script lang="ts">
  import { onMount } from 'svelte'
  import { focusedPane, galleryMode, cwd, showRight, activeRightPanel,
           rightWidth, currentThemeName, panelSide, floatingPanels,
           showPalette, showActionPalette, showGlobalSearch, searchScopeDir, tabs, activeTabId, closeTab, reopenMainTab,
           addTerminalTab, cycleTab, gitBranch, activeSelection, showOpenRemote, shareDialogTerminalId,
           shareDialogFilePath, showWelcomeTour, showShortcutsOverlay } from './lib/stores'
  import { get } from 'svelte/store'
  import { initEvents } from './lib/events'
  import { registerKeybind } from './lib/keybinds'
  import { featureOn } from './lib/features'
  import { registerBuiltinCommands } from './lib/builtinCommands'
  import { applyCustomKeybinds } from './lib/keymap'
  import Terminal from './components/Terminal.svelte'
  import RightSidebar from './components/RightSidebar.svelte'
  import FloatingWindow from './components/FloatingWindow.svelte'
  import { panels } from './lib/panels'
  import FileViewer from './components/FileViewer.svelte'
  import MediaViewer from './components/MediaViewer.svelte'
  import Gallery from './components/Gallery.svelte'
  import TabBar from './components/TabBar.svelte'
  import { GetConfig, NewTerminal, CloseTerminal } from './lib/wails'
  import {
    IconLayoutSidebarRight, IconLayoutSidebarRightFilled,
    IconLayoutSidebar, IconLayoutSidebarFilled, IconGitBranch, IconFolder,
  } from '@tabler/icons-svelte'
  import CommandPalette from './components/CommandPalette.svelte'
  import ActionPalette from './components/ActionPalette.svelte'
  import GlobalSearch from './components/GlobalSearch.svelte'
  import ProcessLogs from './components/ProcessLogs.svelte'
  import Settings from './components/Settings.svelte'
  import DiffViewer from './components/DiffViewer.svelte'
  import MergeConflict from './components/MergeConflict.svelte'
  import OpenRemoteDialog from './components/OpenRemoteDialog.svelte'
  import LiveShareDialog from './components/LiveShareDialog.svelte'
  import EditShareDialog from './components/EditShareDialog.svelte'
  import WelcomeTour from './components/WelcomeTour.svelte'
  import ShortcutsOverlay from './components/ShortcutsOverlay.svelte'
  import { OpenProject, GetRecentProjects, OpenRecentProject, type RecentEntry } from './lib/wails'
  import { projectRoot } from './lib/stores'
  import lightModeIcon from './assets/light_mode.svg'
  import darkModeIcon from './assets/dark_mode.svg'

  type Pane = 'processes' | 'commands' | 'terminal' | 'tree'
  const paneOrder: Pane[] = ['processes', 'commands', 'terminal', 'tree']

  // ─── status bar: VSCode-style file info ────────────────────────────────────
  const LANG_LABELS: Record<string, string> = {
    js: 'JavaScript', mjs: 'JavaScript', cjs: 'JavaScript',
    ts: 'TypeScript', tsx: 'TypeScript React', jsx: 'JavaScript React',
    py: 'Python', css: 'CSS', svelte: 'Svelte', html: 'HTML', vue: 'Vue',
    json: 'JSON', md: 'Markdown', markdown: 'Markdown', yaml: 'YAML', yml: 'YAML',
    go: 'Go', sh: 'Shell Script', bash: 'Shell Script', zsh: 'Shell Script', fish: 'Shell Script',
  }
  function langLabel(p: string): string {
    const ext = p.split('.').pop()?.toLowerCase() ?? ''
    return LANG_LABELS[ext] ?? (ext ? ext.toUpperCase() : 'Plain Text')
  }
  function indentLabel(indent: string): string {
    return indent === '\t' ? 'Tab Size: 4' : `Spaces: ${indent.length}`
  }
  const activeTab = $derived($tabs.find(t => t.id === $activeTabId))

  let recentProjects: RecentEntry[] = $state([])

  onMount(() => {
    (async () => {
      await initEvents()
      try {
        const cfg: any = await GetConfig()
        currentThemeName.set(cfg?.theme || 'obsidian')
        if (!cfg?.onboarding_seen && get(tabs).length === 0) showWelcomeTour.set(true)
      } catch {}
    })()

    GetRecentProjects().then(r => { recentProjects = r ?? [] }).catch(() => {})

    registerBuiltinCommands()
    applyCustomKeybinds()

    // Cmd+N/O/P are handled natively (main.go menu accelerators emit
    // file:new/palette:open/project:change, wired up in initEvents above) —
    // no JS keydown branch for them, or they'd double-fire.
    const offs = [
      registerKeybind({
        combo: 'mod+shift+f',
        handler: (e) => { e.preventDefault(); searchScopeDir.set(null); showGlobalSearch.update(v => !v) },
      }),
      registerKeybind({
        combo: 'mod+shift+p',
        handler: (e) => { if (!featureOn('commandPalette')) return; e.preventDefault(); showActionPalette.update(v => !v) },
      }),
      registerKeybind({
        combo: 'mod+shift+t',
        when: () => featureOn('keyboardShortcuts'),
        handler: async (e) => { e.preventDefault(); try { addTerminalTab(await NewTerminal()) } catch {} },
      }),
      registerKeybind({
        combo: 'mod+w',
        when: () => featureOn('keyboardShortcuts'),
        handler: (e) => {
          e.preventDefault()
          const id = get(activeTabId)
          const t = get(tabs).find(t => t.id === id)
          if (!t) return
          if (t.type === 'terminal' && id !== 'main') CloseTerminal(id)
          closeTab(id)
        },
      }),
      registerKeybind({
        combo: 'mod+shift+]',
        when: () => featureOn('keyboardShortcuts'),
        handler: (e) => { e.preventDefault(); cycleTab(1) },
      }),
      registerKeybind({
        combo: 'mod+shift+[',
        when: () => featureOn('keyboardShortcuts'),
        handler: (e) => { e.preventDefault(); cycleTab(-1) },
      }),
      registerKeybind({
        combo: 'mod+t',
        handler: (e) => {
          e.preventDefault()
          focusedPane.update(p => paneOrder[(paneOrder.indexOf(p) + 1) % paneOrder.length])
          // sidebar follows the focused pane (tree lives in the 'files' panel)
          const pane = get(focusedPane)
          if (pane !== 'terminal') {
            activeRightPanel.set(pane === 'tree' ? 'files' : pane)
            showRight.set(true)
          }
        },
      }),
      registerKeybind({
        combo: 'enter',
        // fire when focus is elsewhere OR no terminal tab exists (closing the
        // terminal tab leaves focusedPane stuck on 'terminal')
        when: () => get(focusedPane) !== 'terminal' || !get(tabs).some(t => t.type === 'terminal'),
        handler: (e) => {
          // skip contexts where Enter means something else (tree row open,
          // button activation) beyond the registry's generic editable guard
          const t = e.target as HTMLElement
          if (t.tagName === 'BUTTON' || t.closest?.('[role="treeitem"], [role="button"]')) return
          e.preventDefault()
          const termTab = get(tabs).find(t => t.type === 'terminal')
          if (termTab) activeTabId.set(termTab.id)
          else reopenMainTab()
          focusedPane.set('terminal')
        },
      }),
      registerKeybind({
        combo: 'shift+?',
        handler: (e) => { e.preventDefault(); showShortcutsOverlay.update(v => !v) },
      }),
      registerKeybind({
        combo: 'escape',
        handler: (e) => {
          // If focus was inside a CM search panel, CM already handled it — don't also close the tab.
          // e.target retains its ancestor chain even after CM removes the panel from the DOM.
          if ((e.target as HTMLElement).closest?.('.cm-search')) return
          // CM consumed it (dismissed autocomplete, cancelled selection) — not a close request
          if (e.defaultPrevented) return
          const active = get(activeTabId)
          const t = get(tabs).find(tt => tt.id === active)
          if (t && t.type !== 'terminal') closeTab(active)
        },
      }),
    ]
    return () => offs.forEach(off => off())
  })

  async function openProject() {
    await OpenProject().catch(() => {})
  }

  async function openRecent(path: string) {
    await OpenRecentProject(path).catch(() => {})
  }

  function startResize(e: MouseEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startRight = $rightWidth
    // dragging the handle grows the sidebar toward the center-col — that's
    // leftward motion when docked right, rightward motion when docked left
    const sign = $panelSide === 'left' ? 1 : -1

    function onMove(ev: MouseEvent) {
      // 220 is the narrowest a Processes row (play/stop/dot/port badge/trash,
      // name already collapsed to 0) still lays out without clipping
      rightWidth.set(Math.max(220, Math.min(500, startRight + sign * (ev.clientX - startX))))
    }
    function onUp() {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
</script>


<div class="root">

  <!-- ─── titlebar ─── -->
  <div class="titlebar">
    <div class="traffic-spacer" style="--wails-draggable:drag"></div>
    <div class="toolbar">

      <div class="tb-fill" style="--wails-draggable:drag"></div>

      <div class="panel-toggles" class:leftside={$panelSide === 'left'}>
        <button class="tb-btn" onclick={() => showRight.update(v => !v)} title="Toggle panel">
          {#if $panelSide === 'left'}
            {#if $showRight}
              <IconLayoutSidebarFilled size={14} />
            {:else}
              <IconLayoutSidebar size={14} />
            {/if}
          {:else}
            {#if $showRight}
              <IconLayoutSidebarRightFilled size={14} />
            {:else}
              <IconLayoutSidebarRight size={14} />
            {/if}
          {/if}
        </button>
      </div>

    </div>
  </div>

  <!-- ─── workspace ─── -->
  <div class="workspace">

    {#if $showRight && $panelSide === 'left'}
    <div class="right-col" style="width:{$rightWidth}px">
      <RightSidebar />
    </div>
    <div class="hsplit-handle"
         onmousedown={startResize}
         role="separator" tabindex="-1"></div>
    {/if}

    <div class="center-col">
      {#if $galleryMode}
        <Gallery />
      {:else}
        <TabBar />
        <div class="tab-content">
          {#if $tabs.length === 0}
            <div class="welcome">
              <img class="welcome-mark"
                   src={$currentThemeName === 'light' ? lightModeIcon : darkModeIcon}
                   alt="bish" draggable="false" />
              <div class="welcome-panels">
                <div class="welcome-rows">
                  <button class="welcome-row" onclick={openProject}>
                    <span>Open Project</span>
                    <span class="keys"><kbd>⌘</kbd><kbd>O</kbd></span>
                  </button>
                  <button class="welcome-row" onclick={() => showPalette.set(true)}>
                    <span>Go to File</span>
                    <span class="keys"><kbd>⌘</kbd><kbd>P</kbd></span>
                  </button>
                  <button class="welcome-row" onclick={() => { searchScopeDir.set(null); showGlobalSearch.set(true) }}>
                    <span>Search in Files</span>
                    <span class="keys"><kbd>⇧</kbd><kbd>⌘</kbd><kbd>F</kbd></span>
                  </button>
                  <button class="welcome-row" onclick={reopenMainTab}>
                    <span>Open Terminal</span>
                    <span class="keys"><kbd>⏎</kbd></span>
                  </button>
                  <button class="welcome-row" onclick={() => showOpenRemote.set(true)}>
                    <span>Open Remote Folder…</span>
                  </button>
                </div>
                {#if recentProjects.length}
                  <div class="welcome-divider"></div>
                  <div class="welcome-recents">
                    <div class="welcome-recents-title">Recent</div>
                    {#each recentProjects.slice(0, 5) as entry (entry.path)}
                      <button class="welcome-row welcome-recent" onclick={() => openRecent(entry.path)} title={entry.path}>
                        <IconFolder size={13} />
                        <span class="recent-name">{entry.name}</span>
                      </button>
                    {/each}
                  </div>
                {/if}
              </div>
            </div>
          {/if}
          {#each $tabs as tab (tab.id)}
            {#if tab.type === 'terminal'}
              <div class="tab-pane" style="display:{$activeTabId === tab.id ? 'flex' : 'none'}">
                <Terminal terminalId={tab.id} />
              </div>
            {:else if $activeTabId === tab.id}
              <div class="tab-pane">
                {#if tab.type === 'file'}
                  <FileViewer path={tab.path ?? ''} tabId={tab.id} />
                {:else if tab.type === 'media'}
                  <MediaViewer path={tab.path ?? ''} />
                {:else if tab.type === 'logs'}
                  <ProcessLogs id={tab.processId ?? ''} tabId={tab.id} />
                {:else if tab.type === 'settings'}
                  <Settings />
                {:else if tab.type === 'diff'}
                  <DiffViewer path={tab.path ?? ''} />
                {:else if tab.type === 'conflict'}
                  <MergeConflict path={tab.path ?? ''} tabId={tab.id} />
                {/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    </div>

    {#if $showRight && $panelSide === 'right'}
    <div class="hsplit-handle"
         onmousedown={startResize}
         role="separator" tabindex="-1"></div>
    <div class="right-col" style="width:{$rightWidth}px">
      <RightSidebar />
    </div>
    {/if}

  </div>

  {#each $floatingPanels as fp (fp.panelId)}
    {@const p = panels.find(pp => pp.id === fp.panelId)}
    {#if p}
      <FloatingWindow panel={p} state={fp} />
    {/if}
  {/each}

  {#if $showPalette}
    <CommandPalette onClose={() => showPalette.set(false)} />
  {/if}

  {#if $showActionPalette}
    <ActionPalette onClose={() => showActionPalette.set(false)} />
  {/if}

  {#if $showGlobalSearch}
    <GlobalSearch />
  {/if}

  <OpenRemoteDialog />

  {#if $shareDialogTerminalId}
    <LiveShareDialog terminalId={$shareDialogTerminalId} onClose={() => shareDialogTerminalId.set(null)} />
  {/if}

  {#if $shareDialogFilePath}
    <EditShareDialog path={$shareDialogFilePath} onClose={() => shareDialogFilePath.set(null)} />
  {/if}

  <WelcomeTour />

  {#if $showShortcutsOverlay}
    <ShortcutsOverlay onClose={() => showShortcutsOverlay.set(false)} />
  {/if}

  <!-- ─── status bar ─── -->
  <div class="statusbar">
    {#if $gitBranch}
      <span class="branch"><IconGitBranch size={12} />{$gitBranch}</span>
    {/if}
    <span class="cwd-text">{$cwd || '~'}</span>
    <span class="fill"></span>
    {#if activeTab?.type === 'file' && $activeSelection && $activeSelection.path === activeTab.path}
      <span class="stat">Ln {$activeSelection.line}, Col {$activeSelection.col + 1}</span>
      <span class="stat">{indentLabel($activeSelection.indent)}</span>
      <span class="stat">UTF-8</span>
      <span class="stat">{langLabel(activeTab.path ?? '')}</span>
    {/if}
  </div>

</div>

<style>
  /* ─── design tokens derived from theme vars ─── */
  :global(:root) {
    /* base catppuccin mocha (fallback before applyTheme runs) */
    --background:    #1e1e2e;
    --foreground:    #cdd6f4;
    --border:        #313244;
    --border-focused:#cba6f7;
    --accent:        #cba6f7;
    --muted:         #585b70;
    --success:       #a6e3a1;
    --error:         #f38ba8;
    --warning:       #fab387;

    /* derived */
    --bg-raised:   color-mix(in srgb, var(--background) 60%, var(--border) 40%);
    --bg-hover:    color-mix(in srgb, var(--foreground) 5%,  transparent);
    --bg-selected: color-mix(in srgb, var(--accent) 14%,     transparent);
    --accent-dim:  color-mix(in srgb, var(--accent) 50%,     transparent);
    --shadow-color:color-mix(in srgb, #000 50%, transparent);
  }

  :global(*, *::before, *::after) { box-sizing: border-box; }
  :global(body) {
    background: var(--background);
    color: var(--foreground);
    font-family: -apple-system, "SF Pro Text", "Helvetica Neue", sans-serif;
    -webkit-font-smoothing: antialiased;
    overflow: hidden;
    height: 100vh;
    margin: 0;
  }

  .root {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }

  /* ─── titlebar ─── */
  .titlebar {
    display: flex;
    align-items: stretch;
    height: 38px;
    flex-shrink: 0;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
  }
  .traffic-spacer {
    width: 80px;
    flex-shrink: 0;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: 2px;
    flex: 1;
    padding-right: 10px;
  }
  .tb-fill { flex: 1; align-self: stretch; cursor: default; }
  .ml-auto { margin-left: auto; }

  .tb-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    border-radius: 5px;
    padding: 5px 7px;
    transition: color 0.12s, background 0.12s;
  }
  .tb-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .tb-btn.active { color: var(--foreground); background: var(--bg-hover); }

  .panel-toggles { display: flex; gap: 1px; }
  .panel-toggles.leftside { order: -1; }


  /* ─── workspace ─── */
  .workspace {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .hsplit-handle {
    width: 1px;
    cursor: ew-resize;
    flex-shrink: 0;
    background: var(--border);
    transition: background 0.1s;
    position: relative;
  }
  /* wider hit-target without affecting visual */
  .hsplit-handle::before {
    content: '';
    position: absolute;
    inset: 0 -3px;
  }
  .hsplit-handle:hover, .hsplit-handle:active { background: var(--border-focused); }

  .center-col {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    position: relative;
    display: flex;
    flex-direction: column;
  }
  .tab-content {
    flex: 1;
    min-height: 0;
    position: relative;
  }
  .tab-pane {
    width: 100%;
    height: 100%;
    flex-direction: column;
    overflow: hidden;
  }
  .welcome {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 28px;
    user-select: none;
  }
  .welcome-mark {
    width: 160px;
    height: 160px;
    opacity: 0.9;
  }
  .welcome-panels {
    display: flex;
    align-items: flex-start;
    gap: 24px;
  }
  .welcome-rows {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 260px;
  }
  .welcome-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 32px;
    background: none;
    border: none;
    color: var(--muted);
    font-size: 13px;
    padding: 4px 8px;
    border-radius: 4px;
    cursor: pointer;
    transition: color 0.1s, background 0.1s;
  }
  .welcome-row:hover { color: var(--foreground); background: var(--bg-hover); }
  .welcome-row .keys { display: flex; gap: 4px; }
  .welcome-row kbd {
    font-family: inherit;
    font-size: 11px;
    line-height: 1;
    padding: 4px 6px;
    border-radius: 4px;
    background: var(--bg-hover);
    border: 1px solid var(--border);
    color: var(--muted);
  }
  .welcome-divider {
    align-self: stretch;
    width: 1px;
    background: var(--border);
  }
  .welcome-recents {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 200px;
    max-width: 260px;
  }
  .welcome-recents-title {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
    padding: 4px 8px;
    opacity: 0.7;
  }
  .welcome-recent {
    justify-content: flex-start;
    gap: 8px;
  }
  .welcome-recent :global(svg) { flex-shrink: 0; opacity: 0.7; }
  .welcome-recent .recent-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .right-col {
    display: flex;
    flex-direction: column;
    min-width: 160px;
    max-width: 500px;
    overflow: hidden;
    flex-shrink: 0;
  }

  /* ─── status bar ─── */
  .statusbar {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 24px;
    padding: 0 12px;
    border-top: 1px solid var(--border);
    font-size: 11px;
    flex-shrink: 0;
    background: var(--bg-raised);
  }
  .cwd-text {
    color: var(--muted);
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 40%;
  }
  .fill { flex: 1; }
  .branch {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--muted);
    font-size: 11px;
    flex-shrink: 0;
  }
  .stat {
    color: var(--muted);
    font-size: 11px;
    white-space: nowrap;
  }
</style>
