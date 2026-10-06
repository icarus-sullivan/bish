<script lang="ts">
  import { IconSearch, IconArrowLeft, IconGitBranch, IconLoader2 } from '@tabler/icons-svelte'
  import type { SessionSummary } from './types'
  import { timeAgo } from '../../lib/claude/format'

  let { sessions, loading, error, onOpen, onBack }: {
    sessions: SessionSummary[]; loading: boolean; error: string
    onOpen: (s: SessionSummary) => void; onBack: () => void
  } = $props()

  let q = $state('')
  let inputEl: HTMLInputElement
  $effect(() => { inputEl?.focus() })

  const filtered = $derived.by(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return sessions
    return sessions.filter(s => s.title.toLowerCase().includes(needle) || s.firstPrompt.toLowerCase().includes(needle) || s.gitBranch.toLowerCase().includes(needle))
  })

  // bucket by day for scanability
  const buckets = $derived.by(() => {
    const out: { label: string; items: SessionSummary[] }[] = []
    const startOfDay = new Date(); startOfDay.setHours(0, 0, 0, 0)
    const today = startOfDay.getTime(), yesterday = today - 86400_000, week = today - 6 * 86400_000
    for (const s of filtered) {
      const label = s.modified >= today ? 'Today' : s.modified >= yesterday ? 'Yesterday' : s.modified >= week ? 'This week' : 'Older'
      const last = out[out.length - 1]
      if (last?.label === label) last.items.push(s); else out.push({ label, items: [s] })
    }
    return out
  })
</script>

<div class="history">
  <div class="top">
    <button class="hdr-btn" onclick={onBack} title="Back"><IconArrowLeft size={13} /></button>
    <div class="search">
      <IconSearch size={12} />
      <input bind:this={inputEl} bind:value={q} placeholder="Search conversations…"
             onkeydown={(e) => { if (e.key === 'Escape') onBack(); if (e.key === 'Enter' && filtered[0]) onOpen(filtered[0]) }} />
    </div>
  </div>
  <div class="list">
    {#if loading}
      <div class="empty"><IconLoader2 size={13} class="spin" /> Loading…</div>
    {:else if error}
      <div class="empty err">{error}</div>
    {:else if !filtered.length}
      <div class="empty">{q ? 'No matches' : 'No past conversations in this project yet.'}</div>
    {/if}
    {#each buckets as b (b.label)}
      <div class="bucket">{b.label}</div>
      {#each b.items as s (s.id)}
        <button class="row" onclick={() => onOpen(s)} title={s.firstPrompt}>
          <span class="title">{s.title}</span>
          <span class="meta">
            {timeAgo(s.modified)}
            {#if s.gitBranch}<span class="branch"><IconGitBranch size={10} /> {s.gitBranch}</span>{/if}
          </span>
        </button>
      {/each}
    {/each}
  </div>
</div>

<style>
  .history { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .top { display: flex; align-items: center; gap: 6px; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .hdr-btn {
    display: flex; align-items: center; justify-content: center; background: none; border: none;
    color: var(--muted); cursor: pointer; padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .search {
    flex: 1; display: flex; align-items: center; gap: 6px; color: var(--muted);
    background: var(--background); border: 1px solid var(--border); border-radius: 5px; padding: 4px 8px;
  }
  .search:focus-within { border-color: var(--accent); }
  .search input { flex: 1; background: none; border: none; outline: none; color: var(--foreground); font-size: 12px; font-family: inherit; }
  .list { flex: 1; overflow-y: auto; padding: 4px 6px 10px; }
  .bucket { font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); padding: 10px 6px 4px; }
  .row {
    display: flex; flex-direction: column; gap: 2px; width: 100%; text-align: left; background: none; border: none;
    border-radius: 5px; padding: 6px; cursor: pointer; font-family: inherit;
  }
  .row:hover { background: var(--bg-hover); }
  .title { font-size: 12px; color: var(--foreground); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { display: flex; gap: 8px; font-size: 10.5px; color: var(--muted); }
  .branch { display: inline-flex; align-items: center; gap: 2px; }
  .empty { display: flex; align-items: center; gap: 6px; justify-content: center; padding: 20px; font-size: 12px; color: var(--muted); }
  .empty.err { color: var(--error); }
</style>
