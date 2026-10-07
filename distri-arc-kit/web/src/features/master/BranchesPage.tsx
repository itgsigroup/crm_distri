import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
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
export function BranchesPage() {
  const { data: me } = useMe()
  const { openSheet } = useFeedback()
  const { data = [] } = useBranchMaster()
  const edit = !!me?.screens.includes('branches')
  return (
    <div className="card">
      <div className="card-h"><h2>Cabang</h2><span className="meta">{data.filter((b) => b.active).length} cabang aktif</span>
        {edit && <button className="btn primary" style={{ height: 32, fontSize: 12.5, marginLeft: 8 }} onClick={() => openSheet(<BranchSheet />)}><Icon name="building" />Tambah cabang</button>}</div>
      {data.length === 0 ? <p style={{ fontSize: 13, color: 'var(--text-3)', margin: 0 }}>Belum ada cabang — tambahkan cabang pertama.</p> : (
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Cabang</th><th>Kota</th><th>Alamat</th><th>Pengguna</th><th>Dealer</th><th /></tr></thead>
            <tbody>{data.map((b) => (
              <tr key={b.id} style={b.active ? undefined : { opacity: 0.5 }}>
                <td><b>{b.name}</b>{!b.active && <> <Pill tone="neutral">Nonaktif</Pill></>}</td>
                <td>{b.city || '—'}</td><td style={{ maxWidth: 280 }}>{b.address || '—'}</td>
                <td className="num">{b.users}</td><td className="num">{b.dealers}</td>
                <td style={{ textAlign: 'right' }}>{edit && <button className="btn quiet" style={{ height: 28, fontSize: 12 }} onClick={() => openSheet(<BranchSheet branch={b} />)}>Ubah</button>}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      )}
    </div>
  )
}
