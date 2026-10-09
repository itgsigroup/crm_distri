import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { bestTarget, suggestions, type SalesProfile } from './match'

type Column = 'name' | 'branch' | 'dealers' | 'status'
type View = 'all' | 'open' | 'merged' | 'suggest'

const useSalesMap = () => useQuery({ queryKey: ['sales-map'], queryFn: () => api.get<{ items: SalesProfile[] }>('/sales-map').then((r) => r.items) })

const NONE: SalesProfile[] = []
const srcLabel = (p: SalesProfile) => (p.source_system === 'import' ? 'BigQuery' : p.source_system === 'odoo' ? 'Odoo' : 'GSI Orbit')

/** Master data → Mapping sales: merge the spellings of one person in the source data into one GSI Orbit sales. */
export function SalesMapPage() {
  const { data: rows = NONE, isLoading } = useSalesMap()
  const qc = useQueryClient()
  const { toast, alert } = useFeedback()
  const [view, setView] = useState<View>('all')
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [target, setTarget] = useState('')
  const [busy, setBusy] = useState(false)
  const [newName, setNewName] = useState('')
  const [newBranch, setNewBranch] = useState('')
  const byId = useMemo(() => new Map(rows.map((r) => [r.id, r])), [rows])
  const sugg = useMemo(() => suggestions(rows), [rows])
  const targets = useMemo(() => rows.filter((r) => !r.merged_into).sort((a, b) => a.name.localeCompare(b.name)), [rows])
  const childrenOf = useMemo(() => {
    const m = new Map<string, SalesProfile[]>()
    for (const r of rows) if (r.merged_into) m.set(r.merged_into, [...(m.get(r.merged_into) ?? []), r])
    return m
  }, [rows])
  const total = (p: SalesProfile) => p.dealers + (childrenOf.get(p.id) ?? []).reduce((a, c) => a + c.dealers, 0)
  const shown = useMemo(() => rows.filter((r) => view === 'all' || (view === 'open' ? !r.merged_into : view === 'merged' ? !!r.merged_into : sugg.has(r.id))), [rows, view, sugg])
  const statusOf = (p: SalesProfile) => (p.merged_into ? `→ ${byId.get(p.merged_into)?.name ?? ''}` : '')
  const t = useDataTable<SalesProfile, Column>({
    rows: shown,
    text: useMemo(() => (p: SalesProfile) => `${p.name} ${p.source_id ?? ''} ${p.branch} ${p.login_email ?? ''}`, []),
    compare: useMemo(() => ({
      name: (a: SalesProfile, b: SalesProfile) => a.name.localeCompare(b.name, 'id'),
      branch: (a: SalesProfile, b: SalesProfile) => a.branch.localeCompare(b.branch, 'id'),
      dealers: (a: SalesProfile, b: SalesProfile) => a.dealers - b.dealers,
      status: (a: SalesProfile, b: SalesProfile) => Number(!!a.merged_into) - Number(!!b.merged_into),
    }), []),
    tie: useMemo(() => (a: SalesProfile, b: SalesProfile) => a.name.localeCompare(b.name, 'id'), []),
    initial: { column: 'name', dir: 'asc' },
    firstDir: (c) => (c === 'dealers' ? 'desc' : 'asc'),
  })

  const refresh = () => {
    for (const k of ['sales-map', 'sales', 'dealers', 'data', 'agenda', 'kpi', 'wa']) qc.invalidateQueries({ queryKey: [k] })
  }
  const merge = (sources: string[], to: string | null) => {
    setBusy(true)
    return api.post<{ message: string }>('/sales-map/merge', { sources, target_id: to }).then((r) => {
      toast(r.message)
      setPicked(new Set())
      setTarget('')
      refresh()
    }, (e: Error) => alert({ icon: 'error', title: 'Mapping belum tersimpan', text: e.message })).finally(() => setBusy(false))
  }
  const create = (name: string, branch: string) =>
    api.post<{ id: string; message: string }>('/sales-map/profiles', { name, branch }).then((r) => {
      toast(r.message)
      refresh()
      return r.id
    })
  const toggle = (id: string) => setPicked((s) => {
    const n = new Set(s)
    if (n.has(id)) n.delete(id)
    else n.add(id)
    return n
  })
  const pickedRows = [...picked].map((id) => byId.get(id)).filter((x): x is SalesProfile => !!x)
  const pickGroup = (p: SalesProfile) => {
    const g = [p, ...(sugg.get(p.id) ?? [])]
    setPicked(new Set(g.map((x) => x.id)))
    setTarget(bestTarget(g)?.id ?? '')
  }
  const mergePicked = () => {
    const to = byId.get(target)
    if (!to) return
    const sources = pickedRows.filter((x) => x.id !== to.id).map((x) => x.id)
    if (sources.length) merge(sources, to.id)
  }
  const createAndMerge = () => {
    const name = newName.trim()
    if (!name) return
    setBusy(true)
    create(name, newBranch.trim()).then((id) => {
      setNewName('')
      setNewBranch('')
      if (pickedRows.length) return merge(pickedRows.map((x) => x.id), id)
    }, (e: Error) => alert({ icon: 'error', title: 'Sales belum ditambahkan', text: e.message })).finally(() => setBusy(false))
  }
  const counts = { all: rows.length, open: targets.length, merged: rows.length - targets.length, suggest: sugg.size }

  return (
    <div className="smap">
      <div className="card smap-head">
        <div>
          <h2>Mapping sales</h2>
          <p>Data BigQuery (Accurate) sering mengeja satu orang beberapa kali — mis. <b>Granike Monica</b> dan <b>Granike Monica M.</b>. Gabungkan ejaan-ejaan itu ke satu sales GSI Orbit: dealer, chat, nomor WhatsApp, dan riwayatnya pindah ke sales tujuan, ejaan sumber hilang dari pilihan sales, dan impor berikutnya tetap tergabung. Bisa dipisah lagi kapan saja.</p>
        </div>
        <div className="smap-stats">
          <span><b>{counts.all}</b>nama di data</span>
          <span><b>{counts.open}</b>sales GSI Orbit</span>
          <span><b>{counts.merged}</b>sudah digabung</span>
          <span className={counts.suggest ? 'warn' : ''}><b>{counts.suggest}</b>saran gabung</span>
        </div>
      </div>

      <div className="card smap-new">
        <div className="card-h"><h2>{pickedRows.length ? `Gabungkan ${pickedRows.length} nama terpilih` : 'Gabungkan nama sales'}</h2><span className="meta">centang nama di tabel, lalu pilih sales tujuan</span></div>
        {pickedRows.length > 0 && <div className="smap-picked">{pickedRows.map((x) => <span key={x.id} className="chip">{x.name}<small>{x.branch} · {x.dealers} dealer</small><button type="button" onClick={() => toggle(x.id)} aria-label={`Lepas ${x.name}`}><Icon name="x" /></button></span>)}</div>}
        <div className="smap-row">
          <label>Ke sales yang sudah ada
            <select value={target} onChange={(e) => setTarget(e.target.value)} aria-label="Sales tujuan">
              <option value="">Pilih sales tujuan…</option>
              {targets.map((x) => <option key={x.id} value={x.id}>{x.name} · {x.branch} · {total(x)} dealer{x.source_system !== 'import' ? ' · GSI Orbit' : ''}</option>)}
            </select>
          </label>
          <button type="button" className="btn primary" disabled={busy || !target || !pickedRows.some((x) => x.id !== target)} onClick={mergePicked}><Icon name="net" />Gabungkan</button>
        </div>
        <div className="smap-or"><span>atau buat sales GSI Orbit baru</span></div>
        <form className="smap-row" onSubmit={(e) => { e.preventDefault(); createAndMerge() }}>
          <label>Nama di GSI Orbit<input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="mis. Granike Monika" aria-label="Nama sales baru" /></label>
          <label>Cabang<input value={newBranch} onChange={(e) => setNewBranch(e.target.value)} placeholder="mis. Semarang" aria-label="Cabang sales baru" /></label>
          <button type="submit" className="btn ghost" disabled={busy || !newName.trim()}><Icon name="check" />{pickedRows.length ? `Buat & gabungkan ${pickedRows.length}` : 'Buat sales'}</button>
        </form>
      </div>

      <div className="card">
        <div className="seg smap-views">
          {([['all', 'Semua'], ['suggest', 'Saran gabung'], ['open', 'Sales GSI Orbit'], ['merged', 'Sudah digabung']] as const).map(([k, l]) => (
            <button key={k} type="button" className={view === k ? 'is-active' : ''} onClick={() => setView(k)}>{l} <small>{counts[k]}</small></button>
          ))}
        </div>
        <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari nama, kode, cabang…" label="Cari sales" meta={`${t.matches.length} nama`} />
        <div className="odl-wrap">
          <table className="odl smap-table">
            <thead>
              <tr>
                <th style={{ width: 34 }} aria-label="Pilih" />
                <SortTh t={t} c="name">Nama di data sumber</SortTh>
                <SortTh t={t} c="branch">Cabang</SortTh>
                <SortTh t={t} c="dealers" right>Dealer</SortTh>
                <SortTh t={t} c="status">Digabung ke</SortTh>
                <th className="r">Aksi</th>
              </tr>
            </thead>
            <tbody>
              {isLoading && <tr><td colSpan={6} className="muted">Memuat…</td></tr>}
              {!isLoading && t.shown.length === 0 && <tr><td colSpan={6} className="muted">{view === 'suggest' ? 'Tidak ada nama yang mirip — semua sudah rapi.' : 'Tidak ada nama.'}</td></tr>}
              {t.shown.map((p) => {
                const s = sugg.get(p.id)
                const kids = childrenOf.get(p.id) ?? []
                return (
                  <tr key={p.id} className={picked.has(p.id) ? 'is-picked' : p.merged_into ? 'is-merged' : ''}>
                    <td><input type="checkbox" checked={picked.has(p.id)} disabled={!!p.merged_into} onChange={() => toggle(p.id)} aria-label={`Pilih ${p.name}`} /></td>
                    <td>
                      <b>{p.name}</b>
                      <div className="smap-sub">
                        <span className={`smap-src ${p.source_system === 'import' ? '' : 'own'}`}>{srcLabel(p)}{p.source_id && p.source_system ? ` · ${p.source_id}` : ''}</span>
                        {p.login_email && <span>login {p.login_email}</span>}
                        {kids.length > 0 && <span>+ {kids.length} ejaan digabung ({kids.map((k) => k.name).join(', ')})</span>}
                        {s && <button type="button" className="smap-hint" onClick={() => pickGroup(p)} title="Pilih nama-nama yang mirip untuk digabung">mirip {s.map((x) => x.name).join(', ')} · pilih</button>}
                      </div>
                    </td>
                    <td>{p.branch}</td>
                    <td className="r num">{p.dealers.toLocaleString('id-ID')}{kids.length ? <small className="muted"> / {total(p).toLocaleString('id-ID')}</small> : null}</td>
                    <td>{p.merged_into ? <span className="smap-to"><Icon name="arrow" />{statusOf(p).slice(2)}</span> : <span className="muted">— sales utama</span>}</td>
                    <td className="r">
                      {p.merged_into
                        ? <button type="button" className="btn quiet" disabled={busy} onClick={() => merge([p.id], null)}>Pisahkan</button>
                        : <select className="smap-quick" value="" disabled={busy} onChange={(e) => e.target.value && merge([p.id], e.target.value)} aria-label={`Gabungkan ${p.name} ke`}>
                            <option value="">Gabung ke…</option>
                            {targets.filter((x) => x.id !== p.id).map((x) => <option key={x.id} value={x.id}>{x.name} · {x.branch}</option>)}
                          </select>}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <TablePager t={t} noun="nama" />
      </div>
    </div>
  )
}
