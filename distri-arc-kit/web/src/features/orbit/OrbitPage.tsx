import { useMemo, useRef, useState, type FocusEvent, type MouseEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import type { BoardItem, Mover } from '../../api/types'
import { Icon } from '../../components/Icon'
import { fmtRp } from '../../lib/format'
import { KUAD, RING_DESC } from '../../lib/i18n/id'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useOrbit, useOrbitMovers, useOrbitSummary, useSales } from '../../app/queries'
import { FILTER_KEYS, activeCount, applyFilter, digest, sortDealers, type OrbitFilter, type Sort } from './filters'
import { OrbitDealerList, OrbitFilterBar, OrbitSummary } from './OrbitTools'
import { CX, CY, H, READABLE_FROM, RINGS, RING_R, W, layoutOrbit, layoutReadable } from './geometry'

export function SalesFilters({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { data: sales = [] } = useSales()
  if (sales.length > 8) { // a real team: a picker, not a wall of buttons
    return (
      <div className="net-filters">
        <select className="sales-pick" value={value} onChange={(e) => onChange(e.target.value)} aria-label="Sales">
          <option value="all">Semua sales ({sales.length})</option>
          {[...sales].sort((a, b) => a.name.localeCompare(b.name)).map((s) => <option key={s.name} value={s.name}>{s.name}{s.branch ? ` · ${s.branch}` : ''}</option>)}
        </select>
      </div>
    )
  }
  return (
    <div className="net-filters">
      {[['all', 'Semua sales'] as const, ...sales.map((s) => [s.name, s.name] as const)].map(([id, n]) => (
        <button key={id} className={value === id ? 'is-active' : ''} onClick={() => onChange(id)}>{n}</button>
      ))}
    </div>
  )
}

export function Legend({ tail }: { tail: string }) {
  return (
    <div className="orbit-legend">
      <span><i style={{ background: 'var(--good)' }} />Limit aman</span>
      <span><i style={{ background: 'var(--warn)' }} />Limit tipis</span>
      <span><i style={{ background: 'var(--bad)' }} />Over limit / overdue</span>
      <span><i style={{ background: 'var(--text-3)' }} />Cash</span>
      <span style={{ marginLeft: 'auto' }}>{tail}</span>
    </div>
  )
}

export function Tip({ d, pos, extra }: { d: BoardItem | null; pos: { x: number; y: number }; extra?: React.ReactNode }) {
  if (!d) return <div className="tip" />
  return (
    <div className="tip show" style={{ left: pos.x, top: pos.y }}>
      <b>{d.name}</b>
      <div className="r"><span>{d.city} · tier {d.tier} · {d.owner.name}</span></div>
      {extra}
    </div>
  )
}

/** Prioritas: show the largest dealers clearly and keep the rest as small context dots. */
export const ORBIT_TOP = 150
export const ORBIT_FEATURED = 30

/** Rank dealers by monthly revenue for the priority view. */
export function topByOmzet(list: BoardItem[], n = ORBIT_TOP) {
  return list.length <= n ? list : [...list].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln).slice(0, n)
}

