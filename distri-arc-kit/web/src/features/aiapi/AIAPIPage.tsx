import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { modelLabel, rpAI, tok } from '../claude/usage'
import { PROVIDER_LABEL, byProvider, dailyBars, fallbackText, waitLabel, type AIAPIInfo, type AIProvider, type Fallback } from './aiapi'

const useAIAPI = () => useQuery({ queryKey: ['ai', 'api'], queryFn: () => api.get<AIAPIInfo>('/ai/api'), refetchInterval: 30_000 })

const COLOR = { claude: 'var(--accent)', openai: 'var(--good)', other: 'var(--text-3)' }
const KEY_HELP: Record<string, [string, string]> = {
  claude: ['sk-ant-api03-…', 'console.anthropic.com → API keys'],
  openai: ['sk-proj-…', 'platform.openai.com → API keys'],
}
const when = (d: string) => `${shortDate(d)} · ${hhmm(d)}`
const todayWIB = () => new Date(Date.now() + 7 * 3600_000).toISOString().slice(0, 10)

function useSave() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  return (msg?: string) => ({
    onSuccess: (r: unknown) => {
      const m = (r as { message?: string } | undefined)?.message ?? msg
      if (m) toast(m)
      qc.invalidateQueries({ queryKey: ['ai'] })
      qc.invalidateQueries({ queryKey: ['mcp'] })
    },
    onError: (e: Error) => toast(e.message),
  })
}

/** Urutan analisis: Claude lewat MCP first, then the API chosen here. */
function OrderCard({ info }: { info: AIAPIInfo }) {
  const done = useSave()
  const set = useMutation({ mutationFn: (fallback: Fallback) => api.put('/ai/api', { fallback }), ...done('Opsi kedua disimpan') })
  const budget = useMutation({ mutationFn: (v: number) => api.put('/ai/api', { daily_budget_idr: v }), ...done('Anggaran harian disimpan') })
  const pct = info.daily_budget_idr > 0 ? Math.min(100, Math.round((info.spent_today_idr / info.daily_budget_idr) * 100)) : 0
  return (
    <div className="card">
      <div className="card-h"><h2>Urutan analisis</h2><span className="meta">siklus Orchestrator · Analisa Stok/Kas · Analisa WhatsApp</span></div>
      <ol className="aa-order">
        <li>
          <span className="aa-step">1</span>
          <div><b>Claude lewat MCP</b><small>claude.ai · Desktop · routine — dibayar langganan Claude, tanpa biaya token di server. Ditunggu {waitLabel(info.mcp_wait_sec)} per siklus.</small></div>
          <Pill tone="good" icon="check">Utama</Pill>
        </li>
        <li>
          <span className="aa-step">2</span>
          <div>
            <b>API di server (opsi kedua)</b>
            <small>{fallbackText(info)} Analisa WhatsApp juga dikerjakan API bila routine claude.ai terlambat lebih dari 2½ jam.</small>
          </div>
          <Pill tone={info.ready ? 'good' : info.fallback === 'off' ? 'neutral' : 'warn'} icon={info.ready ? 'check' : 'alert'}>{info.ready ? 'Siap' : info.fallback === 'off' ? 'Mati' : 'Belum aktif'}</Pill>
        </li>
      </ol>
      <div className="aa-row">
        <span className="aa-lbl">Opsi kedua</span>
        <div className="seg">
          {(['off', 'claude', 'openai'] as const).map((f) => (
            <button key={f} disabled={!info.can_configure || set.isPending} className={info.fallback === f ? 'is-active' : ''} onClick={() => set.mutate(f)}>{f === 'off' ? 'Mati' : PROVIDER_LABEL[f]}</button>
          ))}
        </div>
      </div>
      <div className="aa-row">
        <span className="aa-lbl">Anggaran harian</span>
        <select className="cl-input" disabled={!info.can_configure} value={info.daily_budget_idr} onChange={(e) => budget.mutate(Number(e.target.value))} aria-label="Anggaran harian">
          {[10000, 25000, 50000, 100000, 250000, 500000, 0].map((v) => <option key={v} value={v}>{v ? rpAI(v) + ' / hari' : 'Tanpa batas'}</option>)}
        </select>
        <small className="muted">hari ini {rpAI(info.spent_today_idr)}{info.daily_budget_idr > 0 ? ` · ${pct}%` : ''} · lewat anggaran, API berhenti sampai besok (kedua API dihitung bersama)</small>
      </div>
      {info.daily_budget_idr > 0 && <div className="au-budget"><i style={{ width: `${pct}%` }} className={pct >= 90 ? 'bad' : pct >= 70 ? 'warn' : ''} /></div>}
      <p className="aa-foot"><Icon name="lock" />API hanya mengusulkan lewat tool MCP yang sama dengan Claude — tidak pernah memutuskan atau mengirim ke dealer.</p>
    </div>
  )
}

