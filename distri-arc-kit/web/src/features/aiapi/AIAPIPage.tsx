import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { modelLabel, tok } from '../claude/usage'
import { BUDGET_USD, PROVIDER_LABEL, autoText, byProvider, dailyBars, fallbackText, modelOption, price, usd, type AIAPIInfo, type AIProvider, type Fallback, type FallbackRun, type ProviderID, type WAAuto } from './aiapi'

const useAIAPI = () => useQuery({ queryKey: ['ai', 'api'], queryFn: () => api.get<AIAPIInfo>('/ai/api'), refetchInterval: 30_000 })

const COLOR = { claude: 'var(--accent)', openai: 'var(--good)', other: 'var(--text-3)' }
const KEY_PH: Record<string, string> = { claude: 'sk-ant-api03-…', openai: 'sk-proj-…' }
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

/** One API on one line: key (hint or a field to fill) and the model, every model listed with its price. */
function ProviderRow({ p, edit }: { p: AIProvider; edit: boolean }) {
  const [open, setOpen] = useState(false)
  const [key, setKey] = useState('')
  const done = useSave()
  const save = useMutation({ mutationFn: () => api.put<{ message: string }>(`/ai/api/key/${p.id}`, { api_key: key.trim() }), ...done(), onSettled: () => { setKey(''); setOpen(false) } })
  const del = useMutation({ mutationFn: () => api.del(`/ai/api/key/${p.id}`), ...done(`Kunci ${p.label} dihapus`) })
  const model = useMutation({ mutationFn: (m: string) => api.put('/ai/api', p.id === 'claude' ? { model: m } : { openai_model: m }), ...done('Model disimpan') })
  const k = key.trim()
  const okPrefix = p.id === 'claude' ? k.startsWith('sk-ant-') : k.startsWith('sk-') && !k.startsWith('sk-ant-')
  const models = p.models.some((m) => m.id === p.model) ? p.models : [{ id: p.model, label: p.model, in_usd: null, out_usd: null, live: false }, ...p.models]
  return (
    <div className="aa-prow">
      <span className="aa-dot" style={{ background: COLOR[p.id] }} />
      <b className="aa-pname">{p.label}</b>
      {open ? (
        <span className="aa-keyset">
          <input className="cl-input" type="password" autoComplete="off" autoFocus value={key} onChange={(e) => setKey(e.target.value)} placeholder={KEY_PH[p.id]} aria-label={`Kunci ${p.label}`} />
          <button className="btn primary" disabled={!okPrefix || save.isPending} onClick={() => save.mutate()}><Icon name="lock" />Simpan</button>
          {p.key_source === 'ui' && <button className="btn quiet" disabled={del.isPending} onClick={() => { if (window.confirm(`Hapus kunci ${p.label}?`)) { del.mutate(); setOpen(false) } }}>Hapus</button>}
          <button className="btn quiet" onClick={() => { setOpen(false); setKey('') }}>Batal</button>
        </span>
      ) : (
        <span className="aa-key">
          {p.key_source ? <><Icon name="check" /><code>{p.key_hint}</code>{p.key_source === 'env' ? <small>dari server</small> : null}</> : <small>belum ada kunci</small>}
          {edit && <button className="btn quiet" onClick={() => setOpen(true)}>{p.key_source ? 'Ganti' : 'Isi kunci'}</button>}
        </span>
      )}
      <select className="cl-input aa-model" disabled={!edit || model.isPending} value={p.model} onChange={(e) => model.mutate(e.target.value)} aria-label={`Model ${p.label}`}>
        {models.map((m) => <option key={m.id} value={m.id}>{modelOption(m)}</option>)}
      </select>
    </div>
  )
}

