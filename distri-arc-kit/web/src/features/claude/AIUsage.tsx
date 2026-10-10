import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { Pill } from '../../components/ui'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { hhmm, shortDate } from '../../lib/format'
import { useSchedules } from '../../app/queries'
import { MODE, RUN_COMPARE, TRIGGER, dur, hourly, modelLabel, rpAI, runFirstDir, runText, tok, type AIRun, type AIUsage as Usage, type RunColumn, type UsageTotals } from './usage'

const useAIUsage = () => useQuery({ queryKey: ['mcp', 'usage'], queryFn: () => api.get<Usage>('/mcp/usage'), refetchInterval: 30_000 })

const NONE: AIRun[] = []
const when = (d: string) => `${shortDate(d)} · ${hhmm(d)}`
const STATUS: Record<string, [string, 'good' | 'warn' | 'bad' | 'accent' | 'neutral']> = {
  done: ['Selesai', 'good'], ok: ['Selesai', 'good'], partial: ['Sebagian', 'warn'], template: ['Template', 'neutral'],
  failed: ['Gagal', 'bad'], error: ['Gagal', 'bad'], running: ['Berjalan', 'accent'], queued: ['Antre', 'accent'],
}

function Tile({ label, t }: { label: string; t: UsageTotals }) {
  return (
    <div className="au-tile">
      <small>{label}</small>
      <b>{rpAI(t.cost_idr)}</b>
      <span>{t.calls.toLocaleString('id-ID')} panggilan model AI · {tok(t.tokens_in)} masuk / {tok(t.tokens_out)} keluar{t.template_steps ? ` · ${t.template_steps.toLocaleString('id-ID')} langkah template (tanpa AI)` : ''}</span>
    </div>
  )
}

