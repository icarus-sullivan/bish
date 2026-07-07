<script lang="ts">
  import { commands, focusedPane, selectedCommand, projectRoot, projectCommands, cwd } from '../lib/stores'
  import ContextMenu from './ContextMenu.svelte'
  import { RunCommand, RenameCommand, DeleteCommand, RunProjectCommand, DeleteProjectCommand, AddCommand, AddProjectCommand } from '../lib/wails'
  import { IconPlus, IconPlayerPlayFilled } from '@tabler/icons-svelte'
  import { get } from 'svelte/store'

  let menu: { x: number; y: number; id: string } | null = $state(null)
  let renaming: { id: string; value: string } | null = $state(null)

  let showAdd = $state(false)
  let addName = $state('')
  let addCommand = $state('')
  let addCwd = $state('')

  let addError = $state('')

  function openAdd() {
    addName = ''
    addCommand = ''
    addCwd = get(cwd)

    addError = ''
    showAdd = true
  }

  async function submitAdd() {
    if (!addCommand.trim()) { addError = 'Command is required'; return }
    const dir = addCwd.trim() || get(cwd)
    if (isProject) {
      await AddProjectCommand(addCommand.trim(), dir).catch((e: any) => { addError = String(e) })
    } else {
      await AddCommand(addName.trim(), dir, addCommand.trim()).catch((e: any) => { addError = String(e) })
    }
    showAdd = false
  }

  const isProject = $derived($projectRoot !== '')
  const visibleCommands = $derived($commands.filter(c => c.cwd === $cwd))

  function select(id: string) {
    selectedCommand.set(id)
    focusedPane.set('commands')
  }

  function run(id: string) {
    if (isProject) RunProjectCommand(id)
    else RunCommand(id)
    focusedPane.set('terminal')
  }

  function showMenu(e: MouseEvent, id: string) {
    e.preventDefault()
    select(id)
    menu = { x: e.clientX, y: e.clientY, id }
  }

  function startRename(id: string) {
    const cmd = $commands.find(c => c.id === id)
    if (cmd) renaming = { id, value: cmd.name || cmd.command }
  }

  async function commitRename() {
    if (!renaming) return
    await RenameCommand(renaming.id, renaming.value)
    renaming = null
  }

  function menuItems(id: string) {
    if (isProject) {
      return [
        { label: 'Run', action: () => run(id) },
        { label: 'Delete', action: () => DeleteProjectCommand(id), danger: true },
      ]
    }
    return [
      { label: 'Run', action: () => run(id) },
      { label: 'Rename', action: () => startRename(id) },
      { label: 'Delete', action: () => DeleteCommand(id), danger: true },
    ]
  }

  function dirName(path: string) {
    return path.split('/').pop() || path
  }
</script>

