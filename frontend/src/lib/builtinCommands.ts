import { get } from 'svelte/store'
import { registerCommands } from './commands'
import { NewTerminal } from './wails'
import {
  openFileTab, openSettingsTab, addTerminalTab, closeTab, reopenMainTab,
  showPalette, showGlobalSearch, showRight, tabs, activeTabId,
  cycleTab, terminalFontSize,
} from './stores'

async function newTerminal() {
  try { addTerminalTab(await NewTerminal()) } catch {}
}

// Registered once at startup (App.svelte). Feature-gated palettes still work —
// these are plain actions; the palette UI itself is gated by featureOn.
export function registerBuiltinCommands() {
  registerCommands([
    { id: 'file.new',        title: 'New File',            run: () => openFileTab('__new__') },
    { id: 'file.goto',       title: 'Go to File…',         run: () => showPalette.set(true) },
    { id: 'search.global',   title: 'Search in Files…',    run: () => { showGlobalSearch.set(true) } },
    { id: 'terminal.new',    title: 'New Terminal',        run: newTerminal },
    { id: 'terminal.focus',  title: 'Focus Terminal',      run: reopenMainTab },
    { id: 'settings.open',   title: 'Open Settings',       run: () => openSettingsTab() },
    { id: 'sidebar.toggle',  title: 'Toggle Sidebar',      run: () => showRight.update(v => !v) },
    { id: 'tab.close',       title: 'Close Tab',           run: () => closeTab(get(activeTabId)),
      when: () => get(tabs).length > 0 },
    { id: 'tab.next',        title: 'Next Tab',            run: () => cycleTab(1) },
    { id: 'tab.prev',        title: 'Previous Tab',        run: () => cycleTab(-1) },
    { id: 'terminal.zoomIn',  title: 'Terminal: Zoom In',  run: () => terminalFontSize.update(n => Math.min(28, n + 1)) },
    { id: 'terminal.zoomOut', title: 'Terminal: Zoom Out', run: () => terminalFontSize.update(n => Math.max(8, n - 1)) },
    { id: 'terminal.zoomReset', title: 'Terminal: Reset Zoom', run: () => terminalFontSize.set(13) },
  ])
}
