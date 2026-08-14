// Terminal pairing (phase 1 of Live Share — see internal/liveshare). One
// share per terminal id; guest list updates push in live over the
// 'liveshare:guests' event the Go side emits on join/leave/permission change.
import { writable } from 'svelte/store'
import { on, StartLiveShare, StopLiveShare, GetLiveShareGuests, SetLiveShareGuestPermission } from './wails'
import type { LiveShareGuest } from './wails'

export interface ShareState { url: string; guests: LiveShareGuest[] }

// terminalId -> share state; absent = not currently shared
export const liveShares = writable<Map<string, ShareState>>(new Map())

on('liveshare:guests', (update: { terminalId: string; guests: LiveShareGuest[] }) => {
  liveShares.update(m => {
    const cur = m.get(update.terminalId)
    if (!cur) return m // guest event for a session we don't think is active — ignore
    const next = new Map(m)
    next.set(update.terminalId, { ...cur, guests: update.guests })
    return next
  })
})

export async function startShare(terminalId: string): Promise<string> {
  const url = await StartLiveShare(terminalId)
  const guests = (await GetLiveShareGuests(terminalId).catch(() => [])) ?? []
  liveShares.update(m => {
    const next = new Map(m)
    next.set(terminalId, { url, guests })
    return next
  })
  return url
}

export function stopShare(terminalId: string) {
  StopLiveShare(terminalId).catch(() => {})
  liveShares.update(m => {
    const next = new Map(m)
    next.delete(terminalId)
    return next
  })
}

export function setGuestCanType(terminalId: string, guestId: string, canType: boolean) {
  SetLiveShareGuestPermission(terminalId, guestId, canType).catch(() => {})
}
