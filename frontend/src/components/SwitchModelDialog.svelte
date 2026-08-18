<script lang="ts">
  import { IconCpu } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'

  let { current, onSelect, onClose }: { current: string; onSelect: (alias: string) => void; onClose: () => void } = $props()

  // Aliases the `claude` CLI itself resolves — same names work whether the
  // backing provider is the Anthropic API, Bedrock, or Vertex, so this list
  // never needs the actual (often account-specific) provider model ID.
  const MODELS = [
    { alias: 'default', label: 'Default', desc: "Recommended model for your plan" },
    { alias: 'opus', label: 'Opus', desc: 'Most capable' },
    { alias: 'sonnet', label: 'Sonnet', desc: 'Balanced speed and capability' },
    { alias: 'haiku', label: 'Haiku', desc: 'Fastest' },
    { alias: 'opusplan', label: 'Opus Plan', desc: 'Opus for planning, Sonnet for execution' },
    { alias: 'fable', label: 'Fable', desc: 'Most capable, exploratory' },
  ]
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="overlay" onclick={onClose}>
  <div class="panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={onClose} onclick={(e) => e.stopPropagation()}>
    <div class="header">
      <IconCpu size={13} />
      <span class="title">Switch Model</span>
      <button class="close" onclick={onClose} aria-label="Close">✕</button>
    </div>
    <div class="body">
      <p class="hint">These are the names the CLI itself resolves — the same aliases work regardless of provider (Anthropic API, Bedrock, Vertex).</p>
      {#each MODELS as m (m.alias)}
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <div class="row" class:active={m.alias === current} onclick={() => onSelect(m.alias)} role="option" aria-selected={m.alias === current} tabindex="0">
          <div class="row-text">
            <span class="row-label">{m.label}</span>
            <span class="row-desc">{m.desc}</span>
          </div>
          <span class="row-alias">/model {m.alias}</span>
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed; inset: 0; z-index: 9000;
    background: rgba(0,0,0,0.45);
    display: flex; align-items: center; justify-content: center;
  }
  .panel {
    width: 420px; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
    display: flex; flex-direction: column; overflow: hidden;
  }
  .header {
    display: flex; align-items: center; gap: 6px;
    padding: 10px 14px 8px; border-bottom: 1px solid var(--border);
    color: var(--muted);
  }
  .title { font-size: 12px; font-weight: 600; color: var(--muted); flex: 1; }
  .close {
    background: none; border: none; color: var(--muted);
    cursor: pointer; font-size: 13px; padding: 2px 5px; border-radius: 3px;
  }
  .close:hover { color: var(--foreground); background: var(--bg-hover); }
  .body { padding: 10px 14px 14px; display: flex; flex-direction: column; gap: 6px; }
  .hint { font-size: 11px; color: var(--muted); margin: 0 0 4px; }

  .row {
    display: flex; align-items: center; justify-content: space-between; gap: 10px;
    padding: 7px 10px; border: 1px solid var(--border); border-radius: 6px;
    background: var(--background); cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  .row:hover { background: var(--bg-hover); border-color: var(--accent); }
  .row.active { border-color: var(--accent); }
  .row-text { display: flex; flex-direction: column; gap: 1px; }
  .row-label { font-size: 12px; color: var(--foreground); font-weight: 600; }
  .row-desc { font-size: 11px; color: var(--muted); }
  .row-alias { font-size: 10px; color: var(--muted); font-family: "SF Mono", Menlo, monospace; white-space: nowrap; }
</style>
