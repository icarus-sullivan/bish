<script lang="ts">
  import { loadedExtensions, extensionPanelHTML, sendPanelInput } from '../lib/extensions'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

  let { extName, panelId }: { extName: string; panelId: string } = $props()

  let input = $state('')

  const title = $derived(
    $loadedExtensions.find(e => e.name === extName)?.panels?.find(p => p.id === panelId)?.title ?? extName
  )

  function submit() {
    const value = input.trim()
    if (!value) return
    sendPanelInput(extName, panelId, value)
    input = ''
  }

  function onBodyClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest('a')
    if (!a) return
    const href = a.getAttribute('href')
    if (href && /^https?:\/\//i.test(href)) { e.preventDefault(); BrowserOpenURL(href) }
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">{title}</span>
  </div>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="body" onclick={onBodyClick}>
    {@html $extensionPanelHTML.get(`${extName}:${panelId}`) ?? '<span class="ext-waiting">…</span>'}
  </div>
  <input
    class="ext-panel-input"
    type="text"
    placeholder="Type and press enter…"
    bind:value={input}
    onkeydown={(e) => e.key === 'Enter' && submit()}
  />
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
  .body { flex: 1; overflow-y: auto; font-size: 12px; color: var(--foreground); }
  .body :global(.ext-waiting) { color: var(--muted); padding: 8px 12px; display: inline-block; }
  .ext-panel-input {
    width: 100%; box-sizing: border-box; margin: 0; flex-shrink: 0;
    background: var(--bg-raised); border: none; border-top: 1px solid var(--border);
    color: var(--foreground); font-size: 12px; padding: 8px 12px; outline: none;
  }
  .ext-panel-input:focus { border-top-color: var(--accent); }
</style>
