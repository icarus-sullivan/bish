<script lang="ts">
  import { showWelcomeTour } from '../lib/stores'
  import { GetConfig, SaveConfig } from '../lib/wails'
  import { IconSparkles, IconTerminal2, IconGitBranch, IconBug, IconFolders } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'

  const highlights = [
    { icon: IconSparkles, title: 'AI Assistant', text: 'A chat panel that drives a real coding agent against this project — plan mode, diffs, approve/reject.' },
    { icon: IconTerminal2, title: 'Terminal-first', text: 'The terminal is a first-class panel, not a drawer — saved commands, live-shareable, command blocks.' },
    { icon: IconFolders, title: 'Multi-root & remote', text: 'Add extra workspace folders, or open a project over SSH, from the same window.' },
    { icon: IconBug, title: 'Debugger & Git', text: 'Breakpoints and a real debug session for Go, plus a Git panel with staging, diffs, and conflict resolution.' },
  ]

  async function dismiss() {
    showWelcomeTour.set(false)
    const cfg: any = await GetConfig().catch(() => null)
    if (cfg) await SaveConfig({ ...cfg, onboarding_seen: true }).catch(() => {})
  }
</script>

{#if $showWelcomeTour}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="overlay" onclick={dismiss}>
    <div class="panel" role="dialog" aria-modal="true" aria-label="Welcome to bish" tabindex="-1" use:modalA11y={dismiss} onclick={(e) => e.stopPropagation()}>
      <div class="header">
        <span class="title">Welcome to bish</span>
        <span class="sub">A shell-first IDE — terminal and editor as equals.</span>
      </div>
      <div class="grid">
        {#each highlights as h}
          <div class="card">
            <h.icon size={18} />
            <div class="card-title">{h.title}</div>
            <div class="card-text">{h.text}</div>
          </div>
        {/each}
      </div>
      <div class="footer">
        <button class="btn-connect" onclick={dismiss}>Got it</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed; inset: 0; z-index: 9500;
    background: rgba(0,0,0,0.5);
    display: flex; align-items: center; justify-content: center;
  }
  .panel {
    width: 520px; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 12px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
    display: flex; flex-direction: column; overflow: hidden;
  }
  .header { padding: 20px 22px 12px; }
  .title { display: block; font-size: 16px; font-weight: 700; color: var(--foreground); }
  .sub { display: block; font-size: 12px; color: var(--muted); margin-top: 4px; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 4px 22px 20px; }
  .card { background: var(--background); border: 1px solid var(--border); border-radius: 8px; padding: 12px; color: var(--accent); }
  .card-title { font-size: 12px; font-weight: 700; color: var(--foreground); margin-top: 6px; }
  .card-text { font-size: 11px; color: var(--muted); margin-top: 4px; line-height: 1.5; }
  .footer { display: flex; justify-content: flex-end; padding: 0 22px 18px; }
  .btn-connect {
    background: var(--accent); border: none; border-radius: 6px;
    color: #000; font-size: 12px; font-weight: 600; padding: 7px 18px; cursor: pointer;
  }
  .btn-connect:hover { opacity: 0.85; }
</style>
