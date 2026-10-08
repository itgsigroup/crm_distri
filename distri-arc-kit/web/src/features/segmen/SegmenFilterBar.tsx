import { useMemo } from 'react'
import type { BoardItem, CreditState, Segment, Status } from '../../api/types'
import { Icon } from '../../components/Icon'
import { fmtRp } from '../../lib/format'
import { KUAD, SEGMENT_ORDER } from '../../lib/i18n/id'
import { changeOf, countSegmenFilters, type SegmenFilter } from './filters'

const STATUS: Status[] = ['Key account', 'Aktif', 'At risk', 'Churn', 'Baru', 'Prospek']
const CREDIT: { value: CreditState; label: string }[] = [
  { value: 'aman', label: 'Limit aman' },
  { value: 'tipis', label: 'Limit tipis' },
  { value: 'over limit', label: 'Over limit' },
  { value: 'overdue', label: 'Overdue' },
  { value: 'cash', label: 'Cash' },
]

export function SegmenFilterBar({ list, filtered, value, sel, onChange, onSelect, onClear }: { list: BoardItem[]; filtered: BoardItem[]; value: SegmenFilter; sel: Segment | null; onChange: (patch: Partial<SegmenFilter>) => void; onSelect: (segment: Segment | null) => void; onClear: () => void }) {
  const n = countSegmenFilters(value) + Number(!!sel)
  const facets = useMemo(() => {
    const branches = new Set<string>()
    const statuses = new Set<Status>()
    const credits = new Set<CreditState>()
    const changes = new Set<ReturnType<typeof changeOf>>()
    list.forEach((d) => {
      if (d.branch) branches.add(d.branch)
      statuses.add(d.metrics.status)
      credits.add(d.metrics.credit.state)
      changes.add(changeOf(d))
    })
    return { branches: [...branches].sort((a, b) => a.localeCompare(b, 'id')), statuses, credits, changes }
  }, [list])
  const visible = sel ? filtered.filter((d) => d.metrics.segment === sel) : filtered
  const omzet = visible.reduce((sum, d) => sum + d.metrics.omzet_bln, 0)
  const totalOmzet = list.reduce((sum, d) => sum + d.metrics.omzet_bln, 0)
  const omzetPct = totalOmzet > 0 ? Math.round(omzet / totalOmzet * 100) : 0
  const segmentCounts = new Map<Segment, number>()
  filtered.forEach((d) => segmentCounts.set(d.metrics.segment, (segmentCounts.get(d.metrics.segment) ?? 0) + 1))
  const count = (v: number) => v.toLocaleString('id-ID')
  return (
    <div className="sg-filters">
      <div className="of">
        <div className="search of-q"><Icon name="search" /><input value={value.q} onChange={(e) => onChange({ q: e.target.value })} placeholder="Cari dealer, kota, sales…" aria-label="Cari dealer, kota, atau sales" /></div>
        <select className={`of-s${sel ? ' on' : ''}`} value={sel ?? ''} onChange={(e) => onSelect((e.target.value || null) as Segment | null)} aria-label="Filter segmen">
          <option value="">Semua segmen</option>
          {SEGMENT_ORDER.map((segment) => <option key={segment} value={segment}>{KUAD[segment].nick} ({count(segmentCounts.get(segment) ?? 0)})</option>)}
        </select>
        {(facets.branches.length > 1 || !!value.cabang) && (
          <select className={`of-s${value.cabang ? ' on' : ''}`} value={value.cabang} onChange={(e) => onChange({ cabang: e.target.value })} aria-label="Filter cabang">
            <option value="">Semua cabang</option>
            {value.cabang && !facets.branches.includes(value.cabang) && <option value={value.cabang}>{value.cabang} (tidak ada)</option>}
            {facets.branches.map((branch) => <option key={branch} value={branch}>{branch}</option>)}
          </select>
        )}
        <select className={`of-s${value.status ? ' on' : ''}`} value={value.status} onChange={(e) => onChange({ status: e.target.value as SegmenFilter['status'] })} aria-label="Filter status pelanggan">
          <option value="">Semua status</option>
          {STATUS.filter((status) => facets.statuses.has(status) || value.status === status).map((status) => <option key={status} value={status}>{status}</option>)}
        </select>
        <select className={`of-s${value.credit ? ' on' : ''}`} value={value.credit} onChange={(e) => onChange({ credit: e.target.value as SegmenFilter['credit'] })} aria-label="Filter kondisi limit">
          <option value="">Semua limit</option>
          {CREDIT.filter(({ value: credit }) => facets.credits.has(credit) || value.credit === credit).map(({ value: credit, label }) => <option key={credit} value={credit}>{label}</option>)}
        </select>
        <select className={`of-s${value.revenue ? ' on' : ''}`} value={value.revenue} onChange={(e) => onChange({ revenue: e.target.value as SegmenFilter['revenue'] })} aria-label="Filter peringkat omzet">
          <option value="">Semua omzet</option>
          <option value="top10">10% dealer omzet tertinggi</option>
          <option value="top25">25% dealer omzet tertinggi</option>
          <option value="bottom25">25% dealer omzet terendah</option>
        </select>
        <select className={`of-s${value.change ? ' on' : ''}`} value={value.change} onChange={(e) => onChange({ change: e.target.value as SegmenFilter['change'] })} aria-label="Filter perubahan segmen">
          <option value="">Semua perubahan</option>
          {(facets.changes.has('moved') || value.change === 'moved') && <option value="moved">Pindah segmen</option>}
          {(facets.changes.has('stable') || value.change === 'stable') && <option value="stable">Tetap di segmen</option>}
          {(facets.changes.has('unknown') || value.change === 'unknown') && <option value="unknown">Belum ada pembanding</option>}
        </select>
        {n > 0 && <button type="button" className="btn quiet of-x" onClick={onClear}><Icon name="x" />Hapus filter ({n})</button>}
      </div>
      <p className="sg-filter-summary"><b>{count(visible.length)}</b> dari {count(list.length)} dealer · <b>{fmtRp(omzet)}/bln</b> ({omzetPct}% omzet)</p>
    </div>
  )
}
