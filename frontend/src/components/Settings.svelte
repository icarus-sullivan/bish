<script lang="ts">
  import { onMount } from 'svelte'
  import { GetConfig, SaveConfig, OllamaListModels } from '../lib/wails'
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
  let ollamaModels = $state<{ Name: string; Size: number; Family: string; ParamSize: string; Quant: string }[]>([])
  let ollamaModelsError = $state('')

  onMount(async () => {
    cfg = await GetConfig().catch(() => null)
    if (cfg?.assistant?.provider === 'ollama') loadOllamaModels()
  })

  async function loadOllamaModels() {
    const url = cfg?.assistant?.ollama_url
    if (!url) { ollamaModels = []; ollamaModelsError = ''; return }
    try {
      ollamaModels = await OllamaListModels(url)
      ollamaModelsError = ''
    } catch (e) {
      ollamaModels = []
      ollamaModelsError = `${e}`
    }
  }

  function formatModelSize(bytes: number): string {
    const gb = bytes / 1024 ** 3
    return gb >= 1 ? `${gb.toFixed(1)} GB` : `${(bytes / 1024 ** 2).toFixed(0)} MB`
  }

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

  function onProvider(e: Event) {
    const provider = (e.target as HTMLSelectElement).value
    saveCfg({ assistant: { ...(cfg.assistant ?? {}), provider } })
    if (provider === 'ollama') loadOllamaModels()
  }

  function onOllamaField(key: 'ollama_url' | 'ollama_model', e: Event) {
    const value = (e.target as HTMLInputElement | HTMLSelectElement).value
    saveCfg({ assistant: { ...(cfg.assistant ?? {}), [key]: value } })
    if (key === 'ollama_url') loadOllamaModels()
  }

  function onCompletionField(key: 'enabled' | 'model_path' | 'server_path', e: Event) {
    const target = e.target as HTMLInputElement
    const value = key === 'enabled' ? target.checked : target.value
    saveCfg({ completion: { ...(cfg.completion ?? {}), [key]: value } })
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
      <h2>Assistant</h2>
      <div class="row">
        <div class="labels">
          <span class="label">Provider</span>
          <span class="hint">Backend the AI Assistant panel talks to</span>
        </div>
        <span class="select-wrap">
          <select value={cfg?.assistant?.provider ?? 'claude'} onchange={onProvider}>
            <option value="claude">Claude CLI</option>
            <option value="ollama">Ollama (local model)</option>
          </select>
          <IconChevronDown size={13} class="select-chevron" />
        </span>
      </div>
      {#if (cfg?.assistant?.provider ?? 'claude') === 'ollama'}
        <div class="row">
          <div class="labels">
            <span class="label">Ollama base URL</span>
            <span class="hint">e.g. http://192.168.1.20:11434</span>
          </div>
          <input class="kb-input wide-input" placeholder="http://localhost:11434"
                 value={cfg?.assistant?.ollama_url ?? ''}
                 onchange={(e) => onOllamaField('ollama_url', e)} />
        </div>
        <div class="row">
          <div class="labels">
            <span class="label">Ollama model</span>
            <span class="hint">
              {#if ollamaModelsError}Couldn't reach Ollama at that URL
              {:else}Must support tool calling{/if}
            </span>
          </div>
          <span class="select-wrap wide">
            <select value={cfg?.assistant?.ollama_model ?? ''} onchange={(e) => onOllamaField('ollama_model', e)}>
              <option value="" disabled>{ollamaModels.length ? 'Select a model…' : 'No models found'}</option>
              {#if cfg?.assistant?.ollama_model && !ollamaModels.some(m => m.Name === cfg.assistant.ollama_model)}
                <option value={cfg.assistant.ollama_model}>{cfg.assistant.ollama_model}</option>
              {/if}
              {#each ollamaModels as m}
                <option value={m.Name}>{m.Name}</option>
              {/each}
            </select>
            <IconChevronDown size={13} class="select-chevron" />
          </span>
        </div>
        {#if cfg?.assistant?.ollama_model}
          {@const selected = ollamaModels.find(m => m.Name === cfg.assistant.ollama_model)}
          {#if selected}
            <div class="badge-row">
              {#if selected.ParamSize}<span class="badge">{selected.ParamSize}</span>{/if}
              {#if selected.Quant}<span class="badge">{selected.Quant}</span>{/if}
              {#if selected.Family}<span class="badge">{selected.Family}</span>{/if}
              {#if selected.Size}<span class="badge">{formatModelSize(selected.Size)}</span>{/if}
            </div>
          {/if}
        {/if}
      {/if}
    </section>

    <section>
      <h2>Local code completion</h2>
      <div class="row">
        <div class="labels">
          <span class="label">Enable</span>
          <span class="hint">
            Spawns llama-server as a subprocess for inline suggestions in the autocomplete popup — separate
            from the Assistant panel above, always runs on this machine. Needs llama.cpp installed
            (e.g. `brew install llama.cpp`) and a GGUF model downloaded, e.g.
            `huggingface-cli download ggml-org/Qwen2.5-Coder-0.5B-Q8_0-GGUF qwen2.5-coder-0.5b-q8_0.gguf --local-dir ~/.local/share/bish/models`.
          </span>
        </div>
        <input type="checkbox" checked={cfg?.completion?.enabled ?? false}
               onchange={(e) => onCompletionField('enabled', e)} />
      </div>
      {#if cfg?.completion?.enabled}
        <div class="row">
          <div class="labels">
            <span class="label">Model path</span>
            <span class="hint">Path to a .gguf file, e.g. ~/.local/share/bish/models/qwen2.5-coder-0.5b-q8_0.gguf</span>
          </div>
          <input class="kb-input wide-input" placeholder="/path/to/qwen2.5-coder-0.5b-q8_0.gguf"
                 value={cfg?.completion?.model_path ?? ''}
                 onchange={(e) => onCompletionField('model_path', e)} />
        </div>
        <div class="row">
          <div class="labels">
            <span class="label">Server binary (optional)</span>
            <span class="hint">Override for the llama-server binary — leave blank to use the one on PATH</span>
          </div>
          <input class="kb-input wide-input" placeholder="llama-server"
                 value={cfg?.completion?.server_path ?? ''}
                 onchange={(e) => onCompletionField('server_path', e)} />
        </div>
      {/if}
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
  .select-wrap.wide select { min-width: 220px; }

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
  .kb-input.wide-input { width: 240px; }

  .badge-row {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 6px;
    padding: 10px 0;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 1px 7px;
    font-size: 10px;
    font-weight: 500;
    color: var(--muted);
    background: transparent;
    white-space: nowrap;
  }
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
