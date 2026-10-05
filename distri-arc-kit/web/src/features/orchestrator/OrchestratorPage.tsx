import { useNavigate } from 'react-router'
import type { Cycle, CycleStage } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm } from '../../lib/format'
import { PIPELINE_STAGES } from '../../lib/i18n/id'
import { STAGES, chipState } from '../../app/cycle'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useAgents, useConflicts, useCycleLatest, useCycles, usePolicies } from '../../app/queries'

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

function Runs({ cycles }: { cycles: Cycle[] }) {
  const rows = cycles.filter((c) => c.status !== 'queued' && c.status !== 'running').slice(0, 7)
  return (
    <table className="runs">
      <thead><tr><th>Jam</th><th>No.</th><th>Jalur</th><th>Sinyal</th><th>Otonom</th><th>Keputusan</th><th>Konflik</th><th>Catatan</th></tr></thead>
      <tbody>
        {rows.length === 0 && <tr><td colSpan={8} style={{ color: 'var(--text-3)' }}>Belum ada siklus — siklus pertama berjalan di jam berikutnya (06.00–20.00) atau lewat Analisis ulang.</td></tr>}
        {rows.map((r) => (
          <tr key={r.id}>
            <td className="num">{hhmm(r.started_at)}</td>
            <td className="num">#{(r.number ?? 0).toLocaleString('id-ID')}</td>
            <td><span className={`pill ${r.via === 'mcp' ? 'indigo' : 'neutral'}`}>{(r.via ?? 'api').toUpperCase()}</span></td>
            <td className="num">{r.signals_count ?? 0}</td>
            <td className="num">{r.auto_count ?? 0}</td>
            <td className="num">{r.decision_count ?? 0}</td>
            <td className="num">{r.conflict_count ?? 0}</td>
            <td>{r.status === 'failed' ? <span style={{ color: 'var(--bad)' }}>{r.note}</span> : r.note}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// Orchestrator (mockup screen-orch).
export function OrchestratorPage() {
  const nav = useNavigate()
  const { toast } = useFeedback()
  const { reanalyze } = useOrch()
  const st = useOrchStatus()
  const { data: latest } = useCycleLatest()
  const { data: cycles = [] } = useCycles()
  const { data: conflicts = [] } = useConflicts()
  const { data: agents = [] } = useAgents()
  const { data: pol } = usePolicies()
  const mode = String((pol?.['llm.routing']?.value as { mode?: string } | undefined)?.mode ?? 'both')
  const shown = st.running && latest?.cycle ? latest.cycle : (latest?.last_full ?? latest?.last_done)
  const meta = st.run ? `siklus #${st.run.toLocaleString('id-ID')} · ${st.last} · ${st.dur}` : 'belum ada siklus'
  const later = () => toast('Jalur analisis diatur di Pengaturan (Stage 11) · klien MCP tersambung di Stage 07')

  return (
    <>
      <div className="card orch-head">
        <div className="oh-l">
          <div className="oh-title"><span className="brand-mark sm" /><div><h2>Orchestrator</h2><p>Satu pengatur untuk enam agen: membaca sinyal, membagi tugas, menyelesaikan konflik antar agen, lalu menyerahkan keputusan yang di luar batas otonomi kepada Anda.</p></div></div>
        </div>
        <div className="oh-r">
          <div className="oh-kv"><small>Status</small><b>{st.running ? `Menganalisis ${st.scope} · ${st.stage}` : 'Siap · menunggu siklus'}</b></div>
          <div className="oh-kv"><small>Analisis terakhir</small><b>{st.run ? `#${st.run.toLocaleString('id-ID')} · ${st.last}` : '—'}</b></div>
          <div className="oh-kv"><small>Berikutnya</small><b>{st.next} · tiap jam</b></div>
          <div className="oh-kv"><small>Jalur analisis</small><div className="seg">{[['api', 'API AI'], ['mcp', 'MCP'], ['both', 'Keduanya']].map(([k, l]) => <button key={k} className={mode === k ? 'is-active' : ''} onClick={later}>{l}</button>)}</div></div>
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
          <div className="card">
            <div className="card-h"><h2>Resolusi konflik</h2><span className="ai" style={{ marginLeft: 6 }}>inti pekerjaan Orchestrator</span><span className="meta">dua agen, satu dealer, satu urutan</span></div>
            <ul className="conflicts">
              {conflicts.length === 0 && <li><span className="ii" style={{ background: 'var(--surface-2)', color: 'var(--text-3)' }}><Icon name="net" /></span><div><div className="cx">Belum ada konflik pada siklus terakhir.</div></div></li>}
              {conflicts.map((c) => {
                const k = c.tone && c.tone !== 'neutral' ? c.tone : 'accent'
                return (
                  <li key={c.id}>
                    <span className="ii" style={{ background: `var(--${k}-soft)`, color: `var(--${k})` }}><Icon name="net" /></span>
                    <div>
                      <div className="ct"><b>{c.agent_a}</b> ↔ <b>{c.agent_b}</b>{c.dealer_slug && <> · <button className="ev" onClick={() => nav('/dealer/' + c.dealer_slug)}>{c.dealer_name}</button></>}</div>
                      <div className="cx">{c.title}</div>
                      <div className="cr"><span className="ai" />{c.resolution}</div>
                    </div>
                  </li>
                )
              })}
            </ul>
          </div>
          <div className="card">
            <div className="card-h"><h2>Riwayat analisis</h2><span className="meta">tiap jam · 06.00–20.00</span></div>
            <div className="tbl-wrap"><Runs cycles={cycles} /></div>
          </div>
        </div>
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Agen</h2><span className="meta">peran · otonomi · hasil siklus terakhir</span></div>
            <div className="agent-grid">
              {agents.map((a) => (
                <div className="agent-card" key={a.name}>
                  <div className="ac-h"><span className="ni"><Icon name={a.icon} /></span><div><b>{a.name}</b><span>{a.role}</span></div></div>
                  <div className="ac-kv"><span><small>Otonom</small>{a.auto}</span><span><small>Butuh approve</small>{a.approve}</span><span><small>Tidak boleh</small>{a.never}</span></div>
                  <div className="ac-out"><span className="ai" />{a.output || 'Belum berjalan'}</div>
                  <div className="ac-f">
                    <div className="cal-row"><span>kalibrasi</span><div className="bar"><i style={{ width: `${a.confidence ?? 0}%`, ...(a.confidence !== null && a.confidence < 60 ? { background: 'var(--warn)' } : {}) }} /></div><span className="v num">{a.confidence === null ? '—' : `${a.confidence}%`}</span></div>
                    <button className="btn quiet" style={{ height: 26, fontSize: 12 }} disabled={st.running} onClick={() => reanalyze('agent:' + a.name)}>Jalankan ulang</button>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="card">
            <div className="card-h"><h2>MCP sebagai orchestrator</h2><Pill tone="neutral" icon="plug">Stage 07</Pill></div>
            <p style={{ fontSize: 13, color: 'var(--text-2)', lineHeight: 1.5, marginBottom: 10 }}>Klien MCP (Claude Desktop, ChatGPT, agent eksternal) tidak hanya membaca data: mereka bisa memicu analisis ulang, meminta rencana, dan menjalankan satu agen — lewat tool yang sama dengan yang dipakai Orchestrator internal.</p>
            <ul className="rules">
              <li><div><b>Klien MCP boleh memicu analisis ulang</b><span>orchestrator.reanalyze(scope) · hasil masuk antrean, bukan langsung ke dealer</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
              <li><div><b>Klien MCP boleh mengubah rencana hari ini</b><span>orchestrator.plan.update · setiap perubahan butuh approve Anda</span></div><button className="sw on" aria-label="toggle" onClick={later} /></li>
              <li><div><b>Klien MCP boleh mengirim ke dealer</b><span>Tidak pernah. Pengiriman hanya lewat tombol Setujui di aplikasi ini</span></div><Pill tone="neutral" icon="lock">Terkunci</Pill></li>
            </ul>
            <div className="hr" />
            <h3 className="h3">Panggilan terakhir dari klien MCP</h3>
            <ul className="mcp-log">
              {cycles.filter((c) => c.via === 'mcp').slice(0, 5).map((c) => (
                <li key={c.id}><span className="num t">{hhmm(c.started_at)}</span><div><b>{c.requested_by ?? 'Klien MCP'}</b><code>orchestrator.reanalyze(scope:"{c.scope}")</code><span>→ {c.note}</span></div></li>
              ))}
              {!cycles.some((c) => c.via === 'mcp') && <li><span className="num t">—</span><div><b>Belum ada panggilan</b><span>Server MCP (Streamable HTTP + OAuth) tersambung di Stage 07</span></div></li>}
            </ul>
            <div className="hr" />
            <h3 className="h3">Tool MCP</h3>
            <div className="tools">
              <div><small>Baca</small><code>dealer.list · dealer.get · segmen.list · jadwal.due · jadwal.lewat · kredit.check · stok.aging · chat.thread</code></div>
              <div><small>Analisis</small><code>analisis.dealer · analisis.segmen · analisis.kas · analisis.stok</code></div>
              <div><small>Orkestrasi</small><code>orchestrator.run · orchestrator.reanalyze(scope) · orchestrator.plan · orchestrator.agent.run(nama)</code></div>
              <div><small>Keputusan · human-only</small><code>actions.decide</code></div>
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
