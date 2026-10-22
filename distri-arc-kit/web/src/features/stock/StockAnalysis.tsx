import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { fmtNum } from '../../lib/format'

// Analisa stok on Cek stok (Sam 22 Oct, "as the owner"): where the money sits and what to do — FSN (fast / normal /
// slow / dead), ABC by sales value, aging, turnover against the 40-day target, reorder now, transfers between
// branches — charts and lists, with the insight lines on top. Numbers from formulas on the server (stockanalysis).

type Class = 'fast' | 'normal' | 'slow' | 'dead'
interface Share { key: string; skus: number; value: number; pct: number; turnover_days?: number | null; dead_value?: number; month_sales?: number }
interface Product {
  sku: string; name: string; category: string; qty: number; value: number; per_week: number; cover_days: number | null; age_days: number
  month_sales: number; class: Class; abc: string; critical: boolean; aging: boolean; reorder_qty: number
}
interface Transfer { sku: string; name: string; from: string; to: string; qty: number; value: number }
export interface StockAnalysisData {
  branch: string; skus: number; units: number; value: number; month_sales: number; turnover_days: number | null; turnover_target: number
  aging_days: number; critical_days: number
  classes: Share[]; abc: Share[]; categories: Share[]; branches: Share[]; age_buckets: Share[]
  aging_value: number; aging_skus: number; critical_skus: number
  fast: Product[]; reorder: Product[]; dead: Product[]; slow: Product[]; transfers: Transfer[]
  insights: { tone: 'good' | 'warn' | 'bad' | 'info'; text: string }[]
}

export const CLASS_LABEL: Record<Class, string> = { fast: 'Fast moving', normal: 'Normal', slow: 'Slow moving', dead: 'Dead stock' }
const CLASS_HINT: Record<Class, string> = {
  fast: '20% barang dengan penjualan per minggu tertinggi',
  normal: 'terjual rutin, stok cukup ≤ 90 hari',
  slow: 'masih terjual, tapi stok cukup > 90 hari',
  dead: 'tidak terjual 90 hari terakhir',
}
const TONE_ICON = { good: 'check', warn: 'alert', bad: 'alert', info: 'spark' } as const

/** "Rp 1,25 M", "Rp 350 jt", "Rp 850.000" */
export function rpShort(v: number): string {
  if (v >= 1e9) return 'Rp ' + (v / 1e9).toFixed(2).replace(/\.?0+$/, '').replace('.', ',') + ' M'
  if (v >= 1e6) return 'Rp ' + (v / 1e6).toFixed(v >= 1e8 ? 0 : 1).replace(/\.0$/, '').replace('.', ',') + ' jt'
  return 'Rp ' + Math.round(v).toLocaleString('id-ID')
}

/** "5 hari", "3,5 mgg"-free: days the stock lasts, "—" when it does not sell. */
export const coverText = (d: number | null) => (d == null ? '—' : d < 1 ? '< 1 hari' : `${Math.round(d).toLocaleString('id-ID')} hari`)

export function StockAnalysis({ onPick }: { onPick: (sku: string) => void }) {
  const [branch, setBranch] = useState('')
  const [open, setOpen] = useState(true)
  const { data: a, isLoading } = useQuery({
    queryKey: ['stock', 'analysis', branch],
    placeholderData: (prev) => prev,
    queryFn: () => api.get<StockAnalysisData>('/stock/analysis' + (branch ? `?branch=${encodeURIComponent(branch)}` : '')),
  })
  const { data: all } = useQuery({ queryKey: ['stock', 'analysis', ''], queryFn: () => api.get<StockAnalysisData>('/stock/analysis'), enabled: !!branch })
  const branches = (branch ? all : a)?.branches.map((b) => b.key) ?? []
  return (
    <div className="card sa">
      <div className="card-h">
        <h2>Analisa stok</h2>
        <span className="meta">Fast / slow moving, dead stock, perputaran, reorder · dihitung dari stok dan penjualan 90 hari</span>
        <button className="btn quiet sa-toggle" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? 'Sembunyikan' : 'Tampilkan'}</button>
      </div>
      {open && (
        <>
          {branches.length > 1 && (
            <div className="chips" style={{ marginBottom: 12 }}>
              {['', ...branches].map((b) => <button key={b || 'all'} className={`chip ${branch === b ? 'is-active' : ''}`} onClick={() => setBranch(b)}>{b || 'Semua cabang'}</button>)}
            </div>
          )}
          {isLoading || !a ? <p className="sg-hint">Menghitung analisa stok…</p> : a.skus === 0 ? <p className="sg-hint">Belum ada data stok untuk dianalisa.</p> : <Body a={a} onPick={onPick} />}
        </>
      )}
    </div>
  )
}

