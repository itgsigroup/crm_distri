import type { BoardItem, CreditState, Segment, SegmentSummary, Status } from '../../api/types'
import { SEGMENT_ORDER } from '../../lib/i18n/id'

export type RevenueGroup = '' | 'top10' | 'top25' | 'bottom25'
export type SegmentChange = '' | 'moved' | 'stable' | 'unknown'

export interface SegmenFilter {
  q: string
  cabang: string
  status: Status | ''
  credit: CreditState | ''
  revenue: RevenueGroup
  change: SegmentChange
}

export const EMPTY_SEGMEN_FILTER: SegmenFilter = { q: '', cabang: '', status: '', credit: '', revenue: '', change: '' }

export const countSegmenFilters = (f: SegmenFilter) => Number(!!f.q.trim()) + Number(!!f.cabang) + Number(!!f.status) + Number(!!f.credit) + Number(!!f.revenue) + Number(!!f.change)

export function changeOf(d: BoardItem): Exclude<SegmentChange, ''> {
  if (!d.prev) return 'unknown'
  return d.prev.segment === d.metrics.segment ? 'stable' : 'moved'
}

export function applySegmenFilters(list: BoardItem[], f: SegmenFilter): BoardItem[] {
  const q = f.q.trim().toLocaleLowerCase('id-ID')
  const eligible = list.filter((d) => {
    if (q && !`${d.name} ${d.short_name} ${d.city} ${d.owner.name}`.toLocaleLowerCase('id-ID').includes(q)) return false
    if (f.cabang && d.branch !== f.cabang) return false
    if (f.status && d.metrics.status !== f.status) return false
    if (f.credit && d.metrics.credit.state !== f.credit) return false
    if (f.change && changeOf(d) !== f.change) return false
    return true
  })
  if (!f.revenue) return eligible
  const ranked = [...eligible].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln || a.name.localeCompare(b.name, 'id'))
  const count = Math.ceil(ranked.length * (f.revenue === 'top10' ? 0.1 : 0.25))
  const group = f.revenue === 'bottom25' ? ranked.slice(-count) : ranked.slice(0, count)
  const revenueIds = new Set(group.map((d) => d.id))
  return eligible.filter((d) => revenueIds.has(d.id))
}

export function summarizeSegmen(list: BoardItem[]): { items: SegmentSummary[]; total: number } {
  const total = list.reduce((sum, d) => sum + d.metrics.omzet_bln, 0)
  const bySegment = new Map<Segment, { count: number; omzet: number }>()
  list.forEach((d) => {
    const current = bySegment.get(d.metrics.segment) ?? { count: 0, omzet: 0 }
    current.count++
    current.omzet += d.metrics.omzet_bln
    bySegment.set(d.metrics.segment, current)
  })
  return {
    total,
    items: SEGMENT_ORDER.map((segment) => {
      const row = bySegment.get(segment) ?? { count: 0, omzet: 0 }
      return { segment, count: row.count, omzet_bln: row.omzet, pct: total > 0 ? Math.round(row.omzet / total * 100) : 0 }
    }),
  }
}