export function OrbitBoard({ list, onOpen, dense = false, focus: focusRing = null }: { list: BoardItem[]; onOpen: (id: string) => void; dense?: boolean; focus?: string | null }) {
  const wrap = useRef<HTMLDivElement>(null)
  const [hover, setHover] = useState<{ d: BoardItem; x: number; y: number } | null>(null)
  const readable = !dense && list.length >= READABLE_FROM
  const nodes = useMemo(() => {
    if (readable) return []
    const featured = !dense && list.length > ORBIT_FEATURED ? new Set(topByOmzet(list, ORBIT_FEATURED).map((d) => d.id)) : undefined
    return layoutOrbit(list, 1.2, dense, featured, false)
  }, [list, dense, readable])
  const rnodes = useMemo(() => {
    if (!readable) return []
    return layoutReadable(list, new Set(), focusRing)
  }, [list, readable, focusRing])
  const LA = 0.62 * Math.PI * 2
  const move = (e: MouseEvent, d: BoardItem) => {
    const r = wrap.current!.getBoundingClientRect()
    let px = e.clientX - r.left + 14
    const py = e.clientY - r.top + 14
    if (px + 250 > r.width) px -= 270
    setHover({ d, x: px, y: py })
  }
  const showFocus = (e: FocusEvent<SVGGElement>, d: BoardItem) => {
    const r = wrap.current!.getBoundingClientRect()
    const dot = e.currentTarget.getBoundingClientRect()
    let x = dot.left - r.left + dot.width / 2 + 14
    if (x + 250 > r.width) x = Math.max(8, x - 270)
    setHover({ d, x, y: Math.max(8, dot.top - r.top + dot.height / 2 + 14) })
  }
  const m = hover?.d.metrics
  return (
    <div className="orbit-wrap" ref={wrap} onMouseLeave={() => setHover(null)}>
      <svg className="orbit" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Orbit dealer">
        <path d={`M${CX} ${CY} L${CX - 330} ${CY} A330 330 0 0 1 ${CX} ${CY - 330} Z`} fill="var(--good)" opacity=".05" />
        {RINGS.map((n) => {
          const r = RING_R[n]
          return (
            <g key={n}>
              <circle cx={CX} cy={CY} r={r} className={`ring-l ${n === 'Churn' ? 'dash' : ''}`} />
              <text className="ring-t" x={(CX + (r - 8) * Math.sin(LA)).toFixed(1)} y={(CY - (r - 8) * Math.cos(LA)).toFixed(1)} textAnchor="end">{n}</text>
            </g>
          )
        })}
        <line x1={CX} y1={CY - RING_R['Key account'] + 30} x2={CX} y2={CY - 340} className="due-l" />
        <text className="due-t" x={CX} y={CY - 348} textAnchor="middle">JADWAL ORDER</text>
        <text className="ring-t" x={CX + RING_R.Churn + 6} y={CY + 4}>¼ putaran</text>
        <text className="ring-t" x={CX} y={CY + RING_R.Churn + 18} textAnchor="middle">½ putaran</text>
        <text className="ring-t" x={CX - RING_R.Churn - 6} y={CY + 4} textAnchor="end">¾ putaran</text>
        <circle cx={CX} cy={CY} r={28} fill="var(--text)" />
        <text x={CX} y={CY + 5} textAnchor="middle" className="gsi-t">GSI</text>
        {rnodes.map(({ d, x, y, size, tone, ring, dim }) => (
          <g key={d.id} className={`dn ${dim ? 'dim' : ''}`} role="button" tabIndex={dim ? -1 : 0} aria-label={d.name} onClick={() => onOpen(d.id)} onMouseMove={(e) => move(e, d)} onFocus={(e) => showFocus(e, d)} onBlur={() => setHover(null)} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpen(d.id) } }}>
            <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={Math.max(size, 12).toFixed(1)} className="dn-hit" fill="transparent" pointerEvents="all" />
            <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={size.toFixed(1)} fill={`var(--${tone === 'neutral' ? 'text-3' : tone})`} className={`dn-dot ${ring === 'Churn' ? 'ghost' : ''}`} />
          </g>
        ))}
        {nodes.map(({ d, x, y, size, tone, ring }) => (
          <g key={d.id} className="dn" role="button" tabIndex={0} aria-label={d.name} onClick={() => onOpen(d.id)} onMouseMove={(e) => move(e, d)} onFocus={(e) => showFocus(e, d)} onBlur={() => setHover(null)} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpen(d.id) } }}>
            <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={Math.max(size, 12).toFixed(1)} className="dn-hit" fill="transparent" pointerEvents="all" />
            <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={size.toFixed(1)} fill={`var(--${tone === 'neutral' ? 'text-3' : tone})`} className={`dn-dot ${ring === 'Churn' ? 'ghost' : ''}`} />
          </g>
        ))}
      </svg>
      <Tip
        d={hover?.d ?? null}
        pos={hover ?? { x: 0, y: 0 }}
        extra={
          hover && m && (
            <>
              <div className="r"><span>Status</span><span>{m.status === 'Baru' ? 'Aktif' : m.status}</span></div>
              <div className="r"><span>Siklus order</span><span>{m.rhythm_days ? `${m.rhythm_days} hr · terakhir ${m.last_order_days} hr` : 'baru'}</span></div>
              <div className="r"><span>Jadwal order</span><span>{m.rhythm_days && m.due_in != null ? (m.due_in >= 0 ? `${m.due_in} hr lagi` : `lewat ${-m.due_in} hr`) : '—'}</span></div>
              <div className="r"><span>Share of wallet</span><span>{m.sow}%</span></div>
              <div className="r"><span>Sisa limit</span><span>{creditText(hover.d)}</span></div>
              <div className="r"><span>Segmen</span><span>{KUAD[m.segment].n}</span></div>
              <div className="r"><span>Skor dealer</span><span>{m.score}</span></div>
              <div className="r" style={{ marginTop: 4, opacity: 0.7 }}><span>Klik untuk buka dealer</span></div>
            </>
          )
        }
      />
    </div>
  )
}

