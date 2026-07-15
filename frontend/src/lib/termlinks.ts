// ⌘/⇧-click link support for the terminal: OSC 8 hyperlinks (linkHandler)
// plus regex-detected URLs and file paths (path[:line[:col]]) in plain output.
// Plain click stays selection-only; a modifier is required to follow.
import type { Terminal, ILinkProvider, ILink } from '@xterm/xterm'
import { get } from 'svelte/store'
import { cwd, openFileTab, pendingGoto, isMediaPath } from './stores'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const withMod = (e: MouseEvent) => e.metaKey || e.shiftKey

// OSC 8 hyperlinks emitted by programs themselves (ls --hyperlink, gh, …)
export const terminalLinkHandler = {
  activate(e: MouseEvent, uri: string) { if (withMod(e)) follow(uri) },
  allowNonHttpProtocols: true,
}

function follow(text: string) {
  if (/^https?:\/\//i.test(text)) { BrowserOpenURL(text); return }
  if (text.startsWith('file://')) {
    text = decodeURIComponent(text.slice(7))
    if (!text.startsWith('/')) text = text.slice(text.indexOf('/')) // drop hostname
  }
  const m = /^(.+?)(?::(\d+))?(?::(\d+))?$/.exec(text)
  if (!m) return
  let path = m[1]
  if (path.startsWith('~')) return // ponytail: no home-dir expansion in frontend
  if (!path.startsWith('/')) {
    const base = get(cwd)
    if (!base) return
    path = base.replace(/\/+$/, '') + '/' + path.replace(/^\.\//, '')
  }
  openFileTab(path)
  if (m[2] && !isMediaPath(path)) pendingGoto.set({ path, line: +m[2], col: m[3] ? +m[3] : 0 })
}

const URL_RE = /https?:\/\/[^\s"'`<>()\[\]]+/g
// token with ≥1 internal slash (src/a.ts, /abs/x, ./x, ~/x), optional :line:col;
// lookbehind keeps it off the middle of URLs and other words
const PATH_RE = /(?<=^|[\s"'`<>()\[\]])~?[\w.@+-]*(?:\/[\w.@+-]+)+\/?(?::\d+(?::\d+)?)?/g

export function fileLinkProvider(term: Terminal): ILinkProvider {
  return {
    provideLinks(bufferLine: number, cb: (links: ILink[] | undefined) => void) {
      const buf = term.buffer.active
      // stitch wrapped rows into the logical line; untrimmed rows are exactly
      // `cols` chars wide, so string index ↔ buffer cell math stays exact
      // ponytail: wide (CJK) chars occupy 2 cells but 1 char — underline drifts there
      let first = bufferLine - 1
      while (first > 0 && buf.getLine(first)?.isWrapped) first--
      let text = ''
      for (let y = first; ; y++) {
        const line = buf.getLine(y)
        if (!line) break
        text += line.translateToString(false)
        if (!buf.getLine(y + 1)?.isWrapped) break
      }
      const cols = term.cols
      const pos = (i: number) => ({ x: (i % cols) + 1, y: first + Math.floor(i / cols) + 1 })
      const links: ILink[] = []
      for (const re of [URL_RE, PATH_RE]) {
        re.lastIndex = 0
        for (let m; (m = re.exec(text)); ) {
          const raw = m[0].replace(/[.,;]+$/, '') // sentence punctuation isn't part of the link
          if (!raw) continue
          links.push({
            text: raw,
            range: { start: pos(m.index), end: pos(m.index + raw.length - 1) },
            activate: (e, t) => { if (withMod(e)) follow(t) },
          })
        }
      }
      cb(links.length ? links : undefined)
    },
  }
}
