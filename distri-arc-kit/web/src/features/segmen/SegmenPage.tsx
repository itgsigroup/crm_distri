import { useEffect, useRef, useState, type FocusEvent, type MouseEvent, useMemo } from 'react'
import { useNavigate } from 'react-router'
import type { BoardItem, Mover, Segment, SegmentSummary } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { fmtRp, fx1, shortName } from '../../lib/format'
import { KUAD } from '../../lib/i18n/id'
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
  baru: boolean
}

const SEGMEN_PRIORITY = 150

function topByOmzet(list: BoardItem[], count = SEGMEN_PRIORITY) {
  return list.length <= count ? list : [...list].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln).slice(0, count)
}

function layout(list: BoardItem[], dense = false): Node[] {
  const compact = dense && list.length > SEGMEN_PRIORITY
  return list.map((d) => {
    const m = d.metrics
    const size = 7 + Math.sqrt(m.sow) * 1.3
    return { d, x: xOf(m.freq), y: clampY(m.avg_order), size: compact ? Math.max(3, size * 0.35) : size, baru: m.freq == null }
  }).sort((a, b) => a.d.metrics.omzet_bln - b.d.metrics.omzet_bln)
}

function SegmenChart({ list, sel, onSel, onOpen, thresholds }: { list: BoardItem[]; sel: Segment | null; onSel: (k: Segment | null) => void; onOpen: (id: string) => void; thresholds: { freq_per_month: number; size_idr: number } }) {
  const wrap = useRef<HTMLDivElement>(null)
  const [hover, setHover] = useState<{ d: BoardItem; x: number; y: number } | null>(null)
  const [all, setAll] = useState(false)
  const [query, setQuery] = useState('')
  const xq = px(thresholds.freq_per_month)
  const yq = py(thresholds.size_idr)
  const scoped = useMemo(() => sel ? list.filter((d) => d.metrics.segment === sel) : list, [list, sel])
  const matches = useMemo(() => {
    const q = query.trim().toLocaleLowerCase('id-ID')
    return q ? scoped.filter((d) => d.name.toLocaleLowerCase('id-ID').includes(q) || d.city.toLocaleLowerCase('id-ID').includes(q)) : scoped
  }, [scoped, query])
  const shown = useMemo(() => all ? matches : topByOmzet(matches), [all, matches])
  const nodes = useMemo(() => layout(shown, all), [shown, all])
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
  const showFocus = (e: FocusEvent<SVGGElement>, d: BoardItem) => {
    const r = wrap.current!.getBoundingClientRect()
    const dot = e.currentTarget.getBoundingClientRect()
    let x = dot.left - r.left + dot.width / 2 + 14
    let y = dot.top - r.top + dot.height / 2 + 14
    if (x + 250 > r.width) x = Math.max(8, x - 270)
    if (y + 210 > r.height) y = Math.max(8, y - 210)
    setHover({ d, x, y })
  }
  const hd = hover?.d
  const fq = (s: number) => String(s).replace('.', ',')
  return (
    <div>
      <div className="kuad-controls">
        <select className="st-filter" value={sel ?? ''} onChange={(e) => onSel((e.target.value || null) as Segment | null)} aria-label="Filter segmen">
          <option value="">Semua segmen</option>
          {(['A', 'B', 'C', 'D', 'Baru'] as Segment[]).map((k) => <option key={k} value={k}>{KUAD[k].nick}</option>)}
        </select>
        <input className="st-filter kuad-search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Cari pelanggan atau kota" aria-label="Cari pelanggan atau kota" />
        <span className="meta">{all ? `${matches.length.toLocaleString('id-ID')} pelanggan` : `${shown.length.toLocaleString('id-ID')} prioritas dari ${matches.length.toLocaleString('id-ID')}`}</span>
        <div className="seg" role="radiogroup" aria-label="Jumlah titik yang ditampilkan">
          <button role="radio" aria-checked={!all} className={!all ? 'is-active' : ''} onClick={() => setAll(false)}>Prioritas</button>
          <button role="radio" aria-checked={all} className={all ? 'is-active' : ''} onClick={() => setAll(true)}>Semua titik</button>
        </div>
      </div>
      <div className="orbit-wrap kuad-wrap" ref={wrap} onMouseLeave={() => setHover(null)}>
      <svg className="orbit kuad" viewBox={`0 0 ${W} ${H}`} role="group" aria-label="Peta segmen pelanggan">
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
        <text x={(L + W - R) / 2} y={H - 10} textAnchor="middle" className="kax">← jarang order · BERAPA KALI ORDER SEBULAN · sering order →</text>
        <text transform={`translate(16 ${(T + H - B) / 2}) rotate(-90)`} textAnchor="middle" className="kax">BESAR SEKALI ORDER (Rp) · makin atas makin besar →</text>
        <line x1={xq.toFixed(1)} y1={T} x2={xq.toFixed(1)} y2={H - B} className="kq" />
        <line x1={L} y1={yq.toFixed(1)} x2={W - R} y2={yq.toFixed(1)} className="kq" />
        <text x={(xq + 5).toFixed(1)} y={H - B - 8} className="kt sm" fill="var(--text-2)">sering: ≥ {fq(thresholds.freq_per_month)}×/bln</text>
        <text x={W - R - 5} y={(yq - 7).toFixed(1)} textAnchor="end" className="kt sm" fill="var(--text-2)">besar: ≥ {fmtRp(thresholds.size_idr)} / order</text>
        {ZL.map(([k, x, y, a]) => (
          <g key={k} className={`kzl ${sel && sel !== k ? 'dim' : ''}`} onClick={() => onSel(k)}>
            <text x={x} y={y} textAnchor={a} className="kzn" fill={`var(--${zoneColor(k)})`}>{KUAD[k].n.toUpperCase()}</text>
            <text x={x} y={y + 15} textAnchor={a} className="kzs">{KUAD[k].nick}</text>
          </g>
        ))}
        {nodes.map(({ d, x, y, size, baru }) => {
          const tone = toneOf(d.metrics.credit.state)
          return (
            <g key={d.id} className="dn" role="button" tabIndex={0} aria-label={d.name} onClick={() => onOpen(d.id)} onMouseMove={(e) => move(e, d)} onFocus={(e) => showFocus(e, d)} onBlur={() => setHover(null)} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpen(d.id) } }}>
              <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={Math.max(size, 9).toFixed(1)} className="dn-hit" fill="transparent" pointerEvents="all" />
              <circle cx={x.toFixed(1)} cy={y.toFixed(1)} r={size.toFixed(1)} fill={baru ? 'var(--surface)' : `var(--${tone === 'neutral' ? 'text-3' : tone})`} className="dn-dot" {...(baru ? { stroke: 'var(--text-3)', strokeDasharray: '3 3' } : {})} />
            </g>
          )
        })}
      </svg>
      {hd ? (
        <div className="tip show" style={{ left: hover!.x, top: hover!.y }}>
          <b>{hd.name}</b>
          <div className="r"><span>{hd.city} · tier {hd.tier} · {hd.owner.name}</span></div>
          <div className="r"><span>Segmen</span><span>{KUAD[hd.metrics.segment].n}{hd.prev && hd.prev.segment !== hd.metrics.segment ? ' ← ' + KUAD[hd.prev.segment].n : ''}</span></div>
          <div className="r"><span>Seringnya</span><span>{hd.metrics.freq != null ? `${fx1(hd.metrics.freq)}×/bln${hd.metrics.rhythm_days ? ` · siklus order ${hd.metrics.rhythm_days} hr` : ''}` : 'baru · 1 order'}</span></div>
          <div className="r"><span>Besarnya</span><span>{fmtRp(hd.metrics.avg_order)} / order</span></div>
          <div className="r"><span>Omzet</span><span>{fmtRp(hd.metrics.omzet_bln)} / bln</span></div>
          <div className="r"><span>Porsi belanja di GSI · sisa limit</span><span>{hd.metrics.sow}% · {hd.metrics.credit.state}</span></div>
          <div className="r" style={{ marginTop: 4, opacity: 0.7 }}><span>{KUAD[hd.metrics.segment].play} · klik untuk buka</span></div>
        </div>
      ) : (
        <div className="tip" />
      )}
      {nodes.length === 0 && <div className="net-empty">Tidak ada pelanggan yang cocok dengan filter ini.</div>}
      </div>
    </div>
  )
}

