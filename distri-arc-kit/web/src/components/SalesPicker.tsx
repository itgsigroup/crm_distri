import { useSearchParams } from 'react-router'
import { useMe, useSales } from '../app/queries'
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

export function SalesPicker({ page }: { page: ReturnType<typeof useSalesPage> }) {
  const { data: me } = useMe()
  const { data: list = [] } = useSales()
  if (page.locked) {
    return <span className="sales-pick locked" title="Anda hanya melihat dealer milik Anda"><Icon name="user-x" />Data {me?.name ?? 'Anda'}</span>
  }
  const cur = list.find((x) => x.key === page.sales)
  return (
    <label className={`sales-pick ${page.sales ? 'on' : ''}`} title="Tampilkan halaman satu sales — data sales lain tidak ikut">
      <span className="sp-av">{cur ? cur.initials : <Icon name="people" />}</span>
      <select value={page.sales ?? ''} onChange={(e) => page.setSales(e.target.value)} aria-label="Pilih sales">
        <option value="">Semua sales</option>
        {list.map((x) => <option key={x.key} value={x.key}>{x.name}{x.branch ? ` · ${x.branch}` : ''} ({x.dealers} dealer)</option>)}
        {page.sales && !cur && <option value={page.sales}>{page.sales}</option>}
      </select>
      <Icon name="chev" className="i sp-chev" />
    </label>
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
      <span><b>Halaman sales: {cur?.name ?? page.sales}</b>{cur?.branch ? ` · ${cur.branch}` : ''} · hanya dealer, keputusan, rencana, dan konflik milik {cur?.name.split(' ')[0] ?? 'sales ini'} — data sales lain tidak ikut.</span>
      <button type="button" className="btn quiet" onClick={() => page.setSales('')}><Icon name="x" />Semua sales</button>
    </div>
  )
}
