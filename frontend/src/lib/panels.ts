import type { Component } from 'svelte'
import { derived } from 'svelte/store'
import { IconFolder, IconGitBranch, IconActivity, IconBookmark, IconRocket } from '@tabler/icons-svelte'
import { IconListSearch, IconSparkles, IconAlertTriangle, IconBug, IconFlask, IconPuzzle, IconCode } from '@tabler/icons-svelte'
import FileTree from '../components/FileTree.svelte'
import GitPanel from '../components/GitPanel.svelte'
import ProcessList from '../components/ProcessList.svelte'
import CommandList from '../components/CommandList.svelte'
import CommandCenter from '../components/CommandCenter.svelte'
import Outline from '../components/Outline.svelte'
import Problems from '../components/Problems.svelte'
import DebugPanel from '../components/DebugPanel.svelte'
import Tests from '../components/Tests.svelte'
import ExtensionsPanel from '../components/ExtensionsPanel.svelte'
import ExtensionPanelHost from '../components/ExtensionPanelHost.svelte'
import LanguagesPanel from '../components/LanguagesPanel.svelte'
import AssistantPanel from '../components/AssistantPanel.svelte'
import { loadedExtensions } from './extensions'
import { resolveExtensionIcon } from './extensionIcons'

// The built-in "plugin" registry: a future plugin API pushes onto this array.
export interface Panel {
  id: string
  title: string
  // @tabler/icons-svelte@3.x ships Svelte 4 class components (SvelteComponentTyped).
  // Svelte 5 renders them fine via its legacy-compat layer, but they don't
  // structurally satisfy Svelte 5's function-shaped Component<Props> type —
  // left untyped rather than fighting the two component shapes.
  icon: any
  component: Component<any>
  feature?: string  // when set, panel only shows if featureOn(feature)
  props?: Record<string, any>  // extra props spread onto `component`
}

export const builtinPanels: Panel[] = [
  { id: 'files', title: 'Files', icon: IconFolder, component: FileTree },
  { id: 'git', title: 'Git', icon: IconGitBranch, component: GitPanel },
  { id: 'outline', title: 'Outline', icon: IconListSearch, component: Outline, feature: 'outline' },
  { id: 'problems', title: 'Problems', icon: IconAlertTriangle, component: Problems, feature: 'problems' },
  { id: 'debug', title: 'Debug', icon: IconBug, component: DebugPanel, feature: 'debugger' },
  { id: 'tests', title: 'Tests', icon: IconFlask, component: Tests, feature: 'tests' },
  // aggregate install/enable/disable/uninstall manager — each extension's own
  // contributed panels additionally get their own sidebar entry, below
  { id: 'extensions', title: 'Extensions', icon: IconPuzzle, component: ExtensionsPanel, feature: 'extensions' },
  { id: 'languages', title: 'Languages', icon: IconCode, component: LanguagesPanel, feature: 'languageExtensions' },
  { id: 'assistant', title: 'Assistant', icon: IconSparkles, component: AssistantPanel, feature: 'assistant' },
  // panels stay mounted (display:none) when inactive, and processes run in the
  // Go backend anyway — switching/hiding never kills a running process
  { id: 'processes', title: 'Processes', icon: IconActivity, component: ProcessList },
  { id: 'commands', title: 'Saved Commands', icon: IconBookmark, component: CommandList },
  { id: 'commandCenter', title: 'Command Center', icon: IconRocket, component: CommandCenter, feature: 'commandCenter' },
]

// Reactive: built-ins plus one sidebar entry per enabled extension's
// contributed panel, so each extension can own its own icon instead of
// being lumped into the single "Extensions" aggregate panel above.
export const panels = derived(loadedExtensions, (exts) => [
  ...builtinPanels,
  ...exts.filter(e => e.enabled).flatMap(e => (e.panels ?? []).map(p => ({
    id: `ext:${e.name}:${p.id}`,
    title: p.title,
    icon: resolveExtensionIcon(p.icon),
    component: ExtensionPanelHost,
    props: { extName: e.name, panelId: p.id },
  } satisfies Panel))),
])
