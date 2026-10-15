import type { ThreadView } from '../../api/types'

// Chat inbox helpers (pure, tested): conversation filters with counts, the "waiting for a reply" timer and the
// lookup links shown next to a contact's number.

export type Filter = 'all' | 'unread' | 'unreplied' | 'dealer' | 'group_internal' | 'group_external' | 'new' | 'supplier'

// "Belum dibaca" and "Belum dibalas" cut across the kinds, like WhatsApp's Unread chip.
export const FILTERS: [Filter, string][] = [
  ['all', 'Semua'], ['unread', 'Belum dibaca'], ['unreplied', 'Belum dibalas'], ['dealer', 'Dealer'], ['group_internal', 'Grup internal'], ['group_external', 'Grup pelanggan'], ['new', 'Nomor baru'], ['supplier', 'Pemasok'],
]

type Filtered = Pick<ThreadView, 'kind' | 'group_kind' | 'unread' | 'waiting_since' | 'contact_type'>

/** A customer's message is still waiting for our answer (one-to-one chats; group chatter is not a request; a
 * supplier is not a customer — ADR 0035). */
export function unreplied(t: Pick<ThreadView, 'kind' | 'waiting_since' | 'contact_type'>): boolean {
  return t.kind !== 'group' && t.contact_type !== 'supplier' && !!t.waiting_since
}

export function filterOf(t: Pick<ThreadView, 'kind' | 'group_kind' | 'contact_type'>): Exclude<Filter, 'all' | 'unread' | 'unreplied'> {
  if (t.kind === 'group') return t.group_kind === 'external' ? 'group_external' : 'group_internal'
  if (t.contact_type === 'supplier') return 'supplier'
  return t.kind
}

/** Whether a conversation shows under a filter chip. */
export function inFilter(t: Filtered, f: Filter): boolean {
  if (f === 'all') return true
  if (f === 'unread') return t.unread > 0
  if (f === 'unreplied') return unreplied(t)
  return filterOf(t) === f
}

export function countFilters(list: Filtered[]): Record<Filter, number> {
  const c: Record<Filter, number> = { all: list.length, unread: 0, unreplied: 0, dealer: 0, group_internal: 0, group_external: 0, new: 0, supplier: 0 }
  for (const t of list) {
    c[filterOf(t)]++
    if (t.unread > 0) c.unread++
    if (unreplied(t)) c.unreplied++
  }
  return c
}

/** Search over the name, the subtitle and the last message (lower-cased, Indonesian locale). */
/** Folds the spellings of one Indonesian name: case, "oe" → "u", "j" → "y", doubled letters — Yohanes finds
 * Johannes, Yusuf finds Joesoef (the same rules as the server's searchKey). */
export function searchKey(s: string): string {
  return s.trim().toLocaleLowerCase('id-ID').replace(/oe/g, 'u').replace(/j/g, 'y').replace(/(\p{L})\1+/gu, '$1')
}

export function matchThread(t: Pick<ThreadView, 'title' | 'subtitle' | 'last_body'>, q: string): boolean {
  const s = searchKey(q)
  return !s || searchKey(`${t.title} ${t.subtitle} ${t.last_body}`).includes(s)
}

/** "12 mnt", "21 jam 44 mnt", "3 hr 2 jam" since the customer's unanswered message; tone grows with the wait. */
export function waitLabel(since: string | null | undefined, now: Date): { text: string; tone: 'good' | 'warn' | 'bad' } | null {
  if (!since) return null
  const ms = now.getTime() - new Date(since).getTime()
  if (ms < 0) return null // clock behind the message (frozen demo clock)
  const min = Math.floor(ms / 60000)
  const tone = min < 60 ? 'good' : min < 24 * 60 ? 'warn' : 'bad'
  if (min < 60) return { text: `${min} mnt`, tone }
  const h = Math.floor(min / 60)
  if (h < 24) return { text: `${h} jam ${min % 60} mnt`, tone }
  return { text: `${Math.floor(h / 24)} hr ${h % 24} jam`, tone }
}

/** "+62 812-3450-4471" from digits. */
export function fmtPhone(d: string | undefined): string {
  const x = (d ?? '').replace(/\D/g, '')
  if (!x) return ''
  if (!x.startsWith('62') || x.length < 10) return '+' + x
  const r = x.slice(2)
  return `+62 ${r.slice(0, 3)}-${r.slice(3, 7)}-${r.slice(7)}`
}

/** Quick lookups for a contact: web search, map, Truecaller and a wa.me link. */
export function lookupLinks(name: string, phone: string | undefined, city?: string): { key: string; label: string; href: string }[] {
  const p = (phone ?? '').replace(/\D/g, '')
  const q = encodeURIComponent([name, city].filter(Boolean).join(' '))
  const out = [
    { key: 'google', label: 'Google', href: `https://www.google.com/search?q=${p ? encodeURIComponent('+' + p) + '+' : ''}${q}` },
    { key: 'maps', label: 'Maps', href: `https://www.google.com/maps/search/${q}` },
  ]
  if (p) {
    out.push({ key: 'truecaller', label: 'Truecaller', href: `https://www.truecaller.com/search/id/${p}` })
    out.push({ key: 'wa', label: 'wa.me', href: `https://wa.me/${p}` })
  }
  return out
}
