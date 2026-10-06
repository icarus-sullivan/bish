// ─── Voice dictation (vosk-browser) ─────────────────────────────────────────
// Offline speech recognition entirely in the webview: Kaldi compiled to WASM
// (vosk-browser) fed from getUserMedia. The model ships inside the app
// (scripts/fetch-vosk-model.sh → public/vosk/), so there is nothing to
// install or download at runtime. Gated by the 'dictation' feature — this
// module (and the WASM worker) is only imported on the first mic click.
import type { Model, KaldiRecognizer } from 'vosk-browser'

const MODEL_PATH = 'vosk/model.tar.gz'

let modelPromise: Promise<Model> | null = null

// One model per window, loaded on first use and kept: unpacking the archive
// + building the decoder takes a few seconds, recognizers are cheap after.
function loadModel(): Promise<Model> {
  modelPromise ??= (async () => {
    const { createModel } = await import('vosk-browser')
    // The worker is a blob: worker; fetch the archive on the main thread and
    // hand it a same-origin blob URL so it never has to resolve the app's
    // custom wails:// scheme itself.
    const res = await fetch(new URL(MODEL_PATH, location.href).href)
    if (!res.ok) throw new Error('Speech model not bundled in this build — run `make fetch-vosk-model`')
    const url = URL.createObjectURL(await res.blob())
    try { return await createModel(url) } finally { URL.revokeObjectURL(url) }
  })()
  modelPromise.catch(() => { modelPromise = null }) // let a later click retry
  return modelPromise
}

export interface DictationSession {
  /** Stops the mic, flushes the last utterance, resolves once it's delivered. */
  stop(): Promise<void>
  /** Stops the mic and drops anything not yet finalized. */
  cancel(): void
}

export interface DictationHandlers {
  /** In-progress hypothesis for the current utterance — replaces the previous partial. */
  onPartial(text: string): void
  /** Finalized utterance — the matching partial should be committed as this. */
  onResult(text: string): void
}

let active: DictationSession | null = null

export async function startDictation(h: DictationHandlers): Promise<DictationSession> {
  if (active) throw new Error('Already dictating in another AI panel tab')
  if (!navigator.mediaDevices?.getUserMedia) throw new Error('Microphone access is not available in this window')

  const model = await loadModel()
  const stream = await navigator.mediaDevices.getUserMedia({
    video: false,
    audio: { echoCancellation: true, noiseSuppression: true, channelCount: 1 },
  }).catch((e: Error) => {
    throw new Error(e?.name === 'NotAllowedError'
      ? 'Microphone access denied — allow bish in System Settings → Privacy & Security → Microphone'
      : `Could not open the microphone: ${e?.message ?? e}`)
  })

  const ctx = new AudioContext()
  const rec: KaldiRecognizer = new model.KaldiRecognizer(ctx.sampleRate)
  let live = true
  let flushed: (() => void) | null = null
  rec.on('partialresult', m => { if (live && m.event === 'partialresult') h.onPartial(m.result.partial) })
  rec.on('result', m => {
    if (m.event !== 'result') return
    if (live && m.result.text) h.onResult(m.result.text)
    flushed?.()
  })

  const source = ctx.createMediaStreamSource(stream)
  // ScriptProcessor is deprecated but needs no separate worklet module (which
  // the wails:// scheme makes awkward); 4096 frames ≈ 85ms at 48kHz.
  const node = ctx.createScriptProcessor(4096, 1, 1)
  node.onaudioprocess = e => { if (live) rec.acceptWaveform(e.inputBuffer) }
  source.connect(node)
  node.connect(ctx.destination) // WebKit only fires onaudioprocess when connected; output stays silent

  function teardown() {
    node.onaudioprocess = null
    source.disconnect(); node.disconnect()
    stream.getTracks().forEach(t => t.stop())
    ctx.close().catch(() => {})
    rec.remove()
    active = null
  }

  const session: DictationSession = {
    async stop() {
      node.onaudioprocess = null
      stream.getTracks().forEach(t => t.stop())
      // ask for the trailing utterance; give the worker a moment to answer
      await new Promise<void>(resolve => {
        flushed = resolve
        rec.retrieveFinalResult()
        setTimeout(resolve, 1500)
      })
      live = false
      teardown()
    },
    cancel() {
      live = false
      teardown()
    },
  }
  active = session
  return session
}