/** One API: key (checked, sealed, shown only as a hint) and model. */
function ProviderCard({ p, info }: { p: AIProvider; info: AIAPIInfo }) {
  const [key, setKey] = useState('')
  const done = useSave()
  const save = useMutation({ mutationFn: () => api.put<{ message: string }>(`/ai/api/key/${p.id}`, { api_key: key.trim() }), ...done(), onSettled: () => setKey('') })
  const del = useMutation({ mutationFn: () => api.del(`/ai/api/key/${p.id}`), ...done(`Kunci ${p.label} dihapus`) })
  const model = useMutation({ mutationFn: (m: string) => api.put('/ai/api', p.id === 'claude' ? { model: m } : { openai_model: m }), ...done('Model disimpan') })
  const price = info.prices[p.model]
  const chosen = info.fallback === p.id
  const [ph, where] = KEY_HELP[p.id]
  const okPrefix = p.id === 'claude' ? key.trim().startsWith('sk-ant-') : key.trim().startsWith('sk-') && !key.trim().startsWith('sk-ant-')
  return (
    <div className="card aa-prov">
      <div className="card-h">
        <span className="aa-dot" style={{ background: COLOR[p.id] }} />
        <h2>{p.label}</h2>
        {chosen && <Pill tone="accent">opsi kedua</Pill>}
        <span className="meta">{p.key_source ? <>kunci <code>{p.key_hint}</code>{p.key_source === 'env' ? ' · dari server' : ''}</> : 'kunci belum diisi'}</span>
      </div>
      {info.can_configure && (
        <label className="aa-field">Kunci API
          {p.key_source === 'ui' ? (
            <span className="aa-keyset"><code>{p.key_hint}</code> tersimpan terenkripsi
              <button className="btn quiet" disabled={del.isPending} onClick={() => { if (window.confirm(`Hapus kunci ${p.label}?`)) del.mutate() }}>Hapus</button></span>
          ) : (
            <span className="aa-keyset">
              <input className="cl-input" type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder={ph} aria-label={`Kunci ${p.label}`} />
              <button className="btn primary" disabled={!okPrefix || save.isPending} onClick={() => save.mutate()}><Icon name="lock" />Simpan</button>
            </span>
          )}
          <small>Buat di {where} (berbayar per pemakaian). Kunci diperiksa ke provider, disimpan terenkripsi, tidak pernah ditampilkan lagi.</small>
        </label>
      )}
      <label className="aa-field">Model
        <select className="cl-input" disabled={!info.can_configure || model.isPending} value={p.model} onChange={(e) => model.mutate(e.target.value)} aria-label={`Model ${p.label}`}>
          {p.models.map((m) => <option key={m.id} value={m.id}>{m.label}</option>)}
        </select>
        {price && <small>${price.in_usd_per_mtok} / ${price.out_usd_per_mtok} per 1 jt token masuk / keluar · perkiraan kurs Rp{Math.round(info.idr_per_usd).toLocaleString('id-ID')}/USD</small>}
      </label>
    </div>
  )
}

