import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { ColumnTable, type Column } from '../../components/DataTable'
import { useMe } from '../../app/queries'
import { field, useRoles, type RoleCatalog, type RoleRow } from './api'

function RoleSheet({ role, cat }: { role?: RoleRow; cat: RoleCatalog }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: me } = useMe()
  const edit = !!me?.edit_policies
  const [f, setF] = useState({
    name: role?.name ?? '', description: role?.description ?? '', scope: role?.scope ?? 'all', screens: role?.screens ?? ['today', 'konsep'],
    decide: role?.decide ?? [], policies: role?.policies ?? false, wa_allowed: role?.wa_allowed ?? true, active: role?.active ?? true,
    chat_scope: role?.chat_scope ?? 'all',
  })
  const flip = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
  const screenOff = (s: RoleCatalog['screens'][number]) => s.all_data && f.scope === 'own'
  const kindOff = (k: RoleCatalog['kinds'][number]) => k.policies && !f.policies
  const groups = [...new Set(cat.screens.map((s) => s.group))]
  const save = useMutation({
    mutationFn: () => {
      const body = { ...f, screens: f.screens.filter((k) => !screenOff(cat.screens.find((s) => s.key === k)!)), decide: f.decide.filter((k) => !kindOff(cat.kinds.find((x) => x.key === k)!)) }
      return role ? api.put(`/roles/${role.key}`, body) : api.post('/roles', body)
    },
    onSuccess: (r) => {
      toast((r as { message?: string }).message ?? 'Peran disimpan')
      for (const k of ['roles', 'users', 'me']) qc.invalidateQueries({ queryKey: [k] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const remove = useMutation({
    mutationFn: () => api.del(`/roles/${role!.key}`),
    onSuccess: () => { toast('Peran dihapus'); qc.invalidateQueries({ queryKey: ['roles'] }); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <>
      <SheetHead icon="shield" title={role ? role.name : 'Tambah peran'} sub={edit ? 'Centang halaman dan keputusan yang boleh untuk peran ini' : 'Hanya pemegang hak kebijakan (CEO) yang mengubah peran'} onClose={closeSheet} />
      <div className="sec frm">
        <label>Nama peran<input style={field} value={f.name} disabled={!edit} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="mis. Sales Telemarketing" /></label>
        <label>Keterangan<input style={field} value={f.description} disabled={!edit} onChange={(e) => setF({ ...f, description: e.target.value })} placeholder="mis. Follow-up dealer lewat WhatsApp" /></label>
      </div>
      <div className="sec">
        <h4>Cakupan data</h4>
        <div className="role-bases">
          <button className={`role-base ${f.scope === 'all' ? 'is-active' : ''}`} disabled={!edit} onClick={() => setF({ ...f, scope: 'all' })}><b>Semua data</b><span>Semua dealer, semua chat, semua cabang</span></button>
          <button className={`role-base ${f.scope === 'own' ? 'is-active' : ''}`} disabled={!edit || f.policies} onClick={() => setF({ ...f, scope: 'own' })}><b>Hanya data miliknya</b><span>Dealer yang ia pegang dan chat nomor WhatsApp-nya sendiri</span></button>
        </div>
      </div>
      <div className="sec">
        <h4>Akses halaman <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· menu di sidebar, juga dikunci di server</span>
          {edit && <span style={{ marginLeft: 'auto', display: 'flex', gap: 10, textTransform: 'none', letterSpacing: 0 }}><button className="lnk" onClick={() => setF({ ...f, screens: cat.screens.filter((s) => !screenOff(s)).map((s) => s.key) })}>Pilih semua</button><button className="lnk" onClick={() => setF({ ...f, screens: [] })}>Kosongkan</button></span>}</h4>
        {groups.map((g) => (
          <div key={g} className="role-group">
            <span className="role-gl">{g}</span>
            <div className="role-checks">
              {cat.screens.filter((s) => s.group === g).map((s) => (
                <label key={s.key} className={`role-check ${screenOff(s) ? 'off' : ''}`} title={screenOff(s) ? 'Butuh cakupan "Semua data"' : ''}>
                  <input type="checkbox" disabled={!edit || screenOff(s)} checked={f.screens.includes(s.key) && !screenOff(s)} onChange={() => setF({ ...f, screens: flip(f.screens, s.key) })} />{s.label}
                </label>
              ))}
            </div>
          </div>
        ))}
      </div>
      <div className="sec">
        <h4>Boleh memutuskan <span style={{ fontWeight: 500, textTransform: 'none', letterSpacing: 0 }}>· saran AI yang boleh disetujui / ditolak</span></h4>
        <div className="role-checks">
          {cat.kinds.map((k) => (
            <label key={k.key} className={`role-check ${kindOff(k) ? 'off' : ''}`} title={kindOff(k) ? 'Butuh hak kebijakan (setara CEO)' : ''}>
              <input type="checkbox" disabled={!edit || kindOff(k)} checked={f.decide.includes(k.key) && !kindOff(k)} onChange={() => setF({ ...f, decide: flip(f.decide, k.key) })} />{k.label}
            </label>
          ))}
        </div>
      </div>
      <div className="sec">
        <ul className="rules">
          <li><div><b>Hak kebijakan (setara CEO)</b><span>Ubah kebijakan, otonomi, sumber data dan peran; rilis kredit di atas limit; selalu semua data</span></div>
            <button className={`sw ${f.policies ? 'on' : ''}`} aria-label="Hak kebijakan" disabled={!edit} onClick={() => setF({ ...f, policies: !f.policies, scope: !f.policies ? 'all' : f.scope })} /></li>
          <li><div><b>Memegang nomor WhatsApp</b><span>Pengguna peran ini bisa diberi satu nomor di Chat dan membalas dari nomor itu</span></div>
            <button className={`sw ${f.wa_allowed ? 'on' : ''}`} aria-label="Memegang nomor WhatsApp" disabled={!edit} onClick={() => setF({ ...f, wa_allowed: !f.wa_allowed })} /></li>
          <li><div><b>Chat WhatsApp hanya nomor sendiri</b><span>{f.scope === 'own' ? 'Peran dengan data miliknya selalu hanya membaca chat nomornya sendiri' : 'Matikan untuk membaca chat semua nomor; nyalakan agar hanya nomor yang dipegang pengguna (seperti sales)'}</span></div>
            <button className={`sw ${f.chat_scope === 'own' || f.scope === 'own' ? 'on' : ''}`} aria-label="Chat WhatsApp hanya nomor sendiri" disabled={!edit || f.scope === 'own'} onClick={() => setF({ ...f, chat_scope: f.chat_scope === 'own' ? 'all' : 'own' })} /></li>
          {role && <li><div><b>Aktif</b><span>Peran nonaktif tidak bisa dipilih untuk pengguna baru</span></div>
            <button className={`sw ${f.active ? 'on' : ''}`} aria-label="Peran aktif" disabled={!edit} onClick={() => setF({ ...f, active: !f.active })} /></li>}
        </ul>
      </div>
      <div className="ft">
        {edit && <button className="btn primary" disabled={save.isPending || !f.name.trim()} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>}
        {edit && role && <button className="btn ghost" style={{ color: 'var(--bad)' }} disabled={role.users > 0} title={role.users > 0 ? 'Masih dipakai pengguna' : ''} onClick={() => { if (window.confirm(`Hapus peran ${role.name}?`)) remove.mutate() }}>Hapus</button>}
        <button className="btn quiet" onClick={closeSheet}>{edit ? 'Batal' : 'Tutup'}</button>
      </div>
    </>
  )
}

/** Master data → Peran & akses: custom roles (pages, data scope, decisions, policy rights, WhatsApp). */
const byText = (f: (r: RoleRow) => string) => (a: RoleRow, b: RoleRow) => f(a).localeCompare(f(b), 'id')

// A data table (20 Oct): search, sort per column, pages of 10/25/50 — like Pengguna.
export function RolesPage() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const { data: cat } = useRoles()
  if (!cat) return null
  const label = (k: string) => cat.screens.find((s) => s.key === k)?.label ?? k
  const columns: Column<RoleRow>[] = [
    { key: 'name', label: 'Peran', sort: byText((r) => r.name), cell: (r) => <><b>{r.name}</b>{r.policies && <> <Pill tone="accent">Kebijakan</Pill></>}<div className="mono">{r.description}</div></> },
    { key: 'screens', label: 'Halaman', sort: (a, b) => a.screens.length - b.screens.length, first: 'desc', className: 'role-screens', cell: (r) => <div className="role-tags">{r.screens.map((s) => <span key={s}>{label(s)}</span>)}</div> },
    { key: 'scope', label: 'Data', sort: byText((r) => r.scope), cell: (r) => (r.scope === 'own' ? 'Miliknya' : 'Semua') },
    { key: 'decide', label: 'Keputusan', right: true, sort: (a, b) => a.decide.length - b.decide.length, first: 'desc', cell: (r) => <span className="num">{r.decide.length}</span> },
    { key: 'wa', label: 'WhatsApp', sort: (a, b) => Number(b.wa_allowed) - Number(a.wa_allowed), cell: (r) => <>{r.wa_allowed ? '1 nomor' : '—'}<div className="mono">{r.chat_scope === 'own' ? 'chat nomor sendiri' : 'chat semua nomor'}</div></> },
    { key: 'users', label: 'Pengguna', right: true, sort: (a, b) => a.users - b.users, first: 'desc', cell: (r) => <span className="num">{r.users}</span> },
    { key: 'status', label: 'Status', sort: (a, b) => Number(b.active) - Number(a.active), cell: (r) => <Pill tone={r.active ? 'good' : 'neutral'}>{r.active ? 'Aktif' : 'Nonaktif'}</Pill> },
    { key: 'act', label: '', right: true, cell: (r) => <button className="btn quiet" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<RoleSheet role={r} cat={cat} />)}>{me?.edit_policies ? 'Ubah' : 'Lihat'}</button> },
  ]
  return (
    <div className="card">
      <div className="card-h"><h2>Peran &amp; akses</h2><span className="meta">{cat.items.length} peran · akses tiap halaman bisa dicentang</span>
        {me?.edit_policies && <button className="btn primary" style={{ height: 32, fontSize: 12.5, marginLeft: 8 }} onClick={() => openSheet(<RoleSheet cat={cat} />)}><Icon name="shield" />Tambah peran</button>}</div>
      <ColumnTable rows={cat.items} columns={columns} rowKey={(r) => r.key} noun="peran" placeholder="Cari peran, halaman, keterangan…"
        text={(r) => [r.name, r.description, r.screens.map(label).join(' '), r.scope === 'own' ? 'miliknya' : 'semua', r.policies ? 'kebijakan' : '', r.active ? 'aktif' : 'nonaktif'].join(' ')}
        initial={{ column: 'name', dir: 'asc' }} empty="Belum ada peran." rowClass={(r) => (r.active ? undefined : 'is-off')} />
    </div>
  )
}
