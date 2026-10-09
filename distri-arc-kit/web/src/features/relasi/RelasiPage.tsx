import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import type { Relasi } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useOrbit, useRelasi, useRelasiInsights, useSales } from '../../app/queries'
import { FullscreenButton, useFullscreen } from '../../components/MapViewport'
import { EMPTY, activeCount, applyFilter, type OrbitFilter } from '../orbit/filters'
import { OrbitFilterBar } from '../orbit/OrbitTools'
import { NetView } from './NetView'
import type { Positions } from './layout'

const PERIODS: [number, string][] = [[30, '30 hr'], [60, '60 hr'], [90, '90 hr'], [180, '180 hr']]
const PERIOD_LABEL: Record<number, string> = { 30: '30 hari', 60: '60 hari', 90: '90 hari', 180: '180 hari' }

/** Settles the first layout in a Web Worker (falls back to the main thread). */
function settleOff(g: Relasi): Promise<Positions | undefined> {
  return new Promise((resolve) => {
    if (typeof Worker === 'undefined') return resolve(undefined)
    try {
      const w = new Worker(new URL('./layout.worker.ts', import.meta.url), { type: 'module' })
      const done = (p?: Positions) => {
        w.terminate()
        resolve(p)
      }
      w.onmessage = (e: MessageEvent<Positions>) => done(e.data)
      w.onerror = () => done(undefined)
      w.postMessage({ nodes: g.nodes, edges: g.edges, months: g.months })
    } catch {
      resolve(undefined)
    }
  })
}