export function segmenMover(m: Mover) {
  if (m.kind === 'moved') {
    const why: string[] = []
    if (m.prev_rhythm_days !== m.rhythm_days) why.push(`siklus order ${m.prev_rhythm_days} → ${m.rhythm_days} hari`)
    if (m.prev_avg_order !== m.avg_order) why.push(`${fmtRp(m.prev_avg_order ?? 0)} → ${fmtRp(m.avg_order ?? 0)} per order`)
    const to = KUAD[m.to!]
    const tail = m.up ? to.todo : m.root_cause === 'project_unpaid' ? 'proyek belum cair — tagih dulu, lalu ikuti proyeknya' : to.todo
    return { k: m.up ? 'good' : 'warn', i: m.up ? 'trend' : 'refresh', t: `${m.name} ${m.up ? 'naik' : 'turun'} ke ${to.n} (${to.nick.toLowerCase()})`, s: `Dari ${KUAD[m.from!].n}. ${why.join(' · ')}. Saran: ${tail.charAt(0).toLowerCase() + tail.slice(1)}.` }
  }
  if (m.kind === 'slowing') return { k: 'warn', i: 'refresh', t: `${m.name} mulai jarang order`, s: `Masih ${KUAD[m.to!].n}, tapi siklus order ${m.prev_rhythm_days} → ${m.rhythm_days} hari. Saran: hubungi sebelum turun segmen.` }
  return { k: 'good', i: 'trend', t: `${m.name} makin kuat`, s: `${fmtRp(m.prev_avg_order ?? 0)} → ${fmtRp(m.avg_order ?? 0)} per order · porsi belanja di GSI ${m.sow}%. Saran: pertimbangkan naik limit.` }
}

