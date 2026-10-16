// Links in a bubble (pure, tested): "https://…", "http://…" and "www.…" become links that open in a new tab, like
// WhatsApp. Only web addresses: nothing else (javascript:, file:) is ever made clickable.

export interface LinkPart { t: string; href?: string }

const URL_RE = /\b(?:https?:\/\/|www\.)[^\s<>"']+/gi

/** The text cut into plain parts and links; a sentence's closing punctuation ("…lihat x.com/a." or "(x.com/a)")
 * stays out of the link. */
export function linkParts(text: string): LinkPart[] {
  if (!/https?:\/\/|www\./i.test(text)) return [{ t: text }]
  const out: LinkPart[] = []
  let last = 0
  for (const m of text.matchAll(URL_RE)) {
    let url = m[0]
    // trailing punctuation, and a closing bracket the link does not open
    while (/[.,;:!?)\]}*_~]$/.test(url)) {
      const c = url[url.length - 1]
      const open = { ')': '(', ']': '[', '}': '{' }[c]
      if (open && url.split(open).length >= url.split(c).length) break
      url = url.slice(0, -1)
    }
    if (!url || /^www\.$/i.test(url)) continue
    const at = m.index ?? 0
    if (at > last) out.push({ t: text.slice(last, at) })
    out.push({ t: url, href: /^https?:\/\//i.test(url) ? url : 'https://' + url })
    last = at + url.length
  }
  if (last < text.length) out.push({ t: text.slice(last) })
  return out.length ? out : [{ t: text }]
}
