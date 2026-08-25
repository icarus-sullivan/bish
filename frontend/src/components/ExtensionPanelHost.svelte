<script lang="ts">
  import { loadedExtensions, extensionPanelHTML, extensionPanelSelect, sendPanelInput, sendPanelSelect, runExtensionCommand } from '../lib/extensions'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { IconChevronDown, IconRefresh } from '@tabler/icons-svelte'

  let { extName, panelId }: { extName: string; panelId: string } = $props()

  let input = $state('')

  const title = $derived(
    $loadedExtensions.find(e => e.name === extName)?.panels?.find(p => p.id === panelId)?.title ?? extName
  )

  // Present only if the extension's worker has called setPanelSelect for
  // this panel — most extensions never do, and get a plain text input.
  const select = $derived($extensionPanelSelect.get(`${extName}:${panelId}`))

  // Refresh icon reuses the extension's own manifest-declared "refresh"
  // command — same message a Command Palette invocation would send — so it
  // only appears for extensions that actually contribute one.
  const hasRefresh = $derived(
    !!$loadedExtensions.find(e => e.name === extName)?.commands?.some(c => c.id === 'refresh')
  )

  function submit() {
    sendPanelInput(extName, panelId, input.trim())
    input = ''
  }

  function onSelectChange(e: Event) {
    sendPanelSelect(extName, panelId, (e.target as HTMLSelectElement).value)
  }

  function refresh() {
    runExtensionCommand(extName, 'refresh')
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
  <div class="footer">
    <input
      class="ext-panel-input"
      type="text"
      placeholder="Type and press enter…"
      bind:value={input}
      onkeydown={(e) => e.key === 'Enter' && submit()}
    />
    {#if select}
      <span class="select-wrap">
        <select value={select.value} onchange={onSelectChange}>
          {#each select.options as o}<option value={o.value}>{o.label}</option>{/each}
        </select>
        <IconChevronDown size={13} class="select-chevron" />
      </span>
    {/if}
    {#if hasRefresh}
      <button class="hdr-btn" onclick={refresh} title="Refresh">
        <IconRefresh size={13} />
      </button>
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
  .body { flex: 1; overflow-y: auto; font-size: 12px; color: var(--foreground); }
  .body :global(.ext-waiting) { color: var(--muted); padding: 8px 12px; display: inline-block; }
  .footer {
    display: flex; align-items: center; gap: 6px; flex-shrink: 0;
    background: var(--bg-raised); border-top: 1px solid var(--border); padding: 0 6px;
  }
  .ext-panel-input {
    flex: 1; min-width: 0; width: auto; box-sizing: border-box; margin: 0;
    background: none; border: none;
    color: var(--foreground); font-size: 12px; padding: 8px 6px; outline: none;
  }
  .footer:focus-within { border-top-color: var(--accent); }
  .select-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
  }
  .select-wrap select {
    appearance: none;
    -webkit-appearance: none;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 11px;
    padding: 4px 22px 4px 8px;
    outline: none;
    cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  .select-wrap select:hover { background: var(--bg-hover); }
  .select-wrap select:focus { border-color: var(--accent); }
  .select-wrap option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) {
    position: absolute;
    right: 7px;
    color: var(--muted);
    pointer-events: none;
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
    flex-shrink: 0;
    transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }
</style>
