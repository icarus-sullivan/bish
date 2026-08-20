<script lang="ts">
  // shadcn-svelte's sonner wrapper, ported to bish's own CSS-var theme system
  // instead of Tailwind/mode-watcher: svelte-sonner ships full default CSS
  // (Toaster.svelte's <style> block) driven entirely by --normal-*/--success-*/
  // --error-*/--warning-*/--info-* custom properties, so re-pointing those at
  // bish's tokens is the whole integration — no custom toast CSS needed here.
  import { Toaster as Sonner, type ToasterProps } from 'svelte-sonner'
  import { currentThemeName } from '../lib/stores'

  let { ...restProps }: ToasterProps = $props()
</script>

<Sonner
  theme={$currentThemeName === 'light' ? 'light' : 'dark'}
  richColors
  position="bottom-center"
  closeButton
  style="
    --border-radius: 8px;
    --normal-bg: var(--bg-raised);
    --normal-border: var(--border);
    --normal-text: var(--foreground);
    --success-bg: var(--bg-raised);
    --success-border: var(--success);
    --success-text: var(--success);
    --error-bg: var(--bg-raised);
    --error-border: var(--error);
    --error-text: var(--error);
    --warning-bg: var(--bg-raised);
    --warning-border: var(--warning);
    --warning-text: var(--warning);
    --info-bg: var(--bg-raised);
    --info-border: var(--accent);
    --info-text: var(--accent);
  "
  {...restProps}
/>
