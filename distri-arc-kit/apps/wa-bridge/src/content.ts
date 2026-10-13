import { spawn } from 'node:child_process'
import type { AnyMessageContent } from 'baileys'

/**
 * What a send from Chat carries besides text (the API's outbox payload): a file (photo, video, document), a voice
 * note recorded in the browser, or a poll. The file travels base64 in the signed request and is never stored here.
 */
export interface Content {
  kind: 'image' | 'video' | 'document' | 'audio' | 'poll'
  mime?: string
  file_name?: string
  data?: string // base64
  ptt?: boolean
  seconds?: number
  poll?: { name: string; options: string[]; multi?: boolean }
}

/** The largest file a send may carry (WhatsApp accepts more for documents; Chat keeps uploads small). */
export const MAX_CONTENT_BYTES = 25 * 1024 * 1024

export class ContentError extends Error {}

/** Converts a browser recording (webm/opus, mp4/aac…) to ogg/opus, the only voice-note format WhatsApp phones play. */
export type AudioConverter = (data: Buffer, mime: string) => Promise<Buffer>

export const ffmpegToOgg: AudioConverter = (data, mime) => {
  if (/^audio\/ogg/i.test(mime)) return Promise.resolve(data)
  return new Promise((resolve, reject) => {
    const p = spawn('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-i', 'pipe:0', '-vn', '-c:a', 'libopus', '-b:a', '32k', '-ac', '1', '-f', 'ogg', 'pipe:1'])
    const out: Buffer[] = []
    let err = ''
    p.stdout.on('data', (c: Buffer) => out.push(c))
    p.stderr.on('data', (c: Buffer) => { err += c.toString() })
    p.on('error', (e) => reject(new ContentError('ffmpeg tidak tersedia untuk mengubah pesan suara: ' + e.message)))
    p.on('close', (code) => (code === 0 && out.length ? resolve(Buffer.concat(out)) : reject(new ContentError('pesan suara gagal diubah ke ogg: ' + err.trim().slice(0, 200)))))
    p.stdin.on('error', () => {})
    p.stdin.end(data)
  })
}

/** Builds the Baileys message for a content (text = caption; a poll ignores it). */
export async function toMessage(c: Content, text: string, convert: AudioConverter): Promise<AnyMessageContent> {
  if (c.kind === 'poll') {
    const p = c.poll
    const options = (p?.options ?? []).map((o) => o.trim()).filter(Boolean)
    if (!p?.name?.trim() || options.length < 2 || options.length > 12) throw new ContentError('polling butuh pertanyaan dan 2–12 pilihan')
    return { poll: { name: p.name.trim(), values: options, selectableCount: p.multi ? options.length : 1 } }
  }
  const data = Buffer.from(c.data ?? '', 'base64')
  if (!data.length) throw new ContentError('file kosong')
  if (data.length > MAX_CONTENT_BYTES) throw new ContentError('file lebih dari 25 MB')
  const mime = c.mime || 'application/octet-stream'
  const caption = text.trim() || undefined
  switch (c.kind) {
    case 'image':
      return { image: data, mimetype: mime, caption }
    case 'video':
      return { video: data, mimetype: mime, caption }
    case 'audio':
      if (c.ptt) return { audio: await convert(data, mime), mimetype: 'audio/ogg; codecs=opus', ptt: true, seconds: c.seconds }
      return { audio: data, mimetype: mime }
    default:
      return { document: data, mimetype: mime, fileName: c.file_name || 'lampiran', caption }
  }
}

/** The text the anti-ban guard compares between chats (identical-text pattern): caption, poll question, or file. */
export function guardText(c: Content | undefined, text: string): string {
  if (!c) return text
  if (c.kind === 'poll') return 'poll:' + (c.poll?.name ?? '')
  return text || `${c.kind}:${c.file_name ?? ''}:${(c.data ?? '').length}`
}
