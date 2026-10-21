import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useSearchParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { fmtNum, hhmm, shortDate } from '../../lib/format'
import { StockSource } from './StockPage'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { diffText, parseQty, productCompare, productText, rpFull, verdict, VERDICT, type StockCheck, type StockProduct } from './check'

const ALL = 'Semua cabang'
// a stable empty list while loading: a new [] each render would reset the table every render (useDataTable)
const NONE: StockProduct[] = []
const when = (d: string) => `${shortDate(d)} ${hhmm(d)}`

// Cek stok (21 Oct): find a product in the table, open its check in a modal, see what GSI Orbit and the AI Trigger read for it (per branch, and the BigQuery
// rows per warehouse behind it), type the real stock from Accurate / the warehouse, and keep the count. The verdict
// says which side is wrong: GSI Orbit against its BigQuery rows (sync, Gudang mapping) or BigQuery against real.
export function StockCheckPage() {
  const [params, setParams] = useSearchParams()
  const sku = params.get('sku') ?? ''
  const pick = (next: string) => setParams(next ? { sku: next } : {}, { replace: false })
  return (
    <>
      <StockSource />
      <StockTable current={sku} onPick={pick} />
      <RecentChecks onPick={pick} />
      {sku && <StockModal key={sku} sku={sku} onClose={() => pick('')} />}
    </>
  )
}

/** A product's check as a wide modal over the table (Sam 21 Oct): Esc, the backdrop or × close it — after asking
 * when real counts are typed but not saved. ?sku= keeps it linkable. */
function StockModal({ sku, onClose }: { sku: string; onClose: () => void }) {
  const guard = useRef<() => boolean>(() => true)
  const close = useCallback(() => { if (guard.current()) onClose() }, [onClose])
  const setGuard = useCallback((g: () => boolean) => { guard.current = g }, [])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    document.addEventListener('keydown', onKey)
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.removeEventListener('keydown', onKey); document.body.style.overflow = overflow }
  }, [close])
  return createPortal(
    <div className="sc-modal-bg" onMouseDown={(e) => { if (e.target === e.currentTarget) close() }}>
      <div className="sc-modal" role="dialog" aria-modal="true" aria-labelledby="sc-modal-title">
        <ProductCheck sku={sku} onClose={close} setGuard={setGuard} />
      </div>
    </div>,
    document.body,
  )
}

function ModalHead({ title, sub, onClose }: { title: string; sub?: string; onClose: () => void }) {
  return (
    <div className="sc-modal-h">
      <span className="ni"><Icon name="box" /></span>
      <div><h2 id="sc-modal-title">{title}</h2>{sub && <small>{sub}</small>}</div>
      <button className="close" onClick={onClose} aria-label="Tutup"><Icon name="x" /></button>
    </div>
  )
}

/** Every product in the stock as a data table: search, sort every column both ways, pages of 10/25/50, a branch
 * filter; a row opens the product's check below. */
