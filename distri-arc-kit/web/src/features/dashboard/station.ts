import type { BoardItem } from '../../api/types'
import { fmtNum, fmtRp } from '../../lib/format'
import { dueSoon, mustCollect } from '../orbit/filters'
import type { ProgressAgent } from '../control/progress'

// Stasiun Orbit (dashboard game scene): a space station around the AI core (the Orchestrator) with ten rooms, one
// astronaut each, worked in order — sales, WhatsApp and dealers are read first, the six agents then propose, and the
// last room puts the data together ("Data siap"). The scene never shows a customer: no dealer or contact names, only
// what each astronaut is doing and counts. The same lines fill the "Hasil overview AI" card. Pure, so it is tested.

export type CrewKey = 'sales' | 'wa' | 'dealer' | 'order' | 'followup' | 'stock' | 'credit' | 'collect' | 'prospect' | 'ready'
export type CrewState = 'idle' | 'waiting' | 'reading' | 'done' | 'failed'
export interface CrewWork { state: CrewState; text: string }
export type CrewStatus = Record<CrewKey, CrewWork>
export const CREW_STATE_LABEL: Record<CrewState, string> = { idle: 'siaga', waiting: 'antri', reading: 'menganalisa', done: 'selesai', failed: 'gagal' }

export interface CrewMember {
  key: CrewKey
  n: number
  title: string
  /** what the astronaut does, for the tooltip and the legend */
  does: string
  color: string
  /** the server agent (proposals.agent) this astronaut acts out; the first three and the last follow the cycle stages */
  agent?: string
  /** screen permission and route a click opens */
  screen: string
  to: string
}

export const CREW: CrewMember[] = [
  { key: 'sales', n: 1, title: 'Analisa sales', does: 'Membaca omzet, SO, dan pembayaran dari Odoo', color: '#36b8ff', screen: 'kuad', to: '/orbit/segmen' },
  { key: 'wa', n: 2, title: 'Analisa WhatsApp', does: 'Membaca chat masuk dan pesan yang belum dibalas', color: '#25d366', screen: 'chat', to: '/chat' },
  { key: 'dealer', n: 3, title: 'Analisa dealer', does: 'Menghitung jadwal order, status, dan segmen dealer', color: '#4fd6ff', screen: 'orbit', to: '/orbit' },
  { key: 'order', n: 4, title: 'Eksekusi order', does: 'Menyiapkan usulan order untuk jadwal order yang dekat', color: '#3b8cff', agent: 'AI Order', screen: 'today', to: '/' },
  { key: 'followup', n: 5, title: 'Follow-up', does: 'Menyusun follow-up untuk dealer At risk', color: '#ff9a2e', agent: 'AI Follow-up', screen: 'today', to: '/' },
  { key: 'stock', n: 6, title: 'Cek stok', does: 'Memeriksa stok menua dan stok kritis', color: '#2fd17a', agent: 'AI Stok', screen: 'stock', to: '/stok' },
  { key: 'credit', n: 7, title: 'Cek kredit', does: 'Memeriksa sisa limit kredit dealer', color: '#a26bff', agent: 'AI Kredit', screen: 'ar', to: '/kredit' },
  { key: 'collect', n: 8, title: 'Cek penagihan', does: 'Memeriksa invoice lewat tempo dan over limit', color: '#ff4d5e', agent: 'AI Penagihan', screen: 'ar', to: '/kredit' },
  { key: 'prospect', n: 9, title: 'Prospek', does: 'Memindai nomor baru yang menghubungi lebih dulu', color: '#e9eef7', agent: 'AI Prospek', screen: 'chat', to: '/chat' },
  { key: 'ready', n: 10, title: 'Data siap', does: 'Menyatukan hasil semua astronot jadi usulan untuk diputuskan', color: '#ffcf5a', screen: 'today', to: '/' },
]
export const CREW_OF: Record<CrewKey, CrewMember> = Object.fromEntries(CREW.map((c) => [c.key, c])) as Record<CrewKey, CrewMember>
/** proposals.agent → the astronaut that made it */
export const CREW_BY_AGENT: Record<string, CrewKey> = Object.fromEntries(CREW.filter((c) => c.agent).map((c) => [c.agent!, c.key]))

/** Aggregates only — never a customer's name. Undefined = not loaded (or not allowed). */
export interface CrewFacts {
  omzetMonth?: number | null
  omzetPrev?: number | null
  so?: number
  payments?: number
  waMessages?: number
  unread?: number
  newNumbers?: number
  dealers?: number
  atRisk?: number
  due?: number
  tight?: number
  collect?: number
  overdue?: number
  overdueDealers?: number
  aging?: number
  agingValue?: number
  critical?: number
  waiting?: number
  /** proposals waiting per crew member */
  waitingBy?: Partial<Record<CrewKey, number>>
}