/** Total pembayaran token: totals, per provider, per day, per model. */
function CostCard({ info }: { info: AIAPIInfo }) {
  const t = info.usage.totals
  const prov = byProvider(info.usage.by_model)
  const bars = dailyBars(info.usage.daily, todayWIB(), 30)
  const max = Math.max(0, ...bars.map((b) => b.total))
  const tiles: [string, keyof typeof t][] = [['Hari ini', 'today'], ['7 hari', 'd7'], ['Bulan ini', 'month'], ['30 hari', 'd30']]
  return (
    <div className="card au">
      <div className="card-h"><h2>Total pembayaran token</h2><span className="meta">Claude API + ChatGPT API · biaya perkiraan dari token × harga model</span></div>
      <div className="au-tiles aa-tiles">
        {tiles.map(([l, k]) => (
          <div key={k} className="au-tile">
            <small>{l}</small>
            <b>{rpAI(t[k].cost_idr)}</b>
            <span>{t[k].calls.toLocaleString('id-ID')} panggilan · {tok(t[k].tokens_in)} masuk / {tok(t[k].tokens_out)} keluar</span>
          </div>
        ))}
      </div>
      <div className="aa-split">
        {prov.length === 0 && <small className="muted">Belum ada pemakaian API dalam 30 hari. Pemakaian Claude lewat MCP dibayar langganan, tidak dihitung di sini.</small>}
        {prov.map((p) => (
          <div key={p.provider} className="aa-split-row">
            <span className="aa-dot" style={{ background: COLOR[p.provider] }} />
            <b>{PROVIDER_LABEL[p.provider]}</b>
            <span className="muted">{p.calls.toLocaleString('id-ID')} panggilan · {tok(p.tokens_in)} / {tok(p.tokens_out)} token</span>
            <b className="num">{rpAI(p.cost_idr)}</b>
          </div>
        ))}
      </div>
      {max > 0 && <>
      <div className="aa-chart" role="img" aria-label="Biaya API per hari, 30 hari terakhir">
        {bars.map((b) => (
          <div key={b.day} className="aa-bar" title={`${b.day} · ${rpAI(b.total)}${b.claude ? ` · Claude ${rpAI(b.claude)}` : ''}${b.openai ? ` · ChatGPT ${rpAI(b.openai)}` : ''}`}>
            {(['other', 'openai', 'claude'] as const).map((k) => b[k] > 0 && <i key={k} style={{ height: `${(b[k] / max) * 100}%`, background: COLOR[k] }} />)}
          </div>
        ))}
      </div>
      <div className="aa-axis"><span>{shortDate(bars[0].day)}</span><span>maks {rpAI(max)} / hari</span><span>hari ini</span></div>
      </>}
      {info.usage.by_model.length > 0 && (
        <div className="odl-wrap">
          <table className="odl au-models">
            <thead><tr><th>Model (30 hari)</th><th className="r">Panggilan</th><th className="r">Token masuk</th><th className="r">Token keluar</th><th className="r">Biaya</th><th>Terakhir</th></tr></thead>
            <tbody>
              {info.usage.by_model.map((m) => (
                <tr key={m.provider + m.model}><td><b>{modelLabel(m.model)}</b> <small className="muted">{m.provider}</small></td><td className="r num">{m.calls.toLocaleString('id-ID')}</td><td className="r num">{tok(m.tokens_in)}</td><td className="r num">{tok(m.tokens_out)}</td><td className="r num"><b>{rpAI(m.cost_idr)}</b></td><td>{when(m.last_at)}</td></tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

const RUN_STATUS: Record<string, [string, 'good' | 'warn' | 'bad' | 'accent' | 'neutral']> = {
  ok: ['Berhasil', 'good'], running: ['Berjalan', 'accent'], error: ['Gagal', 'bad'], template: ['Template', 'neutral'],
}

/** The runs of the second option: when the API stepped in, for what, with what result and cost. */
function RunsCard({ info }: { info: AIAPIInfo }) {
  return (
    <div className="card">
      <div className="card-h"><h2>Riwayat opsi kedua</h2><span className="meta">saat API mengambil alih dari Claude lewat MCP</span></div>
      {info.runs.length === 0 ? <small className="muted">Belum pernah — Claude lewat MCP menjawab semua analisis, atau opsi kedua mati.</small> : (
        <div className="odl-wrap">
          <table className="odl">
            <thead><tr><th>Waktu</th><th>Analisis</th><th>Model</th><th>Hasil</th><th className="r">Token</th><th className="r">Biaya</th></tr></thead>
            <tbody>
              {info.runs.map((r) => {
                const [label, tone] = RUN_STATUS[r.status] ?? [r.status, 'neutral']
                return (
                  <tr key={r.id}>
                    <td className="num">{when(r.started_at)}</td>
                    <td>{r.schedule.replace('API cadangan · ', '')}{r.error && <small className="au-sub">{r.error}</small>}</td>
                    <td>{r.model ? modelLabel(r.model) : '—'}</td>
                    <td><Pill tone={tone}>{label}</Pill></td>
                    <td className="r num">{r.tokens_in + r.tokens_out ? `${tok(r.tokens_in)} / ${tok(r.tokens_out)}` : '—'}</td>
                    <td className="r num"><b>{rpAI(r.cost_idr)}</b></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

/** Pengaturan → API & model AI (ADR 0040). */
export function AIAPIPage() {
  const { data: info, error } = useAIAPI()
  if (error) return <div className="stack"><div className="card"><p className="muted">{(error as Error).message}</p></div></div>
  if (!info) return <div className="stack"><div className="card"><p className="muted">Memuat…</p></div></div>
  return (
    <div className="stack aa">
      {!info.can_configure && <div className="au-note"><Icon name="lock" /><div><b>Hanya CEO yang bisa mengubah kunci, model, dan opsi kedua</b><span>Anda bisa melihat status dan biaya.</span></div></div>}
      <OrderCard info={info} />
      <div className="aa-provs">{info.providers.map((p) => <ProviderCard key={p.id} p={p} info={info} />)}</div>
      <CostCard info={info} />
      <RunsCard info={info} />
    </div>
  )
}