// Peta relasi (mockup screen-net).
export function RelasiPage() {
  const nav = useNavigate()
  const [period, setPeriod] = useState(30)
  const [sales, setSales] = useState('all')
  const { data: g } = useRelasi(period, 'all')
  const { data: scoped } = useRelasi(period, sales)
  const { data: ins = [] } = useRelasiInsights(period, sales)
  const { data: salesList = [] } = useSales()
  const { ref: stage, big, toggle: toggleFullscreen, className: fsClass } = useFullscreen<HTMLDivElement>()
  const [zoom, setZoom] = useState(1)
  // the same dealer filters as Orbit and Segmen; dealers that do not match fade out with their lines
  const { data: board = [] } = useOrbit('all')
  const [f, setFState] = useState<OrbitFilter>(EMPTY)
  const setF = (p: Partial<OrbitFilter>) => setFState((cur) => ({ ...cur, ...p }))
  const nFilters = activeCount(f)
  const matched = useMemo(() => (nFilters ? new Set(applyFilter(board, f).map((d) => d.id)) : null), [board, f, nFilters])
  const branches = useMemo(() => [...new Set(board.map((d) => d.branch).filter(Boolean))].sort(), [board])
  const mapDealers = useMemo(() => (g?.nodes ?? []).filter((n) => n.type === 'dealer'), [g])
  const shownDealers = matched ? mapDealers.filter((n) => matched.has(n.id)).length : mapDealers.length
  const canvas = useRef<HTMLCanvasElement>(null)
  const labels = useRef<HTMLDivElement>(null)
  const tip = useRef<HTMLDivElement>(null)
  const view = useRef<NetView | null>(null)
  const [, setFocus] = useState<string | null>(null)

  // build once with the first data; later periods reweight the same view
  useEffect(() => {
    if (!g || view.current || !stage.current || !canvas.current || !labels.current || !tip.current) return
    let cancelled = false
    const els = { stage: stage.current, canvas: canvas.current, labels: labels.current, tip: tip.current }
    void settleOff(g).then((pos) => {
      if (cancelled || view.current) return
      const v = new NetView({ ...els, monthLabels: g.month_labels, periodLabel: PERIOD_LABEL[g.period_days], onOpen: (id) => nav('/dealer/' + id), onFocus: setFocus, onZoom: setZoom }, g.nodes, g.edges, g.months, pos)
      view.current = v
      v.start()
    })
    return () => {
      cancelled = true
    }
  }, [g, nav, stage])

  useEffect(() => () => {
    view.current?.destroy()
    view.current = null
  }, [])

  useEffect(() => {
    if (g && view.current) view.current.update(g.nodes, g.edges, g.months, g.month_labels, PERIOD_LABEL[g.period_days])
  }, [g])

  useEffect(() => {
    const v = view.current
    if (!v || !g) return
    const key = 's-' + sales
    const bySales = (id: string) => sales === 'all' || id === key || g.edges.some((e) => e.sales === key && e.dealer === id && e.w > 0)
    const dealerOk = (id: string) => !matched || matched.has(id)
    // a sales number stays lit while it talks to at least one dealer that passes the filters
    const salesOk = (id: string) => !matched || g.edges.some((e) => e.sales === id && e.w > 0 && matched.has(e.dealer))
    v.filter = (id) => bySales(id) && (id.startsWith('s-') ? salesOk(id) : dealerOk(id))
    v.focus = null
  }, [sales, g, matched])

  const pairs = scoped?.pairs ?? []
  const max = pairs[0]?.w || 1
  return (
    <div className="net">
      <div>
        <div className="card relasi-filter">
          <OrbitFilterBar f={f} set={setF} branches={branches} shown={shownDealers} total={mapDealers.length} />
        </div>
        <div className={`net-stage${fsClass}`} ref={stage}>
          <canvas ref={canvas} />
          <div ref={labels} />
          <div className="net-hud">
            <span className="pill accent"><Icon name="chat" />WhatsApp + order · <span>{PERIOD_LABEL[period]} terakhir</span></span>
            <span className="pill neutral">{scoped ? `${scoped.connections.toLocaleString('id-ID')} koneksi · ${scoped.interactions.toLocaleString('id-ID')} interaksi` : ''}</span>
            {g && g.dealers_active > g.dealers_shown && <span className="pill neutral">{g.dealers_shown} dealer teraktif dari {g.dealers_active.toLocaleString('id-ID')}</span>}
          </div>
          <div className="net-legend"><span><i style={{ background: 'var(--accent)' }} />Nomor sales</span><span><i style={{ background: 'var(--good)' }} />Skor dealer kuat</span><span><i style={{ background: 'var(--warn)' }} />50–69</span><span><i style={{ background: 'var(--bad)' }} />&lt; 50</span><span>Ukuran = interaksi/bulan · Jarak = kedekatan</span></div>
          {g && g.nodes.length === 0 && <div className="net-empty">Belum ada interaksi WhatsApp atau order dalam 6 bulan terakhir.</div>}
          <div className="map-controls" role="toolbar" aria-label="Kontrol peta">
            <div className="map-ctl-group" aria-label="Zoom">
              <button type="button" aria-label="Zoom out" title="Zoom out (scroll ke bawah)" onClick={() => view.current?.zoomBy(1 / 1.4)}>−</button>
              <span aria-live="polite" title="Zoom">{Math.round(zoom * 100)}%</span>
              <button type="button" aria-label="Zoom in" title="Zoom in (scroll ke atas · cubit)" onClick={() => view.current?.zoomBy(1.4)}>+</button>
            </div>
            <button type="button" aria-label="Atur ulang peta" title="Atur ulang: seluruh jaringan" onClick={() => view.current?.reset()}>↺</button>
            <FullscreenButton big={big} onClick={toggleFullscreen} />
          </div>
          {matched && shownDealers === 0 && <div className="net-empty">Tidak ada dealer di peta yang cocok dengan filter ini.</div>}
          <div className="net-hint">Seret untuk memutar · Shift+seret / klik kanan untuk menggeser · scroll / cubit untuk zoom · klik dealer dua kali untuk membuka</div>
          <div className="tip" ref={tip} />
        </div>
      </div>
      <div className="stack">
        <div className="card">
          <div className="card-h"><h2>Lihat dari</h2><span className="meta">Filter nomor sales</span></div>
          <div className="net-filters">
            {salesList.length > 8 ? ( // a real team: a picker, not a wall of buttons
              <select className="sales-pick" value={sales} onChange={(e) => setSales(e.target.value)} aria-label="Sales">
                <option value="all">Semua sales ({salesList.length})</option>
                {[...salesList].sort((a, b) => a.name.localeCompare(b.name)).map((s) => <option key={s.key} value={s.key}>{s.name}{s.branch ? ` · ${s.branch}` : ''}</option>)}
              </select>
            ) : [['all', 'Semua sales'] as const, ...salesList.map((s) => [s.key, s.name] as const)].map(([id, n]) => (
              <button key={id} className={sales === id ? 'is-active' : ''} onClick={() => setSales(id)}>{n}</button>
            ))}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 12, flexWrap: 'wrap' }}>
            <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)' }}>Periode</span>
            <div className="seg">{PERIODS.map(([d, l]) => <button key={d} className={period === d ? 'is-active' : ''} onClick={() => setPeriod(d)}>{l}</button>)}</div>
          </div>
          <div className="hr" />
          <ul className="pairs">
            <li style={{ border: 0, paddingTop: 0 }}><span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)' }}>Pasangan terkuat</span></li>
            {pairs.map((p) => (
              <li key={p.sales + p.dealer}><div><div>{p.sales} ↔ <b>{p.dealer}</b></div><div className="pb"><i style={{ width: `${(p.w / max) * 100}%` }} /></div></div><em className="num">{p.w}</em></li>
            ))}
          </ul>
        </div>
        <div className="card">
          <div className="card-h"><h2>Pola relasi</h2><span className="ai" style={{ marginLeft: 6 }}>dibaca GSI Orbit</span></div>
          <ul className="ins">
            {ins.length === 0 && <li><span /><div><span>Tidak ada pola khusus.</span></div></li>}
            {ins.map((x) => (
              <li key={x.title} onClick={() => nav('/dealer/' + x.dealer)} style={{ cursor: 'pointer' }}>
                <span className="ii" style={{ background: `var(--${x.tone}-soft)`, color: `var(--${x.tone})` }}><Icon name={x.icon} /></span>
                <div><b>{x.title}</b><span>{x.text}</span></div>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  )
}
