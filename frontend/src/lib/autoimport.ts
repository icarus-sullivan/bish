import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete'
import type { EditorView } from '@codemirror/view'
import { GetProjectSymbols } from './wails'
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

// relative module specifier from the importing file to the target, ext stripped
// ponytail: NodeNext ".js" specifiers not handled — add a toggle if it bites
function relModule(fromFile: string, toFile: string): string {
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
