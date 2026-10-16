// WhatsApp text formatting (pure, tested): *bold*, _italic_, ~strikethrough~, ```monospace```, `code` and "> " quote
// lines — shown in bubbles as WhatsApp shows them, and the composer's shortcuts that put the markers around a
// selection. The text sent keeps the markers: the customer's phone formats it.

export interface FmtPart { t: string; b?: boolean; i?: boolean; s?: boolean; m?: boolean }
type Style = Omit<FmtPart, 't'>

const MARKERS: [string, keyof Style][] = [['```', 'm'], ['*', 'b'], ['_', 'i'], ['~', 's'], ['`', 'm']]

// a marker opens after the start, a space or punctuation, and closes before the end, a space or punctuation
const edge = (c: string | undefined) => c === undefined || /[\s\p{P}\p{S}]/u.test(c)

function closing(text: string, from: number, mk: string): number {
  for (let j = text.indexOf(mk, from); j >= 0; j = text.indexOf(mk, j + 1)) {
    if (mk.length === 1 && text.slice(from, j).includes('\n')) return -1 // one line, like WhatsApp
    if (j > from && !/\s/.test(text[j - 1]) && edge(text[j + mk.length])) return j
  }
  return -1
}

/** The text cut into parts with their styles; markers that do not pair stay as written. */
export function formatParts(text: string, style: Style = {}): FmtPart[] {
  const out: FmtPart[] = []
  let plain = ''
  const flush = () => {
    if (plain) out.push({ t: plain, ...style })
    plain = ''
  }
  for (let i = 0; i < text.length;) {
    let done = false
    if (!style.m) {
      for (const [mk, k] of MARKERS) {
        if (!text.startsWith(mk, i) || !edge(text[i - 1]) || /\s/.test(text[i + mk.length] ?? ' ')) continue
        if (style[k] && k !== 'm') continue
        const j = closing(text, i + mk.length, mk)
        if (j < 0) continue
        flush()
        const inner = text.slice(i + mk.length, j)
        if (k === 'm') out.push({ t: inner, ...style, m: true })
        else out.push(...formatParts(inner, { ...style, [k]: true }))
        i = j + mk.length
        done = true
        break
      }
    }
    if (!done) plain += text[i++]
  }
  flush()
  return out
}

/** The text's lines, "> " quote lines marked (the marker dropped), like WhatsApp's quote. */
export function quoteLines(text: string): { t: string; quote: boolean }[] {
  return text.split('\n').map((l) => (/^>\s/.test(l) ? { t: l.replace(/^>\s/, ''), quote: true } : { t: l, quote: false }))
}

export type Mark = '*' | '_' | '~' | '```'

/** The composer's shortcut: the selection wrapped in a marker (spaces at its ends stay outside, as WhatsApp needs),
 * or unwrapped when it already is; with nothing selected, the pair with the caret between. */
export function wrapSelection(text: string, start: number, end: number, mk: Mark): { text: string; start: number; end: number } {
  while (start < end && /\s/.test(text[start])) start++
  while (end > start && /\s/.test(text[end - 1])) end--
  const sel = text.slice(start, end)
  const n = mk.length
  if (text.slice(start - n, start) === mk && text.slice(end, end + n) === mk) {
    return { text: text.slice(0, start - n) + sel + text.slice(end + n), start: start - n, end: end - n }
  }
  if (sel.length > 2 * n && sel.startsWith(mk) && sel.endsWith(mk)) {
    const inner = sel.slice(n, -n)
    return { text: text.slice(0, start) + inner + text.slice(end), start, end: start + inner.length }
  }
  return { text: text.slice(0, start) + mk + sel + mk + text.slice(end), start: start + n, end: end + n }
}

/** Ctrl/⌘ + B, I, Shift+X, Shift+M → the marker (WhatsApp Web's shortcuts), else null. */
export function shortcutMark(e: { key: string; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean; altKey: boolean }): Mark | null {
  if (!(e.ctrlKey || e.metaKey) || e.altKey) return null
  const k = e.key.toLowerCase()
  if (!e.shiftKey && k === 'b') return '*'
  if (!e.shiftKey && k === 'i') return '_'
  if (e.shiftKey && k === 'x') return '~'
  if (e.shiftKey && k === 'm') return '```'
  return null
}
