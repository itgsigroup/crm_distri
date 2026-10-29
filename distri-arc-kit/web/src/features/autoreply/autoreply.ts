// Chat otomatis (ADR 0066, ADR 0077): the setting as the page edits it, and the checks the server makes again.

export interface HourWindow { days: number[]; start: string; end: string }

export interface AutoReplySettings {
  enabled: boolean
  lyra: boolean
  breaks: HourWindow[]
  numbers: string[]
  template: string
  facts: string[]
  approved_name?: string
  approved_at?: string
}

export interface AutoReplyNumber { wa_number: string; label: string; holder: string; team: string; state: string }

export interface AutoReplyLog {
  message_id: string; kind: 'lyra' | 'fact' | 'template'; created_at: string; thread_id: string; title: string; account: string
  question: string; answer: string; status: string
}

export interface AutoReplyInfo {
  settings: AutoReplySettings
  numbers: AutoReplyNumber[]
  recent: AutoReplyLog[]
  ai_ready: boolean
  lyra_ready: boolean
  active_now: boolean
  active_until?: string
  can_edit: boolean
  send_from: string
  send_to: string
  max_facts: number
  human_quiet_min: number
}

/** 1 = Senin … 7 = Minggu, as the server counts. */
export const DAYS: [number, string][] = [[1, 'Sen'], [2, 'Sel'], [3, 'Rab'], [4, 'Kam'], [5, 'Jum'], [6, 'Sab'], [7, 'Min']]

/** The AI Trigger facts a fallback answer may give. */
export const FACTS: [string, string][] = [['stock', 'Stok'], ['price', 'Harga'], ['order', 'Status order'], ['credit', 'Limit & tagihan']]

export const KIND_LABEL: Record<AutoReplyLog['kind'], string> = { lyra: 'Lyra', fact: 'AI Trigger', template: 'Template' }

/** The template for one conversation, as the server fills it (autoreply.Fill). */
export function fill(t: string, name: string, back: string, partOfDay: string): string {
  return t.replaceAll('{salam}', 'Selamat ' + partOfDay).replaceAll('{nama}', name).replaceAll('{kembali}', back).trim()
}

const HM = /^([01]\d|2[0-3]):[0-5]\d$/

/** What is wrong with the hours, "" when the server will take them (autoreply.Validate). */
export function windowsError(ws: HourWindow[], from: string, to: string): string {
  if (ws.length === 0) return 'Tambahkan minimal satu jam aktif'
  if (ws.length > 7) return 'Paling banyak 7 jam aktif'
  for (const w of ws) {
    if (!HM.test(w.start) || !HM.test(w.end) || w.start >= w.end) return 'Jam aktif: selesai harus setelah mulai'
    if (w.start < from || w.end > to) return `Jam aktif harus di antara ${from}–${to} WIB`
    if (w.days.length === 0) return 'Pilih hari untuk setiap jam aktif'
  }
  return ''
}

/** "Sen–Sab", "Sen, Rab, Jum", "Setiap hari". */
export function daysText(days: number[]): string {
  const d = [...new Set(days)].sort((a, b) => a - b)
  if (d.length === 7) return 'Setiap hari'
  const name = (n: number) => DAYS[n - 1][1]
  const run = d.length > 2 && d.every((v, i) => i === 0 || v === d[i - 1] + 1)
  return run ? `${name(d[0])}–${name(d[d.length - 1])}` : d.map(name).join(', ')
}

/** "12.00" from "12:00", as WIB times are written to customers. */
export const dotted = (hm: string) => hm.replace(':', '.')
