<script lang="ts">
  import { IconRefresh, IconExternalLink, IconChevronDown, IconAlertTriangle } from '@tabler/icons-svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { PreviewFrameBlocked } from '../lib/wails'

  let { url: initialUrl }: { url: string } = $props()

  // the frame is cross-origin to bish: no console, no DOM, no route sync —
  // just the page. Anything that needs devtools opens externally.
  let url = $state(initialUrl)
  let draft = $state(initialUrl)
  let width = $state('responsive')
  // reload must re-create the element: contentWindow.location.reload()
  // throws cross-origin and reassigning src only stacks history entries
  let reloadNonce = $state(0)
  let failure = $state<'' | 'unreachable' | 'refused' | 'slow'>('')
  let refusedBy = $state('')
  let loaded = $state(false)
  let frame = $state<HTMLIFrameElement | undefined>()
  let slowTimer: ReturnType<typeof setTimeout> | undefined

  function normalize(u: string) {
    u = u.trim()
    if (!u) return url
    if (/^\d+$/.test(u)) return `http://localhost:${u}`
    if (!/^https?:\/\//i.test(u)) return 'http://' + u
    return u
  }

  function go() {
    url = normalize(draft)
    draft = url
    reload()
  }

  function reload() {
    reloadNonce++
  }

  // a frame that refuses framing (X-Frame-Options / frame-ancestors) never
  // reports an error event — it just stays blank, which reads as "bish is
  // broken". Probe reachability first, then check what actually loaded.
  $effect(() => {
    void reloadNonce
    const target = url
    failure = ''
    loaded = false
    clearTimeout(slowTimer)
    refusedBy = ''
    fetch(target, { mode: 'no-cors', cache: 'no-store' })
      .catch(() => { if (target === url) failure = 'unreachable' })
    PreviewFrameBlocked(target)
      .then(reason => { if (reason && target === url) { refusedBy = reason; failure = 'refused' } })
      .catch(() => {})
    slowTimer = setTimeout(() => { if (!loaded && !failure) failure = 'slow' }, 10000)
    return () => clearTimeout(slowTimer)
  })

  function onLoad() {
    loaded = true
    clearTimeout(slowTimer)
    if (failure === 'slow') failure = ''
    try {
      // readable = same-origin with bish = the blank placeholder WebKit
      // leaves behind when the server refused to be framed
      const href = frame?.contentWindow?.location.href
      const empty = !frame?.contentDocument?.body?.childElementCount
      if ((href === 'about:blank' || href === '') && empty && failure !== 'unreachable') failure = 'refused'
    } catch {
      // cross-origin access denied = a real page from the dev server loaded
    }
  }

  function openExternal() {
    BrowserOpenURL(url)
  }
</script>

<div class="preview">
  <div class="bar">
    <button class="hdr-btn" onclick={reload} title="Reload"><IconRefresh size={13} /></button>
    <input class="addr" bind:value={draft} spellcheck="false" autocapitalize="none" autocomplete="off"
      onkeydown={(e) => { if (e.key === 'Enter') go() }} />
    <span class="select-wrap">
      <select bind:value={width} title="Viewport width">
        <option value="responsive">Responsive</option>
        <option value="390">390 · phone</option>
        <option value="768">768 · tablet</option>
        <option value="1280">1280 · desktop</option>
      </select>
      <IconChevronDown size={13} class="select-chevron" />
    </span>
    <button class="hdr-btn" onclick={openExternal} title="Open in browser"><IconExternalLink size={13} /></button>
  </div>
  <div class="stage">
    {#if failure === 'unreachable' || failure === 'refused'}
      <div class="failure">
        <IconAlertTriangle size={20} />
        {#if failure === 'unreachable'}
          <div class="msg">Nothing is answering at <code>{url}</code> yet.</div>
          <div class="sub">Start the service, then reload.</div>
        {:else}
          <div class="msg">This server refuses to be shown inside another app.</div>
          <div class="sub">It sends <code>{refusedBy || 'X-Frame-Options / frame-ancestors'}</code>. Open it in the browser instead.</div>
        {/if}
        <div class="actions">
          <button class="btn" onclick={reload}>Retry</button>
          <button class="btn primary" onclick={openExternal}>Open in browser</button>
        </div>
      </div>
    {:else}
      {#if failure === 'slow'}
        <div class="slow">Still loading… <button class="link" onclick={openExternal}>open in browser</button></div>
      {/if}
      <div class="frame-wrap" style={width === 'responsive' ? '' : `width:${width}px`}>
        {#key reloadNonce}
          <!-- no allow-top-navigation: a framed dev page must never be able
               to navigate the bish window itself -->
          <iframe bind:this={frame} src={url} title="Preview" onload={onLoad}
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-modals allow-downloads"></iframe>
        {/key}
      </div>
    {/if}
  </div>
</div>

<style>
  .preview { display: flex; flex-direction: column; width: 100%; height: 100%; overflow: hidden; }
  .bar {
    display: flex; align-items: center; gap: 6px;
    padding: 4px 8px; flex-shrink: 0;
    background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .hdr-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    padding: 3px 4px;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .hdr-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .addr {
    flex: 1; min-width: 0;
    background: var(--background); border: 1px solid var(--border); border-radius: 5px;
    color: var(--foreground); font-size: 11px; padding: 4px 8px; outline: none;
    font-family: "SF Mono", Menlo, monospace;
  }
  .addr:focus { border-color: var(--accent); }

  .select-wrap { position: relative; display: inline-flex; align-items: center; }
  select {
    appearance: none;
    -webkit-appearance: none;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--foreground);
    font-size: 11px;
    padding: 4px 24px 4px 8px;
    outline: none;
    cursor: pointer;
    transition: border-color 0.1s, background 0.1s;
  }
  select:hover { background: var(--bg-hover); }
  select:focus { border-color: var(--accent); }
  option { background: var(--background); color: var(--foreground); }
  .select-wrap :global(.select-chevron) {
    position: absolute;
    right: 7px;
    color: var(--muted);
    pointer-events: none;
  }

  .stage {
    flex: 1; min-height: 0; position: relative;
    display: flex; flex-direction: column; align-items: center;
    background: var(--background); overflow: auto;
  }
  .frame-wrap { flex: 1; width: 100%; max-width: 100%; display: flex; }
  .frame-wrap[style*="width"] { border-left: 1px solid var(--border); border-right: 1px solid var(--border); }
  iframe { flex: 1; width: 100%; height: 100%; border: none; background: #fff; }

  .slow {
    width: 100%; box-sizing: border-box; padding: 4px 10px; font-size: 11px;
    color: var(--muted); background: var(--bg-raised); border-bottom: 1px solid var(--border);
  }
  .failure {
    margin: auto; max-width: 380px; text-align: center; padding: 24px;
    display: flex; flex-direction: column; align-items: center; gap: 8px;
    color: var(--muted);
  }
  .msg { color: var(--foreground); font-size: 13px; }
  .sub { font-size: 11px; line-height: 1.5; }
  code { font-family: "SF Mono", Menlo, monospace; font-size: 11px; }
  .actions { display: flex; gap: 8px; margin-top: 6px; }
  .btn {
    background: none; border: 1px solid var(--border); border-radius: 5px;
    color: var(--muted); font-size: 11px; padding: 5px 12px; cursor: pointer;
  }
  .btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .btn.primary { background: var(--accent); border-color: var(--accent); color: #000; font-weight: 600; }
  .btn.primary:hover { opacity: 0.85; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; font-size: 11px; padding: 0; }
</style>