/** Counts from the dealer board (no names leave this function). */
export function boardFacts(board: BoardItem[]): Pick<CrewFacts, 'dealers' | 'atRisk' | 'due' | 'tight' | 'collect'> {
  const active = board.filter((d) => d.metrics.segment !== 'Prospek' && d.metrics.status !== 'Prospek')
  return {
    dealers: active.length,
    atRisk: active.filter((d) => d.metrics.status === 'At risk' || d.metrics.status === 'Churn').length,
    due: active.filter(dueSoon).length,
    tight: active.filter((d) => d.metrics.credit.state === 'tipis').length,
    collect: active.filter(mustCollect).length,
  }
}

/** Proposals waiting for a decision, per astronaut. */
export function waitingBy(agents: string[]): Partial<Record<CrewKey, number>> {
  const out: Partial<Record<CrewKey, number>> = {}
  for (const a of agents) {
    const k = CREW_BY_AGENT[a]
    if (k) out[k] = (out[k] ?? 0) + 1
  }
  return out
}

const n = (v: number | undefined) => (v == null ? '—' : fmtNum(v))

/** One line per astronaut: what its last look at the data found (the card), short enough for a room sign. */
export function crewLines(f: CrewFacts): Record<CrewKey, string> {
  const wb = f.waitingBy ?? {}
  const prop = (k: CrewKey) => (wb[k] ? ` · ${fmtNum(wb[k]!)} usulan` : '')
  const sales = f.omzetMonth != null
    ? `Omzet bulan ini s.d. hari ini ${fmtRp(f.omzetMonth)}` + (f.omzetPrev ? ` · bulan lalu ${fmtRp(f.omzetPrev)}` : '')
    : f.so != null ? `${n(f.so)} SO · ${n(f.payments)} pembayaran dibaca` : 'Menunggu data penjualan'
  const wa = [f.unread != null && `${n(f.unread)} chat belum dibaca`, f.waMessages != null && `${n(f.waMessages)} pesan dianalisa`].filter(Boolean).join(' · ') || 'Tidak ada akses chat'
  return {
    sales,
    wa,
    dealer: `${n(f.dealers)} dealer di orbit · ${n(f.atRisk)} At risk/Churn`,
    order: `${n(f.due)} jadwal order ≤ 14 hari${prop('order')}`,
    followup: `${n(f.atRisk)} dealer perlu follow-up${prop('followup')}`,
    stock: `${n(f.aging)} stok menua · ${n(f.critical)} SKU kritis`,
    credit: `${n(f.tight)} dealer limit tipis${prop('credit')}`,
    collect: f.overdue != null ? `${fmtRp(f.overdue)} lewat tempo · ${n(f.overdueDealers)} dealer` : `${n(f.collect)} dealer over limit / overdue${prop('collect')}`,
    prospect: f.newNumbers != null ? `${n(f.newNumbers)} nomor baru masuk${prop('prospect')}` : 'Tidak ada akses chat',
    ready: `${n(f.waiting)} usulan menunggu keputusan`,
  }
}

export interface CrewCycle {
  /** a cycle is running or being followed */
  active: boolean
  /** index into STAGES: ingest, analyze, synthesize, decide, execute, learn */
  stage: number
  ended: boolean
  failed: boolean
  agents: ProgressAgent[]
  /** new proposals of the followed cycle */
  fresh: number
}

/** What every astronaut does now. Outside a cycle each works its own room (idle, with its line); in a cycle the
 * rooms follow the stages: 1–3 read in Ingest/Analisis, 4–9 act out their agents, 10 puts it together. */
export function crewStatus(lines: Record<CrewKey, string>, c: CrewCycle | null): CrewStatus {
  const out = {} as CrewStatus
  for (const m of CREW) out[m.key] = { state: 'idle', text: lines[m.key] }
  if (!c?.active) return out
  const end = (k: CrewKey, text: string) => { out[k] = { state: c.failed ? 'failed' : 'done', text: c.failed ? 'gagal' : text } }
  const byStage = (k: CrewKey, from: number, to: number, doneText: string) => {
    if (c.ended || c.stage > to) end(k, doneText)
    else if (c.stage >= from) out[k] = { state: 'reading', text: 'menganalisa…' }
    else out[k] = { state: 'waiting', text: 'antri' }
  }
  byStage('sales', 0, 0, 'data penjualan terbaca')
  byStage('wa', 0, 0, 'chat terbaca')
  byStage('dealer', 1, 1, 'orbit dealer dihitung')
  for (const m of CREW) {
    if (!m.agent) continue
    const a = c.agents.find((x) => x.agent === m.agent)
    if (!a) { byStage(m.key, 1, 3, 'tanpa saran baru'); continue }
    const state = c.ended && (a.state === 'waiting' || a.state === 'reading') ? (c.failed ? 'failed' : 'done') : a.state
    const text = state === 'reading' ? (a.covers ? `membaca ${a.covers}` : 'menganalisa…') : state === 'waiting' ? 'antri' : state === 'failed' ? 'gagal' : a.summary || 'selesai'
    out[m.key] = { state, text }
  }
  if (c.ended) end('ready', c.fresh ? `${fmtNum(c.fresh)} usulan baru siap` : 'data siap · tanpa usulan baru')
  else if (c.stage >= 2) out.ready = { state: 'reading', text: 'menyatukan hasil…' }
  else out.ready = { state: 'waiting', text: 'menunggu hasil' }
  return out
}
