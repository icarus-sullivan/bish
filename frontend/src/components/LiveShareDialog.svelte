<script lang="ts">
  import { liveShares, startShare, stopShare, setGuestCanType } from '../lib/liveshare'
  import { IconCopy, IconCheck, IconUsers, IconServer2 } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'

  let { terminalId, onClose }: { terminalId: string; onClose: () => void } = $props()

  let loading = $state(true)
  let error = $state('')
  let copied = $state(false)

  $effect(() => {
    if ($liveShares.has(terminalId)) { loading = false; return }
    loading = true
    startShare(terminalId)
      .then(() => { loading = false })
      .catch((e: any) => { error = String(e?.message ?? e); loading = false })
  })

  // named `share`, not `state` — a local var literally named `state` collides
  // with the `$state` rune (svelte-check misreads `$state(...)` above as
  // store-auto-subscription of a `state` variable and errors)
  const share = $derived($liveShares.get(terminalId))

  function copyLink() {
    if (!share) return
    navigator.clipboard.writeText(share.url)
    copied = true
    setTimeout(() => { copied = false }, 1500)
  }

  function stop() {
    stopShare(terminalId)
    onClose()
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="overlay" onclick={onClose}>
  <div class="panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={onClose} onclick={(e) => e.stopPropagation()}>
    <div class="header">
      <IconServer2 size={13} />
      <span class="title">Share Terminal</span>
      <button class="close" onclick={onClose} aria-label="Close">✕</button>
    </div>
    <div class="body">
      {#if loading}
        <p class="hint">Starting…</p>
      {:else if error}
        <div class="error">{error}</div>
      {:else if share}
        <label class="field">
          <span class="label">Link — anyone on your local network can open this</span>
          <div class="link-row">
            <input class="link-input" readonly value={share.url} onclick={(e) => (e.target as HTMLInputElement).select()} />
            <button class="copy-btn" onclick={copyLink} title="Copy link">
              {#if copied}<IconCheck size={13} />{:else}<IconCopy size={13} />{/if}
            </button>
          </div>
        </label>

        <div class="guests">
          <div class="guests-label"><IconUsers size={12} /> Guests ({share.guests.length})</div>
          {#if share.guests.length === 0}
            <p class="hint">Nobody's connected yet.</p>
          {:else}
            {#each share.guests as g (g.id)}
              <div class="guest-row">
                <span class="guest-name">Guest {g.id}</span>
                <label class="toggle">
                  <input type="checkbox" checked={g.canType}
                         onchange={(e) => setGuestCanType(terminalId, g.id, (e.target as HTMLInputElement).checked)} />
                  Can type
                </label>
              </div>
            {/each}
          {/if}
        </div>
        <p class="hint">New guests start read-only — check "Can type" to hand them the keyboard.</p>
      {/if}
    </div>
    <div class="footer">
      <button class="btn-stop" onclick={stop} disabled={loading}>Stop Sharing</button>
      <button class="btn-close" onclick={onClose}>Done</button>
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
  .body { padding: 12px 14px; display: flex; flex-direction: column; gap: 12px; }
  .field { display: flex; flex-direction: column; gap: 4px; }
  .label { font-size: 11px; color: var(--muted); }
  .hint { font-size: 11px; color: var(--muted); margin: 0; }
  .error { font-size: 11px; color: var(--error); white-space: pre-wrap; }

  .link-row { display: flex; gap: 6px; }
  .link-input {
    flex: 1; background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 12px;
    padding: 6px 8px; outline: none; font-family: "SF Mono", Menlo, monospace;
  }
  .link-input:focus { border-color: var(--accent); }
  .copy-btn {
    display: flex; align-items: center; justify-content: center;
    background: var(--bg-hover); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); cursor: pointer; padding: 0 10px;
  }
  .copy-btn:hover { border-color: var(--accent); }

  .guests { display: flex; flex-direction: column; gap: 4px; }
  .guests-label {
    display: flex; align-items: center; gap: 5px;
    font-size: 10px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--muted);
  }
  .guest-row {
    display: flex; align-items: center; justify-content: space-between;
    padding: 5px 8px; background: var(--background); border: 1px solid var(--border); border-radius: 5px;
    font-size: 12px;
  }
  .guest-name { font-family: "SF Mono", Menlo, monospace; }
  .toggle { display: flex; align-items: center; gap: 5px; font-size: 11px; color: var(--muted); cursor: pointer; }
  .toggle input { cursor: pointer; }

  .footer {
    display: flex; justify-content: space-between; gap: 8px;
    padding: 8px 14px 12px; border-top: 1px solid var(--border);
  }
  .btn-stop {
    background: none; border: 1px solid var(--error); border-radius: 5px;
    color: var(--error); font-size: 11px; padding: 5px 12px; cursor: pointer;
  }
  .btn-stop:hover:not(:disabled) { background: color-mix(in srgb, var(--error) 12%, transparent); }
  .btn-stop:disabled { opacity: 0.5; cursor: default; }
  .btn-close {
    background: var(--accent); border: none; border-radius: 5px;
    color: #000; font-size: 11px; font-weight: 600; padding: 5px 14px; cursor: pointer;
  }
  .btn-close:hover { opacity: 0.85; }
</style>
