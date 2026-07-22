// Language snippets, surfaced through the normal autocomplete popup. Registered
// as an extra language-data `autocomplete` source so they merge with LSP /
// auto-import completions rather than replacing them.
import { snippetCompletion } from '@codemirror/autocomplete'
import type { CompletionSource } from '@codemirror/autocomplete'
import { LanguageSupport } from '@codemirror/language'
import type { Extension } from '@codemirror/state'
import type { IntelKind } from './codeintel'

interface Snip { label: string; detail: string; template: string }

const SNIPPETS: Record<string, Snip[]> = {
  js: [
    { label: 'log',     detail: 'console.log',  template: 'console.log(${})' },
    { label: 'fn',      detail: 'function',      template: 'function ${name}(${}) {\n\t${}\n}' },
    { label: 'af',      detail: 'arrow fn',      template: 'const ${name} = (${}) => {\n\t${}\n}' },
    { label: 'imp',     detail: 'import',        template: "import { ${} } from '${}'" },
    { label: 'foreach', detail: 'forEach',       template: '${arr}.forEach((${item}) => {\n\t${}\n})' },
    { label: 'try',     detail: 'try/catch',     template: 'try {\n\t${}\n} catch (${e}) {\n\t${}\n}' },
  ],
  go: [
    { label: 'iferr', detail: 'if err != nil', template: 'if err != nil {\n\t${}\n}' },
    { label: 'func',  detail: 'function',       template: 'func ${name}(${}) ${} {\n\t${}\n}' },
    { label: 'forr',  detail: 'for range',      template: 'for ${i}, ${v} := range ${coll} {\n\t${}\n}' },
    { label: 'main',  detail: 'func main',      template: 'func main() {\n\t${}\n}' },
  ],
  py: [
    { label: 'def',   detail: 'def',        template: 'def ${name}(${}):\n\t${}' },
    { label: 'class', detail: 'class',      template: 'class ${Name}:\n\tdef __init__(self${}):\n\t\t${}' },
    { label: 'main',  detail: 'main guard', template: "if __name__ == '__main__':\n\t${}" },
  ],
}

function snippetSource(kind: IntelKind): CompletionSource {
  const key = kind === 'svelte' ? 'js' : kind
  const options = (SNIPPETS[key] ?? []).map(d =>
    snippetCompletion(d.template, { label: d.label, detail: d.detail, type: 'snippet' }))
  return (ctx) => {
    const word = ctx.matchBefore(/\w+/)
    if (!word && !ctx.explicit) return null
    return { from: word ? word.from : ctx.pos, options }
  }
}

export function snippets(lang: unknown, kind: IntelKind | null): Extension[] {
  if (!kind || !(lang instanceof LanguageSupport)) return []
  return [lang.language.data.of({ autocomplete: snippetSource(kind) })]
}
