<script lang="ts">
  import { treeNodes, focusedPane, openFileTab, projectRoot, isMediaPath } from '../lib/stores'
  import ContextMenu from './ContextMenu.svelte'
  import { ToggleTreeNode, CdToPath, FSNewFile, FSNewFolder, FSRename, FSDelete, FSCopyPath, FSRevealInFinder, CloseProject, RefreshTree, CollapseAllTree } from '../lib/wails'
  import type { TreeNode } from '../lib/wails'
  import { IconFilePlus, IconFolderPlus, IconRefresh, IconLibraryMinus, IconChevronRight, IconChevronDown } from '@tabler/icons-svelte'
  import { get } from 'svelte/store'

  let menu: { x: number; y: number; node: TreeNode } | null = $state(null)
  let renaming: { path: string; value: string } | null = $state(null)
  let creating: { dirPath: string; isFolder: boolean; value: string } | null = $state(null)
  let activeDir = $state('')

  function resolveDir(): string {
    if (activeDir) return activeDir
    return get(projectRoot) || ''
  }

  function showMenu(e: MouseEvent, node: TreeNode) {
    e.preventDefault()
    e.stopPropagation()
    menu = { x: e.clientX, y: e.clientY, node }
  }

  function menuItems(node: TreeNode) {
    if (node.isDir) {
      return [
        { label: 'New File',       action: () => promptNew(node.path, false) },
        { label: 'New Folder',     action: () => promptNew(node.path, true) },
        { label: 'Open in Terminal', action: () => CdToPath(node.path) },
        { label: 'Copy Path',      action: async () => { const p = await FSCopyPath(node.path); navigator.clipboard.writeText(p) } },
        { label: 'Rename',         action: () => startRename(node.path, node.name) },
        { label: 'Delete',         action: () => FSDelete(node.path), danger: true },
      ]
    }
    return [
      { label: 'Open',            action: () => { openFileTab(node.path) } },
      ...(isMediaPath(node.path) ? [{ label: 'Open as Text', action: () => openFileTab(node.path, true) }] : []),
      { label: 'Reveal in Finder', action: () => FSRevealInFinder(node.path) },
      { label: 'Copy Path',       action: async () => { const p = await FSCopyPath(node.path); navigator.clipboard.writeText(p) } },
      { label: 'Rename',          action: () => startRename(node.path, node.name) },
      { label: 'Delete',          action: () => FSDelete(node.path), danger: true },
    ]
  }

  function startRename(path: string, name: string) {
    renaming = { path, value: name }
  }

  async function commitRename() {
    if (!renaming) return
    const dir = renaming.path.substring(0, renaming.path.lastIndexOf('/'))
    await FSRename(renaming.path, dir + '/' + renaming.value)
    renaming = null
  }

  function promptNew(dirPath: string, isFolder: boolean) {
    if (!dirPath) return
    creating = { dirPath, isFolder, value: '' }
  }

  async function commitCreate() {
    if (!creating) return
    const { dirPath, isFolder, value } = creating
    creating = null
    if (!value.trim()) return
    if (isFolder) await FSNewFolder(dirPath, value.trim())
    else await FSNewFile(dirPath, value.trim())
  }

  function autoFocus(el: HTMLInputElement) { el.focus() }

  function indent(depth: number) { return `${depth * 14}px` }

  // File type color categories
  function fileColor(name: string): string {
    const ext = name.split('.').pop()?.toLowerCase() ?? ''
    if (['js','mjs','cjs','jsx','ts','tsx','svelte','vue'].includes(ext)) return 'var(--accent)'
    if (['go','rs','c','cpp','h','java','kt','swift'].includes(ext)) return '#89b4fa'
    if (['py','rb','php','lua'].includes(ext)) return '#cba6f7'
    if (['json','yaml','yml','toml','env','ini'].includes(ext)) return 'var(--warning)'
    if (['md','txt','rst','adoc'].includes(ext)) return 'var(--muted)'
    if (['png','jpg','jpeg','gif','svg','webp','ico'].includes(ext)) return 'var(--success)'
    if (['sh','bash','zsh','fish'].includes(ext)) return '#94e2d5'
    if (['css','scss','sass','less'].includes(ext)) return '#f5c2e7'
    if (['html','htm','xml'].includes(ext)) return '#fab387'
    return 'var(--muted)'
  }
</script>

<div
  class="panel"
  class:focused={$focusedPane === 'tree'}
  onclick={() => focusedPane.set('tree')}
  onkeydown={(e) => e.key === 'Enter' && focusedPane.set('tree')}
  role="tree"
  tabindex="-1"
