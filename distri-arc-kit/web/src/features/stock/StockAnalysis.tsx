import { useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { fmtNum } from '../../lib/format'

// Analisa stok on Cek stok (Sam 22 Oct, "as the owner"; one card, Sam 22 Oct): where the money sits and what to do,
// answered by one filter — Ringkasan, Fast moving, Umur stok, Per cabang, Per kategori, Reorder segera, Dead stock,
// Slow moving, Mutasi antar cabang. A bar picks its group (an age, a branch, a category) and the table below lists its
// products; a row opens the product's check. Numbers from formulas on the server (stockanalysis).

type Class = 'fast' | 'normal' | 'slow' | 'dead'
interface Share { key: string; skus: number; units?: number; value: number; pct: number; turnover_days?: number | null; dead_value?: number; month_sales?: number }
interface Branch { branch: string; qty: number; per_week: number }
export interface Product {
  sku: string; name: string; category: string; qty: number; value: number; per_week: number; cover_days: number | null; age_days: number
  month_sales: number; class: Class; abc: string; critical: boolean; aging: boolean; reorder_qty: number; branches: Branch[]
}
interface Transfer { sku: string; name: string; from: string; to: string; qty: number; value: number }
export interface StockAnalysisData {
  branch: string; skus: number; units: number; value: number; month_sales: number; turnover_days: number | null; turnover_target: number
  aging_days: number; critical_days: number
  classes: Share[]; abc: Share[]; categories: Share[]; branches: Share[]; age_buckets: Share[]
  aging_value: number; aging_skus: number; critical_skus: number
  transfers: Transfer[]; products: Product[]
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

export type View = 'ringkasan' | 'fast' | 'umur' | 'cabang' | 'kategori' | 'reorder' | 'dead' | 'slow' | 'mutasi'
const VIEWS: [View, string][] = [
  ['ringkasan', 'Ringkasan'], ['fast', 'Fast moving'], ['umur', 'Umur stok'], ['cabang', 'Per cabang'], ['kategori', 'Per kategori'],
  ['reorder', 'Reorder segera'], ['dead', 'Dead stock'], ['slow', 'Slow moving'], ['mutasi', 'Mutasi antar cabang'],
]
const VIEW_HINT: Record<View, string> = {
  ringkasan: 'Komposisi stok, analisa ABC, dan insight untuk owner',
  fast: 'Barang paling cepat terjual — jaga stoknya jangan sampai kosong',
  umur: 'Nilai stok per periode umur — klik periode untuk melihat barangnya',
  cabang: 'Nilai dan perputaran stok per cabang — klik cabang untuk melihat barangnya',
  kategori: 'Nilai dan perputaran stok per kategori — klik kategori untuk melihat barangnya',
  reorder: 'Habis sebelum batas hari kritis pada kecepatan jual sekarang — saran pesan untuk 30 hari',
  dead: 'Tidak terjual 90 hari — push, bundle, atau retur ke pemasok',
  slow: 'Masih terjual tapi stok cukup lebih dari 90 hari — tahan pembelian',
  mutasi: 'Barang diam di satu cabang yang dibutuhkan cabang lain — tanpa pembelian baru',
}

/** "Rp 1,25 M", "Rp 350 jt", "Rp 850.000" */
export function rpShort(v: number): string {
  if (v >= 1e9) return 'Rp ' + (v / 1e9).toFixed(2).replace(/\.?0+$/, '').replace('.', ',') + ' M'
  if (v >= 1e6) return 'Rp ' + (v / 1e6).toFixed(v >= 1e8 ? 0 : 1).replace(/\.0$/, '').replace('.', ',') + ' jt'
  return 'Rp ' + Math.round(v).toLocaleString('id-ID')
}

/** Days the stock lasts, "—" when it does not sell. */
export const coverText = (d: number | null) => (d == null ? '—' : d < 1 ? '< 1 hari' : `${Math.round(d).toLocaleString('id-ID')} hari`)

/** The age period of a product, as the server's buckets. */
export function ageBucket(days: number): string {
  if (days <= 30) return '0–30'
  if (days <= 60) return '31–60'
  if (days <= 90) return '61–90'
  if (days <= 180) return '91–180'
  return '> 180'
}

export function StockAnalysis({ onPick, initialBranch = '', initialView = '', initialGroup = '' }: { onPick: (sku: string) => void; initialBranch?: string; initialView?: string; initialGroup?: string }) {
  const [branch, setBranch] = useState(initialBranch)
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
        <span className="meta">Pilih filter untuk menjawab: fast moving, umur stok, per cabang, per kategori, reorder, dead stock, slow moving, mutasi</span>
        <button className="btn quiet sa-toggle" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? 'Sembunyikan' : 'Tampilkan'}</button>
      </div>
      {open && (
        <>
          {branches.length > 1 && (
            <div className="chips sa-branch">
              {['', ...branches].map((b) => <button key={b || 'all'} className={`chip ${branch === b ? 'is-active' : ''}`} onClick={() => setBranch(b)}>{b || 'Semua cabang'}</button>)}
            </div>
          )}
          {isLoading || !a ? <p className="sg-hint">Menghitung analisa stok…</p> : a.skus === 0 ? <p className="sg-hint">Belum ada data stok untuk dianalisa.</p> : <Body a={a} onPick={onPick} initialView={initialView} initialGroup={initialGroup} />}
        </>
      )}
    </div>
  )
}

