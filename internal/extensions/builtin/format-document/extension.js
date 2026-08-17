// Built-in "Format Document" extension — ships with bish (seeded into
// ~/.bish/extensions on first launch, see extensions.SeedBuiltins), shows
// up in the Command Palette and is bound to a default key (see manifest)
// the moment it's enabled.
//
// The actual formatting logic isn't reimplemented here: this worker has no
// access to the editor or the LSP client, so it just asks the host to run
// bish's existing format-on-save pipeline (same one Settings > Format on
// Save triggers) against whichever file is currently open.
onmessage = (e) => {
  const msg = e.data
  if (msg.type === 'command' && msg.id === 'format') {
    postMessage({ type: 'formatActiveDocument' })
  }
}
