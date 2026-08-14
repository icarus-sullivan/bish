<script lang="ts">
  import { ReadFile, WriteFile, GitStage } from '../lib/wails'
  import { closeTab } from '../lib/stores'
  import { IconGitMerge, IconCheck } from '@tabler/icons-svelte'

  let { path, tabId }: { path: string; tabId: string } = $props()

  type Segment =
    | { type: 'context'; lines: string[] }
    | { type: 'conflict'; ours: string[]; theirs: string[]; oursLabel: string; theirsLabel: string; resolved: 'ours' | 'theirs' | 'both' | null }

  let segments = $state<Segment[]>([])
  let loading = $state(true)
  let error = $state('')
  let saving = $state(false)

  // git's default conflict markers — <<<<<<< ours / ======= / >>>>>>> theirs
  function parse(text: string): Segment[] {
    const lines = text.split('\n')
    const out: Segment[] = []
    let ctx: string[] = []
    let i = 0
    while (i < lines.length) {
      if (lines[i].startsWith('<<<<<<<')) {
        if (ctx.length) { out.push({ type: 'context', lines: ctx }); ctx = [] }
        const oursLabel = lines[i].slice(7).trim()
        i++
        const ours: string[] = []
        while (i < lines.length && !lines[i].startsWith('=======')) { ours.push(lines[i]); i++ }
        i++ // skip =======
        const theirs: string[] = []
        while (i < lines.length && !lines[i].startsWith('>>>>>>>')) { theirs.push(lines[i]); i++ }
        const theirsLabel = (lines[i] ?? '').slice(7).trim()
        i++
        out.push({ type: 'conflict', ours, theirs, oursLabel, theirsLabel, resolved: null })
      } else {
        ctx.push(lines[i]); i++
      }
    }
    if (ctx.length) out.push({ type: 'context', lines: ctx })
    return out
  }

  function rebuild(segs: Segment[]): string {
    return segs.map(s => {
      if (s.type === 'context') return s.lines.join('\n')
      if (s.resolved === 'ours') return s.ours.join('\n')
      if (s.resolved === 'theirs') return s.theirs.join('\n')
      if (s.resolved === 'both') return [...s.ours, ...s.theirs].join('\n')
      return [`<<<<<<< ${s.oursLabel}`, ...s.ours, '=======', ...s.theirs, `>>>>>>> ${s.theirsLabel}`].join('\n')
    }).join('\n')
  }

  $effect(() => {
    const p = path
    loading = true; error = ''
    ReadFile(p).then(text => { segments = parse(text ?? ''); loading = false })
      .catch((e: any) => { error = String(e?.message ?? e); loading = false })
  })

  const conflictCount = $derived(segments.filter(s => s.type === 'conflict').length)
  const resolvedCount = $derived(segments.filter(s => s.type === 'conflict' && s.resolved).length)
  const allResolved = $derived(conflictCount > 0 && resolvedCount === conflictCount)

  function resolve(seg: Segment & { type: 'conflict' }, choice: 'ours' | 'theirs' | 'both') {
    seg.resolved = choice
    segments = [...segments]
  }

  async function saveAndStage() {
    if (!allResolved || saving) return
    saving = true; error = ''
    try {
      await WriteFile(path, rebuild(segments))
      await GitStage(path)
      closeTab(tabId)
    } catch (e: any) {
      error = String(e?.message ?? e)
    } finally {
      saving = false
    }
  }
</script>

<div class="conflict">
  <div class="header">
    <IconGitMerge size={13} />
    <span class="title">{path.split('/').pop()}</span>
    <span class="progress">{resolvedCount} / {conflictCount} resolved</span>
    <button class="save-btn" disabled={!allResolved || saving} onclick={saveAndStage}>
      <IconCheck size={13} /> {saving ? 'Saving…' : 'Save & Stage'}
    </button>
  </div>

  {#if error}<div class="err">{error}</div>{/if}

  {#if loading}
    <div class="empty">Loading…</div>
  {:else if conflictCount === 0}
    <div class="empty">No conflict markers found — already resolved?</div>
  {:else}
    <div class="body">
      {#each segments as seg}
        {#if seg.type === 'context'}
          <pre class="ctx">{seg.lines.join('\n')}</pre>
        {:else}
          <div class="hunk" class:resolved={!!seg.resolved}>
            <div class="hunk-actions">
              <button class:active={seg.resolved === 'ours'} onclick={() => resolve(seg, 'ours')}>Accept Ours</button>
              <button class:active={seg.resolved === 'theirs'} onclick={() => resolve(seg, 'theirs')}>Accept Theirs</button>
              <button class:active={seg.resolved === 'both'} onclick={() => resolve(seg, 'both')}>Accept Both</button>
            </div>
            <div class="sides">
              <div class="side ours" class:dimmed={seg.resolved === 'theirs'}>
                <div class="side-label">Ours{seg.oursLabel ? ` (${seg.oursLabel})` : ''}</div>
                <pre>{seg.ours.join('\n') || ' '}</pre>
              </div>
              <div class="side theirs" class:dimmed={seg.resolved === 'ours'}>
                <div class="side-label">Theirs{seg.theirsLabel ? ` (${seg.theirsLabel})` : ''}</div>
                <pre>{seg.theirs.join('\n') || ' '}</pre>
              </div>
            </div>
          </div>
        {/if}
      {/each}
    </div>
  {/if}
</div>

<style>
  .conflict { display: flex; flex-direction: column; height: 100%; overflow: hidden; background: var(--background); }
  .header {
    display: flex; align-items: center; gap: 8px; padding: 0 12px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border);
    color: var(--muted);
  }
  .title { font-size: 12px; font-weight: 600; color: var(--foreground); }
  .progress { font-size: 11px; color: var(--muted); margin-left: auto; }
  .save-btn {
    display: flex; align-items: center; gap: 5px;
    background: var(--accent); color: #000; border: none; border-radius: 5px;
    font-size: 11px; font-weight: 600; padding: 5px 10px; cursor: pointer;
  }
  .save-btn:disabled { opacity: 0.4; cursor: default; }
  .err { color: var(--error); font-size: 11px; padding: 6px 12px; white-space: pre-wrap; }
  .empty { color: var(--muted); font-size: 12px; padding: 12px 14px; }
  .body { overflow: auto; flex: 1; padding: 8px 0; font-family: "SF Mono", Menlo, monospace; font-size: 12px; }
  .ctx { margin: 0; padding: 2px 12px; color: var(--muted); white-space: pre-wrap; }
  .hunk { margin: 6px 0; border-top: 1px solid var(--border); border-bottom: 1px solid var(--border); }
  .hunk.resolved { background: color-mix(in srgb, var(--success) 6%, transparent); }
  .hunk-actions { display: flex; gap: 6px; padding: 6px 12px; }
  .hunk-actions button {
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 4px;
    color: var(--foreground); font-size: 11px; padding: 3px 8px; cursor: pointer;
  }
  .hunk-actions button:hover { background: var(--bg-hover); }
  .hunk-actions button.active { background: var(--accent); color: #000; border-color: var(--accent); }
  .sides { display: flex; }
  .side { flex: 1; min-width: 0; padding: 0 12px 8px; }
  .side.dimmed { opacity: 0.4; }
  .side-label { font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin-bottom: 2px; }
  .side.ours .side-label { color: var(--success); }
  .side.theirs .side-label { color: var(--accent); }
  .side pre { margin: 0; white-space: pre-wrap; word-break: break-word; }
</style>