<div class="panel" class:focused={$focusedPane === 'commands'}>
  <div class="header">
    <span class="header-label">{isProject ? 'Project Commands' : 'Saved Commands'}</span>
    {#if isProject}
      {#if $projectCommands.length}<span class="header-count">{$projectCommands.length}</span>{/if}
    {:else}
      {#if visibleCommands.length}<span class="header-count">{visibleCommands.length}</span>{/if}
    {/if}
    <button class="header-btn" onclick={openAdd} title="Add command"><IconPlus size={13} /></button>
  </div>
  <div class="list">
    {#if isProject}
      {#if $projectCommands.length === 0}
        <div class="empty">use <code>w cmd</code> to save</div>
      {:else}
        {#each $projectCommands as cmd (cmd.id)}
          <div
            class="row"
            class:selected={$selectedCommand === cmd.id}
            onclick={() => select(cmd.id)}
            ondblclick={() => run(cmd.id)}
            oncontextmenu={(e) => showMenu(e, cmd.id)}
            role="row"
            tabindex="0"
            onkeydown={(e) => e.key === 'Enter' && run(cmd.id)}
          >
            <button class="play-btn" onclick={(e) => { e.stopPropagation(); run(cmd.id) }} title="Run"><IconPlayerPlayFilled size={10} /></button>
            <span class="cmd-name">{cmd.command}</span>
            <span class="dir-badge">{dirName(cmd.directory)}</span>
          </div>
        {/each}
      {/if}
    {:else}
      {#if visibleCommands.length === 0}
        <div class="empty">use <code>w cmd</code> to save</div>
      {:else}
        {#each visibleCommands as cmd (cmd.id)}
          <div
            class="row"
            class:selected={$selectedCommand === cmd.id}
            onclick={() => select(cmd.id)}
            ondblclick={() => run(cmd.id)}
            oncontextmenu={(e) => showMenu(e, cmd.id)}
            role="row"
            tabindex="0"
            onkeydown={(e) => e.key === 'Enter' && run(cmd.id)}
          >
            {#if renaming?.id === cmd.id}
              <input
                class="rename-input"
                bind:value={renaming.value}
                onblur={commitRename}
                onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') renaming = null }}
              />
            {:else}
              <button class="play-btn" onclick={(e) => { e.stopPropagation(); run(cmd.id) }} title="Run"><IconPlayerPlayFilled size={10} /></button>
              <span class="cmd-name">{cmd.name || cmd.command}</span>
              <span class="dir-badge">{dirName(cmd.cwd)}</span>
            {/if}
          </div>
        {/each}
      {/if}
    {/if}
  </div>
</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menuItems(menu.id)} onClose={() => menu = null} />
{/if}

{#if showAdd}
  <div class="add-overlay" onclick={() => showAdd = false} role="dialog" aria-modal="true">
    <div class="add-panel" onclick={(e) => e.stopPropagation()}>
      <div class="add-header">
        <span class="add-title">Add Command</span>
        <button class="add-close" onclick={() => showAdd = false}>✕</button>
      </div>
      <div class="add-body">
        <input class="add-input" bind:value={addCommand} placeholder="command *"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitAdd(); if (e.key === 'Escape') showAdd = false }} />
        {#if !isProject}
        <input class="add-input" bind:value={addName} placeholder="name (optional)"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitAdd(); if (e.key === 'Escape') showAdd = false }} />
        {/if}
        <input class="add-input" bind:value={addCwd} placeholder="working directory (optional)"
          autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') submitAdd(); if (e.key === 'Escape') showAdd = false }} />
        {#if addError}<div class="add-error">{addError}</div>{/if}
      </div>
      <div class="add-footer">
        <button class="add-btn-cancel" onclick={() => showAdd = false}>Cancel</button>
        <button class="add-btn-submit" onclick={submitAdd}>Add</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    border-right: 1px solid var(--border);
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 12px;
    height: 30px;
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
    transition: color 0.15s;
  }
  .panel.focused .header-label { color: var(--accent); }
  .header-count {
    font-size: 10px;
    color: var(--muted);
    background: var(--bg-hover);
    padding: 1px 6px;
    border-radius: 8px;
  }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; }
  .empty { padding: 10px 12px; color: var(--muted); font-size: 11px; font-style: italic; }
  .empty code { color: var(--accent); font-style: normal; font-family: "SF Mono", Menlo, monospace; }

  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 12px;
    cursor: pointer;
    font-size: 12px;
    user-select: none;
    border-radius: 4px;
    margin: 0 4px;
    transition: background 0.1s;
  }
  .row:hover { background: var(--bg-hover); }
  .row.selected { background: var(--bg-selected); }

  .cmd-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dir-badge {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 10px;
    color: var(--muted);
    background: var(--bg-hover);
    padding: 1px 5px;
    border-radius: 3px;
    flex-shrink: 0;
    max-width: 80px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .rename-input {
    flex: 1;
    background: var(--bg-hover);
    border: 1px solid var(--border-focused);
    border-radius: 4px;
    color: var(--foreground);
    font-size: 12px;
    padding: 2px 6px;
    outline: none;
  }

  .header-btn {
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
  .header-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .play-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--accent); cursor: pointer;
    padding: 2px 3px; border-radius: 3px; flex-shrink: 0; opacity: 0.6;
    transition: opacity 0.1s, background 0.1s;
  }
  .play-btn:hover { opacity: 1; background: var(--bg-hover); }

  /* ── add command dialog ── */
  .add-overlay {
    position: fixed; inset: 0; z-index: 9000;
    background: rgba(0,0,0,0.45);
    display: flex; align-items: center; justify-content: center;
  }
  .add-panel {
    width: 340px; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
    display: flex; flex-direction: column; overflow: hidden;
  }
  .add-header {
    display: flex; align-items: center;
    padding: 10px 14px 8px; border-bottom: 1px solid var(--border);
  }
  .add-title { font-size: 12px; font-weight: 600; color: var(--muted); flex: 1; }
  .add-close {
    background: none; border: none; color: var(--muted);
    cursor: pointer; font-size: 13px; padding: 2px 5px; border-radius: 3px;
  }
  .add-close:hover { color: var(--foreground); background: var(--bg-hover); }
  .add-body {
    padding: 12px 14px; display: flex; flex-direction: column; gap: 8px;
  }
  .add-input {
    background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 12px;
    padding: 6px 8px; outline: none;
    font-family: "SF Mono", Menlo, monospace;
  }
  .add-input:focus { border-color: var(--accent); }
  .add-error { font-size: 11px; color: var(--error); }
  .add-footer {
    display: flex; justify-content: flex-end; gap: 8px;
    padding: 8px 14px 12px; border-top: 1px solid var(--border);
  }
  .add-btn-cancel {
    background: none; border: 1px solid var(--border); border-radius: 5px;
    color: var(--muted); font-size: 11px; padding: 5px 12px; cursor: pointer;
  }
  .add-btn-cancel:hover { color: var(--foreground); background: var(--bg-hover); }
  .add-btn-submit {
    background: var(--accent); border: none; border-radius: 5px;
    color: #000; font-size: 11px; font-weight: 600; padding: 5px 14px; cursor: pointer;
  }
  .add-btn-submit:hover { opacity: 0.85; }
</style>
