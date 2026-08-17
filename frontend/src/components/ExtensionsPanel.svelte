<script lang="ts">
  import { loadedExtensions, extensionPanelHTML, sendPanelInput, setExtensionEnabled, uninstallExtension } from '../lib/extensions'
  import { IconPuzzle, IconPower, IconTrash } from '@tabler/icons-svelte'

  // keyed by `${extName}:${panelId}` — a panel's `{@html}` body is sanitized
  // and inert (no event handlers survive DOMPurify), so this is the one real
  // input surface a panel gets: typed text is relayed to the worker on submit.
  const inputs: Record<string, string> = {}

  function submit(extName: string, panelId: string) {
    const key = `${extName}:${panelId}`
    const value = (inputs[key] ?? '').trim()
    if (!value) return
    sendPanelInput(extName, panelId, value)
    inputs[key] = ''
  }

  function uninstall(name: string) {
    if (!confirm(`Uninstall "${name}"? This deletes it from ~/.bish/extensions and cannot be undone.`)) return
    uninstallExtension(name)
  }
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
            <div class="ext-disabled">Disabled</div>
          {:else if !ext.panels || ext.panels.length === 0}
            <div class="ext-disabled">No panels contributed</div>
          {:else}
            {#each ext.panels as p (p.id)}
              <div class="ext-panel-title">{p.title}</div>
              <div class="ext-panel-body">
                {@html $extensionPanelHTML.get(`${ext.name}:${p.id}`) ?? '<span class="ext-waiting">…</span>'}
              </div>
              <input
                class="ext-panel-input"
                type="text"
                placeholder="Type and press enter…"
                bind:value={inputs[`${ext.name}:${p.id}`]}
                onkeydown={(e) => e.key === 'Enter' && submit(ext.name, p.id)}
              />
            {/each}
          {/if}
          <div class="ext-actions">
            <button
              class="ext-action-btn"
              title={ext.enabled ? 'Disable' : 'Enable'}
              onclick={() => setExtensionEnabled(ext.name, !ext.enabled)}
            >
              <IconPower size={13} color={ext.enabled ? 'var(--accent)' : 'var(--muted)'} />
            </button>
            <button class="ext-action-btn" title="Uninstall" onclick={() => uninstall(ext.name)}>
              <IconTrash size={13} />
            </button>
          </div>
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
  .ext-panel-input {
    width: 100%; box-sizing: border-box; margin-top: 6px;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 12px; padding: 6px 10px; outline: none;
  }
  .ext-panel-input:focus { border-color: var(--accent); }

  .ext-actions { display: flex; justify-content: flex-end; gap: 2px; margin-top: 8px; }
  /* same property values as FileTree.svelte's .hdr-btn (project convention
     for small icon buttons), just not in a panel header here */
  .ext-action-btn {
    display: flex; align-items: center; justify-content: center;
    background: none; border: none; color: var(--muted); cursor: pointer;
    padding: 3px 4px; border-radius: 3px; transition: color 0.1s, background 0.1s;
  }
  .ext-action-btn:hover { color: var(--foreground); background: var(--bg-hover); }
</style>
