import { useMemo, useState } from 'react'
import type { AgingItem, NextAction, Proposal } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { useFeedback } from '../../components/feedback'
import { fmtRp, shortName } from '../../lib/format'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useKpi, useSalesByProduct, useStockAging, useStockCritical, useStockProposals } from '../../app/queries'
import { SortTh, TablePager, TableSearch, useDataTable } from '../../components/DataTable'
import { AGING_COMPARE, agingText, agingTie, firstDir, type StockColumn } from './table'

/** A stored proposal as the action button of a row. */
export function toNext(p: Proposal | undefined): NextAction | null {
  if (!p) return null
  return { id: p.id, kind: p.kind, title: p.title, button: p.button ?? 'Lihat', icon: p.icon ?? 'check', agent: p.agent, due_label: p.due_label ?? '', status: p.status, why: p.why, decided_at: p.decided_at, executed_at: p.executed_at, autonomy: p.autonomy }
}

const unit = (name: string) => (/modul|detektor|kabel|power/i.test(name) ? 'pcs' : 'unit')
const num = (v: number) => String(v).replace('.', ',')

// Push stok (mockup screen-stock).
export function StockPage() {
  const { toast } = useFeedback()
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const { data: kpi } = useKpi()
  const { data: aging = [] } = useStockAging()
  const { data: critical = [] } = useStockCritical()
  const { data: sales = [] } = useSalesByProduct()
  const { data: props = [] } = useStockProposals()
  const [branch, setBranch] = useState('')

  const open = (p: Proposal) => p.status !== 'rejected' && p.status !== 'expired' && p.status !== 'suppressed'
  const pushes = props.filter((p) => p.kind === 'push_stock' && !p.payload?.parent && open(p))
  const pushOf = (name: string) => pushes.find((p) => p.payload?.name === name)
  const fixOf = (sku: string, branch: string) =>
    props.find((p) => (p.kind === 'transfer' && p.payload?.sku === sku && p.payload?.to === branch) || (p.kind === 'po_request' && p.payload?.sku === sku && p.payload?.branch === branch))

  const branches = [...new Set(aging.map((x) => x.branch))].sort()
  const inBranch = useMemo(() => aging.filter((x) => !branch || x.branch === branch), [aging, branch])
  const old = aging.filter((x) => x.age_days > 90)
  const oldValue = old.reduce((a, x) => a + x.value, 0)
  const activeValue = pushes.reduce((a, p) => a + Number(p.payload?.stock_value ?? 0), 0)
  const activeDealers = pushes.reduce((a, p) => a + ((p.payload?.dealers as unknown[] | undefined)?.length ?? 0), 0)
  const maxSales = Math.max(1, ...sales.map((s) => s.value))

  return (
    <>
      <div className="kpis">
        <div className="kpi-t"><small>Perputaran stok · perputaran</small><b className="num">{kpi?.stock_turn_days ?? '—'}<em>hari</em></b><span className={`dl ${kpi && kpi.stock_turn_days <= kpi.targets.stock_turn_days ? 'good' : 'warn'}`}>target {kpi?.targets.stock_turn_days ?? 40}</span></div>
        <div className="kpi-t"><small>Stok &gt; 90 hari</small><b className="num">{fmtRp(oldValue)}</b><span className="dl bad">{kpi?.stock_value ? Math.round((oldValue / kpi.stock_value) * 100) : 0}% dari nilai stok</span></div>
        <div className="kpi-t"><small>Push stok aktif</small><b className="num">{pushes.length}</b><span className="dl n">{fmtRp(activeValue)} → {activeDealers} dealer</span></div>
        <div className="kpi-t"><small>Stok kritis</small><b className="num">{critical.length}<em>SKU</em></b><span className="dl warn">habis &lt; 10 hari pada siklus order sekarang</span></div>
      </div>
      <PushStockTable list={inBranch} total={aging.length} branches={branches} branch={branch} setBranch={setBranch} pushOf={pushOf} running={orch.running} onReanalyze={() => reanalyze('screen:stock')} onPromo={() => toast('Promo belum otomatis: atur harga promo di Accurate — data GSI Orbit dibaca dari BigQuery (baca saja)')} />
      <div className="grid-2">
        <div className="card">
            <div className="card-h"><h2>Stok kritis</h2><span className="meta">Dari siklus order dealer di tiap cabang</span></div>
            <ul className="row-list">
              {critical.length === 0 && <li><div><div className="s">Tidak ada SKU yang habis sebelum siklus order berikutnya.</div></div></li>}
              {critical.slice(0, 3).map((c) => {
                const fix = fixOf(c.sku, c.branch)
                const other = (c.other_branches ?? []).sort((a, b) => b.qty - a.qty)[0]
                const sub = [`Habis ±${Math.ceil(c.days_left)} hari`]
                if (c.dependents) sub.push(`${c.dependents} dealer bergantung`)
                if (other) sub.push(`${other.branch} stok ${other.qty}`)
                return (
                  <li key={c.id}>
                    <div>
                      <div className="t"><b>{c.name}</b> · {c.branch} · sisa {c.qty}, siklus order {num(c.weekly_velocity)}/minggu</div>
                      <div className="s">{sub.join(' · ')}</div>
                    </div>
                    <span className="st">{fix ? <ActBtn small next={toNext(fix)} ghost={fix.kind === 'po_request'} /> : null}</span>
                  </li>
                )
              })}
            </ul>
          </div>
          <div className="card">
            <div className="card-h"><h2>Penjualan per produk · 30 hari</h2><span className="meta">Nilai · margin</span></div>
            <div className="aging">
              {sales.map((s) => (
                <div className="row" key={s.product}>
                  <span className="lbl">{s.product}</span>
                  <div className="bar"><i style={{ width: `${Math.round((s.value / maxSales) * 100)}%`, background: 'var(--accent)' }} /></div>
                  <span className="v num">{fmtRp(s.value)}<small>margin {Math.round(s.margin_pct)}%</small></span>
                </div>
              ))}
            </div>
          </div>
      </div>
    </>
  )
}