>
  <div class="header">
    <span class="header-label">Files</span>
    {#if $projectRoot}
      <span class="project-name" title={$projectRoot}>{$projectRoot.split('/').pop()}</span>
    {/if}
    <div class="header-actions">
      <button class="hdr-btn" onclick={() => promptNew(resolveDir(), false)} title="New File"><IconFilePlus size={13} /></button>
      <button class="hdr-btn" onclick={() => promptNew(resolveDir(), true)} title="New Folder"><IconFolderPlus size={13} /></button>
      <button class="hdr-btn" onclick={() => RefreshTree()} title="Refresh"><IconRefresh size={13} /></button>
      <button class="hdr-btn" onclick={() => CollapseAllTree()} title="Collapse All"><IconLibraryMinus size={13} /></button>
      {#if $projectRoot}
        <button class="close-project" onclick={() => CloseProject()} title="Close project">×</button>
      {/if}
    </div>
  </div>
  <div class="list">
    {#each $treeNodes as node (node.path)}
      <div
        class="row"
        class:selected={node.selected}
        style="padding-left: calc(8px + {indent(node.depth)})"
        onclick={() => { ToggleTreeNode(node.path); if (node.isDir) activeDir = node.path; else { activeDir = node.path.substring(0, node.path.lastIndexOf('/')); openFileTab(node.path) } }}
        oncontextmenu={(e) => showMenu(e, node)}
        role="treeitem"
        aria-selected={node.selected}
        aria-expanded={node.isDir ? node.expanded : undefined}
        tabindex="0"
        onkeydown={(e) => { if (e.key === 'Enter') { ToggleTreeNode(node.path); if (!node.isDir) openFileTab(node.path) } }}
      >
        {#if renaming?.path === node.path}
          <input
            class="rename-input"
            bind:value={renaming.value}
            onblur={commitRename}
            onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') renaming = null }}
          />
        {:else if node.isDir}
          <span class="dir-arrow">
            {#if node.expanded}<IconChevronDown size={14} />{:else}<IconChevronRight size={14} />{/if}
          </span>
          <span class="name dir">{node.name}</span>
        {:else}
          <span class="file-dot" style="background:{fileColor(node.name)}"></span>
          <span class="name">{node.name}</span>
        {/if}
      </div>
      {#if creating?.dirPath === node.path}
        <div class="row" style="padding-left: calc(8px + {indent(node.depth + 1)})">
          <span class="create-icon">{#if creating.isFolder}<IconChevronRight size={14} />{:else}·{/if}</span>
          <input
            class="rename-input"
            bind:value={creating.value}
            placeholder={creating.isFolder ? 'folder name' : 'file name'}
            onblur={commitCreate}
            onkeydown={(e) => { if (e.key === 'Enter') commitCreate(); if (e.key === 'Escape') creating = null }}
            use:autoFocus
          />
        </div>
      {/if}
    {/each}
  </div>
</div>

{#if menu}
  <ContextMenu
    x={menu.x}
    y={menu.y}
    items={menuItems(menu.node)}
    onClose={() => menu = null}
  />
{/if}

<style>
  .panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .header {
    display: flex;
    align-items: center;
    padding: 0 12px;
    height: 30px;
    flex-shrink: 0;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
  }
  .header-label {
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--muted);
    transition: color 0.15s;
  }
  .panel.focused .header-label { color: var(--accent); }

  .project-name {
    flex: 1;
    font-size: 11px;
    color: var(--foreground);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-left: 6px;
    font-weight: 500;
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 1px;
    margin-left: auto;
    flex-shrink: 0;
  }
  .hdr-btn {
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
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .close-project {
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 0 2px;
    border-radius: 3px;
    flex-shrink: 0;
  }
  .close-project:hover { color: var(--foreground); background: var(--bg-hover); }

  .list { overflow-y: auto; flex: 1; padding: 4px 0; }

  .row {
    display: flex;
    align-items: center;
    gap: 5px;
    padding-top: 3px;
    padding-bottom: 3px;
    padding-right: 10px;
    cursor: pointer;
    font-size: 12px;
    user-select: none;
    white-space: nowrap;
    border-radius: 4px;
    margin: 0 4px;
    transition: background 0.08s;
  }
  .row:hover { background: var(--bg-hover); }
  .row.selected { background: var(--bg-selected); }

  .dir-arrow {
    display: flex;
    align-items: center;
    color: var(--muted);
    width: 16px;
    flex-shrink: 0;
  }
  .file-dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    flex-shrink: 0;
    margin-left: 4px;
    margin-right: 3px;
    opacity: 0.8;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: 12px;
  }
  .name.dir {
    color: var(--foreground);
    font-weight: 500;
  }

  .create-icon {
    display: flex;
    align-items: center;
    color: var(--muted);
    width: 16px;
    flex-shrink: 0;
  }

  .rename-input {
    flex: 1;
    background: var(--bg-hover);
    border: 1px solid var(--border-focused);
    border-radius: 4px;
    color: var(--foreground);
    font-size: 12px;
    padding: 1px 6px;
    outline: none;
  }
</style>
