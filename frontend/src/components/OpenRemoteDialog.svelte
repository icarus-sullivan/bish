<script lang="ts">
  import { showOpenRemote } from '../lib/stores'
  import { OpenRemoteProject } from '../lib/wails'
  import { IconServer } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'

  let host = $state('')
  let path = $state('')
  let connecting = $state(false)
  let error = $state('')

  function close() {
    showOpenRemote.set(false)
    host = ''; path = ''; error = ''
  }

  async function connect() {
    if (!host.trim() || !path.trim() || connecting) return
    connecting = true
    error = ''
    try {
      await OpenRemoteProject(host.trim(), path.trim())
      close()
    } catch (e: any) {
      error = String(e?.message ?? e)
    } finally {
      connecting = false
    }
  }

</script>

{#if $showOpenRemote}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="overlay" onclick={close}>
    <div class="panel" role="dialog" aria-modal="true" tabindex="-1" use:modalA11y={close} onclick={(e) => e.stopPropagation()}>
      <div class="header">
        <IconServer size={13} />
        <span class="title">Open Remote Folder</span>
        <button class="close" onclick={close} aria-label="Close">✕</button>
      </div>
      <div class="body">
        <label class="field">
          <span class="label">Host</span>
          <input
            class="input" bind:value={host}
            placeholder="user@host, or an SSH config alias"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
            disabled={connecting}
            onkeydown={(e) => { if (e.key === 'Enter') connect() }}
          />
        </label>
        <label class="field">
          <span class="label">Remote path</span>
          <input
            class="input" bind:value={path}
            placeholder="/home/user/project"
            autocapitalize="none" autocorrect="off" autocomplete="off" spellcheck="false"
            disabled={connecting}
            onkeydown={(e) => { if (e.key === 'Enter') connect() }}
          />
        </label>
        <p class="hint">Uses your local <code>ssh</code> config, keys, and agent — same as running <code>ssh {host || 'host'}</code> in a terminal.</p>
        {#if error}<div class="error">{error}</div>{/if}
      </div>
      <div class="footer">
        <button class="btn-cancel" onclick={close}>Cancel</button>
        <button class="btn-connect" disabled={connecting || !host.trim() || !path.trim()} onclick={connect}>
          {connecting ? 'Connecting…' : 'Connect'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed; inset: 0; z-index: 9000;
    background: rgba(0,0,0,0.45);
    display: flex; align-items: center; justify-content: center;
  }
  .panel {
    width: 400px; background: var(--bg-raised);
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
  .body { padding: 12px 14px; display: flex; flex-direction: column; gap: 10px; }
  .field { display: flex; flex-direction: column; gap: 4px; }
  .label { font-size: 11px; color: var(--muted); }
  .input {
    background: var(--background); border: 1px solid var(--border);
    border-radius: 5px; color: var(--foreground); font-size: 12px;
    padding: 6px 8px; outline: none; font-family: "SF Mono", Menlo, monospace;
  }
  .input:focus { border-color: var(--accent); }
  .input:disabled { opacity: 0.6; }
  .hint { font-size: 11px; color: var(--muted); margin: 0; line-height: 1.5; }
  .hint code { font-family: "SF Mono", Menlo, monospace; color: var(--foreground); }
  .error { font-size: 11px; color: var(--error); white-space: pre-wrap; }
  .footer {
    display: flex; justify-content: flex-end; gap: 8px;
    padding: 8px 14px 12px; border-top: 1px solid var(--border);
  }
  .btn-cancel {
    background: none; border: 1px solid var(--border); border-radius: 5px;
    color: var(--muted); font-size: 11px; padding: 5px 12px; cursor: pointer;
  }
  .btn-cancel:hover { color: var(--foreground); background: var(--bg-hover); }
  .btn-connect {
    background: var(--accent); border: none; border-radius: 5px;
    color: #000; font-size: 11px; font-weight: 600; padding: 5px 14px; cursor: pointer;
  }
  .btn-connect:hover:not(:disabled) { opacity: 0.85; }
  .btn-connect:disabled { opacity: 0.5; cursor: default; }
</style>