function Body({ a, onPick, initialView = '', initialGroup = '' }: { a: StockAnalysisData; onPick: (sku: string) => void; initialView?: string; initialGroup?: string }) {
  const [view, setView] = useState<View>(() => (VIEWS.some(([k]) => k === initialView) ? (initialView as View) : 'ringkasan'))
  const [group, setGroup] = useState(initialGroup) // the age period, branch or category picked in its chart
  const cls = Object.fromEntries(a.classes.map((c) => [c.key, c])) as Record<Class, Share>
  const p = a.products
  const lists = useMemo(() => ({
    fast: p.filter((x) => x.class === 'fast').sort((x, y) => y.per_week - x.per_week),
    reorder: p.filter((x) => x.critical).sort((x, y) => (x.cover_days ?? 0) - (y.cover_days ?? 0)),
    dead: p.filter((x) => x.class === 'dead'),
    slow: p.filter((x) => x.class === 'slow'),
  }), [p])
  const counts: Partial<Record<View, number>> = { fast: lists.fast.length, reorder: lists.reorder.length, dead: lists.dead.length, slow: lists.slow.length, mutasi: a.transfers.length, cabang: a.branches.length, kategori: a.categories.length }
  const pick = (v: View) => { setView(v); setGroup('') }
  const over = a.turnover_days != null && a.turnover_days > a.turnover_target
  return (
    <div className="sa-body">
      <div className="sa-kpi">
        <Tile label="Nilai stok" value={rpShort(a.value)} sub={`${fmtNum(a.skus)} barang · ${fmtNum(a.units)} unit`} />
        <Tile label="Penjualan / bulan" value={rpShort(a.month_sales)} sub="perkiraan dari 90 hari terakhir" />
        <Tile label="Perputaran stok" value={a.turnover_days == null ? '—' : `${fmtNum(a.turnover_days)} hari`} sub={`target ≤ ${a.turnover_target} hari`} tone={a.turnover_days == null ? '' : over ? 'warn' : 'good'} />
        <Tile label="Dead stock" value={rpShort(cls.dead?.value ?? 0)} sub={`${cls.dead?.pct ?? 0}% nilai · ${fmtNum(cls.dead?.skus ?? 0)} barang`} tone={(cls.dead?.pct ?? 0) >= 20 ? 'bad' : (cls.dead?.skus ?? 0) > 0 ? 'warn' : 'good'} onClick={() => pick('dead')} />
        <Tile label={`Stok menua > ${a.aging_days} hari`} value={rpShort(a.aging_value)} sub={`${fmtNum(a.aging_skus)} barang`} tone={a.aging_skus ? 'warn' : 'good'} onClick={() => pick('umur')} />
        <Tile label="Reorder segera" value={fmtNum(a.critical_skus)} sub={`habis < ${a.critical_days} hari`} tone={a.critical_skus ? 'bad' : 'good'} onClick={() => pick('reorder')} />
      </div>

      <div className="sa-filter" role="tablist" aria-label="Filter analisa stok">
        {VIEWS.map(([k, l]) => (
          <button key={k} role="tab" aria-selected={view === k} className={view === k ? 'on' : ''} onClick={() => pick(k)}>
            {l}{counts[k] != null && <span>{fmtNum(counts[k]!)}</span>}
          </button>
        ))}
      </div>
      <p className="sa-hint">{VIEW_HINT[view]}</p>

      <div className="sa-view">
        {view === 'ringkasan' && <Summary a={a} cls={cls} onClass={(c) => pick(c === 'normal' ? 'ringkasan' : c)} />}
        {view === 'fast' && (
          <Table rows={lists.fast} onPick={onPick} empty="Belum ada barang fast moving." cols={[
            ['Terjual / mgg', (x) => <b>{fmtNum(x.per_week)}</b>],
            ['Stok', (x) => fmtNum(x.qty)],
            ['Cukup untuk', (x) => <span className={x.critical ? 'bad' : ''}>{coverText(x.cover_days)}</span>],
            ['Penjualan / bln', (x) => rpShort(x.month_sales)],
          ]} />
        )}
        {view === 'umur' && (
          <>
            <Bars pick={group} onPick={setGroup} rows={a.age_buckets.map((b) => ({ key: b.key, label: b.key + ' hari', value: b.value, note: `${b.skus} barang`, warn: b.key === '91–180' || b.key === '> 180' }))} />
            <GroupHead group={group && group + ' hari'} onClear={() => setGroup('')} all="Semua barang, umur tertua dulu" />
            <Table rows={(group ? p.filter((x) => x.qty > 0 && ageBucket(x.age_days) === group) : p.filter((x) => x.qty > 0)).slice().sort((x, y) => y.age_days - x.age_days)} onPick={onPick} cols={[
              ['Umur', (x) => <span className={x.aging ? 'warn' : ''}>{fmtNum(x.age_days)} hari</span>],
              ['Stok', (x) => fmtNum(x.qty)],
              ['Nilai', (x) => <b>{rpShort(x.value)}</b>],
              ['Kelas', (x) => <ClassTag c={x.class} />],
            ]} />
          </>
        )}
        {view === 'cabang' && (
          <>
            <Bars pick={group} onPick={setGroup} rows={a.branches.map((b) => ({ key: b.key, label: b.key, value: b.value, note: `${b.dead_value ? `dead ${rpShort(b.dead_value)} · ` : ''}${b.turnover_days == null ? 'tidak terjual' : `putar ${fmtNum(b.turnover_days)} hari`}`, warn: b.turnover_days == null || b.turnover_days > a.turnover_target }))} />
            <GroupHead group={group} onClear={() => setGroup('')} all="Klik cabang untuk melihat stok barangnya" />
            {group && <BranchTable branch={group} products={p} onPick={onPick} />}
          </>
        )}
        {view === 'kategori' && (
          <>
            <Bars pick={group} onPick={setGroup} rows={a.categories.map((c) => ({ key: c.key, label: c.key, value: c.value, note: c.turnover_days == null ? 'tidak terjual' : `putar ${fmtNum(c.turnover_days)} hari`, warn: c.turnover_days == null || c.turnover_days > a.turnover_target }))} />
            <GroupHead group={group} onClear={() => setGroup('')} all="Klik kategori untuk melihat barangnya" />
            {group && (
              <Table rows={p.filter((x) => (x.category || 'Lainnya') === group)} onPick={onPick} cols={[
                ['Stok', (x) => fmtNum(x.qty)],
                ['Nilai', (x) => <b>{rpShort(x.value)}</b>],
                ['Terjual / mgg', (x) => fmtNum(x.per_week)],
                ['Kelas', (x) => <ClassTag c={x.class} />],
              ]} />
            )}
          </>
        )}
        {view === 'reorder' && (
          <Table rows={lists.reorder} onPick={onPick} empty="Tidak ada barang yang akan habis." cols={[
            ['Stok', (x) => fmtNum(x.qty)],
            ['Habis dalam', (x) => <b className="bad">{coverText(x.cover_days)}</b>],
            ['Terjual / mgg', (x) => fmtNum(x.per_week)],
            ['Saran pesan', (x) => <b>±{fmtNum(x.reorder_qty)} unit</b>],
          ]} />
        )}
        {view === 'dead' && (
          <Table rows={lists.dead} onPick={onPick} empty="Tidak ada dead stock." cols={[
            ['Stok', (x) => fmtNum(x.qty)],
            ['Nilai', (x) => <b>{rpShort(x.value)}</b>],
            ['Umur', (x) => `${fmtNum(x.age_days)} hari`],
            ['Di cabang', (x) => x.branches.filter((b) => b.qty > 0).map((b) => `${b.branch} ${fmtNum(b.qty)}`).join(' · ')],
          ]} />
        )}
        {view === 'slow' && (
          <Table rows={lists.slow} onPick={onPick} empty="Tidak ada slow moving." cols={[
            ['Stok', (x) => fmtNum(x.qty)],
            ['Nilai', (x) => <b>{rpShort(x.value)}</b>],
            ['Cukup untuk', (x) => coverText(x.cover_days)],
            ['Terjual / mgg', (x) => fmtNum(x.per_week)],
          ]} />
        )}
        {view === 'mutasi' && <TransferTable rows={a.transfers} onPick={onPick} branchRead={!!a.branch} />}
      </div>
      <p className="sa-note">Kecepatan jual = rata-rata unit terjual per minggu dalam 90 hari. Perputaran stok = nilai stok ÷ HPP harian (glosarium, target ≤ {a.turnover_target} hari). Penjualan per bulan memakai harga list, atau harga pokok bila belum ada di daftar harga.</p>
    </div>
  )
}

