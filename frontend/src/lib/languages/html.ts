import type { EditorView } from '@codemirror/view'
import { indentInfo, replaceAll } from '../formatUtil'
import type { LanguageModule } from '../languageExtensions'

export const formatter: LanguageModule['formatter'] = async (view: EditorView) => {
  const { size, useTabs } = indentInfo(view)
  const [prettier, htmlPlugin] = await Promise.all([
    import('prettier/standalone'),
    import('prettier/plugins/html'),
  ])
  const out = await prettier.format(view.state.doc.toString(), {
    parser: 'html', plugins: [htmlPlugin.default], tabWidth: size, useTabs,
  })
  replaceAll(view, out)
}
