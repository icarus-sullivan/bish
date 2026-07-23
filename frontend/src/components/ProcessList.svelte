<script lang="ts">
  import { processes, focusedPane, selectedProcess, openLogsTab } from '../lib/stores'
  import ContextMenu from './ContextMenu.svelte'
  import { KillProcess, RestartProcess, StopProcess } from '../lib/wails'
  import { IconPlayerPlayFilled, IconPlayerStopFilled, IconTrash } from '@tabler/icons-svelte'

  let menu: { x: number; y: number; id: string } | null = $state(null)

  function select(id: string) {
    selectedProcess.set(id)
    focusedPane.set('processes')
  }

  function showMenu(e: MouseEvent, id: string) {
    e.preventDefault()
    select(id)
    menu = { x: e.clientX, y: e.clientY, id }
  }

  function viewLogs(id: string) {
    const p = $processes.find(pr => pr.id === id)
    openLogsTab(id, p?.name || p?.cmd || id)
  }

  function menuItems(id: string) {
    return [
      { label: 'View Logs', action: () => viewLogs(id) },
      { label: 'Restart', action: () => RestartProcess(id) },
      { label: 'Stop', action: () => StopProcess(id) },
      { label: 'Kill Process', action: () => KillProcess(id), danger: true },
    ]
  }

  function play(e: MouseEvent, id: string) {
    e.stopPropagation()
    RestartProcess(id)
  }
  function stop(e: MouseEvent, id: string) {
    e.stopPropagation()
    StopProcess(id)
  }
  function trash(e: MouseEvent, id: string) {
    e.stopPropagation()
    KillProcess(id)
  }

  function cpuColor(pct: number) {
    if (pct >= 50) return 'var(--error)'
    if (pct >= 15) return 'var(--warning)'
    return 'var(--success)'
  }
</script>

<div class="panel" class:focused={$focusedPane === 'processes'}>
  <div class="header">
    <span class="header-label">Processes</span>
    <span class="header-count">{$processes.length || ''}</span>
  </div>
  <div class="list">
    {#if $processes.length === 0}
      <div class="empty">no processes</div>
    {:else}
      {#each $processes as p (p.id)}
        <div
          class="row"
          class:selected={$selectedProcess === p.id}
          class:crashed={p.status === 'crashed'}
          onclick={() => select(p.id)}
          ondblclick={() => viewLogs(p.id)}
          oncontextmenu={(e) => showMenu(e, p.id)}
          role="row"
          tabindex="0"
          onkeydown={(e) => e.key === 'Enter' && select(p.id)}
        >
          <button class="row-btn" onclick={(e) => play(e, p.id)} title="Start / restart"><IconPlayerPlayFilled size={12} /></button>
          <button class="row-btn" disabled={p.status !== 'running'} onclick={(e) => stop(e, p.id)} title="Stop"><IconPlayerStopFilled size={12} /></button>
          <!-- status ring — pulsing when running -->
          <span class="status-dot" class:running={p.status === 'running'}
                                   class:crashed={p.status === 'crashed'}
                                   class:stopped={p.status === 'stopped'}></span>
          <span class="proc-name" title={p.name || p.cmd}>{p.name}</span>
          <span class="meta">
            {#if p.ports?.length}
              <span class="badge port" title={p.name || p.cmd}>:{p.ports[0]}</span>
            {/if}
            {#if p.status === 'running' && p.cpu_pct > 0}
              <span class="badge cpu" style="color:{cpuColor(p.cpu_pct)}">{p.cpu_pct.toFixed(1)}%</span>
            {/if}
          </span>
          <button class="row-btn" onclick={(e) => trash(e, p.id)} title="Kill and remove"><IconTrash size={12} /></button>
        </div>
      {/each}
    {/if}
  </div>

</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menuItems(menu.id)} onClose={() => menu = null} />
{/if}

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    position: relative;
    /* right + bottom edges drawn by the split handles beside this pane */
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 12px;
    height: 32px;
    flex-shrink: 0;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--muted);
    transition: color 0.15s;
  }
  .panel.focused .header-label { color: var(--accent); }
  .header-count {
    font-size: 10px;
    color: var(--muted);
    background: var(--bg-hover);
    padding: 1px 6px;
    border-radius: 8px;
  }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; }
  .empty { padding: 10px 12px; color: var(--muted); font-size: 11px; font-style: italic; }

  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 12px;
    cursor: pointer;
    font-size: 12px;
    user-select: none;
    border-radius: 4px;
    margin: 0 4px;
    transition: background 0.1s;
  }
  .row:hover { background: var(--bg-hover); }
  .row.selected { background: var(--bg-selected); }
  .row.crashed { opacity: 0.5; }

  /* ─── signature: pulsing status ring ─── */
  .status-dot {
    width: 7px;
    height: 7px;
    margin-left: 4px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--muted);
    position: relative;
  }
  .row-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; flex-shrink: 0;
    transition: color 0.1s, background 0.1s;
  }
  .row-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .row-btn:disabled { opacity: 0.3; cursor: default; }
  .row-btn:disabled:hover { color: var(--muted); background: none; }

  .status-dot.running  { background: var(--success); }
  .status-dot.crashed  { background: var(--error); }
  .status-dot.stopped  { background: var(--muted); }

  .status-dot.running::after {
    content: '';
    position: absolute;
    inset: -4px;
    border-radius: 50%;
    background: var(--success);
    opacity: 0;
    animation: ring-pulse 2.4s ease-out infinite;
  }
  @keyframes ring-pulse {
    0%   { transform: scale(0.6); opacity: 0.5; }
    80%  { transform: scale(2.0); opacity: 0; }
    100% { transform: scale(2.0); opacity: 0; }
  }

  .proc-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
  }
  .meta { display: flex; align-items: center; gap: 4px; flex-shrink: 0; }

  .badge {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px;
    padding: 1px 5px;
    border-radius: 3px;
    background: var(--bg-hover);
    color: var(--muted);
  }
  .badge.port { color: color-mix(in srgb, var(--accent) 80%, var(--foreground)); }
  .badge.cpu  { background: transparent; padding-right: 0; }

</style>
