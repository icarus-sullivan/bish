<script lang="ts">
  import { loadedExtensions, extensionPanelHTML } from '../lib/extensions'
  import { IconPuzzle } from '@tabler/icons-svelte'
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Extensions</span>
    {#if $loadedExtensions.length}<span class="count">{$loadedExtensions.length}</span>{/if}
  </div>

  <div class="list">
    {#if $loadedExtensions.length === 0}
      <div class="empty">
        <IconPuzzle size={20} />
        <p>No extensions installed.</p>
        <p class="hint">Drop one in <code>~/.bish/extensions/&lt;name&gt;/</code> with a <code>bish-extension.json</code> manifest.</p>
      </div>
    {:else}
      {#each $loadedExtensions as ext (ext.name)}
        <div class="ext">
          <div class="ext-name">{ext.name}</div>
          {#if !ext.enabled}
            <div class="ext-disabled">Disabled — enable it in Settings</div>
          {:else if !ext.panels || ext.panels.length === 0}
            <div class="ext-disabled">No panels contributed</div>
          {:else}
            {#each ext.panels as p (p.id)}
              <div class="ext-panel-title">{p.title}</div>
              <div class="ext-panel-body">
                {@html $extensionPanelHTML.get(`${ext.name}:${p.id}`) ?? '<span class="ext-waiting">…</span>'}
              </div>
            {/each}
          {/if}
        </div>
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

  .list { overflow-y: auto; flex: 1; }
  .empty {
    display: flex; flex-direction: column; align-items: center; gap: 6px;
    color: var(--muted); font-size: 12px; padding: 32px 20px; text-align: center;
  }
  .empty p { margin: 0; }
  .empty .hint { font-size: 11px; }
  .empty code { font-family: "SF Mono", Menlo, monospace; color: var(--accent); }

  .ext { border-bottom: 1px solid var(--border); padding: 10px 12px; }
  .ext-name { font-size: 12px; font-weight: 600; color: var(--foreground); margin-bottom: 4px; }
  .ext-disabled { font-size: 11px; color: var(--muted); font-style: italic; }
  .ext-panel-title {
    font-size: 10px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--muted); margin: 6px 0 3px;
  }
  .ext-panel-body { font-size: 12px; color: var(--foreground); }
  .ext-panel-body :global(.ext-waiting) { color: var(--muted); }
</style>
