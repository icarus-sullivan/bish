// Subsequence fuzzy matcher shared by the file finder and command palette.
export interface FuzzyResult { score: number; indices: number[] }

export function fuzzyMatch(query: string, target: string): FuzzyResult | null {
  const q = query.toLowerCase()
  const p = target.toLowerCase()
  if (q.length === 0) return { score: 0, indices: [] }

  const indices: number[] = []
  let pi = 0
  for (let qi = 0; qi < q.length; qi++) {
    const found = p.indexOf(q[qi], pi)
    if (found === -1) return null
    indices.push(found)
    pi = found + 1
  }

  let score = 0, run = 1
  for (let i = 1; i < indices.length; i++) {
    if (indices[i] === indices[i - 1] + 1) { run++; score += 10 * run } else run = 1
  }
  const first = indices[0]
  if (first === 0 || target[first - 1] === ' ' || target[first - 1] === '/') score += 50
  if (p.includes(q)) score += 100
  return { score, indices }
}
