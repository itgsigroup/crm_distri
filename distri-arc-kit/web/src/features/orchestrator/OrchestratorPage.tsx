import type { CycleStage } from '../../api/types'
import { effort, mcpModel, shortModel } from '../../app/enginePrefs'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { PIPELINE_STAGES } from '../../lib/i18n/id'
import { STAGES, chipState } from '../../app/cycle'
import { useOrch, useOrchStatus } from '../../app/orch'
import { InlineAnalysis } from '../../app/AnalysisSheet'
import { useCycleLatest, useMCPCalls, useMCPInfo, useQueue, useWAAnalysis } from '../../app/queries'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { Queue } from '../control/Queue'
import { WAHistoryCard, WAOverviewCard, WAThreadsCard, useCanWA } from '../wa/WAAnalysis'
import { WASalesGrid } from '../wa/WACards'
import { MCPClientsPanel, MCPRules } from '../settings/AIConnections'

// Default stage descriptions before the first cycle (mockup STAGES).
const STAGE_HINT = ['WA · SO · bayar · stok cabang', '6 agen paralel', 'konflik antar agen', 'otonom · ke Anda', 'antrean per sales', 'kalibrasi']

/** Pipeline analisis (mockup pipeHtml, full form). */
function Pipeline({ stages }: { stages: CycleStage[] }) {
  const { s } = useOrch()
  const st = useOrchStatus()
  return (
    <div className="pipe">
      {PIPELINE_STAGES.map((n, i) => {
        const detail = stages.find((x) => x.stage === STAGES[i])?.detail?.text
        return (
          <span key={n} style={{ display: 'contents' }}>
            {i > 0 && <div className="pl" />}
            <div className={`pst ${chipState(s, i, st.hasCycle)}`}>
              <div className="pn"><i />{n}</div>
              <div className="pd">{detail ?? STAGE_HINT[i]}</div>
            </div>
          </span>
        )
      })}
    </div>
  )
}

function argText(a: Record<string, unknown> | null): string {
  if (!a) return ''
  return Object.entries(a).filter(([, v]) => v !== '' && v != null && !(Array.isArray(v) && v.length === 0)).map(([k, v]) => `${k}:${typeof v === 'string' ? `"${v}"` : Array.isArray(v) ? `[${v.length}]` : JSON.stringify(v)}`).join(', ')
}

