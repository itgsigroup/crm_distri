import { useCallback, useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { AnalystInfo, RunStatus, Schedule } from '../../api/types'
import { Icon } from '../../components/Icon'
import { Markdown } from '../../components/Markdown'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill, type Tone } from '../../components/ui'
import { hhmm, shortDate, wib } from '../../lib/format'
import { useAnalyst, useCronPreview, useMe, useScheduleRun, useScheduleRuns, useSchedules } from '../../app/queries'
import { PROMPTS, SCOPE } from './shared'

// Scheduled analysis (ADR 0022): Claude analyses through the MCP tools on a cron schedule (WIB) and keeps the report.

const PRESETS: [string, string][] = [
  ['Setiap pagi Sen–Sab', '0 7 * * 1-6'],
  ['Setiap hari 07.00', '0 7 * * *'],
  ['Pagi & sore', '0 7,16 * * 1-6'],
  ['Tiap 2 jam kerja', '0 8-18/2 * * 1-6'],
  ['Mingguan Senin', '0 8 * * 1'],
  ['Bulanan tgl 1', '0 8 1 * *'],
]

const DAYS = ['Min', 'Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab']
const when = (t: string) => `${DAYS[wib(t).wd]} ${shortDate(t)} ${hhmm(t)}`
const rpSmall = (v: number) => 'Rp' + Math.round(v).toLocaleString('id-ID')

const STATUS: Record<RunStatus, [string, Tone]> = {
  running: ['berjalan', 'accent'],
  ok: ['Claude', 'good'],
  template: ['template', 'neutral'],
  error: ['gagal', 'bad'],
}

// rough rupiah per run (12 tool calls): Opus 5.5 ≈ $0.30, Sonnet ≈ half, Haiku ≈ a quarter (IDR 16.500/USD)
const PER_RUN: Record<string, number> = { 'claude-opus-5-5': 5000, 'claude-sonnet-5-5': 2500, 'claude-haiku-4-5': 1250 }
const estimate = (model: string, steps: number) => Math.round(((PER_RUN[model] ?? 5000) * Math.max(steps, 4)) / 12 / 100) * 100

