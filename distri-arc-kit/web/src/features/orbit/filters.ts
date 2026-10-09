// Orbit filters and summary — pure functions so the board, the list and the summary tiles agree on one definition.
// Jadwal order / Lewat jadwal follow docs/design/01-glossary.md (due_in; Lewat jadwal = At risk, i.e. cyc > drift).
import type { BoardItem, Segment } from '../../api/types'
import { ringOf } from './geometry'

export type Jadwal = '' | 'minggu' | 'lewat'
export type Limit = '' | 'aman' | 'tipis' | 'tagih' | 'cash'
export type Sort = 'omzet' | 'jadwal' | 'diam'

export interface OrbitFilter {
  q: string
  status: string // a ring: Key account / Aktif / At risk / Churn
  jadwal: Jadwal
  limit: Limit
  segmen: Segment | ''
  cabang: string
}

export const EMPTY: OrbitFilter = { q: '', status: '', jadwal: '', limit: '', segmen: '', cabang: '' }
export const FILTER_KEYS = Object.keys(EMPTY) as (keyof OrbitFilter)[]

/** Window of "Jadwal order" on Orbit and the dashboard: the next 2 weeks. */
export const DUE_DAYS = 14

/** Due within the window: has a cycle, not lost, 0 ≤ due_in ≤ DUE_DAYS. */
export const dueSoon = (d: BoardItem) => {
  const m = d.metrics
  return !!m.rhythm_days && m.due_in != null && m.due_in >= 0 && m.due_in <= DUE_DAYS && m.status !== 'Churn'
}
export const isLate = (d: BoardItem) => d.metrics.status === 'At risk'
export const mustCollect = (d: BoardItem) => d.metrics.credit.state === 'over limit' || d.metrics.credit.state === 'overdue'

function limitOf(d: BoardItem): Limit {
  const s = d.metrics.credit.state
  return s === 'aman' ? 'aman' : s === 'tipis' ? 'tipis' : s === 'cash' ? 'cash' : 'tagih'
}

export function applyFilter(list: BoardItem[], f: OrbitFilter): BoardItem[] {
  const q = f.q.trim().toLowerCase()
  return list.filter((d) => {
    if (q && !`${d.name} ${d.short_name} ${d.city} ${d.owner.name}`.toLowerCase().includes(q)) return false
    if (f.status && ringOf(d) !== f.status) return false
    if (f.jadwal === 'minggu' && !dueSoon(d)) return false
    if (f.jadwal === 'lewat' && !isLate(d)) return false
    if (f.limit && limitOf(d) !== f.limit) return false
    if (f.segmen && d.metrics.segment !== f.segmen) return false
    if (f.cabang && d.branch !== f.cabang) return false
    return true
  })
}

export const activeCount = (f: OrbitFilter) => FILTER_KEYS.filter((k) => f[k] !== '').length

export function sortDealers(list: BoardItem[], by: Sort): BoardItem[] {
  const due = (d: BoardItem) => (d.metrics.rhythm_days && d.metrics.due_in != null ? d.metrics.due_in : Infinity)
  const key: Record<Sort, (d: BoardItem) => number> = {
    omzet: (d) => -d.metrics.omzet_bln,
    jadwal: due, // most overdue first, then the nearest due date
    diam: (d) => -(d.metrics.last_order_days ?? -1),
  }
  return [...list].sort((a, b) => key[by](a) - key[by](b) || b.metrics.omzet_bln - a.metrics.omzet_bln)
}

export interface OrbitDigest {
  total: number
  omzet: number
  due: { n: number; omzet: number }
  late: { n: number; omzet: number }
  collect: { n: number; exposure: number }
  churn: { n: number; omzet: number }
}

export function digest(list: BoardItem[]): OrbitDigest {
  const sum = (xs: BoardItem[]) => xs.reduce((a, d) => a + d.metrics.omzet_bln, 0)
  const due = list.filter(dueSoon)
  const late = list.filter(isLate)
  const collect = list.filter(mustCollect)
  const churn = list.filter((d) => d.metrics.status === 'Churn')
  return {
    total: list.length,
    omzet: sum(list),
    due: { n: due.length, omzet: sum(due) },
    late: { n: late.length, omzet: sum(late) },
    collect: { n: collect.length, exposure: collect.reduce((a, d) => a + d.metrics.credit.exposure, 0) },
    churn: { n: churn.length, omzet: sum(churn) },
  }
}
