import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import type { BoardItem, KPI } from '../../api/types'
import { fmtNum, fmtRp } from '../../lib/format'
import { useStockCritical, useStockPush, useThreads } from '../../app/queries'
import { planTown } from './town'
import { TOWN_H, TownView, type TownTarget } from './TownView'

const KEY = 'dashboard.town'
const read = () => {
  try {
    return localStorage.getItem(KEY) !== 'off'
  } catch {
    return true
  }
}

/** Kota Distribusi: the dashboard as a little game town, driven by the dealer board and the KPIs. */
export function DashboardTown({ board, kpi, can }: { board: BoardItem[]; kpi: KPI | undefined; can: (screen: string) => boolean }) {
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
  const view = useRef<TownView | null>(null)
  const [width, setWidth] = useState(0)
  const [tip, setTip] = useState<{ t: TownTarget; x: number; y: number } | null>(null)
  // shops that fit: two streets, each shop needs ~100 px
  const perRow = Math.max(2, Math.min(8, Math.floor((width - 210) / 104)))
  const plan = useMemo(() => planTown(board, perRow * 2), [board, perRow])
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
    const v = new TownView({
      stage: stage.current,
      canvas: canvas.current,
      plan: { shops: [], tasks: { deliver: [], collect: [], visit: [] } },
      stock: { aging: 0, critical: false },
      unread: 0,
      onPick: (t) => {
        if (t.kind === 'shop') navRef.current('/dealer/' + t.shop.id)
        else if (t.kind === 'warehouse' && canRef.current('stock')) navRef.current('/stok')
        else if (t.kind === 'office' && canRef.current('chat')) navRef.current('/chat')
      },
      onHover: (t, x, y) => setTip(t ? { t, x, y } : null),
    })
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
      <div className="town-off">
        <button type="button" className="btn ghost" onClick={() => setOn(true)}>Tampilkan Kota Distribusi</button>
      </div>
    )
  }
  return (
    <div className="card town-card">
      <div className="town-head">
        <div>
          <span className="db-eyebrow">GSI ORBIT · KOTA DISTRIBUSI</span>
          <h2>Hari ini di kota dealer</h2>
        </div>
        <div className="town-hud">
          <span className="hud-chip gold" title="Omzet dealer per bulan"><i>●</i>{fmtRp(omzet)}<small>/bln</small></span>
          <span className="hud-chip blue" title="Dealer dengan jadwal order ≤ 14 hari — truk mengantar"><i>■</i>{fmtNum(plan.tasks.deliver.length)} antaran</span>
          <span className="hud-chip red" title="Dealer over limit / overdue — kurir menagih"><i>◆</i>{fmtNum(plan.tasks.collect.length)} tagihan</span>
          <span className="hud-chip purple" title="Dealer At risk — sales follow-up"><i>▲</i>{fmtNum(plan.tasks.visit.length)} follow-up</span>
          <span className="hud-level" title={`Order tepat jadwal ${score}% · target ${target}%`}>Level {level}<b><em style={{ width: `${Math.min(100, (score / Math.max(1, target)) * 100)}%` }} /></b><small>{score}% tepat jadwal</small></span>
          <button type="button" className="btn quiet town-hide" onClick={() => setOn(false)} title="Sembunyikan kota">Sembunyikan</button>
        </div>
      </div>
      <div className="town-stage" ref={stage} style={{ height: TOWN_H }}>
        <canvas ref={canvas} aria-label="Kota Distribusi: dealer, truk antar paket, kurir penagih, dan sales" role="img" />
        {tip && (
          <div className="tip show town-tip" style={{ left: Math.min(tip.x + 14, width - 230), top: Math.max(6, tip.y - 70) }}>
            {tip.t.kind === 'shop' ? (
              <>
                <b>{tip.t.shop.name}</b>
                <div className="r"><span>{tip.t.shop.look === 'key' ? 'Key account' : tip.t.shop.look === 'risk' ? 'At risk' : tip.t.shop.look === 'closed' ? 'Churn' : 'Aktif'}</span><span>{fmtRp(tip.t.shop.omzet)}/bln</span></div>
                <div className="r"><span>Jadwal order</span><span>{tip.t.shop.dueIn == null ? '—' : tip.t.shop.dueIn >= 0 ? `${tip.t.shop.dueIn} hari lagi` : `lewat ${-tip.t.shop.dueIn} hari`}</span></div>
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
      <div className="town-legend">
        <span><i style={{ background: '#3b6ef5' }} />Truk: antar ke dealer yang jadwal order ≤ 14 hari</span>
        <span><i style={{ background: '#e5534b' }} />Motor: tagih dealer over limit / overdue</span>
        <span><i style={{ background: '#7c5cf0' }} />Sales jalan: follow-up dealer At risk</span>
        <span><i style={{ background: '#ffcf33' }} />★ Key account · ! At risk · papan silang = Churn · warna atap = sisa limit</span>
      </div>
    </div>
  )
}
