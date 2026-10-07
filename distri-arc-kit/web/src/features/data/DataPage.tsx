import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router'
import { api } from '../../api/client'
import type { DataEntity, DataMapping, DataSource, DataStatus, MappingKind, MasterCustomer, TeamMember } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { fmtRp, hhmm, shortDate } from '../../lib/format'
import { useMe } from '../../app/queries'
import { SchemaMapper } from './SchemaMapper'

const ENTITY: Record<DataEntity, string> = { sales: 'Tim sales', customers: 'Pelanggan', invoices: 'Faktur', invoice_lines: 'Item faktur', stock: 'Stok per gudang' }
const KIND: [MappingKind, string, string][] = [
  ['branch', 'Cabang', 'Nama cabang di data sumber → cabang Distri ARC'],
  ['warehouse', 'Gudang', 'Gudang → cabang (stok dijumlah per cabang)'],
  ['category', 'Kategori', 'Kategori barang → 6 kategori product mix'],
  ['sales', 'Sales', 'Nama sales di data sumber → anggota tim'],
  ['ctype', 'Jenis pelanggan', 'Nilai jenis pelanggan → Dealer (reseller) atau Freelance / SI'],
]
const input = { height: 32, borderRadius: 8, border: '1px solid var(--line)', padding: '0 9px', font: 'inherit', fontSize: 13, background: 'var(--surface)', color: 'var(--text)', minWidth: 0 }

export const useDataStatus = () => useQuery({ queryKey: ['data', 'status'], queryFn: () => api.get<DataStatus>('/data/status'), refetchInterval: (q) => (q.state.data?.runs.some((r) => r.status === 'running') ? 3000 : 20000) })
const useTeam = () => useQuery({ queryKey: ['data', 'team'], queryFn: () => api.get<{ items: TeamMember[] }>('/data/team').then((r) => r.items) })
const useBranches = () => useQuery({ queryKey: ['data', 'branches'], queryFn: () => api.get<{ items: string[] }>('/data/branches').then((r) => r.items) })

function useAct() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  return (fn: () => Promise<{ message?: string } | unknown>, keys: string[][] = [['data']]) =>
    fn().then((r) => {
      const m = (r as { message?: string } | undefined)?.message
      if (m) toast(m)
      for (const k of keys) qc.invalidateQueries({ queryKey: k })
    }, (e: Error) => toast(e.message))
}

