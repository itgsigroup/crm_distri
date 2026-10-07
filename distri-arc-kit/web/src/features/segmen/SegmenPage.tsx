import { useRef, useState, type MouseEvent, useMemo } from 'react'
import { useNavigate } from 'react-router'
import type { BoardItem, Mover, Segment } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { fmtRp, fx1, shortName } from '../../lib/format'
import { KUAD, SEGMENT_ORDER } from '../../lib/i18n/id'
import { useMore } from '../../components/More'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useSegmen, useSegmenMovers, useSegmenSummary } from '../../app/queries'
import { Legend, MoverList, SalesFilters } from '../orbit/OrbitPage'
import { toneOf } from '../orbit/geometry'
import { B, H, L, R, T, W, clampY, px, py, xOf } from './scale'

const zoneColor = (k: Segment) => (KUAD[k].k === 'text-3' ? 'text-2' : KUAD[k].k)
const textColor = (k: Segment) => (KUAD[k].k === 'text-3' ? 'text' : KUAD[k].k)

interface Node {
  d: BoardItem
  x: number
  y: number
  size: number
  name: string
  k: Segment
  baru: boolean
  pv: { x: number; y: number } | null
  tw: number
  la: 'start' | 'end' | 'middle'
  lx: number
  ly: number
  label: boolean // named on the chart (largest revenue first); the rest are plain points
}

/** How many dealers get a name on the segment chart: every point stays (it is data), names only for the top ones. */
const SEGMEN_LABELS = 80

function layout(list: BoardItem[]): Node[] {
  const named = new Set([...list].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln).slice(0, SEGMEN_LABELS).map((d) => d.id))
  const dense = list.length > 300
  const nodes: Node[] = list.map((d) => {
    const m = d.metrics
    const x = xOf(m.freq)
    const y = clampY(m.avg_order)
    const name = shortName(d.name)
    const pv = d.prev ? { x: xOf(d.prev.freq), y: clampY(d.prev.avg_order) } : null
    const label = named.has(d.id)
    const size = (7 + Math.sqrt(m.sow) * 1.3) * (dense && !label ? 0.4 : 1)
    return { d, x, y, size, name, k: m.segment, baru: m.freq == null, pv: label ? pv : null, tw: name.length * (m.avg_order < 10e6 ? 5.6 : 6.4) + 6, la: 'start', lx: 0, ly: 0, label }
  })
  // point positions are data and never move; only labels look for free space
  const obst: number[][] = [[W - R - 130, W - R - 8, T + 6, T + 44], [L + 8, L + 130, T + 6, T + 44], [W - R - 130, W - R - 8, H - B - 46, H - B - 6], [L + 8, L + 130, H - B - 46, H - B - 6]]
  nodes.forEach((a) => obst.push([a.x - a.size, a.x + a.size, a.y - a.size, a.y + a.size]))
  const ovl = (A: number[], C: number[]) => Math.max(0, Math.min(A[1], C[1]) - Math.max(A[0], C[0])) * Math.max(0, Math.min(A[3], C[3]) - Math.max(A[2], C[2]))
  ;nodes.filter((a) => a.label).sort((a, b) => b.size - a.size).forEach((a) => {
    const tw = a.tw
    const h = 14
    const c: ['start' | 'end' | 'middle', number, number, number[]][] = [
      ['start', a.x + a.size + 5, a.y + 4, [a.x + a.size + 5, a.x + a.size + 5 + tw, a.y - 7, a.y + 7]],
      ['end', a.x - a.size - 5, a.y + 4, [a.x - a.size - 5 - tw, a.x - a.size - 5, a.y - 7, a.y + 7]],
      ['middle', a.x, a.y - a.size - 5, [a.x - tw / 2, a.x + tw / 2, a.y - a.size - 5 - h, a.y - a.size - 3]],
      ['middle', a.x, a.y + a.size + 13, [a.x - tw / 2, a.x + tw / 2, a.y + a.size + 3, a.y + a.size + 3 + h]],
    ]
    let best: (typeof c)[number] | null = null
    let bs = Infinity
    c.forEach((cand) => {
      const bx = cand[3]
      if (bx[0] < L || bx[1] > W - R || bx[2] < T || bx[3] > H - B) return
      const sc = obst.reduce((t, o) => t + ovl(bx, o), 0)
      if (sc < bs) {
        bs = sc
        best = cand
      }
    })
    const pick = best ?? c[0]
    a.la = pick[0]
    a.lx = pick[1]
    a.ly = pick[2]
    obst.push(pick[3])
  })
  return nodes
}

