import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useLocation } from 'react-router'
import { Icon } from '../components/Icon'
import { PIPELINE_STAGES, type ScreenKey } from '../lib/i18n/id'
import { chipState, progress } from './cycle'
import { scopeLabel, useOrch, useOrchStatus } from './orch'
import { useCommand } from './command'
import { EngineChoice, useEngine } from './EngineChoice'
import { AnalysisCompact } from './AnalysisSheet'

/** Pipeline chips (mockup pipeHtml compact): done / run per stage. */
export function PipeChips() {
  const { s } = useOrch()
  const st = useOrchStatus()
  return (
    <>
      {PIPELINE_STAGES.map((n, i) => (
        <span key={n} style={{ display: 'contents' }}>
          {i > 0 && <span className="pl" />}
          <span className={`ps ${chipState(s, i, st.hasCycle)}`} title={n}><i />{n}</span>
        </span>
      ))}
    </>
  )
}

/** Scope of "Analisis ulang layar ini" for a screen (04-orchestrator › Scope semantics). */
export function screenScope(screen: ScreenKey, path: string): string {
  switch (screen) {
    case 'orbit':
      return 'screen:orbit'
    case 'kuad':
      return 'screen:segmen'
    case 'net':
      return 'screen:relasi'
    case 'stock':
      return 'screen:stock'
    case 'ar':
      return 'screen:credit'
    case 'stockcheck':
      return 'screen:stock'
    case 'dealer': {
      const id = path.split('/')[2]
      return id ? 'dealer:' + id : 'screen:dealer'
    }
    case 'chat': { // the open conversation (in place of "Analisa lanjut", 19 Oct), else the chats
      const id = path.split('/')[2]
      return id ? 'thread:' + id : 'screen:chat'
    }
  }
  return 'all'
}

/** The agents a scope runs and the page readings that come with them (domain.Scope.Agents, ScreenReadings). */
export function scopeAgents(scope: string): string[] {
  const [kind, id] = scope.split(':')
  let agents: string[] | null = null
  if (kind === 'screen') {
    agents = ({ orbit: ['AI Follow-up', 'AI Kredit'], segmen: ['AI Follow-up', 'AI Kredit'], relasi: ['AI Follow-up', 'AI Kredit'], stock: ['AI Stok'],
      credit: ['AI Kredit', 'AI Penagihan'], chat: ['AI Order', 'AI Prospek'], wa: [] } as Record<string, string[]>)[id] ?? null
  } else if (kind === 'thread') agents = ['AI Order', 'AI Prospek', 'Analisa WhatsApp']
  else if (kind === 'agent') agents = [id]
  if (agents === null) return ['6 agen', 'Analisa Orbit', 'Analisa Segmen', 'Analisa Relasi', 'Analisa Stok', 'Analisa Kas', 'Analisa WhatsApp']
  const out = [...agents]
  // AI Follow-up's page readings: only the page's own on Orbit / Segmen / Peta relasi (ScreenReadingsIn, ADR 0054)
  const page = ({ orbit: 'Analisa Orbit', segmen: 'Analisa Segmen', relasi: 'Analisa Relasi' } as Record<string, string>)[kind === 'screen' ? id : '']
  if (agents.includes('AI Follow-up')) out.push(...(page ? [page] : ['Analisa Orbit', 'Analisa Segmen', 'Analisa Relasi']))
  if (agents.includes('AI Stok')) out.push('Analisa Stok')
  if (agents.includes('AI Kredit') || agents.includes('AI Penagihan')) out.push('Analisa Kas')
  return out
}

const MINI_KEY = 'orch-dock-mini'
function savedMini(): boolean {
  try { return localStorage.getItem(MINI_KEY) === '1' } catch { return false }
}

