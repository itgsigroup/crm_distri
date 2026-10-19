import { useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useLocation } from 'react-router'
import { Icon } from '../components/Icon'
import { PIPELINE_STAGES, type ScreenKey } from '../lib/i18n/id'
import { chipState, progress } from './cycle'
import { SYSTEM_LABEL, scopeLabel, useOrch, useOrchStatus } from './orch'
import { useCycleEngines } from './queries'
import { EFFORTS, MCP_MODELS, effort, mcpModel, setEffort, setMcpModel, shortModel, type Effort } from './enginePrefs'

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
    case 'dealer': {
      const id = path.split('/')[2]
      return id ? 'dealer:' + id : 'screen:dealer'
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
  } else if (kind === 'agent') agents = [id]
  if (agents === null) return ['6 agen', 'Analisa Stok', 'Analisa Kas', 'Analisa WhatsApp']
  const out = [...agents]
  if (agents.includes('AI Stok')) out.push('Analisa Stok')
  if (agents.includes('AI Kredit') || agents.includes('AI Penagihan')) out.push('Analisa Kas')
  return out
}

export type Engine = 'system' | 'mcp' | 'model'
const ENGINE_KEY = 'orch-engine'
function savedEngine(): Engine | null {
  try { const v = localStorage.getItem(ENGINE_KEY); return v === 'system' || v === 'mcp' || v === 'model' ? v : null } catch { return null }
}
const MINI_KEY = 'orch-dock-mini'
function savedMini(): boolean {
  try { return localStorage.getItem(MINI_KEY) === '1' } catch { return false }
}

// Floating Orchestrator dock, fixed at the bottom right of the screen on every page except Dashboard, Orchestrator and Chat (it can shrink to a round icon): re-analyse this page (or
// everything) with GSI Orbit's own rules (Sistem, ADR 0046), Claude through MCP, or the API and model chosen on Pengaturan → API & model AI; the page's
// data refreshes when the cycle is done.
export function Dock({ screen, extra, links }: { screen: ScreenKey; extra?: ReactNode; links?: ReactNode }) {
  const [open, setOpen] = useState(false)
  const [mini, setMiniState] = useState(savedMini)
  const setMini = (m: boolean) => { setMiniState(m); if (m) setOpen(false); try { localStorage.setItem(MINI_KEY, m ? '1' : '0') } catch { /* private mode */ } }
  const { s, reanalyze } = useOrch()
  const st = useOrchStatus()
  const { pathname } = useLocation()
  const { data: eng } = useCycleEngines(open)
  const [pick, setPick] = useState<Engine | null>(savedEngine)
  const [model, setModel] = useState(mcpModel)
  const [eff, setEff] = useState<Effort>(effort)
  const chooseModel = (id: string) => { setModel(id); setMcpModel(id) }
  const chooseEffort = (e: Effort) => { setEff(e); setEffort(e) }
  const modelReady = !!eng?.model.ready
  const engine: Engine = pick === 'model' && !modelReady ? 'system' : pick ?? (modelReady ? 'model' : 'system')
  const choose = (e: Engine) => { setPick(e); try { localStorage.setItem(ENGINE_KEY, e) } catch { /* private mode */ } }
  const scope = screenScope(screen, pathname)
  const run = (sc: string) => reanalyze(sc, engine, engine === 'model' ? eng?.model.label : engine === 'system' ? SYSTEM_LABEL : 'Claude lewat MCP')
  const status = st.running ? `menganalisis ${st.scope} · ${st.stage}` : st.run ? `siap · siklus #${st.run.toLocaleString('id-ID')} · ${st.last}` : 'siap · belum ada siklus'
  // rendered on <body>: inside a page (.screen animates a transform) position: fixed would scroll with the content
  if (mini) {
    return createPortal(
      <button className={`dock-mini ${st.running ? 'running' : ''}`} onClick={() => { setMini(false); setOpen(true) }} aria-label="Buka Orchestrator" title={`Orchestrator · ${status}`}>
        <Icon name="spark" />
        {st.running && <i className="dm-ring" />}
      </button>,
      document.body,
    )
  }
  return createPortal(
    <div className={`dock ${open ? 'open' : ''} ${st.running ? 'running' : ''}`}>
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
      <div className="dk-body">
        <div className="dk-pipe"><PipeChips /></div>
        <div className="dk-track"><i style={{ width: `${progress(s)}%` }} /></div>
        {extra && <div className="dk-extra">{extra}</div>}
        <div className="dk-scope">
          <span>Layar ini: <b>{scopeLabel(scope)}</b></span>
          <small>{scopeAgents(scope).join(' · ')}</small>
        </div>
        <div className="dk-eng" role="radiogroup" aria-label="Dianalisis oleh">
          <button type="button" role="radio" aria-checked={engine === 'system'} className={engine === 'system' ? 'on' : ''} disabled={st.running} title="Aturan agen dan rumus GSI Orbit di server — langsung jalan, tanpa Claude dan tanpa biaya AI" onClick={() => choose('system')}>
            <Icon name="gear" /><span><b>Sistem</b><small>tanpa AI · langsung</small></span>
          </button>
          <button type="button" role="radio" aria-checked={engine === 'mcp'} className={engine === 'mcp' ? 'on' : ''} disabled={st.running} onClick={() => choose('mcp')}>
            <Icon name="plug" /><span><b>MCP Claude</b><small>{shortModel(model)} · {eff}</small></span>
          </button>
          <button type="button" role="radio" aria-checked={engine === 'model'} className={engine === 'model' ? 'on' : ''} disabled={st.running || !modelReady} title={eng?.model.why || undefined} onClick={() => choose('model')}>
            <Icon name="spark" /><span><b>{eng ? eng.model.model_label : 'API & model AI'}</b><small>{!eng ? 'memuat…' : modelReady ? `${eng.model.provider} · ${eff}` : 'kunci API belum diisi'}</small></span>
          </button>
        </div>
        <div className="dk-tune">
          {engine === 'system' ? (
            <span className="dk-model"><span>Mesin</span><b>Aturan &amp; rumus GSI Orbit · tanpa AI</b></span>
          ) : engine === 'mcp' ? (
            <label className="dk-model">
              <span>Model</span>
              <select value={model} disabled={st.running} onChange={(e) => chooseModel(e.target.value)} title="Pilih model yang sama di claude.ai / Desktop — perintahnya menyebut model ini">
                {MCP_MODELS.map((m) => <option key={m.id} value={m.id}>{m.label}</option>)}
              </select>
            </label>
          ) : (
            <span className="dk-model"><span>Model</span><b title="Diatur di Pengaturan → API & model AI">{eng?.model.model_label ?? '—'}</b></span>
          )}
          {engine !== 'system' && <div className="dk-effort" role="radiogroup" aria-label="Effort">
            <span>Effort</span>
            {EFFORTS.map((x) => (
              <button key={x.id} type="button" role="radio" aria-checked={eff === x.id} className={eff === x.id ? 'on' : ''} disabled={st.running} title={x.hint} onClick={() => chooseEffort(x.id)}>{x.label}</button>
            ))}
          </div>}
        </div>
        <div className="dk-acts">
          <button className="btn primary" disabled={st.running} onClick={() => run(scope)}><Icon name="refresh" />{scope === 'all' ? 'Analisis ulang' : 'Analisis ulang layar ini'}</button>
          {scope !== 'all' && <button className="btn ghost" disabled={st.running} onClick={() => run('all')}>Semua</button>}
          {links}
        </div>
      </div>
    </div>,
    document.body,
  )
}
