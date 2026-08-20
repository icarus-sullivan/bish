// Prettier's markdown printer is a reprint of its own remark-based AST, not
// a byte-preserving pass — long lines may get rewrapped depending on the
// user's proseWrap setting (defaulted off/"preserve" below to avoid
// surprising reflows on save).
import type { EditorView } from '@codemirror/view'
import { indentInfo, replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = async (view: EditorView) => {
  const { size, useTabs } = indentInfo(view)
  const [prettier, markdownPlugin] = await Promise.all([
    import('prettier/standalone'),
    import('prettier/plugins/markdown'),
  ])
  const out = await prettier.format(view.state.doc.toString(), {
    parser: 'markdown', plugins: [markdownPlugin.default],
    tabWidth: size, useTabs, proseWrap: 'preserve',
  })
  replaceAll(view, out)
}