function SegmenChart({ list, sel, onSel, onOpen, thresholds }: { list: BoardItem[]; sel: Segment | null; onSel: (k: Segment) => void; onOpen: (id: string) => void; thresholds: { freq_per_month: number; size_idr: number } }) {
  const wrap = useRef<HTMLDivElement>(null)
  const [hover, setHover] = useState<{ d: BoardItem; x: number; y: number } | null>(null)
  const xq = px(thresholds.freq_per_month)
  const yq = py(thresholds.size_idr)
  const nodes = useMemo(() => layout(list), [list])
  const Z: [Segment, number, number, number, number][] = [['A', xq, T, W - R - xq, yq - T], ['C', L, T, xq - L, yq - T], ['B', xq, yq, W - R - xq, H - B - yq], ['D', L, yq, xq - L, H - B - yq]]
  const ZL: [Segment, number, number, 'start' | 'end'][] = [['A', W - R - 10, T + 22, 'end'], ['C', L + 10, T + 22, 'start'], ['B', W - R - 10, H - B - 28, 'end'], ['D', L + 10, H - B - 28, 'start']]
  const move = (e: MouseEvent, d: BoardItem) => {
    const r = wrap.current!.getBoundingClientRect()
    let x = e.clientX - r.left + 14
    let y = e.clientY - r.top + 14
    if (x + 250 > r.width) x -= 270
    if (y + 210 > r.height) y -= 210
    setHover({ d, x, y })
  }
  const hd = hover?.d
  const fq = (s: number) => String(s).replace('.', ',')
  return (
    <div className="orbit-wrap kuad-wrap" ref={wrap} onMouseLeave={() => setHover(null)}>
      <svg className="orbit kuad" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Segmen dealer: sumbu X seringnya order, sumbu Y besarnya order">
        {Z.map(([k, x, y, w, h]) => (
          <rect key={k} x={x.toFixed(1)} y={y.toFixed(1)} width={w.toFixed(1)} height={h.toFixed(1)} fill={`var(--${KUAD[k].k})`} opacity={sel && sel !== k ? 0.015 : k === 'D' ? 0.04 : 0.06} className="kz" onClick={() => onSel(k)} />
        ))}
        {[5e6, 10e6, 20e6, 50e6, 100e6, 200e6].map((v) => {
          const y = py(v)
          return (
            <g key={v}>
              <line x1={L} y1={y.toFixed(1)} x2={W - R} y2={y.toFixed(1)} className="kg" />
              <text x={L - 8} y={(y + 4).toFixed(1)} textAnchor="end" className="kt">{fmtRp(v).replace('Rp ', '')}</text>
            </g>
          )
        })}
        {[0.5, 1, 1.5, 2, 2.5, 3].map((x) => {
          const X = px(x)
          return (
            <g key={x}>
              <line x1={X.toFixed(1)} y1={T} x2={X.toFixed(1)} y2={H - B} className="kg" />
              <text x={X.toFixed(1)} y={H - B + 16} textAnchor="middle" className="kt">{fq(x)}×</text>
              <text x={X.toFixed(1)} y={H - B + 29} textAnchor="middle" className="kt sm">{Math.round(30 / x)} hr</text>
            </g>
          )
        })}
        <text x={(L + W - R) / 2} y={H - 10} textAnchor="middle" className="kax">SERINGNYA ORDER · order per bulan (baris bawah: siklus order)</text>
        <text transform={`translate(16 ${(T + H - B) / 2}) rotate(-90)`} textAnchor="middle" className="kax">BESARNYA ORDER · Rp per order</text>
        <line x1={xq.toFixed(1)} y1={T} x2={xq.toFixed(1)} y2={H - B} className="kq" />
        <line x1={L} y1={yq.toFixed(1)} x2={W - R} y2={yq.toFixed(1)} className="kq" />
        <text x={(xq + 5).toFixed(1)} y={H - B - 8} className="kt sm" fill="var(--text-2)">sering: ≥ {fq(thresholds.freq_per_month)}×/bln · siklus order ≤ {Math.round(30 / thresholds.freq_per_month)} hr</text>
        <text x={W - R - 5} y={(yq - 7).toFixed(1)} textAnchor="end" className="kt sm" fill="var(--text-2)">besar: ≥ {fmtRp(thresholds.size_idr)} / order</text>
        {ZL.map(([k, x, y, a]) => (
          <g key={k} className={`kzl ${sel && sel !== k ? 'dim' : ''}`} onClick={() => onSel(k)}>
            <text x={x} y={y} textAnchor={a} className="kzn" fill={`var(--${zoneColor(k)})`}>{KUAD[k].n.toUpperCase()}</text>
            <text x={x} y={y + 14} textAnchor={a} className="kt sm">{KUAD[k].s}</text>
          </g>
        ))}
        {nodes.filter((a) => a.pv).map((a) => (
          <g key={'p' + a.d.id}>
            <line x1={a.pv!.x.toFixed(1)} y1={a.pv!.y.toFixed(1)} x2={a.x.toFixed(1)} y2={a.y.toFixed(1)} className="ktrail" />
            <circle cx={a.pv!.x.toFixed(1)} cy={a.pv!.y.toFixed(1)} r={(a.size * 0.6).toFixed(1)} className="kprev" />
          </g>
        ))}
        {nodes.map(({ d, x, y, size, name, k, baru, la, lx, ly, label }) => {
          const tone = toneOf(d.metrics.credit.state)
          return (
            <g key={d.id} className={`dn ${sel && sel !== k ? 'ghost' : ''}`} tabIndex={0} onClick={() => onOpen(d.id)} onMouseMove={(e) => move(e, d)}>
              <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={size.toFixed(1)} fill={baru ? 'var(--surface)' : `var(--${tone === 'neutral' ? 'text-3' : tone})`} {...(baru ? { stroke: 'var(--text-3)', strokeDasharray: '3 3' } : {})} />
              {label && <text x={lx.toFixed(1)} y={ly.toFixed(1)} textAnchor={la} className={`dn-t ${d.metrics.avg_order < 10e6 ? 'sm' : ''}`}>{name}</text>}
            </g>
          )
        })}
      </svg>
      {hd ? (
        <div className="tip show" style={{ left: hover!.x, top: hover!.y }}>
          <b>{hd.name}</b>
          <div className="r"><span>{hd.city} · tier {hd.tier} · {hd.owner.name}</span></div>
          <div className="r"><span>Segmen</span><span>{KUAD[hd.metrics.segment].n}{hd.prev && hd.prev.segment !== hd.metrics.segment ? ' ← ' + KUAD[hd.prev.segment].n : ''}</span></div>
          <div className="r"><span>Seringnya</span><span>{hd.metrics.rhythm_days ? `${fx1(30 / hd.metrics.rhythm_days)}×/bln · siklus order ${hd.metrics.rhythm_days} hr` : 'baru · 1 order'}</span></div>
          <div className="r"><span>Besarnya</span><span>{fmtRp(hd.metrics.avg_order)} / order</span></div>
          <div className="r"><span>Omzet</span><span>{fmtRp(hd.metrics.omzet_bln)} / bln</span></div>
          <div className="r"><span>Share of wallet · sisa limit</span><span>{hd.metrics.sow}% · {hd.metrics.credit.state}</span></div>
          <div className="r" style={{ marginTop: 4, opacity: 0.7 }}><span>{KUAD[hd.metrics.segment].play} · klik untuk buka</span></div>
        </div>
      ) : (
        <div className="tip" />
      )}
    </div>
  )
}

