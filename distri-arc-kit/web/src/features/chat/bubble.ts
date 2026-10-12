import type { MsgAnalysis } from '../../api/types'
import { intentLabel, intentTone, itemsText, waitLabel } from '../wa/wa'

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
      line.state = 'pesan penutup — tidak perlu dibalas'
      break
    case 'reply':
      line.state = w != null ? `membalas setelah pelanggan menunggu ${waitLabel(w)}` : 'membalas pelanggan'
      line.tone = w != null && w > REPLY_DUE ? 'warn' : 'good'
      break
    case 'follow_up':
      line.state = 'sales menyapa lebih dulu'
      break
  }
  if (a.analyzed && a.intent && a.intent !== 'none' && a.state !== 'closing' && a.state !== 'reply' && a.state !== 'follow_up') {
    line.request = { label: intentLabel(a.intent), tone: intentTone(a.intent), items: itemsText(a.items ?? []) }
  }
  return line
}
