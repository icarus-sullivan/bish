<script lang="ts">
  import type { Panel } from '../lib/panels'
  import type { FloatingPanel } from '../lib/stores'
  import { floatingPanels, activeRightPanel, showRight, panelSide, nextZ } from '../lib/stores'
  import { IconLayoutSidebarRight, IconLayoutSidebar, IconGripHorizontal } from '@tabler/icons-svelte'

  let { panel, state }: { panel: Panel; state: FloatingPanel } = $props()

  function update(patch: Partial<FloatingPanel>) {
    floatingPanels.update(fs => fs.map(f => f.panelId === state.panelId ? { ...f, ...patch } : f))
  }

  function focus() {
    update({ z: nextZ() })
  }

  function dockBack() {
    floatingPanels.update(fs => fs.filter(f => f.panelId !== state.panelId))
    activeRightPanel.set(state.panelId)
    showRight.set(true)
  }

  function startDrag(e: MouseEvent) {
    e.preventDefault()
    focus()
    const startX = e.clientX
    const startY = e.clientY
    const startLeft = state.x
    const startTop = state.y

    function onMove(ev: MouseEvent) {
      const maxX = window.innerWidth - 120
      const maxY = window.innerHeight - 40
      update({
        x: Math.max(0, Math.min(maxX, startLeft + (ev.clientX - startX))),
        y: Math.max(0, Math.min(maxY, startTop + (ev.clientY - startY))),
      })
    }
    function onUp() {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
</script>

<div
  class="float-win"
  style="left:{state.x}px; top:{state.y}px; width:{state.width}px; height:{state.height}px; z-index:{state.z}"
  onmousedown={focus}
  role="dialog"
  aria-label={panel.title}
  tabindex="-1"
>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="float-header" onmousedown={startDrag}>
    <panel.icon size={14} />
    <span class="float-title">{panel.title}</span>
    <IconGripHorizontal size={13} class="grip" />
    <button class="hdr-btn" onclick={dockBack} title="Dock back to sidebar">
      {#if $panelSide === 'left'}
        <IconLayoutSidebar size={13} />
      {:else}
        <IconLayoutSidebarRight size={13} />
      {/if}
    </button>
  </div>
  <div class="float-body">
    <panel.component />
  </div>
</div>

<style>
  .float-win {
    position: fixed;
    display: flex;
    flex-direction: column;
    background: var(--background);
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    box-shadow:
      0 4px 6px -1px rgba(0,0,0,0.3),
      0 10px 24px -4px rgba(0,0,0,0.4);
  }
  .float-header {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    padding: 6px 8px;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
    cursor: move;
    user-select: none;
    color: var(--muted);
  }
  .float-title {
    flex: 1;
    font-size: 12px;
    color: var(--foreground);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .float-header :global(.grip) { opacity: 0.5; }
  .float-body {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
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
</style>
