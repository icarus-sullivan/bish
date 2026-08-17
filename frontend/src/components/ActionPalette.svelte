<script lang="ts">
  import { onMount } from 'svelte'
  import { listCommands, type Command } from '../lib/commands'
  import { fuzzyMatch } from '../lib/fuzzy'
  import { customKeybinds } from '../lib/keymap'
  import { formatCombo } from '../lib/keybinds'
  import { featureOn } from '../lib/features'

  // user override (Settings > Keybindings) wins over the command's own
  // manifest-declared default, same precedence keymap.ts's applyCustomKeybinds uses
  function comboFor(cmd: Command): string {
    const custom = featureOn('customKeybinds') ? $customKeybinds[cmd.id] : undefined
    const combo = custom || cmd.key
    return combo ? formatCombo(combo) : ''
  }

  let { onClose }: { onClose: () => void } = $props()

  let query = $state('')
  let selectedIdx = $state(0)
  let inputEl: HTMLInputElement
  const all = listCommands()

  const results = $derived.by<Command[]>(() => {
    const q = query.trim()
    if (!q) return all
    return all
      .map(c => ({ c, m: fuzzyMatch(q, c.title) }))
      .filter(r => r.m)
      .sort((a, b) => b.m!.score - a.m!.score)
      .map(r => r.c)
  })

  $effect(() => { results; selectedIdx = 0 })

  function run(cmd: Command) { onClose(); cmd.run() }

  function handleKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { onClose(); return }
    if (e.key === 'ArrowDown') { e.preventDefault(); selectedIdx = Math.min(selectedIdx + 1, results.length - 1) }
    if (e.key === 'ArrowUp')   { e.preventDefault(); selectedIdx = Math.max(selectedIdx - 1, 0) }
    if (e.key === 'Enter' && results.length > 0) run(results[selectedIdx])
  }

  onMount(() => inputEl?.focus())
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}>
  <div class="palette" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
    <div class="search-row">
      <span class="prompt">›</span>
      <input
        bind:this={inputEl}
        bind:value={query}
        onkeydown={handleKey}
        class="search-input"
        placeholder="Run a command…"
        autocomplete="off"
        spellcheck="false"
      />
    </div>
    {#if results.length > 0}
      <div class="results">
        {#each results as r, i (r.id)}
          <div
            class="result-row"
            class:active={i === selectedIdx}
            onclick={() => run(r)}
            onmouseenter={() => selectedIdx = i}
            role="option"
            aria-selected={i === selectedIdx}
            tabindex="-1"
          >
            <span class="cmd-title">{r.title}</span>
            {#if comboFor(r)}<span class="cmd-combo">{comboFor(r)}</span>{/if}
          </div>
        {/each}
      </div>
    {:else}
      <div class="empty">No matching commands</div>
    {/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed; inset: 0; z-index: 500;
    background: color-mix(in srgb, #000 50%, transparent);
    display: flex; align-items: flex-start; justify-content: center; padding-top: 80px;
  }
  .palette {
    width: 560px; max-height: 460px; background: var(--bg-raised);
    border: 1px solid var(--border-focused); border-radius: 8px; overflow: hidden;
    display: flex; flex-direction: column;
    box-shadow: 0 24px 48px color-mix(in srgb, #000 60%, transparent);
  }
  .search-row {
    display: flex; align-items: center; gap: 8px; padding: 10px 14px;
    border-bottom: 1px solid var(--border); flex-shrink: 0;
  }
  .prompt { color: var(--accent); font-weight: 700; }
  .search-input {
    flex: 1; background: none; border: none; outline: none;
    color: var(--foreground); font-size: 14px; font-family: inherit;
  }
  .search-input::placeholder { color: var(--muted); }
  .results { overflow-y: auto; flex: 1; padding: 4px 0; }
  .result-row {
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
    padding: 7px 14px; cursor: pointer; border-radius: 4px; margin: 0 4px;
    font-size: 13px; color: var(--foreground);
  }
  .result-row.active { background: var(--bg-selected); }
  .result-row:hover { background: var(--bg-hover); }
  .cmd-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cmd-combo {
    flex-shrink: 0; color: var(--muted); font-family: "SF Mono", Menlo, monospace;
    font-size: 11px; letter-spacing: 0.02em;
  }
  .empty { padding: 20px 14px; font-size: 13px; color: var(--muted); text-align: center; }
</style>
