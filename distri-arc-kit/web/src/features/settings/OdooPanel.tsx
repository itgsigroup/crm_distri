import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { hhmm, shortDate } from '../../lib/format'
import { useConnections, useOdooCategories } from '../../app/queries'

const MODEL: Record<string, string> = {
  'res.partner': 'Dealer & kontak', 'sale.order': 'Sales order', 'stock.picking': 'Pengiriman', 'account.move': 'Invoice',
  'account.payment': 'Pembayaran', 'stock.quant': 'Stok per cabang', 'product.product': 'Produk & harga tier',
}

/** Pengaturan → Sumber sinyal → Odoo: read-only sync status, connection test, manual sync, category map. */
export function OdooPanel() {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const { data: conn } = useConnections()
  const { data: cats } = useOdooCategories()
  const test = useMutation({
    mutationFn: () => api.post<{ ok: boolean; version?: string; error?: string; mode: string }>('/connections/odoo/test'),
    onSuccess: (r) => toast(r.ok ? `Odoo ${r.version} · ${r.mode} · tersambung` : `Gagal: ${r.error}`),
  })
  const sync = useMutation({
    mutationFn: (full: boolean) => api.post('/connections/odoo/sync', { full }),
    onSuccess: () => { toast('Sinkronisasi masuk antrean'); setTimeout(() => qc.invalidateQueries({ queryKey: ['connections'] }), 2500) },
  })
  const save = useMutation({
    mutationFn: (rows: { odoo_category_id: number; kat: string }[]) => api.put('/connections/odoo/categories', rows),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['connections', 'categories'] }); toast('Pemetaan kategori disimpan · berlaku di sinkron berikutnya') },
    onError: (e: Error) => toast(e.message),
  })
  const o = conn?.odoo
  const katOf = (id: number) => cats?.map.find((m) => m.odoo_category_id === id)?.kat ?? ''
  return (
    <>
      <SheetHead icon="doc" title="Odoo · source of truth" sub={`Mode ${o?.mode ?? '…'} · baca saja${o?.write ? ' · tulis SO draft aktif' : ' · tulis SO draft nonaktif (ODOO_WRITE=false)'}`} onClose={closeSheet} />
      <div className="sec">
        <h4>Sinkronisasi · tiap 10 menit</h4>
        <ul className="route">
          {(o?.models ?? []).filter((m) => MODEL[m.model]).map((m) => (
            <li key={m.model}>
              <div><b>{MODEL[m.model]}</b><span>{m.model} · {m.records} record terakhir</span></div>
              <span className="pill neutral">{m.last_run_at ? `${shortDate(m.last_run_at)} ${hhmm(m.last_run_at)}` : 'belum'}</span>
            </li>
          ))}
          {(o?.models ?? []).length === 0 && <li><div><b>Belum pernah sinkron</b><span>Jalankan sinkron penuh pertama</span></div></li>}
        </ul>
        <div style={{ display: 'flex', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>
          <button className="btn ghost" onClick={() => test.mutate()}><Icon name="plug" />Uji koneksi</button>
          <button className="btn ghost" onClick={() => sync.mutate(false)}><Icon name="refresh" />Sinkronkan sekarang</button>
          <button className="btn quiet" onClick={() => sync.mutate(true)}>Sinkron penuh</button>
        </div>
      </div>
      <div className="sec">
        <h4>Kategori Odoo → product mix (6 KAT)</h4>
        <div className="tbl-wrap">
          <table className="tbl">
            <thead><tr><th>Kategori Odoo</th><th>KAT</th></tr></thead>
            <tbody>
              {(cats?.odoo ?? []).map((c) => (
                <tr key={c.id}>
                  <td>{c.complete_name || c.name} <span className="mono">#{c.id}</span></td>
                  <td>
                    <select className="frm" style={{ height: 32, borderRadius: 9, border: '1px solid var(--line-strong)', background: 'var(--surface)', color: 'var(--text)', font: 'inherit', fontSize: 12.5, padding: '0 8px' }}
                      value={katOf(c.id)} onChange={(e) => save.mutate([{ odoo_category_id: c.id, kat: e.target.value }])}>
                      <option value="" disabled>— pilih —</option>
                      {(cats?.kat ?? []).map((k) => <option key={k} value={k}>{k}</option>)}
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="ft"><button className="btn ghost" onClick={closeSheet}>Tutup</button><span className="spacer" /><span className="pol"><Icon name="lock" />GSI Orbit tidak pernah mengubah data Odoo</span></div>
    </>
  )
}

