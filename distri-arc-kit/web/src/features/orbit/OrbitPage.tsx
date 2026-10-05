import { useRef, useState, type MouseEvent } from 'react'
import { useNavigate } from 'react-router'
import type { BoardItem, Mover } from '../../api/types'
import { Icon } from '../../components/Icon'
import { fmtRp } from '../../lib/format'
import { KUAD, RING_DESC } from '../../lib/i18n/id'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useOrbit, useOrbitMovers, useOrbitSummary, useSales } from '../../app/queries'
import { CX, CY, H, RINGS, RING_R, W, layoutOrbit } from './geometry'

export function SalesFilters({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { data: sales = [] } = useSales()
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

export function OrbitBoard({ list, onOpen }: { list: BoardItem[]; onOpen: (id: string) => void }) {
  const wrap = useRef<HTMLDivElement>(null)
  const [hover, setHover] = useState<{ d: BoardItem; x: number; y: number } | null>(null)
  const nodes = layoutOrbit(list)
  const LA = 0.62 * Math.PI * 2
  const move = (e: MouseEvent, d: BoardItem) => {
    const r = wrap.current!.getBoundingClientRect()
    let px = e.clientX - r.left + 14
    const py = e.clientY - r.top + 14
    if (px + 250 > r.width) px -= 270
    setHover({ d, x: px, y: py })
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
        {nodes.map(({ d, x, y, size, tone, ring, name, sm, side }) => (
          <g key={d.id} className="dn" tabIndex={0} onClick={() => onOpen(d.id)} onMouseMove={(e) => move(e, d)} onKeyDown={(e) => e.key === 'Enter' && onOpen(d.id)}>
            <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={size.toFixed(1)} fill={`var(--${tone === 'neutral' ? 'text-3' : tone})`} className={ring === 'Churn' ? 'ghost' : ''} />
            <text x={(x + side * (size + 5)).toFixed(1)} y={(y + 4).toFixed(1)} textAnchor={side > 0 ? 'start' : 'end'} className={`dn-t ${sm ? 'sm' : ''}`}>{name}</text>
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

export function OrbitPage() {
  const [sales, setSales] = useState('all')
  const nav = useNavigate()
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const { data: list = [] } = useOrbit(sales)
  const { data: summary = [] } = useOrbitSummary(sales)
  const { data: movers = [] } = useOrbitMovers(sales)
  const avgSow = list.length ? Math.round(list.reduce((a, d) => a + d.metrics.sow, 0) / list.length) : 0
  return (
    <div className="net">
      <div>
        <div className="card orbit-card">
          <div className="orbit-hud">
            <SalesFilters value={sales} onChange={setSales} />
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
              <span className="meta">{list.length} dealer · share of wallet rata-rata {avgSow}%</span>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} disabled={orch.running} onClick={() => reanalyze('screen:orbit')}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} onClick={() => nav('/panduan')}><Icon name="doc" />Cara baca</button>
            </div>
          </div>
          <OrbitBoard list={list} onOpen={(id) => nav('/dealer/' + id)} />
          <Legend tail="Ukuran = share of wallet · Sudut = posisi dalam siklus order (atas = jadwal order) · Status = Key account / Aktif / At risk / Churn" />
        </div>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Isi orbit</h2><span className="meta">Dealer per status</span></div>
          <ul className="pulse" id="orbit-rings">
            {summary.map((r) => (
              <li key={r.status}>
                <span className="lbl"><b>{r.status}</b> · {r.count} dealer</span>
                <em className="num">{fmtRp(r.omzet_bln)}/bln</em>
                <span className="dl n">{RING_DESC[r.status]}</span>
              </li>
            ))}
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Yang bergerak</h2><span className="ai" style={{ marginLeft: 6 }}>dibaca AI Follow-up</span></div>
          <MoverList items={movers} render={orbitMover} />
        </div>
      </div>
    </div>
  )
}
