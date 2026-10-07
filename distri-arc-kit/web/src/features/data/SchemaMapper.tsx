import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { BQSchema, DataEntity, DataStatus } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe } from '../../app/queries'
import { buildSQL, missingRequired } from './sql'

const ENTITY: Record<DataEntity, string> = { sales: 'Tim sales', customers: 'Pelanggan', invoices: 'Faktur', invoice_lines: 'Item faktur', stock: 'Stok per gudang' }
const input = { height: 30, borderRadius: 8, border: '1px solid var(--line)', padding: '0 9px', font: 'inherit', fontSize: 12.5, background: 'var(--surface)', color: 'var(--text)', minWidth: 0, width: '100%' }

type Pick = { table: string; cols: Record<string, string>; where: string }
type TestResult = { ok: boolean; rows?: number; missing?: string[]; error?: string }

/** Pengaturan → Data & master → Pemetaan BigQuery: browse the project's tables (read-only), map columns to the
 * contract per entity (pre-filled from Accurate / Indonesian column names), preview the SQL and save it. */
export function SchemaMapper({ st }: { st: DataStatus }) {
  const { data: me } = useMe()
  const ceo = me?.role === 'ceo'
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [entity, setEntity] = useState<DataEntity>('customers')
  const [picks, setPicks] = useState<Partial<Record<DataEntity, Pick>>>({})
  const [tests, setTests] = useState<Record<string, TestResult>>({})
  const [filter, setFilter] = useState('')
  const schema = useMutation({
    mutationFn: () => api.get<BQSchema>('/data/schema'),
    onSuccess: (s) => setPicks(Object.fromEntries(s.suggestions.map((g) => [g.entity, { table: g.table, cols: g.columns, where: '' }]))),
    onError: (e: Error) => toast(e.message),
  })
  const test = useMutation({ mutationFn: () => api.post<{ results: Record<string, TestResult> }>('/data/test'), onSuccess: (r) => setTests(r.results), onError: (e: Error) => toast(e.message) })
  const s = schema.data
  const project = s?.project ?? (st.source.bigquery.project_id || st.credentials.project_id || '')
  const sqlOf = (e: DataEntity) => {
    const p = picks[e]
    return p?.table ? buildSQL(project, e, st.contract[e], p.table, p.cols, p.where) : ''
  }
  const ready = st.entities.filter((e) => picks[e]?.table && missingRequired(st.contract[e], picks[e]!.cols).length === 0)
  const save = () => {
    const queries = { ...st.source.bigquery.queries }
    for (const e of ready) queries[e] = sqlOf(e)
    api.put('/data/source', { ...st.source, mode: 'bigquery', bigquery: { ...st.source.bigquery, project_id: st.source.bigquery.project_id || project, queries } })
      .then(() => { toast(`${ready.length} query disimpan — jalankan Tes query lalu Sinkron sekarang`); qc.invalidateQueries({ queryKey: ['data'] }) }, (e: Error) => toast(e.message))
  }
  const pick = picks[entity]
  const table = s?.tables.find((t) => `${t.dataset}.${t.table}` === pick?.table)
  const cands = s?.suggestions.find((g) => g.entity === entity)?.candidates ?? []
  const setPick = (p: Partial<Pick>) => setPicks({ ...picks, [entity]: { table: '', cols: {}, where: '', ...pick, ...p } })
  const chooseTable = (t: string) => setPick({ table: t, cols: cands.find((c) => c.table === t)?.columns ?? {} })
  const missing = pick?.table ? missingRequired(st.contract[entity], pick.cols) : st.contract[entity].filter((c) => c.required).map((c) => c.name)
  const shown = (s?.tables ?? []).filter((t) => !filter || `${t.dataset}.${t.table} ${(t.columns ?? []).map((c) => c.name).join(' ')}`.toLowerCase().includes(filter.toLowerCase()))

  return (
    <div className="card">
      <div className="card-h"><h2>Pemetaan BigQuery</h2><span className="meta">{s ? `${project} · ${s.tables.length} tabel · baca saja` : 'Jelajahi tabel Accurate di BigQuery, cocokkan kolom, simpan query'}</span></div>
      {!st.credentials.set ? (
        <p style={{ fontSize: 13, color: 'var(--text-2)', margin: 0 }}>Unggah kunci service account di <b>Sumber data asli → BigQuery</b> dulu, lalu kembali ke sini.</p>
      ) : !s ? (
        <button className="btn primary" disabled={schema.isPending} onClick={() => schema.mutate()}><Icon name="search" />{schema.isPending ? 'Membaca skema BigQuery…' : 'Jelajahi BigQuery'}</button>
      ) : (
        <div className="stack" style={{ gap: 12 }}>
          <div className="chips">
            {st.entities.map((e) => {
              const p = picks[e]
              const ok = p?.table && missingRequired(st.contract[e], p.cols).length === 0
              return <button key={e} className={`chip ${entity === e ? 'is-active' : ''}`} onClick={() => setEntity(e)}>{ENTITY[e]}{ok ? ' ✓' : <span className="badge-un">!</span>}</button>
            })}
          </div>
          <div className="q-box">
            <div className="q-h"><b>{ENTITY[entity]}</b><span>Tabel sumber — saran diurutkan dari yang paling cocok</span>
              {tests[entity] && <Pill tone={tests[entity].ok ? 'good' : 'bad'}>{tests[entity].ok ? `OK · ${tests[entity].rows} contoh` : tests[entity].error ?? `kurang: ${tests[entity].missing?.join(', ')}`}</Pill>}</div>
            <select style={input} value={pick?.table ?? ''} onChange={(e) => chooseTable(e.target.value)} aria-label={`Tabel ${ENTITY[entity]}`}>
              <option value="">— pilih tabel —</option>
              {cands.length > 0 && <optgroup label="Saran">{cands.map((c) => <option key={'s' + c.table} value={c.table}>★ {c.table}</option>)}</optgroup>}
              <optgroup label="Semua tabel">{s.tables.map((t) => <option key={t.dataset + t.table} value={`${t.dataset}.${t.table}`}>{t.dataset}.{t.table} · {t.rows.toLocaleString('id-ID')} baris{t.type !== 'TABLE' ? ` · ${t.type.toLowerCase()}` : ''}</option>)}</optgroup>
            </select>
            {pick?.table && (
              <>
                <div className="map-grid">
                  {st.contract[entity].map((c) => (
                    <label key={c.name} title={c.desc}>
                      <span><b className="mono">{c.name}{c.required ? '*' : ''}</b> {c.desc}</span>
                      <input style={input} list={`cols-${entity}`} value={pick.cols[c.name] ?? ''} placeholder="— tidak ada —" onChange={(e) => setPick({ cols: { ...pick.cols, [c.name]: e.target.value } })} aria-label={`Kolom ${c.name}`} />
                    </label>
                  ))}
                  <datalist id={`cols-${entity}`}>{(table?.columns ?? []).map((c) => <option key={c.name} value={c.name}>{c.type}</option>)}</datalist>
                </div>
                <label className="map-where"><span>Filter (opsional, SQL WHERE)</span><input style={input} value={pick.where} onChange={(e) => setPick({ where: e.target.value })} placeholder={entity === 'invoices' ? 'mis. tanggal_faktur >= "2024-01-01"' : 'mis. aktif = true'} /></label>
                {missing.length > 0 && <Pill tone="warn">Kolom wajib belum dipetakan: {missing.join(', ')}</Pill>}
                <pre className="sql-pre">{sqlOf(entity)}</pre>
              </>
            )}
          </div>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
            {ceo && <button className="btn primary" disabled={!ready.length} onClick={save}><Icon name="check" />Simpan {ready.length} query</button>}
            <button className="btn quiet" disabled={test.isPending} onClick={() => test.mutate()}><Icon name="search" />{test.isPending ? 'Menguji…' : 'Tes query tersimpan'}</button>
            <button className="btn ghost" disabled={schema.isPending} onClick={() => schema.mutate()}><Icon name="refresh" />Baca ulang skema</button>
            {!ceo && <span style={{ fontSize: 12, color: 'var(--text-3)' }}>Menyimpan query hanya untuk CEO</span>}
          </div>
          <details className="bq-tables">
            <summary>Semua tabel ({s.tables.length})</summary>
            <input style={{ ...input, margin: '8px 0' }} value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Cari tabel atau kolom…" />
            <ul className="rules">{shown.slice(0, 200).map((t) => (
              <li key={t.dataset + t.table}><div><b>{t.dataset}.{t.table}</b><span>{t.rows.toLocaleString('id-ID')} baris · {(t.columns ?? []).map((c) => c.name).join(', ')}</span></div></li>
            ))}</ul>
          </details>
        </div>
      )}
    </div>
  )
}
