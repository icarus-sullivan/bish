<script lang="ts">
  import { lineDiff, unifiedRows, type DiffRow } from '../../lib/claude/diff'

  let { oldText = '', newText = '', unified = null, unifiedKind = 'update', maxRows = 400 }: {
    oldText?: string; newText?: string; maxRows?: number
    // a ready-made unified diff (Codex file changes) instead of old/new text
    unified?: string | null; unifiedKind?: 'add' | 'delete' | 'update'
  } = $props()

  const rows = $derived<DiffRow[]>(unified !== null ? unifiedRows(unified, unifiedKind) : lineDiff(oldText, newText))
  let expanded = $state(false)
  const shown = $derived(expanded ? rows : rows.slice(0, maxRows))
</script>

<div class="diff">
  {#each shown as r, i (i)}
    {#if r.kind === 'gap'}
      <div class="row gap">⋯</div>
    {:else}
      <div class="row {r.kind}">
        <span class="no">{r.kind === 'add' ? '' : r.oldNo ?? ''}</span>
        <span class="no">{r.kind === 'del' ? '' : r.newNo ?? ''}</span>
        <span class="sign">{r.kind === 'add' ? '+' : r.kind === 'del' ? '−' : ' '}</span>
        <span class="text">{r.text}</span>
      </div>
    {/if}
  {/each}
  {#if rows.length > shown.length}
    <button class="more" onclick={() => expanded = true}>Show {rows.length - shown.length} more lines</button>
  {/if}
</div>

<style>
  .diff {
    font-family: "SF Mono", Menlo, monospace; font-size: 11px; line-height: 1.45;
    background: var(--background); border-top: 1px solid var(--border);
    max-height: 360px; overflow: auto;
  }
  .row { display: flex; white-space: pre; min-width: max-content; }
  .no { width: 30px; flex-shrink: 0; text-align: right; padding-right: 6px; color: var(--muted); opacity: 0.6; user-select: none; }
  .sign { width: 14px; flex-shrink: 0; text-align: center; user-select: none; color: var(--muted); }
  .text { padding-right: 8px; }
  .row.add { background: color-mix(in srgb, var(--success) 14%, transparent); }
  .row.add .sign { color: var(--success); }
  .row.del { background: color-mix(in srgb, var(--error) 14%, transparent); }
  .row.del .sign { color: var(--error); }
  .row.gap { color: var(--muted); padding-left: 52px; background: var(--bg-raised); }
  .more {
    display: block; width: 100%; background: var(--bg-raised); border: none; border-top: 1px solid var(--border);
    color: var(--muted); font-size: 11px; padding: 4px; cursor: pointer;
  }
  .more:hover { color: var(--foreground); }
</style>
