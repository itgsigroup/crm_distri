import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type Items } from '../../api/client'
import type { BoardItem, KPI, ThreadView } from '../../api/types'
import { fmtNum, fmtRp, hhmm } from '../../lib/format'
import { STAGE_LABEL } from '../../lib/i18n/id'
import { useOrch } from '../../app/orch'
import { STAGES } from '../../app/cycle'
import { useCycleLatest, useQueue, useStockCritical, useStockPush, useWAStatus } from '../../app/queries'
import { STATE_LABEL, ordered, summarize, type CycleProgress } from '../control/progress'
import { AGENT_COLOR, AGENT_NAME, AGENT_OF, capsules, planStation, type AgentKey } from './station'
import { STATION_H, StationView, type StationTarget, type Work } from './StationView'

const KEY = 'dashboard.station'
// the engine the Orchestrator dock remembers (Sistem = GSI Orbit rules, no AI; MCP Claude), shared with it
const ENGINE_KEY = 'orch-engine'
type Engine = 'system' | 'mcp'
const savedEngine = (): Engine => {
  try {
    return localStorage.getItem(ENGINE_KEY) === 'system' ? 'system' : 'mcp'
  } catch {
    return 'mcp'
  }
}
const ENDED = ['done', 'partial', 'failed']
const read = () => {
  try {
    return localStorage.getItem(KEY) !== 'off'
  } catch {
    return true
  }
}

const ROLE: Record<AgentKey, string> = {
  order: 'Mengantar ke dealer yang jadwal order-nya dekat',
  collect: 'Menagih dealer over limit / overdue',
  credit: 'Memasang perisai di dealer dengan sisa limit tipis',
  followup: 'Spacewalk follow-up ke dealer At risk',
  stock: 'Merapikan stok menua di cargo bay',
  prospect: 'Memindai nomor baru di radar pasif',
}
const CREDIT: Record<string, string> = { good: 'Limit aman', warn: 'Limit tipis', bad: 'Over limit / overdue', neutral: 'Cash' }

