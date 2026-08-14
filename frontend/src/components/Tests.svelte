<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'
  import { GetGoTests } from '../lib/wails'
  import type { GoTest } from '../lib/wails'
  import { activeRightPanel, projectRoot, pendingGoto, openFileTab } from '../lib/stores'
  import { runGoTest, testStatus, testKey } from '../lib/goTests'
  import { IconRefresh, IconPlayerPlayFilled } from '@tabler/icons-svelte'

  let tests = $state<GoTest[]>([])
  let loading = $state(false)

  async function refresh() {
    const root = get(projectRoot)
    if (!root) { tests = []; return }
    loading = true
    tests = (await GetGoTests(root).catch(() => [])) ?? []
    loading = false
  }

  onMount(refresh)
  // panel stays mounted (display:none) when hidden — only refresh when
  // actually visible, or a project switch would spawn a full AST walk unseen
  $effect(() => {
    const active = $activeRightPanel === 'tests'
    void $projectRoot
    if (active) refresh()
  })

  const grouped = $derived.by(() => {
    const m = new Map<string, GoTest[]>()
    for (const t of tests) {
      const arr = m.get(t.file) ?? []
      arr.push(t)
      m.set(t.file, arr)
    }
    return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  })

  const statusOf = (t: GoTest) => $testStatus.get(testKey(t.pkg, t.name)) ?? 'idle'

  function jump(t: GoTest) {
    pendingGoto.set({ path: t.file, line: t.line, col: 0 })
    openFileTab(t.file, true)
  }

  function run(t: GoTest, e: MouseEvent) {
    e.stopPropagation()
    runGoTest(t.pkg, t.name)
  }

  function basename(path: string) { return path.split('/').pop() || path }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Tests</span>
    {#if tests.length}<span class="count">{tests.length}</span>{/if}
    <div class="header-actions">
      <button class="hdr-btn" onclick={refresh} title="Refresh"><IconRefresh size={13} /></button>
    </div>
  </div>

  <div class="list">
    {#if loading}
      <div class="empty">Scanning…</div>
    {:else if tests.length === 0}
      <div class="empty">No Go tests found</div>
    {:else}
      {#each grouped as [file, fileTests] (file)}
        <div class="file-row" title={file}>{basename(file)}</div>
        {#each fileTests as t (t.pkg + t.name)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div class="row" onclick={() => jump(t)} role="button" tabindex="0">
            <span class="dot" class:running={statusOf(t) === 'running'} class:passed={statusOf(t) === 'passed'} class:failed={statusOf(t) === 'failed'}></span>
            <span class="name">{t.name}</span>
            <button class="play-btn" onclick={(e) => run(t, e)} title="Run"><IconPlayerPlayFilled size={10} /></button>
          </div>
        {/each}
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
  .header-actions { display: flex; align-items: center; gap: 1px; margin-left: auto; }
  .hdr-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; }
  .empty { color: var(--muted); font-size: 12px; padding: 8px 12px; }

  .file-row {
    padding: 6px 12px 2px; font-size: 11px; font-weight: 600; color: var(--muted);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .row {
    display: flex; align-items: center; gap: 6px; width: 100%;
    padding: 3px 10px 3px 18px; background: none; border: none; cursor: pointer;
    text-align: left; color: var(--foreground); font-size: 12px; border-radius: 4px;
  }
  .row:hover { background: var(--bg-hover); }
  .row:hover .play-btn { opacity: 1; }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--muted); flex-shrink: 0; }
  .dot.running { background: var(--warning); }
  .dot.passed { background: var(--success); }
  .dot.failed { background: var(--error); }
  .name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: "SF Mono", Menlo, monospace; }
  .play-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--accent); cursor: pointer;
    padding: 2px 3px; border-radius: 3px; flex-shrink: 0; opacity: 0;
    transition: opacity 0.1s, background 0.1s;
  }
  .play-btn:hover { opacity: 1 !important; background: var(--bg-hover); }
</style>