/** Push stok as a data table: search, sort every column both ways, pages of 10/25/50. */
function PushStockTable({ list, total, branches, branch, setBranch, pushOf, running, onReanalyze, onPromo }: {
  list: AgingItem[]; total: number; branches: string[]; branch: string; setBranch: (b: string) => void
  pushOf: (name: string) => Proposal | undefined; running: boolean; onReanalyze: () => void; onPromo: () => void
}) {
  const t = useDataTable<AgingItem, StockColumn>({ rows: list, text: agingText, compare: AGING_COMPARE, tie: agingTie, initial: { column: 'umur', dir: 'desc' }, firstDir })
  const matches = t.matches
  const totalValue = matches.reduce((a, x) => a + x.value, 0)
  return (
    <div className="card push-card">
      <div className="card-h">
        <h2>Push stok</h2><span className="ai" style={{ marginLeft: 6 }}>AI Stok</span><span className="meta">Stok menua → dealer yang product mix-nya cocok &amp; jadwal order</span>
        <button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 8 }} disabled={running} onClick={onReanalyze}><Icon name="refresh" />Analisis ulang</button>
      </div>
      {branches.length > 1 && (
        <div className="chips" style={{ marginBottom: 8 }}>
          {['', ...branches].map((b) => <button key={b || 'all'} className={`chip ${branch === b ? 'is-active' : ''}`} onClick={() => setBranch(b)}>{b || 'Semua cabang'}</button>)}
        </div>
      )}
      <TableSearch value={t.query} onChange={t.setQuery} placeholder="Cari produk, SKU, cabang, dealer…" label="Cari stok menua" meta={`${matches.length.toLocaleString('id-ID')} stok menua · ${fmtRp(totalValue)}${matches.length !== total ? ` · dari ${total.toLocaleString('id-ID')}` : ''}`} />
      {matches.length === 0 ? (
        <p className="sg-hint">{list.length ? 'Tidak ada stok yang cocok dengan pencarian ini.' : 'Tidak ada stok menua.'}</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl stock-table">
            <thead><tr><SortTh t={t} c="produk">Produk</SortTh><SortTh t={t} c="cabang">Cabang</SortTh><SortTh t={t} c="qty" right>Qty</SortTh><SortTh t={t} c="nilai" right>Nilai</SortTh><SortTh t={t} c="umur" right>Umur stok</SortTh><SortTh t={t} c="dealer">Dealer yang cocok</SortTh><th aria-label="Aksi" /></tr></thead>
            <tbody>
              {t.shown.map((x) => {
                const k = x.age_days > 120 ? 'bad' : 'warn'
                const p = pushOf(x.name)
                const ds = (x.candidates ?? []).map((c) => shortName(c.name))
                const n = x.candidate_count ?? ds.length
                const note = n ? '' : x.age_days > 180 ? 'Usul: diskon atau retur ke supplier' : 'Usul: bundle untuk dealer tier C'
                return (
                  <tr key={x.id}>
                    <td className="odl-name"><b>{x.name}</b><small>{x.sku}{x.category ? ` · ${x.category}` : ''}</small></td>
                    <td>{x.branch}</td>
                    <td className="r num">{x.qty.toLocaleString('id-ID')} <small>{unit(x.name)}</small></td>
                    <td className="odl-rp">{fmtRp(x.value)}</td>
                    <td className="r"><em className="num" style={{ color: `var(--${k})`, fontStyle: 'normal', fontWeight: 700 }}>{x.age_days} hr</em></td>
                    <td className="stock-dealers">{n ? <><b>{n}</b> · {ds.slice(0, 2).join(', ')}{n > 2 ? `, +${n - 2}` : ''}</> : <span className="muted">belum ada · {note}</span>}</td>
                    <td className="odl-act">{p ? <ActBtn small next={toNext(p)} /> : <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={onPromo}>Buat promo</button>}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun="stok" />
    </div>
  )
}
