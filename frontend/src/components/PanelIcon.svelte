<script lang="ts">
  import type { Panel, ExtensionPanel } from '../lib/panels'

  // Renders a panel's icon at `size` px whatever form it takes: a tabler
  // component (built-ins + named extension icons), sanitized inline SVG
  // markup, or an <img> (URL / data URI) for extension-supplied icons.
  let { panel, size }: { panel: Panel | ExtensionPanel; size: number } = $props()

  const svg = $derived((panel as ExtensionPanel).iconSvg ?? '')
  const src = $derived((panel as ExtensionPanel).iconSrc ?? '')
</script>

{#if svg}
  <span class="custom-icon" style="width:{size}px;height:{size}px">{@html svg}</span>
{:else if src}
  <img class="custom-icon" {src} alt="" width={size} height={size} draggable="false" />
{:else}
  <panel.icon {size} />
{/if}

<style>
  .custom-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    object-fit: contain;
  }
  .custom-icon :global(svg) { width: 100%; height: 100%; }
</style>
