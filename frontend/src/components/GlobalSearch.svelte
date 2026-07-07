<script lang="ts">
  import { onMount } from 'svelte'
  import { showGlobalSearch, openFileTab, cwd, projectRoot } from '../lib/stores'
  import { SearchInFiles, ReplaceInFiles } from '../lib/wails'
  import { get } from 'svelte/store'
  import type { SearchResultDTO } from '../lib/wails'

  let query = $state('')
  let replaceText = $state('')
  let caseSensitive = $state(false)
  let wholeWord = $state(false)
  let useRegex = $state(false)
  let results = $state<SearchResultDTO[]>([])
  let searching = $state(false)
  let replacing = $state(false)
  let replaceCount = $state<number | null>(null)
  let searchError = $state('')
  let inputEl: HTMLInputElement

  onMount(() => { inputEl?.focus() })

  function searchDir() {
    return get(projectRoot) || get(cwd)
  }

  // takeLatest: ignore results from stale requests
  let gen = 0

  $effect(() => {
    const q = query
    const cs = caseSensitive
    const ww = wholeWord
    const rx = useRegex

    replaceCount = null
    searchError = ''

    if (!q.trim()) {
      results = []
      searching = false
      return
    }

    const myGen = ++gen
    searching = true

    const timer = setTimeout(async () => {
      try {
        const res = await SearchInFiles(searchDir(), q, cs, ww, rx)
        if (myGen === gen) {
          results = res ?? []
          searching = false
        }
      } catch (e: any) {
        if (myGen === gen) {
          searchError = rx ? 'Invalid regex' : String(e)
          results = []
          searching = false
        }
      }
    }, 280)

    return () => clearTimeout(timer)
  })

  async function runReplace() {
    if (!query.trim()) return
    replacing = true
    searchError = ''
    try {
      replaceCount = await ReplaceInFiles(searchDir(), query, replaceText, caseSensitive, wholeWord, useRegex)
      // re-trigger search by bumping gen through a dummy state write
      gen++ // stale guard — next $effect tick re-runs naturally via query deps
      const res = await SearchInFiles(searchDir(), query, caseSensitive, wholeWord, useRegex)
      results = res ?? []
    } catch (e: any) {
      searchError = String(e)
    } finally {
      replacing = false
    }
  }

  function openResult(r: SearchResultDTO) {
    openFileTab(r.file)
    showGlobalSearch.set(false)
  }

  function handleKey(e: KeyboardEvent) {
    if (e.key === 'Escape') showGlobalSearch.set(false)
  }

  function relPath(file: string) {
    const dir = searchDir()
    return dir && file.startsWith(dir + '/') ? file.slice(dir.length + 1) : file
  }

  const grouped = $derived(
    results.reduce<Record<string, SearchResultDTO[]>>((acc, r) => {
      (acc[r.file] ??= []).push(r)
      return acc
    }, {})
  )
</script>

<svelte:window onkeydown={handleKey} />

