// Cek stok: types of /stock/check and how a branch's numbers compare (used by StockCheckPage; tested).

export interface StockCheckRow {
  id: string
  sku: string
  product: string
  branch: string
  arc_qty: number
  real_qty: number
  note: string
  synced_at: string | null
  checked_by_name: string
  checked_at: string
}

export interface StockCheckBranch {
  branch: string
  arc_qty: number
  source_qty: number
  last_check: StockCheckRow | null
}

export interface SourceStock {
  warehouse: string
  branch: string
  mapped: boolean
  product: string
  qty: number
  source: string
  updated_at: string
}

export interface StockAnswer {
  headline: string
  tone: 'good' | 'warn' | 'bad' | 'neutral'
  qty: number
  ready: boolean
  draft?: string
  note?: string
  branch?: string
  others: { branch: string; qty: number }[]
  facts: { label: string; value: string }[]
}

export interface StockCheck {
  sku: string
  product: string
  branch: string
  branches: StockCheckBranch[]
  source_rows: SourceStock[]
  answer: StockAnswer | null
  source: string
  synced_at: string | null
  checks: StockCheckRow[]
}

export interface StockProduct {
  sku: string
  name: string
  category: string
  qty: number
  by_branch: Record<string, number>
  last_check: { at: string; branches: number; off: number } | null
}

/** Name, SKU and category: what the stock table is searched in. */
export const productText = (r: StockProduct) => `${r.name} ${r.sku} ${r.category}`

/** Compare functions of the stock table: produk, total, cek (last check; never checked counts as oldest) and one
 * "b:<branch>" per branch. */
export function productCompare(branches: string[]): Record<string, (a: StockProduct, b: StockProduct) => number> {
  const out: Record<string, (a: StockProduct, b: StockProduct) => number> = {
    produk: (a, b) => a.name.localeCompare(b.name, 'id'),
    total: (a, b) => a.qty - b.qty,
    cek: (a, b) => (a.last_check ? Date.parse(a.last_check.at) : 0) - (b.last_check ? Date.parse(b.last_check.at) : 0),
  }
  for (const br of branches) out['b:' + br] = (a, b) => (a.by_branch[br] ?? 0) - (b.by_branch[br] ?? 0)
  return out
}

/** What a branch's difference says: which side is wrong, in words for the user. */
export type Verdict = 'cocok' | 'sinkron' | 'sumber' | 'belum'

/**
 * Compares one branch. real null = not counted yet.
 * - sinkron: GSI Orbit reads other than its own source rows (sync not applied, or Gudang → cabang mapping)
 * - sumber: the source rows (BigQuery) differ from the real stock (the query or the BigQuery table)
 */
export function verdict(b: Pick<StockCheckBranch, 'arc_qty' | 'source_qty'>, real: number | null, hasSource: boolean): Verdict {
  if (hasSource && b.arc_qty !== b.source_qty) return 'sinkron'
  if (real == null) return 'belum'
  return real === b.arc_qty ? 'cocok' : 'sumber'
}

export const VERDICT: Record<Verdict, [string, 'good' | 'warn' | 'bad' | 'neutral']> = {
  cocok: ['Cocok', 'good'],
  sinkron: ['AI ≠ BigQuery · cek sinkron / mapping gudang', 'warn'],
  sumber: ['BigQuery ≠ real · cek query / data sumber', 'bad'],
  belum: ['Belum dicek', 'neutral'],
}

/** Parses a typed count: "" → null, "1.234" → 1234; negative or not a number → null. */
export function parseQty(v: string): number | null {
  const t = v.trim().replace(/\./g, '').replace(',', '.')
  if (t === '') return null
  const n = Math.round(Number(t))
  return Number.isFinite(n) && n >= 0 ? n : null
}

/** Signed difference GSI Orbit − real, as text: "+3", "−24", "0". */
export function diffText(arc: number, real: number): string {
  const d = arc - real
  return d > 0 ? `+${d.toLocaleString('id-ID')}` : d < 0 ? `−${(-d).toLocaleString('id-ID')}` : '0'
}
