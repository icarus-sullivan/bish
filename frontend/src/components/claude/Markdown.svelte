<script lang="ts">
  // Rendered (already sanitized) markdown with copy buttons on code blocks
  // and links routed safely: http(s) → system browser, project paths → editor.
  import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
  import { openFileTab, pendingGoto } from '../../lib/stores'

  let { html, root, streaming = false }: { html: string; root: string; streaming?: boolean } = $props()
  let el: HTMLDivElement

  $effect(() => {
    html
    if (!el) return
    for (const pre of el.querySelectorAll('pre')) {
      if (pre.querySelector('.copy-code')) continue
      const b = document.createElement('button')
      b.className = 'copy-code'
      b.type = 'button'
      b.textContent = 'Copy'
      pre.appendChild(b)
    }
  })

  // "path/to/file.ts:42" or "path/to/file.ts#L42" → editor, but only inside the project
  function openProjectPath(href: string): boolean {
    const m = /^(?:file:\/\/)?([^#?]+?)(?::(\d+)(?::\d+)?|#L(\d+)(?:-L?\d+)?)?$/.exec(decodeURIComponent(href))
    if (!m || !root) return false
    const raw = m[1]
    const abs = normalize(raw.startsWith('/') ? raw : root + '/' + raw)
    if (abs !== root && !abs.startsWith(root + '/')) return false
    openFileTab(abs)
    const line = Number(m[2] || m[3] || 0)
    if (line > 0) pendingGoto.set({ path: abs, line, col: 0 })
    return true
  }

  function normalize(p: string): string {
    const out: string[] = []
    for (const seg of p.split('/')) {
      if (!seg || seg === '.') continue
      if (seg === '..') out.pop()
      else out.push(seg)
    }
    return '/' + out.join('/')
  }

  function onClick(e: MouseEvent) {
    const target = e.target as HTMLElement
    const copy = target.closest('.copy-code')
    if (copy) {
      const pre = copy.closest('pre')
      const code = pre?.querySelector('code')?.textContent ?? pre?.textContent ?? ''
      navigator.clipboard.writeText(code.replace(/Copy$/, ''))
      copy.textContent = 'Copied'
      setTimeout(() => { copy.textContent = 'Copy' }, 1200)
      return
    }
    const a = target.closest('a[data-href]')
    if (!a) return
    e.preventDefault()
    const href = a.getAttribute('data-href') ?? ''
    if (/^https?:\/\//i.test(href)) BrowserOpenURL(href)
    else openProjectPath(href)
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.target as HTMLElement).matches('a[data-href]')) onClick(e as unknown as MouseEvent)
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="md" class:streaming bind:this={el} onclick={onClick} onkeydown={onKey}>{@html html}</div>

<style>
  .md { font-size: 12.5px; line-height: 1.6; color: var(--foreground); overflow-wrap: anywhere; }
  .md :global(p) { margin: 0 0 8px; }
  .md :global(p:last-child) { margin-bottom: 0; }
  .md :global(h1), .md :global(h2), .md :global(h3), .md :global(h4), .md :global(h5), .md :global(h6) {
    margin: 12px 0 6px; font-size: 13px; font-weight: 600;
  }
  .md :global(h1) { font-size: 14px; }
  .md :global(:first-child) { margin-top: 0; }
  .md :global(ul), .md :global(ol) { margin: 0 0 8px; padding-left: 20px; }
  .md :global(li) { margin: 0 0 3px; }
  .md :global(li > p) { margin: 0; }
  .md :global(code) {
    font-family: "SF Mono", Menlo, monospace; font-size: 11.5px;
    background: var(--bg-raised); border: 1px solid var(--border); border-radius: 3px; padding: 0 3px;
  }
  .md :global(pre) {
    position: relative; background: var(--bg-raised); border: 1px solid var(--border);
    padding: 8px 10px; border-radius: 5px; overflow-x: auto; margin: 0 0 8px;
  }
  .md :global(pre code) { background: none; border: none; padding: 0; font-size: 11.5px; line-height: 1.5; }
  .md :global(.copy-code) {
    position: absolute; top: 4px; right: 4px; opacity: 0;
    background: var(--background); border: 1px solid var(--border); border-radius: 3px;
    color: var(--muted); font-size: 10px; padding: 1px 6px; cursor: pointer; transition: opacity 0.1s;
  }
  .md :global(pre:hover .copy-code) { opacity: 1; }
  .md :global(.copy-code:hover) { color: var(--foreground); }
  .md :global(blockquote) { border-left: 2px solid var(--border); margin: 0 0 8px; padding-left: 10px; color: var(--muted); }
  .md :global(table) { border-collapse: collapse; margin: 0 0 8px; display: block; overflow-x: auto; }
  .md :global(th), .md :global(td) { border: 1px solid var(--border); padding: 3px 7px; }
  .md :global(th) { font-weight: 600; background: var(--bg-raised); }
  .md :global(hr) { border: none; border-top: 1px solid var(--border); margin: 8px 0; }
  .md :global(a) { color: var(--accent); cursor: pointer; text-decoration: none; }
  .md :global(a:hover) { text-decoration: underline; }
  .md.streaming > :global(:last-child)::after {
    content: ''; display: inline-block; width: 6px; height: 12px; margin-left: 2px; vertical-align: -1px;
    background: var(--accent); animation: md-caret 1s steps(2) infinite;
  }
  @keyframes md-caret { 50% { opacity: 0; } }
</style>
