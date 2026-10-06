import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'

interface ImportResult { imported: number; skipped: string[] | null }

/** Pengaturan → Identifikasi nomor: sources and the manual Getcontact CSV import. */
export function IdentifyPanel() {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [csv, setCsv] = useState('')
  const [res, setRes] = useState<ImportResult | null>(null)
  const imp = useMutation({
    mutationFn: async () => {
      const r = await fetch('/api/identifications/import', { method: 'POST', headers: { 'Content-Type': 'text/csv' }, body: csv })
      const data = await r.json()
      if (!r.ok) throw new Error(data?.error?.message ?? r.statusText)
      return data as ImportResult
    },
    onSuccess: (r) => {
      setRes(r)
      toast(`${r.imported} nomor Getcontact diimpor`)
      qc.invalidateQueries({ queryKey: ['chat'] })
    },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <>
      <SheetHead icon="people" title="Identifikasi nomor" sub="Hanya nomor yang menghubungi nomor sales lebih dulu" onClose={closeSheet} />
      <div className="sec">
        <h4>Sumber</h4>
        <ul className="ext">
          <li><Icon name="check" /><span>Profil WA Business — dibaca lewat nomor sales yang menerima pesan</span></li>
          <li><Icon name="alert" /><span>Truecaller — menunggu akses API (sementara tidak dipakai)</span></li>
          <li><Icon name="check" /><span>Getcontact — impor manual CSV di bawah</span></li>
          <li><Icon name="check" /><span>Odoo — kontak dealer yang sudah ada</span></li>
        </ul>
      </div>
      <div className="sec">
        <h4>Impor Getcontact (CSV)</h4>
        <p style={{ fontSize: 12.5, color: 'var(--text-2)', margin: '0 0 8px' }}>Kolom: <code>nomor,nama,tag</code>. Satu baris per nomor.</p>
        <textarea className="prev" style={{ width: '100%', minHeight: 110, font: 'inherit', fontSize: 13, color: 'var(--text)' }} value={csv} onChange={(e) => setCsv(e.target.value)} placeholder={'nomor,nama,tag\n6282212343310,Mandiri Elektronik Pati,2'} />
        <input type="file" accept=".csv,text/csv" style={{ marginTop: 8, fontSize: 12 }} onChange={(e) => { const f = e.target.files?.[0]; if (f) void f.text().then(setCsv) }} />
        {res && <p style={{ fontSize: 12.5, marginTop: 8 }}>{res.imported} diimpor{res.skipped?.length ? ` · dilewati: ${res.skipped.join('; ')}` : ''}</p>}
      </div>
      <div className="ft">
        <button className="btn primary" disabled={!csv.trim() || imp.isPending} onClick={() => imp.mutate()}><Icon name="check" />Impor</button>
        <button className="btn quiet" onClick={closeSheet}>Tutup</button>
        <span className="spacer" />
        <span className="pol"><Icon name="lock" />Identifikasi hanya untuk nomor inbound</span>
      </div>
    </>
  )
}
