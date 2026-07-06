<script lang="ts">
  import { onMount } from 'svelte'
  import { focusedPane, galleryMode, cwd, showLeft, showRight,
           leftWidth, rightWidth, processHeight, currentThemeName,
           showPalette, showGlobalSearch, tabs, activeTabId, openFileTab, closeTab } from './lib/stores'
  import { get } from 'svelte/store'
  import { initEvents } from './lib/events'
  import ProcessList from './components/ProcessList.svelte'
  import CommandList from './components/CommandList.svelte'
  import Terminal from './components/Terminal.svelte'
  import FileTree from './components/FileTree.svelte'
  import FileViewer from './components/FileViewer.svelte'
  import MediaViewer from './components/MediaViewer.svelte'
  import Gallery from './components/Gallery.svelte'
  import TabBar from './components/TabBar.svelte'
  import { GetConfig, SaveConfig } from './lib/wails'
  import {
    IconLayoutSidebarLeftCollapse, IconLayoutSidebarLeftExpand,
    IconLayoutSidebarRightCollapse, IconLayoutSidebarRightExpand,
    IconPalette,
  } from '@tabler/icons-svelte'
  import CommandPalette from './components/CommandPalette.svelte'
  import GlobalSearch from './components/GlobalSearch.svelte'
  import ProcessLogs from './components/ProcessLogs.svelte'
  import { OpenProject } from './lib/wails'
  import { WindowGetPosition, WindowSetPosition } from '../wailsjs/runtime/runtime'
  import { projectRoot } from './lib/stores'

  type Pane = 'processes' | 'commands' | 'terminal' | 'tree'
  const paneOrder: Pane[] = ['processes', 'commands', 'terminal', 'tree']

  let currentTheme = $state('obsidian')
  let appConfig: any = $state(null)

  const themes = [
    { value: 'catppuccin',  label: 'Catppuccin' },
    { value: 'tokyo-night', label: 'Tokyo Night' },
    { value: 'obsidian',    label: 'Obsidian' },
    { value: 'vos',         label: 'Vos' },
    { value: 'gruvbox',     label: 'Gruvbox' },
    { value: 'nord',        label: 'Nord' },
    { value: 'monokai',     label: 'Monokai' },
    { value: 'light',       label: 'Light' },
    { value: 'default',     label: 'Void' },
  ]

  onMount(async () => {
    await initEvents()
    try {
      appConfig = await GetConfig()
      const t = appConfig?.theme || 'obsidian'
      currentTheme = t
      currentThemeName.set(t)
    } catch {}
  })

  async function onThemeChange(e: Event) {
    const name = (e.target as HTMLSelectElement).value
    currentTheme = name
    currentThemeName.set(name)
    if (appConfig) {
      await SaveConfig({ ...appConfig, theme: name }).catch(() => {})
    }
  }

  function handleKey(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'p') {
      e.preventDefault()
      showPalette.set(true)
      return
    }
    if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'f') {
      e.preventDefault()
      showGlobalSearch.update(v => !v)
      return
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'o') {
      e.preventDefault()
      openProject()
      return
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'n') {
      e.preventDefault()
      openFileTab('__new__')
      return
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 't') {
      e.preventDefault()
      focusedPane.update(p => paneOrder[(paneOrder.indexOf(p) + 1) % paneOrder.length])
      return
    }
    if (e.key === 'Enter' && $focusedPane !== 'terminal') {
      e.preventDefault()
      const termTab = $tabs.find(t => t.type === 'terminal')
      if (termTab) activeTabId.set(termTab.id)
      focusedPane.set('terminal')
    }
    if (e.key === 'Escape') {
      const active = get(activeTabId)
      const t = $tabs.find(tt => tt.id === active)
      if (t && t.type !== 'terminal') closeTab(active)
    }
  }

  async function openProject() {
    await OpenProject().catch(() => {})
  }

  async function startWindowDrag(e: MouseEvent) {
    if ((e.target as HTMLElement).closest('button, select, input, a')) return
    e.preventDefault()
    const pos = await WindowGetPosition()
    const startSX = e.screenX
    const startSY = e.screenY
    const initX = pos.x
    const initY = pos.y
    function onMove(mv: MouseEvent) {
      WindowSetPosition(initX + mv.screenX - startSX, initY + mv.screenY - startSY)
    }
    function onUp() {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  type ResizeTarget = 'left' | 'right' | 'vsplit'

  function startResize(e: MouseEvent, target: ResizeTarget) {
    e.preventDefault()
    const startX = e.clientX
    const startY = e.clientY
    const startLeft = $leftWidth
    const startRight = $rightWidth
    const startPH = $processHeight

    function onMove(ev: MouseEvent) {
      if (target === 'left')   leftWidth.set(Math.max(160, Math.min(500, startLeft + ev.clientX - startX)))
      else if (target === 'right')  rightWidth.set(Math.max(160, Math.min(500, startRight - (ev.clientX - startX))))
      else if (target === 'vsplit') processHeight.set(Math.max(80, startPH + ev.clientY - startY))
    }
    function onUp() {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
</script>

<svelte:document onkeydown={handleKey} />

<div class="root">

  <!-- ─── titlebar ─── -->
  <div class="titlebar" onmousedown={startWindowDrag}>
    <div class="traffic-spacer"></div>
    <div class="toolbar">

      <div class="theme-picker" title="Switch theme">
        <IconPalette size={12} />
        <select value={currentTheme} onchange={onThemeChange} class="theme-select">
          {#each themes as t}
            <option value={t.value}>{t.label}</option>
          {/each}
        </select>
      </div>

      <div class="tb-fill"></div>

      <div class="panel-toggles">
        <button class="tb-btn" onclick={() => showLeft.update(v => !v)} title="Toggle sidebar">
          {#if $showLeft}
            <IconLayoutSidebarLeftCollapse size={14} />
          {:else}
            <IconLayoutSidebarLeftExpand size={14} />
          {/if}
        </button>
        <button class="tb-btn" onclick={() => showRight.update(v => !v)} title="Toggle panel">
          {#if $showRight}
            <IconLayoutSidebarRightCollapse size={14} />
          {:else}
            <IconLayoutSidebarRightExpand size={14} />
          {/if}
        </button>
      </div>

    </div>
  </div>

  <!-- ─── workspace ─── -->
  <div class="workspace">

    {#if $showLeft}
    <div class="left-col" style="width:{$leftWidth}px">
      <div class="pane" style="height:{$processHeight}px">
        <ProcessList />
      </div>
      <div class="vsplit-handle"
           onmousedown={(e) => startResize(e, 'vsplit')}
           role="separator" tabindex="-1"></div>
      <div class="pane pane-flex">
        <CommandList />
      </div>
    </div>
    <div class="hsplit-handle"
         onmousedown={(e) => startResize(e, 'left')}
         role="separator" tabindex="-1"></div>
    {/if}

    <div class="center-col">
      {#if $galleryMode}
        <Gallery />
      {:else}
        <TabBar />
        <div class="tab-content">
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
                {/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    </div>

    {#if $showRight}
    <div class="hsplit-handle"
         onmousedown={(e) => startResize(e, 'right')}
         role="separator" tabindex="-1"></div>
    <div class="right-col" style="width:{$rightWidth}px">
      <FileTree />
    </div>
    {/if}

  </div>

  {#if $showPalette}
    <CommandPalette onClose={() => showPalette.set(false)} />
  {/if}

  {#if $showGlobalSearch}
    <GlobalSearch />
  {/if}

  <!-- ─── status bar ─── -->
  <div class="statusbar">
    <span class="cwd-text">{$cwd || '~'}</span>
    <span class="fill"></span>
    <span class="pane-chip" class:terminal={$focusedPane === 'terminal'}
                            class:tree={$focusedPane === 'tree'}>{$focusedPane}</span>
    <span class="hints">⌘T · Ctrl+B · Esc</span>
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
    align-items: center;
    height: 38px;
    flex-shrink: 0;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
    cursor: default;
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

  .tb-fill { flex: 1; }
  .panel-toggles { display: flex; gap: 1px; }


  .theme-picker {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 4px 8px;
    border-radius: 5px;
    cursor: pointer;
    color: var(--muted);
    transition: color 0.12s, background 0.12s;
  }
  .theme-picker:hover { color: var(--foreground); background: var(--bg-hover); }
  .theme-select {
    background: transparent;
    border: none;
    color: inherit;
    font: 11px/1 -apple-system, sans-serif;
    cursor: pointer;
    appearance: none;
    -webkit-appearance: none;
    padding: 0;
  }
  .theme-select:focus { outline: none; }
  .theme-select option { background: var(--background); color: var(--foreground); }

  /* ─── workspace ─── */
  .workspace {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .left-col {
    display: flex;
    flex-direction: column;
    min-width: 160px;
    max-width: 500px;
    overflow: hidden;
    flex-shrink: 0;
  }
  .pane { overflow: hidden; flex-shrink: 0; }
  .pane-flex { flex: 1; min-height: 80px; flex-shrink: 1; overflow: hidden; }

  .vsplit-handle {
    height: 4px;
    cursor: ns-resize;
    flex-shrink: 0;
    background: var(--border);
    transition: background 0.1s;
  }
  .vsplit-handle:hover, .vsplit-handle:active { background: var(--border-focused); }

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
  .pane-chip {
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--muted);
    padding: 1px 7px;
    border-radius: 3px;
    background: var(--bg-hover);
  }
  .pane-chip.terminal { color: var(--accent); background: var(--bg-selected); }
  .hints {
    color: color-mix(in srgb, var(--muted) 60%, transparent);
    font-size: 10px;
  }
</style>
