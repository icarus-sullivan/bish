<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'
  import { GitStatus, GitStage, GitUnstage, GitCommit, GitBranches, GitCheckout, on } from '../lib/wails'
  import type { GitStatusDTO, GitFileStatus } from '../lib/wails'
  import { activeRightPanel, projectRoot, openFileTab, openDiffTab } from '../lib/stores'
  import { featureOn } from '../lib/features'
  import { IconRefresh, IconGitBranch, IconPlus, IconMinus, IconCheck, IconChevronDown } from '@tabler/icons-svelte'

  let status = $state<GitStatusDTO | null>(null)
  let branches = $state<string[]>([])
  let message = $state('')
  let busy = $state(false)
  let error = $state('')

  const openChange = (p: string) => featureOn('gitDiff') ? openDiffTab(p) : openFileTab(p)

  async function refresh() {
    status = await GitStatus().catch(() => null)
    branches = await GitBranches().catch(() => [])
  }

  // this panel stays mounted (display:none) when hidden — only hit git when
  // it's actually visible, or a switch to another panel would still spawn
  // `git status`/`git branch` on every panel change.
  onMount(() => on('tree:update', () => { if (get(activeRightPanel) === 'git') refresh() }))

  $effect(() => {
    const active = $activeRightPanel === 'git'
    void $projectRoot
    if (active) refresh()
  })

  // XY porcelain: X=index(staged), Y=worktree(unstaged); "??" untracked
  const isStaged   = (s: string) => s[0] !== ' ' && s[0] !== '?'
  const isUnstaged = (s: string) => s[1] !== ' ' || s === '??'
  const staged   = $derived(((status?.files ?? []) as GitFileStatus[]).filter(f => isStaged(f.status)))
  const unstaged = $derived(((status?.files ?? []) as GitFileStatus[]).filter(f => isUnstaged(f.status)))

  async function act(fn: () => Promise<void>) {
    busy = true; error = ''
    try { await fn() } catch (e: any) { error = String(e?.message ?? e) }
    busy = false
    await refresh()
  }

  const stage   = (p: string) => act(() => GitStage(p))
  const unstage = (p: string) => act(() => GitUnstage(p))
  async function commit() {
    if (!message.trim() || staged.length === 0) return
    await act(() => GitCommit(message))
    if (!error) message = ''
  }
  async function switchBranch(e: Event) {
    const b = (e.target as HTMLSelectElement).value
    if (b && b !== status?.branch) await act(() => GitCheckout(b))
  }

  function statusColor(s: string): string {
    if (s.includes('?')) return 'var(--muted)'
    if (s.includes('D')) return 'var(--error)'
    if (s.includes('A')) return 'var(--success)'
    return 'var(--warning)'
  }
  const code = (s: string) => s.trim() || s
  function fileName(p: string) { return p.split('/').pop() || p }
  function dirName(p: string) {
    const root = $projectRoot
    let rel = root && p.startsWith(root + '/') ? p.slice(root.length + 1) : p
    const i = rel.lastIndexOf('/')
    return i === -1 ? '' : rel.slice(0, i)
  }
</script>

