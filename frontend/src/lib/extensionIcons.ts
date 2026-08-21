// Extension manifests are JSON, so a panel's icon is a string name rather
// than an imported component. This is the allow-list of @tabler/icons-svelte
// icons a manifest may reference — unknown/missing names fall back to
// IconPuzzle. Extend as extensions need new icons.
import {
  IconPuzzle, IconTimeline, IconTicket, IconMessage, IconBrandSlack,
  IconListCheck, IconLayoutKanban, IconBrandGithub, IconBell, IconCalendarEvent,
} from '@tabler/icons-svelte'

const ICONS: Record<string, any> = {
  IconTimeline, IconTicket, IconMessage, IconBrandSlack,
  IconListCheck, IconLayoutKanban, IconBrandGithub, IconBell, IconCalendarEvent,
}

export function resolveExtensionIcon(name: string | undefined): any {
  return (name && ICONS[name]) || IconPuzzle
}
