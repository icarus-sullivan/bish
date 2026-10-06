import { marked } from 'marked'
import DOMPurify from 'dompurify'

// Markdown → sanitized HTML. Images are dropped (a remote <img> in model
// output would fetch from an arbitrary host the moment it renders), and
// every link is neutralized to a data attribute the panel's click handler
// routes itself — http(s) to the system browser, project paths to the editor.
const PURIFY_CFG = {
  FORBID_TAGS: ['img', 'style', 'form', 'input', 'button', 'iframe', 'object', 'embed', 'video', 'audio', 'svg', 'math'],
  FORBID_ATTR: ['style', 'srcset', 'action', 'formaction'],
}

// own instance so the link-rewriting hook never leaks into other
// DOMPurify users (markdown preview etc.)
let purifier: ReturnType<typeof DOMPurify> | null = null
function ensureHook() {
  if (purifier) return purifier
  purifier = DOMPurify(window)
  purifier.addHook('afterSanitizeAttributes', (node: Element) => {
    if (node.tagName === 'A') {
      const href = node.getAttribute('href') ?? ''
      node.removeAttribute('href')
      node.removeAttribute('target')
      if (href) node.setAttribute('data-href', href)
      node.setAttribute('role', 'link')
      node.setAttribute('tabindex', '0')
    }
  })
  return purifier
}

export function renderMarkdown(md: string): string {
  const p = ensureHook()
  const html = marked.parse(md, { async: false, gfm: true, breaks: false }) as string
  return p.sanitize(html, PURIFY_CFG) as unknown as string
}

// ESC [ … final-byte, OSC … BEL/ST, and lone control chars — tool output
// and decision_reason text can carry terminal styling.
const ANSI = /\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]|[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g
export function stripAnsi(s: string): string {
  return s.replace(ANSI, '')
}

export function basename(p: string): string {
  return p.split('/').filter(Boolean).pop() ?? p
}

export function relPath(p: string, root: string): string {
  if (root && p.startsWith(root + '/')) return p.slice(root.length + 1)
  return p
}

export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k'
  return String(n)
}

export function fmtCost(usd: number): string {
  if (!usd) return '$0.00'
  return usd < 0.01 ? '<$0.01' : '$' + usd.toFixed(2)
}

export function fmtDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${Math.round(s % 60)}s`
}

export function timeAgo(ms: number): string {
  const d = (Date.now() - ms) / 1000
  if (d < 60) return 'just now'
  if (d < 3600) return `${Math.floor(d / 60)}m ago`
  if (d < 86400) return `${Math.floor(d / 3600)}h ago`
  if (d < 86400 * 7) return `${Math.floor(d / 86400)}d ago`
  return new Date(ms).toLocaleDateString()
}

// Tool-result content is either a string or a list of text/image blocks.
export function resultText(content: unknown): string {
  if (typeof content === 'string') return stripAnsi(content)
  if (Array.isArray(content)) {
    return content
      .map((b: any) => b?.type === 'text' ? b.text : b?.type === 'image' ? '[image]' : '')
      .filter(Boolean).join('\n')
      .replace(ANSI, '')
  }
  return ''
}

// A one-line human summary of what a tool call does, for the collapsed card.
export function toolSummary(name: string, input: any, root: string): string {
  if (!input || typeof input !== 'object') return ''
  const p = (v: any) => typeof v === 'string' ? relPath(v, root) : ''
  switch (name) {
    case 'Read': {
      const range = input.offset ? ` (from line ${input.offset}${input.limit ? `, ${input.limit} lines` : ''})` : ''
      return p(input.file_path) + range
    }
    case 'Write': case 'Edit': case 'MultiEdit': case 'NotebookEdit':
      return p(input.file_path ?? input.notebook_path)
    case 'Bash': return input.description || input.command || ''
    case 'BashOutput': case 'KillShell': case 'KillBash': return input.bash_id ?? input.shell_id ?? ''
    case 'Glob': return input.pattern + (input.path ? ` in ${p(input.path)}` : '')
    case 'Grep': return `"${input.pattern}"` + (input.path ? ` in ${p(input.path)}` : '') + (input.glob ? ` (${input.glob})` : '')
    case 'WebFetch': return input.url ?? ''
    case 'WebSearch': return input.query ?? ''
    case 'Task': case 'Agent': return input.description ?? ''
    case 'TodoWrite': {
      const t = Array.isArray(input.todos) ? input.todos : []
      const done = t.filter((x: any) => x.status === 'completed').length
      return `${done}/${t.length} done`
    }
    case 'Skill': return input.skill ?? input.command ?? ''
    case 'apply_patch': {
      const ch = Array.isArray(input.changes) ? input.changes : []
      return ch.length === 1 ? relPath(ch[0].path, root) : `${ch.length} files`
    }
    case 'ToolSearch': return input.query ?? ''
    default: {
      const first = Object.values(input).find(v => typeof v === 'string') as string | undefined
      return first ? first.slice(0, 120) : ''
    }
  }
}

// "mcp__server__tool" → { server, tool }
export function mcpParts(name: string): { server: string; tool: string } | null {
  const m = /^mcp__(.+?)__(.+)$/.exec(name)
  return m ? { server: m[1], tool: m[2] } : null
}

export function toolLabel(name: string): string {
  const mcp = mcpParts(name)
  if (mcp) return `${mcp.server}: ${mcp.tool}`
  if (name === 'Task' || name === 'Agent') return 'Agent'
  if (name === 'apply_patch') return 'Edit'
  return name
}

export const FILE_EDIT_TOOLS = new Set(['Write', 'Edit', 'MultiEdit', 'NotebookEdit', 'write_file'])

// Human description of a CLI-provided permission suggestion — the
// "don't ask again for …" label.
export function describeSuggestion(s: any): string {
  if (!s || typeof s !== 'object') return ''
  if (typeof s.label === 'string') return s.label
  const where = s.destination === 'session' ? 'this session'
    : s.destination === 'localSettings' ? 'this project (local)'
    : s.destination === 'projectSettings' ? 'this project'
    : s.destination === 'userSettings' ? 'all projects' : s.destination
  switch (s.type) {
    case 'addRules': {
      const rules = (s.rules ?? []).map((r: any) => r.ruleContent ? `${r.toolName}(${r.ruleContent})` : r.toolName).join(', ')
      return `Always ${s.behavior ?? 'allow'} ${rules} in ${where}`
    }
    case 'setMode': return `Switch to ${s.mode} for ${where}`
    case 'addDirectories': return `Allow access to ${(s.directories ?? []).join(', ')} in ${where}`
    default: return `${s.type} (${where})`
  }
}
