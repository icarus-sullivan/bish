<script lang="ts">
  import { panels } from '../lib/panels'
  import { activeRightPanel, focusedPane, openSettingsTab } from '../lib/stores'
  import type { Pane } from '../lib/stores'
  import { IconSettings } from '@tabler/icons-svelte'

  // keep the statusbar focus chip in sync (git has no pane — leave it alone)
  const paneFor: Record<string, Pane> = { files: 'tree', processes: 'processes', commands: 'commands' }

  function select(id: string) {
    activeRightPanel.set(id)
    if (paneFor[id]) focusedPane.set(paneFor[id])
  }
</script>

<div class="sidebar">
  <div class="panels">
    <!-- keep every panel mounted (display:none) so FileTree scroll/selection
         survives tab switches — same trick App.svelte uses for terminals -->
    {#each panels as p (p.id)}
      <div class="panel-host" style="display:{$activeRightPanel === p.id ? 'flex' : 'none'}">
        <p.component />
      </div>
    {/each}
  </div>
  <div class="strip">
    {#each panels as p (p.id)}
      <button
        class="hdr-btn"
        class:active={$activeRightPanel === p.id}
        onclick={() => select(p.id)}
        title={p.title}
      >
        <p.icon size={20} />
      </button>
    {/each}
    <button class="hdr-btn settings" onclick={openSettingsTab} title="Settings">
      <IconSettings size={20} />
    </button>
  </div>
</div>

<style>
  .sidebar {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .panels {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .panel-host {
    flex: 1;
    min-height: 0;
    flex-direction: column;
    overflow: hidden;
  }
  .strip {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    width: 44px;
    flex-shrink: 0;
    padding: 8px;
    background: var(--bg-raised);
    border-left: 1px solid var(--border);
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
  .hdr-btn.active { color: var(--foreground); }
  .hdr-btn.settings { margin-top: auto; margin-bottom: 4px; }
</style>
