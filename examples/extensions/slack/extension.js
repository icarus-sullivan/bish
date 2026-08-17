// bish Slack extension — read and reply to Slack from inside the IDE.
// Copy this folder to ~/.bish/extensions/slack to try it.
//
// Runs in a Web Worker: no DOM/filesystem of its own, but fetch() and
// WebSocket ARE available in a worker, so this talks to slack.com directly.
// The bish backend (Go) is never involved — no server-side OAuth flow, no
// bish-side Slack code at all. Token storage and panel input are relayed
// through the extension host (getSecret/setSecret/input messages) purely
// because a worker has no localStorage or DOM of its own to hold them.
//
// One-time setup (internal Slack app, tokens pasted by hand — no OAuth
// redirect needed):
//   1. https://api.slack.com/apps -> Create New App -> From scratch.
//   2. OAuth & Permissions -> Bot Token Scopes: chat:write, channels:history,
//      channels:read, users:read (add groups:history + groups:read too if
//      you want private channels). Install to Workspace. Copy the
//      "Bot User OAuth Token" (starts xoxb-).
//   3. Socket Mode -> enable it -> generate an App-Level Token with the
//      connections:write scope (starts xapp-).
//   4. Event Subscriptions -> enable -> subscribe to bot events
//      message.channels (and message.groups for private channels).
//   5. Invite the bot to the target channel (/invite @yourbot in Slack),
//      then grab its channel ID (channel name -> View channel details).
//   6. Run "Slack: Connect" from the Command Palette (Cmd+Shift+P) and
//      paste the bot token, app token, and channel ID when the panel asks.
//
// Messages exchanged with the host, beyond the base extension protocol:
//   worker -> host  { type: 'getSecret', reqId, key }   (reply carries value)
//   worker -> host  { type: 'setSecret', key, value }   (fire-and-forget)
//   host -> worker  { type: 'input', panelId, value }   (panel input submit)

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
  postMessage({ type: 'setPanelHTML', panelId: 'chat', html })
}

const state = {
  botToken: null,
  appToken: null,
  channel: null,
  stage: 'boot', // boot -> needBot -> needApp -> needChannel -> connecting -> connected
  log: [],       // { user, text, ts }
  names: new Map(),
  ws: null,
}

function draw(errorLine) {
  const prompt = {
    needBot: 'Paste your Slack <b>Bot User OAuth Token</b> (xoxb-...) below and press enter.',
    needApp: 'Paste your Slack <b>App-Level Token</b> (xapp-...) below and press enter.',
    needChannel: 'Paste the target <b>channel ID</b> (e.g. C0123456) below and press enter.',
    connecting: 'Connecting…',
  }[state.stage]
  if (prompt) { render(`<div style="padding:8px 12px">${prompt}</div>`); return }

  const lines = state.log.map(m =>
    `<div style="margin:2px 0"><b>${esc(state.names.get(m.user) || m.user)}</b>: ${esc(m.text)}</div>`
  ).join('')
  const err = errorLine ? `<div style="padding:0 12px 8px;color:#e5484d">${esc(errorLine)}</div>` : ''
  render(`<div style="padding:8px 12px">${lines || '<i>no messages yet</i>'}</div>${err}`)
}

async function slackFetch(method, token, body) {
  const res = await fetch(`https://slack.com/api/${method}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json; charset=utf-8',
    },
    body: JSON.stringify(body ?? {}),
  })
  return res.json()
}

async function nameFor(userId) {
  if (state.names.has(userId)) return state.names.get(userId)
  const r = await slackFetch('users.info', state.botToken, { user: userId }).catch(() => null)
  const name = r?.ok ? (r.user.profile.display_name || r.user.real_name || r.user.name) : userId
  state.names.set(userId, name)
  return name
}

async function loadHistory() {
  const r = await slackFetch('conversations.history', state.botToken, { channel: state.channel, limit: 30 }).catch(() => null)
  if (!r?.ok) { draw(`history failed: ${r?.error || 'network error'}`); return }
  const msgs = (r.messages || []).filter(m => m.type === 'message' && !m.subtype).reverse()
  for (const m of msgs) {
    await nameFor(m.user)
    state.log.push({ user: m.user, text: m.text, ts: m.ts })
  }
  draw()
}

async function connectSocketMode() {
  state.stage = 'connecting'
  draw()
  const r = await slackFetch('apps.connections.open', state.appToken, null).catch(() => null)
  if (!r?.ok) { draw(`connect failed: ${r?.error || 'network error'}`); return }

  const ws = new WebSocket(r.url)
  state.ws = ws

  ws.onopen = async () => {
    state.stage = 'connected'
    await loadHistory()
  }
  ws.onmessage = async (e) => {
    let payload
    try { payload = JSON.parse(e.data) } catch { return }
    if (payload.envelope_id) ws.send(JSON.stringify({ envelope_id: payload.envelope_id }))
    if (payload.type === 'disconnect') { ws.close(); connectSocketMode(); return }
    const event = payload.payload?.event
    if (event?.type === 'message' && !event.subtype && event.channel === state.channel) {
      await nameFor(event.user)
      state.log.push({ user: event.user, text: event.text, ts: event.ts })
      draw()
    }
  }
  ws.onclose = () => {
    if (state.stage === 'connected') { state.stage = 'connecting'; setTimeout(connectSocketMode, 2000) }
  }
}

async function boot() {
  state.botToken = await getSecret('botToken')
  if (!state.botToken) { state.stage = 'needBot'; draw(); return }
  state.appToken = await getSecret('appToken')
  if (!state.appToken) { state.stage = 'needApp'; draw(); return }
  state.channel = await getSecret('channel')
  if (!state.channel) { state.stage = 'needChannel'; draw(); return }
  await connectSocketMode()
}

onmessage = async (e) => {
  const msg = e.data
  if (msg.type === 'reply') {
    const resolve = pending.get(msg.reqId)
    if (resolve) { pending.delete(msg.reqId); resolve(msg.value) }
    return
  }
  if (msg.type === 'command' && msg.id === 'connect') { boot(); return }
  if (msg.type === 'input' && msg.panelId === 'chat') {
    const value = String(msg.value || '').trim()
    if (!value) return

    if (state.stage === 'needBot') { state.botToken = value; setSecret('botToken', value); state.stage = 'needApp'; draw(); return }
    if (state.stage === 'needApp') { state.appToken = value; setSecret('appToken', value); state.stage = 'needChannel'; draw(); return }
    if (state.stage === 'needChannel') { state.channel = value; setSecret('channel', value); connectSocketMode(); return }

    if (state.stage === 'connected') {
      state.log.push({ user: 'you', text: value, ts: String(Date.now() / 1000) })
      draw()
      const r = await slackFetch('chat.postMessage', state.botToken, { channel: state.channel, text: value }).catch(() => null)
      if (!r?.ok) draw(`send failed: ${r?.error || 'network error'}`)
    }
  }
}

render('<div style="padding:8px 12px">Run "Slack: Connect" from the Command Palette (⌘⇧P) to set up.</div>')
