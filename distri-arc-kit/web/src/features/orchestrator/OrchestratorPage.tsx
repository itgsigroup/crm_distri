import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import type { AgentInfo, Conflict, CycleStage, WAAnalysis, WAHistoryRow } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { PIPELINE_STAGES } from '../../lib/i18n/id'
import { STAGES, chipState } from '../../app/cycle'
import { useOrch, useOrchStatus } from '../../app/orch'
import { EFFORTS, effort, setEffort, type Effort } from '../../app/enginePrefs'
import { useAgents, useConflicts, useCycleEngines, useCycleLatest, useMCPCalls, useMCPInfo, useQueue, useWAAnalysis } from '../../app/queries'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { ColumnTable, type Column } from '../../components/DataTable'
import { QueueTable } from '../control/tables'
import { useCanWA } from '../wa/WAAnalysis'
import { waitLabel } from '../wa/wa'
import { MCPClientsPanel, MCPRules } from '../settings/AIConnections'
import { CONFLICT_COMPARE, conflictText } from './tables'

// Orchestrator (mockup screen-orch, the components of 08-frontend.md): header, Pipeline, Resolusi konflik, Riwayat
// analisis, Agen, MCP, Matriks otonomi — every analysis read from WhatsApp; Analisis ulang by Claude run on the server
// (API & model AI through the MCP tools, no Claude Desktop) with its progress in the pop-up (Sam 19 Oct, ADR 0045).

// Default stage descriptions before the first cycle (mockup STAGES).
const STAGE_HINT = ['chat WhatsApp · stok BigQuery', '6 agen membaca chat', 'konflik antar agen', 'otonom · ke Anda', 'antrean per sales', 'kalibrasi']

const NO_WA = 'Analisa WhatsApp mengutip pesan pelanggan, jadi hanya tampil untuk pengguna dengan akses halaman Chat.'
const fmt = (v: number | undefined | null) => (v ?? 0).toLocaleString('id-ID')

/** Pipeline analisis (mockup pipeHtml, full form); Ingest says what was read from WhatsApp. */
function Pipeline({ stages, wa }: { stages: CycleStage[]; wa: WAAnalysis | undefined }) {
  const { s } = useOrch()
  const st = useOrchStatus()
  return (
    <div className="pipe orch-pipe">
      {PIPELINE_STAGES.map((n, i) => {
        const detail = i === 0 && wa?.cycle_number ? `${fmt(wa.totals.threads)} chat WhatsApp · ${fmt(wa.totals.inbound)} pesan masuk · stok` : stages.find((x) => x.stage === STAGES[i])?.detail?.text
        return (
          <div key={n} className={`pst ${chipState(s, i, st.hasCycle)}`}>
            <div className="pn"><i />{n}</div>
            <div className="pd">{detail ?? STAGE_HINT[i]}</div>
          </div>
        )
      })}
    </div>
  )
}

/** What each agent read in the chats (intent counts measured from WhatsApp, ADR 0045). */
function waOutput(name: string, wa: WAAnalysis | undefined): string {
  const t = wa?.totals
  if (!t) return 'Belum ada Analisa WhatsApp'
  const n = (k: keyof typeof t.by_intent) => t.by_intent[k] ?? 0
  const due = waitLabel(wa.reply_due_min ?? 120)
  switch (name) {
    case 'AI Order': return `${n('order')} chat minta order`
    case 'AI Follow-up': return `${t.waiting} chat belum dibalas · ${t.needs_reply} lewat ${due}`
    case 'AI Kredit': return `${n('release')} chat minta rilis kredit`
    case 'AI Stok': return `${n('ask_stock')} chat tanya stok · ${n('ask_price')} tanya harga`
    case 'AI Penagihan': return `${n('return')} chat retur / klaim`
    case 'AI Prospek': return `${t.new_numbers} nomor baru menghubungi`
  }
  return `${fmt(t.threads)} chat dibaca`
}

