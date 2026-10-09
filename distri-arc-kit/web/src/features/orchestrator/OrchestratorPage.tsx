import { useMemo } from 'react'
import { useNavigate } from 'react-router'
import type { Conflict, Cycle, CycleStage } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { PIPELINE_STAGES } from '../../lib/i18n/id'
import { STAGES, chipState } from '../../app/cycle'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useAgents, useConflicts, useCycleLatest, useCycles, useMCPCalls, useMCPInfo, useQueue } from '../../app/queries'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { Queue } from '../control/Queue'
import { MCPClientsPanel, MCPRules, ModeSeg } from '../settings/AIConnections'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { CONFLICT_COMPARE, RUN_COMPARE, conflictFirstDir, conflictText, conflictTie, finishedRuns, runFirstDir, runText, runTie, type ConflictColumn, type RunColumn } from './tables'

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

/** Riwayat analisis as a data table: search, sort every column both ways, pages of 10/25/50. */
function Runs({ cycles }: { cycles: Cycle[] }) {
  const rows = useMemo(() => finishedRuns(cycles), [cycles])
  const t = useDataTable<Cycle, RunColumn>({ rows, text: runText, compare: RUN_COMPARE, tie: runTie, initial: { column: 'jam', dir: 'desc' }, firstDir: runFirstDir })
  return (
    <>
      <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari nomor, jalur, catatan…" label="Cari riwayat analisis" meta={`${t.matches.length.toLocaleString('id-ID')} siklus`} />
      {rows.length === 0 ? (
        <p className="sg-hint">Belum ada siklus — siklus pertama berjalan di jam berikutnya (06.00–20.00) atau lewat Analisis ulang.</p>
      ) : t.matches.length === 0 ? (
        <p className="sg-hint">Tidak ada siklus yang cocok dengan pencarian ini.</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl runs-table">
            <thead><tr>
              <SortTh t={t} c="jam">Waktu</SortTh><SortTh t={t} c="no" right>No.</SortTh><SortTh t={t} c="jalur">Jalur</SortTh>
              <SortTh t={t} c="sinyal" right>Sinyal</SortTh><SortTh t={t} c="otonom" right>Otonom</SortTh><SortTh t={t} c="keputusan" right>Keputusan</SortTh>
              <SortTh t={t} c="konflik" right>Konflik</SortTh><SortTh t={t} c="catatan">Catatan</SortTh>
            </tr></thead>
            <tbody>
              {t.shown.map((r) => (
                <tr key={r.id}>
                  <td className="num nowrap">{shortDate(r.started_at)} · {hhmm(r.started_at)}</td>
                  <td className="r num">#{(r.number ?? 0).toLocaleString('id-ID')}</td>
                  <td><span className={`pill ${r.via === 'mcp' ? 'indigo' : 'neutral'}`}>{(r.via ?? 'api').toUpperCase()}</span></td>
                  <td className="r num">{r.signals_count ?? 0}</td>
                  <td className="r num">{r.auto_count ?? 0}</td>
                  <td className="r num">{r.decision_count ?? 0}</td>
                  <td className="r num">{r.conflict_count ?? 0}</td>
                  <td className="runs-note">{r.status === 'failed' ? <span style={{ color: 'var(--bad)' }}>{r.note}</span> : r.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun="siklus" />
    </>
  )
}

/** Resolusi konflik as a data table: search, sort every column both ways, pages of 10/25/50. */
function Conflicts({ rows, onOpen }: { rows: Conflict[]; onOpen: (slug: string) => void }) {
  const t = useDataTable<Conflict, ConflictColumn>({ rows, text: conflictText, compare: CONFLICT_COMPARE, tie: conflictTie, initial: { column: 'agen', dir: 'asc' }, firstDir: conflictFirstDir })
  return (
    <>
      <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari agen, dealer, konflik, resolusi…" label="Cari konflik" meta={`${t.matches.length.toLocaleString('id-ID')} konflik`} />
      {rows.length === 0 ? (
        <p className="sg-hint">Belum ada konflik pada siklus terakhir.</p>
      ) : t.matches.length === 0 ? (
        <p className="sg-hint">Tidak ada konflik yang cocok dengan pencarian ini.</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl conflicts-table">
            <thead><tr><SortTh t={t} c="agen">Agen</SortTh><SortTh t={t} c="dealer">Dealer</SortTh><SortTh t={t} c="konflik">Konflik</SortTh><SortTh t={t} c="resolusi">Resolusi Orchestrator</SortTh></tr></thead>
            <tbody>
              {t.shown.map((c) => {
                const k = c.tone && c.tone !== 'neutral' ? c.tone : 'accent'
                return (
                  <tr key={c.id}>
                    <td className="nowrap"><span className="cf-dot" style={{ background: `var(--${k})` }} /><b>{c.agent_a}</b> ↔ <b>{c.agent_b}</b></td>
                    <td>{c.dealer_slug ? <button className="ev" onClick={() => onOpen(c.dealer_slug!)}>{c.dealer_name}</button> : <span style={{ color: 'var(--text-3)' }}>—</span>}</td>
                    <td>{c.title}</td>
                    <td className="cf-res"><span className="ai" />{c.resolution}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun="konflik" />
    </>
  )
}

function argText(a: Record<string, unknown> | null): string {
  if (!a) return ''
  return Object.entries(a).filter(([, v]) => v !== '' && v != null && !(Array.isArray(v) && v.length === 0)).map(([k, v]) => `${k}:${typeof v === 'string' ? `"${v}"` : Array.isArray(v) ? `[${v.length}]` : JSON.stringify(v)}`).join(', ')
}

// Orchestrator (mockup screen-orch).
export function OrchestratorPage() {
  const nav = useNavigate()
  const { openSheet } = useFeedback()
  const { reanalyze } = useOrch()
  const st = useOrchStatus()
  const { data: latest } = useCycleLatest()
  const { data: cycles = [] } = useCycles()
  const page = useSalesPage()
  const sales = page.sales
  const { data: conflicts = [] } = useConflicts(sales)
  const { data: agents = [] } = useAgents(sales)
  const { data: queue = [] } = useQueue(sales)
  const { data: info } = useMCPInfo()
  const { data: calls = [] } = useMCPCalls()
  const shown = st.running && latest?.cycle ? latest.cycle : (latest?.last_full ?? latest?.last_done)
  const meta = st.run ? `siklus #${st.run.toLocaleString('id-ID')} · ${st.last} · ${st.dur}` : 'belum ada siklus'

  return (
    <>
      <div className="sales-bar"><SalesPicker page={page} /></div>
      <SalesBanner page={page} />
      <div className="card orch-head">
        <div className="oh-l">
          <div className="oh-title"><span className="brand-mark sm" /><div><h2>Orchestrator</h2><p>Satu pengatur untuk enam agen: membaca sinyal, membagi tugas, menyelesaikan konflik antar agen, lalu menyerahkan keputusan yang di luar batas otonomi kepada Anda.</p></div></div>
        </div>
        <div className="oh-r">
          <div className="oh-kv"><small>Status</small><b>{st.running ? `Menganalisis ${st.scope} · ${st.stage}` : 'Siap · menunggu siklus'}</b></div>
          <div className="oh-kv"><small>Analisis terakhir</small><b>{st.run ? `#${st.run.toLocaleString('id-ID')} · ${st.last}` : '—'}</b></div>
          <div className="oh-kv"><small>Berikutnya</small><b>{st.next} · tiap jam</b></div>
          <div className="oh-kv"><small>Jalur analisis</small><ModeSeg /></div>
          <div className="oh-btns">
            <button className="btn primary" disabled={st.running} onClick={() => reanalyze('all')}><Icon name="refresh" />Analisis ulang sekarang</button>
            <button className="btn ghost" disabled={st.running} onClick={() => reanalyze('all', 'mcp')}><Icon name="plug" />Lewat MCP</button>
          </div>
        </div>
      </div>
      <div className="orch-grid">
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Pipeline analisis</h2><span className="meta">{meta}</span></div>
            <Pipeline stages={shown?.stages ?? []} />
          </div>
          {sales && (
            <div className="card">
              <div className="card-h"><h2>Keputusan sales ini</h2><span className="meta">{queue.length ? `${queue.length} item hari ini` : 'Tidak ada keputusan hari ini'}</span></div>
              <Queue items={queue} />
            </div>
          )}
          <div className="card">
            <div className="card-h"><h2>Resolusi konflik</h2><span className="ai" style={{ marginLeft: 6 }}>inti pekerjaan Orchestrator</span><span className="meta">dua agen, satu dealer, satu urutan</span></div>
            <Conflicts rows={conflicts} onOpen={(slug) => nav('/dealer/' + slug)} />
          </div>
          <div className="card">
            <div className="card-h"><h2>Riwayat analisis</h2><span className="meta">{sales ? 'siklus berjalan untuk seluruh tim · tanpa rincian dealer' : 'tiap jam · 06.00–20.00 · 100 siklus terakhir'}</span></div>
            <Runs cycles={cycles} />
          </div>
        </div>
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Agen</h2><span className="meta">{sales ? 'peran · otonomi · usulan hari ini untuk dealer sales ini' : 'peran · otonomi · hasil siklus terakhir'}</span></div>
            <div className="agent-grid">
              {agents.map((a) => (
                <div className="agent-card" key={a.name}>
                  <div className="ac-h"><span className="ni"><Icon name={a.icon} /></span><div><b>{a.name}</b><span>{a.role}</span></div></div>
                  <div className="ac-kv"><span><small>Otonom</small>{a.auto}</span><span><small>Butuh approve</small>{a.approve}</span><span><small>Tidak boleh</small>{a.never}</span></div>
                  <div className="ac-out"><span className="ai" />{sales ? `${a.today} usulan hari ini untuk dealer sales ini` : a.output || 'Belum berjalan'}</div>
                  <div className="ac-f">
                    <div className="cal-row"><span>kalibrasi</span><div className="bar"><i style={{ width: `${a.confidence ?? 0}%`, ...(a.confidence !== null && a.confidence < 60 ? { background: 'var(--warn)' } : {}) }} /></div><span className="v num">{a.confidence === null ? '—' : `${a.confidence}%`}</span></div>
                    <button className="btn quiet" style={{ height: 26, fontSize: 12 }} disabled={st.running} onClick={() => reanalyze('agent:' + a.name)}>Jalankan ulang</button>
                  </div>
                </div>
              ))}
            </div>
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
      </div>
    </>
  )
}