/** Sumber data: BigQuery (project, key, one query per entity) or CSV. */
function SourceCard({ st }: { st: DataStatus }) {
  const { data: me } = useMe()
  const ceo = me?.role === 'ceo'
  const act = useAct()
  const [src, setSrc] = useState<DataSource>(st.source)
  const [tests, setTests] = useState<Record<string, { ok: boolean; rows?: number; missing?: string[]; error?: string }>>({})
  const bq = src.bigquery
  const setBQ = (p: Partial<DataSource['bigquery']>) => setSrc({ ...src, bigquery: { ...bq, ...p } })
  const upload = (f: File) => f.text().then((t) => act(() => fetch('/api/data/credentials', { method: 'PUT', credentials: 'same-origin', body: t, headers: { 'Content-Type': 'application/json' } }).then(async (r) => {
    const d = await r.json()
    if (!r.ok) throw new Error(d?.error?.message ?? 'Gagal')
    return d
  })))
  const test = useMutation({ mutationFn: () => api.post<{ results: typeof tests }>('/data/test'), onSuccess: (r) => setTests(r.results) })
  return (
    <div className="card">
      <div className="card-h"><h2>Sumber data asli</h2><span className="meta">{st.dealers} pelanggan · {st.invoices} faktur di Distri ARC</span></div>
      <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
        <div className="seg">{(['none', 'bigquery', 'csv'] as const).map((m) => <button key={m} disabled={!ceo} className={src.mode === m ? 'is-active' : ''} onClick={() => setSrc({ ...src, mode: m })}>{{ none: 'Belum', bigquery: 'BigQuery', csv: 'Impor CSV' }[m]}</button>)}</div>
        <span style={{ fontSize: 12.5, color: 'var(--text-2)' }}>{src.mode === 'bigquery' ? `Ditarik otomatis tiap ${bq.sync_minutes} menit · baca saja` : src.mode === 'csv' ? 'Unggah file CSV per jenis data di bawah' : 'Pilih sumber data'}</span>
      </div>
      {src.mode === 'bigquery' && (
        <div className="stack" style={{ marginTop: 14, gap: 10 }}>
          <div className="data-grid">
            <label>Project ID<input style={input} value={bq.project_id} disabled={!ceo} onChange={(e) => setBQ({ project_id: e.target.value })} placeholder="mis. gsi-data" /></label>
            <label>Lokasi<input style={input} value={bq.location} disabled={!ceo} onChange={(e) => setBQ({ location: e.target.value })} placeholder="mis. asia-southeast2" /></label>
            <label>Sinkron tiap (menit)<input style={input} type="number" min={15} max={1440} value={bq.sync_minutes} disabled={!ceo} onChange={(e) => setBQ({ sync_minutes: Number(e.target.value) })} /></label>
          </div>
          <ul className="rules">
            <li><div><b>Kunci service account</b><span>{st.credentials.set ? (st.credentials.error ?? `${st.credentials.client_email} · disimpan terenkripsi`) : 'Belum diunggah — buat di Google Cloud: IAM → Service accounts → peran BigQuery Data Viewer + Job User → Keys → JSON'}</span></div>
              {ceo && <label className="btn quiet" style={{ height: 28, fontSize: 12 }}><Icon name="lock" />{st.credentials.set ? 'Ganti' : 'Unggah JSON'}<input type="file" accept=".json,application/json" hidden onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} /></label>}</li>
          </ul>
          {st.entities.map((e) => (
            <div key={e} className="q-box">
              <div className="q-h"><b>{ENTITY[e]}</b><span>kolom: {st.contract[e].map((c) => c.required ? c.name + '*' : c.name).join(', ')}</span>
                {tests[e] && <Pill tone={tests[e].ok ? 'good' : 'bad'}>{tests[e].ok ? `OK · ${tests[e].rows} contoh` : tests[e].error ?? `kurang: ${tests[e].missing?.join(', ')}`}</Pill>}</div>
              <textarea value={bq.queries[e] ?? ''} disabled={!ceo} onChange={(ev) => setBQ({ queries: { ...bq.queries, [e]: ev.target.value } })} placeholder={`SELECT … AS ${st.contract[e].filter((c) => c.required).map((c) => c.name).join(', … AS ')} FROM \`project.dataset.tabel\``} spellCheck={false} />
            </div>
          ))}
        </div>
      )}
      <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
        {ceo && <button className="btn primary" onClick={() => act(() => api.put('/data/source', src))}><Icon name="check" />Simpan</button>}
        {src.mode === 'bigquery' && <button className="btn quiet" disabled={test.isPending} onClick={() => test.mutate()}><Icon name="search" />{test.isPending ? 'Menguji…' : 'Tes query'}</button>}
        {src.mode === 'bigquery' && <button className="btn quiet" onClick={() => act(() => api.post('/data/sync'))}><Icon name="refresh" />Sinkron sekarang</button>}
        <button className="btn ghost" onClick={() => act(() => api.post('/data/apply'))}><Icon name="refresh" />Proses ulang</button>
      </div>
    </div>
  )
}

/** CSV import per entity, with header templates. */
function CSVCard({ st }: { st: DataStatus }) {
  const act = useAct()
  const staged = Object.fromEntries(st.staged.map((s) => [s.entity, s]))
  const upload = (e: DataEntity, f: File) => act(() => fetch(`/api/data/import/${e}`, { method: 'POST', credentials: 'same-origin', body: f, headers: { 'Content-Type': 'text/csv' } }).then(async (r) => {
    const d = await r.json()
    if (!r.ok) throw new Error(d?.error?.message ?? 'Gagal')
    return d
  }))
  return (
    <div className="card">
      <div className="card-h"><h2>Impor CSV</h2><span className="meta">Urutan: tim sales → pelanggan → faktur → item faktur → stok</span></div>
      <ul className="rules">
        {st.entities.map((e) => (
          <li key={e}><div><b>{ENTITY[e]}</b><span>{staged[e] ? `${staged[e].n} baris · terakhir ${shortDate(staged[e].last_at)} ${hhmm(staged[e].last_at)}` : 'Belum ada data'} · <a href={`/api/data/template/${e}`} download>template</a></span></div>
            <label className="btn quiet" style={{ height: 28, fontSize: 12 }}><Icon name="doc" />Unggah CSV<input type="file" accept=".csv,text/csv" hidden onChange={(ev) => { const f = ev.target.files?.[0]; if (f) upload(e, f); ev.target.value = '' }} /></label></li>
        ))}
      </ul>
    </div>
  )
}

