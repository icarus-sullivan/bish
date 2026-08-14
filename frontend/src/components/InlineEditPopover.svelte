<script lang="ts">
  import { onMount } from 'svelte'
  import { IconCheck, IconX } from '@tabler/icons-svelte'
  import { registerKeybind } from '../lib/keybinds'
  import { requestInlineEdit } from '../lib/inlineEdit'

  let { x, y, path, selectedText, onAccept, onClose }: {
    x: number; y: number; path: string; selectedText: string
    onAccept: (newText: string) => void; onClose: () => void
  } = $props()

  let instruction = $state('')
  let phase = $state<'input' | 'loading' | 'diff' | 'error'>('input')
  let proposed = $state('')
  let errorMsg = $state('')
  let inputEl = $state<HTMLInputElement>()

  onMount(() => {
    inputEl?.focus()
    return registerKeybind({ combo: 'escape', handler: onClose })
  })

  async function submit() {
    const text = instruction.trim()
    if (!text || phase === 'loading') return
    phase = 'loading'
    try {
      proposed = await requestInlineEdit(path, selectedText, text)
      phase = 'diff'
    } catch (e: any) {
      errorMsg = String(e?.message ?? e)
      phase = 'error'
    }
  }

  function retry() {
    phase = 'input'
    setTimeout(() => inputEl?.focus())
  }
</script>

<svelte:window onclick={onClose} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="popover" style="left:{x}px; top:{y}px" onclick={(e) => e.stopPropagation()}>
  {#if phase === 'input' || phase === 'loading'}
    <input
      class="prompt"
      bind:this={inputEl}
      bind:value={instruction}
      placeholder="Describe the edit… (Enter to submit)"
      disabled={phase === 'loading'}
      onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit() } }}
    />
    {#if phase === 'loading'}
      <div class="loading"><span class="dot"></span><span class="dot"></span><span class="dot"></span> Thinking…</div>
    {/if}
  {:else if phase === 'diff'}
    <div class="diff-preview">
      <pre class="block del">{selectedText}</pre>
      <pre class="block add">{proposed}</pre>
    </div>
    <div class="actions">
      <button class="accept" onclick={() => onAccept(proposed)}><IconCheck size={13} /> Accept</button>
      <button class="reject" onclick={onClose}><IconX size={13} /> Reject</button>
      <button class="retry" onclick={retry}>Try again</button>
    </div>
  {:else if phase === 'error'}
    <div class="error">{errorMsg}</div>
    <div class="actions">
      <button class="retry" onclick={retry}>Try again</button>
      <button class="reject" onclick={onClose}><IconX size={13} /> Close</button>
    </div>
  {/if}
</div>

<style>
  .popover {
    position: fixed;
    z-index: 9999;
    width: 380px;
    max-width: calc(100vw - 24px);
    background: color-mix(in srgb, var(--background) 85%, var(--border) 15%);
    border: 1px solid var(--border-focused);
    border-radius: 8px;
    padding: 8px;
    box-shadow:
      0 4px 6px -1px rgba(0,0,0,0.3),
      0 10px 24px -4px rgba(0,0,0,0.4),
      0 0 0 0.5px rgba(255,255,255,0.04) inset;
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    font-size: 12px;
  }

  .prompt {
    width: 100%; box-sizing: border-box; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 5px; color: var(--foreground);
    font-size: 12px; padding: 6px 8px; outline: none;
  }
  .prompt:focus { border-color: var(--accent); }
  .prompt:disabled { opacity: 0.6; }

  .loading {
    display: flex; align-items: center; gap: 5px;
    font-size: 11px; color: var(--muted); padding: 6px 2px 0;
  }
  .loading .dot {
    width: 4px; height: 4px; border-radius: 50%; background: var(--muted);
    animation: pulse 1.1s ease-in-out infinite;
  }
  .loading .dot:nth-child(2) { animation-delay: 0.15s; }
  .loading .dot:nth-child(3) { animation-delay: 0.3s; }
  @keyframes pulse { 0%, 60%, 100% { opacity: 0.25; } 30% { opacity: 1; } }

  .diff-preview {
    display: flex; flex-direction: column; gap: 4px;
    max-height: 260px; overflow-y: auto; border-radius: 5px;
  }
  .block {
    margin: 0; padding: 5px 8px; font-family: "SF Mono", Menlo, monospace;
    font-size: 11px; line-height: 1.5; white-space: pre-wrap; word-break: break-word;
    border-radius: 4px;
  }
  .block.del { background: color-mix(in srgb, var(--error) 14%, transparent); text-decoration: line-through; opacity: 0.85; }
  .block.add { background: color-mix(in srgb, var(--success) 14%, transparent); }

  .error {
    font-size: 11px; color: var(--error); white-space: pre-wrap;
    font-family: "SF Mono", Menlo, monospace; padding: 4px 2px;
  }

  .actions { display: flex; gap: 6px; margin-top: 8px; }
  .actions button {
    display: flex; align-items: center; gap: 4px; font-size: 11px; border-radius: 4px;
    padding: 4px 8px; cursor: pointer; border: 1px solid var(--border); background: var(--bg-raised);
    color: var(--foreground);
  }
  .actions .accept { border-color: var(--success); color: var(--success); }
  .actions .reject { border-color: var(--error); color: var(--error); }
  .actions .retry { margin-left: auto; }
</style>
