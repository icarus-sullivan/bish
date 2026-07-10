<script lang="ts">
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { WebglAddon } from '@xterm/addon-webgl'
  import { SearchAddon } from '@xterm/addon-search'
  import '@xterm/xterm/css/xterm.css'
  import { focusedPane, theme, activeTabId, setTerminalTitle } from '../lib/stores'
  import { get } from 'svelte/store'
  import { on, WritePTY, ResizePTY, WritePTYTab, ResizePTYTab } from '../lib/wails'
  import { OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime'

  let { terminalId = 'main' }: { terminalId?: string } = $props()

  let container: HTMLDivElement
  let term: Terminal
  let fitAddon: FitAddon
  let searchAddon: SearchAddon

  // term-wrap renders (and sizes) immediately; the terminal itself isn't even
  // constructed until the container has real layout, and stays hidden until
  // the renderer is decided — no intermediate frame is ever visible
  let ready = $state(false)

  // find-in-terminal bar
  let showFind = $state(false)
  let findQuery = $state('')
  let findInput: HTMLInputElement | undefined = $state()

  function closeFind() {
    showFind = false
    findQuery = ''
    searchAddon?.clearDecorations()
    term?.focus()
  }
  function findKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.preventDefault(); closeFind() }
    else if (e.key === 'Enter' && e.shiftKey) { e.preventDefault(); searchAddon?.findPrevious(findQuery) }
    else if (e.key === 'Enter') { e.preventDefault(); searchAddon?.findNext(findQuery) }
  }

  function themeFor(t: any) {
    const cs = getComputedStyle(document.documentElement)
    const get = (v: string) => cs.getPropertyValue(v).trim()
    return {
      background:          (t?.background)    || get('--background') || '#1e1e2e',
      foreground:          (t?.foreground)    || get('--foreground') || '#cdd6f4',
      cursor:              (t?.accent)        || get('--accent')     || '#cba6f7',
      cursorAccent:        (t?.background)    || get('--background') || '#1e1e2e',
      selectionBackground: (t?.border)        || get('--border')     || '#313244',
      black:   '#45475a', red:     (t?.error)   || '#f38ba8',
      green:   (t?.success) || '#a6e3a1',
      yellow:  (t?.warning) || '#fab387',
      blue:    '#89b4fa', magenta: '#f5c2e7',
      cyan:    '#94e2d5', white:   '#bac2de',
      brightBlack:   '#585b70', brightRed:    '#f38ba8',
      brightGreen:   '#a6e3a1', brightYellow: '#fab387',
      brightBlue:    '#89b4fa', brightMagenta:'#f5c2e7',
      brightCyan:    '#94e2d5', brightWhite:  '#a6adc8',
    }
  }

  // Runs only once the container has non-zero layout — every metrics-derived
  // step (open, fit, webgl atlas) sees final dimensions, so there is nothing
  // to flash or correct afterwards.
  function setupTerminal(): () => void {
    term = new Terminal({
      fontFamily: '"SF Mono", Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      lineHeight: 1.4,
      theme: themeFor(null),
      cursorBlink: true,
      allowTransparency: false,
      scrollback: 10000,
      padding: 8,
      allowProposedApi: true,
    } as any)

    fitAddon = new FitAddon()
    const unicode11 = new Unicode11Addon()
    searchAddon = new SearchAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(unicode11)
    term.loadAddon(searchAddon)
    term.unicode.activeVersion = '11'
    term.open(container)
    fitAddon.fit()

    // GPU renderer (what VSCode uses); no GL / context loss → DOM fallback.
    // Loaded only while this tab is active — browsers cap WebGL contexts at
    // ~8-16, so hidden tabs must not each hold one.
    let gl: WebglAddon | undefined
    function loadGl() {
      if (gl) return
      try {
        const addon = new WebglAddon()
        addon.onContextLoss(() => { addon.dispose(); gl = undefined })
        term.loadAddon(addon)
        gl = addon
      } catch (err) {
        console.error('webgl addon failed, using DOM renderer', err)
      }
    }
    function dropGl() {
      gl?.dispose()
      gl = undefined
    }
    loadGl()
    // reveal one frame later: first visible paint is the final renderer
    requestAnimationFrame(() => { ready = true })

    term.attachCustomKeyEventHandler((e) => {
      if (e.ctrlKey && e.key === 'b' && e.type === 'keydown') {
        focusedPane.set('processes')
        return false
      }
      if ((e.metaKey || e.ctrlKey) && e.key === 'f' && e.type === 'keydown') {
        showFind = true
        requestAnimationFrame(() => findInput?.focus())
        return false
      }
      return true
    })

    const isMain = terminalId === 'main'
    term.onData((data) => isMain ? WritePTY(data) : WritePTYTab(terminalId, data))
    // OSC 0/2 title escapes (set by preexec in the bish shell init, or by
    // programs like claude/vim themselves) → tab label
    term.onTitleChange((t) => setTerminalTitle(terminalId, t))

    const dataEvent = isMain ? 'pty:data' : 'pty:data:' + terminalId
    const exitEvent = isMain ? 'pty:exit' : 'pty:exit:' + terminalId
    // keep the cancellers: a leaked listener writes to a disposed terminal on
    // remount (close tab → Enter), throwing inside the event dispatch and
    // starving the new terminal of pty:data entirely
    const offData = on(dataEvent, (data: string) => term.write(data))
    const offExit = on(exitEvent, () => term.write('\r\n\x1b[2m[process exited]\x1b[0m\r\n'))

    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit()
      if (isMain) ResizePTY(term.rows, term.cols)
      else ResizePTYTab(terminalId, term.rows, term.cols)
    })
    resizeObserver.observe(container)

    // Focus + GPU renderer when this terminal's tab becomes active;
    // hidden tabs render via the DOM fallback (buffer/scrollback unaffected)
    const unsubActive = activeTabId.subscribe((id) => {
      if (id === terminalId) {
        loadGl()
        requestAnimationFrame(() => { fitAddon.fit(); term.focus() })
      } else {
        dropGl()
      }
    })

    const unsubPane = focusedPane.subscribe((p) => {
      if (p === 'terminal' && get(activeTabId) === terminalId) term.focus()
    })

    const unsubTheme = theme.subscribe((t) => {
      if (term) term.options.theme = themeFor(t) as any
    })

    OnFileDrop((_x: number, _y: number, paths: string[]) => {
      if (!paths.length) return
      // only handle if this terminal's tab is active
      if (get(activeTabId) !== terminalId) return
      const text = paths.map(p => `"${p}"`).join(' ')
      const isMain = terminalId === 'main'
      if (isMain) { WritePTY(text) } else { WritePTYTab(terminalId, text) }
      term.focus()
    }, false)

    return () => {
      offData()
      offExit()
      unsubActive()
      unsubPane()
      unsubTheme()
      resizeObserver.disconnect()
      OnFileDropOff()
      dropGl()
      term.dispose()
    }
  }

  // Boot the terminal only when the element has real layout. Element sized on
  // mount → boot now; mounted under display:none → wait for first layout.
  // ponytail: a hidden-mounted terminal misses pty:data until first shown —
  // terminal tabs are always created active, so that path is theoretical.
  function initTerm(el: HTMLDivElement) {
    container = el
    let cleanup: (() => void) | undefined
    // let waiter: ResizeObserver | undefined
    // if (el.offsetWidth > 0 && el.offsetHeight > 0) {
    //   cleanup = setupTerminal()
    // } else {
    //   waiter = new ResizeObserver(() => {
    //     if (el.offsetWidth === 0 || el.offsetHeight === 0) return
    //     waiter!.disconnect()
    //     waiter = undefined
    //     cleanup = setupTerminal()
    //   })
    //   waiter.observe(el)
    // }
    // return {
    //   destroy() {
    //     waiter?.disconnect()
    //     cleanup?.()
    //   },
    // }
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        console.log('loading terminal')
        cleanup = setupTerminal()
      })
    })

    return {
      destroy() {
        cleanup?.()
      }
    }
  }
