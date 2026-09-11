// Extension-to-CodeMirror-language mapping and per-theme syntax highlight
// palettes — shared by FileViewer.svelte (the host editor) and
// coeditGuest.ts (the Live Share co-editing guest page, bundled separately
// via esbuild into internal/liveshare/assets/coedit.js). Both need the same
// highlighting; keeping one copy means the guest page never drifts from
// what the host actually renders.
import { syntaxHighlighting, HighlightStyle, StreamLanguage, LanguageSupport } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'
import { javascript } from '@codemirror/lang-javascript'
import { python } from '@codemirror/lang-python'
import { css } from '@codemirror/lang-css'
import { html } from '@codemirror/lang-html'
import { json } from '@codemirror/lang-json'
import { markdown } from '@codemirror/lang-markdown'
import { yaml } from '@codemirror/lang-yaml'
import { go } from '@codemirror/lang-go'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import { svelte } from '@replit/codemirror-lang-svelte'

// ─── language detection ───────────────────────────────────────────────────
export function langFor(p: string) {
  const ext = p.split('.').pop()?.toLowerCase() ?? ''
  if (['js', 'mjs', 'cjs'].includes(ext))      return javascript()
  if (ext === 'ts')                            return javascript({ typescript: true })
  if (ext === 'tsx')                           return javascript({ typescript: true, jsx: true })
  if (ext === 'jsx')                           return javascript({ jsx: true })
  if (ext === 'py')                            return python()
  if (ext === 'css')                           return css()
  if (ext === 'svelte')                        return svelte()
  if (['html', 'vue'].includes(ext))           return html()
  if (ext === 'json')                          return json()
  if (['md', 'markdown'].includes(ext))        return markdown()
  if (['yaml', 'yml'].includes(ext))           return yaml()
  if (ext === 'go')                            return go()
  // wrapped in LanguageSupport (not the bare StreamLanguage) so it passes
  // codeIntel/snippets/qwenComplete's `instanceof LanguageSupport` gate —
  // bash now has a registered langext server (bash-language-server), so
  // this needs to actually reach codeIntel, not just get highlighted.
  if (['sh', 'bash', 'zsh', 'fish'].includes(ext)) return new LanguageSupport(StreamLanguage.define(shell))
  return []
}

// ─── per-theme syntax highlight palettes ─────────────────────────────────
type HSpec = Parameters<typeof HighlightStyle.define>[0]

