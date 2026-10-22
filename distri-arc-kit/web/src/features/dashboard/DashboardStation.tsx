import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import type { KPI } from '../../api/types'
import { fmtNum, fmtRp, hhmm } from '../../lib/format'
import { STAGE_LABEL } from '../../lib/i18n/id'
import { useOrch } from '../../app/orch'
import { useEngine } from '../../app/EngineChoice'
import { STAGES } from '../../app/cycle'
import { summarize } from '../control/progress'
import { CREW, CREW_BY_AGENT, CREW_OF, CREW_STATE_LABEL, type CrewKey } from './station'
import { STATION_H, StationView, type StationTarget } from './StationView'
import type { Crew } from './useCrew'

const KEY = 'dashboard.station'
const read = () => {
  try {
    return localStorage.getItem(KEY) !== 'off'
  } catch {
    return true
  }
}

/** Stasiun Orbit: the dashboard as a 3D space station; the AI core (Orchestrator) in the middle, ten rooms around it
 * with an astronaut each. No customer data is drawn — only what each astronaut does and counts. */
export function DashboardStation({ crew, kpi, can }: { crew: Crew; kpi: KPI | undefined; can: (screen: string) => boolean }) {
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
  const { reanalyze, showPanel } = useOrch()
  // MCP Claude or ChatGPT (Sam 22 Oct, ADR 0059), shared with the Orchestrator dock
  const eng = useEngine()
  const { facts, status, running, stage, lastDone, latest, follow, setFollow, prog, ended, fresh, waiting } = crew
  // the scene shows the result itself: no pop-up while it is on screen
  useEffect(() => (on ? showPanel() : undefined), [on, showPanel])
  const run = () => reanalyze('all', eng.engine, eng.engine === 'chatgpt' ? 'ChatGPT' : 'Claude lewat MCP', true)

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
  const capKey = waiting.map((q) => CREW_BY_AGENT[q.agent] ?? 'ready').join('|')
  const statusKey = JSON.stringify(status)
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
        if (t.kind === 'core') { if (ok('orch')) go('/orchestrator') }
        else if (ok(CREW_OF[t.key].screen)) go(CREW_OF[t.key].to)
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

  useEffect(() => { view.current?.setCrew(JSON.parse(statusKey)) }, [statusKey, on])
  useEffect(() => { view.current?.setDock(capKey ? (capKey.split('|') as CrewKey[]) : []) }, [capKey, on])
  useEffect(() => { view.current?.setCycle({ running, stage, hasCycle: !!lastDone }) }, [running, stage, lastDone, on])

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
    if (t.kind === 'core') {
      return <>
        <b>Inti AI · Orchestrator</b>
        <div className="r"><span>Status</span><span>{running ? `${STAGE_LABEL[STAGES[stage]]}…` : 'Siaga'}</span></div>
        <div className="r"><span>Analisis terakhir</span><span>{lastDone ? `#${fmtNum(lastDone.number ?? 0)} · ${hhmm(lastDone.started_at)}` : 'belum ada'}</span></div>
        {latest && <div className="r"><span>Berikutnya</span><span>{hhmm(latest.next_at)}</span></div>}
        {can('orch') && hint('Klik untuk buka Orchestrator')}
      </>
    }
    const m = CREW_OF[t.key]
    const w = status[t.key]
    return <>
      <b style={{ color: m.key === 'prospect' ? undefined : m.color }}>Astronot {m.n} · {m.title}</b>
      <div className="r"><span>{m.does}</span></div>
      <div className="r"><span>Sekarang</span><span>{CREW_STATE_LABEL[w.state]}</span></div>
      <div className="r"><span>{w.text}</span></div>
      {can(m.screen) && hint('Klik untuk buka detail')}
    </>
  }
  return (
    <div ref={card} className={`card mine-card station-card${full ? ' station-full' : ''}`}>
      <div className="mine-head">
        <div>
          <span className="db-eyebrow">GSI ORBIT · STASIUN ORBIT</span>
          <h2>Kru AI bekerja di stasiun</h2>
        </div>
        <div className="mine-hud">
          {facts.omzetMonth != null && <span className="hud-chip gold" title="Omzet aktual bulan ini — hasil Analisa sales"><i>●</i>{fmtRp(facts.omzetMonth)}<small>bln ini</small></span>}
          <span className="hud-chip blue" title="Dealer dengan jadwal order dekat — Eksekusi order"><i>■</i>{fmtNum(facts.due ?? 0)} order</span>
          <span className="hud-chip red" title="Dealer over limit / overdue — Cek penagihan"><i>◆</i>{fmtNum(facts.collect ?? 0)} tagihan</span>
          <span className="hud-chip purple" title="Dealer dengan sisa limit tipis — Cek kredit"><i>⬡</i>{fmtNum(facts.tight ?? 0)} kredit</span>
          <span className="hud-chip amber" title="Dealer At risk / Churn — Follow-up"><i>▲</i>{fmtNum(facts.atRisk ?? 0)} follow-up</span>
          {facts.newNumbers != null && <span className="hud-chip cyan" title="Nomor baru yang menghubungi lebih dulu — Prospek"><i>☄</i>{fmtNum(facts.newNumbers)} nomor baru</span>}
          <span className="hud-level" title={`Order tepat jadwal ${score}% · target ${target}%`}>Level {level}<b><em style={{ width: `${Math.min(100, (score / Math.max(1, target)) * 100)}%` }} /></b><small>{score}% tepat jadwal</small></span>
          <span className="station-engine" role="radiogroup" aria-label="Dianalisis oleh">
            <button type="button" role="radio" aria-checked={eng.engine === 'mcp'} className={eng.engine === 'mcp' ? 'on' : ''} disabled={running} onClick={() => eng.choose('mcp')} title="Claude menganalisa lewat MCP (routine claude.ai)">MCP Claude</button>
            <button type="button" role="radio" aria-checked={eng.engine === 'chatgpt'} className={eng.engine === 'chatgpt' ? 'on' : ''} disabled={running} onClick={() => eng.choose('chatgpt')} title="ChatGPT API menganalisa di server — langsung jalan">ChatGPT</button>
          </span>
          <button type="button" className="btn primary station-run" disabled={running} onClick={run} title="Jalankan Orchestrator: kesepuluh astronot bekerja berurutan sampai data siap, hasilnya jadi usulan untuk diputuskan">
            {running ? 'AI agent bekerja…' : '🚀 AI Agent bekerja'}
          </button>
          {!full && <button type="button" className="btn quiet mine-hide" onClick={() => setOn(false)} title="Sembunyikan stasiun">Sembunyikan</button>}
        </div>
      </div>
      <div className="mine-stage station-stage" ref={stageEl} style={stageH ? { height: stageH } : undefined}>
        <canvas ref={canvas} aria-label="Stasiun Orbit: Inti AI (Orchestrator) di tengah dan sepuluh ruangan kru AI — analisa sales, WhatsApp, dealer, eksekusi order, follow-up, cek stok, kredit, penagihan, prospek, sampai data siap" role="img" />
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
              <b>{!prog ? 'Menyiapkan kru AI…' : ended ? (prog.status === 'failed' ? 'Analisis gagal' : 'Hasil kerja kru AI') : 'Kru AI bekerja'}</b>
              <span>{prog?.number ? `Siklus #${fmtNum(prog.number)}` : ''}{prog?.via ? ` · ${prog.via === 'system' ? 'Sistem' : prog.via === 'mcp' ? 'MCP Claude' : prog.via.toUpperCase()}` : ''}</span>
              <button type="button" aria-label="Tutup hasil" onClick={() => setFollow(null)}>×</button>
            </div>
            {prog && !ended && <p className="sw-now">{summarize(prog).now || `${STAGE_LABEL[prog.stage ?? 'ingest'] ?? 'Ingest'}…`}</p>}
            <ul className="sw-agents">
              {CREW.map((m) => {
                const w = status[m.key]
                const st = w.state === 'idle' ? 'waiting' : w.state
                return (
                  <li key={m.key} className={st}>
                    <i style={{ background: m.color }} />
                    <b>{m.n}. {m.title}</b>
                    <span>{CREW_STATE_LABEL[w.state]}{w.text && !w.text.startsWith(CREW_STATE_LABEL[w.state]) ? ` · ${w.text}` : ''}</span>
                  </li>
                )
              })}
            </ul>
            {ended && prog?.status !== 'failed' && (
              <div className="sw-res">
                <b>{fresh ? `${fmtNum(fresh)} usulan baru menunggu keputusan Anda` : `Tidak ada usulan baru · ${fmtNum(waiting.length)} usulan menunggu`}</b>
                {can('today') && <button type="button" className="btn primary sw-go" onClick={() => nav('/')}>Putuskan di Pusat kendali</button>}
              </div>
            )}
          </div>
        )}
        {no3d && <p className="mine-no3d station-no3d">Tampilan 3D tidak didukung peramban ini (WebGL tidak tersedia).</p>}
        {tip && (
          <div className="tip show mine-tip" style={{ left: Math.min(tip.x + 14, width - 270), top: Math.max(6, tip.y - 80) }}>
            {tipBody(tip.t)}
          </div>
        )}
      </div>
      <div className="mine-legend">
        <span><i style={{ background: 'radial-gradient(#e6fbff,#36b8ff)' }} />Inti AI = Orchestrator: data dari setiap ruangan mengalir ke sini; 6 busur menyala per tahap siklus</span>
        {CREW.map((m) => (
          <span key={m.key}><i style={{ background: m.color, boxShadow: m.key === 'prospect' ? 'inset 0 0 0 1px #8a94a6' : undefined }} />{m.n}. {m.title}: {m.does.toLowerCase()}</span>
        ))}
        <span className="mine-hint">Seret untuk memutar · pinch / Ctrl+scroll untuk zoom · klik ruangan untuk detail · klik dua kali untuk kembali</span>
      </div>
    </div>
  )
}