/** Sisa limit text as the mockup napas().t. */
export function creditText(d: BoardItem) {
  const s = d.metrics.credit.state
  return s === 'overdue' ? 'overdue · invoice lewat tempo' : s
}

export function MoverList({ items, render }: { items: Mover[]; render: (m: Mover) => { k: string; i: string; t: string; s: string } }) {
  return (
    <ul className="ins">
      {items.length === 0 && <li><span /><div><span>Tidak ada pergerakan.</span></div></li>}
      {items.map((m, idx) => {
        const x = render(m)
        return (
          <li key={m.kind + m.dealer_id + idx}>
            <span className="ii" style={{ background: `var(--${x.k}-soft)`, color: `var(--${x.k})` }}><Icon name={x.i} /></span>
            <div><b>{x.t}</b><span>{x.s}</span></div>
          </li>
        )
      })}
    </ul>
  )
}

export function orbitMover(m: Mover) {
  const bad = m.credit_state === 'over limit' || m.credit_state === 'overdue'
  if (m.kind === 'moving_out') return { k: 'warn', i: 'refresh', t: `${m.name} bergerak keluar`, s: `${m.last_order_days} hari dari siklus order ${m.rhythm_days}. ${bad ? 'Sisa limit juga over limit — dua KPI melambat bersama.' : 'Follow-up sebelum lewat 2×.'}` }
  if (m.kind === 'approaching') return { k: 'good', i: 'check', t: `${m.name} mendekati jadwal order`, s: `${m.due_in === 0 ? 'Hari ini' : m.due_in + ' hari lagi'} · rekomendasi order siap · ${bad ? 'over limit / overdue — tagih dulu' : 'sisa limit ' + (m.credit_state === 'overdue' ? 'overdue · invoice lewat tempo' : m.credit_state)}` }
  return { k: 'accent', i: 'box', t: `${m.name}: Key account, tapi product mix ${m.mix}/6`, s: 'Share of wallet bisa naik dengan meperluas product mix, bukan menurunkan harga.' }
}

const SORTS: Sort[] = ['omzet', 'jadwal', 'diam']

/** Filters live in the URL (?status=At+risk&jadwal=lewat…) so a filtered orbit can be shared or bookmarked. */
function useOrbitFilter(): [OrbitFilter, (p: Partial<OrbitFilter>) => void, Sort, (s: Sort) => void] {
  const [params, setParams] = useSearchParams()
  const f = Object.fromEntries(FILTER_KEYS.map((k) => [k, params.get(k) ?? ''])) as unknown as OrbitFilter
  const sortParam = params.get('urut') as Sort | null
  const sort: Sort = sortParam && SORTS.includes(sortParam) ? sortParam : 'omzet'
  const write = (next: OrbitFilter, s: Sort) => {
    const out = new URLSearchParams()
    FILTER_KEYS.forEach((k) => next[k] && out.set(k, next[k]))
    if (s !== 'omzet') out.set('urut', s)
    setParams(out, { replace: true })
  }
  return [f, (p) => write({ ...f, ...p }, sort), sort, (s) => write(f, s)]
}

