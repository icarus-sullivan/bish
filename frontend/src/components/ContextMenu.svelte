<script lang="ts">
  import { onMount } from 'svelte'
  import { registerKeybind } from '../lib/keybinds'

  interface MenuItem {
    label: string
    action: () => void
    danger?: boolean
  }

  let { x = 0, y = 0, items = [], onClose }: {
    x: number; y: number; items: MenuItem[]; onClose: () => void
  } = $props()

  function handleAction(item: MenuItem) {
    item.action()
    onClose()
  }

  onMount(() => registerKeybind({ combo: 'escape', handler: onClose }))
</script>

<svelte:window onclick={onClose} />

<div class="menu" style="left:{x}px; top:{y}px" role="menu">
  {#each items as item, i}
    {#if i > 0 && item.danger && !items[i-1].danger}
      <div class="sep"></div>
    {/if}
    <button
      class="item"
      class:danger={item.danger}
      onclick={(e) => { e.stopPropagation(); handleAction(item) }}
      role="menuitem"
    >
      <span class="item-label">{item.label}</span>
    </button>
  {/each}
</div>

<style>
  .menu {
    position: fixed;
    z-index: 9999;
    background: color-mix(in srgb, var(--background) 85%, var(--border) 15%);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 5px 0;
    min-width: 170px;
    box-shadow:
      0 4px 6px -1px rgba(0,0,0,0.3),
      0 10px 24px -4px rgba(0,0,0,0.4),
      0 0 0 0.5px rgba(255,255,255,0.04) inset;
    font-size: 12px;
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
  }

  .sep {
    height: 1px;
    background: var(--border);
    margin: 4px 0;
  }

  .item {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 6px 14px;
    text-align: left;
    background: none;
    border: none;
    color: var(--foreground);
    cursor: pointer;
    font-size: 12px;
    font-family: -apple-system, "SF Pro Text", sans-serif;
    gap: 8px;
    border-radius: 4px;
    margin: 0 4px;
    width: calc(100% - 8px);
    transition: background 0.08s;
  }
  .item:hover { background: var(--bg-selected); }
  .item.danger { color: var(--error); }
  .item.danger:hover { background: color-mix(in srgb, var(--error) 12%, transparent); }
  .item-label { flex: 1; }
</style>
