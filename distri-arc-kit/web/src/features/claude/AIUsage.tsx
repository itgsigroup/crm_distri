import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { Pill } from '../../components/ui'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { hhmm, shortDate } from '../../lib/format'
import { useSchedules } from '../../app/queries'
import { MODE, RUN_COMPARE, TRIGGER, dur, hourly, modelLabel, rpAI, runFirstDir, runText, tok, type AIRun, type AIUsage as Usage, type RunColumn, type UsageTotals } from './usage'

const useAIUsage = () => useQuery({ queryKey: ['mcp', 'usage'], queryFn: () => api.get<Usage>('/mcp/usage'), refetchInterval: 60_000 })

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
      <span>{t.calls.toLocaleString('id-ID')} panggilan model · {tok(t.tokens_in)} masuk / {tok(t.tokens_out)} keluar</span>
    </div>
  )
}

/** MCP Claude → Pemakaian AI: which model each analysis uses, what it costs, how often it runs and when it ran. */
export function AIUsage() {
  const { data } = useAIUsage()
  const { data: schedules = [] } = useSchedules()
  const [kind, setKind] = useState<'all' | 'cycle' | 'schedule'>('all')
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
  const active = schedules.filter((s) => s.enabled)
  const budgetPct = a.daily_budget_idr > 0 ? Math.min(100, Math.round((a.spent_today_idr / a.daily_budget_idr) * 100)) : 0
  const price = (m: string) => data.prices[m]

  return (
    <div className="card au">
      <div className="card-h"><h2>Pemakaian AI</h2><span className="ai" style={{ marginLeft: 6 }}>model · biaya · jadwal · riwayat</span><span className="meta">diperbarui tiap menit · biaya perkiraan (kurs Rp{Math.round(data.idr_per_usd).toLocaleString('id-ID')}/USD)</span></div>

      <div className="au-engines">
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
                <tr key={m.provider + m.model}><td><b>{modelLabel(m.model)}</b> <small className="muted">{m.provider}</small></td><td className="r num">{m.calls.toLocaleString('id-ID')}</td><td className="r num">{tok(m.tokens_in)}</td><td className="r num">{tok(m.tokens_out)}</td><td className="r num"><b>{rpAI(m.cost_idr)}</b></td><td>{when(m.last_at)}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="au-hist">
        <div className="au-hh">
          <h3>Riwayat analisis</h3>
          <div className="seg">
            {([['all', 'Semua'], ['cycle', 'Siklus Orchestrator'], ['schedule', 'Analisis terjadwal']] as const).map(([k, l]) => (
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
                  <td><b>{r.title}</b><small className="au-sub">{TRIGGER[r.trigger] ?? r.trigger}{r.by ? ` · ${r.by}` : ''}{r.kind === 'cycle' ? ` · ${r.calls} panggilan model` : ` · ${r.calls} langkah MCP`}</small></td>
                  <td>{modelLabel(r.model)}</td>
                  <td className="r num">{r.tokens_in + r.tokens_out ? `${tok(r.tokens_in)} / ${tok(r.tokens_out)}` : '—'}</td>
                  <td className="r num"><b>{rpAI(r.cost_idr)}</b></td>
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