/** Every model of both APIs with its price per million tokens (input / output). */
function ModelTable({ providers }: { providers: AIProvider[] }) {
  const n = providers.reduce((a, p) => a + p.models.length, 0)
  return (
    <details className="aa-all">
      <summary>Semua model &amp; harga ({n}) · USD per 1 juta token</summary>
      <div className="aa-all-grid">
        {providers.map((p) => (
          <table key={p.id} className="aa-mt">
            <thead><tr><th>{p.label}</th><th className="r">Input</th><th className="r">Output</th></tr></thead>
            <tbody>
              {p.models.map((m) => (
                <tr key={m.id} className={m.id === p.model ? 'is-on' : ''}>
                  <td>{m.label || m.id}{m.id === p.model && <small className="aa-on"> · dipakai</small>}</td>
                  <td className="r num">{price(m.in_usd)}</td>
                  <td className="r num">{price(m.out_usd)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ))}
      </div>
      <small className="muted">Daftar bawaan GSI Orbit, ditambah model yang tersedia untuk kunci Anda (diambil dari Anthropic / OpenAI). Harga "?" belum diketahui — lihat halaman harga provider.</small>
    </details>
  )
}

/** Model API: the second option, both keys and models, the daily budget — one card. */
function ModelCard({ info }: { info: AIAPIInfo }) {
  const done = useSave()
  const set = useMutation({ mutationFn: (fallback: Fallback) => api.put('/ai/api', { fallback }), ...done('Opsi kedua disimpan') })
  const budget = useMutation({ mutationFn: (v: number) => api.put('/ai/api', { daily_budget_idr: v }), ...done('Anggaran harian disimpan') })
  const rate = info.idr_per_usd
  const cur = BUDGET_USD.find((u) => Math.round(u * rate) === info.daily_budget_idr)
  const edit = info.can_configure
  return (
    <div className="card">
      <div className="card-h"><h2>Model API</h2><span className="meta">Claude lewat MCP tetap utama · API dipakai bila Claude tidak menjawab</span></div>
      <div className="aa-row">
        <span className="aa-lbl">Opsi kedua</span>
        <div className="seg">
          {(['off', 'claude', 'openai'] as const).map((f) => (
            <button key={f} disabled={!edit || set.isPending} className={info.fallback === f ? 'is-active' : ''} onClick={() => set.mutate(f)}>{f === 'off' ? 'Mati' : PROVIDER_LABEL[f]}</button>
          ))}
        </div>
        <Pill tone={info.ready ? 'good' : info.fallback === 'off' ? 'neutral' : 'warn'}>{info.ready ? 'Siap' : info.fallback === 'off' ? 'Mati' : 'Belum aktif'}</Pill>
      </div>
      <p className="aa-hint">{fallbackText(info)}</p>
      <div className="aa-provs">{info.providers.map((p) => <ProviderRow key={p.id} p={p} edit={edit} />)}</div>
      <div className="aa-row">
        <span className="aa-lbl">Anggaran / hari</span>
        <select className="cl-input" disabled={!edit} value={cur ?? 'x'} onChange={(e) => budget.mutate(Math.round(Number(e.target.value) * rate))} aria-label="Anggaran harian">
          {cur === undefined && <option value="x">{usd(info.daily_budget_idr, rate)}</option>}
          {BUDGET_USD.map((v) => <option key={v} value={v}>{v ? `$${v}` : 'Tanpa batas'}</option>)}
        </select>
        <small className="muted">terpakai hari ini {usd(info.spent_today_idr, rate)}</small>
      </div>
      <ModelTable providers={info.providers} />
    </div>
  )
}

const RUN_STATUS: Record<string, [string, 'good' | 'warn' | 'bad' | 'accent' | 'neutral']> = {
  ok: ['Berhasil', 'good'], running: ['Berjalan', 'accent'], error: ['Gagal', 'bad'], template: ['Template', 'neutral'],
}

const HOURS = Array.from({ length: 24 }, (_, h) => h)

/** Analisa WhatsApp otomatis (ADR 0043): on/off, the hours and the API + model; a failed run is not retried with the
 * other API. Edited as a draft, saved with one button. */
function AutoCard({ info }: { info: AIAPIInfo }) {
  const saved = info.wa_auto
  const [d, setD] = useState<WAAuto>(saved)
  const done = useSave()
  const save = useMutation({ mutationFn: (a: WAAuto) => api.put('/ai/api/wa-auto', { enabled: a.enabled, hours: a.hours, weekend: a.weekend, engine: a.engine, model: a.model }), ...done('Analisa WhatsApp otomatis disimpan') })
  const edit = info.can_configure
  const prov = info.providers.find((p) => p.id === d.engine)
  const models = prov ? (prov.models.some((m) => m.id === d.model) ? prov.models : [{ id: d.model, label: d.model, in_usd: null, out_usd: null, live: false }, ...prov.models]) : []
  const pickEngine = (e: ProviderID) => setD({ ...d, engine: e, model: info.providers.find((p) => p.id === e)?.model ?? d.model })
  const toggleHour = (h: number) => setD({ ...d, hours: d.hours.includes(h) ? d.hours.filter((x) => x !== h) : [...d.hours, h].sort((a, b) => a - b) })
  const dirty = JSON.stringify([d.enabled, [...d.hours].sort((a, b) => a - b), d.weekend, d.engine, d.model]) !== JSON.stringify([saved.enabled, [...saved.hours].sort((a, b) => a - b), saved.weekend, saved.engine, saved.model])
  const runs: FallbackRun[] = info.wa_auto_runs ?? []
  return (
    <div className="card">
      <div className="card-h"><h2>Analisa WhatsApp otomatis</h2><span className="meta">server menjalankan di jam pilihan · tanpa cadangan AI lain</span></div>
      <div className="aa-row">
        <span className="aa-lbl">Status</span>
        <div className="seg">
          <button disabled={!edit} className={!d.enabled ? 'is-active' : ''} onClick={() => setD({ ...d, enabled: false })}>Mati</button>
          <button disabled={!edit} className={d.enabled ? 'is-active' : ''} onClick={() => setD({ ...d, enabled: true })}>Aktif</button>
        </div>
        {saved.enabled && saved.next_run_at && <small className="muted">berikutnya {when(saved.next_run_at)} WIB</small>}
      </div>
      <div className="aa-row">
        <span className="aa-lbl">Pakai AI</span>
        <div className="seg">
          {(['claude', 'openai'] as const).map((e) => (
            <button key={e} disabled={!edit} className={d.engine === e ? 'is-active' : ''} onClick={() => pickEngine(e)}>{PROVIDER_LABEL[e]}</button>
          ))}
        </div>
        <select className="cl-input aa-model" disabled={!edit} value={d.model} onChange={(e) => setD({ ...d, model: e.target.value })} aria-label="Model analisa otomatis">
          {models.map((m) => <option key={m.id} value={m.id}>{modelOption(m)}</option>)}
        </select>
      </div>
      <div className="aa-row aa-row-top">
        <span className="aa-lbl">Jam (WIB)</span>
        <div className="aa-hours" role="group" aria-label="Jam analisa otomatis">
          {HOURS.map((h) => (
            <button key={h} disabled={!edit} className={d.hours.includes(h) ? 'is-on' : ''} aria-pressed={d.hours.includes(h)} onClick={() => toggleHour(h)}>{String(h).padStart(2, '0')}</button>
          ))}
        </div>
      </div>
      <div className="aa-row">
        <span className="aa-lbl">Hari</span>
        <label className="aa-check"><input type="checkbox" disabled={!edit} checked={d.weekend} onChange={(e) => setD({ ...d, weekend: e.target.checked })} />Juga hari Minggu <small className="muted">(selain Senin–Sabtu)</small></label>
      </div>
      <p className="aa-hint">{autoText(d, info.providers)}</p>
      {edit && (
        <div className="aa-row">
          <button className="btn primary" disabled={!dirty || d.hours.length === 0 || save.isPending} onClick={() => save.mutate(d)}><Icon name="check" />Simpan</button>
          {dirty && <button className="btn quiet" onClick={() => setD(saved)}>Batal</button>}
        </div>
      )}
      {runs.length > 0 && (
        <div className="odl-wrap aa-auto-runs">
          <table className="odl">
            <thead><tr><th>Waktu</th><th>Model</th><th>Hasil</th><th className="r">Biaya</th></tr></thead>
            <tbody>
              {runs.slice(0, 8).map((r) => {
                const [label, tone] = RUN_STATUS[r.status] ?? [r.status, 'neutral']
                return (
                  <tr key={r.id}>
                    <td className="num">{when(r.started_at)}{r.error && <small className="au-sub">{r.error}</small>}</td>
                    <td>{r.model ? modelLabel(r.model) : '—'}</td>
                    <td><Pill tone={tone}>{label}</Pill></td>
                    <td className="r num"><b>{usd(r.cost_idr, info.idr_per_usd)}</b></td>
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

/** Biaya token in dollars: totals, per API, per day. */
function CostCard({ info }: { info: AIAPIInfo }) {
  const t = info.usage.totals
  const rate = info.idr_per_usd
  const prov = byProvider(info.usage.by_model)
  const bars = dailyBars(info.usage.daily, todayWIB(), 30)
  const max = Math.max(0, ...bars.map((b) => b.total))
  const tiles: [string, keyof typeof t][] = [['Hari ini', 'today'], ['7 hari', 'd7'], ['Bulan ini', 'month'], ['30 hari', 'd30']]
  return (
    <div className="card au">
      <div className="card-h"><h2>Biaya token</h2><span className="meta">perkiraan · token × harga model</span></div>
      <div className="au-tiles aa-tiles">
        {tiles.map(([l, k]) => (
          <div key={k} className="au-tile">
            <small>{l}</small>
            <b>{usd(t[k].cost_idr, rate)}</b>
            <span>{tok(t[k].tokens_in + t[k].tokens_out)} token</span>
          </div>
        ))}
      </div>
      <div className="aa-split">
        {prov.length === 0 && <small className="muted">Belum ada pemakaian API dalam 30 hari. Claude lewat MCP dibayar langganan, tidak dihitung di sini.</small>}
        {prov.map((p) => (
          <div key={p.provider} className="aa-split-row">
            <span className="aa-dot" style={{ background: COLOR[p.provider] }} />
            <b>{PROVIDER_LABEL[p.provider]}</b>
            <span className="muted">30 hari · {tok(p.tokens_in)} masuk / {tok(p.tokens_out)} keluar</span>
            <b className="num">{usd(p.cost_idr, rate)}</b>
          </div>
        ))}
      </div>
      {max > 0 && <>
        <div className="aa-chart" role="img" aria-label="Biaya API per hari, 30 hari terakhir">
          {bars.map((b) => (
            <div key={b.day} className="aa-bar" title={`${b.day} · ${usd(b.total, rate)}`}>
              {(['other', 'openai', 'claude'] as const).map((k) => b[k] > 0 && <i key={k} style={{ height: `${(b[k] / max) * 100}%`, background: COLOR[k] }} />)}
            </div>
          ))}
        </div>
        <div className="aa-axis"><span>{shortDate(bars[0].day)}</span><span>tertinggi {usd(max, rate)} / hari</span><span>hari ini</span></div>
      </>}
    </div>
  )
}

/** When the API took over from Claude through MCP (shown once it has). */
function RunsCard({ info }: { info: AIAPIInfo }) {
  return (
    <div className="card">
      <div className="card-h"><h2>Saat API mengambil alih</h2></div>
      <div className="odl-wrap">
        <table className="odl">
          <thead><tr><th>Waktu</th><th>Analisis</th><th>Model</th><th>Hasil</th><th className="r">Biaya</th></tr></thead>
          <tbody>
            {info.runs.map((r) => {
              const [label, tone] = RUN_STATUS[r.status] ?? [r.status, 'neutral']
              return (
                <tr key={r.id}>
                  <td className="num">{when(r.started_at)}</td>
                  <td>{r.schedule.replace('API cadangan · ', '')}{r.error && <small className="au-sub">{r.error}</small>}</td>
                  <td>{r.model ? modelLabel(r.model) : '—'}</td>
                  <td><Pill tone={tone}>{label}</Pill></td>
                  <td className="r num"><b>{usd(r.cost_idr, info.idr_per_usd)}</b></td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
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
      <ModelCard info={info} />
      <AutoCard key={JSON.stringify(info.wa_auto)} info={info} />
      <CostCard info={info} />
      {info.runs.length > 0 && <RunsCard info={info} />}
      {!info.can_configure && <small className="muted"><Icon name="lock" /> Hanya CEO yang bisa mengubah kunci, model, opsi kedua, dan analisa otomatis.</small>}
    </div>
  )
}
