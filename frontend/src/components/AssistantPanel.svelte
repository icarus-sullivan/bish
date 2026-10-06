<script lang="ts">
  // AI panel: switch between agent providers (Claude Code, Codex). Both
  // shells stay mounted, so a conversation in one keeps running — and can
  // keep asking for approvals — while you work in the other.
  import { IconLoader2 } from '@tabler/icons-svelte'
  import { claudeProvider, codexProvider, type Provider } from '../lib/claude/providers'
  import AgentPanel from './agent/AgentPanel.svelte'
  import CodexSetup from './codex/CodexSetup.svelte'

  const KEY = 'bish.ai.provider'
  const providers: Provider[] = [claudeProvider, codexProvider]
  let current = $state<'claude' | 'codex'>((() => {
    try { return localStorage.getItem(KEY) === 'codex' ? 'codex' : 'claude' } catch { return 'claude' }
  })())
  function show(id: 'claude' | 'codex') {
    current = id
    try { localStorage.setItem(KEY, id) } catch {}
  }

  // mounted lazily on first view so an unused provider costs nothing
  let mounted = $state<Record<string, boolean>>({})
  $effect(() => { if (!mounted[current]) mounted[current] = true })

  // per-provider activity, reported up by each shell for the switcher badges
  let activity = $state<Record<string, { busy: boolean; asks: number }>>({})
</script>

<div class="ai">
  <div class="switcher" role="tablist">
    {#each providers as p (p.id)}
      <button class="prov" class:active={current === p.id} role="tab" aria-selected={current === p.id} onclick={() => show(p.id)}>
        <span class="glyph">{p.glyph}</span>{p.name}
        {#if activity[p.id]?.asks}<span class="badge ask" title="Waiting for your approval">{activity[p.id].asks}</span>
        {:else if activity[p.id]?.busy && current !== p.id}<IconLoader2 size={10} class="spin" />{/if}
      </button>
    {/each}
  </div>
  <div class="host">
    {#each providers as p (p.id)}
      {#if mounted[p.id]}
        <div class="pane" style:display={current === p.id ? 'flex' : 'none'}>
          {#if p.id === 'codex'}
            <CodexSetup>
              <AgentPanel provider={p} visible={current === p.id} onShow={() => show(p.id)}
                          onActivity={(busy, asks) => activity[p.id] = { busy, asks }} />
            </CodexSetup>
          {:else}
            <AgentPanel provider={p} visible={current === p.id} onShow={() => show(p.id)}
                        onActivity={(busy, asks) => activity[p.id] = { busy, asks }} />
          {/if}
        </div>
      {/if}
    {/each}
  </div>
</div>

<style>
  .ai { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .switcher {
    display: flex; gap: 2px; padding: 4px 6px; flex-shrink: 0;
    background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .prov {
    display: flex; align-items: center; gap: 5px; background: none; border: none; border-radius: 4px;
    color: var(--muted); font-size: 11.5px; padding: 3px 9px; cursor: pointer; font-family: inherit;
    transition: color 0.1s, background 0.1s;
  }
  .prov:hover { color: var(--foreground); }
  .prov.active { color: var(--foreground); background: var(--bg-hover); }
  .glyph { color: var(--accent); font-family: "SF Mono", Menlo, monospace; font-size: 10.5px; }
  .badge { font-size: 9.5px; border-radius: 7px; padding: 0 5px; background: var(--warning); color: var(--background); font-weight: 700; }
  .host { flex: 1; min-height: 0; position: relative; }
  .pane { position: absolute; inset: 0; flex-direction: column; }
</style>
