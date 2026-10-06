// bish Linear extension — see your assigned issues from inside the IDE.
// Copy this folder to ~/.bish/extensions/linear to try it.
//
// Runs in a Web Worker: no DOM/filesystem of its own, but fetch() IS
// available in a worker, so this talks to Linear's GraphQL API directly.
// The bish backend (Go) is never involved — no server-side OAuth flow, no
// bish-side Linear code at all. Token storage and panel input are relayed
// through the extension host (getSecret/setSecret/input messages) purely
// because a worker has no localStorage or DOM of its own to hold them.
//
// One-time setup (personal API key, no OAuth redirect needed):
//   1. https://linear.app/settings/account/security -> Personal API keys
//      -> New API key. Copy it (starts lin_api_...).
//   2. Run "Linear: Connect" from the Command Palette (Cmd+Shift+P) and
//      paste the key when the panel asks.
//
// Messages exchanged with the host, beyond the base extension protocol:
//   worker -> host  { type: 'getSecret', reqId, key }     (reply carries value)
//   worker -> host  { type: 'setSecret', key, value }     (fire-and-forget)
//   worker -> host  { type: 'setPanelSelect', panelId, options, value }
//                     (adds/updates the status dropdown next to the panel input)
//   host -> worker  { type: 'input', panelId, value }     (title filter box submit)
//   host -> worker  { type: 'select', panelId, value }    (status dropdown change)
//   host -> worker  { type: 'click', panelId, action, value }
//                     (click on a data-action element: "open" an issue by
//                      identifier, or "back" to the list)
//
// Clicking an issue renders a Linear-style detail view (properties,
// description, sub-issues, comments) inside the panel — it never navigates
// the webview. Any <a href> in the panel (the external-link icon, links in
// descriptions/comments) is opened in the user's default browser by the host.
//
// The panel's refresh icon just re-sends the manifest's "refresh" command
// (same as running it from the Command Palette) — no extra protocol needed.

let reqCounter = 0
const pending = new Map()

function ask(type, extra = {}) {
  return new Promise(resolve => {
    const reqId = ++reqCounter
    pending.set(reqId, resolve)
    postMessage({ type, reqId, ...extra })
  })
}
const getSecret = key => ask('getSecret', { key })
const setSecret = (key, value) => postMessage({ type: 'setSecret', key, value })

