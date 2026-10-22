import { useSyncExternalStore } from 'react'

// Chat drafts (Sam 22 Oct, like WhatsApp): what is typed in a chat's reply box stays when another chat is opened —
// and after a reload — until it is sent or cleared; the chat list shows "Draf: …". Kept per browser.

const KEY = 'chat.drafts'
const MAX = 200 // the newest drafts kept

let drafts: Record<string, string> = load()
const listeners = new Set<() => void>()

function load(): Record<string, string> {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? '{}')
    return v && typeof v === 'object' ? (v as Record<string, string>) : {}
  } catch {
    return {}
  }
}

export function getDraft(thread: string): string {
  return drafts[thread] ?? ''
}

/** Keeps a chat's draft; blank text removes it. */
export function setDraft(thread: string, text: string) {
  const blank = !text.trim()
  if ((blank && !(thread in drafts)) || drafts[thread] === text) return
  const next = { ...drafts }
  delete next[thread] // re-added last: the newest draft is kept when trimming
  if (!blank) next[thread] = text
  const keys = Object.keys(next)
  for (const k of keys.slice(0, Math.max(0, keys.length - MAX))) delete next[k]
  drafts = next
  try {
    localStorage.setItem(KEY, JSON.stringify(drafts))
  } catch { /* kept for this tab only */ }
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => { listeners.delete(l) }
}

/** Every chat's draft, live (the chat list). */
export function useDrafts(): Record<string, string> {
  return useSyncExternalStore(subscribe, () => drafts)
}
