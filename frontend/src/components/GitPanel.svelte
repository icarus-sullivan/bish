<script lang="ts">
  import { onMount } from 'svelte'
  import { GitStatus, on } from '../lib/wails'
  import type { GitStatusDTO } from '../lib/wails'
  import { activeRightPanel, projectRoot, openFileTab } from '../lib/stores'
  import { IconRefresh, IconGitBranch } from '@tabler/icons-svelte'

  let status: GitStatusDTO | null = $state(null)

  async function refresh() {
    status = await GitStatus().catch(() => null)
  }

  onMount(() => {
    refresh()
    // tree:update fires on FS changes — free change signal.
    // EventsOn returns a canceller for just this listener (EventsOff would
    // also kill the store listener in events.ts).
    return on('tree:update', refresh)
  })

  // refresh when this panel becomes active or the project changes
  $effect(() => {
    if ($activeRightPanel === 'git' || $projectRoot) refresh()
  })

  function statusColor(s: string): string {
    if (s.includes('?')) return 'var(--muted)'
    if (s.includes('D')) return 'var(--error)'
    if (s.includes('A')) return 'var(--success)'
    return 'var(--warning)' // M, R, etc.
  }

  function fileName(p: string) { return p.split('/').pop() || p }
  function dirName(p: string) {
    const root = $projectRoot
    let rel = root && p.startsWith(root + '/') ? p.slice(root.length + 1) : p
    const i = rel.lastIndexOf('/')
    return i === -1 ? '' : rel.slice(0, i)
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Git</span>
    {#if status?.branch}
      <span class="branch" title={status.branch}>
        <IconGitBranch size={11} />{status.branch}
      </span>
    {/if}
    <div class="header-actions">
      <button class="hdr-btn" onclick={refresh} title="Refresh"><IconRefresh size={13} /></button>
    </div>
  </div>
  <div class="list">
    {#if !status}
      <div class="empty">Not a git repository</div>
    {:else if status.files.length === 0}
      <div class="empty">No changes</div>
    {:else}
      {#each status.files as f (f.path)}
        <div class="row" onclick={() => openFileTab(f.path)} role="button" tabindex="0"
             onkeydown={(e) => { if (e.key === 'Enter') openFileTab(f.path) }}>
          <span class="st" style="color:{statusColor(f.status)}">{f.status}</span>
          <span class="name">{fileName(f.path)}</span>
          {#if dirName(f.path)}<span class="dir">{dirName(f.path)}</span>{/if}
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }
  .header {
    display: flex;
    align-items: center;
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
  }
  .branch {
    display: flex;
    align-items: center;
    gap: 3px;
    flex: 1;
    font-size: 11px;
    color: var(--foreground);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-left: 6px;
    font-weight: 500;
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 1px;
    margin-left: auto;
    flex-shrink: 0;
  }
  .hdr-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    padding: 3px 4px;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; user-select: none; }
  .empty {
    color: var(--muted);
    font-size: 12px;
    padding: 8px 12px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    cursor: pointer;
    font-size: 12px;
    white-space: nowrap;
    border-radius: 4px;
    margin: 0 4px;
    transition: background 0.08s;
  }
  .row:hover { background: var(--bg-hover); }
  .st {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px;
    font-weight: 700;
    width: 18px;
    flex-shrink: 0;
  }
  .name { overflow: hidden; text-overflow: ellipsis; }
  .dir {
    color: var(--muted);
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
