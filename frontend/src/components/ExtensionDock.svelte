<script lang="ts">
  import { onDestroy } from 'svelte'
  import { extensionPanels } from '../lib/panels'
  import { activeExtPanel } from '../lib/stores'
  import ExtensionPanelHost from './ExtensionPanelHost.svelte'

  // One extension at a time, opened from its tab-bar icon. Like
  // RightSidebar, a panel mounts the first time it's shown and then stays
  // mounted (hidden) so switching extensions keeps scroll/input state.
  // (Plain subscribe, not $effect: no reactive writes.)
  let mountedIds = $state<string[]>([])
  const unsub = activeExtPanel.subscribe(id => {
    if (id && !mountedIds.includes(id)) mountedIds = [...mountedIds, id]
  })
  onDestroy(unsub)

  const close = () => activeExtPanel.set(null)
</script>

<div class="dock">
  {#each $extensionPanels as p (p.id)}
    {#if mountedIds.includes(p.id)}
      <div class="panel-host" style="display:{$activeExtPanel === p.id ? 'flex' : 'none'}">
        <ExtensionPanelHost extName={p.extName} panelId={p.panelId} onClose={close} />
      </div>
    {/if}
  {/each}
</div>

<style>
  .dock {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .panel-host {
    flex: 1;
    min-height: 0;
    flex-direction: column;
    overflow: hidden;
  }
</style>
