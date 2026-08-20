import type { Component } from 'svelte'
import { IconFolder, IconGitBranch, IconActivity, IconBookmark } from '@tabler/icons-svelte'
import { IconListSearch, IconSparkles, IconAlertTriangle, IconBug, IconFlask, IconPuzzle, IconCode } from '@tabler/icons-svelte'
import FileTree from '../components/FileTree.svelte'
import GitPanel from '../components/GitPanel.svelte'
import ProcessList from '../components/ProcessList.svelte'
import CommandList from '../components/CommandList.svelte'
import Outline from '../components/Outline.svelte'
import Problems from '../components/Problems.svelte'
import DebugPanel from '../components/DebugPanel.svelte'
import Tests from '../components/Tests.svelte'
import ExtensionsPanel from '../components/ExtensionsPanel.svelte'
import LanguagesPanel from '../components/LanguagesPanel.svelte'
import AssistantPanel from '../components/AssistantPanel.svelte'

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
}

export const panels: Panel[] = [
  { id: 'files', title: 'Files', icon: IconFolder, component: FileTree },
  { id: 'git', title: 'Git', icon: IconGitBranch, component: GitPanel },
  { id: 'outline', title: 'Outline', icon: IconListSearch, component: Outline, feature: 'outline' },
  { id: 'problems', title: 'Problems', icon: IconAlertTriangle, component: Problems, feature: 'problems' },
  { id: 'debug', title: 'Debug', icon: IconBug, component: DebugPanel, feature: 'debugger' },
  { id: 'tests', title: 'Tests', icon: IconFlask, component: Tests, feature: 'tests' },
  // v1: one aggregating panel for every extension's contributed panels —
  // pushing a distinct entry per extension onto this array needs `panels`
  // to become a store RightSidebar reacts to, deferred until there's real
  // extension usage to justify it
  { id: 'extensions', title: 'Extensions', icon: IconPuzzle, component: ExtensionsPanel, feature: 'extensions' },
  { id: 'languages', title: 'Languages', icon: IconCode, component: LanguagesPanel, feature: 'languageExtensions' },
  { id: 'assistant', title: 'Assistant', icon: IconSparkles, component: AssistantPanel, feature: 'assistant' },
  // panels stay mounted (display:none) when inactive, and processes run in the
  // Go backend anyway — switching/hiding never kills a running process
  { id: 'processes', title: 'Processes', icon: IconActivity, component: ProcessList },
  { id: 'commands', title: 'Saved Commands', icon: IconBookmark, component: CommandList },
]
