// Debugger (DAP) client-side state. All DAP protocol handling — handshake,
// stepping, stack/variable resolution — happens in Go (internal/dap); this
// module just mirrors the resolved state and owns the one piece of state
// that's naturally UI-side: which lines have breakpoints.
import { writable, get } from 'svelte/store'
import {
  DebugStart, DebugSetBreakpoints, DebugContinue, DebugStepOver,
  DebugStepIn, DebugStepOut, DebugStop, on,
} from './wails'
import { projectRoot, cwd } from './stores'

export interface DebugFrame { id: number; name: string; path: string; line: number }
export interface DebugVariable { name: string; value: string; type: string }
export interface DebugState {
  status: 'idle' | 'starting' | 'running' | 'paused' | 'terminated' | 'error'
  frames: DebugFrame[]
  variables: DebugVariable[]
  error?: string
}

export const debugState = writable<DebugState>({ status: 'idle', frames: [], variables: [] })
export const debugOutput = writable<string[]>([])
export const breakpoints = writable<Map<string, Set<number>>>(new Map())

on('dap:state', (s: DebugState) => debugState.set(s))
on('dap:output', (line: string) => debugOutput.update(lines => [...lines.slice(-499), line]))

export function toggleBreakpoint(path: string, line: number) {
  breakpoints.update(m => {
    const next = new Map(m)
    const set = new Set(next.get(path) ?? [])
    if (set.has(line)) set.delete(line)
    else set.add(line)
    if (set.size === 0) next.delete(path)
    else next.set(path, set)
    return next
  })
  const status = get(debugState).status
  if (status === 'running' || status === 'paused') {
    DebugSetBreakpoints(path, [...(get(breakpoints).get(path) ?? [])]).catch(() => {})
  }
}

export async function startDebugging(): Promise<void> {
  const root = get(projectRoot) || get(cwd)
  debugOutput.set([])
  debugState.set({ status: 'starting', frames: [], variables: [] })
  const bps: Record<string, number[]> = {}
  for (const [path, lines] of get(breakpoints)) bps[path] = [...lines]
  try {
    await DebugStart(root, bps)
    debugState.set({ status: 'running', frames: [], variables: [] })
  } catch (e: any) {
    debugState.set({ status: 'error', frames: [], variables: [], error: String(e?.message ?? e) })
  }
}

export function continueExec() { DebugContinue().catch(() => {}) }
export function stepOver() { DebugStepOver().catch(() => {}) }
export function stepIn() { DebugStepIn().catch(() => {}) }
export function stepOut() { DebugStepOut().catch(() => {}) }
export function stopDebugging() {
  DebugStop().catch(() => {})
  debugState.set({ status: 'idle', frames: [], variables: [] })
}
