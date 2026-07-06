<script lang="ts">
  import { ReadFileBase64 } from '../lib/wails'

  let { path }: { path: string } = $props()

  const ext = $derived(path.split('.').pop()?.toLowerCase() ?? '')
  const isVideo = $derived(/^(mp4|mov|webm|mkv|avi)$/.test(ext))
  const isSvg   = $derived(ext === 'svg')

  const mimeMap: Record<string, string> = {
    jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png',
    gif: 'image/gif',  webp: 'image/webp', bmp: 'image/bmp',
    tiff: 'image/tiff', tif: 'image/tiff', ico: 'image/x-icon',
    svg: 'image/svg+xml',
  }
  const mime = $derived(mimeMap[ext] ?? 'image/jpeg')

  let src = $state('')
  $effect(() => {
    const p = path
    if (!p || isVideo) { src = ''; return }
    src = ''
    ReadFileBase64(p).then(b64 => {
      src = `data:${mime};base64,${b64}`
    }).catch(() => { src = '' })
  })

  const videoUrl = $derived(`/localfile?path=${encodeURIComponent(path)}`)
  const filename = $derived(path.split('/').pop() ?? '')
</script>

<div class="viewer">
  {#if isVideo}
    <video src={videoUrl} controls class="media"></video>
    <!-- ponytail: no captions for local video -->
  {:else if src}
    <img {src} alt={filename} class="media" />
  {:else}
    <div class="loading">Loading…</div>
  {/if}
</div>

<style>
  .viewer {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--background);
    overflow: hidden;
  }
  .media {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
  .loading {
    color: var(--muted);
    font-size: 13px;
  }
</style>
