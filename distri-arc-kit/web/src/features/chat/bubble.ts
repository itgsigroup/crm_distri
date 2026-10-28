import type { MediaView, MessageView } from '../../api/types'

// Media and edits in a bubble (ADR 0029).

/** The caption of a media bubble: the body without the "[gambar]" / "[dokumen BoQ.pdf]" label the server adds. */
export function mediaCaption(body: string, media?: MediaView): string {
  return media ? body.replace(/^\[[^\]]*\]\s*/, '') : body
}

/** A contact card's bubble text "Pak Budi · +62812…" as its name and number (number "" when the card had none). */
export function contactCard(caption: string): { name: string; phone: string } {
  const m = /^(.*?)\s*·\s*(\+?[\d\s-]{6,})$/.exec(caption.trim())
  return m ? { name: m[1].trim(), phone: m[2].replace(/[^\d+]/g, '') } : { name: caption.trim(), phone: '' }
}

const KIND_LABEL: Record<string, string> = { image: 'Foto', video: 'Video', gif: 'GIF', audio: 'Audio', document: 'Dokumen', sticker: 'Stiker', location: 'Lokasi', contact: 'Kontak', poll: 'Polling', call: 'Panggilan', system: 'Info grup' }

/** A call bubble (ADR 0062): its line and tone — a missed or rejected call is a customer still waiting. */
export function callLine(md: MediaView): { text: string; tone: 'bad' | 'good' } {
  const kind = md.video ? 'Panggilan video' : 'Panggilan suara'
  if (md.call === 'answered') return { text: md.seconds ? `${kind} · ${fmtDuration(md.seconds)}` : `${kind} · diangkat di HP`, tone: 'good' }
  return { text: `${kind} ${md.call === 'rejected' ? 'ditolak' : 'tak terjawab'}`, tone: 'bad' }
}

/** "Pesan suara", "Foto", "BoQ.pdf"… */
export function mediaName(m: MediaView): string {
  if (m.kind === 'audio' && m.ptt) return 'Pesan suara'
  if (m.kind === 'document' && m.file_name) return m.file_name
  return KIND_LABEL[m.kind] ?? 'Lampiran'
}

/** 0:07, 1:32, 12:05 */
export function fmtDuration(sec?: number): string {
  if (!sec || sec < 0) return ''
  const m = Math.floor(sec / 60)
  return `${m}:${String(Math.floor(sec % 60)).padStart(2, '0')}`
}

/** 820 B, 120 KB, 3,4 MB */
export function fmtSize(b?: number): string {
  if (!b || b <= 0) return ''
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${Math.round(b / 1024)} KB`
  return `${(b / 1024 / 1024).toLocaleString('id-ID', { maximumFractionDigits: 1 })} MB`
}

/** WhatsApp accepts edits for 15 minutes after sending. */
export const EDIT_WINDOW_MS = 15 * 60_000

/** Whether the edit button shows now: queued replies always, sent texts until 15 minutes have passed. */
export function editableNow(m: Pick<MessageView, 'can_edit' | 'status' | 'sent_at'>, now: Date): boolean {
  if (!m.can_edit) return false
  return m.status === 'pending' || now.getTime() - new Date(m.sent_at).getTime() < EDIT_WINDOW_MS
}

/** "Hapus untuk semua orang" is offered for 48 hours after sending (WhatsApp allows about 2 days). */
export const DELETE_WINDOW_MS = 48 * 3600_000

/** Whether "Hapus untuk semua orang" shows now: queued replies always, sent messages for 48 hours. */
const FORWARD_KINDS = new Set(['image', 'video', 'gif', 'audio', 'document'])

/** Teruskan: a stored message with text, or a file WhatsApp still holds; not one still sending, nor a poll, location,
 * contact or sticker (the server refuses those — forward them from the phone). */
export function forwardable(m: Pick<MessageView, 'body' | 'media' | 'status' | 'deleted'>): boolean {
  if (m.deleted || m.status === 'pending' || m.status === 'failed') return false
  if (m.media) return FORWARD_KINDS.has(m.media.kind) && m.media.viewable
  return m.body.trim() !== ''
}

export function deletableNow(m: Pick<MessageView, 'can_delete' | 'status' | 'sent_at'>, now: Date): boolean {
  if (!m.can_delete) return false
  return m.status === 'pending' || now.getTime() - new Date(m.sent_at).getTime() < DELETE_WINDOW_MS
}
