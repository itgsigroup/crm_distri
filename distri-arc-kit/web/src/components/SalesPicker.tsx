import { useSearchParams } from 'react-router'
import { useMe, useSales } from '../app/queries'
import type { Sales } from '../api/types'
import { Icon } from './Icon'

// One sales' page on Pusat kendali and Orchestrator: ?sales=<key> in the address, so every sales has their own link
// (/?sales=andi, /orchestrator?sales=andi). The API filters by it (and always shows a sales user only their own).

export function useSalesPage() {
  const [params, setParams] = useSearchParams()
  const { data: me } = useMe()
  const locked = me?.role === 'sales' || me?.scope === 'own'
  const sales = locked ? undefined : params.get('sales') || undefined
  const setSales = (key: string) => {
    const p = new URLSearchParams(params)
    if (key) p.set('sales', key)
    else p.delete('sales')
    setParams(p)
  }
  /** a path on another page that keeps the same sales */
  const link = (path: string) => (sales ? `${path}?sales=${encodeURIComponent(sales)}` : path)
  return { sales, setSales, locked, link }
}

/** Sales of the filter: the Pengguna sales (Mapping sales) as chips; BigQuery names not linked to a user in a list. */
export function splitSales(list: Sales[]): { chips: Sales[]; rest: Sales[] } {
  const linked = list.filter((x) => x.user_id).sort((a, b) => a.name.localeCompare(b.name, 'id'))
  if (linked.length) return { chips: linked, rest: list.filter((x) => !x.user_id).sort((a, b) => a.name.localeCompare(b.name, 'id')) }
  // nothing mapped yet: the sales with the most dealers as chips, the rest in the list
  const byDealers = [...list].sort((a, b) => b.dealers - a.dealers || a.name.localeCompare(b.name, 'id'))
  return { chips: byDealers.slice(0, 10), rest: byDealers.slice(10).sort((a, b) => a.name.localeCompare(b.name, 'id')) }
}

/** "Filter sales": Semua sales + one chip per sales; each opens that sales' own page (?sales=). */
export function SalesPicker({ page }: { page: ReturnType<typeof useSalesPage> }) {
  const { data: me } = useMe()
  const { data: list = [] } = useSales()
  if (page.locked) {
    return (
      <div className="sales-filter locked">
        <span className="sf-l"><Icon name="user-x" />Data Anda</span>
        <span className="sales-pick locked" title="Peran Anda hanya melihat dealer milik Anda">{me?.name ?? 'Anda'}</span>
      </div>
    )
  }
  const { chips, rest } = splitSales(list)
  const inRest = rest.find((x) => x.key === page.sales)
  return (
    <div className="sales-filter" role="toolbar" aria-label="Filter sales">
      <span className="sf-l"><Icon name="people" />Filter sales</span>
      <div className="sf-chips">
        <button type="button" className={!page.sales ? 'is-active' : ''} aria-pressed={!page.sales} onClick={() => page.setSales('')}>Semua sales</button>
        {chips.map((x) => (
          <button key={x.key} type="button" className={page.sales === x.key ? 'is-active' : ''} aria-pressed={page.sales === x.key} onClick={() => page.setSales(x.key)} title={`${x.user_name ?? x.name} · ${x.branch} · ${x.dealers} dealer`}>
            <span className="sp-av">{x.initials}</span>{x.user_name ?? x.name}<small>{x.dealers}</small>
          </button>
        ))}
      </div>
      {rest.length > 0 && (
        <label className={`sales-pick ${inRest ? 'on' : ''}`} title="Nama sales dari BigQuery yang belum dihubungkan ke pengguna (Master data → Mapping sales)">
          <select value={inRest ? inRest.key : ''} onChange={(e) => page.setSales(e.target.value)} aria-label="Pilih sales lain">
            <option value="">{chips.some((x) => x.user_id) ? `Belum terhubung (${rest.length})…` : `Sales lain (${rest.length})…`}</option>
            {rest.map((x) => <option key={x.key} value={x.key}>{x.name}{x.branch ? ` · ${x.branch}` : ''} ({x.dealers} dealer)</option>)}
          </select>
          <Icon name="chev" className="i sp-chev" />
        </label>
      )}
      {page.sales && !list.some((x) => x.key === page.sales) && <span className="sales-pick on">{page.sales}</span>}
    </div>
  )
}

/** "Halaman sales: Andi · Semarang" under the page header while one sales is chosen. */
export function SalesBanner({ page }: { page: ReturnType<typeof useSalesPage> }) {
  const { data: list = [] } = useSales()
  if (!page.sales) return null
  const cur = list.find((x) => x.key === page.sales)
  return (
    <div className="sales-banner">
      <span className="sp-av">{cur?.initials ?? '?'}</span>
      <span><b>Halaman sales: {cur?.user_name ?? cur?.name ?? page.sales}</b>{cur?.branch ? ` · ${cur.branch}` : ''} · {cur?.dealers ?? 0} dealer · hanya dealer, keputusan, rencana, dan konflik miliknya — data sales lain tidak ikut.</span>
      <button type="button" className="btn quiet" onClick={() => page.setSales('')}><Icon name="x" />Semua sales</button>
    </div>
  )
}