function RunsCard({ st }: { st: DataStatus }) {
  if (!st.runs.length) return null
  return (
    <div className="card">
      <div className="card-h"><h2>Riwayat impor</h2></div>
      <div className="tbl-wrap">
        <table className="tbl">
          <thead><tr><th>Waktu</th><th>Sumber</th><th>Data</th><th>Baris</th><th>Masuk</th><th>Dilewati</th><th>Belum dipetakan</th><th>Status</th></tr></thead>
          <tbody>{st.runs.map((r) => (
            <tr key={r.id}><td className="mono">{shortDate(r.started_at)} {hhmm(r.started_at)}</td><td>{r.source}</td><td>{r.entity === 'apply' ? 'proses' : ENTITY[r.entity as DataEntity] ?? r.entity}</td><td className="num">{r.rows}</td><td className="num">{r.upserted}</td><td className="num">{r.skipped}</td><td className="num">{r.unmapped}</td>
              <td><Pill tone={r.status === 'done' ? 'good' : r.status === 'running' ? 'warn' : 'bad'}>{r.status === 'done' ? 'Selesai' : r.status === 'running' ? 'Berjalan' : 'Gagal'}</Pill>{r.error && <div style={{ fontSize: 11, color: 'var(--bad)' }}>{r.error}</div>}</td></tr>
          ))}</tbody>
        </table>
      </div>
    </div>
  )
}

/** Mapping editor: unmapped values first; saving re-applies the data. */
function MappingCard({ st }: { st: DataStatus }) {
  const [kind, setKind] = useState<MappingKind>('branch')
  const { data: rows = [] } = useQuery({ queryKey: ['data', 'mappings', kind], queryFn: () => api.get<{ items: DataMapping[] }>(`/data/mappings?kind=${kind}`).then((r) => r.items) })
  const { data: team = [] } = useTeam()
  const { data: branches = [] } = useBranches()
  const [edits, setEdits] = useState<Record<string, string>>({})
  const act = useAct()
  const counts = Object.fromEntries(st.mappings.map((m) => [m.kind, m]))
  const changed = Object.entries(edits)
  const suggestable = rows.filter((m) => !m.target && m.suggested && edits[m.source_value] === undefined)
  const control = (m: DataMapping) => {
    const v = edits[m.source_value] ?? m.target ?? ''
    const set = (t: string) => setEdits({ ...edits, [m.source_value]: t })
    if (kind === 'category') return <select style={input} value={v} onChange={(e) => set(e.target.value)} aria-label={`Kategori ${m.source_value}`}><option value="">— belum —</option>{st.categories.map((c) => <option key={c}>{c}</option>)}</select>
    if (kind === 'ctype') return <select style={input} value={v} onChange={(e) => set(e.target.value)}><option value="">— belum —</option><option value="reseller">Dealer (reseller)</option><option value="si">Freelance / SI</option></select>
    if (kind === 'sales') return <select style={input} value={v} onChange={(e) => set(e.target.value)}><option value="">— belum —</option>{team.filter((t) => t.role === 'sales').map((t) => <option key={t.id} value={t.id}>{t.name} · {t.branch}</option>)}</select>
    return (<><input style={input} list="branch-list" value={v} onChange={(e) => set(e.target.value)} placeholder="mis. Semarang" aria-label={`Cabang untuk ${m.source_value}`} /><datalist id="branch-list">{branches.map((b) => <option key={b} value={b} />)}</datalist></>)
  }
  return (
    <div className="card">
      <div className="card-h"><h2>Mapping master data</h2><span className="meta">{KIND.find((k) => k[0] === kind)?.[2]}</span></div>
      <div className="chips" style={{ marginBottom: 10 }}>
        {KIND.map(([k, l]) => <button key={k} className={`chip ${kind === k ? 'is-active' : ''}`} onClick={() => { setKind(k); setEdits({}) }}>{l}{counts[k]?.unmapped ? <span className="badge-un">{counts[k].unmapped}</span> : null}</button>)}
      </div>
      {rows.length === 0 ? <p style={{ fontSize: 13, color: 'var(--text-3)', margin: 0 }}>Nilai muncul di sini setelah data pertama diimpor.</p> : (
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Nilai di data sumber</th><th>Muncul</th><th>Dipetakan ke</th></tr></thead>
            <tbody>{rows.map((m) => (
              <tr key={m.source_value} style={!m.target && edits[m.source_value] === undefined ? { background: 'var(--warn-soft)' } : undefined}>
                <td><b>{m.source_value}</b>{m.updated_by && <div className="mono">{m.updated_by}</div>}</td><td className="num">{m.seen}</td><td>{control(m)}{!m.target && m.suggested && edits[m.source_value] === undefined && <div style={{ fontSize: 11, color: 'var(--text-3)' }}>saran: {kind === 'ctype' ? (m.suggested === 'si' ? 'Freelance / SI' : 'Dealer (reseller)') : m.suggested}</div>}</td></tr>
            ))}</tbody>
          </table>
        </div>
      )}
      <div style={{ display: 'flex', gap: 8, marginTop: 12, alignItems: 'center' }}>
        {suggestable.length > 0 && <button className="btn quiet" onClick={() => setEdits({ ...edits, ...Object.fromEntries(suggestable.map((m) => [m.source_value, m.suggested!])) })}><Icon name="spark" />Isi {suggestable.length} saran</button>}
        <button className="btn primary" disabled={!changed.length} onClick={() => act(() => api.put('/data/mappings', changed.map(([source_value, target]) => ({ kind, source_value, target }))).then((r) => { setEdits({}); return r }))}><Icon name="check" />Simpan {changed.length || ''} mapping</button>
        <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Setelah disimpan, data diproses ulang dengan mapping baru</span>
      </div>
    </div>
  )
}

