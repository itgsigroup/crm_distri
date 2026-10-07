import { useState } from 'react'
import type { NextAction, Proposal } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { useFeedback } from '../../components/feedback'
import { fmtRp, shortName } from '../../lib/format'
import { useOrch, useOrchStatus } from '../../app/orch'
import { useKpi, useSalesByProduct, useStockAging, useStockCritical, useStockProposals } from '../../app/queries'

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
  const [showAll, setShowAll] = useState(false)

  const open = (p: Proposal) => p.status !== 'rejected' && p.status !== 'expired' && p.status !== 'suppressed'
  const pushes = props.filter((p) => p.kind === 'push_stock' && !p.payload?.parent && open(p))
  const pushOf = (name: string) => pushes.find((p) => p.payload?.name === name)
  const fixOf = (sku: string, branch: string) =>
    props.find((p) => (p.kind === 'transfer' && p.payload?.sku === sku && p.payload?.to === branch) || (p.kind === 'po_request' && p.payload?.sku === sku && p.payload?.branch === branch))

  const branches = [...new Set(aging.map((x) => x.branch))].sort()
  const inBranch = aging.filter((x) => !branch || x.branch === branch)
  const shown = showAll ? inBranch : inBranch.slice(0, 12)
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
      <div className="grid-2">
        <div className="card">
          <div className="card-h">
            <h2>Push stok</h2><span className="ai" style={{ marginLeft: 6 }}>AI Stok</span><span className="meta">Stok menua → dealer yang product mix-nya cocok &amp; jadwal order</span>
            <button className="btn ghost" style={{ height: 28, fontSize: 12, marginLeft: 8 }} disabled={orch.running} onClick={() => reanalyze('screen:stock')}><Icon name="refresh" />Analisis ulang</button>
          </div>
          {branches.length > 1 && (
            <div className="chips" style={{ marginBottom: 8 }}>
              {['', ...branches].map((b) => <button key={b || 'all'} className={`chip ${branch === b ? 'is-active' : ''}`} onClick={() => setBranch(b)}>{b || 'Semua cabang'}</button>)}
            </div>
          )}
          <ul className="l2c">
            {shown.map((x) => {
              const k = x.age_days > 120 ? 'bad' : 'warn'
              const p = pushOf(x.name)
              const ds = (p?.payload?.dealers as { name: string }[] | undefined)?.map((d) => shortName(d.name)) ?? (x.candidates ?? []).map((c) => shortName(c.name))
              const dealers = ds.length ? `${ds.slice(0, 2).join(', ')}${ds.length > 2 ? `, +${ds.length - 2}` : ''}` : 'belum ada yang cocok'
              const note = ds.length ? '' : x.age_days > 180 ? ' · Usul: diskon atau retur ke supplier' : ' · Usul: bundle untuk dealer tier C'
              return (
                <li key={x.id}>
                  <div><b>{x.name}</b><span className="s">{x.qty} {unit(x.name)} · {x.branch} · {fmtRp(x.value)}</span></div>
                  <div>
                    <div className="seg5"><i className={`cur ${k}`} /><i /><i /><i /><i /></div>
                    <div className="stg"><span>umur stok</span><em className="num" style={{ color: `var(--${k})` }}>{x.age_days} hr</em></div>
                  </div>
                  <div className="note">Dealer yang cocok: {dealers}{note}</div>
                  <div>{p ? <ActBtn small next={toNext(p)} /> : <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => toast('Promo belum otomatis: atur harga promo di Accurate — data Distri ARC dibaca dari BigQuery (baca saja)')}>Buat promo</button>}</div>
                </li>
              )
            })}
          </ul>
          {inBranch.length > 12 && (
            <button className="btn quiet" style={{ marginTop: 8, height: 30, fontSize: 12.5 }} onClick={() => setShowAll(!showAll)}>
              {showAll ? 'Ringkas' : `Tampilkan semua ${inBranch.length} stok menua · ${fmtRp(inBranch.reduce((a, x) => a + x.value, 0))}`}
            </button>
          )}
        </div>
        <div className="stack">
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
      </div>
    </>
  )
}