function Engine({ info }: { info: AnalystInfo }) {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [open, setOpen] = useState(false)
  const [key, setKey] = useState('')
  const refresh = () => qc.invalidateQueries({ queryKey: ['mcp', 'analyst'] })
  const saveKey = useMutation({
    mutationFn: () => api.put<{ message: string }>('/mcp/analyst/key', { api_key: key }),
    onSuccess: (r) => { toast(r.message); setKey(''); refresh() },
    onError: (e: Error) => toast(e.message),
  })
  const delKey = useMutation({
    mutationFn: () => api.del('/mcp/analyst/key'),
    onSuccess: () => { toast('Kunci dihapus · analisis kembali ke mode template'); refresh() },
    onError: (e: Error) => toast(e.message),
  })
  const cfg = useMutation({
    mutationFn: (c: { model: string; daily_budget_idr: number }) => api.put('/mcp/analyst', c),
    onSuccess: () => { toast('Pengaturan analisis disimpan'); refresh() },
    onError: (e: Error) => toast(e.message),
  })
  const model = info.models.find((m) => m.id === info.model)
  const claude = info.engine === 'claude'
  return (
    <div className="an-engine">
      <div className="an-engine-row">
        <Pill tone={claude ? 'good' : 'warn'} icon={claude ? 'spark' : 'alert'}>{claude ? model?.label.split(' — ')[0] ?? info.model : 'Mode template'}</Pill>
        <span className="an-engine-txt">
          {claude
            ? <>Claude memanggil tool MCP lalu menulis analisis &amp; tindakan prioritas · kunci {info.key_hint}{info.key_source === 'env' ? ' (dari server)' : ''}</>
            : <>Kunci Claude API belum diisi — jadwal tetap jalan dengan laporan angka dari tool MCP, tanpa analisis Claude</>}
          {' · '}hari ini {info.runs_today} analisis · {rpSmall(info.spent_today_idr)}{info.daily_budget_idr > 0 ? ` dari anggaran ${rpSmall(info.daily_budget_idr)}` : ''}
        </span>
        {info.can_configure && <button className="btn ghost" onClick={() => setOpen(!open)}><Icon name="gear" />{open ? 'Tutup' : 'Atur mesin'}</button>}
      </div>
      {open && info.can_configure && (
        <div className="an-engine-form">
          <label>Kunci Claude API
            {info.key_source === 'ui' ? (
              <span className="an-keyset"><code>{info.key_hint}</code> tersimpan terenkripsi<button className="btn quiet" onClick={() => { if (window.confirm('Hapus kunci Claude API? Analisis kembali ke mode template.')) delKey.mutate() }}>Hapus</button></span>
            ) : (
              <span className="an-keyset">
                <input className="cl-input" type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder="sk-ant-api03-…" aria-label="Kunci Claude API" />
                <button className="btn primary" disabled={!key.startsWith('sk-ant-') || saveKey.isPending} onClick={() => saveKey.mutate()}><Icon name="lock" />Simpan</button>
              </span>
            )}
            <small>Buat di console.anthropic.com → API keys (berbayar per pemakaian). Kunci diperiksa ke Anthropic, disimpan terenkripsi, dan tidak pernah ditampilkan lagi.</small>
          </label>
          <label>Model
            <select className="cl-input" value={info.model} onChange={(e) => cfg.mutate({ model: e.target.value, daily_budget_idr: info.daily_budget_idr })} aria-label="Model">
              {info.models.map((m) => <option key={m.id} value={m.id}>{m.label}</option>)}
            </select>
            <small>± {rpSmall(estimate(info.model, 12))} per analisis (12 langkah, perkiraan)</small>
          </label>
          <label>Anggaran harian
            <select className="cl-input" value={info.daily_budget_idr} onChange={(e) => cfg.mutate({ model: info.model, daily_budget_idr: Number(e.target.value) })} aria-label="Anggaran harian">
              {[10000, 25000, 50000, 100000, 250000, 500000, 0].map((v) => <option key={v} value={v}>{v ? rpSmall(v) + ' / hari' : 'Tanpa batas'}</option>)}
            </select>
            <small>Lewat anggaran → laporan template sampai besok</small>
          </label>
        </div>
      )}
    </div>
  )
}

function CronField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [draft, setDraft] = useState(value)
  const [debounced, setDebounced] = useState(value)
  useEffect(() => { const t = window.setTimeout(() => setDebounced(draft), 350); return () => window.clearTimeout(t) }, [draft])
  useEffect(() => { onChange(debounced) }, [debounced, onChange])
  const { data: pv, isFetching } = useCronPreview(debounced)
  const pick = (c: string) => { setDraft(c); setDebounced(c) }
  return (
    <div className="full an-cron">
      <span className="an-lbl">Jadwal (WIB)</span>
      <div className="chips">{PRESETS.map(([l, c]) => <button key={c} type="button" className={`chip ${draft.trim() === c ? 'is-active' : ''}`} onClick={() => pick(c)}>{l}</button>)}</div>
      <input value={draft} onChange={(e) => setDraft(e.target.value)} spellCheck={false} aria-label="Ekspresi cron" placeholder="menit jam tanggal bulan hari — mis. 0 7 * * 1-6" style={{ fontFamily: 'var(--font-mono)' }} />
      <div className={`an-cron-pv ${pv && !pv.ok ? 'bad' : ''}`}>
        {!pv || isFetching ? 'Memeriksa…' : pv.ok
          ? <><b>{pv.description}</b> · berikutnya {pv.next?.slice(0, 3).map(when).join(' · ')}</>
          : pv.error}
      </div>
      <small>Format cron: <code>menit jam tanggal bulan hari</code> · <code>*</code> semua · <code>1-6</code> Senin–Sabtu · <code>*/2</code> tiap 2 · <code>7,16</code> daftar · minimal jarak 15 menit</small>
    </div>
  )
}