function StockTable({ current, onPick }: { current: string; onPick: (sku: string) => void }) {
  const { data: rows = NONE, isLoading } = useQuery({ queryKey: ['stock', 'products'], queryFn: () => api.get<StockProduct[]>('/stock/products') })
  const [branch, setBranch] = useState('')
  const branches = useMemo(() => [...new Set(rows.flatMap((r) => Object.keys(r.by_branch)))].sort((a, b) => a.localeCompare(b, 'id')), [rows])
  const list = useMemo(() => (branch ? rows.filter((r) => (r.by_branch[branch] ?? 0) !== 0) : rows), [rows, branch])
  const compare = useMemo(() => productCompare(branches), [branches])
  const t = useDataTable<StockProduct, string>({ rows: list, text: productText, compare, tie: (a, b) => a.name.localeCompare(b.name, 'id'), initial: { column: 'produk', dir: 'asc' }, firstDir: (c) => (c === 'produk' ? 'asc' : 'desc') })
  const total = t.matches.reduce((a, r) => a + (branch ? r.by_branch[branch] ?? 0 : r.qty), 0)
  return (
    <div className="card sc-search">
      <div className="card-h"><h2>Data stok</h2><span className="meta">Yang dibaca AI per cabang · klik barang untuk mencocokkan dengan stok real</span></div>
      {branches.length > 1 && (
        <div className="chips" style={{ marginBottom: 10 }}>
          {['', ...branches].map((b) => <button key={b || 'all'} className={`chip ${branch === b ? 'is-active' : ''}`} onClick={() => setBranch(b)}>{b || ALL}</button>)}
        </div>
      )}
      <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari nama barang, kode, kategori…" label="Cari stok"
        meta={`${t.matches.length.toLocaleString('id-ID')} barang · ${fmtNum(total)} unit${t.matches.length !== list.length ? ` · dari ${list.length.toLocaleString('id-ID')}` : ''}`} />
      {isLoading ? <p className="sg-hint">Memuat data stok…</p> : t.matches.length === 0 ? (
        <p className="sg-hint">{rows.length ? 'Tidak ada barang yang cocok dengan pencarian ini.' : 'Belum ada data stok.'}</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl sc-table sc-stock">
            <thead><tr>
              <SortTh t={t} c="produk">Produk</SortTh>
              <SortTh t={t} c="total" right>Total</SortTh>
              <SortTh t={t} c="harga" right>Harga</SortTh>
              {branches.map((b) => <SortTh key={b} t={t} c={'b:' + b} right>{b}</SortTh>)}
              <SortTh t={t} c="cek">Cek terakhir</SortTh>
              <th aria-label="Aksi" />
            </tr></thead>
            <tbody>
              {t.shown.map((r) => (
                <tr key={r.sku} className={r.sku === current ? 'is-active' : undefined} onClick={() => onPick(r.sku)}>
                  <td className="odl-name"><b>{r.name}</b><small>{r.sku}{r.category ? ` · ${r.category}` : ''}</small></td>
                  <td className="r num"><b>{fmtNum(r.qty)}</b></td>
                  <td className={`r num${r.price ? '' : ' muted'}`}>{rpFull(r.price)}</td>
                  {branches.map((b) => <td key={b} className={`r num${r.by_branch[b] ? '' : ' muted'}`}>{r.by_branch[b] != null ? fmtNum(r.by_branch[b]) : '—'}</td>)}
                  <td className="sc-last">{r.last_check ? <><span className={`sc-tag ${r.last_check.off ? 'bad' : 'good'}`}>{r.last_check.off ? `${r.last_check.off} selisih` : 'Cocok'}</span><small>{when(r.last_check.at)} · {r.last_check.branches} cabang</small></> : <span className="muted">Belum dicek</span>}</td>
                  <td className="odl-act"><button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={(e) => { e.stopPropagation(); onPick(r.sku) }}><Icon name="check" />Cek</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun="barang" />
    </div>
  )
}

function ProductCheck({ sku, onClose, setGuard }: { sku: string; onClose: () => void; setGuard: (g: () => boolean) => void }) {
  const { toast } = useFeedback()
  const qc = useQueryClient()
  const [branch, setBranch] = useState(ALL)
  const { data: c, error, isLoading } = useQuery({
    queryKey: ['stock', 'check', sku, branch],
    placeholderData: (prev) => prev,
    queryFn: () => api.get<StockCheck>(`/stock/check?sku=${encodeURIComponent(sku)}&branch=${encodeURIComponent(branch)}`),
  })
  const [real, setReal] = useState<Record<string, string>>({})
  const [note, setNote] = useState('')
  const save = useMutation({
    mutationFn: () => api.post<{ saved: number }>('/stock/checks', {
      sku, note, counts: Object.entries(real).map(([b, v]) => ({ branch: b, real_qty: parseQty(v) })).filter((x) => x.real_qty != null),
    }),
    onSuccess: (r) => {
      toast(`${r.saved} cabang tersimpan`)
      setReal({})
      setNote('')
      void qc.invalidateQueries({ queryKey: ['stock', 'check', sku] })
      void qc.invalidateQueries({ queryKey: ['stock', 'checks'] })
    },
    onError: (e: Error) => toast(e.message),
  })

  const rows = c?.branches ?? []
  const hasSource = (c?.source_rows.length ?? 0) > 0
  const totals = { arc: 0, src: 0, realSum: 0, counted: 0 }
  for (const b of rows) {
    totals.arc += b.arc_qty
    totals.src += b.source_qty
    const v = parseQty(real[b.branch] ?? '')
    if (v != null) { totals.realSum += v; totals.counted++ }
  }

  const typed = Object.values(real).some((v) => parseQty(v) != null)
  useEffect(() => setGuard(() => !typed || window.confirm('Stok real yang diisi belum disimpan. Tutup tanpa menyimpan?')), [typed, setGuard])
  if (isLoading) return <><ModalHead title="Memuat…" sub={sku} onClose={onClose} /><div className="sc-modal-b"><p className="sg-hint">Memuat data stok…</p></div></>
  if (error || !c) return <><ModalHead title={sku} onClose={onClose} /><div className="sc-modal-b"><p className="sg-hint">{(error as Error)?.message ?? 'Produk tidak ditemukan'}</p></div></>

  const withStock = rows.filter((b) => b.arc_qty > 0 || b.source_qty > 0).map((b) => b.branch)
  const a = c.answer
  return (
    <>
      <ModalHead title={c.product} sub={`${c.sku} · ${c.source}${c.synced_at ? ` · sinkron ${when(c.synced_at)}` : ''}`} onClose={onClose} />
      <div className="sc-modal-b">
      <div className="kpis sc-kpis">
        <div className="kpi-t"><small>Harga (tier A)</small><b className="num">{rpFull(c.price.list)}</b><span className="dl n">{c.price.list ? (Object.entries(c.price.tiers ?? {}).filter(([k, v]) => k !== 'A' && v > 0).sort().map(([k, v]) => `Tier ${k} ${rpFull(v)}`).join(' · ') || 'harga list') : 'belum ada di daftar harga'}</span></div>
        <div className="kpi-t"><small>Dibaca AI (GSI Orbit)</small><b className="num">{fmtNum(totals.arc)}</b><span className="dl n">semua cabang · {c.sku}</span></div>
        <div className="kpi-t"><small>Baris BigQuery</small><b className="num">{hasSource ? fmtNum(totals.src) : '—'}</b><span className={`dl ${hasSource && totals.src !== totals.arc ? 'bad' : 'n'}`}>{hasSource ? `${c.source_rows.length} baris gudang` : 'tidak ada baris sumber'}</span></div>
        <div className="kpi-t"><small>Stok real diisi</small><b className="num">{totals.counted ? fmtNum(totals.realSum) : '—'}</b><span className="dl n">{totals.counted} cabang diisi</span></div>
        <div className="kpi-t"><small>Selisih AI − real</small><b className="num">{totals.counted ? diffText(rows.filter((b) => parseQty(real[b.branch] ?? '') != null).reduce((s, b) => s + b.arc_qty, 0), totals.realSum) : '—'}</b><span className="dl n">hanya cabang yang diisi</span></div>
      </div>

      <div className="card">
        <div className="card-h"><h2>Jawaban AI Trigger</h2><span className="ai" style={{ marginLeft: 6 }}>Tanya stok</span><span className="meta">Sama persis dengan kartu di Chat untuk percakapan cabang ini</span></div>
        <div className="chips" style={{ marginBottom: 12 }}>
          {[ALL, ...withStock].map((b) => <button key={b} className={`chip ${branch === b ? 'is-active' : ''}`} onClick={() => setBranch(b)}>{b}</button>)}
        </div>
        {a ? (
          <div className={`sc-answer ${a.tone}`}>
            <b>{a.headline}</b>
            {a.others.length > 0 && <div className="muted">Cabang lain: {a.others.slice(0, 5).map((o) => `${o.branch} ${fmtNum(o.qty)}`).join(' · ')}</div>}
            {a.facts.length > 0 && <div className="muted">{a.facts.map((f) => `${f.label}: ${f.value}`).join(' · ')}</div>}
            {a.note && <div>{a.note}</div>}
            {a.draft && <div className="sc-draft">“{a.draft}”</div>}
          </div>
        ) : <p className="sg-hint">AI Trigger tidak membaca baris stok untuk barang ini.</p>}
        <p className="sc-hint">“{ALL}” = percakapan sales yang memegang semua cabang: stok semua cabang dijumlah.</p>
      </div>

      <div className="card">
        <div className="card-h"><h2>Cocokkan per cabang</h2><span className="meta">{c.product} · isi stok real dari Accurate (Kts / Stok dapat dijual) atau hitungan gudang</span></div>
        <div className="odl-wrap">
          <table className="odl sc-table">
            <thead><tr><th>Cabang</th><th className="r">Dibaca AI</th><th className="r">Baris BigQuery</th><th className="r">Stok real</th><th className="r">Selisih</th><th>Hasil</th><th>Cek terakhir</th></tr></thead>
            <tbody>
              {rows.map((b) => {
                const v = parseQty(real[b.branch] ?? '')
                const vd = verdict(b, v, hasSource)
                const [label, tone] = vd === 'sumber' && !hasSource ? ['AI ≠ real', 'bad'] : VERDICT[vd]
                const last = b.last_check
                return (
                  <tr key={b.branch}>
                    <td><b>{b.branch}</b></td>
                    <td className="r num">{fmtNum(b.arc_qty)}</td>
                    <td className="r num">{hasSource ? fmtNum(b.source_qty) : '—'}</td>
                    <td className="r"><input className="sc-in num" inputMode="numeric" value={real[b.branch] ?? ''} placeholder="—" aria-label={`Stok real ${b.branch}`} onChange={(e) => setReal({ ...real, [b.branch]: e.target.value })} /></td>
                    <td className="r num">{v != null ? diffText(b.arc_qty, v) : ''}</td>
                    <td><span className={`sc-tag ${tone}`}>{label}</span></td>
                    <td className="sc-last">{last ? <>real <b className="num">{fmtNum(last.real_qty)}</b> vs AI {fmtNum(last.arc_qty)}<small>{when(last.checked_at)} · {last.checked_by_name || '—'}</small></> : <span className="muted">—</span>}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className="sc-save">
          <input className="sc-note" value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} placeholder="Catatan (mis. dari Accurate 21 Okt 09.30, termasuk Transit)" aria-label="Catatan" />
          <button className="btn primary" disabled={!typed || save.isPending} onClick={() => save.mutate()}><Icon name="check" />{save.isPending ? 'Menyimpan…' : 'Simpan hasil cek'}</button>
        </div>
      </div>

      <div className="card">
        <div className="card-h"><h2>Data mentah BigQuery per gudang</h2><span className="meta">Yang dijumlah per cabang · {c.source}{c.synced_at ? ` · sinkron ${when(c.synced_at)}` : ''}</span></div>
        {hasSource ? (
          <div className="odl-wrap">
            <table className="odl sc-table">
              <thead><tr><th>Gudang (sumber)</th><th>→ Cabang</th><th className="r">Qty</th><th>Diperbarui</th></tr></thead>
              <tbody>
                {c.source_rows.map((x) => (
                  <tr key={x.warehouse}>
                    <td>{x.warehouse}</td>
                    <td>{x.branch}{!x.mapped && <span className="sc-tag warn" style={{ marginLeft: 8 }}>belum dipetakan</span>}</td>
                    <td className="r num">{fmtNum(x.qty)}</td>
                    <td className="muted">{when(x.updated_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : <p className="sg-hint">Tidak ada baris sumber untuk barang ini (data contoh atau belum sinkron).</p>}
        <p className="sc-hint">Gudang dipetakan ke cabang di Pengaturan → Data &amp; master → Mapping → Gudang. Angka di sini sudah dibatasi oleh query stok BigQuery (gudang jual saja, tanpa Transit/Display/Servis).</p>
      </div>

      <ChecksTable title={`Riwayat cek · ${c.product}`} rows={c.checks} />
      </div>
    </>
  )
}

function RecentChecks({ onPick }: { onPick: (sku: string) => void }) {
  const { data = [] } = useQuery({ queryKey: ['stock', 'checks'], queryFn: () => api.get<StockCheck['checks']>('/stock/checks?limit=100') })
  return <ChecksTable title="Cek stok terakhir" rows={data} onPick={onPick} />
}

function ChecksTable({ title, rows, onPick }: { title: string; rows: StockCheck['checks']; onPick?: (sku: string) => void }) {
  const off = rows.filter((x) => x.real_qty !== x.arc_qty).length
  return (
    <div className="card">
      <div className="card-h"><h2>{title}</h2><span className="meta">{rows.length ? `${rows.length} baris · ${off} selisih` : ''}</span></div>
      {rows.length === 0 ? <p className="sg-hint">Belum ada hasil cek. Cari barang di atas, isi stok real, lalu simpan.</p> : (
        <div className="odl-wrap">
          <table className="odl sc-table">
            <thead><tr><th>Waktu</th>{onPick && <th>Barang</th>}<th>Cabang</th><th className="r">Dibaca AI</th><th className="r">Real</th><th className="r">Selisih</th><th>Oleh</th><th>Catatan</th></tr></thead>
            <tbody>
              {rows.map((x) => (
                <tr key={x.id}>
                  <td className="muted">{when(x.checked_at)}</td>
                  {onPick && <td><button className="sc-link" onClick={() => onPick(x.sku)}>{x.product}</button><small className="muted"> {x.sku}</small></td>}
                  <td>{x.branch}</td>
                  <td className="r num">{fmtNum(x.arc_qty)}</td>
                  <td className="r num">{fmtNum(x.real_qty)}</td>
                  <td className="r num"><span className={`sc-tag ${x.real_qty === x.arc_qty ? 'good' : 'bad'}`}>{diffText(x.arc_qty, x.real_qty)}</span></td>
                  <td>{x.checked_by_name || '—'}</td>
                  <td className="muted">{x.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
