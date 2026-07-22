<script lang="ts">
  import { GitDiffText } from '../lib/wails'

  let { path }: { path: string } = $props()

  let lines: { text: string; cls: string }[] = $state([])
  let loading = $state(true)

  function classify(l: string): string {
    if (l.startsWith('@@')) return 'hunk'
    if (l.startsWith('+++') || l.startsWith('---')) return 'meta'
    if (l.startsWith('diff ') || l.startsWith('index ') ||
        l.startsWith('new file') || l.startsWith('deleted file') ||
        l.startsWith('similarity ') || l.startsWith('rename ')) return 'meta'
    if (l.startsWith('+')) return 'add'
    if (l.startsWith('-')) return 'del'
    return 'ctx'
  }

  $effect(() => {
    const p = path
    loading = true
    GitDiffText(p).then(txt => {
      lines = (txt ?? '').split('\n').map(t => ({ text: t, cls: classify(t) }))
      // drop a single trailing blank line from the split
      if (lines.length && lines[lines.length - 1].text === '') lines.pop()
      loading = false
    }).catch(() => { lines = []; loading = false })
  })
</script>

<div class="diff">
  {#if loading}
    <div class="empty">Loading diff…</div>
  {:else if lines.length === 0}
    <div class="empty">No changes vs git HEAD</div>
  {:else}
    <pre class="body">{#each lines as l}<span class="ln {l.cls}">{l.text || ' '}</span>{/each}</pre>
  {/if}
</div>

<style>
  .diff {
    width: 100%;
    height: 100%;
    overflow: auto;
    background: var(--background);
  }
  .empty { color: var(--muted); font-size: 12px; padding: 12px 14px; }
  .body {
    margin: 0;
    padding: 8px 0;
    font-family: "SF Mono", Menlo, monospace;
    font-size: 12px;
    line-height: 1.5;
  }
  .ln {
    display: block;
    padding: 0 12px;
    white-space: pre;
  }
  .ln.add  { background: color-mix(in srgb, var(--success) 14%, transparent); color: var(--foreground); }
  .ln.del  { background: color-mix(in srgb, var(--error) 14%, transparent);   color: var(--foreground); }
  .ln.hunk { color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
  .ln.meta { color: var(--muted); }
  .ln.ctx  { color: var(--foreground); }
</style>