function Body({ a, onPick }: { a: StockAnalysisData; onPick: (sku: string) => void }) {
  const cls = Object.fromEntries(a.classes.map((c) => [c.key, c])) as Record<Class, Share>
  const over = a.turnover_days != null && a.turnover_days > a.turnover_target
  return (
    <div className="sa-body">
      <div className="sa-kpi">
        <Tile label="Nilai stok" value={rpShort(a.value)} sub={`${fmtNum(a.skus)} barang · ${fmtNum(a.units)} unit`} />
        <Tile label="Penjualan / bulan" value={rpShort(a.month_sales)} sub="perkiraan dari 90 hari terakhir" />
        <Tile label="Perputaran stok" value={a.turnover_days == null ? '—' : `${fmtNum(a.turnover_days)} hari`} sub={`target ≤ ${a.turnover_target} hari`} tone={a.turnover_days == null ? '' : over ? 'warn' : 'good'} />
        <Tile label="Dead stock" value={rpShort(cls.dead?.value ?? 0)} sub={`${cls.dead?.pct ?? 0}% nilai · ${fmtNum(cls.dead?.skus ?? 0)} barang`} tone={(cls.dead?.pct ?? 0) >= 20 ? 'bad' : (cls.dead?.skus ?? 0) > 0 ? 'warn' : 'good'} />
        <Tile label={`Stok menua > ${a.aging_days} hari`} value={rpShort(a.aging_value)} sub={`${fmtNum(a.aging_skus)} barang`} tone={a.aging_skus ? 'warn' : 'good'} />
        <Tile label="Reorder segera" value={fmtNum(a.critical_skus)} sub={`habis < ${a.critical_days} hari`} tone={a.critical_skus ? 'bad' : 'good'} />
      </div>

      {a.insights.length > 0 && (
        <ul className="sa-ins">
          {a.insights.map((x) => <li key={x.text} className={x.tone}><Icon name={TONE_ICON[x.tone]} /><span>{x.text}</span></li>)}
        </ul>
      )}

      <div className="sa-grid">
        <section className="sa-panel">
          <h3>Komposisi stok (FSN)</h3>
          <div className="sa-stack" role="img" aria-label={a.classes.map((c) => `${CLASS_LABEL[c.key as Class]} ${c.pct}%`).join(', ')}>
            {a.classes.filter((c) => c.value > 0).map((c) => (
              <i key={c.key} className={`fsn-${c.key}`} style={{ flexGrow: c.value }} title={`${CLASS_LABEL[c.key as Class]} · ${rpShort(c.value)} (${c.pct}%) · ${c.skus} barang`} />
            ))}
          </div>
          <table className="sa-legend">
            <thead><tr><th>Kelas</th><th className="r">Barang</th><th className="r">Nilai stok</th><th className="r">Porsi jual</th></tr></thead>
            <tbody>
              {a.classes.map((c) => (
                <tr key={c.key} title={CLASS_HINT[c.key as Class]}>
                  <td><span className="sa-cls"><span className={`sa-sw fsn-${c.key}`} />{CLASS_LABEL[c.key as Class]}</span><small>{CLASS_HINT[c.key as Class]}</small></td>
                  <td className="r num">{fmtNum(c.skus)}</td>
                  <td className="r num">{rpShort(c.value)} <small>{c.pct}%</small></td>
                  <td className="r num">{a.month_sales > 0 ? `${Math.round(((c.month_sales ?? 0) / a.month_sales) * 100)}%` : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        <section className="sa-panel">
          <h3>Umur stok</h3>
          <Bars rows={a.age_buckets.map((b) => ({ key: b.key + ' hari', value: b.value, note: `${b.skus} barang`, warn: b.key === '91–180' || b.key === '> 180' }))} />
          <h3 style={{ marginTop: 14 }}>Analisa ABC <small>berdasarkan nilai penjualan</small></h3>
          <div className="sa-abc">
            {a.abc.map((c) => (
              <div key={c.key}>
                <b>{c.key}</b>
                <span>{fmtNum(c.skus)} barang</span>
                <small>{a.month_sales > 0 ? Math.round(((c.month_sales ?? 0) / a.month_sales) * 100) : 0}% penjualan · stok {rpShort(c.value)}</small>
              </div>
            ))}
          </div>
        </section>

        <section className="sa-panel">
          <h3>Nilai stok per kategori</h3>
          <Bars rows={a.categories.map((c) => ({ key: c.key, value: c.value, note: c.turnover_days == null ? 'tidak terjual' : `putar ${fmtNum(c.turnover_days)} hari`, warn: c.turnover_days == null || c.turnover_days > a.turnover_target }))} />
        </section>

        {a.branches.length > 1 && (
          <section className="sa-panel">
            <h3>Nilai stok per cabang</h3>
            <Bars rows={a.branches.map((b) => ({ key: b.key, value: b.value, note: `${b.dead_value ? `dead ${rpShort(b.dead_value)} · ` : ''}${b.turnover_days == null ? 'tidak terjual' : `putar ${fmtNum(b.turnover_days)} hari`}`, warn: b.turnover_days == null || b.turnover_days > a.turnover_target }))} />
          </section>
        )}
      </div>

      <div className="sa-lists">
        <List title="Fast moving" hint="paling cepat terjual — jangan sampai kosong" items={a.fast} onPick={onPick}
          right={(p) => <><b>{fmtNum(p.per_week)}/mgg</b><small className={p.critical ? 'bad' : ''}>stok {coverText(p.cover_days)}</small></>} />
        <List title="Reorder segera" hint={`habis < ${a.critical_days} hari · saran untuk 30 hari`} items={a.reorder} onPick={onPick} empty="Tidak ada barang yang akan habis."
          right={(p) => <><b className="bad">{coverText(p.cover_days)}</b><small>pesan ±{fmtNum(p.reorder_qty)} unit</small></>} />
        <List title="Dead stock terbesar" hint="tidak terjual 90 hari — push, bundle, atau retur" items={a.dead} onPick={onPick} empty="Tidak ada dead stock."
          right={(p) => <><b>{rpShort(p.value)}</b><small>{fmtNum(p.qty)} unit · {fmtNum(p.age_days)} hari</small></>} />
        <List title="Slow moving" hint="stok cukup > 90 hari — tahan pembelian" items={a.slow} onPick={onPick} empty="Tidak ada slow moving."
          right={(p) => <><b>{rpShort(p.value)}</b><small>stok {coverText(p.cover_days)}</small></>} />
        {a.transfers.length > 0 && (
          <section className="sa-list">
            <h3>Mutasi antar cabang <small>barang diam di satu cabang, dibutuhkan cabang lain</small></h3>
            <ol>
              {a.transfers.map((t) => (
                <li key={t.sku + t.from + t.to} onClick={() => onPick(t.sku)} title="Buka cek barang">
                  <span><b>{t.name}</b><small>{t.from} → {t.to}</small></span>
                  <span className="r"><b>{fmtNum(t.qty)} unit</b><small>{rpShort(t.value)}</small></span>
                </li>
              ))}
            </ol>
          </section>
        )}
      </div>
      <p className="sa-note">Kecepatan jual = rata-rata unit terjual per minggu dalam 90 hari. Perputaran stok = nilai stok ÷ HPP harian (glosarium, target ≤ {a.turnover_target} hari). Penjualan per bulan memakai harga list, atau harga pokok bila belum ada di daftar harga.</p>
    </div>
  )
}

function Tile({ label, value, sub, tone = '' }: { label: string; value: string; sub: string; tone?: string }) {
  return <div className={`sa-tile ${tone}`}><span>{label}</span><b>{value}</b><small>{sub}</small></div>
}

function Bars({ rows }: { rows: { key: string; value: number; note: string; warn?: boolean }[] }) {
  const max = Math.max(1, ...rows.map((r) => r.value))
  return (
    <div className="sa-bars">
      {rows.map((r) => (
        <div key={r.key} title={`${r.key} · ${rpShort(r.value)} · ${r.note}`}>
          <span className="k">{r.key}</span>
          <div className="t"><i className={r.warn ? 'warn' : ''} style={{ width: `${(r.value / max) * 100}%` }} /></div>
          <span className="v num">{rpShort(r.value)}<small className={r.warn ? 'warn' : ''}>{r.note}</small></span>
        </div>
      ))}
    </div>
  )
}

function List({ title, hint, items, right, onPick, empty = 'Belum ada.' }: { title: string; hint: string; items: Product[]; right: (p: Product) => ReactNode; onPick: (sku: string) => void; empty?: string }) {
  return (
    <section className="sa-list">
      <h3>{title} <small>{hint}</small></h3>
      {items.length === 0 ? <p className="sa-empty">{empty}</p> : (
        <ol>
          {items.map((p) => (
            <li key={p.sku} onClick={() => onPick(p.sku)} title="Buka cek barang">
              <span><b>{p.name}</b><small>{p.sku}{p.category ? ` · ${p.category}` : ''}{p.abc ? ` · ABC ${p.abc}` : ''}{p.aging ? ' · menua' : ''}</small></span>
              <span className="r">{right(p)}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
