<script lang="ts">
  import { IconCode, IconCheck, IconDownload, IconChevronRight, IconChevronDown as IconExpand } from '@tabler/icons-svelte'
  import { languageExtensions, loadLanguageExtensions, loadLanguage } from '../lib/languageExtensions'
  import { installServer } from '../lib/lsp'
  import { installFormatter } from '../lib/formatterInstall'
  import DefaultLanguageSettings from './DefaultLanguageSettings.svelte'
  import { onMount } from 'svelte'

  let expanded: string | null = $state(null)

  onMount(() => { loadLanguageExtensions() })

  function toggle(id: string) {
    expanded = expanded === id ? null : id
  }
</script>

<div class="panel">
  <div class="header">
    <span class="header-label">Languages</span>
    {#if $languageExtensions.length}<span class="count">{$languageExtensions.length}</span>{/if}
  </div>

  <div class="list">
    {#each $languageExtensions as def (def.id)}
      <div class="lang">
        <button class="lang-row" onclick={() => toggle(def.id)}>
          {#if expanded === def.id}<IconExpand size={13} />{:else}<IconChevronRight size={13} />{/if}
          <span class="lang-name">{def.name}</span>
          <span class="pills">
            {#if def.server}
              <span class="pill" class:ok={def.serverInstalled}>
                {#if def.serverInstalled}<IconCheck size={10} />{/if} server
              </span>
            {/if}
            {#if def.formatter}
              <span class="pill" class:ok={def.formatterInstalled}>
                {#if def.formatterInstalled}<IconCheck size={10} />{/if} formatter
              </span>
            {:else if def.builtinFormatter}
              <span class="pill ok"><IconCheck size={10} /> built-in</span>
            {/if}
          </span>
        </button>

        {#if expanded === def.id}
          <div class="lang-detail">
            <div class="quick-actions">
              {#if def.server && !def.serverInstalled}
                <button class="action-btn" onclick={() => installServer(def.id)}>
                  <IconDownload size={12} /> Install server
                </button>
              {/if}
              {#if def.formatter && !def.formatterInstalled}
                <button class="action-btn" onclick={() => installFormatter(def.id, def.name)}>
                  <IconDownload size={12} /> Install formatter
                </button>
              {/if}
            </div>
            {#await loadLanguage(def.id) then mod}
              {#if mod.Settings}
                <mod.Settings id={def.id} {def} />
              {:else}
                <DefaultLanguageSettings id={def.id} {def} />
              {/if}
            {/await}
          </div>
        {/if}
      </div>
    {:else}
      <div class="empty">
        <IconCode size={20} />
        <p>Loading languages…</p>
      </div>
    {/each}
  </div>
</div>

<style>
  .panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
  .header {
    display: flex; align-items: center; gap: 8px; padding: 0 12px; height: 32px;
    flex-shrink: 0; background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; color: var(--muted);
  }
  .count {
    font-size: 10px; color: var(--muted); background: var(--bg-hover);
    border-radius: 8px; padding: 1px 6px;
  }

  .list { overflow-y: auto; flex: 1; }
  .empty {
    display: flex; flex-direction: column; align-items: center; gap: 6px;
    color: var(--muted); font-size: 12px; padding: 32px 20px; text-align: center;
  }
  .empty p { margin: 0; }

  .lang { border-bottom: 1px solid var(--border); }
  .lang-row {
    display: flex; align-items: center; gap: 6px; width: 100%;
    background: none; border: none; color: var(--foreground); cursor: pointer;
    padding: 8px 12px; font-size: 12px; text-align: left;
  }
  .lang-row:hover { background: var(--bg-hover); }
  .lang-row :global(svg:first-child) { color: var(--muted); flex-shrink: 0; }
  .lang-name { flex: 1; font-weight: 600; }
  .pills { display: flex; gap: 4px; }
  .pill {
    display: flex; align-items: center; gap: 2px;
    font-size: 9px; color: var(--muted); background: var(--bg-hover);
    border-radius: 8px; padding: 1px 6px;
  }
  .pill.ok { color: var(--success); }

  .lang-detail { background: var(--bg-raised); border-top: 1px solid var(--border); }
  .quick-actions { display: flex; gap: 6px; padding: 8px 12px 0; }
  .action-btn {
    display: flex; align-items: center; gap: 4px;
    background: none; border: 1px solid var(--border); border-radius: 4px;
    color: var(--accent); font-size: 11px; padding: 4px 8px; cursor: pointer;
  }
  .action-btn:hover { background: var(--bg-hover); }
</style>