export function OrbitPage() {
  const [sales, setSales] = useState('all')
  const nav = useNavigate()
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const { data: list = [] } = useOrbit(sales)
  const [all, setAll] = useState(false)
  const [f, setF, sort, setSort] = useOrbitFilter()
  const filtered = useMemo(() => applyFilter(list, f), [list, f])
  const nFilters = activeCount(f)
  const many = filtered.length > ORBIT_TOP
  const shown = useMemo(() => (all ? filtered : topByOmzet(filtered)), [filtered, all])
  const sorted = useMemo(() => sortDealers(filtered, sort), [filtered, sort])
  const g = useMemo(() => digest(list), [list])
  const branches = useMemo(() => [...new Set(list.map((d) => d.branch).filter(Boolean))].sort(), [list])
  const { data: osum } = useOrbitSummary(sales)
  const summary = osum?.items ?? []
  const { data: movers = [] } = useOrbitMovers(sales)
  const avgSow = filtered.length ? Math.round(filtered.reduce((a, d) => a + d.metrics.sow, 0) / filtered.length) : 0
  return (
    <div className="net">
      <div className="stack">
        <div className="card orbit-card">
          <div className="orbit-hud">
            <SalesFilters value={sales} onChange={setSales} />
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} disabled={orch.running} onClick={() => reanalyze('screen:orbit')}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} onClick={() => nav('/panduan')}><Icon name="doc" />Cara baca</button>
            </div>
          </div>
          <OrbitSummary g={g} f={f} set={setF} />
          <OrbitFilterBar f={f} set={setF} branches={branches} shown={filtered.length} total={list.length} />
          <div className="orbit-hud" style={{ marginTop: 4 }}>
            <span className="meta">{many && !all ? `${ORBIT_TOP} dealer dengan omzet terbesar dari ${filtered.length} dealer` : `${filtered.length} dealer di orbit`} · porsi belanja di GSI rata-rata {avgSow}%</span>
            {many && (
              <div className="seg" role="radiogroup" aria-label="Tampilan orbit">
                <button role="radio" aria-checked={!all} className={!all ? 'is-active' : ''} onClick={() => setAll(false)}>Prioritas</button>
                <button role="radio" aria-checked={all} className={all ? 'is-active' : ''} onClick={() => setAll(true)}>Semua titik</button>
              </div>
            )}
          </div>
          <div style={{ position: 'relative' }}>
            <OrbitBoard list={shown} dense={all && many} onOpen={(id) => nav('/dealer/' + id)} />
            {list.length > 0 && filtered.length === 0 && <div className="net-empty">Tidak ada dealer yang cocok dengan filter ini.</div>}
          </div>
          <Legend tail="Arahkan kursor ke titik untuk melihat data · klik titik untuk membuka dealer" />
        </div>
        <OrbitDealerList list={sorted} sort={sort} onSort={setSort} filtered={nFilters > 0} />
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Isi orbit</h2><span className="meta">{f.status ? <button className="ev-more" onClick={() => setF({ status: '' })}>tampilkan semua</button> : 'klik status untuk menyaring'}</span></div>
          <ul className="pulse" id="orbit-rings">
            {summary.map((r) => (
              <li key={r.status} className={f.status === r.status ? 'is-sel' : ''} style={{ cursor: 'pointer', opacity: f.status && f.status !== r.status ? 0.5 : 1 }} onClick={() => setF({ status: f.status === r.status ? '' : r.status })}>
                <span className="lbl"><b>{r.status}</b> · {r.count} dealer</span>
                <em className="num">{fmtRp(r.omzet_bln)}/bln</em>
                <span className="dl n">{RING_DESC[r.status]}</span>
              </li>
            ))}
          </ul>
        {!!osum?.prospects && (
          <p className="prospek-line"><b>{osum?.prospects.toLocaleString('id-ID')} prospek</b> belum pernah order — tidak digambar di sini. <button className="ev" onClick={() => nav('/dealer?status=Prospek')}>Lihat daftar</button></p>
        )}
        </div>
        <div className="card">
          <div className="card-h"><h2>Yang bergerak</h2><span className="ai" style={{ marginLeft: 6 }}>dibaca AI Follow-up</span></div>
          <MoverList items={movers} render={orbitMover} />
        </div>
      </div>
    </div>
  )
}