export function segmenMover(m: Mover) {
  if (m.kind === 'moved') {
    const why: string[] = []
    if (m.prev_rhythm_days !== m.rhythm_days) why.push(`siklus order ${m.prev_rhythm_days} → ${m.rhythm_days} hr`)
    if (m.prev_avg_order !== m.avg_order) why.push(`${fmtRp(m.prev_avg_order ?? 0)} → ${fmtRp(m.avg_order ?? 0)}/order`)
    const to = KUAD[m.to!]
    const tail = m.up ? to.play.toLowerCase() : m.root_cause === 'project_unpaid' ? 'proyek belum cair — ikuti proyeknya, tagih dulu' : to.play.toLowerCase()
    return { k: m.up ? 'good' : 'warn', i: m.up ? 'trend' : 'refresh', t: `${m.name}: ${KUAD[m.from!].n} → ${to.n}`, s: why.join(' · ') + ' · ' + tail }
  }
  if (m.kind === 'slowing') return { k: 'warn', i: 'refresh', t: `${m.name}: tetap ${KUAD[m.to!].n}, tapi melambat`, s: `siklus order ${m.prev_rhythm_days} → ${m.rhythm_days} hr · masih sering, awasi sebelum turun segmen` }
  return { k: 'good', i: 'trend', t: `${m.name}: ${KUAD[m.to!].n} makin kuat`, s: `${fmtRp(m.prev_avg_order ?? 0)} → ${fmtRp(m.avg_order ?? 0)}/order · share of wallet ${m.sow}% — kandidat kenaikan limit` }
}

