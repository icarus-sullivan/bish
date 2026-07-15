import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete'
import type { EditorView } from '@codemirror/view'
import { GetProjectSymbols, ReadFile } from './wails'
import type { SymbolInfo } from './wails'
import type { IntelKind } from './codeintel'

const TTL = 30_000
let cache: { root: string; symbols: SymbolInfo[]; at: number } | null = null
let inflight: Promise<SymbolInfo[]> | null = null

export function invalidateSymbols() {
  if (cache) cache.at = 0
}

async function getSymbols(root: string): Promise<SymbolInfo[]> {
  if (cache?.root === root) {
    if (Date.now() - cache.at >= TTL) refresh(root) // stale-while-revalidate
    return cache.symbols
  }
  return refresh(root)
}

function refresh(root: string): Promise<SymbolInfo[]> {
  if (!inflight) {
    inflight = GetProjectSymbols(root)
      .then((syms: SymbolInfo[] | null) => {
        cache = { root, symbols: syms ?? [], at: Date.now() }
        inflight = null
        return cache.symbols
      })
      .catch(() => {
        inflight = null
        return cache?.symbols ?? []
      })
  }
  return inflight
}

function dirname(p: string) {
  return p.substring(0, p.lastIndexOf('/'))
}

// ─── tsconfig path aliases (prefer '$lib/x' / '@/x' over '../../x') ──────────

interface Alias { prefix: string; dir: string } // '$lib/' → '/abs/src/lib/'

const tsconfigCache = new Map<string, Alias[] | null>() // dir → aliases from dir/tsconfig.json
const aliasesByFileDir = new Map<string, Alias[]>()     // importing-file dir → nearest aliases
// ponytail: session-lifetime cache — tsconfig edits need a file reopen to show up

function resolvePath(base: string, rel: string): string {
  if (rel.startsWith('/')) return rel
  const parts = base.split('/')
  for (const seg of rel.split('/')) {
    if (seg === '..') parts.pop()
    else if (seg !== '.' && seg !== '') parts.push(seg)
  }
  return parts.join('/')
}

function stripJsonc(raw: string): string {
  return raw
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
    .replace(/,(\s*[}\]])/g, '$1')
}

// paths from dir/tsconfig.json, following relative `extends` (svelte-kit keeps
// $lib in the generated .svelte-kit/tsconfig.json the root config extends)
async function loadTsconfig(dir: string): Promise<Alias[] | null> {
  const hit = tsconfigCache.get(dir)
  if (hit !== undefined) return hit
  let out: Alias[] | null = null
  let cfgDir = dir
  let raw = await ReadFile(dir + '/tsconfig.json').catch(() => null)
  for (let hop = 0; raw != null && hop < 4; hop++) {
    let cfg: any
    try { cfg = JSON.parse(stripJsonc(raw)) } catch { break }
    const paths = cfg?.compilerOptions?.paths
    if (paths) {
      const base = cfg.compilerOptions.baseUrl ? resolvePath(cfgDir, cfg.compilerOptions.baseUrl) : cfgDir
      out = []
      for (const [alias, targets] of Object.entries(paths as Record<string, string[]>)) {
        if (!alias.endsWith('/*') || !targets?.[0]?.endsWith('/*')) continue // exact-match aliases skipped
        out.push({ prefix: alias.slice(0, -1), dir: resolvePath(base, targets[0].slice(0, -2)) + '/' })
      }
      break
    }
    const ext = cfg?.extends
    if (typeof ext !== 'string' || !ext.startsWith('.')) break
    const extPath = resolvePath(cfgDir, ext.endsWith('.json') ? ext : ext + '.json')
    cfgDir = dirname(extPath)
    raw = await ReadFile(extPath).catch(() => null)
  }
  tsconfigCache.set(dir, out)
  return out
}

// nearest tsconfig with paths, walking up from the importing file to root —
// handles apps nested below the opened project root
async function ensureAliases(fileDir: string, root: string) {
  if (aliasesByFileDir.has(fileDir)) return
  let found: Alias[] = []
  for (let dir = fileDir; dir; dir = dirname(dir)) {
    const a = await loadTsconfig(dir)
    if (a?.length) { found = a; break }
    if (dir === root) break
  }
  aliasesByFileDir.set(fileDir, found)
}

function aliasModule(fromFile: string, toFile: string): string | null {
  let best: Alias | null = null
  for (const a of aliasesByFileDir.get(dirname(fromFile)) ?? [])
    if (toFile.startsWith(a.dir) && (!best || a.dir.length > best.dir.length)) best = a
  return best && best.prefix + toFile.slice(best.dir.length)
}

// module specifier from the importing file to the target, ext stripped —
// tsconfig path alias when one covers the target, else relative
// ponytail: NodeNext ".js" specifiers not handled — add a toggle if it bites
function relModule(fromFile: string, toFile: string): string {
  const alias = aliasModule(fromFile, toFile)
  if (alias) return alias.replace(/\.(tsx?|jsx?|mjs|cjs)$/, '')
  const from = dirname(fromFile).split('/')
  const to = toFile.split('/')
  let i = 0
  while (i < from.length && from[i] === to[i]) i++
  const p = (i === from.length ? './' : '../'.repeat(from.length - i)) + to.slice(i).join('/')
  return p.replace(/\.(tsx?|jsx?|mjs|cjs)$/, '')
}