/** Customer master: type, tier, credit limit, terms and owner — kept on the next import once edited. */
function CustomersCard() {
  const [q, setQ] = useState('')
  const [type, setType] = useState('')
  const [incomplete, setIncomplete] = useState(false)
  const { data: rows = [] } = useQuery({ queryKey: ['data', 'customers', q, type, incomplete], queryFn: () => api.get<{ items: MasterCustomer[] }>(`/data/customers?limit=100&q=${encodeURIComponent(q)}&type=${type}&incomplete=${incomplete ? 1 : 0}`).then((r) => r.items) })
  const { data: team = [] } = useTeam()
  const act = useAct()
  const save = (c: MasterCustomer, patch: Record<string, unknown>) => act(() => api.patch(`/data/customers/${c.id}`, patch), [['data', 'customers'], ['dealers'], ['orbit']])
  const sales = useMemo(() => team.filter((t) => t.role === 'sales'), [team])
  return (
    <div className="card">
      <div className="card-h"><h2>Master pelanggan</h2><span className="meta">Isian di sini tidak tertimpa impor berikutnya</span></div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 10 }}>
        <input style={{ ...input, flex: 1, minWidth: 180 }} value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari pelanggan / kota…" />
        <div className="seg">{[['', 'Semua'], ['reseller', 'Dealer'], ['si', 'Freelance / SI']].map(([k, l]) => <button key={k} className={type === k ? 'is-active' : ''} onClick={() => setType(k)}>{l}</button>)}</div>
        <button className={`chip ${incomplete ? 'is-active' : ''}`} onClick={() => setIncomplete(!incomplete)}>Belum lengkap</button>
      </div>
      <div className="tbl-wrap">
        <table className="tbl">
          <thead><tr><th>Pelanggan</th><th>Jenis</th><th>Tier</th><th>Limit kredit</th><th>Termin</th><th>Sales</th><th>Omzet/bln</th></tr></thead>
          <tbody>{rows.map((c) => (
            <tr key={c.id}>
              <td><Link to={`/dealer/${c.slug}`}><b>{c.name}</b></Link><div className="mono">{[c.city, c.branch].filter(Boolean).join(' · ')}</div></td>
              <td><select style={input} value={c.customer_type} onChange={(e) => save(c, { customer_type: e.target.value })} aria-label={`Jenis ${c.name}`}><option value="reseller">Dealer (reseller)</option><option value="si">Freelance / SI</option></select></td>
              <td><select style={{ ...input, width: 64 }} value={c.tier ?? ''} onChange={(e) => e.target.value && save(c, { tier: e.target.value })}><option value="">—</option><option>A</option><option>B</option><option>C</option></select></td>
              <td><input style={{ ...input, width: 120, textAlign: 'right' }} defaultValue={c.credit_limit ? String(c.credit_limit / 1e6) : ''} placeholder="jt" onBlur={(e) => { const v = Math.round(Number(e.target.value || 0) * 1e6); if (v !== c.credit_limit) save(c, { credit_limit: v }) }} aria-label={`Limit ${c.name} (juta)`} /></td>
              <td><input style={{ ...input, width: 64, textAlign: 'right' }} defaultValue={c.payment_terms_days} onBlur={(e) => { const v = Number(e.target.value); if (v > 0 && v !== c.payment_terms_days) save(c, { payment_terms_days: v }) }} aria-label={`Termin ${c.name}`} /></td>
              <td><select style={input} value={c.owner_id ?? ''} onChange={(e) => e.target.value && save(c, { owner_id: e.target.value })}><option value="">—</option>{sales.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select></td>
              <td className="num">{c.omzet_bln ? fmtRp(c.omzet_bln) : '—'}</td>
            </tr>
          ))}</tbody>
        </table>
        {rows.length === 0 && <p style={{ fontSize: 13, color: 'var(--text-3)' }}>Belum ada pelanggan — impor data dulu.</p>}
      </div>
    </div>
  )
}

