import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import type { BoardItem, KPI } from '../../api/types'
import { fmtNum, fmtRp } from '../../lib/format'
import { useStockCritical, useStockPush, useThreads } from '../../app/queries'
import { planMine } from './mine'
import { MINE_H, MineView, type MineTarget } from './MineView'

const KEY = 'dashboard.mine'
const read = () => {
  try {
    return localStorage.getItem(KEY) !== 'off'
  } catch {
    return true
  }
}

/** Tambang Distribusi: the dashboard as a 3D open-pit mine, driven by the dealer board and the KPIs. */
export function DashboardMine({ board, kpi, can }: { board: BoardItem[]; kpi: KPI | undefined; can: (screen: string) => boolean }) {
  const nav = useNavigate()
  const [on, setOnState] = useState(read)
  const setOn = (v: boolean) => {
    setOnState(v)
    try {
      localStorage.setItem(KEY, v ? 'on' : 'off')
    } catch {
      /* private window: not remembered */
    }
  }
  const { data: push = [] } = useStockPush()
  const { data: critical = [] } = useStockCritical()
  const { data: threads = [] } = useThreads('all', '')
  const unread = threads.reduce((a, t) => a + t.unread, 0)
  const stage = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const view = useRef<MineView | null>(null)
  const [width, setWidth] = useState(0)
  const [no3d, setNo3d] = useState(false)
  const [tip, setTip] = useState<{ t: MineTarget; x: number; y: number } | null>(null)
  // dig sites that stay readable: one per ~90 px of width
  const sites = Math.max(6, Math.min(16, Math.floor(width / 90)))
  const plan = useMemo(() => planMine(board, sites), [board, sites])
  const navRef = useRef(nav)
  const canRef = useRef(can)
  useEffect(() => { navRef.current = nav; canRef.current = can })

  useEffect(() => {
    const el = stage.current
    if (!el || !on) return
    const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => setWidth(el.clientWidth))
    ro?.observe(el)
    setWidth(el.clientWidth)
    return () => ro?.disconnect()
  }, [on])

  useEffect(() => {
    if (!on || !stage.current || !canvas.current) return
    const v = new MineView({
      stage: stage.current,
      canvas: canvas.current,
      stock: { aging: 0, critical: false },
      unread: 0,
      onPick: (t) => {
        if (t.kind === 'site') navRef.current('/dealer/' + t.site.id)
        else if (t.kind === 'warehouse' && canRef.current('stock')) navRef.current('/stok')
        else if (t.kind === 'office' && canRef.current('chat')) navRef.current('/chat')
      },
      onHover: (t, x, y) => setTip(t ? { t, x, y } : null),
    })
    if (!v.ok) {
      setNo3d(true)
      return
    }
    view.current = v
    v.start()
    return () => { v.destroy(); view.current = null }
  }, [on])

  useEffect(() => { view.current?.setPlan(plan) }, [plan, on])
  useEffect(() => { view.current?.setStock({ aging: push.length, critical: critical.length > 0 }, unread) }, [push.length, critical.length, unread, on])

  const active = board.filter((d) => d.metrics.segment !== 'Prospek')
  const omzet = active.reduce((a, d) => a + d.metrics.omzet_bln, 0)
  const score = kpi?.on_schedule_pct ?? 0
  const target = kpi?.targets.on_schedule_pct ?? 80
  const level = Math.max(1, Math.min(10, Math.round(score / 10)))

  if (!on) {
    return (
      <div className="mine-off">
        <button type="button" className="btn ghost" onClick={() => setOn(true)}>Tampilkan Tambang Distribusi</button>
      </div>
    )
  }
  return (
    <div className="card mine-card">
      <div className="mine-head">
        <div>
          <span className="db-eyebrow">GSI ORBIT · TAMBANG DISTRIBUSI</span>
          <h2>Hari ini di tambang dealer</h2>
        </div>
        <div className="mine-hud">
          <span className="hud-chip gold" title="Omzet dealer per bulan"><i>●</i>{fmtRp(omzet)}<small>/bln</small></span>
          <span className="hud-chip amber" title="Dealer dengan jadwal order ≤ 14 hari — excavator menggali, dump truck mengantar"><i>■</i>{fmtNum(plan.tasks.deliver.length)} antaran</span>
          <span className="hud-chip red" title="Dealer over limit / overdue — pickup merah menagih"><i>◆</i>{fmtNum(plan.tasks.collect.length)} tagihan</span>
          <span className="hud-chip purple" title="Dealer At risk — surveyor follow-up"><i>▲</i>{fmtNum(plan.tasks.visit.length)} follow-up</span>
          <span className="hud-level" title={`Order tepat jadwal ${score}% · target ${target}%`}>Level {level}<b><em style={{ width: `${Math.min(100, (score / Math.max(1, target)) * 100)}%` }} /></b><small>{score}% tepat jadwal</small></span>
          <button type="button" className="btn quiet mine-hide" onClick={() => setOn(false)} title="Sembunyikan tambang">Sembunyikan</button>
        </div>
      </div>
      <div className="mine-stage" ref={stage} style={{ height: MINE_H }}>
        <canvas ref={canvas} aria-label="Tambang Distribusi: dealer sebagai titik tambang, penambang, excavator, dump truck pengantar, pickup penagih, dan surveyor" role="img" />
        {no3d && <p className="mine-no3d">Tampilan 3D tidak didukung peramban ini (WebGL tidak tersedia).</p>}
        {!no3d && !plan.sites.length && <p className="mine-no3d">Belum ada dealer di tambang — data dealer sedang dimuat atau kosong.</p>}
        {tip && (
          <div className="tip show mine-tip" style={{ left: Math.min(tip.x + 14, width - 230), top: Math.max(6, tip.y - 70) }}>
            {tip.t.kind === 'site' ? (
              <>
                <b>{tip.t.site.name}</b>
                <div className="r"><span>{tip.t.site.look === 'key' ? 'Key account' : tip.t.site.look === 'risk' ? 'At risk' : tip.t.site.look === 'closed' ? 'Churn' : 'Aktif'}</span><span>{fmtRp(tip.t.site.omzet)}/bln</span></div>
                <div className="r"><span>Jadwal order</span><span>{tip.t.site.dueIn == null ? '—' : tip.t.site.dueIn >= 0 ? `${tip.t.site.dueIn} hari lagi` : `lewat ${-tip.t.site.dueIn} hari`}</span></div>
                <div className="r" style={{ opacity: 0.7, marginTop: 4 }}><span>Klik untuk buka dealer</span></div>
              </>
            ) : tip.t.kind === 'warehouse' ? (
              <><b>Gudang GSI</b><div className="r"><span>Stok menua</span><span>{fmtNum(push.length)} item</span></div><div className="r"><span>Stok kritis</span><span>{fmtNum(critical.length)} SKU</span></div>{can('stock') && <div className="r" style={{ opacity: 0.7, marginTop: 4 }}><span>Klik untuk Push stok</span></div>}</>
            ) : (
              <><b>Kantor sales</b><div className="r"><span>Chat belum dibaca</span><span>{fmtNum(unread)}</span></div>{can('chat') && <div className="r" style={{ opacity: 0.7, marginTop: 4 }}><span>Klik untuk buka Chat</span></div>}</>
            )}
          </div>
        )}
      </div>
      <div className="mine-legend">
        <span><i style={{ background: '#ff7a1a' }} />Penambang: satu titik tambang per dealer, omzet terbesar di dekat bibir tambang</span>
        <span><i style={{ background: '#f2a900' }} />Excavator + dump truck: antar ke dealer yang jadwal order ≤ 14 hari</span>
        <span><i style={{ background: '#e5534b' }} />Pickup merah: tagih dealer over limit / overdue</span>
        <span><i style={{ background: '#7c5cf0' }} />Surveyor: follow-up dealer At risk</span>
        <span><i style={{ background: '#ffcf33' }} />Warna bijih = sisa limit · emas = Key account · ! At risk · palang = Churn</span>
        <span className="mine-hint">Seret untuk memutar · klik dua kali untuk kembali</span>
      </div>
    </div>
  )
}
