// Cari di dalam chat (pure, tested): the parts of a text that match, and moving between results.

export interface ChatHit { id: string; sent_at: string; direction: 'in' | 'out'; from_name: string; snippet: string }

/** The text cut into plain and matching parts, case-insensitive; the spaces of the query are matched as typed
 * (one space between words, as the server searches). */
export function marks(text: string, q: string): { t: string; hit: boolean }[] {
  const needle = q.trim().replace(/\s+/g, ' ')
  if (needle.length < 2 || !text) return [{ t: text, hit: false }]
  const re = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/ /g, '\\s+'), 'gi')
  const out: { t: string; hit: boolean }[] = []
  let last = 0
  for (const m of text.matchAll(re)) {
    const at = m.index ?? 0
    if (at > last) out.push({ t: text.slice(last, at), hit: false })
    out.push({ t: m[0], hit: true })
    last = at + m[0].length
  }
  if (last < text.length) out.push({ t: text.slice(last), hit: false })
  return out.length ? out : [{ t: text, hit: false }]
}

/** The next result: results are newest first, so "up" (older) is the next index and "down" (newer) the previous;
 * both stop at the ends, like WhatsApp. */
export function step(cur: number, count: number, dir: 'up' | 'down'): number {
  if (count === 0) return -1
  if (cur < 0) return 0
  return dir === 'up' ? Math.min(count - 1, cur + 1) : Math.max(0, cur - 1)
}

/** "3 dari 12", "Tidak ada hasil". */
export function countLabel(cur: number, count: number, capped: boolean): string {
  if (count === 0) return 'Tidak ada hasil'
  return `${cur < 0 ? count : cur + 1} dari ${count}${capped ? '+' : ''}`
}
