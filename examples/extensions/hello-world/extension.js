// Sample bish extension — copy this folder to ~/.bish/extensions/hello-world
// to try it, then restart bish (or toggle it off/on in Settings > Extensions).
//
// This script runs in its own Web Worker: no DOM, no filesystem, no network.
// Commands and panels are declared in bish-extension.json, not registered
// here — this script only reacts when one of them fires.
//
// Messages you receive:
//   { type: 'command', id }         — a contributed command was run
//   { type: 'reply', reqId, value } — response to a request you sent
//
// Messages you can send:
//   { type: 'setPanelHTML', panelId, html }  — set a contributed panel's
//                                               content (sanitized before render)
//   { type: 'getActiveFilePath', reqId }     — ask for the active file's path

let reqCounter = 0
const pending = new Map()

function getActiveFilePath() {
  return new Promise(resolve => {
    const reqId = ++reqCounter
    pending.set(reqId, resolve)
    postMessage({ type: 'getActiveFilePath', reqId })
  })
}

onmessage = async (e) => {
  const msg = e.data
  if (msg.type === 'reply') {
    const resolve = pending.get(msg.reqId)
    if (resolve) { pending.delete(msg.reqId); resolve(msg.value) }
    return
  }
  if (msg.type === 'command' && msg.id === 'sayHello') {
    const path = await getActiveFilePath()
    postMessage({
      type: 'setPanelHTML',
      panelId: 'greeting',
      html: `<div style="padding:8px 12px">👋 Hello from the sample extension!<br>Active file: <code>${path || '(none)'}</code></div>`,
    })
  }
}

// something visible before the command ever runs, so the panel isn't blank
postMessage({
  type: 'setPanelHTML',
  panelId: 'greeting',
  html: '<div style="padding:8px 12px">Run "Hello World: Say Hello" from the Command Palette (⌘⇧P).</div>',
})
