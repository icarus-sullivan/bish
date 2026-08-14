<script lang="ts">
  import { diagnostics, type DiagnosticEntry } from '../lib/diagnostics'
  import { openFileTab, pendingGoto } from '../lib/stores'
  import { IconAlertTriangle, IconAlertCircle, IconInfoCircle } from '@tabler/icons-svelte'

  const files = $derived([...$diagnostics.entries()].sort((a, b) => a[0].localeCompare(b[0])))
  const total = $derived(files.reduce((n, [, items]) => n + items.length, 0))
  const errorCount = $derived(files.reduce((n, [, items]) => n + items.filter(i => i.severity === 'error').length, 0))

  function basename(path: string): string {
    return path.split('/').pop() || path
  }
  function dirname(path: string): string {
    const parts = path.split('/')
    parts.pop()
    return parts.join('/')
  }

  function jump(path: string, d: DiagnosticEntry) {
    pendingGoto.set({ path, line: d.line, col: d.col })
    openFileTab(path, true)
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Problems</span>
    {#if total > 0}<span class="count">{total}</span>{/if}
  </div>

  <div class="list">
    {#if files.length === 0}
      <div class="empty">No problems</div>
    {:else}
      {#each files as [path, items] (path)}
        <div class="file-group">
          <div class="file-row" title={path}>
            <span class="file-name">{basename(path)}</span>
            <span class="file-dir">{dirname(path)}</span>
          </div>
          {#each items as d, i (i)}
            <button class="row" onclick={() => jump(path, d)}>
              <span class="icon" class:error={d.severity === 'error'} class:warning={d.severity === 'warning'}>
                {#if d.severity === 'error'}
                  <IconAlertCircle size={13} />
                {:else if d.severity === 'warning'}
                  <IconAlertTriangle size={13} />
                {:else}
                  <IconInfoCircle size={13} />
                {/if}
              </span>
              <span class="msg">{d.message}</span>
              <span class="ln">{d.line}:{d.col + 1}</span>
            </button>
          {/each}
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .header {
    display: flex; align-items: center; gap: 8px; padding: 0 12px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; color: var(--muted);
  }
  .count {
    font-size: 10px; color: var(--muted); background: var(--bg-hover);
    border-radius: 8px; padding: 1px 6px;
  }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; }
  .empty { color: var(--muted); font-size: 12px; padding: 8px 12px; }

  .file-group { margin-bottom: 2px; }
  .file-row {
    display: flex; align-items: baseline; gap: 6px; padding: 4px 10px;
    font-size: 11px; font-weight: 600;
  }
  .file-name { color: var(--foreground); }
  .file-dir { color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .row {
    display: flex; align-items: center; gap: 6px; width: 100%;
    padding: 3px 10px 3px 18px; background: none; border: none; cursor: pointer;
    text-align: left; color: var(--foreground); font-size: 12px; border-radius: 4px;
  }
  .row:hover { background: var(--bg-hover); }
  .icon { display: flex; flex-shrink: 0; color: var(--muted); }
  .icon.error { color: var(--error); }
  .icon.warning { color: var(--warning); }
  .msg { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ln { color: var(--muted); font-size: 10px; flex-shrink: 0; }
</style>
