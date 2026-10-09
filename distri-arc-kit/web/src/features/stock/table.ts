import type { AgingItem } from '../../api/types'

// Push stok table: search and sort as pure functions (tested), the page only renders.

export type StockColumn = 'produk' | 'cabang' | 'qty' | 'nilai' | 'umur' | 'dealer'
export type Dir = 'asc' | 'desc'

/** Text columns start A→Z, numbers start with the largest. */
export const firstDir = (c: StockColumn): Dir => (c === 'produk' || c === 'cabang' ? 'asc' : 'desc')

const dealerCount = (x: AgingItem) => x.candidate_count ?? x.candidates?.length ?? 0

/** Search in product name, SKU, category, branch and the names of the matching dealers. */
export function searchAging(list: AgingItem[], query: string): AgingItem[] {
  const q = query.trim().toLocaleLowerCase('id-ID')
  if (!q) return list
  return list.filter((x) => `${x.name} ${x.sku} ${x.category} ${x.branch} ${(x.candidates ?? []).map((c) => c.name).join(' ')}`.toLocaleLowerCase('id-ID').includes(q))
}

export function sortAging(list: AgingItem[], column: StockColumn, dir: Dir): AgingItem[] {
  const key: Record<StockColumn, (a: AgingItem, b: AgingItem) => number> = {
    produk: (a, b) => a.name.localeCompare(b.name, 'id'),
    cabang: (a, b) => a.branch.localeCompare(b.branch, 'id'),
    qty: (a, b) => a.qty - b.qty,
    nilai: (a, b) => a.value - b.value,
    umur: (a, b) => a.age_days - b.age_days,
    dealer: (a, b) => dealerCount(a) - dealerCount(b),
  }
  const sign = dir === 'asc' ? 1 : -1
  return [...list].sort((a, b) => (key[column](a, b) || a.name.localeCompare(b.name, 'id') || a.branch.localeCompare(b.branch, 'id')) * sign)
}