/** Riwayat analisis: every Analisa WhatsApp (data table). */
function WAHistory({ rows }: { rows: WAHistoryRow[] }) {
  const n = (v: number | null) => v ?? -1
  const cols: Column<WAHistoryRow>[] = [
    { key: 'jam', label: 'Waktu', cell: (r) => <span className="num">{shortDate(r.at)} · {hhmm(r.at)}</span>, sort: (a, b) => Date.parse(a.at) - Date.parse(b.at), first: 'desc', className: 'nowrap' },
    { key: 'no', label: 'Siklus', cell: (r) => (r.cycle_number ? `#${fmt(r.cycle_number)}` : '—'), sort: (a, b) => n(a.cycle_number) - n(b.cycle_number), first: 'desc', right: true },
    { key: 'jalur', label: 'Jalur', cell: (r) => <span className={`pill ${r.via === 'mcp' ? 'indigo' : 'neutral'}`}>{(r.via || 'api').toUpperCase()}</span>, sort: (a, b) => a.via.localeCompare(b.via) },
    { key: 'chat', label: 'Chat', cell: (r) => <>{fmt(r.threads)}<span className="cc-sub">{fmt(r.inbound)} masuk</span></>, sort: (a, b) => a.threads - b.threads, first: 'desc', right: true },
    { key: 'menunggu', label: 'Menunggu', cell: (r) => fmt(r.waiting), sort: (a, b) => a.waiting - b.waiting, first: 'desc', right: true },
    { key: 'balas', label: 'Balas', cell: (r) => (r.reply_median_min == null ? '—' : waitLabel(r.reply_median_min)), sort: (a, b) => n(a.reply_median_min) - n(b.reply_median_min), right: true },
    { key: 'sumber', label: 'Sumber', cell: (r) => (r.source === 'ai' ? <Pill tone="accent">Claude</Pill> : <Pill tone="neutral">angka</Pill>), sort: (a, b) => a.source.localeCompare(b.source) },
  ]
  return (
    <ColumnTable rows={rows} columns={cols} rowKey={(r) => r.at + (r.cycle_number ?? '')} noun="analisa" placeholder="Cari siklus, jalur, sumber…" initial={{ column: 'jam', dir: 'desc' }}
      text={(r) => `#${r.cycle_number ?? ''} ${r.via} ${r.status} ${r.source === 'ai' ? 'claude' : 'angka'}`}
      empty="Belum ada Analisa WhatsApp — muncul setelah siklus pertama atau Analisis ulang lewat Claude." />
  )
}

/** Resolusi konflik on the dealers that wrote on WhatsApp (data table). */
function Conflicts({ rows }: { rows: Conflict[] }) {
  const nav = useNavigate()
  const cols: Column<Conflict>[] = [
    { key: 'agen', label: 'Agen', cell: (c) => <span className="nowrap"><span className="cf-dot" style={{ background: `var(--${c.tone && c.tone !== 'neutral' ? c.tone : 'accent'})` }} /><b>{c.agent_a}</b> ↔ <b>{c.agent_b}</b></span>, sort: CONFLICT_COMPARE.agen },
    { key: 'dealer', label: 'Dealer', cell: (c) => (c.dealer_slug ? <button className="ev" onClick={() => nav('/dealer/' + c.dealer_slug)}>{c.dealer_name}</button> : '—'), sort: CONFLICT_COMPARE.dealer },
    { key: 'konflik', label: 'Konflik & resolusi', cell: (c) => <>{c.title}<span className="cc-sub"><span className="ai" />{c.resolution}</span></>, sort: CONFLICT_COMPARE.konflik },
  ]
  return (
    <ColumnTable rows={rows} columns={cols} rowKey={(c) => c.id} noun="konflik" placeholder="Cari agen, dealer, konflik, resolusi…" initial={{ column: 'agen', dir: 'asc' }}
      text={conflictText} empty="Belum ada konflik pada dealer yang chat di siklus terakhir." />
  )
}

/** One agent: role, what it read in the chats, its proposals from chat, calibration, re-run through Claude. */
function AgentCard({ a, wa, fromChat, run, ready }: { a: AgentInfo; wa: WAAnalysis | undefined; fromChat: number; run: (scope: string) => void; ready: boolean }) {
  const st = useOrchStatus()
  return (
    <div className="agent-card">
      <div className="ac-h"><span className="ni"><Icon name={a.icon} /></span><div><b>{a.name}</b><span>{a.role}</span></div></div>
      <div className="ac-out"><Icon name="chat" />{waOutput(a.name, wa)}{fromChat > 0 && <b> · {fromChat} usulan</b>}</div>
      <div className="ac-f">
        <div className="cal-row"><span>kalibrasi</span><div className="bar"><i style={{ width: `${a.confidence ?? 0}%`, ...(a.confidence !== null && a.confidence < 60 ? { background: 'var(--warn)' } : {}) }} /></div><span className="v num">{a.confidence === null ? '—' : `${a.confidence}%`}</span></div>
        <button className="btn quiet" disabled={st.running || !ready} onClick={() => run('agent:' + a.name)} title="Claude menganalisis ulang agen ini di server">Jalankan ulang</button>
      </div>
    </div>
  )
}