</script>

<div class="term-wrap">
  {#if showFind}
    <div class="find-bar">
      <input
        bind:this={findInput}
        bind:value={findQuery}
        placeholder="Find…"
        spellcheck="false"
        onkeydown={findKeydown}
        oninput={() => searchAddon?.findNext(findQuery, { incremental: true })}
      />
      <button class="find-btn" onclick={() => searchAddon?.findPrevious(findQuery)} title="Previous (⇧⏎)">↑</button>
      <button class="find-btn" onclick={() => searchAddon?.findNext(findQuery)} title="Next (⏎)">↓</button>
      <button class="find-btn" onclick={closeFind} title="Close (Esc)">✕</button>
    </div>
  {/if}
  <div use:initTerm class="term-container" style:visibility={ready ? 'visible' : 'hidden'}></div>
</div>

<style>
  .term-wrap {
    width: 100%;
    height: 100%;
    overflow: hidden;
    position: relative;
  }
  .find-bar {
    position: absolute;
    top: 6px;
    right: 14px;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 5px 6px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 16px var(--shadow-color);
  }
  .find-bar input {
    background: var(--background);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--foreground);
    font-size: 12px;
    font-family: "SF Mono", Menlo, monospace;
    padding: 3px 8px;
    width: 180px;
    outline: none;
  }
  .find-bar input:focus { border-color: var(--accent); }
  .find-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    padding: 3px 4px;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .find-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .term-container {
    width: 100%;
    height: 100%;
  }
  :global(.xterm) { height: 100%; }
  :global(.xterm-viewport) { scrollbar-width: thin; scrollbar-color: var(--border) transparent; }
</style>