const BOX_ORDER: Segment[] = ['A', 'B', 'C', 'D'] // rows: sering / jarang · columns: besar / kecil
const VIEW_KEY = 'segmen.view'

/** The default Segmen view: four boxes in plain words — who is in it, how much they bring, what to do. */
function SegmenBoxes({ list, summary, sel, onSel, thresholds }: { list: BoardItem[]; summary: SegmentSummary[]; sel: Segment | null; onSel: (k: Segment) => void; thresholds: { freq_per_month: number; size_idr: number } }) {
  const top = useMemo(() => {
    const by: Partial<Record<Segment, BoardItem[]>> = {}
    for (const d of list) (by[d.metrics.segment] ??= []).push(d)
    Object.values(by).forEach((a) => a!.sort((x, y) => y.metrics.omzet_bln - x.metrics.omzet_bln))
    return by
  }, [list])
  const row = (k: Segment) => summary.find((r) => r.segment === k) ?? { segment: k, count: 0, omzet_bln: 0, pct: 0 }
  const freq = fx1(thresholds.freq_per_month)
  const size = fmtRp(thresholds.size_idr)
  const tags: Record<string, [string, boolean][]> = {
    A: [['Sering order', true], ['Order besar', true]],
    B: [['Sering order', true], ['Order kecil', false]],
    C: [['Jarang order', false], ['Order besar', true]],
    D: [['Jarang order', false], ['Order kecil', false]],
  }
  const baru = row('Baru')
  return (
    <div className="sg">
      <p className="sg-how">Dealer dibagi 4 kotak menurut dua hal: <b>seberapa sering order</b> (patokan {freq}× sebulan) dan <b>seberapa besar sekali order</b> (patokan {size}). Klik kotak untuk melihat dealernya.</p>
      <div className="sg-grid">
        {BOX_ORDER.map((k) => {
          const r = row(k)
          const names = (top[k] ?? []).slice(0, 3).map((d) => shortName(d.name))
          const rest = r.count - names.length
          return (
            <button key={k} type="button" className={`sg-box sg-${k}${sel === k ? ' is-sel' : ''}${sel && sel !== k ? ' is-dim' : ''}`} onClick={() => onSel(k)} aria-pressed={sel === k}>
              <span className="sg-head">
                <span className="sg-letter">{k}</span>
                <span className="sg-name"><b>{KUAD[k].nick}</b><small>{KUAD[k].n}</small></span>
              </span>
              <span className="sg-tags">{tags[k].map(([t, up]) => <span key={t} className={up ? 'up' : ''}>{t}</span>)}</span>
              <span className="sg-nums">
                <span><em>{r.count.toLocaleString('id-ID')}</em> dealer</span>
                <span><em>{fmtRp(r.omzet_bln)}</em> /bulan</span>
              </span>
              <span className="sg-bar" aria-label={`${r.pct}% dari total omzet`}><i style={{ width: `${Math.max(r.pct, r.count ? 2 : 0)}%` }} /></span>
              <span className="sg-pct"><b>{r.pct}%</b> dari total omzet</span>
              <span className="sg-do"><small>Yang dilakukan</small>{KUAD[k].todo}</span>
              <span className="sg-top">{names.length ? <>Contoh: {names.join(', ')}{rest > 0 ? ` +${rest.toLocaleString('id-ID')} lainnya` : ''}</> : 'Belum ada dealer di sini'}</span>
            </button>
          )
        })}
      </div>
      {baru.count > 0 && (
        <button type="button" className={`sg-new${sel === 'Baru' ? ' is-sel' : ''}`} onClick={() => onSel('Baru')}>
          <b>{baru.count.toLocaleString('id-ID')} dealer baru</b>
          <span>order pertama ≤ 90 hari — belum masuk kotak, {KUAD.Baru.todo.toLowerCase()}</span>
          <em>{fmtRp(baru.omzet_bln)}/bln</em>
        </button>
      )}
    </div>
  )
}

