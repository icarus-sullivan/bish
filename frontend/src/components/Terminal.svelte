<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import '@xterm/xterm/css/xterm.css'
  import { focusedPane, theme, activeTabId } from '../lib/stores'
  import { get } from 'svelte/store'
  import { on, WritePTY, ResizePTY, WritePTYTab, ResizePTYTab } from '../lib/wails'
  import { OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime'

  let { terminalId = 'main' }: { terminalId?: string } = $props()

  let container: HTMLDivElement
  let term: Terminal
  let fitAddon: FitAddon

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

  onMount(() => {
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
    term.loadAddon(fitAddon)
    term.loadAddon(unicode11)
    term.unicode.activeVersion = '11'
    term.open(container)
    fitAddon.fit()

    term.attachCustomKeyEventHandler((e) => {
      if (e.ctrlKey && e.key === 'b' && e.type === 'keydown') {
        focusedPane.set('processes')
        return false
      }
      return true
    })

    const isMain = terminalId === 'main'
    term.onData((data) => isMain ? WritePTY(data) : WritePTYTab(terminalId, data))

    const dataEvent = isMain ? 'pty:data' : 'pty:data:' + terminalId
    const exitEvent = isMain ? 'pty:exit' : 'pty:exit:' + terminalId
    on(dataEvent, (data: string) => term.write(data))
    on(exitEvent, () => term.write('\r\n\x1b[2m[process exited]\x1b[0m\r\n'))

    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit()
      if (isMain) ResizePTY(term.rows, term.cols)
      else ResizePTYTab(terminalId, term.rows, term.cols)
    })
    resizeObserver.observe(container)

    // Focus when this terminal's tab becomes active
    const unsubActive = activeTabId.subscribe((id) => {
      if (id === terminalId) {
        requestAnimationFrame(() => { fitAddon.fit(); term.focus() })
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
      unsubActive()
      unsubPane()
      unsubTheme()
      resizeObserver.disconnect()
      OnFileDropOff()
      term.dispose()
    }
  })
</script>

<div class="term-wrap">
  <div bind:this={container} class="term-container"></div>
</div>

<style>
  .term-wrap {
    width: 100%;
    height: 100%;
    overflow: hidden;
  }
  .term-container {
    width: 100%;
    height: 100%;
  }
  :global(.xterm) { height: 100%; }
  :global(.xterm-viewport) { scrollbar-width: thin; scrollbar-color: var(--border) transparent; }
</style>
