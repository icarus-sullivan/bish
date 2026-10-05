// Per-tab editor buffers that outlive FileViewer. Only the active tab has a
// mounted FileViewer (App.svelte), so without this every tab switch destroyed
// the EditorView and re-read the file from disk — silently dropping unsaved
// edits (the tab kept its modified dot), undo history, cursor and scroll.
//
// States are kept as JSON rather than live EditorState objects: a live state
// carries extension instances bound to the old view (LSP compartments holding
// a client, update listeners closing over the destroyed component), while
// fromJSON rebuilds doc + selection + undo history + folds under the new
// view's fresh extensions.
import type { EditorState, StateEffect } from '@codemirror/state'
import { get } from 'svelte/store'
import { historyField } from '@codemirror/commands'
import { foldState } from '@codemirror/language'
import { tabs } from './stores'

export const serializedFields = { history: historyField, folds: foldState }

export interface CachedEditor {
  path: string
  json: any
  scroll: StateEffect<unknown>  // view.scrollSnapshot() — position-anchored, survives remount
  mtime: number      // disk mtime the buffer was loaded/saved against (0 = unknown)
  modified: boolean
  warnedMtime?: number  // external change already announced for this buffer
}

const cache = new Map<string, CachedEditor>()

export function stashEditor(tabId: string, entry: CachedEditor) {
  // a closing tab unmounts after it left the store — never keep its buffer,
  // or reopening the same file (same 'file:<path>' id) would resurrect the
  // edits the user just discarded
  if (!get(tabs).some(t => t.id === tabId)) return
  cache.set(tabId, entry)
}

export function takeEditor(tabId: string, path: string): CachedEditor | null {
  const e = cache.get(tabId)
  cache.delete(tabId)
  return e && e.path === path ? e : null
}

export function serializeState(state: EditorState): any {
  return state.toJSON(serializedFields)
}

// closed tabs drop their buffer (the close itself already confirmed discard)
tabs.subscribe(ts => {
  if (cache.size === 0) return
  const live = new Set(ts.map(t => t.id))
  for (const id of cache.keys()) if (!live.has(id)) cache.delete(id)
})