function readView(): 'kotak' | 'titik' {
  try {
    return localStorage.getItem(VIEW_KEY) === 'titik' ? 'titik' : 'kotak'
  } catch {
    return 'kotak'
  }
}

export function SegmenPage() {
  const [sales, setSales] = useState('all')
  const [sel, setSel] = useState<Segment | null>(null)
  const [view, setViewState] = useState(readView)
  const setView = (v: 'kotak' | 'titik') => {
    setViewState(v)
    try {
      localStorage.setItem(VIEW_KEY, v)
    } catch {
      /* private window: the choice just isn't remembered */
    }
  }
  const nav = useNavigate()
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const { data } = useSegmen(sales)
  const { data: sum } = useSegmenSummary(sales)
  const { data: movers = [] } = useSegmenMovers(sales)
  const list = useMemo(() => data?.items ?? [], [data])
  const toggle = (k: Segment) => setSel(sel === k ? null : k)
  const chooseSegment = (k: Segment | null) => setSel(k)
  const total = sum?.total_omzet_bln ?? 0
  const selList = useMemo(() => (sel ? list.filter((d) => d.metrics.segment === sel).sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln) : []), [list, sel])
  const [selShown, selMore] = useMore(selList, 15)
  const detail = useRef<HTMLDivElement>(null)
  // one column (phone/tablet): the dealer list sits below the boxes, so bring it into view
  useEffect(() => {
    if (sel && window.innerWidth <= 1100) detail.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [sel])
  return (
    <div className="net">
      <div>
        <div className="card orbit-card">
          <div className="orbit-hud">
            <SalesFilters value={sales} onChange={setSales} />
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
              <span className="meta">{list.length.toLocaleString('id-ID')} dealer · {fmtRp(total)}/bln</span>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} disabled={orch.running} onClick={() => reanalyze('screen:segmen')}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" style={{ height: 30, fontSize: 12 }} onClick={() => nav('/panduan#t-kuadran')}><Icon name="doc" />Cara baca</button>
            </div>
          </div>
          <div className="sg-bar-h">
            <h2>Isi segmen</h2>
            <div className="seg" role="tablist" aria-label="Tampilan segmen">
              <button role="tab" aria-selected={view === 'kotak'} className={view === 'kotak' ? 'is-active' : ''} onClick={() => setView('kotak')}>Kotak</button>
              <button role="tab" aria-selected={view === 'titik'} className={view === 'titik' ? 'is-active' : ''} onClick={() => setView('titik')}>Peta titik</button>
            </div>
          </div>
          {data && view === 'kotak' && <SegmenBoxes list={list} summary={sum?.items ?? []} sel={sel} onSel={toggle} thresholds={data.thresholds} />}
          {data && view === 'titik' && (
            <>
              <SegmenChart list={list} sel={sel} onSel={chooseSegment} onOpen={(id) => nav('/dealer/' + id)} thresholds={data.thresholds} />
              <Legend tail="Titik besar = porsi belanja di GSI lebih besar · arahkan kursor untuk melihat data" />
            </>
          )}
          {!!sum?.prospects && (
            <p className="prospek-line"><b>{sum.prospects.toLocaleString('id-ID')} prospek</b> belum pernah order — tidak dihitung di sini. <button className="ev" onClick={() => nav('/dealer?status=Prospek')}>Lihat daftar</button></p>
          )}
        </div>
      </div>
      <div className="stack">
        <div className="card" ref={detail} style={{ scrollMarginTop: 12 }}>
          <div className="card-h"><h2>{sel ? `${KUAD[sel].nick} · ${KUAD[sel].n}` : 'Dealer per kotak'}</h2>{sel && <span className="meta">{selList.length.toLocaleString('id-ID')} dealer</span>}</div>
          <div className="kplays">
            {sel ? (
              <>
                <div className="kp is-open">
                  <b style={{ color: `var(--${textColor(sel)})` }}>{KUAD[sel].todo}</b>
                  <p>{KUAD[sel].desc}</p>
                  {KUAD[sel].risk && <p className="risk"><Icon name="alert" /><span>{KUAD[sel].risk}</span></p>}
                  <small>Dibantu: {KUAD[sel].agent}</small>
                </div>
                <ul className="kdl">
                  {selList.length === 0 && <li><span style={{ color: 'var(--text-3)' }}>Tidak ada dealer</span></li>}
                  {selShown.map((d) => (
                    <li key={d.id}>
                      <button className="ev" onClick={() => nav('/dealer/' + d.id)}>{d.name}</button>
                      <span>{d.metrics.freq != null ? fx1(d.metrics.freq) + '× sebulan' : 'baru'} · {fmtRp(d.metrics.avg_order)} per order</span>
                      {d.next && <ActBtn small next={d.next} />}
                    </li>
                  ))}
                </ul>
                {selMore}
                <button className="btn quiet" style={{ height: 28, fontSize: 12, marginTop: 10 }} onClick={() => setSel(null)}>Tutup</button>
              </>
            ) : (
              <p className="sg-hint">Klik salah satu kotak — daftar dealernya muncul di sini, lengkap dengan apa yang perlu dilakukan.</p>
            )}
          </div>
        </div>
        <div className="card">
          <div className="card-h"><h2>Pindah kotak</h2><span className="ai" style={{ marginLeft: 6 }}>3 bulan terakhir</span></div>
          <MoverList items={movers} render={segmenMover} />
        </div>
      </div>
    </div>
  )
}
