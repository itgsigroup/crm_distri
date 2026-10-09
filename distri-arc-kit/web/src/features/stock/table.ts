import type { AgingItem } from '../../api/types'
import type { Dir } from '../../components/DataTable'

// Push stok table: what a row is searched in and how each column compares (used by useDataTable; tested).

export type StockColumn = 'produk' | 'cabang' | 'qty' | 'nilai' | 'umur' | 'dealer'

/** Text columns start A→Z, numbers start with the largest. */
export const firstDir = (c: StockColumn): Dir => (c === 'produk' || c === 'cabang' ? 'asc' : 'desc')

const dealerCount = (x: AgingItem) => x.candidate_count ?? x.candidates?.length ?? 0

/** Product name, SKU, category, branch and the names of the matching dealers. */
export const agingText = (x: AgingItem) => `${x.name} ${x.sku} ${x.category} ${x.branch} ${(x.candidates ?? []).map((c) => c.name).join(' ')}`

export const AGING_COMPARE: Record<StockColumn, (a: AgingItem, b: AgingItem) => number> = {
  produk: (a, b) => a.name.localeCompare(b.name, 'id'),
  cabang: (a, b) => a.branch.localeCompare(b.branch, 'id'),
  qty: (a, b) => a.qty - b.qty,
  nilai: (a, b) => a.value - b.value,
  umur: (a, b) => a.age_days - b.age_days,
  dealer: (a, b) => dealerCount(a) - dealerCount(b),
}
export const agingTie = (a: AgingItem, b: AgingItem) => a.name.localeCompare(b.name, 'id') || a.branch.localeCompare(b.branch, 'id')
