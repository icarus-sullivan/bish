// Go test run state. Running a test reuses the exact same process-manager
// mechanism as Task Runner (RunGoTest → a.mgr.Add on the Go side) — so
// pass/fail here is just watching the shared `processes` store for the
// spawned process's status to resolve, not a parallel tracking system.
import { writable, get } from 'svelte/store'
import { RunGoTest } from './wails'
import { processes } from './stores'

export type TestStatus = 'idle' | 'running' | 'passed' | 'failed'

export function testKey(pkg: string, name: string): string {
  return pkg + '\0' + name
}

export const testStatus = writable<Map<string, TestStatus>>(new Map())

function setStatus(k: string, s: TestStatus) {
  testStatus.update(m => {
    const next = new Map(m)
    next.set(k, s)
    return next
  })
}

// processId -> test key, for runs we started and are still waiting to resolve
const pending = new Map<string, string>()

processes.subscribe(procs => {
  if (pending.size === 0) return
  for (const p of procs) {
    const k = pending.get(p.id)
    if (!k) continue
    if (p.status === 'running') {
      setStatus(k, 'running')
    } else if (p.status === 'stopped' || p.status === 'crashed') {
      setStatus(k, p.status === 'stopped' && p.exit_code === 0 ? 'passed' : 'failed')
      pending.delete(p.id)
    }
  }
})

export async function runGoTest(pkg: string, name: string): Promise<void> {
  const k = testKey(pkg, name)
  setStatus(k, 'running')
  try {
    const id = await RunGoTest(pkg, name)
    pending.set(id, k)
  } catch {
    setStatus(k, 'failed')
  }
}

export function statusFor(pkg: string, name: string): TestStatus {
  return get(testStatus).get(testKey(pkg, name)) ?? 'idle'
}