/** Stasiun Orbit: the dashboard as a 3D space miniature; the AI core (Orchestrator) in the middle, dealers as planets. */
export function DashboardStation({ board, kpi, can }: { board: BoardItem[]; kpi: KPI | undefined; can: (screen: string) => boolean }) {
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
  // comets: new numbers, refreshed every minute — outside the 'chat' keys so a new message does not refetch the chat
  // list (the heaviest query) on the dashboard
  const { data: fresh = [] } = useQuery({
    queryKey: ['station', 'comets'],
    enabled: on && can('chat'),
    staleTime: 60_000,
    refetchInterval: 60_000,
    queryFn: () => api.get<Items<ThreadView>>('/chat/threads?tab=new').then((r) => r.items),
  })
  const { data: wa } = useWAStatus()
  const unread = can('chat') ? wa?.items.reduce((a, n) => a + (n.unread_count ?? 0), 0) ?? 0 : 0
  const { data: queue = [] } = useQueue()
  const waiting = queue.filter((q) => q.status === 'proposed')
  const { s: live, reanalyze, showPanel } = useOrch()
  const [engine, setEngineState] = useState(savedEngine)
  const setEngine = (e: Engine) => {
    setEngineState(e)
    try {
      localStorage.setItem(ENGINE_KEY, e)
    } catch {
      /* private window */
    }
  }
  const { data: latest } = useCycleLatest()
  const running = live.running || !!latest?.running
  const stage = live.running ? Math.min(live.shown, STAGES.length - 1) : Math.max(0, STAGES.indexOf(latest?.cycle?.stage ?? 'ingest'))
  const lastDone = latest?.last_done ?? null
  // the cycle the astronauts act out: any cycle that runs while the scene is open, kept after it ends until closed
  const [follow, setFollow] = useState<{ id: string; mine: boolean } | null>(null)
  const [seen, setSeen] = useState<string | null>(null)
  const runningId = live.running && live.cycleId ? live.cycleId : latest?.running ? latest.cycle?.id ?? null : null
  if (runningId && runningId !== seen) { // a new cycle started: follow it (state adjusted while rendering)
    setSeen(runningId)
    setFollow({ id: runningId, mine: live.mine })
  }
  const { data: prog } = useQuery({
    queryKey: ['cycle-progress', follow?.id],
    enabled: on && !!follow,
    queryFn: () => api.get<CycleProgress>(`/cycles/${follow!.id}/progress`),
    refetchInterval: (q) => (ENDED.includes(q.state.data?.status ?? '') ? false : 2000),
  })
  const ended = !!prog && ENDED.includes(prog.status)
  // a scheduled cycle's result closes by itself; one started here stays until it is closed
  useEffect(() => {
    if (!ended || !follow || follow.mine) return
    const t = setTimeout(() => setFollow(null), 45_000)
    return () => clearTimeout(t)
  }, [ended, follow])
  // the scene shows the result itself: no pop-up while it is on screen
  useEffect(() => (on ? showPanel() : undefined), [on, showPanel])
  const work = useMemo((): Work | null => {
    if (!follow || !prog || prog.cycle_id !== follow.id) return null
    const w: Work = {}
    for (const a of prog.agents) {
      const k = AGENT_OF[a.agent]
      if (!k) continue
      const state = ended && (a.state === 'waiting' || a.state === 'reading') ? (prog.status === 'failed' ? 'failed' : 'done') : a.state
      w[k] = { state, text: state === 'reading' ? a.covers ?? '' : a.summary ?? '' }
    }
    return w
  }, [follow, prog, ended])
  const startedAt = prog?.stages.find((x) => x.stage === 'ingest')?.started_at ?? null
  const fresh2 = startedAt ? waiting.filter((q) => Date.parse(q.created_at) >= Date.parse(startedAt)) : []
  const run = () => reanalyze('all', engine, engine === 'system' ? 'Sistem GSI Orbit' : 'Claude lewat MCP', true)

  // Perbesar: a taller scene; Layar penuh: the card fills the screen (CSS keeps it full window where the API is missing)
  const card = useRef<HTMLDivElement>(null)
  const [big, setBig] = useState(false)
  const [full, setFull] = useState(false)
  const toggleFull = () => {
    if (full) {
      if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
      setFull(false)
      return
    }
    setFull(true)
    card.current?.requestFullscreen?.().catch(() => {})
  }
  useEffect(() => {
    const f = () => { if (!document.fullscreenElement) setFull(false) }
    document.addEventListener('fullscreenchange', f)
    return () => document.removeEventListener('fullscreenchange', f)
  }, [])
  useEffect(() => {
    if (!full) return
    const k = (e: KeyboardEvent) => { if (e.key === 'Escape') setFull(false) }
    window.addEventListener('keydown', k)
    return () => window.removeEventListener('keydown', k)
  }, [full])
  const stageH = full ? undefined : big ? Math.max(STATION_H, Math.min(900, window.innerHeight - 180)) : STATION_H

  const stageEl = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const view = useRef<StationView | null>(null)
  const [width, setWidth] = useState(0)
  const [no3d, setNo3d] = useState(false)
  const [tip, setTip] = useState<{ t: StationTarget; x: number; y: number } | null>(null)
  const planets = Math.max(6, Math.min(16, Math.floor(width / 85)))
  const plan = useMemo(() => planStation(board, fresh, planets), [board, fresh, planets])
  const capKey = waiting.map((q) => q.agent).join('|')
  const navRef = useRef(nav)
  const canRef = useRef(can)
  useEffect(() => { navRef.current = nav; canRef.current = can })

  useEffect(() => {
    const el = stageEl.current
    if (!el || !on) return
    const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => setWidth(el.clientWidth))
    ro?.observe(el)
    setWidth(el.clientWidth)
    return () => ro?.disconnect()
  }, [on])

  useEffect(() => {
    if (!on || !stageEl.current || !canvas.current) return
    const v = new StationView({
      stage: stageEl.current,
      canvas: canvas.current,
      onPick: (t) => {
        const go = navRef.current
        const ok = canRef.current
        if (t.kind === 'planet') go('/dealer/' + t.planet.id)
        else if (t.kind === 'core' && ok('orch')) go('/orchestrator')
        else if ((t.kind === 'dock' || t.kind === 'commander') && ok('today')) go('/')
        else if (t.kind === 'cargo' && ok('stock')) go('/stok')
        else if (t.kind === 'comms' && ok('chat')) go('/chat')
        else if (t.kind === 'comet' && ok('chat')) go('/chat/' + t.comet.id)
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
  useEffect(() => { view.current?.setDock(capsules(capKey ? capKey.split('|') : [])) }, [capKey, on])
  useEffect(() => { view.current?.setStock(push.length, critical.length > 0) }, [push.length, critical.length, on])
  useEffect(() => { view.current?.setUnread(unread) }, [unread, on])
  useEffect(() => { view.current?.setCycle({ running, stage, hasCycle: !!lastDone }) }, [running, stage, lastDone, on])
  useEffect(() => { view.current?.setWork(work) }, [work, on])

  const active = board.filter((d) => d.metrics.segment !== 'Prospek')
  const omzet = active.reduce((a, d) => a + d.metrics.omzet_bln, 0)
  const score = kpi?.on_schedule_pct ?? 0
  const target = kpi?.targets.on_schedule_pct ?? 80
  const level = Math.max(1, Math.min(10, Math.round(score / 10)))

  if (!on) {
    return (
      <div className="mine-off">
        <button type="button" className="btn ghost" onClick={() => setOn(true)}>Tampilkan Stasiun Orbit</button>
      </div>
    )
  }
  const hint = (s: string) => <div className="r" style={{ opacity: 0.7, marginTop: 4 }}><span>{s}</span></div>
  const tipBody = (t: StationTarget) => {
    switch (t.kind) {
      case 'planet': {
        const p = t.planet
        return <>
          <b>{p.name}</b>
          <div className="r"><span>{p.look === 'key' ? 'Key account' : p.look === 'risk' ? 'At risk' : p.look === 'closed' ? 'Churn' : 'Aktif'}</span><span>{fmtRp(p.omzet)}/bln</span></div>
          <div className="r"><span>Jadwal order</span><span>{p.dueIn == null ? '—' : p.dueIn >= 0 ? `${p.dueIn} hari lagi` : `lewat ${-p.dueIn} hari`}</span></div>
          <div className="r"><span>Kredit</span><span>{CREDIT[p.tone]}</span></div>
          {hint('Klik untuk buka dealer')}
        </>
      }
      case 'core':
        return <>
          <b>Inti AI · Orchestrator</b>
          <div className="r"><span>Status</span><span>{running ? `${STAGE_LABEL[STAGES[stage]]}…` : 'Siaga'}</span></div>
          <div className="r"><span>Analisis terakhir</span><span>{lastDone ? `#${fmtNum(lastDone.number ?? 0)} · ${hhmm(lastDone.started_at)}` : 'belum ada'}</span></div>
          {latest && <div className="r"><span>Berikutnya</span><span>{hhmm(latest.next_at)}</span></div>}
          {can('orch') && hint('Klik untuk buka Orchestrator')}
        </>
      case 'dock':
      case 'commander':
        return <>
          <b>{t.kind === 'commander' ? 'Komandan · manusia yang memutuskan' : 'Dek Keputusan'}</b>
          <div className="r"><span>Usulan menunggu</span><span>{fmtNum(waiting.length)}</span></div>
          <div className="r"><span>AI mengusulkan, manusia memutuskan</span></div>
          {can('today') && hint('Klik untuk buka Pusat kendali')}
        </>
      case 'cargo':
        return <><b>Cargo bay · AI Stok</b><div className="r"><span>Stok menua</span><span>{fmtNum(push.length)} item</span></div><div className="r"><span>Stok kritis</span><span>{fmtNum(critical.length)} SKU</span></div>{can('stock') && hint('Klik untuk Push stok')}</>
      case 'comms':
        return <><b>Antena komunikasi</b><div className="r"><span>Chat belum dibaca</span><span>{fmtNum(unread)}</span></div>{can('chat') && hint('Klik untuk buka Chat')}</>
      case 'comet':
        return <>
          <b>Nomor baru · {t.comet.name}</b>
          <div className="r"><span>Menghubungi lebih dulu</span><span>{t.comet.at ? hhmm(t.comet.at) : '—'}</span></div>
          <div className="r"><span>AI Prospek memindai identitasnya</span></div>
          {can('chat') && hint('Klik untuk buka chat')}
        </>
      case 'agent':
        return <>
          <b style={{ color: t.agent === 'prospect' ? undefined : AGENT_COLOR[t.agent] }}>{AGENT_NAME[t.agent]}</b>
          <div className="r"><span>{ROLE[t.agent]}</span></div>
          <div className="r"><span>Sekarang: {t.status || 'siaga'}</span></div>
        </>
    }
  }
  return (
    <div ref={card} className={`card mine-card station-card${full ? ' station-full' : ''}`}>
      <div className="mine-head">
        <div>
          <span className="db-eyebrow">GSI ORBIT · STASIUN ORBIT</span>
          <h2>Inti AI dan dealer di orbitnya</h2>
        </div>
        <div className="mine-hud">
          <span className="hud-chip gold" title="Omzet dealer per bulan"><i>●</i>{fmtRp(omzet)}<small>/bln</small></span>
          <span className="hud-chip blue" title="Dealer dengan jadwal order dekat — AI Order mengantar"><i>■</i>{fmtNum(plan.tasks.order.length)} order</span>
          <span className="hud-chip red" title="Dealer over limit / overdue — AI Penagihan menagih"><i>◆</i>{fmtNum(plan.tasks.collect.length)} tagihan</span>
          <span className="hud-chip purple" title="Dealer dengan sisa limit tipis — AI Kredit memasang perisai"><i>⬡</i>{fmtNum(plan.tasks.credit.length)} kredit</span>
          <span className="hud-chip amber" title="Dealer At risk — AI Follow-up spacewalk"><i>▲</i>{fmtNum(plan.tasks.followup.length)} follow-up</span>
          {can('chat') && <span className="hud-chip cyan" title="Nomor baru yang menghubungi lebih dulu — komet yang dipindai AI Prospek"><i>☄</i>{fmtNum(plan.comets.length)} nomor baru</span>}
          <span className="hud-level" title={`Order tepat jadwal ${score}% · target ${target}%`}>Level {level}<b><em style={{ width: `${Math.min(100, (score / Math.max(1, target)) * 100)}%` }} /></b><small>{score}% tepat jadwal</small></span>
          <span className="station-engine" role="radiogroup" aria-label="Dianalisis oleh">
            <button type="button" role="radio" aria-checked={engine === 'system'} className={engine === 'system' ? 'on' : ''} disabled={running} onClick={() => setEngine('system')} title="Aturan agen dan rumus GSI Orbit di server — langsung jalan, tanpa biaya AI">Sistem</button>
            <button type="button" role="radio" aria-checked={engine === 'mcp'} className={engine === 'mcp' ? 'on' : ''} disabled={running} onClick={() => setEngine('mcp')} title="Claude menganalisa lewat MCP">MCP Claude</button>
          </span>
          <button type="button" className="btn primary station-run" disabled={running} onClick={run} title="Jalankan Orchestrator: keenam agen menganalisa, hasilnya jadi usulan di Dek Keputusan">
            {running ? 'AI agent bekerja…' : '🚀 AI Agent bekerja'}
          </button>
          {!full && <button type="button" className="btn quiet mine-hide" onClick={() => setOn(false)} title="Sembunyikan stasiun">Sembunyikan</button>}
        </div>
      </div>
      <div className="mine-stage station-stage" ref={stageEl} style={stageH ? { height: stageH } : undefined}>
        <canvas ref={canvas} aria-label="Stasiun Orbit: Inti AI (Orchestrator) di tengah, dealer sebagai planet, enam agen AI sebagai astronot, komandan manusia di Dek Keputusan, dan komet untuk nomor baru" role="img" />
        <div className="station-stages" aria-label="Tahap siklus Orchestrator">
          {STAGES.map((k, i) => (
            <span key={k} className={running ? (i < stage ? 'done' : i === stage ? 'run' : '') : lastDone ? 'done' : ''}>{STAGE_LABEL[k]}</span>
          ))}
        </div>
        <div className="station-dock" title="Usulan menunggu keputusan manusia">
          <b>{fmtNum(waiting.length)}</b><span>usulan menunggu<br />keputusan Anda</span>
        </div>
        <div className="station-zoom">
          <button type="button" aria-label="Perbesar" onClick={() => view.current?.zoomBy(0.8)}>+</button>
          <button type="button" aria-label="Perkecil" onClick={() => view.current?.zoomBy(1.25)}>−</button>
          <button type="button" aria-label="Kembali ke tampilan awal" title="Kembali ke tampilan awal" onClick={() => view.current?.resetView()}>⟲</button>
          {!full && <button type="button" aria-label={big ? 'Perkecil adegan' : 'Perbesar adegan'} title={big ? 'Perkecil adegan' : 'Perbesar adegan'} className={big ? 'on' : ''} onClick={() => setBig(!big)}>{big ? '⇲' : '⇱'}</button>}
          <button type="button" aria-label={full ? 'Keluar layar penuh' : 'Layar penuh'} title={full ? 'Keluar layar penuh (Esc)' : 'Layar penuh'} className={full ? 'on' : ''} onClick={toggleFull}>⛶</button>
        </div>
        {follow && (
          <div className="station-work" aria-live="polite">
            <div className="sw-h">
              <b>{!prog ? 'Menyiapkan AI agent…' : ended ? (prog.status === 'failed' ? 'Analisis gagal' : 'Hasil kerja AI agent') : 'AI agent bekerja'}</b>
              <span>{prog?.number ? `Siklus #${fmtNum(prog.number)}` : ''}{prog?.via ? ` · ${prog.via === 'system' ? 'Sistem' : prog.via === 'mcp' ? 'MCP Claude' : prog.via.toUpperCase()}` : ''}</span>
              <button type="button" aria-label="Tutup hasil" onClick={() => setFollow(null)}>×</button>
            </div>
            {prog && !ended && <p className="sw-now">{summarize(prog).now || `${STAGE_LABEL[prog.stage ?? 'ingest'] ?? 'Ingest'}…`}</p>}
            {prog && prog.agents.length > 0 && (
              <ul className="sw-agents">
                {ordered(prog.agents).map((a) => {
                  const k = AGENT_OF[a.agent]
                  const st = work && k ? work[k]?.state ?? a.state : a.state
                  return (
                    <li key={a.agent} className={st}>
                      <i style={{ background: k ? AGENT_COLOR[k] : '#8a94a6' }} />
                      <b>{a.agent}</b>
                      <span>{STATE_LABEL[st]}{st === 'reading' && a.covers ? ` · ${a.covers}` : a.summary ? ` · ${a.summary}` : ''}</span>
                    </li>
                  )
                })}
              </ul>
            )}
            {prog && !prog.agents.length && !ended && <p className="sw-now">Semua astronot berkumpul di Inti AI…</p>}
            {ended && prog?.status !== 'failed' && (
              <div className="sw-res">
                <b>{fresh2.length ? `${fmtNum(fresh2.length)} usulan baru menunggu keputusan Anda` : `Tidak ada usulan baru · ${fmtNum(waiting.length)} usulan menunggu`}</b>
                {fresh2.slice(0, 4).map((q) => {
                  const k = AGENT_OF[q.agent]
                  return <button type="button" key={q.id} className="sw-prop" onClick={() => can('today') && nav('/')}><i style={{ background: k ? AGENT_COLOR[k] : '#8a94a6' }} /><span>{q.title}</span></button>
                })}
                {can('today') && <button type="button" className="btn primary sw-go" onClick={() => nav('/')}>Putuskan di Pusat kendali</button>}
              </div>
            )}
          </div>
        )}
        {no3d && <p className="mine-no3d station-no3d">Tampilan 3D tidak didukung peramban ini (WebGL tidak tersedia).</p>}
        {!no3d && !plan.planets.length && <p className="mine-no3d station-no3d">Belum ada dealer di orbit — data dealer sedang dimuat atau kosong.</p>}
        {tip && (
          <div className="tip show mine-tip" style={{ left: Math.min(tip.x + 14, width - 250), top: Math.max(6, tip.y - 80) }}>
            {tipBody(tip.t)}
          </div>
        )}
      </div>
      <div className="mine-legend">
        <span><i style={{ background: 'radial-gradient(#e6fbff,#36b8ff)' }} />Inti AI = Orchestrator: semua analisa lahir di sini; 6 busur menyala per tahap siklus</span>
        <span><i style={{ background: '#4fd6ff' }} />Planet = dealer · ukuran = omzet · makin dekat inti = jadwal order makin dekat · cincin emas = Key account · cincin merah = At risk · es = Churn</span>
        {(['order', 'collect', 'credit', 'followup', 'stock', 'prospect'] as AgentKey[]).map((k) => (
          <span key={k}><i style={{ background: AGENT_COLOR[k], boxShadow: k === 'prospect' ? 'inset 0 0 0 1px #8a94a6' : undefined }} />{AGENT_NAME[k]}: {ROLE[k].toLowerCase()}</span>
        ))}
        <span><i style={{ background: '#f2c14e' }} />Komandan (helm emas) = manusia: kapsul usulan hanya terbang ke dealer setelah disetujui</span>
        <span><i style={{ background: '#9fd8ff' }} />Komet = nomor baru yang menghubungi lebih dulu — sistem tidak mencari leads</span>
        <span className="mine-hint">Seret untuk memutar · pinch / Ctrl+scroll untuk zoom · klik dua kali untuk kembali</span>
      </div>
    </div>
  )
}
