<script lang="ts">
  import { tabs, activeTabId, closeTab, addTerminalTab,
           closeTabsToRight, closeTabsToLeft, closeOtherTabs, closeAllTabs,
           type Tab } from '../lib/stores'
  import { NewTerminal, CloseTerminal } from '../lib/wails'
  import { IconTerminal2, IconFile, IconListDetails, IconPlus, IconX } from '@tabler/icons-svelte'
  import ContextMenu from './ContextMenu.svelte'

  async function newTerminal() {
    try {
      const id = await NewTerminal()
      addTerminalTab(id)
    } catch {}
  }

  function handleClose(e: MouseEvent, tab: Tab) {
    e.stopPropagation()
    if (tab.type === 'terminal' && tab.id !== 'main') {
      CloseTerminal(tab.id)
    }
    closeTab(tab.id)
  }

  function tabIcon(tab: Tab) {
    if (tab.type === 'terminal') return IconTerminal2
    if (tab.type === 'logs') return IconListDetails
    return IconFile
  }

  // right-click context menu
  let tabMenu = $state<{ x: number; y: number; tab: Tab } | null>(null)

  function showTabMenu(e: MouseEvent, tab: Tab) {
    e.preventDefault()
    e.stopPropagation()
    tabMenu = { x: e.clientX, y: e.clientY, tab }
  }

  function withPtyCleanup(termIds: string[]) {
    for (const id of termIds) CloseTerminal(id)
  }

  function menuItems(tab: Tab) {
    const current = $tabs
    const idx = current.findIndex(t => t.id === tab.id)
    const hasRight = idx < current.length - 1
    const hasLeft = idx > 0
    return [
      {
        label: 'Close',
        action: () => {
          if (tab.type === 'terminal' && tab.id !== 'main') CloseTerminal(tab.id)
          closeTab(tab.id)
        },
      },
      {
        label: 'Close Others',
        action: () => withPtyCleanup(closeOtherTabs(tab.id)),
      },
      ...(hasLeft ? [{
        label: 'Close All to Left',
        action: () => withPtyCleanup(closeTabsToLeft(tab.id)),
      }] : []),
      ...(hasRight ? [{
        label: 'Close All to Right',
        action: () => withPtyCleanup(closeTabsToRight(tab.id)),
      }] : []),
      {
        label: 'Close All',
        action: () => withPtyCleanup(closeAllTabs()),
        danger: true,
      },
    ]
  }
</script>

<div class="tabbar">
  {#each $tabs as tab (tab.id)}
    <button
      class="tab"
      class:active={$activeTabId === tab.id}
      onclick={() => activeTabId.set(tab.id)}
      oncontextmenu={(e) => showTabMenu(e, tab)}
    >
      <svelte:component this={tabIcon(tab)} size={11} />
      <span class="tab-label">{tab.label}</span>
      {#if tab.type !== 'terminal' || $tabs.filter(t => t.type === 'terminal').length > 1}
        <button class="tab-close" onclick={(e) => handleClose(e, tab)} title="Close">
          <IconX size={10} />
        </button>
      {/if}
    </button>
  {/each}
  <button class="new-terminal" onclick={newTerminal} title="New Terminal">
    <IconPlus size={12} />
  </button>
</div>

{#if tabMenu}
  <ContextMenu
    x={tabMenu.x}
    y={tabMenu.y}
    items={menuItems(tabMenu.tab)}
    onClose={() => tabMenu = null}
  />
{/if}

<style>
  .tabbar {
    display: flex;
    align-items: stretch;
    height: 32px;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
    overflow-x: auto;
    overflow-y: hidden;
    flex-shrink: 0;
    scrollbar-width: none;
  }
  .tabbar::-webkit-scrollbar { display: none; }

  .tab {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 0 10px;
    border: none;
    border-right: 1px solid var(--border);
    background: transparent;
    color: var(--muted);
    font-size: 11px;
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    transition: color 0.1s, background 0.1s;
    position: relative;
  }
  .tab:hover { color: var(--foreground); background: var(--bg-hover); }
  .tab.active {
    color: var(--foreground);
    background: var(--background);
    border-bottom: 2px solid var(--accent);
  }

  .tab-label {
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tab-close {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: inherit;
    cursor: pointer;
    padding: 2px;
    border-radius: 3px;
    opacity: 0.5;
    margin-left: 2px;
  }
  .tab-close:hover { opacity: 1; background: var(--bg-hover); }

  .new-terminal {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0 10px;
    border: none;
    background: none;
    color: var(--muted);
    cursor: pointer;
    flex-shrink: 0;
    transition: color 0.1s;
  }
  .new-terminal:hover { color: var(--foreground); }
</style>
