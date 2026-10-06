import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { AutonomyPolicy, AutonomyRow } from '../../api/types'
import { useFeedback } from '../../components/feedback'
import { useAutonomy, useMe } from '../../app/queries'

export const KIND_LABEL: Record<string, string> = {
  followup: 'follow-up', collect: 'pengingat', installment: 'cicilan', credit_release: 'rilis kredit', credit_limit: 'ubah limit', credit_hold: 'tahan SO',
  price_counter: 'harga khusus', push_stock: 'bundle stok', so_draft: 'SO draft', return: 'retur', transfer: 'transfer', po_request: 'PO', new_dealer: 'dealer baru',
  price_list: 'kirim harga', stock_list: 'daftar kandidat', identify: 'identifikasi',
}
const LOCKED = new Set(['credit_release', 'credit_limit'])

/** Pengaturan → Matriks otonomi: click a kind to move it between Otonom and Butuh approve (CEO). */
export function AutonomyCard() {
  const { data: pol } = useAutonomy()
  const { data: me } = useMe()
  const { toast } = useFeedback()
  const qc = useQueryClient()
  const save = useMutation({
    mutationFn: (body: Partial<AutonomyPolicy>) => api.put<AutonomyPolicy>('/policies/autonomy', body),
    onSuccess: () => {
      toast('Matriks otonomi disimpan · berlaku di siklus berikutnya')
      for (const k of ['policies', 'agents']) qc.invalidateQueries({ queryKey: [k] })
    },
    onError: (e: Error) => toast(e.message),
  })
  if (!pol) return null
  const canEdit = !!me?.edit_policies
  const move = (agent: string, kind: string, from: 'auto' | 'approve') => {
    if (!canEdit) return toast('Hanya CEO yang mengubah matriks otonomi')
    if (from === 'approve' && LOCKED.has(kind)) return toast(`${KIND_LABEL[kind]} tidak pernah otonom — terkunci`)
    const row: AutonomyRow = pol.matrix[agent]
    const to = from === 'auto' ? 'approve' : 'auto'
    const next = { ...row, [from]: row[from].filter((k) => k !== kind), [to]: row[to].includes(kind) ? row[to] : [...row[to], kind] }
    save.mutate({ matrix: { ...pol.matrix, [agent]: next } })
  }
  const order = pol.order.filter((a) => pol.matrix[a])
  return (
    <div className="card">
      <div className="card-h"><h2>Matriks otonomi</h2><span className="meta">{canEdit ? 'klik jenis untuk memindah kolom' : 'dibaca Orchestrator sebelum membagi tugas'}</span></div>
      <div className="tbl-wrap">
        <table className="matrix">
          <thead><tr><th>Agen</th><th>Otonom</th><th>Butuh approve</th><th>Tidak boleh</th></tr></thead>
          <tbody>
            {order.map((a) => {
              const row = pol.matrix[a]
              return (
                <tr key={a}>
                  <td><b>{a}</b></td>
                  <td>{row.auto.map((k) => <button key={k} className="chip g" style={{ margin: 2 }} onClick={() => move(a, k, 'auto')}>{KIND_LABEL[k] ?? k}</button>)}</td>
                  <td>{row.approve.filter((k) => !row.auto.includes(k)).map((k) => <button key={k} className="chip w" style={{ margin: 2 }} onClick={() => move(a, k, 'approve')}>{KIND_LABEL[k] ?? k}</button>)}</td>
                  <td><span className="chip b">{row.labels?.never ?? row.never.join(', ')}</span></td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <ul className="rules" style={{ marginTop: 10 }}>
        <li><div><b>Kirim pesan otonom ke dealer</b><span>{pol.guard.dealer_messages === 'auto' ? 'Langkah otonom dikirim Orchestrator pada jamnya' : 'Langkah otonom menunggu klik “Jalankan sekarang” (ADR 0008)'} · confidence ≥ {pol.guard.min_confidence}</span></div>
          <div className="seg">
            <button className={pol.guard.dealer_messages === 'confirm' ? 'is-active' : ''} onClick={() => (canEdit ? save.mutate({ guard: { ...pol.guard, dealer_messages: 'confirm' } }) : toast('Hanya CEO'))}>Konfirmasi</button>
            <button className={pol.guard.dealer_messages === 'auto' ? 'is-active' : ''} onClick={() => (canEdit ? save.mutate({ guard: { ...pol.guard, dealer_messages: 'auto' } }) : toast('Hanya CEO'))}>Otomatis</button>
          </div>
        </li>
      </ul>
    </div>
  )
}
