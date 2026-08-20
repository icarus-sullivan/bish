<script lang="ts">
  // Demonstrates a language owning custom settings UI beyond
  // DefaultLanguageSettings' generic fields — Python's dedicated formatter
  // has two real candidates (ruff, black) worth picking explicitly rather
  // than relying on PATH-lookup order.
  import { GetLanguageOverride, SetLanguageOverride } from '../lib/wails'
  import type { LanguageExtensionDTO, LanguageOverride } from '../lib/wails'
  import { loadLanguageExtensions } from '../lib/languageExtensions'
  import DefaultLanguageSettings from './DefaultLanguageSettings.svelte'

  let { id, def }: { id: string; def: LanguageExtensionDTO } = $props()

  let ov: LanguageOverride = $state({})
  $effect(() => { GetLanguageOverride(id).then(v => { ov = v ?? {} }) })

  type Choice = 'auto' | 'ruff' | 'black'
  function choice(): Choice {
    if (ov.formatter_path === 'ruff') return 'ruff'
    if (ov.formatter_path === 'black') return 'black'
    return 'auto'
  }
  async function pick(c: Choice) {
    if (c === 'auto') {
      ov.formatter_path = undefined
      ov.formatter_args = undefined
    } else if (c === 'ruff') {
      ov.formatter_path = 'ruff'
      ov.formatter_args = ['format', '--stdin-filename', '{path}', '-']
    } else {
      ov.formatter_path = 'black'
      ov.formatter_args = ['-q', '-']
    }
    await SetLanguageOverride(id, ov)
    await loadLanguageExtensions()
  }
</script>

<div class="picker">
  <div class="picker-title">Formatter</div>
  <div class="options">
    <button class:active={choice() === 'auto'} onclick={() => pick('auto')}>Auto</button>
    <button class:active={choice() === 'ruff'} onclick={() => pick('ruff')}>ruff</button>
    <button class:active={choice() === 'black'} onclick={() => pick('black')}>black</button>
  </div>
</div>

<DefaultLanguageSettings {id} {def} />

<style>
  .picker { padding: 10px 12px 0; font-size: 12px; }
  .picker-title {
    font-size: 10px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--muted); margin-bottom: 6px;
  }
  .options { display: flex; gap: 4px; }
  .options button {
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 4px;
    color: var(--foreground); font-size: 11px; padding: 4px 10px; cursor: pointer;
  }
  .options button:hover { background: var(--bg-hover); }
  .options button.active { border-color: var(--accent); color: var(--accent); }
</style>
