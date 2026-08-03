// Local Qwen2.5-Coder inline suggestions, surfaced through the normal
// autocomplete popup as one more source — same shape as snippets.ts, merged
// alongside LSP/auto-import/snippet completions rather than replacing them.
// The model is deliberately not trusted for anything requiring exact
// project knowledge (symbol names, real import paths): that stays on
// autoImportSource/LSP. This only fills gaps for boilerplate/pattern
// completions (for-loops, common idioms) that those don't attempt.
import type { CompletionSource } from '@codemirror/autocomplete'
import { LanguageSupport } from '@codemirror/language'
import type { Extension } from '@codemirror/state'
import type { IntelKind } from './codeintel'
import { CompletionSuggest } from './wails'

// Bounds the /infill request size — recent context is what FIM needs, not
// the whole file.
const PREFIX_CHARS = 2000
const SUFFIX_CHARS = 1000
// Wait for typing to settle before asking the model; CodeMirror's
// documented pattern for slow async sources is to poll ctx.aborted after a
// delay rather than debounce externally.
const DEBOUNCE_MS = 250

function qwenCompletionSource(): CompletionSource {
  return async (ctx) => {
    const word = ctx.matchBefore(/\w*/)
    if (!word && !ctx.explicit) return null

    await new Promise((r) => setTimeout(r, DEBOUNCE_MS))
    if (ctx.aborted) return null

    const doc = ctx.state.doc
    const pos = ctx.pos
    const prefix = doc.sliceString(Math.max(0, pos - PREFIX_CHARS), pos)
    const suffix = doc.sliceString(pos, Math.min(doc.length, pos + SUFFIX_CHARS))

    const text = await CompletionSuggest(prefix, suffix).catch(() => '')
    if (ctx.aborted || !text) return null

    return { from: word ? word.from : pos, options: [{ label: text, type: 'text', boost: -10 }] }
  }
}

export function qwenComplete(lang: unknown, kind: IntelKind | null): Extension[] {
  if (!kind || !(lang instanceof LanguageSupport)) return []
  return [lang.language.data.of({ autocomplete: qwenCompletionSource() })]
}
