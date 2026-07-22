<script lang="ts">
  import { onMount } from 'svelte'
  import { GetConfig, SaveConfig } from '../lib/wails'
  import { currentThemeName, persistPrefs, formatOnSave } from '../lib/stores'
  import { features, FEATURES } from '../lib/features'
  import { customKeybinds, applyCustomKeybinds } from '../lib/keymap'
  import { listCommands } from '../lib/commands'
  import { themes } from '../lib/themes'
  import { get } from 'svelte/store'
  import { IconChevronDown } from '@tabler/icons-svelte'

  const editorFeatures = FEATURES.filter(f => f.section === 'editor')
  const terminalFeatures = FEATURES.filter(f => f.section === 'terminal')
  const commandList = listCommands()

  function onKeybind(id: string, e: Event) {
    const combo = (e.target as HTMLInputElement).value
    customKeybinds.update(m => ({ ...m, [id]: combo }))
    applyCustomKeybinds()
  }

  function onFeature(id: string, e: Event) {
    const checked = (e.target as HTMLInputElement).checked
    features.update(f => ({ ...f, [id]: checked }))
    saveCfg({ features: get(features) })
  }

  let cfg: any = $state(null)

  onMount(async () => {
    cfg = await GetConfig().catch(() => null)
  })

  async function saveCfg(patch: Record<string, any>) {
    if (!cfg) return
    cfg = { ...cfg, ...patch }
    await SaveConfig(cfg).catch(() => {})
  }

  function onTheme(e: Event) {
    const name = (e.target as HTMLSelectElement).value
    currentThemeName.set(name)
    saveCfg({ theme: name })
  }

  function onPersist(key: keyof import('../lib/stores').PersistPrefs, e: Event) {
    const checked = (e.target as HTMLInputElement).checked
    persistPrefs.update(p => ({ ...p, [key]: checked }))
    saveCfg({ persist: get(persistPrefs) })
  }

  const persistItems: { key: keyof import('../lib/stores').PersistPrefs; label: string; hint: string }[] = [
    { key: 'panel_width',   label: 'Panel width',        hint: 'Remember the right sidebar width per project' },
    { key: 'right_sidebar', label: 'Sidebar visibility', hint: 'Remember whether the right sidebar is open' },
    { key: 'right_panel',   label: 'Active panel',       hint: 'Remember which sidebar panel was selected' },
    { key: 'tabs',          label: 'Open tabs',          hint: 'Save open file tabs and reopen them next time' },
  ]
</script>

<div class="settings">
  <div class="inner">
    <h1>Settings</h1>

    <section>
      <h2>Appearance</h2>
      <div class="row">
        <div class="labels">
          <span class="label">Theme</span>
          <span class="hint">Color theme for the whole app</span>
        </div>
        <span class="select-wrap">
          <select value={$currentThemeName} onchange={onTheme}>
            {#each themes as t}
              <option value={t.value}>{t.label}</option>
            {/each}
          </select>
          <IconChevronDown size={13} class="select-chevron" />
        </span>
      </div>
    </section>

    <section>
      <h2>Editor</h2>
      <div class="row">
        <div class="labels">
          <span class="label">Format on save</span>
          <span class="hint">Format via the language server before writing (needs LSP installed)</span>
        </div>
        <input type="checkbox" checked={$formatOnSave}
               onchange={(e) => { formatOnSave.set((e.target as HTMLInputElement).checked); saveCfg({ format_on_save: (e.target as HTMLInputElement).checked }) }} />
      </div>
      {#each editorFeatures as f}
        <div class="row">
          <div class="labels">
            <span class="label">{f.label}</span>
            <span class="hint">{f.hint}</span>
          </div>
          <input type="checkbox" checked={$features[f.id]} onchange={(e) => onFeature(f.id, e)} />
        </div>
      {/each}
    </section>

    <section>
      <h2>Terminal</h2>
      {#each terminalFeatures as f}
        <div class="row">
          <div class="labels">
            <span class="label">{f.label}</span>
            <span class="hint">{f.hint}</span>
          </div>
          <input type="checkbox" checked={$features[f.id]} onchange={(e) => onFeature(f.id, e)} />
        </div>
      {/each}
    </section>

    <section>
      <h2>Keyboard</h2>
      <p class="section-hint">Assign a combo to any command, e.g. <code>mod+shift+k</code> (mod = ⌘/Ctrl). Blank = unbound.</p>
      {#each commandList as c}
        <div class="row">
          <div class="labels">
            <span class="label">{c.title}</span>
            <span class="hint">{c.id}</span>
          </div>
          <input class="kb-input" placeholder="unbound" value={$customKeybinds[c.id] ?? ''}
                 autocapitalize="none" autocorrect="off" spellcheck="false"
                 onchange={(e) => onKeybind(c.id, e)} />
        </div>
      {/each}
    </section>

    <section>
      <h2>Session</h2>
      <p class="section-hint">What bish remembers per project (stored in ~/.config/bish)</p>
      {#each persistItems as item}
        <div class="row">
          <div class="labels">
            <span class="label">{item.label}</span>
            <span class="hint">{item.hint}</span>
          </div>
          <input type="checkbox" checked={$persistPrefs[item.key]}
                 onchange={(e) => onPersist(item.key, e)} />
        </div>
      {/each}
    </section>
  </div>
</div>

<style>
  .settings {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    background: var(--background);
  }
  .inner {
    max-width: 560px;
    margin: 0 auto;
    padding: 32px 24px;
  }
  h1 {
    font-size: 18px;
    font-weight: 600;
    margin: 0 0 24px;
    color: var(--foreground);
  }
  h2 {
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--muted);
    margin: 0 0 4px;
  }
  section { margin-bottom: 28px; }
  .section-hint {
    font-size: 11px;
    color: var(--muted);
    margin: 0 0 8px;
  }
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    padding: 10px 0;
    border-bottom: 1px solid var(--border);
  }
  .labels { display: flex; flex-direction: column; gap: 2px; }
  .label { font-size: 13px; color: var(--foreground); }
  .hint { font-size: 11px; color: var(--muted); }

  .select-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
  }
  select {
    appearance: none;
    -webkit-appearance: none;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 12px;
    padding: 6px 28px 6px 10px;
    outline: none;
    cursor: pointer;
    min-width: 140px;
    transition: border-color 0.1s, background 0.1s;
  }
  select:hover { background: var(--bg-hover); }
  select:focus { border-color: var(--accent); }
  option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) {
    position: absolute;
    right: 9px;
    color: var(--muted);
    pointer-events: none;
  }

  .kb-input {
    width: 140px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 12px;
    font-family: "SF Mono", Menlo, monospace;
    padding: 5px 8px;
    outline: none;
  }
  .kb-input:focus { border-color: var(--accent); }
  code {
    font-family: "SF Mono", Menlo, monospace;
    font-size: 11px;
    background: var(--bg-raised);
    padding: 1px 4px;
    border-radius: 3px;
  }

  input[type='checkbox'] {
    width: 15px;
    height: 15px;
    accent-color: var(--accent);
    cursor: pointer;
    flex-shrink: 0;
  }
</style>
