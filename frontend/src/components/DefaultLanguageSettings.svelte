<script lang="ts">
  import { IconChevronDown } from '@tabler/icons-svelte'
  import { GetLanguageOverride, SetLanguageOverride } from '../lib/wails'
  import type { LanguageExtensionDTO, LanguageOverride } from '../lib/wails'
  import { installFormatter } from '../lib/formatterInstall'
  import { loadLanguageExtensions } from '../lib/languageExtensions'

  let { id, def }: { id: string; def: LanguageExtensionDTO } = $props()

  let ov: LanguageOverride = $state({})
  let loaded = $state(false)

  $effect(() => {
    id
    loaded = false
    GetLanguageOverride(id).then(v => { ov = v ?? {}; loaded = true })
  })

  async function save() {
    await SetLanguageOverride(id, ov)
    await loadLanguageExtensions()
  }

  function formatOnSaveValue(): string {
    if (ov.format_on_save === true) return 'on'
    if (ov.format_on_save === false) return 'off'
    return 'default'
  }
  function setFormatOnSave(v: string) {
    ov.format_on_save = v === 'default' ? undefined : v === 'on'
    save()
  }
</script>

{#if loaded}
  <div class="settings">
    {#if def.server}
      <div class="group">
        <div class="group-title">Language server</div>
        <label class="row">
          <span>Custom binary path</span>
          <input type="text" placeholder="(auto-detect on PATH)" bind:value={ov.server_path} onchange={save} />
        </label>
        <label class="row check">
          <input type="checkbox" bind:checked={ov.disable_server} onchange={save} />
          <span>Disable</span>
        </label>
      </div>
    {/if}

    {#if def.formatter}
      <div class="group">
        <div class="group-title">
          Formatter
          {#if !def.formatterInstalled}
            <button class="install-btn" onclick={() => installFormatter(id, def.name)}>Install</button>
          {/if}
        </div>
        <label class="row">
          <span>Custom binary path</span>
          <input type="text" placeholder="(auto-detect on PATH)" bind:value={ov.formatter_path} onchange={save} />
        </label>
        <label class="row check">
          <input type="checkbox" bind:checked={ov.disable_formatter} onchange={save} />
          <span>Disable</span>
        </label>
      </div>
    {/if}

    <div class="group">
      <div class="group-title">Editor</div>
      <label class="row">
        <span>Format on save</span>
        <span class="select-wrap">
          <select value={formatOnSaveValue()} onchange={(e) => setFormatOnSave(e.currentTarget.value)}>
            <option value="default">Use global default</option>
            <option value="on">Always</option>
            <option value="off">Never</option>
          </select>
          <IconChevronDown size={13} class="select-chevron" />
        </span>
      </label>
      <label class="row">
        <span>Indent style</span>
        <span class="select-wrap">
          <select bind:value={ov.indent_style} onchange={save}>
            <option value="">Use file default</option>
            <option value="spaces">Spaces</option>
            <option value="tab">Tab</option>
          </select>
          <IconChevronDown size={13} class="select-chevron" />
        </span>
      </label>
    </div>
  </div>
{/if}

<style>
  .settings { padding: 10px 12px; font-size: 12px; }
  .group { margin-bottom: 14px; }
  .group-title {
    display: flex; align-items: center; gap: 8px;
    font-size: 10px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--muted); margin-bottom: 6px;
  }
  .row { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 4px 0; }
  .row.check { justify-content: flex-start; }
  .row span:first-child { color: var(--foreground); }
  input[type="text"] {
    width: 160px; background: var(--bg-raised); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 12px; padding: 5px 8px; outline: none;
  }
  input[type="text"]:focus { border-color: var(--accent); }
  input[type="checkbox"] { accent-color: var(--accent); }

  .install-btn {
    background: none; border: 1px solid var(--border); border-radius: 4px;
    color: var(--accent); font-size: 10px; padding: 2px 8px; cursor: pointer;
  }
  .install-btn:hover { background: var(--bg-hover); }

  .select-wrap { position: relative; display: inline-flex; align-items: center; }
  select {
    appearance: none; -webkit-appearance: none;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 12px; padding: 5px 24px 5px 8px; outline: none; cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  select:hover { background: var(--bg-hover); }
  select:focus { border-color: var(--accent); }
  option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) { position: absolute; right: 7px; color: var(--muted); pointer-events: none; }
</style>
