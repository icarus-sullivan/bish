<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'
  import { FileOutline, on } from '../lib/wails'
  import type { OutlineSym } from '../lib/wails'
  import { tabs, activeTabId, activeRightPanel, pendingGoto } from '../lib/stores'
  import { IconRefresh } from '@tabler/icons-svelte'

  let syms = $state<OutlineSym[]>([])
  let query = $state('')

  function activePath(): string {
    const t = get(tabs).find(t => t.id === get(activeTabId))
    return t && t.type === 'file' && t.path && t.path !== '__new__' ? t.path : ''
  }

  async function refresh() {
    const p = activePath()
    syms = p ? (await FileOutline(p).catch(() => [])) ?? [] : []
  }

  // panel stays mounted (display:none) when hidden — only parse when visible,
  // else every save/tab-switch would parse the file for a panel nobody sees.
  onMount(() => on('tree:update', () => { if (get(activeRightPanel) === 'outline') refresh() }))

  // re-fetch when the active tab changes (only while this panel is visible)
  $effect(() => {
    const active = $activeRightPanel === 'outline'
    void $activeTabId
    if (active) refresh()
  })

  const filtered = $derived(
    query.trim()
      ? syms.filter(s => s.name.toLowerCase().includes(query.toLowerCase()))
      : syms
  )

  function jump(s: OutlineSym) {
    const p = activePath()
    if (p) pendingGoto.set({ path: p, line: s.line, col: 0 })
  }

  // one-letter kind badge + color
  const badge: Record<string, string> = {
    func: 'ƒ', method: 'm', class: 'C', type: 'T', interface: 'I',
    const: 'k', var: 'v', enum: 'E',
  }
  function kindColor(k: string): string {
    if (k === 'func' || k === 'method') return 'var(--accent)'
    if (k === 'class' || k === 'type' || k === 'interface' || k === 'enum') return 'var(--warning)'
    return 'var(--muted)'
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Outline</span>
    <div class="header-actions">
      <button class="hdr-btn" onclick={refresh} title="Refresh"><IconRefresh size={13} /></button>
    </div>
  </div>

  {#if syms.length > 0}
    <input class="filter" placeholder="Filter symbols…" bind:value={query}
           autocapitalize="none" autocorrect="off" spellcheck="false" />
  {/if}

  <div class="list">
    {#if syms.length === 0}
      <div class="empty">No symbols</div>
    {:else if filtered.length === 0}
      <div class="empty">No match</div>
    {:else}
      {#each filtered as s (s.line + ':' + s.name)}
        <button class="row" style="padding-left:{10 + s.depth * 12}px" onclick={() => jump(s)}>
          <span class="badge" style="color:{kindColor(s.kind)}">{badge[s.kind] ?? '•'}</span>
          <span class="name">{s.name}</span>
          <span class="ln">{s.line}</span>
        </button>
      {/each}
    {/if}
  </div>
</div>

<style>
  .panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .header {
    display: flex; align-items: center; padding: 0 12px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; color: var(--muted);
  }
  .header-actions { display: flex; align-items: center; gap: 1px; margin-left: auto; flex-shrink: 0; }
  .hdr-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .filter {
    margin: 6px 8px; background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 12px; padding: 4px 8px; outline: none;
  }
  .filter:focus { border-color: var(--accent); }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; }
  .empty { color: var(--muted); font-size: 12px; padding: 8px 12px; }
  .row {
    display: flex; align-items: baseline; gap: 8px; width: 100%;
    padding: 3px 10px; background: none; border: none; cursor: pointer;
    text-align: left; color: var(--foreground); font-size: 12px;
    white-space: nowrap; border-radius: 4px;
  }
  .row:hover { background: var(--bg-hover); }
  .badge {
    font-family: "SF Mono", Menlo, monospace; font-size: 11px; font-weight: 700;
    width: 12px; flex-shrink: 0; text-align: center;
  }
  .name { overflow: hidden; text-overflow: ellipsis; flex: 1; }
  .ln { color: var(--muted); font-size: 10px; flex-shrink: 0; }
</style>
