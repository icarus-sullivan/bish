// Small line diff for rendering Edit/MultiEdit/Write tool calls inline.
// LCS over lines; falls back to "all removed, all added" past a size cap so
// a huge Write can't stall the UI thread.

export interface DiffRow { kind: 'ctx' | 'add' | 'del' | 'gap'; text: string; oldNo?: number; newNo?: number }

const MAX_CELLS = 2_000_000

export function lineDiff(oldText: string, newText: string, context = 3): DiffRow[] {
  const a = oldText === '' ? [] : oldText.split('\n')
  const b = newText === '' ? [] : newText.split('\n')
  const rows: DiffRow[] = []
  if (a.length * b.length > MAX_CELLS) {
    a.forEach((t, i) => rows.push({ kind: 'del', text: t, oldNo: i + 1 }))
    b.forEach((t, i) => rows.push({ kind: 'add', text: t, newNo: i + 1 }))
    return rows
  }
  // trim common prefix/suffix first — most edits touch a few lines
  let pre = 0
  while (pre < a.length && pre < b.length && a[pre] === b[pre]) pre++
  let suf = 0
  while (suf < a.length - pre && suf < b.length - pre && a[a.length - 1 - suf] === b[b.length - 1 - suf]) suf++
  const A = a.slice(pre, a.length - suf)
  const B = b.slice(pre, b.length - suf)
  const n = A.length, m = B.length
  const dp: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1))
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--)
      dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])

  const full: DiffRow[] = []
  for (let k = 0; k < pre; k++) full.push({ kind: 'ctx', text: a[k], oldNo: k + 1, newNo: k + 1 })
  let i = 0, j = 0
  while (i < n || j < m) {
    if (i < n && j < m && A[i] === B[j]) {
      full.push({ kind: 'ctx', text: A[i], oldNo: pre + i + 1, newNo: pre + j + 1 }); i++; j++
    } else if (j < m && (i >= n || dp[i][j + 1] >= dp[i + 1][j])) {
      full.push({ kind: 'add', text: B[j], newNo: pre + j + 1 }); j++
    } else {
      full.push({ kind: 'del', text: A[i], oldNo: pre + i + 1 }); i++
    }
  }
  for (let k = 0; k < suf; k++) {
    const oi = a.length - suf + k, ni = b.length - suf + k
    full.push({ kind: 'ctx', text: a[oi], oldNo: oi + 1, newNo: ni + 1 })
  }

  // collapse long unchanged runs to `context` lines around each change
  const keep = new Uint8Array(full.length)
  full.forEach((r, idx) => {
    if (r.kind === 'ctx') return
    for (let k = Math.max(0, idx - context); k <= Math.min(full.length - 1, idx + context); k++) keep[k] = 1
  })
  let gap = false
  full.forEach((r, idx) => {
    if (keep[idx]) { rows.push(r); gap = false }
    else if (!gap) { rows.push({ kind: 'gap', text: '' }); gap = true }
  })
  return rows
}

export function diffStats(rows: DiffRow[]): { add: number; del: number } {
  let add = 0, del = 0
  for (const r of rows) { if (r.kind === 'add') add++; else if (r.kind === 'del') del++ }
  return { add, del }
}

// Rows from a unified diff (the hunk text Codex reports for a file change).
// Text without hunk headers is treated as whole-file content of `kind`.
export function unifiedRows(diff: string, kind: 'add' | 'delete' | 'update' = 'update'): DiffRow[] {
  const lines = diff.replace(/\n$/, '').split('\n')
  if (!lines.some(l => l.startsWith('@@'))) {
    if (kind === 'delete') return lines.map((t, i) => ({ kind: 'del', text: t, oldNo: i + 1 }))
    return lines.map((t, i) => ({ kind: 'add', text: t.startsWith('+') && kind === 'add' && lines.every(x => x.startsWith('+') || !x) ? t.slice(1) : t, newNo: i + 1 }))
  }
  const rows: DiffRow[] = []
  let o = 0, n = 0
  for (const l of lines) {
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(l)
    if (h) {
      if (rows.length) rows.push({ kind: 'gap', text: '' })
      o = Number(h[1]); n = Number(h[2])
      continue
    }
    if (l.startsWith('---') || l.startsWith('+++') || l.startsWith('diff ') || l.startsWith('index ')) continue
    if (l.startsWith('+')) rows.push({ kind: 'add', text: l.slice(1), newNo: n++ })
    else if (l.startsWith('-')) rows.push({ kind: 'del', text: l.slice(1), oldNo: o++ })
    else if (l.startsWith('\\')) continue
    else rows.push({ kind: 'ctx', text: l.slice(1), oldNo: o++, newNo: n++ })
  }
  return rows
}
