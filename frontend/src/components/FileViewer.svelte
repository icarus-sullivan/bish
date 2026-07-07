<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, keymap } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { defaultKeymap, historyKeymap, indentMore, indentLess } from '@codemirror/commands'
  import { search, searchKeymap } from '@codemirror/search'
  import { completionKeymap } from '@codemirror/autocomplete'
  import { syntaxHighlighting, HighlightStyle, StreamLanguage } from '@codemirror/language'
  import { Prec } from '@codemirror/state'
  import { basicSetup } from 'codemirror'
  import { tags as t } from '@lezer/highlight'
  import { javascript } from '@codemirror/lang-javascript'
  import { python } from '@codemirror/lang-python'
  import { css } from '@codemirror/lang-css'
  import { html } from '@codemirror/lang-html'
  import { json } from '@codemirror/lang-json'
  import { markdown } from '@codemirror/lang-markdown'
  import { yaml } from '@codemirror/lang-yaml'
  import { go } from '@codemirror/lang-go'
  import { shell } from '@codemirror/legacy-modes/mode/shell'
  import { ReadFile, WriteFile, SaveNewFile } from '../lib/wails'
  import { currentThemeName, cwd, projectRoot, updateTabPath, closeTab } from '../lib/stores'
  import { get } from 'svelte/store'

  const UNTITLED = '__new__'

  let { path, tabId }: { path: string; tabId: string } = $props()

  let container: HTMLDivElement
  let view: EditorView | null = null
  let modified = $state(false)
  let panelObserver: MutationObserver | undefined

  function patchSearchPanel(root: HTMLElement) {
    const panel = root.querySelector<HTMLElement>('.cm-search')
    if (!panel) return

    // disable auto-capitalize/correct on all text inputs
    panel.querySelectorAll<HTMLInputElement>('input[type=text]').forEach(el => {
      el.setAttribute('autocapitalize', 'none')
      el.setAttribute('autocorrect', 'off')
      el.setAttribute('autocomplete', 'off')
      el.setAttribute('spellcheck', 'false')
    })

    // replace button text with arrows + add tooltips
    const nextBtn = panel.querySelector<HTMLButtonElement>('button[name="next"]')
    const prevBtn = panel.querySelector<HTMLButtonElement>('button[name="prev"]')
    const selBtn  = panel.querySelector<HTMLButtonElement>('button[name="select"]')
    if (nextBtn && nextBtn.textContent !== '↓') { nextBtn.textContent = '↓'; nextBtn.title = 'Next match' }
    if (prevBtn && prevBtn.textContent !== '↑') { prevBtn.textContent = '↑'; prevBtn.title = 'Previous match' }
    if (selBtn)  selBtn.title = 'Select all matches'

    // add tooltips to toggle labels
    panel.querySelectorAll<HTMLElement>('label').forEach(label => {
      const cb = label.querySelector<HTMLInputElement>('input[type=checkbox]')
      if (!cb) return
      if (cb.name === 'case') label.title = 'Match case'
      if (cb.name === 're')   label.title = 'Use regex'
      if (cb.name === 'word') label.title = 'Match whole word'
    })

    // row break before replace input (only once)
    if (!panel.querySelector('.cm-row-break')) {
      const replaceField = panel.querySelector<HTMLElement>('input[name="replace"]')
      if (replaceField) {
        const spacer = document.createElement('div')
        spacer.className = 'cm-row-break'
        panel.insertBefore(spacer, replaceField)
      }
    }
  }
  let saving = $state(false)
  let saveError = $state('')

  const filename = $derived(path === UNTITLED ? 'Untitled' : (path.split('/').pop() ?? ''))

  // ─── language detection ───────────────────────────────────────────────────
  function langFor(p: string) {
    const ext = p.split('.').pop()?.toLowerCase() ?? ''
    if (['js','mjs','cjs'].includes(ext))      return javascript()
    if (ext === 'ts')                           return javascript({ typescript: true })
    if (ext === 'tsx')                          return javascript({ typescript: true, jsx: true })
    if (ext === 'jsx')                          return javascript({ jsx: true })
    if (ext === 'py')                           return python()
    if (ext === 'css')                          return css()
    if (['html','svelte','vue'].includes(ext))  return html()
    if (ext === 'json')                         return json()
    if (['md','markdown'].includes(ext))        return markdown()
    if (['yaml','yml'].includes(ext))           return yaml()
    if (ext === 'go')                           return go()
    if (['sh','bash','zsh','fish'].includes(ext)) return StreamLanguage.define(shell)
    return []
  }

  // ─── per-theme syntax highlight palettes ─────────────────────────────────
  type HSpec = Parameters<typeof HighlightStyle.define>[0]

  const palettes: Record<string, HSpec> = {
    catppuccin: [
      { tag: t.keyword,                                          color: '#cba6f7', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#89b4fa' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#f9e2af' },
      { tag: t.string,                                           color: '#a6e3a1' },
      { tag: t.number,                                           color: '#fab387' },
      { tag: t.bool,                                             color: '#fab387' },
      { tag: t.null,                                             color: '#f38ba8' },
      { tag: t.comment,                                          color: '#585b70', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#89dceb' },
      { tag: t.punctuation,                                      color: '#9399b2' },
      { tag: t.tagName,                                          color: '#f38ba8' },
      { tag: t.attributeName,                                    color: '#89b4fa' },
      { tag: t.propertyName,                                     color: '#89dceb' },
      { tag: t.variableName,                                     color: '#cdd6f4' },
      { tag: t.definition(t.variableName),                       color: '#cdd6f4' },
      { tag: t.self,                                             color: '#f38ba8' },
    ],
    'tokyo-night': [
      { tag: t.keyword,                                          color: '#bb9af7', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#7aa2f7' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#e0af68' },
      { tag: t.string,                                           color: '#9ece6a' },
      { tag: t.number,                                           color: '#ff9e64' },
      { tag: t.bool,                                             color: '#ff9e64' },
      { tag: t.null,                                             color: '#f7768e' },
      { tag: t.comment,                                          color: '#565f89', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#89ddff' },
      { tag: t.punctuation,                                      color: '#c0caf5' },
      { tag: t.tagName,                                          color: '#f7768e' },
      { tag: t.attributeName,                                    color: '#bb9af7' },
      { tag: t.propertyName,                                     color: '#73daca' },
      { tag: t.variableName,                                     color: '#c0caf5' },
      { tag: t.self,                                             color: '#f7768e' },
    ],
    obsidian: [
      { tag: t.keyword,                                          color: '#7c5fe8', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#9d84f0' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#b8a4f5' },
      { tag: t.string,                                           color: '#b5e853' },
      { tag: t.number,                                           color: '#c9a227' },
      { tag: t.bool,                                             color: '#c9a227' },
      { tag: t.null,                                             color: '#e05c5c' },
      { tag: t.comment,                                          color: '#3a3550', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#8878cc' },
      { tag: t.punctuation,                                      color: '#5c5080' },
      { tag: t.tagName,                                          color: '#e05c5c' },
      { tag: t.attributeName,                                    color: '#9d84f0' },
      { tag: t.propertyName,                                     color: '#b8a4f5' },
      { tag: t.variableName,                                     color: '#d8d0c0' },
      { tag: t.self,                                             color: '#7c5fe8' },
    ],
    vos: [
      { tag: t.keyword,                                          color: '#569cd6', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#dcdcaa' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#4ec9b0' },
      { tag: t.string,                                           color: '#ce9178' },
      { tag: t.number,                                           color: '#b5cea8' },
      { tag: t.bool,                                             color: '#569cd6' },
      { tag: t.null,                                             color: '#569cd6' },
      { tag: t.comment,                                          color: '#6a9955', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#d4d4d4' },
      { tag: t.punctuation,                                      color: '#d4d4d4' },
      { tag: t.tagName,                                          color: '#4ec9b0' },
      { tag: t.attributeName,                                    color: '#9cdcfe' },
      { tag: t.propertyName,                                     color: '#9cdcfe' },
      { tag: t.variableName,                                     color: '#9cdcfe' },
      { tag: t.definition(t.variableName),                       color: '#dcdcaa' },
      { tag: t.self,                                             color: '#569cd6' },
      { tag: t.modifier,                                         color: '#569cd6' },
    ],
    gruvbox: [
      { tag: t.keyword,                                          color: '#fb4934', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#8ec07c' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#fabd2f' },
      { tag: t.string,                                           color: '#b8bb26' },
      { tag: t.number,                                           color: '#d3869b' },
      { tag: t.bool,                                             color: '#d3869b' },
      { tag: t.null,                                             color: '#fb4934' },
      { tag: t.comment,                                          color: '#928374', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#ebdbb2' },
      { tag: t.punctuation,                                      color: '#a89984' },
      { tag: t.tagName,                                          color: '#83a598' },
      { tag: t.attributeName,                                    color: '#fabd2f' },
      { tag: t.propertyName,                                     color: '#8ec07c' },
      { tag: t.variableName,                                     color: '#ebdbb2' },
    ],
    nord: [
      { tag: t.keyword,                                          color: '#81a1c1', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#88c0d0' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#ebcb8b' },
      { tag: t.string,                                           color: '#a3be8c' },
      { tag: t.number,                                           color: '#b48ead' },
      { tag: t.bool,                                             color: '#b48ead' },
      { tag: t.null,                                             color: '#bf616a' },
      { tag: t.comment,                                          color: '#616e88', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#81a1c1' },
      { tag: t.punctuation,                                      color: '#d8dee9' },
      { tag: t.tagName,                                          color: '#bf616a' },
      { tag: t.attributeName,                                    color: '#8fbcbb' },
      { tag: t.propertyName,                                     color: '#88c0d0' },
      { tag: t.variableName,                                     color: '#d8dee9' },
    ],
    monokai: [
      { tag: t.keyword,                                          color: '#f92672', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#a6e22e' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#66d9ef' },
      { tag: t.string,                                           color: '#e6db74' },
      { tag: t.number,                                           color: '#ae81ff' },
      { tag: t.bool,                                             color: '#ae81ff' },
      { tag: t.null,                                             color: '#ae81ff' },
      { tag: t.comment,                                          color: '#75715e', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#f8f8f2' },
      { tag: t.punctuation,                                      color: '#f8f8f2' },
      { tag: t.tagName,                                          color: '#f92672' },
      { tag: t.attributeName,                                    color: '#a6e22e' },
      { tag: t.propertyName,                                     color: '#66d9ef' },
      { tag: t.variableName,                                     color: '#f8f8f2' },
    ],
    light: [
      { tag: t.keyword,                                          color: '#0000ff', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#795e26' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#267f99' },
      { tag: t.string,                                           color: '#a31515' },
      { tag: t.number,                                           color: '#098658' },
      { tag: t.bool,                                             color: '#0000ff' },
      { tag: t.null,                                             color: '#0000ff' },
      { tag: t.comment,                                          color: '#008000', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#000000' },
      { tag: t.punctuation,                                      color: '#000000' },
      { tag: t.variableName,                                     color: '#001080' },
      { tag: t.propertyName,                                     color: '#001080' },
    ],
    default: [
      { tag: t.keyword,                                          color: '#7986cb', fontWeight: '600' },
      { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#64b5f6' },
      { tag: [t.typeName, t.className, t.namespace],             color: '#4dd0e1' },
      { tag: t.string,                                           color: '#4dc988' },
      { tag: t.number,                                           color: '#ffa040' },
      { tag: t.bool,                                             color: '#ffa040' },
      { tag: t.null,                                             color: '#ff5f6e' },
      { tag: t.comment,                                          color: '#3a3f5c', fontStyle: 'italic' },
      { tag: t.operator,                                         color: '#8c8fa8' },
      { tag: t.punctuation,                                      color: '#5c6180' },
      { tag: t.variableName,                                     color: '#d4d8ed' },
      { tag: t.propertyName,                                     color: '#a0aec8' },
    ],
  }

  function highlightFor(themeName: string) {
    const spec = palettes[themeName] ?? palettes.catppuccin
    return syntaxHighlighting(HighlightStyle.define(spec))
  }

  // ─── theme-aware chrome ───────────────────────────────────────────────────
  function isDark(): boolean {
    const hex = getComputedStyle(document.documentElement)
      .getPropertyValue('--background').trim().replace('#', '')
    if (hex.length < 6) return true
    const r = parseInt(hex.slice(0, 2), 16)
    const g = parseInt(hex.slice(2, 4), 16)
    const b = parseInt(hex.slice(4, 6), 16)
    return (0.299 * r + 0.587 * g + 0.114 * b) / 255 < 0.5
  }

  function bishTheme(dark: boolean) {
    return EditorView.theme({
      '&': { height: '100%', fontSize: '13px' },
      '.cm-scroller': {
        fontFamily: '"SF Mono", Menlo, Monaco, "Courier New", monospace',
        overflow: 'auto',
        lineHeight: '1.6',
      },
      '.cm-content': { caretColor: 'var(--accent)', padding: '4px 0 12px' },
      '.cm-focused': { outline: 'none' },
      '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--accent)', borderLeftWidth: '2px' },
      '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
        background: 'var(--bg-selected) !important',
      },
      '.cm-activeLine': { backgroundColor: 'var(--bg-hover)' },
      '.cm-activeLineGutter': { backgroundColor: 'var(--bg-hover)' },
      '.cm-gutters': {
        background: 'var(--bg-raised)',
        color: 'var(--muted)',
        border: 'none',
        borderRight: '1px solid var(--border)',
      },
      '.cm-lineNumbers .cm-gutterElement': { padding: '0 14px 0 8px', minWidth: '44px' },
      '.cm-foldGutter .cm-gutterElement': { color: 'var(--muted)' },
      '.cm-matchingBracket': {
        background: 'color-mix(in srgb, var(--accent) 20%, transparent)',
        outline: '1px solid var(--accent)',
        borderRadius: '2px',
      },
      '.cm-panels': {
        background: 'var(--bg-raised)',
        borderBottom: '1px solid var(--border)',
        color: 'var(--foreground)',
      },
      '.cm-search': {
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: '6px',
        padding: '10px 14px',
        rowGap: '8px',
      },
      '.cm-textfield': {
        background: 'var(--background)',
        border: '1px solid var(--border)',
        borderRadius: '5px',
        color: 'var(--foreground)',
        fontSize: '12px',
        padding: '5px 8px',
        outline: 'none',
        fontFamily: '"SF Mono", Menlo, monospace',
        minWidth: '180px',
        transition: 'border-color 0.1s',
      },
      '.cm-textfield:focus': { borderColor: 'var(--accent)' },
      '.cm-button': {
        background: 'none',
        border: '1px solid var(--border)',
        borderRadius: '4px',
        color: 'var(--muted)',
        fontSize: '11px',
        padding: '4px 10px',
        cursor: 'pointer',
        fontFamily: '-apple-system, sans-serif',
        transition: 'color 0.1s, background 0.1s',
        backgroundImage: 'none',
      },
      '.cm-button:hover': {
        color: 'var(--foreground)',
        background: 'var(--bg-hover)',
        borderColor: 'var(--border)',
      },
      // "Replace All" is the last button — highlight it as the primary action
      '.cm-button[name="replace"]': {
        background: 'none',
        color: 'var(--muted)',
      },
      '.cm-button[name="replaceAll"]': {
        background: 'var(--accent)',
        color: '#000',
        fontWeight: '600',
        border: 'none',
      },
      '.cm-button[name="replaceAll"]:hover': {
        opacity: '0.85',
        color: '#000',
        background: 'var(--accent)',
      },
      '.cm-searchMatch': { background: 'color-mix(in srgb, var(--warning) 30%, transparent)', outline: '1px solid var(--warning)' },
      '.cm-searchMatch.cm-searchMatch-selected': { background: 'color-mix(in srgb, var(--accent) 35%, transparent)' },
      '.cm-selectionMatch': { background: 'color-mix(in srgb, var(--accent) 15%, transparent)' },
      '.cm-tooltip': {
        border: '1px solid var(--border)',
        background: 'var(--background)',
        borderRadius: '6px',
        boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
        color: 'var(--foreground)',
      },
      '.cm-tooltip-autocomplete ul li[aria-selected]': { background: 'var(--bg-selected)' },
      '.cm-completionLabel': { color: 'var(--foreground)' },
      '.cm-completionDetail': { color: 'var(--muted)' },
    }, { dark })
  }

  // ─── load/reload ──────────────────────────────────────────────────────────
  async function load(p: string, themeName: string) {
    view?.destroy()
    view = null
    modified = false
    saveError = ''

    let content = ''
    if (p !== UNTITLED) {
      content = await ReadFile(p).catch((e: any) => {
        saveError = String(e)
        return ''
      })
    }
    if (!container) return

    view = new EditorView({
      state: EditorState.create({
        doc: content,
        extensions: [
          basicSetup,
          bishTheme(isDark()),
          highlightFor(themeName),
          langFor(p),
          search({ top: true }),
          // Highest priority: always consume Tab so focus never escapes the editor
          Prec.highest(keymap.of([
            { key: 'Tab',       run: (v) => { indentMore(v); return true } },
            { key: 'Shift-Tab', run: (v) => { indentLess(v); return true } },
          ])),
          keymap.of([
            { key: 'Mod-s', run: () => { save(); return true } },
            ...defaultKeymap,
            ...historyKeymap,
            ...searchKeymap,
            ...completionKeymap,
          ]),
          EditorView.updateListener.of((upd) => {
            if (upd.docChanged) modified = true
          }),
        ],
      }),
      parent: container,
    })

    panelObserver?.disconnect()
    panelObserver = new MutationObserver(() => patchSearchPanel(container))
    panelObserver.observe(container, { childList: true, subtree: true })
  }

  // Reload when path or theme changes
  $effect(() => { load(path, $currentThemeName) })

  // ─── save ─────────────────────────────────────────────────────────────────
  async function save() {
    if (!view || saving) return
    saving = true
    saveError = ''
    try {
      if (path === UNTITLED) {
        const dir = get(projectRoot) || get(cwd)
        const realPath = await SaveNewFile(view.state.doc.toString(), dir)
        if (realPath) {
          updateTabPath(tabId, realPath)
          modified = false
        }
      } else {
        await WriteFile(path, view.state.doc.toString())
        modified = false
      }
    } catch (e: any) {
      saveError = String(e)
    } finally {
      saving = false
    }
  }

  onDestroy(() => {
    panelObserver?.disconnect()
    view?.destroy()
  })
</script>

<svelte:window onkeydown={(e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === 's') { e.preventDefault(); save() }
}} />

<div class="viewer-wrap">
  <div class="viewer-bar">
    <span class="filename" class:modified>{filename}{modified ? ' ●' : ''}</span>
    {#if saving}
      <span class="status muted">Saving…</span>
    {:else if saveError}
      <span class="status err" title={saveError}>Error saving</span>
    {/if}
    <span class="spacer"></span>
    <span class="hint">⌘S · ⌘F · ⌘H · ⌘Z</span>
    <button class="close-btn" onclick={() => closeTab(tabId)} title="Close">✕</button>
  </div>
  <div bind:this={container} class="cm-container"></div>
</div>

<style>
  .viewer-wrap {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
    overflow: hidden;
    background: var(--background);
  }

  .viewer-bar {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 12px;
    height: 30px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    background: var(--bg-raised);
  }

  .filename {
    color: var(--foreground);
    font-family: "SF Mono", Menlo, monospace;
    font-size: 12px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .filename.modified { color: var(--warning); }

  .status { font-size: 11px; }
  .status.muted { color: var(--muted); }
  .status.err   { color: var(--error); cursor: help; }

  .spacer { flex: 1; }
  .hint { font-size: 10px; color: color-mix(in srgb, var(--muted) 60%, transparent); }

  .close-btn {
    background: none; border: none;
    color: var(--muted); cursor: pointer;
    font-size: 14px; line-height: 1;
    padding: 3px 5px; border-radius: 3px;
  }
  .close-btn:hover { color: var(--foreground); background: var(--bg-hover); }

  .cm-container { flex: 1; overflow: hidden; min-height: 0; }
  .cm-container :global(.cm-editor) { height: 100%; }

  /* ── CodeMirror search panel layout & icons ── */
  :global(.cm-search) {
    display: flex !important;
    flex-wrap: wrap !important;
    align-items: center !important;
    gap: 6px !important;
    padding: 10px 14px !important;
    row-gap: 4px !important;
  }
  :global(.cm-search br) { display: none; }
  :global(.cm-row-break) {
    order: 10;
    flex-basis: 100%;
    height: 0;
  }
  /* row 1 ordering: find → toggles → arrows → all */
  :global(.cm-textfield[name="search"])  { order: 1; }
  :global(.cm-search label)              { order: 2; }
  :global(.cm-search button[name="prev"]) { order: 3; }
  :global(.cm-search button[name="next"]) { order: 4; }
  :global(.cm-search button[name="select"]) { order: 5; }
  /* row 2 */
  :global(.cm-textfield[name="replace"])      { order: 11; }
  :global(.cm-search button[name="replace"])  { order: 12; }
  :global(.cm-search button[name="replaceAll"]) { order: 13; }
  :global(.cm-search button[name="close"])    { order: 99; }
  :global(.cm-search label) {
    display: flex !important;
    align-items: center !important;
    justify-content: center !important;
    width: 26px !important;
    height: 26px !important;
    background: none !important;
    border: 1px solid transparent !important;
    border-radius: 4px !important;
    color: var(--muted) !important;
    font-size: 0 !important;
    font-family: "SF Mono", Menlo, monospace !important;
    cursor: pointer !important;
    user-select: none !important;
    transition: color 0.08s, background 0.08s, border-color 0.08s !important;
  }
  :global(.cm-search label:hover) {
    color: var(--foreground) !important;
    background: var(--bg-hover) !important;
  }
  :global(.cm-search label:has(input:checked)) {
    color: var(--accent) !important;
    background: color-mix(in srgb, var(--accent) 12%, transparent) !important;
    border-color: color-mix(in srgb, var(--accent) 35%, transparent) !important;
  }
  :global(.cm-search label::after) {
    font-size: 11px !important;
    line-height: 1 !important;
  }
  :global(.cm-search label:has([name="case"])::after) { content: "Aa" !important; }
  :global(.cm-search label:has([name="re"])::after)   { content: ".*" !important; }
  :global(.cm-search label:has([name="word"])::after) { content: "ab" !important; }
  /* next / prev as icon-sized arrow buttons */
  :global(.cm-search button[name="next"]),
  :global(.cm-search button[name="prev"]) {
    width: 26px !important;
    height: 26px !important;
    padding: 0 !important;
    font-size: 14px !important;
    display: flex !important;
    align-items: center !important;
    justify-content: center !important;
    border: 1px solid transparent !important;
    background: none !important;
    color: var(--muted) !important;
    cursor: pointer !important;
    border-radius: 4px !important;
    transition: color 0.08s, background 0.08s !important;
  }
  :global(.cm-search button[name="next"]:hover),
  :global(.cm-search button[name="prev"]:hover) {
    color: var(--foreground) !important;
    background: var(--bg-hover) !important;
  }
  :global(.cm-search input[type=checkbox]) {
    position: absolute !important;
    opacity: 0 !important;
    width: 0 !important;
    height: 0 !important;
    pointer-events: none !important;
  }
</style>
