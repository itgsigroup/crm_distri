import type { WAInsight, WAIntent } from '../../api/types'
import type { Dir } from '../../components/DataTable'

// Analisa WhatsApp (ADR 0026): labels, the conversations table's text and per-column compares (tested).

export const INTENTS: { key: WAIntent; label: string; tone: string }[] = [
  { key: 'order', label: 'Minta order', tone: 'good' },
  { key: 'ask_price', label: 'Tanya harga', tone: 'accent' },
  { key: 'ask_stock', label: 'Tanya stok', tone: 'indigo' },
  { key: 'release', label: 'Minta rilis kredit', tone: 'bad' },
  { key: 'return', label: 'Retur / klaim', tone: 'warn' },
  { key: 'other', label: 'Lainnya', tone: 'neutral' },
]

export const intentLabel = (k: WAIntent) => INTENTS.find((i) => i.key === k)?.label ?? 'Tanpa pesan pelanggan'
export const intentTone = (k: WAIntent) => INTENTS.find((i) => i.key === k)?.tone ?? 'neutral'

/** "45 menit", "5 jam", "2 hari" (the Go WaitLabel). */
export function waitLabel(min: number): string {
  if (min < 60) return `${Math.max(0, Math.round(min))} menit`
  if (min < 48 * 60) return `${Math.floor(min / 60)} jam`
  return `${Math.floor(min / (24 * 60))} hari`
}

/** "10 Kamera IP 4MP + 2 HDD 2TB" */
export const itemsText = (xs: WAInsight['items']) => xs.map((i) => (i.qty > 0 ? `${i.qty} ${i.name}` : i.name)).join(' + ')

/** Who the conversation is with: the dealer, else the new number's name. */
export const who = (r: WAInsight) => r.dealer_name || r.title

export type WAColumn = 'percakapan' | 'sales' | 'permintaan' | 'menunggu' | 'balas' | 'pesan'

const txt = (a: string | null | undefined, b: string | null | undefined) => (a ?? '').localeCompare(b ?? '', 'id')
const num = (v: number | null | undefined, empty: number) => (v == null ? empty : v)

export const waText = (r: WAInsight) => `${r.title} ${r.dealer_name ?? ''} ${r.sales} ${intentLabel(r.intent)} ${itemsText(r.items)} ${r.last_text} ${r.summary}`
export const WA_COMPARE: Record<WAColumn, (a: WAInsight, b: WAInsight) => number> = {
  percakapan: (a, b) => txt(who(a), who(b)),
  sales: (a, b) => txt(a.sales, b.sales),
  permintaan: (a, b) => txt(intentLabel(a.intent), intentLabel(b.intent)),
  // answered conversations sort below every waiting one
  menunggu: (a, b) => num(a.waiting_since ? a.wait_min : null, -1) - num(b.waiting_since ? b.wait_min : null, -1),
  balas: (a, b) => num(a.reply_median_min, -1) - num(b.reply_median_min, -1),
  pesan: (a, b) => txt(a.last_text, b.last_text),
}
export const waTie = (a: WAInsight, b: WAInsight) => b.priority - a.priority || txt(a.thread_id, b.thread_id)
export const waFirstDir = (c: WAColumn): Dir => (c === 'menunggu' || c === 'balas' ? 'desc' : 'asc')

/** Filter chips of the conversations table. */
export type WAFilter = 'all' | 'reply' | 'waiting' | WAIntent
export function waFilter(rows: WAInsight[], f: WAFilter): WAInsight[] {
  if (f === 'all') return rows
  if (f === 'reply') return rows.filter((r) => r.needs_reply)
  if (f === 'waiting') return rows.filter((r) => !!r.waiting_since)
  return rows.filter((r) => r.intent === f)
}
