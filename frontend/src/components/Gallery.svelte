<script lang="ts">
  import { galleryImages, galleryIndex, galleryMode } from '../lib/stores'
  import { ReadFileBase64, mediaUrl } from '../lib/wails'

  let slideshow: ReturnType<typeof setInterval> | null = null
  let slideshowActive = $state(false)

  $effect(() => {
    if (slideshowActive) {
      slideshow = setInterval(() => {
        galleryIndex.update(i => (i + 1) % $galleryImages.length)
      }, 5000)
    } else {
      if (slideshow) clearInterval(slideshow)
    }
    return () => { if (slideshow) clearInterval(slideshow) }
  })

  function next() { galleryIndex.update(i => (i + 1) % $galleryImages.length) }
  function prev() { galleryIndex.update(i => (i - 1 + $galleryImages.length) % $galleryImages.length) }
  function exit() { galleryMode.set(false); if (slideshow) clearInterval(slideshow) }

  function handleKey(e: KeyboardEvent) {
    if (!$galleryMode) return
    if (e.key === 'ArrowRight') { e.preventDefault(); next() }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); prev() }
    else if (e.key === 'Escape') exit()
    else if (e.key === 's') slideshowActive = !slideshowActive
  }

  function isVideo(path: string) {
    return /\.(mp4|mov|webm|mkv|avi)$/i.test(path)
  }

  // Videos stream over the localhost media server (mediaUrl); images use IPC
  // to avoid dev-mode routing issues (Vite dev server doesn't proxy /localfile).

  const mimeForPath = (path: string) => {
    const ext = path.split('.').pop()?.toLowerCase() ?? ''
    return ({ jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png',
              gif: 'image/gif', webp: 'image/webp', bmp: 'image/bmp',
              tiff: 'image/tiff', tif: 'image/tiff' })[ext] ?? 'image/jpeg'
  }

  const current = $derived($galleryImages[$galleryIndex] ?? '')

  let imageSrc = $state('')
  $effect(() => {
    const path = current
    if (!path || isVideo(path)) { imageSrc = ''; return }
    imageSrc = ''
    ReadFileBase64(path).then(b64 => {
      imageSrc = `data:${mimeForPath(path)};base64,${b64}`
    }).catch(() => { imageSrc = '' })
  })
  const filename = $derived(current.split('/').pop() ?? '')
  const total = $derived($galleryImages.length)
</script>

<svelte:window onkeydown={handleKey} />

{#if $galleryMode && current}
  <div class="gallery">
    <div class="toolbar">
      <span class="info">{filename} ({$galleryIndex + 1}/{total})</span>
      <button class="btn" onclick={() => slideshowActive = !slideshowActive}
        class:active={slideshowActive}>⏵</button>
      <button class="close-btn" onclick={exit} title="Close">✕</button>
    </div>

    <div class="viewer">
      {#if isVideo(current)}
        <video src={mediaUrl(current)} autoplay controls class="media"></video>
      {:else if imageSrc}
        <img src={imageSrc} alt={filename} class="media" />
      {:else}
        <div class="loading">Loading…</div>
      {/if}
      <!-- ponytail: no captions for local video; add track when serving subtitles -->

    </div>

    <div class="nav-prev" onclick={prev} onkeydown={(e) => e.key === 'Enter' && prev()} role="button" tabindex="0" aria-label="Previous">‹</div>
    <div class="nav-next" onclick={next} onkeydown={(e) => e.key === 'Enter' && next()} role="button" tabindex="0" aria-label="Next">›</div>
  </div>
{/if}

<style>
  .gallery {
    position: absolute;
    inset: 0;
    background: var(--background);
    display: flex;
    flex-direction: column;
    z-index: 100;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    height: 32px;
    background: var(--bg-raised);
    flex-shrink: 0;
    border-bottom: 1px solid var(--border);
  }
  .btn, .close-btn {
    background: none;
    border: none;
    color: var(--muted);
    cursor: pointer;
    font-size: 13px;
    line-height: 1;
    padding: 3px 6px;
    border-radius: 3px;
    transition: color 0.1s, background 0.1s;
  }
  .btn:hover, .close-btn:hover { color: var(--foreground); background: var(--bg-hover); }
  .btn.active { color: var(--accent); }
  .close-btn { margin-left: auto; }
  .info { color: var(--muted); font-size: 11px; flex: 1; text-align: center; font-family: "SF Mono", Menlo, monospace; }
  .viewer {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
  }
  .media {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
  .nav-prev, .nav-next {
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
    font-size: 48px;
    color: var(--muted);
    cursor: pointer;
    padding: 20px 10px;
    user-select: none;
    transition: color 0.15s;
  }
  .nav-prev:hover, .nav-next:hover { color: var(--foreground); }
  .nav-prev { left: 8px; }
  .nav-next { right: 8px; }
  .loading { color: rgba(255,255,255,0.4); font-size: 13px; }
</style>
