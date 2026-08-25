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
  if (state.stage === 'loading') {
    render('<div style="padding:8px 12px">Loading issues…</div>')
    return
  }

  const header = `<div style="padding:6px 12px;font-size:11px;opacity:0.6;border-bottom:1px solid rgba(128,128,128,0.15)">
    ${state.titleQuery ? `filtering by "${esc(state.titleQuery)}"` : 'type below to filter by ticket title — use the dropdown to filter by status'}
  </div>`
  const rows = visibleIssues().map(i => `
    <div style="padding:6px 12px;border-bottom:1px solid rgba(128,128,128,0.15)">
      <a href="${esc(i.url)}" target="_blank" rel="noopener" style="color:inherit;text-decoration:none">
        <span style="opacity:0.6">${esc(i.identifier)}</span>
        <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${esc(i.stateColor || '#888')};margin:0 6px"></span>
        ${esc(i.title)}
      </a>
    </div>
  `).join('')
  const err = errorLine ? `<div style="padding:0 12px 8px;color:#e5484d">${esc(errorLine)}</div>` : ''
  render(`<div>${header}${rows || '<div style="padding:8px 12px"><i>no matching issues</i></div>'}</div>${err}`)
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

async function loadIssues() {
  state.stage = 'loading'
  draw()
  const r = await linearFetch(ISSUES_QUERY).catch(() => null)
  if (!r || r.errors) { draw(`fetch failed: ${r?.errors?.[0]?.message || 'network error'}`); state.stage = 'ready'; return }
  state.issues = (r.data?.viewer?.assignedIssues?.nodes || []).map(n => ({
    identifier: n.identifier, title: n.title, url: n.url,
    stateName: n.state?.name, stateType: n.state?.type, stateColor: n.state?.color,
  }))
  state.stage = 'ready'
  pushSelect()
  draw()
}

function schedulePoll() {
  if (state.pollTimer) clearInterval(state.pollTimer)
  state.pollTimer = setInterval(loadIssues, 60_000)
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
    // "refresh" also clears filters back to defaults, matching the panel's
    // refresh icon — a full reset, not just a refetch.
    state.titleQuery = ''
    state.statusFilter = ACTIVE
    loadIssues()
    return
  }
  if (msg.type === 'select' && msg.panelId === 'issues' && state.stage === 'ready') {
    state.statusFilter = String(msg.value || ACTIVE)
    draw()
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
      state.titleQuery = value
      draw()
    }
  }
}

render('<div style="padding:8px 12px">Run "Linear: Connect" from the Command Palette (⌘⇧P) to set up.</div>')