// Floating Orchestrator dock, fixed at the bottom right of the screen on every page except Dashboard and Orchestrator (it can shrink to a round icon; on Chat it analyses the open conversation): re-analyse this page (or
// everything) with GSI Orbit's own rules (Sistem, ADR 0046) or Claude through MCP — no API key, no ChatGPT (19 Oct); the page's
// data refreshes when the cycle is done.
// With `head` (Chat, 20 Oct) it is a button in the conversation's header next to the name, and the dock opens under it.
export function Dock({ screen, extra, links, head }: { screen: ScreenKey; extra?: ReactNode; links?: ReactNode; head?: boolean }) {
  const [open, setOpen] = useState(false)
  const headBtn = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const [at, setAt] = useState<{ top: number; right: number } | null>(null)
  // on Chat it starts as the round icon above the reply box (19 Oct): the open dock must not cover the composer
  const [mini, setMiniState] = useState(() => screen === 'chat' || savedMini())
  const setMini = (m: boolean) => { setMiniState(m); if (m) setOpen(false); try { localStorage.setItem(MINI_KEY, m ? '1' : '0') } catch { /* private mode */ } }
  const { s, reanalyze, queued } = useOrch()
  const st = useOrchStatus()
  const { pathname } = useLocation()
  // MCP Claude or ChatGPT (Sam 22 Oct, ADR 0059), with the model and effort — shared with the Orchestrator page
  const eng = useEngine()
  // "Tanya atau perintahkan Orchestrator" lives here (19 Oct, out of the top bar); ⌘K opens the dock on it
  const ask = useCommand()
  const [q, setQ] = useState('')
  const askRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        if (head) place()
        setMiniState(false)
        setOpen(true)
        setTimeout(() => askRef.current?.focus(), 0) // after the dock opened
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [head])
  // the header dock closes on Esc or a click outside it
  useEffect(() => {
    if (!head || !open) return
    const onDown = (e: MouseEvent) => {
      const n = e.target as Node
      if (!panel.current?.contains(n) && !headBtn.current?.contains(n)) setOpen(false)
    }
    const onEsc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.preventDefault(); setOpen(false) } }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onEsc)
    return () => { document.removeEventListener('mousedown', onDown); document.removeEventListener('keydown', onEsc) }
  }, [head, open])
  function place() {
    const r = headBtn.current?.getBoundingClientRect()
    if (r) setAt({ top: r.bottom + 8, right: Math.max(12, window.innerWidth - r.right) })
  }
  const submitAsk = (e: FormEvent) => {
    e.preventDefault()
    const v = q.trim()
    if (!v) return
    setQ('')
    setOpen(false) // the answer shows on its page (Cek stok's table, a dealer) — the dock must not cover it
    ask(v)
  }
  const scope = screenScope(screen, pathname)
  // one pop-up (Sam 22 Oct): "Analisis ulang" opens no second pop-up — the dock keeps its usual look and shows the
  // analysis compactly under the stages (stage, time, agents, Claude, Batalkan, then berhasil / gagal); closing it
  // lets the analysis run on, the result pops up when done
  const [asked, setAsked] = useState(false)
  const run = (sc: string) => {
    setAsked(true)
    reanalyze(sc, eng.engine, eng.engine === 'chatgpt' ? 'ChatGPT' : 'Claude lewat MCP', true)
  }
  const live = open && asked && queued && (s.running ? s.cycleId === queued.id : s.last?.cycleId === queued.id) ? queued : null
  const status = st.running ? `menganalisis ${st.scope} · ${st.stage}` : st.run ? `siap · siklus #${st.run.toLocaleString('id-ID')} · ${st.last}` : 'siap · belum ada siklus'
  const inner = (
    <div className="dk-body">
      <form className="dk-ask" autoComplete="off" onSubmit={submitAsk}>
        <Icon name="spark" />
        <input ref={askRef} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Tanya atau perintahkan Orchestrator…" title="mis. “dealer mana yang lewat jadwal?” atau “analisis ulang stok”" aria-label="Tanya atau perintahkan Orchestrator" />
        <button type="submit" className="dk-ask-go" aria-label="Kirim" disabled={!q.trim()}><Icon name="send" /></button>
      </form>
      <div className="dk-pipe"><PipeChips /></div>
      <div className="dk-track"><i style={{ width: `${progress(s)}%` }} /></div>
      {live && <AnalysisCompact cycle={live} />}
      {extra && <div className="dk-extra">{extra}</div>}
      <div className="dk-scope">
        <span>Layar ini: <b>{scopeLabel(scope)}</b></span>
        <small>{scopeAgents(scope).join(' · ')}</small>
      </div>
      <EngineChoice e={eng} disabled={st.running} />
      <div className="dk-acts">
        <button className="btn primary" disabled={st.running} onClick={() => run(scope)}><Icon name="refresh" />{scope === 'all' ? 'Analisis ulang' : 'Analisis ulang layar ini'}</button>
        {scope !== 'all' && <button className="btn ghost" disabled={st.running} onClick={() => run('all')}>Semua</button>}
        {links}
      </div>
    </div>
  )
  if (head) {
    return (
      <>
        <button ref={headBtn} type="button" className={`ibc-orch${open ? ' on' : ''}${st.running ? ' running' : ''}`} onClick={() => { place(); setOpen(!open) }}
          aria-label="Orchestrator" aria-expanded={open} title={`Orchestrator · ${status}`}>
          <Icon name="spark" /><span>Orchestrator</span>
        </button>
        {open && at && createPortal(
          <div ref={panel} className={`dock dock-head open ${st.running ? 'running' : ''}`} style={{ top: at.top, right: at.right }}>{inner}</div>,
          document.body,
        )}
      </>
    )
  }
  // rendered on <body>: inside a page (.screen animates a transform) position: fixed would scroll with the content
  if (mini) {
    return createPortal(
      <button className={`dock-mini dock-at-${screen} ${st.running ? 'running' : ''}`} onClick={() => { setMini(false); setOpen(true) }} aria-label="Buka Orchestrator" title={`Orchestrator · ${status}`}>
        <Icon name="spark" />
        {st.running && <i className="dm-ring" />}
      </button>,
      document.body,
    )
  }
  return createPortal(
    <div className={`dock dock-at-${screen} ${open ? 'open' : ''} ${st.running ? 'running' : ''}`}>
      <div className="dk-head">
        <button className="dk-main" onClick={() => setOpen(!open)} aria-label="Orchestrator">
          <span className="dk-dot" />
          <span className="dk-txt">
            <b>Orchestrator</b>
            <span>{status}</span>
          </span>
        </button>
        <button type="button" className="dk-min" onClick={() => setMini(true)} aria-label="Ringkas jadi ikon" title="Ringkas jadi ikon"><Icon name="chev" /></button>
      </div>
      {inner}
    </div>,
    document.body,
  )
}
