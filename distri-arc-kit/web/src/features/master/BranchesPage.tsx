import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { ColumnTable, type Column } from '../../components/DataTable'
import { useMe } from '../../app/queries'
import { field, useBranchMaster, type Branch } from './api'

function BranchSheet({ branch }: { branch?: Branch }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [f, setF] = useState({ name: branch?.name ?? '', city: branch?.city ?? '', address: branch?.address ?? '', active: branch?.active ?? true })
  const save = useMutation({
    mutationFn: () => (branch ? api.put(`/branches/${branch.id}`, f) : api.post('/branches', f)),
    onSuccess: (r) => {
      toast((r as { message?: string }).message ?? 'Cabang disimpan')
      for (const k of ['branches', 'users', 'data']) qc.invalidateQueries({ queryKey: [k] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <>
      <SheetHead icon="building" title={branch ? branch.name : 'Tambah cabang'} sub={branch ? `${branch.users} pengguna · ${branch.dealers} dealer` : 'Cabang dipakai untuk pengguna, dealer, stok, dan mapping data'} onClose={closeSheet} />
      <div className="sec frm">
        <label>Nama cabang<input style={field} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="mis. Semarang" /></label>
        <label>Kota<input style={field} value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} placeholder="mis. Semarang" /></label>
        <label style={{ gridColumn: '1 / -1' }}>Alamat<input style={field} value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} placeholder="Alamat kantor / gudang (opsional)" /></label>
        {branch && branch.name !== f.name.trim() && f.name.trim() && <small style={{ gridColumn: '1 / -1', color: 'var(--warn)' }}>Nama baru ikut diterapkan ke {branch.users} pengguna, {branch.dealers} dealer, dan mapping data cabang/gudang.</small>}
      </div>
      {branch && (
        <div className="sec"><ul className="rules"><li><div><b>Aktif</b><span>Cabang nonaktif tidak muncul di pilihan</span></div>
          <button className={`sw ${f.active ? 'on' : ''}`} aria-label="Cabang aktif" onClick={() => setF({ ...f, active: !f.active })} /></li></ul></div>
      )}
      <div className="ft">
        <button className="btn primary" disabled={save.isPending || !f.name.trim()} onClick={() => save.mutate()}><Icon name="check" />Simpan</button>
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
      </div>
    </>
  )
}

/** Master data → Cabang. */
const byText = (f: (b: Branch) => string) => (a: Branch, b: Branch) => f(a).localeCompare(f(b), 'id')

// A data table (20 Oct): search, sort per column, pages of 10/25/50 — like Pengguna and Peran & akses.
export function BranchesPage() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const { data = [] } = useBranchMaster()
  const edit = !!me?.screens.includes('branches')
  const columns: Column<Branch>[] = [
    { key: 'name', label: 'Cabang', sort: byText((b) => b.name), cell: (b) => <b>{b.name}</b> },
    { key: 'city', label: 'Kota', sort: byText((b) => b.city || '~'), cell: (b) => b.city || '—' },
    { key: 'address', label: 'Alamat', className: 'br-addr', cell: (b) => b.address || '—' },
    { key: 'users', label: 'Pengguna', right: true, sort: (a, b) => a.users - b.users, first: 'desc', cell: (b) => <span className="num">{b.users}</span> },
    { key: 'dealers', label: 'Dealer', right: true, sort: (a, b) => a.dealers - b.dealers, first: 'desc', cell: (b) => <span className="num">{b.dealers.toLocaleString('id-ID')}</span> },
    { key: 'status', label: 'Status', sort: (a, b) => Number(b.active) - Number(a.active), cell: (b) => <Pill tone={b.active ? 'good' : 'neutral'}>{b.active ? 'Aktif' : 'Nonaktif'}</Pill> },
    { key: 'act', label: '', right: true, cell: (b) => edit && <button className="btn quiet" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<BranchSheet branch={b} />)}>Ubah</button> },
  ]
  return (
    <div className="card">
      <div className="card-h"><h2>Cabang</h2><span className="meta">{data.filter((b) => b.active).length} cabang aktif</span>
        {edit && <button className="btn primary" style={{ height: 32, fontSize: 12.5, marginLeft: 8 }} onClick={() => openSheet(<BranchSheet />)}><Icon name="building" />Tambah cabang</button>}</div>
      <ColumnTable rows={data} columns={columns} rowKey={(b) => b.id} noun="cabang" placeholder="Cari cabang, kota, alamat…"
        text={(b) => [b.name, b.city, b.address, b.active ? 'aktif' : 'nonaktif'].join(' ')}
        initial={{ column: 'name', dir: 'asc' }} empty="Belum ada cabang — tambahkan cabang pertama." rowClass={(b) => (b.active ? undefined : 'is-off')} />
    </div>
  )
}