const palettes: Record<string, HSpec> = {
  catppuccin: [
    { tag: t.keyword,                                          color: '#cba6f7', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#89b4fa' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#f9e2af' },
    { tag: t.string,                                           color: '#a6e3a1' },
    { tag: t.number,                                           color: '#fab387' },
    { tag: t.bool,                                             color: '#fab387' },
    { tag: t.null,                                             color: '#f38ba8' },
    { tag: t.comment,                                          color: '#585b70', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#89dceb' },
    { tag: t.punctuation,                                      color: '#9399b2' },
    { tag: t.tagName,                                          color: '#f38ba8' },
    { tag: t.attributeName,                                    color: '#89b4fa' },
    { tag: t.propertyName,                                     color: '#89dceb' },
    { tag: t.variableName,                                     color: '#cdd6f4' },
    { tag: t.definition(t.variableName),                       color: '#cdd6f4' },
    { tag: t.self,                                             color: '#f38ba8' },
  ],
  'tokyo-night': [
    { tag: t.keyword,                                          color: '#bb9af7', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#7aa2f7' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#e0af68' },
    { tag: t.string,                                           color: '#9ece6a' },
    { tag: t.number,                                           color: '#ff9e64' },
    { tag: t.bool,                                             color: '#ff9e64' },
    { tag: t.null,                                             color: '#f7768e' },
    { tag: t.comment,                                          color: '#565f89', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#89ddff' },
    { tag: t.punctuation,                                      color: '#c0caf5' },
    { tag: t.tagName,                                          color: '#f7768e' },
    { tag: t.attributeName,                                    color: '#bb9af7' },
    { tag: t.propertyName,                                     color: '#73daca' },
    { tag: t.variableName,                                     color: '#c0caf5' },
    { tag: t.self,                                             color: '#f7768e' },
  ],
  obsidian: [
    { tag: t.keyword,                                          color: '#7c5fe8', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#9d84f0' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#b8a4f5' },
    { tag: t.string,                                           color: '#b5e853' },
    { tag: t.number,                                           color: '#c9a227' },
    { tag: t.bool,                                             color: '#c9a227' },
    { tag: t.null,                                             color: '#e05c5c' },
    { tag: t.comment,                                          color: '#3a3550', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#8878cc' },
    { tag: t.punctuation,                                      color: '#5c5080' },
    { tag: t.tagName,                                          color: '#e05c5c' },
    { tag: t.attributeName,                                    color: '#9d84f0' },
    { tag: t.propertyName,                                     color: '#b8a4f5' },
    { tag: t.variableName,                                     color: '#d8d0c0' },
    { tag: t.self,                                             color: '#7c5fe8' },
  ],
  vos: [
    { tag: t.keyword,                                          color: '#569cd6', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#dcdcaa' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#4ec9b0' },
    { tag: t.string,                                           color: '#ce9178' },
    { tag: t.number,                                           color: '#b5cea8' },
    { tag: t.bool,                                             color: '#569cd6' },
    { tag: t.null,                                             color: '#569cd6' },
    { tag: t.comment,                                          color: '#6a9955', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#d4d4d4' },
    { tag: t.punctuation,                                      color: '#d4d4d4' },
    { tag: t.tagName,                                          color: '#4ec9b0' },
    { tag: t.attributeName,                                    color: '#9cdcfe' },
    { tag: t.propertyName,                                     color: '#9cdcfe' },
    { tag: t.variableName,                                     color: '#9cdcfe' },
    { tag: t.definition(t.variableName),                       color: '#dcdcaa' },
    { tag: t.self,                                             color: '#569cd6' },
    { tag: t.modifier,                                         color: '#569cd6' },
  ],
  gruvbox: [
    { tag: t.keyword,                                          color: '#fb4934', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#8ec07c' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#fabd2f' },
    { tag: t.string,                                           color: '#b8bb26' },
    { tag: t.number,                                           color: '#d3869b' },
    { tag: t.bool,                                             color: '#d3869b' },
    { tag: t.null,                                             color: '#fb4934' },
    { tag: t.comment,                                          color: '#928374', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#ebdbb2' },
    { tag: t.punctuation,                                      color: '#a89984' },
    { tag: t.tagName,                                          color: '#83a598' },
    { tag: t.attributeName,                                    color: '#fabd2f' },
    { tag: t.propertyName,                                     color: '#8ec07c' },
    { tag: t.variableName,                                     color: '#ebdbb2' },
  ],
  nord: [
    { tag: t.keyword,                                          color: '#81a1c1', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#88c0d0' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#ebcb8b' },
    { tag: t.string,                                           color: '#a3be8c' },
    { tag: t.number,                                           color: '#b48ead' },
    { tag: t.bool,                                             color: '#b48ead' },
    { tag: t.null,                                             color: '#bf616a' },
    { tag: t.comment,                                          color: '#616e88', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#81a1c1' },
    { tag: t.punctuation,                                      color: '#d8dee9' },
    { tag: t.tagName,                                          color: '#bf616a' },
    { tag: t.attributeName,                                    color: '#8fbcbb' },
    { tag: t.propertyName,                                     color: '#88c0d0' },
    { tag: t.variableName,                                     color: '#d8dee9' },
  ],
  monokai: [
    { tag: t.keyword,                                          color: '#f92672', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#a6e22e' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#66d9ef' },
    { tag: t.string,                                           color: '#e6db74' },
    { tag: t.number,                                           color: '#ae81ff' },
    { tag: t.bool,                                             color: '#ae81ff' },
    { tag: t.null,                                             color: '#ae81ff' },
    { tag: t.comment,                                          color: '#75715e', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#f8f8f2' },
    { tag: t.punctuation,                                      color: '#f8f8f2' },
    { tag: t.tagName,                                          color: '#f92672' },
    { tag: t.attributeName,                                    color: '#a6e22e' },
    { tag: t.propertyName,                                     color: '#66d9ef' },
    { tag: t.variableName,                                     color: '#f8f8f2' },
  ],
  light: [
    { tag: t.keyword,                                          color: '#0000ff', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#795e26' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#267f99' },
    { tag: t.string,                                           color: '#a31515' },
    { tag: t.number,                                           color: '#098658' },
    { tag: t.bool,                                             color: '#0000ff' },
    { tag: t.null,                                             color: '#0000ff' },
    { tag: t.comment,                                          color: '#008000', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#000000' },
    { tag: t.punctuation,                                      color: '#000000' },
    { tag: t.variableName,                                     color: '#001080' },
    { tag: t.propertyName,                                     color: '#001080' },
  ],
  default: [
    { tag: t.keyword,                                          color: '#7986cb', fontWeight: '600' },
    { tag: [t.function(t.variableName), t.function(t.propertyName)], color: '#64b5f6' },
    { tag: [t.typeName, t.className, t.namespace],             color: '#4dd0e1' },
    { tag: t.string,                                           color: '#4dc988' },
    { tag: t.number,                                           color: '#ffa040' },
    { tag: t.bool,                                             color: '#ffa040' },
    { tag: t.null,                                             color: '#ff5f6e' },
    { tag: t.comment,                                          color: '#3a3f5c', fontStyle: 'italic' },
    { tag: t.operator,                                         color: '#8c8fa8' },
    { tag: t.punctuation,                                      color: '#5c6180' },
    { tag: t.variableName,                                     color: '#d4d8ed' },
    { tag: t.propertyName,                                     color: '#a0aec8' },
  ],
}

export function highlightFor(themeName: string) {
  const spec = palettes[themeName] ?? palettes.catppuccin
  return syntaxHighlighting(HighlightStyle.define(spec))
}
