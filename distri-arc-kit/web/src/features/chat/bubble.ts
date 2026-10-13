import type { MediaView, MessageView, MsgAnalysis } from '../../api/types'
import { waitLabel } from '../wa/wa'

// Analisa WhatsApp per bubble (ADR 0027): the line under each message.

export interface BubbleLine {
  /** reply state: "dibalas 25 menit kemudian", "menunggu 5 jam — perlu dibalas" */
  state: string
  tone: 'good' | 'warn' | 'bad' | 'neutral'
  /** customer's request ("Minta order · 10 Kamera IP 4MP"), when the analysis recognised one */
  request?: { label: string; tone: string; items: string }
}

const REPLY_DUE = 120

export function bubbleLine(a: MsgAnalysis): BubbleLine {
  const w = a.wait_min
  const line: BubbleLine = { state: '', tone: 'neutral' }
  switch (a.state) {
    case 'replied':
      line.state = w != null ? `dibalas ${waitLabel(w)} kemudian` : 'dibalas'
      line.tone = w != null && w > REPLY_DUE ? 'warn' : 'good'
      break
    case 'waiting':
      line.state = `menunggu balasan ${waitLabel(w ?? 0)}`
      line.tone = 'warn'
      break
    case 'needs_reply':
      line.state = `menunggu ${waitLabel(w ?? 0)} — perlu dibalas`
      line.tone = 'bad'
      break
    case 'closing':
      break // no rule-based judgement in the bubble (ADR 0032)
    case 'reply':
      line.state = w != null ? `membalas setelah pelanggan menunggu ${waitLabel(w)}` : 'membalas pelanggan'
      line.tone = w != null && w > REPLY_DUE ? 'warn' : 'good'
      break
    case 'follow_up':
      line.state = 'sales menyapa lebih dulu'
      break
  }
  return line // requests are not read by rules any more; Claude's note says what the customer wants (ADR 0032)
}

// Media and edits in a bubble (ADR 0029).

/** The caption of a media bubble: the body without the "[gambar]" / "[dokumen BoQ.pdf]" label the server adds. */
export function mediaCaption(body: string, media?: MediaView): string {
  return media ? body.replace(/^\[[^\]]*\]\s*/, '') : body
}

const KIND_LABEL: Record<string, string> = { image: 'Foto', video: 'Video', gif: 'GIF', audio: 'Audio', document: 'Dokumen', sticker: 'Stiker', location: 'Lokasi', contact: 'Kontak', poll: 'Polling' }

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
export function deletableNow(m: Pick<MessageView, 'can_delete' | 'status' | 'sent_at'>, now: Date): boolean {
  if (!m.can_delete) return false
  return m.status === 'pending' || now.getTime() - new Date(m.sent_at).getTime() < DELETE_WINDOW_MS
}
