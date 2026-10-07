// Extension manifests are JSON, so a panel's icon is a string: a name from
// this allow-list of @tabler/icons-svelte icons, an http(s) URL, base64
// image data, or a local image file (see extensionIconSource). Unknown
// names fall back to IconPuzzle. Named imports only — `import *` would
// bundle every tabler icon. Extend as needed.
import DOMPurify from 'dompurify'
import type { ExtContribution } from './wails'
import {
  IconPuzzle, IconTimeline, IconTicket, IconMessage, IconBrandSlack,
  IconListCheck, IconLayoutKanban, IconBrandGithub, IconBell, IconCalendarEvent,
  IconDatabase, IconCloud, IconRobot, IconChartBar, IconNotes, IconBrandGitlab,
  IconMail, IconTerminal, IconWorld, IconKey, IconBook, IconBolt, IconStar,
  IconMusic, IconPhoto, IconShield, IconServer, IconApi, IconBrandDocker, IconCpu,
} from '@tabler/icons-svelte'

const ICONS: Record<string, any> = {
  IconTimeline, IconTicket, IconMessage, IconBrandSlack,
  IconListCheck, IconLayoutKanban, IconBrandGithub, IconBell, IconCalendarEvent,
  IconDatabase, IconCloud, IconRobot, IconChartBar, IconNotes, IconBrandGitlab,
  IconMail, IconTerminal, IconWorld, IconKey, IconBook, IconBolt, IconStar,
  IconMusic, IconPhoto, IconShield, IconServer, IconApi, IconBrandDocker, IconCpu,
}

export function resolveExtensionIcon(name: string | undefined): any {
  return (name && ICONS[name]) || IconPuzzle
}

const sanitizeSVG = (svg: string) => DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } })

// Non-tabler icon forms: `svg` = sanitized inline markup (themes via
// currentColor), `src` = an <img> source. Both '' = render the tabler
// component from resolveExtensionIcon instead.
//   - local file → already read by the Go side into iconSvg / iconSrc
//   - http(s) URL / data:image/… URI → <img src>
//   - raw base64 → inline if it decodes to SVG markup, else an image
export function extensionIconSource(p: ExtContribution): { svg: string; src: string } {
  if (p.iconSvg) return { svg: sanitizeSVG(p.iconSvg), src: '' }
  if (p.iconSrc) return { svg: '', src: p.iconSrc }
  const icon = (p.icon ?? '').trim()
  // tabler names are valid base64 alphabet too — rule them out first
  if (/^Icon[A-Z]\w*$/.test(icon)) return { svg: '', src: '' }
  if (/^https?:\/\//i.test(icon) || /^data:image\//i.test(icon)) return { svg: '', src: icon }
  if (icon.length >= 16 && /^[A-Za-z0-9+/\s]+=*$/.test(icon)) {
    try {
      const text = atob(icon.replace(/\s+/g, ''))
      if (/^\s*(<\?xml[^>]*>\s*)?<svg[\s>]/i.test(text)) return { svg: sanitizeSVG(text), src: '' }
      return { svg: '', src: `data:image/png;base64,${icon.replace(/\s+/g, '')}` }
    } catch {}
  }
  return { svg: '', src: '' }
}
