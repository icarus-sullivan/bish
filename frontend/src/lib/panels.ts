import type { Component } from 'svelte'
import { IconFolder, IconGitBranch, IconActivity, IconBookmark } from '@tabler/icons-svelte'
import FileTree from '../components/FileTree.svelte'
import GitPanel from '../components/GitPanel.svelte'
import ProcessList from '../components/ProcessList.svelte'
import CommandList from '../components/CommandList.svelte'

// The built-in "plugin" registry: a future plugin API pushes onto this array.
export interface Panel {
  id: string
  title: string
  icon: Component<any>
  component: Component<any>
}

export const panels: Panel[] = [
  { id: 'files', title: 'Files', icon: IconFolder, component: FileTree },
  { id: 'git', title: 'Git', icon: IconGitBranch, component: GitPanel },
  // panels stay mounted (display:none) when inactive, and processes run in the
  // Go backend anyway — switching/hiding never kills a running process
  { id: 'processes', title: 'Processes', icon: IconActivity, component: ProcessList },
  { id: 'commands', title: 'Saved Commands', icon: IconBookmark, component: CommandList },
]