/** MCP Claude → Pemakaian AI: which model each analysis uses, what it costs, how often it runs and when it ran. */
export function AIUsage() {
  const { data } = useAIUsage()
  const { data: schedules = [] } = useSchedules()
  const [kind, setKind] = useState<'all' | 'cycle' | 'schedule' | 'mcp'>('all')
  const runs = data?.runs ?? NONE
  const shown = useMemo(() => (kind === 'all' ? runs : runs.filter((r) => r.kind === kind)), [runs, kind])
  const t = useDataTable<AIRun, RunColumn>({
    rows: shown,
    text: runText,
    compare: RUN_COMPARE,
    tie: RUN_COMPARE.started_at,
    initial: { column: 'started_at', dir: 'desc' },
    firstDir: runFirstDir,
  })
  if (!data) return null
  const o = data.orchestrator
  const a = data.analyst
  const m = data.mcp ?? { connections: [], calls_today: 0, sessions_30d: 0, last_at: null }
  const active = schedules.filter((s) => s.enabled)
  const budgetPct = a.daily_budget_idr > 0 ? Math.min(100, Math.round((a.spent_today_idr / a.daily_budget_idr) * 100)) : 0
  const price = (m: string) => data.prices[m]

  return (
    <div className="card au">
      <div className="card-h"><h2>Pemakaian AI</h2><span className="ai" style={{ marginLeft: 6 }}>model · biaya · jadwal · riwayat</span><span className="meta">diperbarui otomatis · biaya perkiraan (kurs Rp{Math.round(data.idr_per_usd).toLocaleString('id-ID')}/USD)</span></div>

      {(o.engine === 'template' || a.engine === 'template') && (
        <div className="au-note">
          <Icon name="alert" />
          <div>
            <b>{m.connections.length ? 'Claude terhubung lewat MCP · analisis otomatis di server masih mode template' : o.engine === 'template' && a.engine === 'template' ? 'AI belum aktif — sistem berjalan dalam mode template' : o.engine === 'template' ? 'Siklus Orchestrator berjalan dalam mode template' : 'Analisis terjadwal berjalan dalam mode template'}</b>
            <span>Tanpa API key Claude, tidak ada model AI yang dipanggil (di riwayat tertulis "Template (tanpa AI)", dulu "fake"). Ini <b>bukan data karangan</b>: semua angka, status dealer, jadwal order, limit, dan usulan dihitung oleh aturan agen dari data asli (BigQuery/Accurate, WhatsApp). Yang belum ada hanya kalimat alasan &amp; draft pesan yang ditulis ulang oleh Claude — sementara memakai kalimat template dari aturan yang sama. Untuk mengaktifkan Claude: isi <code>LLM_PROVIDER=anthropic</code> dan <code>ANTHROPIC_API_KEY</code> di server{a.engine === 'template' ? ', atau simpan kunci Claude di kartu Analisis terjadwal' : ''}.</span>
          </div>
        </div>
      )}

      <div className="au-engines">
        <div className="au-engine">
          <div className="au-eh"><span className="au-ic claude"><Icon name="plug" /></span><div><b>Claude lewat MCP</b><small>claude.ai · Desktop · Code membaca data GSI Orbit lewat tool MCP</small></div><span className={`au-live ${m.connections.length ? 'on' : ''}`}>{m.connections.length ? 'Terhubung' : 'Belum terhubung'}</span></div>
          <dl>
            <div><dt>Model</dt><dd>Claude di akun Anda <small>· dibayar langganan Claude, tanpa biaya API di server</small></dd></div>
            <div><dt>Koneksi</dt><dd>{m.connections.length ? m.connections.slice(0, 3).map((c, i) => <span key={c.name + c.user + i} className="au-sch">{c.name}{c.user && !c.name.includes(c.user) ? ` · ${c.user}` : ''}<small> · {c.calls_today} panggilan hari ini{c.last_seen_at ? ` · terakhir ${when(c.last_seen_at)}` : ''}</small></span>).concat(m.connections.length > 3 ? [<small key="more">+{m.connections.length - 3} koneksi lain · lihat kartu Koneksi</small>] : []) : <small>Tambahkan konektor di claude.ai → Settings → Connectors</small>}</dd></div>
            <div><dt>Hari ini</dt><dd>{m.calls_today.toLocaleString('id-ID')} panggilan tool <small>· {m.sessions_30d} sesi dalam 30 hari{m.last_at ? ` · terakhir ${when(m.last_at)}` : ''}</small></dd></div>
          </dl>
        </div>
        <div className="au-engine">
          <div className="au-eh"><span className="au-ic"><Icon name="spark" /></span><div><b>Siklus Orchestrator</b><small>6 agen menganalisis semua dealer · jalur {MODE[o.mode] ?? o.mode}</small></div></div>
          <dl>
            <div><dt>Model</dt><dd>{o.engine === 'claude' ? <>{modelLabel(o.model)} <small>· cadangan {o.fallback}</small></> : <>Template <small>· API key belum diisi, tanpa biaya</small></>}</dd></div>
            <div><dt>Setiap</dt><dd>{hourly(o.from_hour, o.to_hour)} <small>· + tombol Analisis ulang</small></dd></div>
            <div><dt>Berikutnya</dt><dd>{when(o.next_run_at)} WIB</dd></div>
            {o.engine === 'claude' && price(o.model) && <div><dt>Harga</dt><dd>${price(o.model).in_usd_per_mtok} / ${price(o.model).out_usd_per_mtok} <small>per 1 jt token masuk / keluar</small></dd></div>}
          </dl>
        </div>
        <div className="au-engine">
          <div className="au-eh"><span className="au-ic mcp"><Icon name="cal" /></span><div><b>Analisis terjadwal (MCP)</b><small>Claude membaca data lewat tool MCP sesuai jadwal</small></div></div>
          <dl>
            <div><dt>Model</dt><dd>{a.engine === 'claude' ? modelLabel(a.model) : <>Template <small>· kunci Claude belum diisi</small></>}</dd></div>
            <div><dt>Setiap</dt><dd>{active.length ? active.map((s) => <span key={s.id} className="au-sch">{s.name} · {s.description}{s.next_runs[0] ? <small> · berikutnya {when(s.next_runs[0])}</small> : null}</span>) : <small>Belum ada jadwal aktif</small>}</dd></div>
            <div><dt>Hari ini</dt><dd>{a.runs_today} analisis · {rpAI(a.spent_today_idr)}{a.daily_budget_idr > 0 ? <small> dari anggaran {rpAI(a.daily_budget_idr)}</small> : <small> · tanpa batas</small>}</dd></div>
            {a.engine === 'claude' && price(a.model) && <div><dt>Harga</dt><dd>${price(a.model).in_usd_per_mtok} / ${price(a.model).out_usd_per_mtok} <small>per 1 jt token masuk / keluar</small></dd></div>}
          </dl>
          {a.daily_budget_idr > 0 && <div className="au-budget" title={`${budgetPct}% anggaran harian terpakai`}><i style={{ width: `${budgetPct}%` }} className={budgetPct >= 90 ? 'bad' : budgetPct >= 70 ? 'warn' : ''} /></div>}
        </div>
      </div>

      <div className="au-tiles">
        <Tile label="Hari ini" t={data.cost.today} />
        <Tile label="7 hari" t={data.cost.d7} />
        <Tile label="30 hari" t={data.cost.d30} />
      </div>

      {data.cost.by_model.length > 0 && (
        <div className="odl-wrap">
          <table className="odl au-models">
            <thead><tr><th>Model (30 hari)</th><th className="r">Panggilan</th><th className="r">Token masuk</th><th className="r">Token keluar</th><th className="r">Biaya</th><th>Terakhir</th></tr></thead>
            <tbody>
              {data.cost.by_model.map((m) => (
                <tr key={m.provider + m.model}><td><b>{modelLabel(m.model)}</b> {m.provider !== 'fake' && m.provider !== 'template' ? <small className="muted">{m.provider}</small> : <small className="muted">tanpa biaya</small>}</td><td className="r num">{m.calls.toLocaleString('id-ID')}</td><td className="r num">{tok(m.tokens_in)}</td><td className="r num">{tok(m.tokens_out)}</td><td className="r num"><b>{rpAI(m.cost_idr)}</b></td><td>{when(m.last_at)}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="au-hist">
        <div className="au-hh">
          <h3>Riwayat analisis</h3>
          <div className="seg">
            {([['all', 'Semua'], ['mcp', 'Claude lewat MCP'], ['cycle', 'Siklus Orchestrator'], ['schedule', 'Analisis terjadwal']] as const).map(([k, l]) => (
              <button key={k} type="button" className={kind === k ? 'is-active' : ''} onClick={() => setKind(k)}>{l} <small>{k === 'all' ? runs.length : runs.filter((r) => r.kind === k).length}</small></button>
            ))}
          </div>
        </div>
        <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari analisis, model, pemicu…" label="Cari riwayat analisis" meta={`${t.matches.length} analisis`} />
        <div className="odl-wrap">
          <table className="odl">
            <thead>
              <tr>
                <SortTh t={t} c="started_at">Waktu</SortTh>
                <SortTh t={t} c="title">Analisis</SortTh>
                <SortTh t={t} c="model">Model</SortTh>
                <SortTh t={t} c="tokens" right>Token</SortTh>
                <SortTh t={t} c="cost_idr" right>Biaya</SortTh>
                <SortTh t={t} c="duration_ms" right>Durasi</SortTh>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {t.shown.length === 0 && <tr><td colSpan={7} className="muted">Belum ada analisis.</td></tr>}
              {t.shown.map((r) => (
                <tr key={r.kind + r.id}>
                  <td className="num">{when(r.started_at)}</td>
                  <td><b>{r.title}</b><small className="au-sub">{TRIGGER[r.trigger] ?? r.trigger}{r.by ? ` · ${r.by}` : ''}{r.kind === 'mcp' ? ` · ${r.calls} panggilan tool` : r.kind === 'schedule' ? ` · ${r.calls} langkah MCP` : r.calls ? ` · ${r.calls} panggilan model AI${r.template_steps ? ` + ${r.template_steps} template` : ''}` : ` · ${r.template_steps ?? 0} langkah template, tanpa AI`}</small></td>
                  <td>{modelLabel(r.model)}</td>
                  <td className="r num">{r.tokens_in + r.tokens_out ? `${tok(r.tokens_in)} / ${tok(r.tokens_out)}` : '—'}</td>
                  <td className="r num">{r.kind === 'mcp' ? <small className="muted" title="Dibayar langganan Claude, tanpa biaya API di server">langganan</small> : <b>{rpAI(r.cost_idr)}</b>}</td>
                  <td className="r num">{dur(r.duration_ms)}</td>
                  <td><Pill tone={STATUS[r.status]?.[1] ?? 'neutral'}>{STATUS[r.status]?.[0] ?? r.status}</Pill></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <TablePager t={t} noun="analisis" />
      </div>
    </div>
  )
}
