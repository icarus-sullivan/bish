<script lang="ts">
  import { onMount } from 'svelte'
  import { get } from 'svelte/store'
  import { cwd, projectRoot, openFileTab } from '../lib/stores'
  import { GetAllFiles } from '../lib/wails'

  let { onClose }: { onClose: () => void } = $props()

  let query = $state('')
  let results: MatchResult[] = $state([])
  let selectedIdx = $state(0)
  let inputEl: HTMLInputElement

  interface MatchResult {
    path: string
    filename: string
    relPath: string
    score: number
    indices: number[]
  }

  // File index cache — invalidated when root changes
  let cachedRoot = ''
  let cachedFiles: string[] = []

  async function getFiles(): Promise<string[]> {
    const root = get(projectRoot) || get(cwd)
    if (root === cachedRoot && cachedFiles.length > 0) return cachedFiles
    const files = await GetAllFiles(root).catch(() => [])
    cachedRoot = root
    cachedFiles = files ?? []
    return cachedFiles
  }

  function globToRegex(pattern: string): RegExp {
    // Escape all regex specials except *, then replace * with .*
    const escaped = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&')
    return new RegExp('^' + escaped.replace(/\*/g, '.*') + '$', 'i')
  }

  function fuzzyMatch(query: string, path: string): { score: number; indices: number[] } | null {
    const q = query.toLowerCase()
    const p = path.toLowerCase()
    if (q.length === 0) return { score: 0, indices: [] }

    const indices: number[] = []
    let pi = 0
    for (let qi = 0; qi < q.length; qi++) {
      const found = p.indexOf(q[qi], pi)
      if (found === -1) return null
      indices.push(found)
      pi = found + 1
    }

    let score = 0
    // consecutive run bonus
    let run = 1
    for (let i = 1; i < indices.length; i++) {
      if (indices[i] === indices[i - 1] + 1) {
        run++
        score += 10 * run
      } else {
        run = 1
      }
    }
    // start-of-segment bonus
    const first = indices[0]
    if (first === 0 || path[first - 1] === '/') score += 50
    // filename match bonus
    const slash = path.lastIndexOf('/')
    const filename = path.slice(slash + 1).toLowerCase()
    if (filename.includes(q)) score += 100

    return { score, indices }
  }

  async function search(q: string) {
    if (q.trim() === '') {
      results = []
      selectedIdx = 0
      return
    }
    const files = await getFiles()
    const root = get(projectRoot) || get(cwd)
    const matched: MatchResult[] = []

    if (q.includes('*')) {
      const re = globToRegex(q)
      for (const f of files) {
        const slash = f.lastIndexOf('/')
        const filename = f.slice(slash + 1)
        if (!re.test(filename)) continue
        matched.push({
          path: f,
          filename,
          relPath: f.startsWith(root + '/') ? f.slice(root.length + 1) : f,
          score: 0,
          indices: [],
        })
      }
      matched.sort((a, b) => a.filename.localeCompare(b.filename))
    } else {
      for (const f of files) {
        const m = fuzzyMatch(q, f)
        if (!m) continue
        const slash = f.lastIndexOf('/')
        matched.push({
          path: f,
          filename: f.slice(slash + 1),
          relPath: f.startsWith(root + '/') ? f.slice(root.length + 1) : f,
          score: m.score,
          indices: m.indices,
        })
      }
      matched.sort((a, b) => b.score - a.score)
    }

    results = matched.slice(0, 100)
    selectedIdx = 0
  }

  function selectFile(path: string) {
    openFileTab(path)
    onClose()
  }

  function handleKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { onClose(); return }
    if (e.key === 'ArrowDown') { e.preventDefault(); selectedIdx = Math.min(selectedIdx + 1, results.length - 1) }
    if (e.key === 'ArrowUp')   { e.preventDefault(); selectedIdx = Math.max(selectedIdx - 1, 0) }
    if (e.key === 'Enter' && results.length > 0) selectFile(results[selectedIdx].path)
  }

  $effect(() => {
    search(query)
  })

  onMount(() => {
    inputEl?.focus()
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}>
  <div class="palette" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
    <div class="search-row">
      <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
      </svg>
      <input
        bind:this={inputEl}
        bind:value={query}
        onkeydown={handleKey}
        class="search-input"
        placeholder="Search files…"
        autocomplete="off"
        spellcheck="false"
      />
      {#if query}
        <button class="clear-btn" onclick={() => { query = ''; inputEl?.focus() }}>×</button>
      {/if}
    </div>

    {#if results.length > 0}
      <div class="results">
        {#each results as r, i (r.path)}
          <div
            class="result-row"
            class:active={i === selectedIdx}
            onclick={() => selectFile(r.path)}
            onmouseenter={() => selectedIdx = i}
            role="option"
            aria-selected={i === selectedIdx}
            tabindex="-1"
          >
            <div class="result-name">{r.filename}</div>
            <div class="result-path">{r.relPath}</div>
          </div>
        {/each}
      </div>
    {:else if query.trim()}
      <div class="empty">No files match</div>
    {/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 500;
    background: color-mix(in srgb, #000 50%, transparent);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 80px;
  }

  .palette {
    width: 600px;
    max-height: 480px;
    background: var(--bg-raised);
    border: 1px solid var(--border-focused);
    border-radius: 8px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    box-shadow: 0 24px 48px color-mix(in srgb, #000 60%, transparent);
  }

  .search-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .search-icon { color: var(--muted); flex-shrink: 0; }
  .search-input {
    flex: 1;
    background: none;
    border: none;
    outline: none;
    color: var(--foreground);
    font-size: 14px;
    font-family: inherit;
  }
  .search-input::placeholder { color: var(--muted); }
  .clear-btn {
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 16px;
    line-height: 1;
    padding: 0 2px;
  }
  .clear-btn:hover { color: var(--foreground); }

  .results {
    overflow-y: auto;
    flex: 1;
    padding: 4px 0;
  }

  .result-row {
    padding: 6px 14px;
    cursor: pointer;
    border-radius: 4px;
    margin: 0 4px;
  }
  .result-row.active { background: var(--bg-selected); }
  .result-row:hover { background: var(--bg-hover); }

  .result-name {
    font-size: 13px;
    color: var(--foreground);
    font-family: "SF Mono", Menlo, monospace;
  }
  .result-path {
    font-size: 11px;
    color: var(--muted);
    margin-top: 1px;
    font-family: "SF Mono", Menlo, monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .empty {
    padding: 20px 14px;
    font-size: 13px;
    color: var(--muted);
    text-align: center;
  }

</style>