function Summary({ a, cls, onClass }: { a: StockAnalysisData; cls: Record<Class, Share>; onClass: (c: Class) => void }) {
  return (
    <div className="sa-sum">
      <div>
        <h3>Komposisi stok (FSN)</h3>
        <div className="sa-stack" role="img" aria-label={a.classes.map((c) => `${CLASS_LABEL[c.key as Class]} ${c.pct}%`).join(', ')}>
          {a.classes.filter((c) => c.value > 0).map((c) => (
            <i key={c.key} className={`fsn-${c.key}`} style={{ flexGrow: c.value }} title={`${CLASS_LABEL[c.key as Class]} · ${rpShort(c.value)} (${c.pct}%) · ${c.skus} barang`} />
          ))}
        </div>
        <table className="sa-legend">
          <thead><tr><th>Kelas</th><th className="r">Barang</th><th className="r">Nilai stok</th><th className="r">Porsi jual</th></tr></thead>
          <tbody>
            {(['fast', 'normal', 'slow', 'dead'] as Class[]).map((k) => {
              const c = cls[k]
              return (
                <tr key={k} title={CLASS_HINT[k]} className={k === 'normal' ? '' : 'go'} onClick={() => onClass(k)}>
                  <td><span className="sa-cls"><span className={`sa-sw fsn-${k}`} />{CLASS_LABEL[k]}</span><small>{CLASS_HINT[k]}</small></td>
                  <td className="r num">{fmtNum(c?.skus ?? 0)}</td>
                  <td className="r num">{rpShort(c?.value ?? 0)} <small>{c?.pct ?? 0}%</small></td>
                  <td className="r num">{a.month_sales > 0 ? `${Math.round(((c?.month_sales ?? 0) / a.month_sales) * 100)}%` : '—'}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
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
      </div>
      <div>
        <h3>Insight</h3>
        {a.insights.length === 0 ? <p className="sa-empty">Belum ada insight.</p> : (
          <ul className="sa-ins">
            {a.insights.map((x) => <li key={x.text} className={x.tone}><Icon name={TONE_ICON[x.tone]} /><span>{x.text}</span></li>)}
          </ul>
        )}
      </div>
    </div>
  )
}

function Tile({ label, value, sub, tone = '', onClick }: { label: string; value: string; sub: string; tone?: string; onClick?: () => void }) {
  const body = <><span>{label}</span><b>{value}</b><small>{sub}</small></>
  return onClick
    ? <button type="button" className={`sa-tile go ${tone}`} onClick={onClick} title="Buka filternya">{body}</button>
    : <div className={`sa-tile ${tone}`}>{body}</div>
}

function ClassTag({ c }: { c: Class }) {
  return <span className="sa-ctag"><span className={`sa-sw fsn-${c}`} />{CLASS_LABEL[c]}</span>
}

function Bars({ rows, pick, onPick }: { rows: { key: string; label: string; value: number; note: string; warn?: boolean }[]; pick: string; onPick: (k: string) => void }) {
  const max = Math.max(1, ...rows.map((r) => r.value))
  return (
    <div className="sa-bars">
      {rows.map((r) => (
        <button type="button" key={r.key} className={pick === r.key ? 'on' : ''} onClick={() => onPick(pick === r.key ? '' : r.key)} title={`${r.label} · ${rpShort(r.value)} · ${r.note} — klik untuk melihat barangnya`}>
          <span className="k">{r.label}</span>
          <span className="t"><i className={r.warn ? 'warn' : ''} style={{ width: `${(r.value / max) * 100}%` }} /></span>
          <span className="v num">{rpShort(r.value)}<small className={r.warn ? 'warn' : ''}>{r.note}</small></span>
        </button>
      ))}
    </div>
  )
}

function GroupHead({ group, onClear, all }: { group: string; onClear: () => void; all: string }) {
  return (
    <div className="sa-ghead">
      {group ? <><b>{group}</b><button className="btn quiet" onClick={onClear}><Icon name="x" />Lepas pilihan</button></> : <span>{all}</span>}
    </div>
  )
}

const PAGE = 15

/** Products as a table: search, then a page at a time; a row opens the product's check. */
function Table({ rows, cols, onPick, empty = 'Tidak ada barang.' }: { rows: Product[]; cols: [string, (x: Product) => ReactNode][]; onPick: (sku: string) => void; empty?: string }) {
  const [q, setQ] = useState('')
  const [n, setN] = useState(PAGE)
  const s = q.trim().toLowerCase()
  const shown = s ? rows.filter((x) => `${x.name} ${x.sku} ${x.category}`.toLowerCase().includes(s)) : rows
  if (rows.length === 0) return <p className="sa-empty">{empty}</p>
  return (
    <div className="sa-table">
      <div className="sa-tsearch"><Icon name="search" /><input value={q} onChange={(e) => { setQ(e.target.value); setN(PAGE) }} placeholder="Cari barang…" aria-label="Cari barang di filter ini" /><span>{fmtNum(shown.length)} barang</span></div>
      <div className="odl-wrap">
        <table className="odl">
          <thead><tr><th>Produk</th>{cols.map(([h]) => <th key={h} className="r">{h}</th>)}</tr></thead>
          <tbody>
            {shown.slice(0, n).map((x) => (
              <tr key={x.sku} onClick={() => onPick(x.sku)} title="Buka cek barang">
                <td className="odl-name"><b>{x.name}</b><small>{x.sku}{x.category ? ` · ${x.category}` : ''}{x.abc ? ` · ABC ${x.abc}` : ''}</small></td>
                {cols.map(([h, f]) => <td key={h} className="r num">{f(x)}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {shown.length > n && <button className="btn quiet sa-more" onClick={() => setN(n + PAGE * 2)}>Tampilkan {fmtNum(Math.min(PAGE * 2, shown.length - n))} lagi · {fmtNum(shown.length - n)} tersisa</button>}
    </div>
  )
}

/** The stock of one branch: its quantity there, its pace there, and the value at the product's average unit value. */
function BranchTable({ branch, products, onPick }: { branch: string; products: Product[]; onPick: (sku: string) => void }) {
  const rows = products.filter((x) => x.branches.some((b) => b.branch === branch && (b.qty > 0 || b.per_week > 0)))
  const at = (x: Product) => x.branches.find((b) => b.branch === branch)!
  const value = (x: Product) => (x.qty > 0 ? Math.round((x.value / x.qty) * Math.max(0, at(x).qty)) : 0)
  return <Table rows={rows.slice().sort((x, y) => value(y) - value(x))} onPick={onPick} empty="Tidak ada stok di cabang ini." cols={[
    ['Stok di cabang', (x) => fmtNum(at(x).qty)],
    ['Terjual / mgg', (x) => fmtNum(at(x).per_week)],
    ['Nilai ±', (x) => <b>{rpShort(value(x))}</b>],
    ['Kelas', (x) => <ClassTag c={x.class} />],
  ]} />
}

function TransferTable({ rows, onPick, branchRead }: { rows: Transfer[]; onPick: (sku: string) => void; branchRead: boolean }) {
  if (branchRead) return <p className="sa-empty">Mutasi membandingkan antar cabang — pilih <b>Semua cabang</b> di atas.</p>
  if (rows.length === 0) return <p className="sa-empty">Tidak ada peluang mutasi: tidak ada barang yang diam di satu cabang sementara cabang lain kekurangan.</p>
  return (
    <div className="odl-wrap">
      <table className="odl">
        <thead><tr><th>Produk</th><th>Dari</th><th>Ke</th><th className="r">Unit</th><th className="r">Nilai</th></tr></thead>
        <tbody>
          {rows.map((t) => (
            <tr key={t.sku + t.from + t.to} onClick={() => onPick(t.sku)} title="Buka cek barang">
              <td className="odl-name"><b>{t.name}</b><small>{t.sku}</small></td>
              <td>{t.from} <small className="muted">tidak terjual</small></td>
              <td>{t.to} <small className="muted">stok tipis</small></td>
              <td className="r num"><b>{fmtNum(t.qty)}</b></td>
              <td className="r num">{rpShort(t.value)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
