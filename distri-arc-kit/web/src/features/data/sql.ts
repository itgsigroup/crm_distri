import type { ContractColumn, DataEntity } from '../../api/types'

const CAST = new Set(['code', 'customer_code', 'invoice_number', 'number', 'sku'])
const ident = /^[A-Za-z0-9_]+$/

/** Import query for one table and column mapping (contract column → source column or expression); mirrors importer.BuildSQL. */
export function buildSQL(project: string, entity: DataEntity, contract: ContractColumn[], table: string, cols: Record<string, string>, where = ''): string {
  const sel: string[] = []
  for (const c of contract) {
    const src = (cols[c.name] ?? '').trim()
    if (!src) continue
    let expr = ident.test(src) ? '`' + src + '`' : src
    if (CAST.has(c.name)) expr = `CAST(${expr} AS STRING)`
    sel.push(`${expr} AS ${c.name}`)
  }
  const distinct = entity === 'customers' || entity === 'sales' ? 'DISTINCT ' : ''
  let q = `SELECT ${distinct}${sel.join(',\n       ')}\nFROM \`${project}.${table}\``
  if (where.trim()) q += `\nWHERE ${where.trim()}`
  return q
}

export const missingRequired = (contract: ContractColumn[], cols: Record<string, string>) => contract.filter((c) => c.required && !(cols[c.name] ?? '').trim()).map((c) => c.name)
