import type { Component } from 'svelte'
import { IconFolder, IconGitBranch, IconActivity, IconBookmark } from '@tabler/icons-svelte'
import { IconListSearch, IconSparkles } from '@tabler/icons-svelte'
import FileTree from '../components/FileTree.svelte'
import GitPanel from '../components/GitPanel.svelte'
import ProcessList from '../components/ProcessList.svelte'
import CommandList from '../components/CommandList.svelte'
import Outline from '../components/Outline.svelte'
import AssistantPanel from '../components/AssistantPanel.svelte'

// The built-in "plugin" registry: a future plugin API pushes onto this array.
export interface Panel {
  id: string
  title: string
  icon: Component<any>
  component: Component<any>
  feature?: string  // when set, panel only shows if featureOn(feature)
}

export const panels: Panel[] = [
  { id: 'files', title: 'Files', icon: IconFolder, component: FileTree },
  { id: 'git', title: 'Git', icon: IconGitBranch, component: GitPanel },
  { id: 'outline', title: 'Outline', icon: IconListSearch, component: Outline, feature: 'outline' },
  { id: 'assistant', title: 'Assistant', icon: IconSparkles, component: AssistantPanel, feature: 'assistant' },
  // panels stay mounted (display:none) when inactive, and processes run in the
  // Go backend anyway — switching/hiding never kills a running process
  { id: 'processes', title: 'Processes', icon: IconActivity, component: ProcessList },
  { id: 'commands', title: 'Saved Commands', icon: IconBookmark, component: CommandList },
]
