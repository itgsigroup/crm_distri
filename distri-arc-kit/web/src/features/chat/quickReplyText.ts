import type { QuickReply } from '../../api/types'

// Balasan cepat like WhatsApp Business (pure, tested; ADR 0065): "/" at the start of a word opens the team's saved
// texts; picking one puts its text in place of "/…" — the person still presses Kirim.

/** The "/…" being typed right before the caret (at the start or after a space or new line), or null. */
export function slashQuery(text: string, caret: number): { start: number; q: string } | null {
  const before = text.slice(0, caret)
  const m = /(^|\s)\/([^\s/]{0,24})$/.exec(before)
  if (!m) return null
  return { start: caret - m[2].length - 1, q: m[2] }
}

/** Quick replies for what was typed after "/": shortcuts that start with it, then shortcuts and texts that contain
 * it (case-insensitive); the list's own order (most used first) within each group; at most 8. */
export function matchQuick(items: QuickReply[], q: string): QuickReply[] {
  const s = q.trim().toLocaleLowerCase('id-ID')
  if (!s) return items.slice(0, 8)
  const starts = items.filter((x) => x.shortcut.startsWith(s))
  const rest = items.filter((x) => !x.shortcut.startsWith(s) && (x.shortcut.includes(s) || x.body.toLocaleLowerCase('id-ID').includes(s)))
  return [...starts, ...rest].slice(0, 8)
}

/** The text with the picked reply in place of the "/…" typed (the caret goes after it). */
export function insertQuick(text: string, at: { start: number; q: string }, body: string): { text: string; caret: number } {
  const end = at.start + 1 + at.q.length
  return { text: text.slice(0, at.start) + body + text.slice(end), caret: at.start + body.length }
}

/** A shortcut as the API keeps it: lower case, no "/", no spaces. */
export const cleanShortcut = (s: string) => s.trim().replace(/^\/+/, '').toLowerCase().replace(/\s+/g, '-')

/** A shortcut the API accepts: a letter or digit, then letters, digits, "-" or "_", at most 24. */
export const validShortcut = (s: string) => /^[a-z0-9][a-z0-9_-]{0,23}$/.test(s)