function pyModule(root: string, file: string): string {
  let rel = file.startsWith(root + '/') ? file.slice(root.length + 1) : file
  rel = rel.replace(/\.py$/, '').replace(/\//g, '.')
  return rel.replace(/\.__init__$/, '')
}

function detailFor(sym: SymbolInfo, filePath: string, root: string, kind: IntelKind): string {
  if (kind === 'go') return sym.pkg
  if (kind === 'py') return pyModule(root, sym.file)
  return relModule(filePath, sym.file)
}

// index of the position just after the last top-of-file import line
function afterLastImport(doc: string, re: RegExp): number {
  let pos = 0
  let last = -1
  for (const line of doc.split('\n')) {
    if (re.test(line)) last = pos + line.length
    else if (line.trim() !== '' && last !== -1) break
    pos += line.length + 1
    if (pos > 8192) break // imports live at the top
  }
  return last
}

function importChange(view: EditorView, sym: SymbolInfo, filePath: string, root: string, kind: IntelKind) {
  const doc = view.state.doc.toString()
  const head = doc.slice(0, 8192)

  if (kind === 'go') {
    if (!sym.importPath || dirname(sym.file) === dirname(filePath)) return null // same package
    if (head.includes(`"${sym.importPath}"`)) return null
    const block = head.match(/import\s*\(/)
    if (block) {
      const close = head.indexOf(')', block.index! + block[0].length)
      if (close !== -1) return { from: close, insert: `\t"${sym.importPath}"\n` }
    }
    const pkg = head.match(/^package\s+\w+.*$/m)
    if (pkg) return { from: pkg.index! + pkg[0].length, insert: `\n\nimport "${sym.importPath}"` }
    return null
  }

  if (kind === 'py') {
    const mod = pyModule(root, sym.file)
    const nameRe = new RegExp(`^(from|import)\\b.*\\b${sym.name}\\b`, 'm')
    if (nameRe.test(head)) return null
    const line = `from ${mod} import ${sym.name}`
    const at = afterLastImport(doc, /^(from|import)\s/)
    return at === -1 ? { from: 0, insert: line + '\n' } : { from: at, insert: '\n' + line }
  }

  // js/ts/svelte — svelte imports live indented inside <script>, hence \s*
  const mod = relModule(filePath, sym.file)
  const nameRe = new RegExp(`^\\s*import\\b[^;\\n]*[{,\\s]${sym.name}[,\\s}]`, 'm')
  if (nameRe.test(head)) return null
  const modRe = new RegExp(`^\\s*import\\s*\\{([^}]*)\\}\\s*from\\s*['"]${mod.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}['"]`, 'm')
  const existing = head.match(modRe)
  if (existing && sym.kind !== 'default') {
    // splice into the existing named-import braces
    const braceEnd = existing.index! + existing[0].indexOf('}')
    const trailing = existing[1].trim().endsWith(',') || existing[1].trim() === '' ? ' ' : ', '
    return { from: braceEnd, insert: `${trailing}${sym.name}` }
  }
  const line = sym.kind === 'default'
    ? `import ${sym.name} from '${mod}'`
    : `import { ${sym.name} } from '${mod}'`
  const at = afterLastImport(doc, /^\s*import\s/)
  if (at !== -1) return { from: at, insert: '\n' + line }
  if (kind === 'svelte') {
    // no imports yet: after the <script> open tag, or create the block
    const script = head.match(/<script[^>]*>/)
    if (script) return { from: script.index! + script[0].length, insert: '\n  ' + line }
    return { from: 0, insert: `<script>\n  ${line}\n</script>\n\n` }
  }
  return { from: 0, insert: line + '\n' }
}

export function autoImportSource(filePath: string, root: string, kind: IntelKind) {
  return async (ctx: CompletionContext): Promise<CompletionResult | null> => {
    // Go labels are dotted (pkg.Name), so match across dots there; for js/py a
    // preceding dot means member access — not an import target
    const word = ctx.matchBefore(kind === 'go' ? /[\w.]+/ : /[\w$]+/)
    if (!word || (word.text.length < 2 && !ctx.explicit)) return null
    if (kind !== 'go' && ctx.state.sliceDoc(word.from - 1, word.from) === '.') return null
    if (kind === 'js' || kind === 'svelte') await ensureAliases(dirname(filePath), root)
    const syms = await getSymbols(root)
    if (!syms.length) return null
    return {
      from: word.from,
      options: syms
        .filter(s => s.file !== filePath)
        .map(s => ({
          label: kind === 'go' && dirname(s.file) !== dirname(filePath) ? `${s.pkg}.${s.name}` : s.name,
          type: s.kind === 'class' || s.kind === 'type' ? 'class' : s.kind === 'func' ? 'function' : 'variable',
          detail: detailFor(s, filePath, root, kind),
          apply: (view: EditorView, _c: unknown, from: number, to: number) => {
            const label = kind === 'go' && dirname(s.file) !== dirname(filePath) ? `${s.pkg}.${s.name}` : s.name
            const imp = importChange(view, s, filePath, root, kind)
            let cursor = from + label.length
            if (imp && imp.from <= from) cursor += imp.insert.length
            view.dispatch({
              changes: [{ from, to, insert: label }, ...(imp ? [imp] : [])],
              selection: { anchor: cursor },
              scrollIntoView: true,
            })
          },
        })),
      validFor: kind === 'go' ? /^[\w.]*$/ : /^[\w$]*$/,
    }
  }
}