{#snippet fileRow(f: GitFileStatus, action: 'stage' | 'unstage')}
  <div class="row">
    <span class="st" style="color:{statusColor(f.status)}">{code(f.status)}</span>
    <button class="name-btn" onclick={() => openChange(f.path)} title={f.path}>
      <span class="name">{fileName(f.path)}</span>
      {#if dirName(f.path)}<span class="dir">{dirName(f.path)}</span>{/if}
    </button>
    {#if action === 'stage'}
      <button class="act-btn" title="Stage" disabled={busy} onclick={() => stage(f.path)}><IconPlus size={13} /></button>
    {:else}
      <button class="act-btn" title="Unstage" disabled={busy} onclick={() => unstage(f.path)}><IconMinus size={13} /></button>
    {/if}
  </div>
{/snippet}

<div class="panel">
  <div class="header">
    <span class="header-label">Git</span>
    {#if status?.branch}
      <span class="branch-wrap" title={status.branch}>
        <IconGitBranch size={11} />
        <span class="select-wrap">
          <select class="branch-select" value={status.branch} onchange={switchBranch} disabled={busy}>
            {#if !branches.includes(status.branch)}<option>{status.branch}</option>{/if}
            {#each branches as b}<option value={b}>{b}</option>{/each}
          </select>
          <IconChevronDown size={12} class="select-chevron" />
        </span>
      </span>
    {/if}
    <div class="header-actions">
      <button class="hdr-btn" onclick={refresh} title="Refresh"><IconRefresh size={13} /></button>
    </div>
  </div>

  {#if !status}
    <div class="empty">Not a git repository</div>
  {:else}
    <div class="commit-box">
      <input class="commit-input" placeholder="Commit message…" bind:value={message}
             onkeydown={(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) commit() }} />
      <button class="commit-btn" disabled={busy || !message.trim() || staged.length === 0} onclick={commit}>
        <IconCheck size={13} /> Commit{staged.length ? ` (${staged.length})` : ''}
      </button>
    </div>
    {#if error}<div class="err">{error}</div>{/if}

    <div class="list">
      {#if status.files.length === 0}
        <div class="empty">No changes</div>
      {:else}
        {#if staged.length}
          <div class="section">Staged</div>
          {#each staged as f (f.path)}{@render fileRow(f, 'unstage')}{/each}
        {/if}
        {#if unstaged.length}
          <div class="section">Changes</div>
          {#each unstaged as f (f.path)}{@render fileRow(f, 'stage')}{/each}
        {/if}
      {/if}
    </div>
  {/if}
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
  .branch-wrap {
    display: flex; align-items: center; gap: 5px; flex: 1;
    margin-left: 6px; color: var(--muted); min-width: 0;
  }
  .select-wrap { position: relative; display: inline-flex; align-items: center; flex: 1; min-width: 0; }
  .branch-select {
    appearance: none;
    -webkit-appearance: none;
    width: 100%;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 11px;
    font-weight: 500;
    padding: 3px 24px 3px 8px;
    outline: none;
    cursor: pointer;
    text-overflow: ellipsis;
    transition: border-color 0.1s, background 0.1s;
  }
  .branch-select:hover { background: var(--bg-hover); }
  .branch-select:focus { border-color: var(--accent); }
  .branch-select option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) {
    position: absolute;
    right: 7px;
    color: var(--muted);
    pointer-events: none;
  }
  .header-actions { display: flex; align-items: center; gap: 1px; margin-left: auto; flex-shrink: 0; }
  .hdr-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .commit-box { display: flex; flex-direction: column; gap: 6px; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .commit-input {
    background: var(--background); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 12px; padding: 5px 8px; outline: none;
  }
  .commit-input:focus { border-color: var(--accent); }
  .commit-btn {
    display: flex; align-items: center; justify-content: center; gap: 5px;
    padding: 5px 10px; background: var(--accent); color: #000; border: none;
    border-radius: 5px; font-size: 11px; font-weight: 600; cursor: pointer;
    transition: opacity 0.1s;
  }
  .commit-btn:disabled { opacity: 0.4; cursor: default; }
  .commit-btn:not(:disabled):hover { opacity: 0.85; }
  .err { color: var(--error); font-size: 11px; padding: 6px 12px; white-space: pre-wrap; }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; user-select: none; }
  .section {
    font-size: 10px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase;
    color: var(--muted); padding: 6px 12px 3px;
  }
  .empty { color: var(--muted); font-size: 12px; padding: 8px 12px; }
  .row {
    display: flex; align-items: center; gap: 6px; padding: 2px 8px 2px 10px;
    font-size: 12px; white-space: nowrap; border-radius: 4px; margin: 0 4px;
  }
  .row:hover { background: var(--bg-hover); }
  .st { font-family: "SF Mono", Menlo, monospace; font-size: 10px; font-weight: 700; width: 18px; flex-shrink: 0; }
  .name-btn {
    display: flex; align-items: baseline; gap: 6px; flex: 1; min-width: 0;
    background: none; border: none; color: var(--foreground); cursor: pointer;
    text-align: left; padding: 2px 0; overflow: hidden;
  }
  .name { overflow: hidden; text-overflow: ellipsis; }
  .dir { color: var(--muted); font-size: 11px; overflow: hidden; text-overflow: ellipsis; }
  .act-btn {
    display: flex; align-items: center; justify-content: center; flex-shrink: 0;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 2px 3px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .act-btn:hover:not(:disabled) { color: var(--foreground); background: var(--bg-selected); }
  .act-btn:disabled { opacity: 0.4; cursor: default; }
</style>
