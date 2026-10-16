import { useState } from 'react'
import { useLocation } from 'react-router'
import { Icon } from '../components/Icon'
import { PIPELINE_STAGES, type ScreenKey } from '../lib/i18n/id'
import { chipState, progress } from './cycle'
import { scopeLabel, useOrch, useOrchStatus } from './orch'
import { useCycleEngines } from './queries'

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

export type Engine = 'mcp' | 'model'
const ENGINE_KEY = 'orch-engine'
function savedEngine(): Engine | null {
  try { const v = localStorage.getItem(ENGINE_KEY); return v === 'mcp' || v === 'model' ? v : null } catch { return null }
}

// Floating Orchestrator dock on every screen except Pusat kendali, Orchestrator and Chat: re-analyse this page (or
// everything) with Claude through MCP or with the API and model chosen on Pengaturan → API & model AI; the page's
// data refreshes when the cycle is done.
export function Dock({ screen }: { screen: ScreenKey }) {
  const [open, setOpen] = useState(false)
  const { s, reanalyze } = useOrch()
  const st = useOrchStatus()
  const { pathname } = useLocation()
  const { data: eng } = useCycleEngines(open)
  const [pick, setPick] = useState<Engine | null>(savedEngine)
  const modelReady = !!eng?.model.ready
  const engine: Engine = pick === 'model' && !modelReady ? 'mcp' : pick ?? (modelReady ? 'model' : 'mcp')
  const choose = (e: Engine) => { setPick(e); try { localStorage.setItem(ENGINE_KEY, e) } catch { /* private mode */ } }
  const scope = screenScope(screen, pathname)
  const run = (sc: string) => reanalyze(sc, engine, engine === 'model' ? eng?.model.label : 'Claude lewat MCP')
  return (
    <div className={`dock ${open ? 'open' : ''} ${st.running ? 'running' : ''}`}>
      <button className="dk-main" onClick={() => setOpen(!open)} aria-label="Orchestrator">
        <span className="dk-dot" />
        <span className="dk-txt">
          <b>Orchestrator</b>
          <span>{st.running ? `menganalisis ${st.scope} · ${st.stage}` : st.run ? `siap · siklus #${st.run.toLocaleString('id-ID')} · ${st.last}` : 'siap · belum ada siklus'}</span>
        </span>
      </button>
      <div className="dk-body">
        <div className="dk-pipe"><PipeChips /></div>
        <div className="dk-track"><i style={{ width: `${progress(s)}%` }} /></div>
        <div className="dk-scope">
          <span>Layar ini: <b>{scopeLabel(scope)}</b></span>
          <small>{scopeAgents(scope).join(' · ')}</small>
        </div>
        <div className="dk-eng" role="radiogroup" aria-label="Dianalisis oleh">
          <button type="button" role="radio" aria-checked={engine === 'mcp'} className={engine === 'mcp' ? 'on' : ''} disabled={st.running} onClick={() => choose('mcp')}>
            <Icon name="plug" /><span><b>MCP Claude</b><small>claude.ai / Desktop</small></span>
          </button>
          <button type="button" role="radio" aria-checked={engine === 'model'} className={engine === 'model' ? 'on' : ''} disabled={st.running || !modelReady} title={eng?.model.why || undefined} onClick={() => choose('model')}>
            <Icon name="spark" /><span><b>{eng ? eng.model.model_label : 'API & model AI'}</b><small>{!eng ? 'memuat…' : modelReady ? `${eng.model.provider} · di server` : 'kunci API belum diisi'}</small></span>
          </button>
        </div>
        <div className="dk-acts">
          <button className="btn primary" disabled={st.running} onClick={() => run(scope)}><Icon name="refresh" />Analisis ulang layar ini</button>
          <button className="btn ghost" disabled={st.running} onClick={() => run('all')}>Semua</button>
        </div>
      </div>
    </div>
  )
}