function argText(a: Record<string, unknown> | null): string {
  if (!a) return ''
  return Object.entries(a).filter(([, v]) => v !== '' && v != null && !(Array.isArray(v) && v.length === 0)).map(([k, v]) => `${k}:${typeof v === 'string' ? `"${v}"` : Array.isArray(v) ? `[${v.length}]` : JSON.stringify(v)}`).join(', ')
}

export function OrchestratorPage() {
  const { openSheet } = useFeedback()
  const { reanalyze } = useOrch()
  const st = useOrchStatus()
  const { data: latest } = useCycleLatest()
  const page = useSalesPage()
  const sales = page.sales
  const { data: conflicts = [] } = useConflicts(sales)
  const { data: agents = [] } = useAgents(sales)
  const { data: queue = [] } = useQueue(sales)
  const { data: info } = useMCPInfo()
  const { data: calls = [] } = useMCPCalls()
  const canWA = useCanWA()
  const { data: wa } = useWAAnalysis(sales, canWA)
  // Claude analyses on the server (Sam 19 Oct): the API and model of Pengaturan → API & model AI run the same MCP tools —
  // nobody opens Claude Desktop; the effort is shared with the dock
  const { data: eng } = useCycleEngines()
  const ready = !!eng?.model.ready
  const [eff, setEff] = useState<Effort>(effort)
  const run = (scope: string) => reanalyze(scope, 'model', eng?.model.label)
  const shown = st.running && latest?.cycle ? latest.cycle : (latest?.last_full ?? latest?.last_done)
  const meta = st.run ? `siklus #${st.run.toLocaleString('id-ID')} · ${st.last} · ${st.dur}` : 'belum ada siklus'
  // WhatsApp only: the proposals tied to a conversation, the conflicts on dealers that wrote
  const waIds = new Set((wa?.items ?? []).flatMap((r) => r.proposal_ids))
  const waQueue = queue.filter((q) => waIds.has(q.id))
  const waDealers = new Set((wa?.items ?? []).map((r) => r.dealer_id).filter(Boolean))
  const waConflicts = conflicts.filter((c) => c.dealer_id && waDealers.has(c.dealer_id))
  const lastModel = st.lastDone?.model_label ? `${st.lastDone.via === 'mcp' ? 'MCP · ' : ''}${st.lastDone.model_label}${st.lastDone.effort ? ` · ${st.lastDone.effort}` : ''}` : '—'

  return (
    <>
      <SalesPicker page={page} />
      <SalesBanner page={page} />
      <div className="card orch-head">
        <div className="oh-l">
          <div className="oh-title"><span className="brand-mark sm" /><div><h2>Orchestrator</h2><p>Satu pengatur untuk enam agen: membaca chat WhatsApp (dan stok BigQuery untuk push stok), membagi tugas, menyelesaikan konflik antar agen, lalu menyerahkan keputusan yang di luar batas otonomi kepada Anda. Analisanya ditulis Claude, dijalankan sistem di server — tanpa membuka Claude Desktop.</p></div></div>
          <div className="oh-eng">
            <span className="dk-model"><span>Mesin</span>{eng ? (ready ? <b>{eng.model.label} · lewat sistem</b> : <b className="oh-warn">{eng.model.why || 'API belum siap'}</b>) : <b>memuat…</b>} <Link to="/pengaturan/api-ai" className="oh-link">ubah</Link></span>
            <div className="dk-effort" role="radiogroup" aria-label="Effort"><span>Effort</span>
              {EFFORTS.map((x) => <button key={x.id} type="button" role="radio" aria-checked={eff === x.id} className={eff === x.id ? 'on' : ''} disabled={st.running} title={x.hint} onClick={() => { setEff(x.id); setEffort(x.id) }}>{x.label}</button>)}
            </div>
          </div>
        </div>
        <div className="oh-r">
          <div className="oh-kv"><small>Status</small><b>{st.running ? `Menganalisis ${st.scope} · ${st.stage}` : 'Siap · menunggu siklus'}</b></div>
          <div className="oh-kv"><small>Analisis terakhir</small><b>{st.run ? `#${st.run.toLocaleString('id-ID')} · ${st.last}` : '—'}</b></div>
          {canWA && <div className="oh-kv"><small>WhatsApp terakhir dibaca</small><b>{wa?.generated_at ? `#${fmt(wa.cycle_number)} · ${shortDate(wa.generated_at)} · ${hhmm(wa.generated_at)}` : 'belum ada'}</b></div>}
          <div className="oh-kv"><small>Model AI terakhir</small><b>{lastModel}</b></div>
          <div className="oh-btns">
            <button className="btn primary" disabled={st.running || !ready} onClick={() => run('all')} title={ready ? `${eng?.model.label} membaca chat WhatsApp di server — tidak perlu membuka Claude Desktop` : eng?.model.why || 'Isi kunci API di Pengaturan → API & model AI'}><Icon name="refresh" />Analisis ulang</button>
          </div>
        </div>
      </div>
      {!canWA ? (
        <div className="card"><p className="sg-hint">{NO_WA}</p></div>
      ) : (
        <div className="orch-grid">
          <div className="stack">
            <div className="card">
              <div className="card-h"><h2>Pipeline analisis</h2><span className="meta">{meta}</span></div>
              <Pipeline stages={shown?.stages ?? []} wa={wa} />
            </div>
            {sales && (
              <div className="card">
                <div className="card-h"><h2>Keputusan sales ini</h2><span className="meta">{waQueue.length ? `${waQueue.length} usulan dari chat` : 'Tidak ada usulan dari chat hari ini'}</span></div>
                <QueueTable items={waQueue} />
              </div>
            )}
            <div className="card">
              <div className="card-h"><h2>Resolusi konflik</h2><span className="ai" style={{ marginLeft: 6 }}>inti pekerjaan Orchestrator</span><span className="meta">dealer yang chat · dua agen, satu urutan</span></div>
              <Conflicts rows={waConflicts} />
            </div>
            <div className="card">
              <div className="card-h"><h2>Riwayat analisis</h2><span className="meta">Analisa WhatsApp per siklus · {wa?.window_days ?? 7} hari terakhir per baris</span></div>
              <WAHistory rows={wa?.history ?? []} />
            </div>
            <div className="card">
              <div className="card-h"><h2>Matriks otonomi</h2><span className="meta">dibaca Orchestrator sebelum membagi tugas</span></div>
              <div className="tbl-wrap">
                <table className="matrix">
                  <thead><tr><th>Agen</th><th>Otonom</th><th>Butuh approve</th><th>Tidak boleh</th></tr></thead>
                  <tbody>{agents.map((a) => <tr key={a.name}><td><b>{a.name}</b></td><td><span className="chip g">{a.auto}</span></td><td><span className="chip w">{a.approve}</span></td><td><span className="chip b">{a.never}</span></td></tr>)}</tbody>
                </table>
              </div>
            </div>
          </div>
          <div className="stack">
            <div className="card">
              <div className="card-h"><h2>Agen</h2><span className="meta">dibaca dari chat WhatsApp · {sales ? 'dealer sales ini' : 'siklus terakhir'}</span></div>
              <div className="agent-grid">
                {agents.map((a) => <AgentCard key={a.name} a={a} wa={wa} fromChat={waQueue.filter((q) => q.agent === a.name).length} run={run} ready={ready} />)}
              </div>
            </div>
            <div className="card">
              <div className="card-h"><h2>MCP sebagai orchestrator</h2>{info?.enabled ? <Pill tone="good" icon="check">Aktif</Pill> : <Pill tone="neutral" icon="plug">Mati</Pill>}<button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 'auto' }} onClick={() => openSheet(<MCPClientsPanel />)}>Klien &amp; token</button></div>
              <p className="orch-note">Klien MCP (Claude Desktop, ChatGPT, agent eksternal) tidak hanya membaca data: mereka bisa memicu analisis ulang, meminta rencana, dan menjalankan satu agen — lewat tool yang sama dengan yang dipakai Orchestrator internal.</p>
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
        </div>
      )}
    </>
  )
}
