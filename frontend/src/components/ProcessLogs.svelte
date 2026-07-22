<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { processes, closeTab } from '../lib/stores'
  import { GetProcessLogs } from '../lib/wails'

  let { id, tabId }: { id: string; tabId: string } = $props()

  let lines: string[] = $state([])
  let logEl: HTMLDivElement
  let autoScroll = true

  const proc = $derived($processes.find(p => p.id === id))

  async function refresh() {
    const fetched = await GetProcessLogs(id).catch(() => null)
    if (fetched == null) return
    lines = fetched
    if (autoScroll) {
      await tick()
      logEl?.scrollTo({ top: logEl.scrollHeight })
    }
  }

  function onScroll() {
    if (!logEl) return
    autoScroll = logEl.scrollHeight - logEl.scrollTop - logEl.clientHeight < 40
  }

  // refresh whenever process list updates (catches status changes fast)
  $effect(() => {
    void $processes
    refresh()
  })

  // ALSO poll independently — refreshLoop's processes:update only fires when
  // CPU%/mem/status actually changed (app.go), which log content isn't part
  // of, so a quiet-CPU process (e.g. rsync between chunks) could sit on
  // stale output otherwise. This is what makes the view genuinely live.
  onMount(() => {
    refresh()
    const t = setInterval(refresh, 1000)
    return () => clearInterval(t)
  })

  function statusColor(status: string) {
    if (status === 'running') return 'var(--success)'
    if (status === 'crashed') return 'var(--error)'
    return 'var(--muted)'
  }
</script>

<div class="logs-view">
  <div class="header">
    {#if proc}
      <span class="status-dot" style="background:{statusColor(proc.status)}"></span>
      <span class="proc-name">{proc.name}</span>
      {#if proc.ports?.length}
        <span class="badge">:{proc.ports[0]}</span>
      {/if}
      <span class="status-text" style="color:{statusColor(proc.status)}">{proc.status}</span>
    {:else}
      <span class="proc-name">Process logs</span>
    {/if}
    <span class="fill"></span>
    <button class="close-btn" onclick={() => closeTab(tabId)} title="Close logs">✕</button>
  </div>

  <div class="log-body" bind:this={logEl} onscroll={onScroll}>
    {#if lines.length === 0}
      <div class="empty">no output captured yet</div>
    {:else}
      {#each lines as line, i (i)}
        <div class="line">{line}</div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .logs-view {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    background: var(--background);
  }

  .header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 14px;
    height: 36px;
    flex-shrink: 0;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
    font-size: 12px;
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .proc-name {
    font-weight: 600;
    color: var(--foreground);
    font-size: 13px;
  }

  .badge {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px;
    padding: 1px 5px;
    border-radius: 3px;
    background: var(--bg-hover);
    color: color-mix(in srgb, var(--accent) 80%, var(--foreground));
  }

  .status-text {
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .fill { flex: 1; }

  .close-btn {
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 3px 6px;
    border-radius: 4px;
  }
  .close-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .log-body {
    flex: 1;
    overflow-y: auto;
    padding: 8px 0;
    font-family: "SF Mono", Menlo, "Courier New", monospace;
    font-size: 12px;
    line-height: 1.6;
  }

  .line {
    padding: 0 16px;
    white-space: pre-wrap;
    word-break: break-all;
    color: var(--foreground);
  }

  /* alternating row tint for readability */
  .line:nth-child(odd) {
    background: color-mix(in srgb, var(--foreground) 2%, transparent);
  }

  .empty {
    padding: 24px 16px;
    color: var(--muted);
    font-size: 12px;
    font-style: italic;
  }
</style>