function esc(s) {
  return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

function render(html) {
  postMessage({ type: 'setPanelHTML', panelId: 'issues', html })
}

const ACTIVE = '__active__'
const ALL = '__all__'

const state = {
  apiKey: null,
  stage: 'boot',      // boot -> needKey -> loading -> ready
  issues: [],         // { identifier, title, url, stateName, stateType, stateColor }
  titleQuery: '',     // free-text filter, matched against identifier + title
  statusFilter: ACTIVE, // ACTIVE (hide completed) | ALL | an exact state name
  pollTimer: null,
  view: 'list',       // list | detail
  detailId: null,     // identifier of the issue shown in the detail view
  detail: null,       // fetched issue (ISSUE_QUERY shape), null while loading
  detailError: '',
  detailReq: 0,       // bumps per fetch so a stale response can't overwrite a newer one
}

// Tabler icons, inlined — the worker can't import the app's icon components.
const ICON_EXTERNAL = `<svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 6h-6a2 2 0 0 0 -2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2 -2v-6"/><path d="M11 13l9 -9"/><path d="M15 4h5v5"/></svg>`
const ICON_BACK = `<svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12l14 0"/><path d="M5 12l6 6"/><path d="M5 12l6 -6"/></svg>`

const BORDER = 'border-bottom:1px solid var(--border)'
const MUTED = 'color:var(--muted)'

function externalLink(url) {
  return `<a href="${esc(url)}" title="Open in browser" style="display:inline-flex;align-items:center;padding:3px 4px;border-radius:3px;${MUTED}">${ICON_EXTERNAL}</a>`
}

function stateDot(color) {
  return `<span style="display:inline-block;flex-shrink:0;width:8px;height:8px;border-radius:50%;background:${esc(color || '#888')}"></span>`
}

// Initials avatar with a stable per-name hue — Linear avatar images aren't
// worth a network round-trip per comment in a narrow side panel.
function avatar(name, size = 18) {
  const n = name || '?'
  const initials = n.split(/\s+/).filter(Boolean).slice(0, 2).map(w => w[0]).join('').toUpperCase() || '?'
  let h = 0
  for (const c of n) h = (h * 31 + c.charCodeAt(0)) % 360
  return `<span style="display:inline-flex;align-items:center;justify-content:center;flex-shrink:0;width:${size}px;height:${size}px;border-radius:50%;background:hsl(${h},45%,45%);color:#fff;font-size:${Math.round(size * 0.45)}px;font-weight:600">${esc(initials)}</span>`
}

function ago(iso) {
  const t = Date.parse(iso)
  if (!t) return ''
  const m = Math.floor((Date.now() - t) / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  return new Date(t).toLocaleDateString()
}

// --- Minimal markdown -> HTML (Linear descriptions/comments are markdown) ---
// Input is escaped first, so every tag below is one we emit ourselves; the
// host still runs DOMPurify over the result.

function inline(s) {
  // Finished fragments (code spans, links) are stashed behind placeholders so
  // the emphasis passes below can't mangle them (e.g. underscores in URLs).
  const stash = []
  const keep = html => { stash.push(html); return `\u0000${stash.length - 1}\u0000` }
  const link = (u, text) => keep(`<a href="${u}" style="color:var(--accent)">${text}</a>`)
  s = s.replace(/`([^`]+)`/g, (_, c) =>
    keep(`<code style="font-family:var(--font-mono,monospace);font-size:11px;background:var(--bg-raised);padding:1px 4px;border-radius:3px">${c}</code>`))
  // Linear upload URLs need auth, so images become links instead of <img>.
  s = s.replace(/!\[([^\]]*)\]\((https?:[^)\s]+)\)/g, (_, alt, u) => link(u, `[image${alt ? `: ${alt}` : ''}]`))
  s = s.replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, (_, text, u) => link(u, text))
  s = s.replace(/(^|[\s(])(https?:\/\/[^\s<)]+)/g, (_, pre, u) => pre + link(u, u))
  s = s.replace(/\*\*(.+?)\*\*/g, '<b>$1</b>')
  s = s.replace(/__(.+?)__/g, '<b>$1</b>')
  s = s.replace(/(^|[^*\w])\*([^*\n]+)\*(?!\w)/g, '$1<i>$2</i>')
  s = s.replace(/(^|[^_\w])_([^_\n]+)_(?!\w)/g, '$1<i>$2</i>')
  s = s.replace(/~~(.+?)~~/g, '<s>$1</s>')
  // loop: a stashed link's text can itself hold a stashed code span
  while (/\u0000\d+\u0000/.test(s)) s = s.replace(/\u0000(\d+)\u0000/g, (_, i) => stash[i])
  return s
}

const LIST_RE = /^(\s*)([-*+]|\d+[.)])\s+/
const isBlockStart = l => /^```/.test(l) || /^#{1,6}\s/.test(l) || LIST_RE.test(l) || /^&gt;/.test(l) || /^(-{3,}|\*{3,})\s*$/.test(l)

function md(src) {
  if (!src) return ''
  const lines = esc(src).replace(/\r/g, '').split('\n')
  let out = ''
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    let m
    if (/^```/.test(line)) {
      const buf = []
      i++
      while (i < lines.length && !/^```/.test(lines[i])) buf.push(lines[i++])
      i++
      out += `<pre style="background:var(--bg-raised);border:1px solid var(--border);border-radius:4px;padding:8px;margin:6px 0;overflow-x:auto;font-size:11px"><code style="font-family:var(--font-mono,monospace)">${buf.join('\n')}</code></pre>`
      continue
    }
    if ((m = line.match(/^(#{1,6})\s+(.*)/))) {
      const size = [15, 14, 13, 12, 12, 12][m[1].length - 1]
      out += `<div style="font-weight:600;font-size:${size}px;margin:10px 0 4px">${inline(m[2])}</div>`
      i++
      continue
    }
    if (LIST_RE.test(line)) {
      const ordered = /^\s*\d/.test(line)
      const items = []
      while (i < lines.length && LIST_RE.test(lines[i])) {
        const indent = lines[i].match(LIST_RE)[1].length
        let t = lines[i].replace(LIST_RE, '')
        const cb = t.match(/^\[([ xX])\]\s+/)
        if (cb) t = (cb[1] === ' ' ? '☐ ' : '☑ ') + t.slice(cb[0].length)
        items.push(`<li style="margin:2px 0 2px ${indent * 6}px">${inline(t)}</li>`)
        i++
      }
      const tag = ordered ? 'ol' : 'ul'
      out += `<${tag} style="margin:4px 0;padding-left:18px">${items.join('')}</${tag}>`
      continue
    }
    if (/^&gt;/.test(line)) {
      const buf = []
      while (i < lines.length && /^&gt;/.test(lines[i])) buf.push(inline(lines[i++].replace(/^&gt;\s?/, '')))
      out += `<div style="border-left:3px solid var(--border);padding-left:8px;margin:6px 0;${MUTED}">${buf.join('<br>')}</div>`
      continue
    }
    if (/^(-{3,}|\*{3,})\s*$/.test(line)) {
      out += '<hr style="border:none;border-top:1px solid var(--border);margin:8px 0">'
      i++
      continue
    }
    if (!line.trim()) { i++; continue }
    const buf = []
    while (i < lines.length && lines[i].trim() && !isBlockStart(lines[i])) buf.push(inline(lines[i++]))
    out += `<p style="margin:4px 0;line-height:1.5">${buf.join('<br>')}</p>`
  }
  return out
}

// Subsequence fuzzy match, case-insensitive — same algorithm as a command
// palette: every character of query must appear in text, in order, not
// necessarily contiguous.
function fuzzyMatch(query, text) {
  let qi = 0
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  for (let ti = 0; ti < t.length && qi < q.length; ti++) if (t[ti] === q[qi]) qi++
  return qi === q.length
}

// Status dropdown options: two fixed entries plus every distinct status
// name actually seen on the account's issues, discovered from Linear's
// response rather than hardcoded (workspaces customize workflow states).
function statusOptions() {
  const names = [...new Set(state.issues.map(i => i.stateName).filter(Boolean))].sort()
  return [
    { value: ACTIVE, label: 'Active' },
    { value: ALL, label: 'All statuses' },
    ...names.map(n => ({ value: n, label: n })),
  ]
}

function pushSelect() {
  postMessage({ type: 'setPanelSelect', panelId: 'issues', options: statusOptions(), value: state.statusFilter })
}

function visibleIssues() {
  let list = state.issues
  if (state.statusFilter === ACTIVE) list = list.filter(i => i.stateType !== 'completed')
  else if (state.statusFilter !== ALL) list = list.filter(i => i.stateName === state.statusFilter)
  if (state.titleQuery) list = list.filter(i => fuzzyMatch(state.titleQuery, `${i.identifier} ${i.title}`))
  return list
}

function draw(errorLine) {
  if (state.stage === 'needKey') {
    render('<div style="padding:8px 12px">Paste your Linear <b>Personal API key</b> (lin_api_...) below and press enter.</div>')
    return
  }
  if (state.view === 'detail') { drawDetail(); return }
  if (state.stage === 'loading') {
    render('<div style="padding:8px 12px">Loading issues…</div>')
    return
  }

  const header = `<div style="padding:6px 12px;font-size:11px;${MUTED};${BORDER}">
    ${state.titleQuery ? `filtering by "${esc(state.titleQuery)}"` : 'type below to filter by ticket title — use the dropdown to filter by status'}
  </div>`
  const rows = visibleIssues().map(i => `
    <div class="ext-row" data-action="open" data-value="${esc(i.identifier)}" style="display:flex;align-items:center;gap:6px;padding:6px 8px 6px 12px;${BORDER}">
      <span style="${MUTED};flex-shrink:0">${esc(i.identifier)}</span>
      ${stateDot(i.stateColor)}
      <span style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(i.title)}</span>
      ${externalLink(i.url)}
    </div>
  `).join('')
  const err = errorLine ? `<div style="padding:0 12px 8px;color:#e5484d">${esc(errorLine)}</div>` : ''
  render(`<div>${header}${rows || '<div style="padding:8px 12px"><i>no matching issues</i></div>'}</div>${err}`)
}

function prop(label, value) {
  if (!value) return ''
  return `<div style="${MUTED};padding:3px 0">${label}</div><div style="display:flex;align-items:center;flex-wrap:wrap;gap:6px;padding:3px 0;min-width:0">${value}</div>`
}

function drawDetail() {
  const d = state.detail
  const topBar = `<div style="display:flex;align-items:center;gap:6px;padding:4px 8px;${BORDER};position:sticky;top:0;background:var(--background);z-index:1">
    <span data-action="back" title="Back to issues" style="display:inline-flex;align-items:center;gap:4px;padding:3px 4px;border-radius:3px;${MUTED}">${ICON_BACK}<span>Issues</span></span>
    <span style="${MUTED}">/</span>
    <span style="flex:1;${MUTED}">${esc(state.detailId)}</span>
    ${d ? externalLink(d.url) : ''}
  </div>`

  if (state.detailError) {
    render(`${topBar}<div style="padding:8px 12px;color:#e5484d">${esc(state.detailError)}</div>`)
    return
  }
  if (!d) {
    render(`${topBar}<div style="padding:8px 12px">Loading ${esc(state.detailId)}…</div>`)
    return
  }

  const labels = (d.labels?.nodes || []).map(l =>
    `<span style="display:inline-flex;align-items:center;gap:4px;border:1px solid var(--border);border-radius:10px;padding:0 7px;font-size:11px">${stateDot(l.color)}${esc(l.name)}</span>`).join('')
  const person = p => p ? `${avatar(p.displayName)}<span>${esc(p.displayName)}</span>` : ''
  const props = `<div style="display:grid;grid-template-columns:max-content 1fr;column-gap:14px;padding:8px 12px;${BORDER}">
    ${prop('Status', d.state ? `${stateDot(d.state.color)}<span>${esc(d.state.name)}</span>` : '')}
    ${prop('Priority', d.priority ? esc(d.priorityLabel) : '')}
    ${prop('Assignee', person(d.assignee) || `<span style="${MUTED}">Unassigned</span>`)}
    ${prop('Labels', labels)}
    ${prop('Project', d.project ? esc(d.project.name) : '')}
    ${prop('Cycle', d.cycle ? esc(d.cycle.name || `Cycle ${d.cycle.number}`) : '')}
    ${prop('Team', d.team ? esc(d.team.name) : '')}
    ${prop('Estimate', d.estimate != null ? esc(d.estimate) : '')}
    ${prop('Due', d.dueDate ? esc(new Date(d.dueDate + 'T00:00:00').toLocaleDateString()) : '')}
    ${prop('Parent', d.parent ? `<span data-action="open" data-value="${esc(d.parent.identifier)}" style="color:var(--accent)">${esc(d.parent.identifier)} ${esc(d.parent.title)}</span>` : '')}
    ${prop('Created', `${person(d.creator)}<span style="${MUTED}">${esc(ago(d.createdAt))}</span>`)}
  </div>`

  const description = `<div style="padding:8px 12px;${BORDER}">
    ${md(d.description) || `<span style="${MUTED}"><i>No description</i></span>`}
  </div>`

  const children = d.children?.nodes || []
  const subIssues = children.length ? `<div style="${BORDER}">
    <div style="padding:8px 12px 4px;font-weight:600">Sub-issues <span style="${MUTED};font-weight:400">${children.length}</span></div>
    ${children.map(c => `<div class="ext-row" data-action="open" data-value="${esc(c.identifier)}" style="display:flex;align-items:center;gap:6px;padding:4px 12px">
      ${stateDot(c.state?.color)}<span style="${MUTED};flex-shrink:0">${esc(c.identifier)}</span>
      <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(c.title)}</span>
    </div>`).join('')}
    <div style="height:4px"></div>
  </div>` : ''

  render(`${topBar}
    <div style="padding:10px 12px 6px;font-size:15px;font-weight:600;line-height:1.35">${esc(d.title)}</div>
    ${props}${description}${subIssues}${drawComments(d.comments?.nodes || [])}`)
}

function drawComment(c, replies) {
  const name = c.user?.displayName || 'Unknown'
  return `<div style="border:1px solid var(--border);border-radius:6px;margin:0 12px 8px;background:var(--bg-raised)">
    <div style="padding:8px 10px">
      <div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">
        ${avatar(name)}<b>${esc(name)}</b>
        <span style="${MUTED}">${esc(ago(c.createdAt))}${c.editedAt ? ' (edited)' : ''}</span>
      </div>
      ${md(c.body)}
    </div>
    ${replies.map(r => `<div style="border-top:1px solid var(--border);padding:8px 10px 8px 34px">
      <div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">
        ${avatar(r.user?.displayName || 'Unknown', 16)}<b>${esc(r.user?.displayName || 'Unknown')}</b>
        <span style="${MUTED}">${esc(ago(r.createdAt))}${r.editedAt ? ' (edited)' : ''}</span>
      </div>
      ${md(r.body)}
    </div>`).join('')}
  </div>`
}

function drawComments(nodes) {
  const byTime = (a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt)
  const ids = new Set(nodes.map(c => c.id))
  // A reply whose parent isn't in this page is shown as top-level so it
  // doesn't silently vanish.
  const top = nodes.filter(c => !c.parent || !ids.has(c.parent.id)).sort(byTime)
  const replies = new Map()
  for (const c of nodes) {
    if (c.parent && ids.has(c.parent.id)) replies.set(c.parent.id, [...(replies.get(c.parent.id) || []), c])
  }
  return `<div style="padding:8px 0 4px">
    <div style="padding:0 12px 8px;font-weight:600">Comments <span style="${MUTED};font-weight:400">${nodes.length}</span></div>
    ${top.map(c => drawComment(c, (replies.get(c.id) || []).sort(byTime))).join('') || `<div style="padding:0 12px 8px;${MUTED}"><i>No comments yet</i></div>`}
  </div>`
}

async function linearFetch(query, variables) {
  const res = await fetch('https://api.linear.app/graphql', {
    method: 'POST',
    headers: {
      // Linear's API takes the raw key here — no "Bearer " prefix.
      Authorization: state.apiKey,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ query, variables }),
  })
  return res.json()
}

const ISSUES_QUERY = `
  query {
    viewer {
      assignedIssues(first: 100) {
        nodes { identifier title url state { name type color } }
      }
    }
  }
`

// issue(id:) accepts either the UUID or the human identifier (ENG-123).
const ISSUE_QUERY = `
  query($id: String!) {
    issue(id: $id) {
      identifier title url description priority priorityLabel estimate dueDate createdAt
      state { name type color }
      assignee { displayName }
      creator { displayName }
      team { name }
      project { name }
      cycle { number name }
      labels { nodes { name color } }
      parent { identifier title }
      children { nodes { identifier title state { color } } }
      comments(first: 100) { nodes { id body createdAt editedAt user { displayName } parent { id } } }
    }
  }
`

// silent: background poll — keep current view, don't flash "Loading…".
async function loadIssues(silent = false) {
  if (!silent) {
    state.stage = 'loading'
    draw()
  }
  const r = await linearFetch(ISSUES_QUERY).catch(() => null)
  if (!r || r.errors) {
    state.stage = 'ready'
    if (state.view === 'list') draw(`fetch failed: ${r?.errors?.[0]?.message || 'network error'}`)
    return
  }
  state.issues = (r.data?.viewer?.assignedIssues?.nodes || []).map(n => ({
    identifier: n.identifier, title: n.title, url: n.url,
    stateName: n.state?.name, stateType: n.state?.type, stateColor: n.state?.color,
  }))
  state.stage = 'ready'
  pushSelect()
  if (state.view === 'list') draw()
}

// silent: refetch the open issue in place (poll/refresh) without blanking it.
async function loadDetail(id, silent = false) {
  const req = ++state.detailReq
  state.view = 'detail'
  state.detailId = id
  if (!silent) {
    state.detail = null
    state.detailError = ''
    draw()
  }
  const r = await linearFetch(ISSUE_QUERY, { id }).catch(() => null)
  if (req !== state.detailReq || state.view !== 'detail') return
  if (!r || r.errors || !r.data?.issue) {
    if (!silent) state.detailError = `couldn't load ${id}: ${r?.errors?.[0]?.message || 'network error'}`
  } else {
    state.detail = r.data.issue
    state.detailError = ''
  }
  draw()
}

function backToList() {
  state.view = 'list'
  state.detail = null
  state.detailId = null
  state.detailError = ''
  state.detailReq++
  draw()
}

function poll() {
  loadIssues(true)
  if (state.view === 'detail' && state.detailId) loadDetail(state.detailId, true)
}

function schedulePoll() {
  if (state.pollTimer) clearInterval(state.pollTimer)
  state.pollTimer = setInterval(poll, 60_000)
}

async function boot() {
  state.apiKey = await getSecret('apiKey')
  if (!state.apiKey) { state.stage = 'needKey'; draw(); return }
  await loadIssues()
  schedulePoll()
}

onmessage = async (e) => {
  const msg = e.data
  if (msg.type === 'reply') {
    const resolve = pending.get(msg.reqId)
    if (resolve) { pending.delete(msg.reqId); resolve(msg.value) }
    return
  }
  if (msg.type === 'command' && msg.id === 'connect') { boot(); return }
  if (msg.type === 'command' && msg.id === 'refresh') {
    if (!state.apiKey) return
    // In the detail view, refresh just refetches the open issue.
    if (state.view === 'detail' && state.detailId) { loadDetail(state.detailId); return }
    // In the list, "refresh" also clears filters back to defaults, matching
    // the panel's refresh icon — a full reset, not just a refetch.
    state.titleQuery = ''
    state.statusFilter = ACTIVE
    pushSelect()
    loadIssues()
    return
  }
  if (msg.type === 'click' && msg.panelId === 'issues' && state.apiKey) {
    if (msg.action === 'open' && msg.value) loadDetail(String(msg.value))
    else if (msg.action === 'back') backToList()
    return
  }
  if (msg.type === 'select' && msg.panelId === 'issues' && state.stage === 'ready') {
    state.statusFilter = String(msg.value || ACTIVE)
    if (state.view === 'detail') backToList()
    else draw()
    return
  }
  if (msg.type === 'input' && msg.panelId === 'issues') {
    const value = String(msg.value || '').trim()

    if (state.stage === 'needKey') {
      if (!value) return
      state.apiKey = value
      setSecret('apiKey', value)
      await loadIssues()
      schedulePoll()
      return
    }

    if (state.stage === 'ready') {
      // Typing a filter from the detail view drops back to the filtered list.
      state.titleQuery = value
      if (state.view === 'detail') backToList()
      else draw()
    }
  }
}

render('<div style="padding:8px 12px">Run "Linear: Connect" from the Command Palette (⌘⇧P) to set up.</div>')