function TeamCard() {
  const { data: team = [] } = useTeam()
  const { data: branches = [] } = useBranches()
  const act = useAct()
  const [f, setF] = useState({ name: '', branch: '', wa_number: '' })
  return (
    <div className="card">
      <div className="card-h"><h2>Tim sales</h2><span className="meta">{team.filter((t) => t.role === 'sales').length} sales · login dibuat di Pengguna &amp; peran</span></div>
      <ul className="rules">
        {team.map((t) => (
          <li key={t.id}><div><b>{t.name}{!t.active && ' (nonaktif)'}</b><span>{t.role} · {t.branch}{t.wa_number ? ` · ${t.wa_number}` : ''}{t.external_name && t.external_name !== t.name ? ` · di sumber: ${t.external_name}` : ''} · {t.dealers} pelanggan{t.login_email ? ` · login ${t.login_email}` : ''}</span></div></li>
        ))}
      </ul>
      <form className="wa-add" style={{ gridTemplateColumns: '1fr 1fr 1fr auto' }} onSubmit={(e) => { e.preventDefault(); act(() => api.post('/data/team', f).then((r) => { setF({ name: '', branch: '', wa_number: '' }); return r })) }}>
        <input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Nama sales" aria-label="Nama sales" />
        <input value={f.branch} list="branch-list-team" onChange={(e) => setF({ ...f, branch: e.target.value })} placeholder="Cabang" aria-label="Cabang sales" />
        <datalist id="branch-list-team">{branches.map((b) => <option key={b} value={b} />)}</datalist>
        <input value={f.wa_number} onChange={(e) => setF({ ...f, wa_number: e.target.value })} placeholder="WhatsApp (opsional)" aria-label="WhatsApp sales" />
        <button className="btn primary" type="submit" disabled={!f.name.trim() || !f.branch.trim()}><Icon name="people" />Tambah</button>
      </form>
    </div>
  )
}

/** Pengaturan → Data & master. */
export function DataPage() {
  const { data: st } = useDataStatus()
  if (!st) return null
  return (
    <div className="stack">
      <SourceCard key={JSON.stringify(st.source)} st={st} />
      {st.source.mode !== 'csv' && <SchemaMapper st={st} />}
      <div className="ai-grid">
        <div className="stack"><MappingCard st={st} /><TeamCard /></div>
        <div className="stack"><CSVCard st={st} /><RunsCard st={st} /></div>
      </div>
      <CustomersCard />
      <p style={{ fontSize: 12, color: 'var(--text-3)', margin: 0 }}><Link to="/pengaturan">← Pengaturan</Link></p>
    </div>
  )
}

/** Pengaturan card: data source at a glance. */
export function DataCard() {
  const { data: st } = useDataStatus()
  if (!st) return null
  const unmapped = st.mappings.reduce((n, m) => n + m.unmapped, 0)
  const last = st.runs.find((r) => r.entity === 'apply')
  return (
    <div className="card">
      <div className="card-h"><h2>Data &amp; master</h2><Pill tone={st.source.mode === 'none' ? 'warn' : 'good'}>{{ none: 'Belum ada sumber', bigquery: 'BigQuery', csv: 'Impor CSV' }[st.source.mode]}</Pill></div>
      <ul className="rules">
        <li><div><b>{st.dealers} pelanggan · {st.invoices} faktur</b><span>{last ? `Diproses ${shortDate(last.started_at)} ${hhmm(last.started_at)}` : 'Belum ada impor'}{unmapped ? ` · ${unmapped} nilai belum dipetakan` : ''}</span></div><Link className="btn quiet" style={{ height: 26, fontSize: 12 }} to="/pengaturan/data">Buka</Link></li>
      </ul>
    </div>
  )
}
