import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { candidates, looksLike, namesByUser, type MapUser, type SalesProfile } from './match'

type Column = 'name' | 'branch' | 'dealers' | 'user'
type View = 'all' | 'open' | 'linked'

interface SalesMap { items: SalesProfile[]; users: MapUser[] }
const EMPTY: SalesMap = { items: [], users: [] }
const useSalesMap = () => useQuery({ queryKey: ['sales-map'], queryFn: () => api.get<SalesMap>('/sales-map') })

const srcLabel = (p: SalesProfile) => (p.source_system === 'import' ? 'BigQuery' : p.source_system === 'odoo' ? 'Odoo' : 'GSI Orbit')
const initials = (s: string) => s.split(/\s+/).filter(Boolean).map((w) => w[0]).join('').slice(0, 2).toUpperCase() || '?'
const lower = (s: string) => s.toLocaleLowerCase('id-ID')

/** Master data → Mapping sales: one Pengguna (login account) ↔ many sales names from BigQuery. */
export function SalesMapPage() {
  const { data = EMPTY, isLoading } = useSalesMap()
  const rows = data.items
  const users = data.users
  const qc = useQueryClient()
  const { toast, alert } = useFeedback()
  const [userId, setUserId] = useState('')
  const [userQ, setUserQ] = useState('')
  const [addQ, setAddQ] = useState('')
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)
  const [view, setView] = useState<View>('all')

  const byUser = useMemo(() => namesByUser(rows), [rows])
  const userById = useMemo(() => new Map(users.map((u) => [u.id, u])), [users])
  const current = userById.get(userId) ?? users.find((u) => u.role === 'sales') ?? users[0]
  const mine = current ? byUser.get(current.id) ?? [] : []
  const dealersOf = (id: string) => (byUser.get(id) ?? []).reduce((a, r) => a + r.dealers, 0)
  const pool = useMemo(() => candidates(rows, current), [rows, current])
  const poolShown = pool.filter((r) => !addQ.trim() || lower(`${r.name} ${r.source_id ?? ''} ${r.branch}`).includes(lower(addQ.trim())))
  const usersShown = users.filter((u) => !userQ.trim() || lower(`${u.name} ${u.email ?? ''} ${u.role_name} ${u.branch}`).includes(lower(userQ.trim())))
  const linkedCount = rows.filter((r) => r.user_id && !r.main).length
  const openCount = rows.filter((r) => !r.user_id && !r.merged_into).length

  const refresh = () => {
    for (const k of ['sales-map', 'sales', 'dealers', 'data', 'agenda', 'kpi', 'wa', 'users', 'me']) qc.invalidateQueries({ queryKey: [k] })
  }
  const link = (sources: string[], user: MapUser) => {
    if (!sources.length) return
    setBusy(true)
    api.post<{ message: string }>('/sales-map/merge', { sources, user_id: user.id }).then((r) => {
      toast(r.message)
      setPicked(new Set())
      refresh()
    }, (e: Error) => alert({ icon: 'error', title: 'Belum terhubung', text: e.message })).finally(() => setBusy(false))
  }
  const unlink = (p: SalesProfile) => {
    setBusy(true)
    api.post<{ message: string }>('/sales-map/merge', { sources: [p.id], target_id: null }).then((r) => {
      toast(r.message)
      refresh()
    }, (e: Error) => alert({ icon: 'error', title: 'Belum dilepas', text: e.message })).finally(() => setBusy(false))
  }
  const toggle = (id: string) => setPicked((s) => {
    const n = new Set(s)
    if (n.has(id)) n.delete(id)
    else n.add(id)
    return n
  })
  const pickUser = (id: string) => {
    setUserId(id)
    setPicked(new Set())
    setAddQ('')
  }

  // every BigQuery name with the user it belongs to
  const tableRows = useMemo(() => rows.filter((r) => (view === 'all' ? !r.main || r.source_system === 'import' : view === 'open' ? !r.user_id && !r.merged_into : !!r.user_id && !r.main)), [rows, view])
  const userName = (r: SalesProfile) => (r.user_id ? userById.get(r.user_id)?.name ?? '' : '')
  const t = useDataTable<SalesProfile, Column>({
    rows: tableRows,
    text: useMemo(() => (p: SalesProfile) => `${p.name} ${p.source_id ?? ''} ${p.branch}`, []),
    compare: useMemo(() => ({
      name: (a: SalesProfile, b: SalesProfile) => a.name.localeCompare(b.name, 'id'),
      branch: (a: SalesProfile, b: SalesProfile) => a.branch.localeCompare(b.branch, 'id'),
      dealers: (a: SalesProfile, b: SalesProfile) => a.dealers - b.dealers,
      user: (a: SalesProfile, b: SalesProfile) => Number(!!a.user_id) - Number(!!b.user_id),
    }), []),
    tie: useMemo(() => (a: SalesProfile, b: SalesProfile) => a.name.localeCompare(b.name, 'id'), []),
    initial: { column: 'name', dir: 'asc' },
    firstDir: (c) => (c === 'dealers' ? 'desc' : 'asc'),
  })

  return (
    <div className="smap">
      <div className="card smap-head">
        <div>
          <h2>Mapping sales</h2>
          <p>Hubungkan <b>Pengguna</b> GSI Orbit dengan nama sales di data BigQuery (Accurate). Satu pengguna bisa memegang <b>banyak nama</b> BigQuery — mis. Pengguna <b>Granike Monika</b> ↔ "Granike Monica" dan "Granike Monica M.". Dealer, chat, dan riwayat dari nama-nama itu menjadi milik pengguna tersebut, nama BigQuery-nya hilang dari pilihan sales, dan impor berikutnya tetap terhubung.</p>
        </div>
        <div className="smap-stats">
          <span><b>{users.length}</b>pengguna</span>
          <span><b>{rows.filter((r) => r.source_system === 'import').length}</b>nama BigQuery</span>
          <span><b>{linkedCount}</b>terhubung</span>
          <span className={openCount ? 'warn' : ''}><b>{openCount}</b>belum terhubung</span>
        </div>
      </div>

      <div className="card smap-split">
        <div className="smap-users">
          <div className="smap-h">Pengguna</div>
          <div className="search smap-q"><Icon name="search" /><input value={userQ} onChange={(e) => setUserQ(e.target.value)} placeholder="Cari pengguna…" aria-label="Cari pengguna" /></div>
          <ul>
            {isLoading && <li className="muted">Memuat…</li>}
            {usersShown.map((u) => {
              const n = (byUser.get(u.id) ?? []).filter((r) => !r.main).length
              return (
                <li key={u.id}>
                  <button type="button" className={current?.id === u.id ? 'is-active' : ''} onClick={() => pickUser(u.id)}>
                    <span className="sp-av">{initials(u.name)}</span>
                    <span className="smap-u"><b>{u.name}</b><small>{u.role_name}{u.branch ? ` · ${u.branch}` : ''}</small></span>
                    <span className={`smap-n ${n ? '' : u.role === 'sales' ? 'warn' : 'muted'}`} title={`${n} nama BigQuery · ${dealersOf(u.id)} dealer`}>{n ? `${n} nama` : u.role === 'sales' ? 'belum' : '—'}</span>
                  </button>
                </li>
              )
            })}
          </ul>
        </div>

        <div className="smap-detail">
          {current ? (
            <>
              <div className="smap-dh">
                <span className="sp-av lg">{initials(current.name)}</span>
                <div><b>{current.name}</b><small>{[current.role_name, current.branch, current.email].filter(Boolean).join(' · ')}</small></div>
                <span className="smap-total"><b>{dealersOf(current.id).toLocaleString('id-ID')}</b>dealer</span>
              </div>

              <div className="smap-h">Data BigQuery milik {current.name.split(' ')[0]} <small>{mine.filter((r) => !r.main).length} nama</small></div>
              {mine.length === 0 && <p className="smap-empty">Belum ada nama BigQuery. Centang dari daftar di bawah lalu klik Hubungkan.</p>}
              <ul className="smap-linked">
                {mine.map((r) => (
                  <li key={r.id}>
                    <Icon name={r.main ? 'user-x' : 'net'} />
                    <span className="smap-u"><b>{r.name}</b><small>{srcLabel(r)}{r.source_id && r.source_system ? ` · ${r.source_id}` : ''} · {r.branch}</small></span>
                    <span className="num">{r.dealers.toLocaleString('id-ID')} dealer</span>
                    {r.main ? <span className="smap-main" title="Profil sales utama pengguna ini — nama BigQuery lain digabung ke sini">utama</span>
                      : <button type="button" className="btn quiet" disabled={busy} onClick={() => unlink(r)}>Lepas</button>}
                  </li>
                ))}
              </ul>

              <div className="smap-h">Tambah dari BigQuery <small>{pool.length} nama belum terhubung</small></div>
              <div className="search smap-q"><Icon name="search" /><input value={addQ} onChange={(e) => setAddQ(e.target.value)} placeholder="Cari nama BigQuery…" aria-label="Cari nama BigQuery" /></div>
              <ul className="smap-pool">
                {poolShown.length === 0 && <li className="muted">Tidak ada nama BigQuery yang belum terhubung{addQ ? ' dan cocok' : ''}.</li>}
                {poolShown.slice(0, 60).map((r) => (
                  <li key={r.id}>
                    <label className={picked.has(r.id) ? 'is-picked' : ''}>
                      <input type="checkbox" checked={picked.has(r.id)} onChange={() => toggle(r.id)} />
                      <span className="smap-u"><b>{r.name}</b><small>{srcLabel(r)}{r.source_id && r.source_system ? ` · ${r.source_id}` : ''} · {r.branch}{r.merged_count ? ` · +${r.merged_count} ejaan` : ''}</small></span>
                      {looksLike(current.name, r.name) && <span className="smap-hint">mirip</span>}
                      <span className="num">{r.dealers.toLocaleString('id-ID')} dealer</span>
                    </label>
                  </li>
                ))}
                {poolShown.length > 60 && <li className="muted">+{poolShown.length - 60} nama lagi — persempit dengan pencarian</li>}
              </ul>
              <div className="smap-act">
                <button type="button" className="btn primary" disabled={busy || picked.size === 0} onClick={() => link([...picked], current)}>
                  <Icon name="net" />{picked.size ? `Hubungkan ${picked.size} nama ke ${current.name}` : 'Centang nama BigQuery'}
                </button>
              </div>
            </>
          ) : <p className="smap-empty">Belum ada pengguna aktif. Tambahkan di Master data → Pengguna.</p>}
        </div>
      </div>

      <div className="card">
        <div className="card-h"><h2>Semua nama BigQuery</h2><span className="meta">pilih pengguna di kolom Pengguna untuk menghubungkan langsung</span></div>
        <div className="seg smap-views">
          {([['all', 'Semua'], ['open', 'Belum terhubung'], ['linked', 'Sudah terhubung']] as const).map(([k, l]) => (
            <button key={k} type="button" className={view === k ? 'is-active' : ''} onClick={() => setView(k)}>{l}</button>
          ))}
        </div>
        <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari nama, kode, cabang…" label="Cari nama BigQuery" meta={`${t.matches.length} nama`} />
        <div className="odl-wrap">
          <table className="odl smap-table">
            <thead>
              <tr>
                <SortTh t={t} c="name">Nama di BigQuery</SortTh>
                <SortTh t={t} c="branch">Cabang</SortTh>
                <SortTh t={t} c="dealers" right>Dealer</SortTh>
                <SortTh t={t} c="user">Pengguna</SortTh>
                <th className="r">Aksi</th>
              </tr>
            </thead>
            <tbody>
              {!isLoading && t.shown.length === 0 && <tr><td colSpan={5} className="muted">Tidak ada nama.</td></tr>}
              {t.shown.map((r) => (
                <tr key={r.id} className={r.user_id ? 'is-merged' : ''}>
                  <td><b>{r.name}</b><div className="smap-sub"><span className={`smap-src ${r.source_system === 'import' ? '' : 'own'}`}>{srcLabel(r)}{r.source_id && r.source_system ? ` · ${r.source_id}` : ''}</span></div></td>
                  <td>{r.branch}</td>
                  <td className="r num">{r.dealers.toLocaleString('id-ID')}</td>
                  <td>
                    {r.main ? <span className="smap-to"><Icon name="user-x" />{userName(r)} <small className="muted">· utama</small></span>
                      : r.user_id ? <span className="smap-to"><Icon name="arrow" />{userName(r)}</span>
                      : r.merged_into ? <span className="muted">digabung ke {rows.find((x) => x.id === r.merged_into)?.name ?? '—'}</span>
                      : <select className="smap-quick" value="" disabled={busy} onChange={(e) => { const u = userById.get(e.target.value); if (u) link([r.id], u) }} aria-label={`Hubungkan ${r.name} ke pengguna`}>
                          <option value="">Pilih pengguna…</option>
                          {users.map((u) => <option key={u.id} value={u.id}>{u.name} · {u.role_name}{looksLike(u.name, r.name) ? ' · mirip' : ''}</option>)}
                        </select>}
                  </td>
                  <td className="r">{r.user_id && !r.main ? <button type="button" className="btn quiet" disabled={busy} onClick={() => unlink(r)}>Lepas</button> : null}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <TablePager t={t} noun="nama" />
      </div>
    </div>
  )
}
