import type { ThreadView } from '../../api/types'

// Chat inbox helpers (pure, tested): conversation filters with counts, the "waiting for a reply" timer and the
// lookup links shown next to a contact's number.

export type Filter = 'all' | 'dealer' | 'group_internal' | 'group_external' | 'new'

export const FILTERS: [Filter, string][] = [
  ['all', 'Semua'], ['dealer', 'Dealer'], ['group_internal', 'Grup internal'], ['group_external', 'Grup eksternal'], ['new', 'Nomor baru'],
]

export function filterOf(t: Pick<ThreadView, 'kind' | 'group_kind'>): Exclude<Filter, 'all'> {
  if (t.kind === 'group') return t.group_kind === 'external' ? 'group_external' : 'group_internal'
  return t.kind
}

export function countFilters(list: Pick<ThreadView, 'kind' | 'group_kind'>[]): Record<Filter, number> {
  const c: Record<Filter, number> = { all: list.length, dealer: 0, group_internal: 0, group_external: 0, new: 0 }
  for (const t of list) c[filterOf(t)]++
  return c
}

/** Search over the name, the subtitle and the last message (lower-cased, Indonesian locale). */
export function matchThread(t: Pick<ThreadView, 'title' | 'subtitle' | 'last_body'>, q: string): boolean {
  const s = q.trim().toLocaleLowerCase('id-ID')
  return !s || `${t.title} ${t.subtitle} ${t.last_body}`.toLocaleLowerCase('id-ID').includes(s)
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
