// Pinned messages and labels in Chat (pure, tested).

import type { ChatLabel, ChatPin } from '../../api/types'

/** Colors offered for a new label (WhatsApp Business palette). */
export const LABEL_COLORS = ['#34B7F1', '#25D366', '#F5B400', '#FF7043', '#E91E63', '#8E7CC3', '#00A884', '#607D8B']

/** The labels of the given ids, in the team's order; ids of deleted labels are dropped. */
export function labelsOf(ids: string[] | undefined, all: ChatLabel[]): ChatLabel[] {
  if (!ids?.length) return []
  const want = new Set(ids)
  return all.filter((l) => want.has(l.id))
}

/** A chat carries the label (the list's label filter; '' = every chat). */
export function hasLabel(t: { labels?: string[] }, id: string): boolean {
  return !id || !!t.labels?.includes(id)
}

/** The next pin the bar shows after a click (it cycles, newest first, like WhatsApp). */
export function nextPin(cur: number, count: number): number {
  return count === 0 ? 0 : (cur + 1) % count
}

/** One line of a pinned message for the bar. */
export function pinLine(p: Pick<ChatPin, 'body' | 'media' | 'from_name' | 'direction'>): string {
  const text = p.body.replace(/\s+/g, ' ').trim() || (p.media ? `[${p.media.kind}]` : '')
  return text.length > 90 ? text.slice(0, 89) + '…' : text
}

/** The chat list with the chats this person pinned on top (last pinned first), the rest in their order. */
export function pinnedFirst<T extends { pinned_at?: string }>(list: T[]): T[] {
  const pinned = list.filter((t) => t.pinned_at).sort((a, b) => (b.pinned_at ?? '').localeCompare(a.pinned_at ?? ''))
  return pinned.length ? [...pinned, ...list.filter((t) => !t.pinned_at)] : list
}