function ScheduleSheet({ sch, info }: { sch?: Schedule; info: AnalystInfo }) {
  const { closeSheet, toast } = useFeedback()
  const { data: me } = useMe()
  const qc = useQueryClient()
  const [f, setF] = useState({ name: sch?.name ?? '', prompt: sch?.prompt ?? PROMPTS[0], cron: sch?.cron ?? '0 7 * * 1-6', scopes: sch?.scopes ?? ['read', 'analyze'], max_steps: sch?.max_steps ?? 12, enabled: sch?.enabled ?? true })
  const { data: pv } = useCronPreview(f.cron)
  const save = useMutation({
    mutationFn: () => (sch ? api.put(`/mcp/schedules/${sch.id}`, f) : api.post('/mcp/schedules', f)),
    onSuccess: () => { toast(sch ? 'Jadwal diperbarui' : 'Jadwal dibuat'); qc.invalidateQueries({ queryKey: ['mcp'] }); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const del = useMutation({
    mutationFn: () => api.del(`/mcp/schedules/${sch!.id}`),
    onSuccess: () => { toast('Jadwal dihapus'); qc.invalidateQueries({ queryKey: ['mcp'] }); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const setCron = useCallback((c: string) => setF((x) => ({ ...x, cron: c })), [])
  const flip = (s: string) => setF({ ...f, scopes: f.scopes.includes(s) ? f.scopes.filter((x) => x !== s) : [...f.scopes, s] })
  const ok = f.name.trim() && f.prompt.trim().length >= 10 && pv?.ok
  return (
    <>
      <SheetHead icon="cal" title={sch ? sch.name : 'Jadwal analisis baru'} sub="Claude menganalisis lewat tool MCP pada waktu ini, laporannya tersimpan di riwayat" onClose={closeSheet} />
      <div className="sec frm">
        <label className="full">Nama<input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="mis. Ringkasan pagi, Piutang mingguan" maxLength={80} /></label>
        <label className="full">Yang dianalisis (prompt)
          <textarea className="an-prompt" value={f.prompt} onChange={(e) => setF({ ...f, prompt: e.target.value })} rows={5} maxLength={4000} />
        </label>
        <div className="full chips an-examples">
          {PROMPTS.map((p, i) => <button key={p} type="button" className="chip" title={p} onClick={() => setF({ ...f, prompt: p })}>Contoh {i + 1}</button>)}
        </div>
        <CronField value={f.cron} onChange={setCron} />
        <div className="full">
          <span className="an-lbl">Izin tool</span>
          <div className="chips">
            <button type="button" className="chip is-active" disabled title="Selalu">{SCOPE.read[0]}</button>
            <button type="button" className={`chip ${f.scopes.includes('analyze') ? 'is-active' : ''}`} onClick={() => flip('analyze')}>{SCOPE.analyze[0]}</button>
            {me?.role === 'ceo' && <button type="button" className={`chip ${f.scopes.includes('orchestrate') ? 'is-active' : ''}`} onClick={() => flip('orchestrate')}>{SCOPE.orchestrate[0]}</button>}
            <Pill tone="neutral" icon="lock">Memutuskan/kirim: tidak pernah</Pill>
          </div>
        </div>
        <label>Batas langkah (panggilan tool)
          <select value={f.max_steps} onChange={(e) => setF({ ...f, max_steps: Number(e.target.value) })}>
            {[4, 6, 8, 12, 16, 20, 30].map((n) => <option key={n} value={n}>{n} langkah</option>)}
          </select>
        </label>
        <label>Status
          <select value={f.enabled ? '1' : '0'} onChange={(e) => setF({ ...f, enabled: e.target.value === '1' })}>
            <option value="1">Aktif</option><option value="0">Jeda</option>
          </select>
        </label>
        <small className="full an-note">
          {info.engine === 'claude' ? `Perkiraan biaya ± ${rpSmall(estimate(info.model, f.max_steps))} per analisis.` : 'Mode template: tanpa biaya model sampai kunci Claude API diisi.'} Hasil hanya laporan &amp; usulan — keputusan tetap di aplikasi.
        </small>
      </div>
      <div className="ft">
        <button className="btn primary" disabled={!ok || save.isPending} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>
        {sch && <button className="btn ghost" style={{ color: 'var(--bad)' }} onClick={() => { if (window.confirm(`Hapus jadwal ${sch.name} beserta riwayat laporannya?`)) del.mutate() }}>Hapus</button>}
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
      </div>
    </>
  )
}

function Report({ id }: { id: string }) {
  const { data: run, isLoading } = useScheduleRun(id)
  if (isLoading || !run) return <p className="cl-empty">Memuat laporan…</p>
  return (
    <div className="an-report">
      <div className="an-report-meta">
        <Pill tone={STATUS[run.status][1]}>{STATUS[run.status][0]}</Pill>
        <span>{when(run.started_at)} · {run.trigger === 'manual' ? `dijalankan ${run.triggered_by ?? ''}` : 'terjadwal'}{run.model ? ` · ${run.model}` : ''}{run.cost_idr ? ` · ${rpSmall(run.cost_idr)}` : ''}{run.tokens_in ? ` · ${(run.tokens_in / 1000).toFixed(1)}k/${(run.tokens_out / 1000).toFixed(1)}k token` : ''}</span>
      </div>
      {run.error && <div className="an-err"><Icon name="alert" />{run.error}</div>}
      {run.status === 'running' ? <p className="cl-empty">Claude sedang menganalisis… laporan muncul otomatis.</p> : run.report ? <Markdown text={run.report} /> : <p className="cl-empty">Tidak ada laporan.</p>}
      {run.steps.length > 0 && (
        <details className="an-steps">
          <summary>{run.steps.length} panggilan tool MCP</summary>
          <ol>{run.steps.map((s, i) => <li key={i}><code>{s.tool}</code>{s.args && Object.keys(s.args).length ? <span className="mono"> {JSON.stringify(s.args)}</span> : null} · {s.ms} ms {s.status !== 'ok' && <Pill tone="bad">{s.status}</Pill>}{s.status !== 'ok' && s.summary ? ` ${s.summary}` : ''}</li>)}</ol>
        </details>
      )}
    </div>
  )
}

function ReportsSheet({ sch }: { sch: Schedule }) {
  const { closeSheet } = useFeedback()
  const { data: runs = [], isLoading } = useScheduleRuns(sch.id)
  const [sel, setSel] = useState('')
  const current = sel || runs[0]?.id || ''
  return (
    <div className="an-sheet">
      <SheetHead icon="doc" title={`Laporan · ${sch.name}`} sub={`${sch.description} · ${runs.length} laporan terakhir`} onClose={closeSheet} />
      {isLoading ? <p className="cl-empty" style={{ marginTop: 16 }}>Memuat…</p> : runs.length === 0 ? <p className="cl-empty" style={{ marginTop: 16 }}>Belum ada laporan. Tekan <b>Jalankan</b> untuk mencoba sekarang.</p> : (
        <div className="an-runs">
          <ul className="an-runlist">
            {runs.map((r) => (
              <li key={r.id}><button className={r.id === current ? 'is-active' : ''} onClick={() => setSel(r.id)}>
                <b>{when(r.started_at)}</b>
                <span><Pill tone={STATUS[r.status][1]}>{STATUS[r.status][0]}</Pill>{r.cost_idr ? ` ${rpSmall(r.cost_idr)}` : ''}</span>
              </button></li>
            ))}
          </ul>
          {current && <Report id={current} />}
        </div>
      )}
    </div>
  )
}

function Row({ sch, info }: { sch: Schedule; info: AnalystInfo }) {
  const qc = useQueryClient()
  const { toast, openSheet } = useFeedback()
  const run = useMutation({
    mutationFn: () => api.post<{ message: string }>(`/mcp/schedules/${sch.id}/run`),
    onSuccess: (r) => { toast(r.message); qc.invalidateQueries({ queryKey: ['mcp'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const toggle = useMutation({
    mutationFn: () => api.put(`/mcp/schedules/${sch.id}`, { ...sch, enabled: !sch.enabled }),
    onSuccess: () => { toast(sch.enabled ? 'Jadwal dijeda' : 'Jadwal aktif'); qc.invalidateQueries({ queryKey: ['mcp', 'schedules'] }) },
    onError: (e: Error) => toast(e.message),
  })
  const last = sch.last_run
  const edit = info.can_schedule
  return (
    <li className={`an-row ${sch.enabled ? '' : 'off'}`}>
      <button className={`sw ${sch.enabled ? 'on' : ''}`} disabled={!edit || toggle.isPending} aria-label={sch.enabled ? 'Jeda jadwal' : 'Aktifkan jadwal'} onClick={() => toggle.mutate()} />
      <div className="an-row-main">
        <b>{sch.name}</b>
        <span>{sch.description} · <code>{sch.cron}</code> · {sch.scopes.map((s) => SCOPE[s]?.[0] ?? s).join(', ')} · maks {sch.max_steps} langkah</span>
        <span>
          {sch.enabled && sch.next_runs[0] ? <>Berikutnya <b>{when(sch.next_runs[0])}</b></> : 'Dijeda'}
          {last && <> · terakhir {when(last.started_at)} <Pill tone={STATUS[last.status][1]}>{STATUS[last.status][0]}</Pill>{last.cost_idr ? ` ${rpSmall(last.cost_idr)}` : ''}</>}
        </span>
      </div>
      <div className="an-row-act">
        {edit && <button className="btn ghost" disabled={run.isPending || last?.status === 'running'} onClick={() => run.mutate()}><Icon name="refresh" />{last?.status === 'running' ? 'Berjalan…' : 'Jalankan'}</button>}
        <button className="btn ghost" onClick={() => openSheet(<ReportsSheet sch={sch} />)}><Icon name="doc" />Laporan</button>
        {edit && <button className="btn quiet" onClick={() => openSheet(<ScheduleSheet sch={sch} info={info} />)}>Ubah</button>}
      </div>
    </li>
  )
}

/** MCP Claude → Analisis terjadwal: cron schedules, the engine (API key, model, budget) and the reports. */
export function Schedules() {
  const { data: info } = useAnalyst()
  const { data: list = [] } = useSchedules()
  const { openSheet } = useFeedback()
  const running = list.some((s) => s.last_run?.status === 'running')
  const qc = useQueryClient()
  useEffect(() => {
    if (!running) return
    const t = window.setInterval(() => qc.invalidateQueries({ queryKey: ['mcp'] }), 5000)
    return () => window.clearInterval(t)
  }, [running, qc])
  if (!info) return null
  return (
    <div className="card">
      <div className="card-h">
        <h2>Analisis terjadwal</h2>
        <span className="meta">Claude menganalisis lewat MCP sesuai jadwal cron (WIB) · laporan tersimpan di sini</span>
        {info.can_schedule && <button className="btn primary" style={{ marginLeft: 'auto', height: 32, fontSize: 12.5 }} onClick={() => openSheet(<ScheduleSheet info={info} />)}><Icon name="cal" />Jadwal baru</button>}
      </div>
      <Engine info={info} />
      {list.length === 0 ? <p className="cl-empty">Belum ada jadwal. Buat satu — mis. ringkasan setiap pagi Senin–Sabtu 07.00.</p> : (
        <ul className="an-list">{list.map((s) => <Row key={s.id} sch={s} info={info} />)}</ul>
      )}
    </div>
  )
}
