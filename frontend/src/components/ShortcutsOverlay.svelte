<script lang="ts">
  import { listCommands } from '../lib/commands'
  import { customKeybinds } from '../lib/keymap'
  import { IconKeyboard } from '@tabler/icons-svelte'
  import { modalA11y } from '../lib/a11y'

  let { onClose }: { onClose: () => void } = $props()

  const CATEGORY_BY_PREFIX: Record<string, string> = {
    file: 'Editor', tab: 'Editor', search: 'Editor', settings: 'Panels',
    sidebar: 'Panels', terminal: 'Terminal', git: 'Git', debug: 'Debug',
  }
  function categoryOf(id: string): string {
    return CATEGORY_BY_PREFIX[id.split('.')[0]] ?? 'Other'
  }

  // native/window-level shortcuts registered directly in App.svelte — not in
  // the command registry (no palette entry), so listed here by hand.
  const nativeShortcuts: { title: string; combo: string; category: string }[] = [
    { title: 'Command Palette', combo: '⌘⇧P', category: 'Editor' },
    { title: 'Go to File', combo: '⌘P', category: 'Editor' },
    { title: 'Search in Files', combo: '⌘⇧F', category: 'Editor' },
    { title: 'New Terminal', combo: '⌘⇧T', category: 'Terminal' },
    { title: 'Close Tab', combo: '⌘W', category: 'Panels' },
    { title: 'Next / Previous Tab', combo: '⌘⇧] / ⌘⇧[', category: 'Panels' },
    { title: 'Toggle Panel Focus', combo: '⌘T', category: 'Panels' },
    { title: 'This Shortcuts Overlay', combo: '?', category: 'Panels' },
  ]

  const grouped = $derived.by(() => {
    const rows = listCommands().map(c => ({
      title: c.title,
      combo: $customKeybinds[c.id]?.toUpperCase() ?? '',
      category: categoryOf(c.id),
    })).filter(r => r.combo)
    const all = [...nativeShortcuts, ...rows]
    const byCat = new Map<string, typeof all>()
    for (const r of all) {
      if (!byCat.has(r.category)) byCat.set(r.category, [])
      byCat.get(r.category)!.push(r)
    }
    return [...byCat.entries()]
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="overlay" onclick={onClose}>
  <div class="panel" role="dialog" aria-modal="true" aria-label="Keyboard shortcuts" tabindex="-1" use:modalA11y={onClose} onclick={(e) => e.stopPropagation()}>
    <div class="header">
      <IconKeyboard size={14} />
      <span class="title">Keyboard Shortcuts</span>
      <button class="close" onclick={onClose} aria-label="Close">✕</button>
    </div>
    <div class="body">
      {#each grouped as [category, rows]}
        <div class="cat">
          <div class="cat-title">{category}</div>
          {#each rows as r}
            <div class="row">
              <span class="cmd">{r.title}</span>
              <span class="combo">{r.combo}</span>
            </div>
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed; inset: 0; z-index: 9500;
    background: rgba(0,0,0,0.45);
    display: flex; align-items: center; justify-content: center;
  }
  .panel {
    width: 560px; max-height: 70vh; background: var(--bg-raised);
    border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.5);
    display: flex; flex-direction: column; overflow: hidden;
  }
  .header {
    display: flex; align-items: center; gap: 6px;
    padding: 10px 14px 8px; border-bottom: 1px solid var(--border);
    color: var(--muted); flex-shrink: 0;
  }
  .title { font-size: 12px; font-weight: 600; color: var(--muted); flex: 1; }
  .close { background: none; border: none; color: var(--muted); cursor: pointer; font-size: 13px; padding: 2px 5px; border-radius: 3px; }
  .close:hover { color: var(--foreground); background: var(--bg-hover); }
  .body { overflow-y: auto; padding: 8px 14px 14px; display: grid; grid-template-columns: 1fr 1fr; gap: 0 24px; }
  .cat { margin-top: 10px; }
  .cat-title { font-size: 10px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); margin-bottom: 4px; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 0; font-size: 12px; }
  .cmd { color: var(--foreground); }
  .combo { color: var(--muted); font-family: "SF Mono", Menlo, monospace; font-size: 11px; white-space: nowrap; }
</style>