<div class="overlay" onclick={() => showGlobalSearch.set(false)} role="dialog" aria-modal="true">
  <div class="panel" onclick={(e) => e.stopPropagation()}>

    <div class="header">
      <span class="title">Search in Files</span>
      <button class="close-btn" onclick={() => showGlobalSearch.set(false)}>✕</button>
    </div>

    <div class="inputs">
      <div class="row">
        <input
          bind:this={inputEl}
          bind:value={query}
          class="input"
          placeholder="Search…"
          autocapitalize="none"
          autocorrect="off"
          autocomplete="off"
          spellcheck="false"
        />
        <div class="toggles">
          <button
            class="toggle" class:active={caseSensitive}
            onclick={() => caseSensitive = !caseSensitive}
            title="Match case"
          >Aa</button>
          <button
            class="toggle" class:active={wholeWord}
            onclick={() => wholeWord = !wholeWord}
            title="Match whole word"
          >ab</button>
          <button
            class="toggle" class:active={useRegex}
            onclick={() => useRegex = !useRegex}
            title="Use regular expression"
          >.*</button>
        </div>
        {#if searching}
          <span class="spinner">…</span>
        {/if}
      </div>
      <div class="row">
        <input
          bind:value={replaceText}
          class="input"
          placeholder="Replace with…"
          autocapitalize="none"
          autocorrect="off"
          autocomplete="off"
          spellcheck="false"
          onkeydown={(e) => { if (e.key === 'Enter') runReplace() }}
        />
        <button class="btn" onclick={runReplace} disabled={replacing || !query.trim()}>
          {replacing ? '…' : 'Replace All'}
        </button>
      </div>
    </div>

    {#if searchError}
      <div class="msg err">{searchError}</div>
    {:else if replaceCount !== null}
      <div class="msg ok">Replaced in {replaceCount} file{replaceCount === 1 ? '' : 's'}</div>
    {/if}

    <div class="results">
      {#if results.length === 0 && !searching && query.trim()}
        <div class="empty">No results</div>
      {:else}
        {#each Object.entries(grouped) as [file, hits]}
          <div class="file-group">
            <div class="file-header">{relPath(file)}</div>
            {#each hits as r}
              <button class="result-row" onclick={() => openResult(r)}>
                <span class="line-num">{r.line}</span>
                <span class="line-text">{r.text.trim()}</span>
              </button>
            {/each}
          </div>
        {/each}
        {#if results.length >= 500}
          <div class="empty">Showing first 500 matches</div>
        {/if}
      {/if}
    </div>

  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 9000;
    background: rgba(0,0,0,0.45);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 60px;
  }

  .panel {
    width: 620px;
    max-width: 90vw;
    max-height: 70vh;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .header {
    display: flex;
    align-items: center;
    padding: 10px 14px 8px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .title { font-size: 12px; font-weight: 600; color: var(--muted); flex: 1; }
  .close-btn {
    background: none; border: none; color: var(--muted);
    cursor: pointer; font-size: 13px; padding: 2px 5px; border-radius: 3px;
  }
  .close-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .inputs {
    padding: 10px 12px 8px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex-shrink: 0;
    border-bottom: 1px solid var(--border);
  }

  .row {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .input {
    flex: 1;
    background: var(--background);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 12px;
    padding: 5px 8px;
    outline: none;
    font-family: "SF Mono", Menlo, monospace;
  }
  .input:focus { border-color: var(--accent); }

  .toggles {
    display: flex;
    gap: 2px;
  }

  .toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    background: none;
    border: 1px solid transparent;
    border-radius: 4px;
    color: var(--muted);
    font-size: 11px;
    font-family: "SF Mono", Menlo, monospace;
    cursor: pointer;
    transition: color 0.08s, background 0.08s, border-color 0.08s;
  }
  .toggle:hover { color: var(--foreground); background: var(--bg-hover); }
  .toggle.active {
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    border-color: color-mix(in srgb, var(--accent) 35%, transparent);
  }

  .spinner {
    font-size: 11px;
    color: var(--muted);
    width: 14px;
    flex-shrink: 0;
  }

  .btn {
    padding: 5px 12px;
    background: var(--accent);
    color: #000;
    border: none;
    border-radius: 5px;
    font-size: 11px;
    cursor: pointer;
    white-space: nowrap;
    font-weight: 600;
    transition: opacity 0.1s;
  }
  .btn:disabled { opacity: 0.4; cursor: default; }
  .btn:not(:disabled):hover { opacity: 0.85; }

  .msg {
    padding: 5px 14px;
    font-size: 11px;
    flex-shrink: 0;
  }
  .msg.err { color: var(--error); }
  .msg.ok  { color: var(--success); }

  .results {
    overflow-y: auto;
    flex: 1;
    min-height: 0;
    padding: 4px 0;
  }

  .empty {
    padding: 10px 14px;
    font-size: 11px;
    color: var(--muted);
  }

  .file-group { margin-bottom: 2px; }

  .file-header {
    padding: 5px 14px 3px;
    font-size: 10px;
    font-weight: 600;
    color: var(--accent);
    font-family: "SF Mono", Menlo, monospace;
    position: sticky;
    top: 0;
    background: var(--bg-raised);
  }

  .result-row {
    display: flex;
    width: 100%;
    align-items: baseline;
    gap: 10px;
    padding: 3px 14px 3px 20px;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    color: var(--foreground);
    font-family: "SF Mono", Menlo, monospace;
    transition: background 0.06s;
  }
  .result-row:hover { background: var(--bg-selected); }

  .line-num {
    color: var(--muted);
    font-size: 10px;
    min-width: 28px;
    flex-shrink: 0;
    text-align: right;
  }

  .line-text {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 11px;
  }
</style>
