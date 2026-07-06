<script lang="ts">
  import { commands, focusedPane, selectedCommand, projectRoot, projectCommands } from '../lib/stores'
  import ContextMenu from './ContextMenu.svelte'
  import { RunCommand, RenameCommand, DeleteCommand, RunProjectCommand, DeleteProjectCommand } from '../lib/wails'

  let menu: { x: number; y: number; id: string } | null = $state(null)
  let renaming: { id: string; value: string } | null = $state(null)

  const isProject = $derived($projectRoot !== '')

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
      {#if $projectCommands.length}
        <span class="header-count">{$projectCommands.length}</span>
      {/if}
    {:else}
      {#if $commands.length}
        <span class="header-count">{$commands.length}</span>
      {/if}
    {/if}
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
            <span class="caret">›</span>
            <span class="cmd-name">{cmd.command}</span>
            <span class="dir-badge">{dirName(cmd.directory)}</span>
          </div>
        {/each}
      {/if}
    {:else}
      {#if $commands.length === 0}
        <div class="empty">use <code>w cmd</code> to save</div>
      {:else}
        {#each $commands as cmd (cmd.id)}
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
              <span class="caret">›</span>
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

  .caret {
    font-size: 14px;
    color: var(--accent);
    flex-shrink: 0;
    line-height: 1;
    opacity: 0.7;
  }
  .row.selected .caret { opacity: 1; }

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
</style>
