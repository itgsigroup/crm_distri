import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { SOWRow } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtRp, shortName } from '../../lib/format'

/** Dealer → Konfirmasi share of wallet: sales confirm the 20 biggest dealers once per quarter (stage 14). */
export function SOWSheet() {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data } = useQuery({ queryKey: ['sow', 'top'], queryFn: () => api.get<{ quarter: string; branch: string; items: SOWRow[] }>('/sow/top?limit=20') })
  const [vals, setVals] = useState<Record<string, string>>({})
  const changed = Object.entries(vals).filter(([, v]) => v !== '')
  const save = useMutation({
    mutationFn: () => api.post<{ message: string }>('/sow/confirm', { items: changed.map(([id, v]) => ({ dealer_id: id, sow: Number(v) })) }),
    onSuccess: (r) => {
      toast(r.message)
      for (const k of ['dealers', 'dealer', 'orbit', 'segmen', 'sow']) qc.invalidateQueries({ queryKey: [k] })
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const input = { width: 64, height: 30, borderRadius: 8, border: '1px solid var(--line)', padding: '0 8px', font: 'inherit', textAlign: 'right' as const, background: 'var(--surface)', color: 'var(--text)' }
  return (
    <>
      <SheetHead icon="check" title="Konfirmasi share of wallet" sub={`${data?.quarter ?? ''}${data?.branch ? ' · cabang ' + data.branch : ''} · porsi belanja dealer di GSI menurut sales`} onClose={closeSheet} />
      <div className="sec">
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Dealer</th><th>Omzet/bln</th><th>Sekarang</th><th>Konfirmasi %</th></tr></thead>
            <tbody>
              {(data?.items ?? []).map((d) => (
                <tr key={d.id}>
                  <td><b>{shortName(d.name)}</b><div className="mono">{d.sales_name ?? '—'}</div></td>
                  <td className="num">{fmtRp(d.omzet_bln)}</td>
                  <td className="num">{d.sow}% <span style={{ fontSize: 11, color: 'var(--text-3)' }}>{d.confirmed_sow != null ? 'dikonfirmasi' : d.sow_source === 'estimated' ? 'estimasi' : 'default'}</span></td>
                  <td><input style={input} type="number" min={0} max={100} aria-label={`Share of wallet ${d.name}`} placeholder={String(d.confirmed_sow ?? d.sow)} value={vals[d.id] ?? ''} onChange={(e) => setVals({ ...vals, [d.id]: e.target.value })} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="ft">
        <button className="btn primary" disabled={!changed.length || save.isPending} onClick={() => save.mutate()}><Icon name="check" />Simpan {changed.length || ''}</button>
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Konfirmasi terbaru menggantikan estimasi · skor dihitung ulang</span>
      </div>
    </>
  )
}
