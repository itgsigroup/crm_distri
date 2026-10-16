import type { Mention } from '../../api/types'

// Tags in group chats like WhatsApp (pure, tested): a message carries "@<number>" for each member it tags and the
// list of tags; Chat shows "@Name". The composer offers the group's members after "@" and sends the numbers.

export interface MentionPart { t: string; tag?: boolean }

/** The text cut into plain parts and tags ("@6281…" → "@Budi"); "@digits" with no matching tag stays as written. */
export function mentionParts(text: string, mentions?: Mention[] | null): MentionPart[] {
  if (!mentions?.length || !text.includes('@')) return [{ t: text }]
  const out: MentionPart[] = []
  let last = 0
  for (const m of text.matchAll(/@(\d{5,20})/g)) {
    const tag = mentions.find((x) => x.token === m[1])
    if (!tag) continue
    const at = m.index ?? 0
    if (at > last) out.push({ t: text.slice(last, at) })
    out.push({ t: '@' + (tag.name || tag.number || tag.token), tag: true })
    last = at + m[0].length
  }
  if (last < text.length) out.push({ t: text.slice(last) })
  return out.length ? out : [{ t: text }]
}

/** The "@…" being typed right before the caret (at the start or after a space), or null. */
export function mentionQuery(text: string, caret: number): { start: number; q: string } | null {
  const before = text.slice(0, caret)
  const m = /(^|\s)@([^\s@]{0,30})$/.exec(before)
  if (!m) return null
  return { start: caret - m[2].length - 1, q: m[2] }
}

/** A group member: whom "@" tags and "Anggota" lists (thread_id: the one-to-one chat with them, when it exists). */
export interface Member { number: string; name: string; admin?: boolean; gsi?: boolean; thread_id?: string }

/** Members whose name or number matches what was typed after "@" (case-insensitive), at most 8. */
export function matchMembers(members: Member[], q: string): Member[] {
  const s = q.trim().toLocaleLowerCase('id-ID')
  return members.filter((m) => !s || m.name.toLocaleLowerCase('id-ID').includes(s) || m.number.includes(s)).slice(0, 8)
}

/** The text with a picked member in place of the "@…" typed: "@Name " (the caret goes after it). */
export function insertMention(text: string, at: { start: number; q: string }, m: Member): { text: string; caret: number } {
  const tag = '@' + m.name + ' '
  const end = at.start + 1 + at.q.length
  return { text: text.slice(0, at.start) + tag + text.slice(end), caret: at.start + tag.length }
}

/** What goes to WhatsApp: each picked "@Name" still in the text becomes "@<number>" (longest names first). */
export function toWire(text: string, picks: Member[]): string {
  let out = text
  for (const p of [...picks].sort((a, b) => b.name.length - a.name.length)) out = out.split('@' + p.name).join('@' + p.number)
  return out
}
