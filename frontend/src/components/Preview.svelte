<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { IconRefresh, IconExternalLink, IconChevronDown, IconAlertTriangle, IconArrowLeft, IconArrowRight } from '@tabler/icons-svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import {
    PreviewFrameBlocked, on, BrowserSupported, BrowserOpen, BrowserSetFrame, BrowserSetVisible,
    BrowserNavigate, BrowserReload, BrowserBack, BrowserForward, BrowserClose,
  } from '../lib/wails'
  import { featureOn } from '../lib/features'
  import { nativeViewBlockers, forwardNativeViewKeys } from '../lib/nativeview'

  let { url: initialUrl, tabId, active }: { url: string; tabId: string; active: boolean } = $props()

  // Two renderers. Native (macOS, `nativePreview`): a real top-level WebKit
  // view laid over .stage by Go — first-party, so cookies/localStorage/
  // logins work and persist, and X-Frame-Options doesn't apply. Iframe
  // (fallback): a third-party frame under wails://, so WebKit drops its
  // cookies; also cross-origin to bish: no console, no DOM, no route sync.
  // null = still asking Go which one this platform has.
  let native = $state<boolean | null>(null)
  let nativeOpened = false
  let placeholder = $state<HTMLDivElement | undefined>()
  let editingAddr = false
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
    if (native) { probe(url); BrowserNavigate(tabId, url) }
    else reload()
  }

  function reload() {
    if (native) { probe(url); BrowserReload(tabId) }
    else reloadNonce++
  }

  // the native view renders its own error pages, but "nothing listening
  // yet" deserves the friendlier hint below
  function probe(target: string) {
    failure = ''
    fetch(target, { mode: 'no-cors', cache: 'no-store' })
      .catch(() => { if (target === url) failure = 'unreachable' })
  }

  onMount(async () => {
    const ok = featureOn('nativePreview') && await BrowserSupported().catch(() => false)
    native = ok
    if (!ok || destroyed) return
    forwardNativeViewKeys()
    probe(url)
    BrowserOpen(tabId, url)
    nativeOpened = true
  })

  let destroyed = false
  onDestroy(() => { destroyed = true; if (nativeOpened) BrowserClose(tabId) })

  $effect(() => {
    if (!native) return
    return on('browser:nav', (n: { id: string; url: string }) => {
      if (n.id !== tabId || !n.url || n.url === 'about:blank') return
      url = n.url
      if (!editingAddr) draft = n.url
    })
  })

  // keep the native view glued to the placeholder: size changes come from
  // the ResizeObserver, pure moves (window resize) from the window listener
  $effect(() => {
    const el = placeholder
    if (!native || !el) return
    const sync = () => {
      const r = el.getBoundingClientRect()
      if (r.width && r.height) BrowserSetFrame(tabId, r.left, r.top, r.width, r.height)
    }
    const ro = new ResizeObserver(sync)
    ro.observe(el)
    window.addEventListener('resize', sync)
    sync()
    return () => { ro.disconnect(); window.removeEventListener('resize', sync) }
  })

  $effect(() => {
    if (!native) return
    BrowserSetVisible(tabId, active && $nativeViewBlockers === 0 && failure !== 'unreachable' && !!placeholder)
  })

  // a frame that refuses framing (X-Frame-Options / frame-ancestors) never
  // reports an error event — it just stays blank, which reads as "bish is
  // broken". Probe reachability first, then check what actually loaded.
  $effect(() => {
    void reloadNonce
    if (native !== false) return
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
    {#if native}
      <button class="hdr-btn" onclick={() => BrowserBack(tabId)} title="Back"><IconArrowLeft size={13} /></button>
      <button class="hdr-btn" onclick={() => BrowserForward(tabId)} title="Forward"><IconArrowRight size={13} /></button>
    {/if}
    <button class="hdr-btn" onclick={reload} title="Reload"><IconRefresh size={13} /></button>
    <input class="addr" bind:value={draft} spellcheck="false" autocapitalize="none" autocomplete="off"
      onfocus={() => { editingAddr = true }} onblur={() => { editingAddr = false }}
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
        {#if native}
          <!-- Go lays the native view over this box -->
          <div class="native-slot" bind:this={placeholder}></div>
        {:else if native === false}
        {#key reloadNonce}
          <!-- no allow-top-navigation: a framed dev page must never be able
               to navigate the bish window itself -->
          <iframe bind:this={frame} src={url} title="Preview" onload={onLoad}
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-modals allow-downloads"></iframe>
        {/key}
        {/if}
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
  .native-slot { flex: 1; }
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