export function SegmenPage() {
  const [sales, setSales] = useState('all')
  const [sel, setSel] = useState<Segment | null>(null)
  const nav = useNavigate()
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const { data } = useSegmen(sales)
  const { data: sum } = useSegmenSummary(sales)
  const { data: movers = [] } = useSegmenMovers(sales)
  const list = useMemo(() => data?.items ?? [], [data])
  const toggle = (k: Segment) => setSel(sel === k ? null : k)
  const total = sum?.total_omzet_bln ?? 0
  const selList = useMemo(() => (sel ? list.filter((d) => d.metrics.segment === sel).sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln) : []), [list, sel])
  const [selShown, selMore] = useMore(selList, 15)
  return (
    <div className="net">
      <div>
        <div className="card orbit-card">
          <div className="orbit-hud">
            <SalesFilters value={sales} onChange={setSales} />
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
              <span className="meta">{list.length} dealer · {fmtRp(total)}/bln</span>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} disabled={orch.running} onClick={() => reanalyze('screen:segmen')}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} onClick={() => nav('/panduan#t-kuadran')}><Icon name="doc" />Cara baca</button>
            </div>
          </div>
          {data && <SegmenChart list={list} sel={sel} onSel={toggle} onOpen={(id) => nav('/dealer/' + id)} thresholds={data.thresholds} />}
          <Legend tail={`Ukuran = share of wallet · X = order per bulan (30 ÷ siklus order) · Y = Rp per order · titik putus = posisi 3 bulan lalu`} />
        </div>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Isi segmen</h2><span className="meta">Dealer · omzet per bulan</span></div>
          <ul className="pulse" id="kuad-rings">
            {(sum?.items ?? []).filter((r) => r.count > 0 || r.segment !== 'Baru').map((r) => (
              <li key={r.segment} className={sel === r.segment ? 'is-sel' : ''} style={{ cursor: 'pointer' }} onClick={() => toggle(r.segment)}>
                <span className="lbl"><b style={{ color: `var(--${textColor(r.segment)})` }}>{KUAD[r.segment].n}</b> · {r.count} dealer</span>
                <em className="num">{fmtRp(r.omzet_bln)}/bln</em>
                <span className="dl n">{r.pct}% omzet · {KUAD[r.segment].s}</span>
              </li>
            ))}
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Yang dilakukan</h2><span className="meta">{sel ? KUAD[sel].s : 'klik segmen untuk detail'}</span></div>
          <div className="kplays">
            {sel ? (
              <>
                <div className="kp is-open">
                  <b style={{ color: `var(--${textColor(sel)})` }}>{KUAD[sel].n} — {KUAD[sel].play}</b>
                  <p>{KUAD[sel].desc}</p>
                  {KUAD[sel].risk && <p className="risk"><Icon name="alert" /><span>{KUAD[sel].risk}</span></p>}
                  <small>Agen: {KUAD[sel].agent}</small>
                </div>
                <ul className="kdl">
                  {selList.length === 0 && <li><span style={{ color: 'var(--text-3)' }}>Tidak ada dealer</span></li>}
                  {selShown.map((d) => (
                    <li key={d.id}>
                      <button className="ev" onClick={() => nav('/dealer/' + d.id)}>{d.name}</button>
                      <span>{d.metrics.rhythm_days ? fx1(30 / d.metrics.rhythm_days) + '×/bln' : 'baru'} · {fmtRp(d.metrics.avg_order)}/order</span>
                      {d.next && <ActBtn small next={d.next} />}
                    </li>
                  ))}
                </ul>
                {selMore}
                <button className="btn quiet" style={{ height: 28, fontSize: 12, marginTop: 10 }} onClick={() => setSel(null)}>Tutup</button>
              </>
            ) : (
              SEGMENT_ORDER.slice(0, 4).map((k) => (
                <div className="kp" key={k} onClick={() => setSel(k)}>
                  <b style={{ color: `var(--${textColor(k)})` }}>{KUAD[k].n}</b>
                  <span>{KUAD[k].play}</span>
                  <small>{KUAD[k].agent}</small>
                </div>
              ))
            )}
          </div>
        </div>
        <div className="card">
          <div className="card-h"><h2>Berpindah segmen</h2><span className="ai" style={{ marginLeft: 6 }}>3 bulan terakhir</span></div>
          <MoverList items={movers} render={segmenMover} />
        </div>
      </div>
    </div>
  )
}

