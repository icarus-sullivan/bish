<script lang="ts">
  import { panels } from '../lib/panels'
  import { activeRightPanel, focusedPane, openSettingsTab, panelSide,
           floatingPanels, showRight, nextZ } from '../lib/stores'
  import type { Pane, FloatingPanel } from '../lib/stores'
  import { featureOn, features } from '../lib/features'
  import { IconSettings } from '@tabler/icons-svelte'
  import ContextMenu from './ContextMenu.svelte'
  import { get } from 'svelte/store'

  // re-evaluate gating when toggles change ($features touched for reactivity)
  const visible = $derived.by(() => {
    void $features
    return $panels.filter(p => !p.feature || featureOn(p.feature))
  })

  const floatingIds = $derived(new Set($floatingPanels.map(f => f.panelId)))
  // docked panels only — popped-out ones are mounted by FloatingWindow instead,
  // never both at once
  const docked = $derived(visible.filter(p => !floatingIds.has(p.id)))

  // keep the statusbar focus chip in sync (git has no pane — leave it alone)
  const paneFor: Record<string, Pane> = { files: 'tree', processes: 'processes', commands: 'commands' }

  function select(id: string) {
    if (floatingIds.has(id)) {
      // already popped out — clicking its gutter icon just brings it to front
      floatingPanels.update(fs => fs.map(f => f.panelId === id ? { ...f, z: nextZ() } : f))
      return
    }
    activeRightPanel.set(id)
    if (paneFor[id]) focusedPane.set(paneFor[id])
  }

  let menu = $state<{ x: number; y: number; panelId: string } | null>(null)

  // Anchor to the icon's own bottom edge rather than the click point, and
  // open toward the panel body (leftward when docked right, rightward when
  // docked left) so the menu never runs off the window's outer edge.
  function showMenu(e: MouseEvent, id: string) {
    e.preventDefault()
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const x = $panelSide === 'left' ? rect.left : rect.right
    menu = { x, y: rect.bottom, panelId: id }
  }

  function popOut(id: string) {
    const count = get(floatingPanels).length
    const fp: FloatingPanel = {
      panelId: id, width: 360, height: 420, z: nextZ(),
      x: 80 + 24 * count, y: 80 + 24 * count,
    }
    floatingPanels.update(fs => [...fs, fp])
    // if the panel we just popped out was active, fall back to another docked one
    if (get(activeRightPanel) === id) {
      const next = docked.find(p => p.id !== id)
      if (next) activeRightPanel.set(next.id)
    }
  }

  function dockBack(id: string) {
    floatingPanels.update(fs => fs.filter(f => f.panelId !== id))
    activeRightPanel.set(id)
    showRight.set(true)
  }

  function menuItems(id: string) {
    return floatingIds.has(id)
      ? [{ label: 'Dock back', action: () => dockBack(id) }]
      : [{ label: 'Open in floating window', action: () => popOut(id) }]
  }
</script>

<div class="sidebar" class:leftside={$panelSide === 'left'}>
  <div class="panels">
    <!-- keep every panel mounted (display:none) so FileTree scroll/selection
         survives tab switches — same trick App.svelte uses for terminals -->
    {#each docked as p (p.id)}
      <div class="panel-host" style="display:{$activeRightPanel === p.id ? 'flex' : 'none'}">
        <p.component {...(p.props ?? {})} />
      </div>
    {/each}
  </div>
  <div class="strip" class:leftside={$panelSide === 'left'}>
    {#each visible as p (p.id)}
      <button
        class="hdr-btn"
        class:active={$activeRightPanel === p.id && !floatingIds.has(p.id)}
        class:floating={floatingIds.has(p.id)}
        onclick={() => select(p.id)}
        oncontextmenu={(e) => showMenu(e, p.id)}
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

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} align={$panelSide === 'left' ? 'left' : 'right'} items={menuItems(menu.panelId)} onClose={() => menu = null} />
{/if}

<style>
  .sidebar {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  /* docked left: gutter strip belongs on the outer edge (far left), panels
     stay adjacent to the resize handle — mirror image of the right dock */
  .sidebar.leftside { flex-direction: row-reverse; }
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
  .strip.leftside { border-left: none; border-right: 1px solid var(--border); }
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
  .hdr-btn.floating { color: var(--accent); }
  .hdr-btn.settings { margin-top: auto; margin-bottom: 4px; }
</style>