// Orchestrator (mockup screen-orch).
export function OrchestratorPage() {
  const { openSheet } = useFeedback()
  const { reanalyze } = useOrch()
  const st = useOrchStatus()
  const { data: latest } = useCycleLatest()
  const page = useSalesPage()
  const sales = page.sales
  const { data: queue = [] } = useQueue(sales)
  const { data: info } = useMCPInfo()
  const { data: calls = [] } = useMCPCalls()
  const shown = st.running && latest?.cycle ? latest.cycle : (latest?.last_full ?? latest?.last_done)
  const meta = st.run ? `siklus #${st.run.toLocaleString('id-ID')} · ${st.last} · ${st.dur}` : 'belum ada siklus'
  // Analisa WhatsApp (ADR 0026) sits inside the page; it quotes customers, so only for users with the chat page
  const canWA = useCanWA()
  const { data: wa } = useWAAnalysis(sales, canWA)
  const waIds = new Set((wa?.items ?? []).flatMap((r) => r.proposal_ids))
  const waQueue = queue.filter((q) => waIds.has(q.id))

  return (
    <>
      <SalesPicker page={page} />
      <SalesBanner page={page} />
      <div className="card orch-head">
        <div className="oh-l">
          <div className="oh-title"><span className="brand-mark sm" /><div><h2>Orchestrator</h2><p>Satu pengatur untuk enam agen. Analisanya ditulis Claude lewat MCP; GSI Orbit hanya menyiapkan data dan angka, menyelesaikan konflik antar usulan, lalu menyerahkan keputusan kepada Anda.</p></div></div>
        </div>
        <div className="oh-r">
          <div className="oh-kv"><small>Status</small><b>{st.running ? `Menganalisis ${st.scope} · ${st.stage}` : 'Siap · menunggu siklus'}</b></div>
          <div className="oh-kv"><small>Analisis terakhir</small><b>{st.run ? `#${st.run.toLocaleString('id-ID')} · ${st.last}` : '—'}</b></div>
          {canWA && <div className="oh-kv"><small>WhatsApp terakhir dibaca</small><b>{wa?.generated_at ? `#${(wa.cycle_number ?? 0).toLocaleString('id-ID')} · ${shortDate(wa.generated_at)} · ${hhmm(wa.generated_at)}` : 'belum ada'}</b></div>}
          <div className="oh-kv"><small>Berikutnya</small><b>{st.next} · tiap jam</b></div>
          <div className="oh-kv"><small>Model AI terakhir</small><b title="Model dan effort yang dipilih untuk analisis terakhir (Orchestrator di kanan bawah)">{st.lastDone?.model_label ? `${st.lastDone.via === 'mcp' ? 'MCP · ' : ''}${st.lastDone.model_label}${st.lastDone.effort ? ` · ${st.lastDone.effort}` : ''}` : 'Claude lewat MCP'}</b></div>
          <div className="oh-btns">
            <button className="btn primary" disabled={st.running} onClick={() => reanalyze('all', 'mcp', undefined, true)} title={`Claude menganalisis lewat MCP · ${shortModel(mcpModel())} · effort ${effort()} (ubah di Orchestrator kanan bawah)`}><Icon name="plug" />Analisis ulang (Claude · MCP)</button>
          </div>
        </div>
      </div>
      <InlineAnalysis />
      {!canWA ? (
        <div className="card"><p className="sg-hint">Analisa WhatsApp mengutip pesan pelanggan, jadi hanya tampil untuk pengguna dengan akses halaman Chat.</p></div>
      ) : <div className="orch-grid">
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Pipeline analisis</h2><span className="meta">{meta}</span></div>
            <Pipeline stages={shown?.stages ?? []} />
          </div>
          <WAOverviewCard data={wa} scoped={!!sales || page.locked} />
          {sales && (
            <div className="card">
              <div className="card-h"><h2>Keputusan sales ini</h2><span className="meta">{waQueue.length ? `${waQueue.length} usulan dari chat` : 'Tidak ada usulan dari chat hari ini'}</span></div>
              <Queue items={waQueue} />
            </div>
          )}
          <WAThreadsCard data={wa} />
          <WAHistoryCard rows={wa?.history ?? []} />
        </div>
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Sales</h2><span className="meta">chat · belum dibalas · waktu balas</span></div>
            <WASalesGrid rows={wa?.sales ?? []} />
          </div>
          <div className="card">
            <div className="card-h"><h2>MCP sebagai orchestrator</h2>{info?.enabled ? <Pill tone="good" icon="check">Aktif</Pill> : <Pill tone="neutral" icon="plug">Mati</Pill>}<button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 'auto' }} onClick={() => openSheet(<MCPClientsPanel />)}>Klien &amp; token</button></div>
            <p style={{ fontSize: 13, color: 'var(--text-2)', lineHeight: 1.5, marginBottom: 10 }}>Klien MCP (Claude Desktop, ChatGPT, agent eksternal) tidak hanya membaca data: mereka bisa memicu analisis ulang, meminta rencana, dan menjalankan satu agen — lewat tool yang sama dengan yang dipakai Orchestrator internal.</p>
            <MCPRules />
            <div className="hr" />
            <h3 className="h3">Panggilan terakhir dari klien MCP</h3>
            <ul className="mcp-log">
              {calls.slice(0, 5).map((m) => (
                <li key={m.id}><span className="num t">{hhmm(m.created_at)}</span><div><b>{m.client_name ?? 'Klien tanpa token'}</b><code style={{ overflowWrap: 'anywhere' }}>{m.tool}({argText(m.args)})</code><span style={m.status === 'ok' ? undefined : { color: 'var(--bad)' }}>→ {m.status === 'ok' ? m.result_summary : `${m.status} · ${m.result_summary ?? ''}`}</span></div></li>
              ))}
              {calls.length === 0 && <li><span className="num t">—</span><div><b>Belum ada panggilan</b><span>Buat token di Klien &amp; token, lalu sambungkan Claude Desktop ke {info?.endpoint ?? '/mcp'}</span></div></li>}
            </ul>
            <div className="hr" />
            <h3 className="h3">Tool MCP</h3>
            <div className="tools">
              {([['read', 'Baca'], ['analyze', 'Analisis'], ['orchestrate', 'Orkestrasi'], ['decide', 'Keputusan · human-only']] as const).map(([k, l]) => (
                <div key={k}><small>{l}</small><code style={{ overflowWrap: 'anywhere' }}>{(info?.tools ?? []).filter((t) => t.scope === k).map((t) => t.name).join(' · ')}</code></div>
              ))}
            </div>
          </div>
        </div>
      </div>}
    </>
  )
}
