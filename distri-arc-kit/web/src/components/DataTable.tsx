import { useMemo, useState, type ReactNode } from 'react'
import { Icon } from './Icon'

// Shared data-table behaviour (search, sort asc/desc per column, pages of 10/25/50) with the look of the Orbit dealer
// list (.odl classes). A page supplies the rows, how to search them and one compare function per column.

export type Dir = 'asc' | 'desc'

export interface TableSpec<T, C extends string> {
  rows: T[]
  /** lower-cased text a row is searched in */
  text: (row: T) => string
  compare: Record<C, (a: T, b: T) => number>
  /** sort used when two rows are equal on the chosen column */
  tie?: (a: T, b: T) => number
  initial: { column: C; dir: Dir }
  /** direction a column starts with when first clicked (text A→Z, numbers largest first) */
  firstDir: (column: C) => Dir
}

export function useDataTable<T, C extends string>(spec: TableSpec<T, C>) {
  const { rows, text, compare, tie, initial, firstDir } = spec
  const [query, setQuery] = useState('')
  const [column, setColumn] = useState<C>(initial.column)
  const [dir, setDir] = useState<Dir>(initial.dir)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const matches = useMemo(() => {
    const q = query.trim().toLocaleLowerCase('id-ID')
    return q ? rows.filter((r) => text(r).toLocaleLowerCase('id-ID').includes(q)) : rows
  }, [rows, query, text])
  const sorted = useMemo(() => {
    const sign = dir === 'asc' ? 1 : -1
    return [...matches].sort((a, b) => (compare[column](a, b) || (tie ? tie(a, b) : 0)) * sign)
  }, [matches, column, dir, compare, tie])
  const pageCount = Math.max(1, Math.ceil(sorted.length / pageSize))
  const current = Math.min(page, pageCount)
  const shown = sorted.slice((current - 1) * pageSize, current * pageSize)
  // back to page 1 when the search, page size or rows change (adjusting state during render, not in an effect)
  const [seen, setSeen] = useState({ query, pageSize, rows })
  if (seen.query !== query || seen.pageSize !== pageSize || seen.rows !== rows) {
    setSeen({ query, pageSize, rows })
    setPage(1)
  }
  const sortBy = (c: C) => {
    if (column === c) setDir(dir === 'asc' ? 'desc' : 'asc')
    else {
      setColumn(c)
      setDir(firstDir(c))
    }
    setPage(1)
  }
  return { query, setQuery, matches, shown, column, dir, sortBy, page: current, pageCount, setPage, pageSize, setPageSize }
}

type Table<C extends string> = Pick<ReturnType<typeof useDataTable<unknown, C>>, 'column' | 'dir' | 'sortBy'>

/** A sortable column header: click to sort, click again to reverse. */
export function SortTh<C extends string>({ t, c, children, right = false }: { t: Table<C>; c: C; children: ReactNode; right?: boolean }) {
  const on = t.column === c
  return (
    <th className={right ? 'r' : undefined} aria-sort={on ? (t.dir === 'asc' ? 'ascending' : 'descending') : 'none'}>
      <button type="button" className="odl-sort" onClick={() => t.sortBy(c)} title={on ? (t.dir === 'asc' ? 'Urut naik — klik untuk urut turun' : 'Urut turun — klik untuk urut naik') : 'Urutkan'}>
        {children}<span aria-hidden="true">{on ? (t.dir === 'asc' ? '↑' : '↓') : '↕'}</span>
      </button>
    </th>
  )
}

/** Search box with a result count on the right. */
export function TableSearch({ value, onChange, placeholder, label, meta }: { value: string; onChange: (v: string) => void; placeholder: string; label: string; meta: ReactNode }) {
  return (
    <div className="odl-tools">
      <div className="search of-q odl-q"><Icon name="search" /><input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label={label} /></div>
      <span className="meta">{meta}</span>
    </div>
  )
}

/** "Menampilkan 1–10 dari N …", rows per page and previous / next. */
export function TablePager({ t, noun }: { t: Pick<ReturnType<typeof useDataTable>, 'matches' | 'page' | 'pageCount' | 'setPage' | 'pageSize' | 'setPageSize'>; noun: string }) {
  const n = t.matches.length
  if (!n) return null
  return (
    <div className="odl-footer">
      <span>Menampilkan {(t.page - 1) * t.pageSize + 1}–{Math.min(t.page * t.pageSize, n)} dari {n.toLocaleString('id-ID')} {noun}</span>
      <label>Baris
        <select className="of-s" value={t.pageSize} onChange={(e) => t.setPageSize(Number(e.target.value))} aria-label="Jumlah baris per halaman">
          {[10, 25, 50].map((v) => <option key={v} value={v}>{v}</option>)}
        </select>
      </label>
      <div className="odl-pages">
        <button type="button" className="btn quiet" disabled={t.page <= 1} onClick={() => t.setPage(t.page - 1)}>Sebelumnya</button>
        <span>Halaman {t.page} dari {t.pageCount}</span>
        <button type="button" className="btn quiet" disabled={t.page >= t.pageCount} onClick={() => t.setPage(t.page + 1)}>Berikutnya</button>
      </div>
    </div>
  )
}

/** One column of a ColumnTable: its header, how a cell reads, and how two rows compare on it (omitted: not sortable). */
export interface Column<T> {
  key: string
  label: string
  cell: (row: T) => ReactNode
  sort?: (a: T, b: T) => number
  /** direction the column starts with when first clicked (default: A→Z) */
  first?: Dir
  right?: boolean
  className?: string
}

/** A whole data table from column definitions: search, sort asc/desc per column, pages of 10/25/50 (Pusat kendali
 * cards, 19 Oct). The first sortable column (or `initial`) orders the rows at first. */
export function ColumnTable<T>({ rows, columns, text, rowKey, initial, empty, noun, placeholder, className = '', rowClass }: {
  rows: T[]; columns: Column<T>[]; text: (row: T) => string; rowKey: (row: T) => string
  initial?: { column: string; dir: Dir }; empty: ReactNode; noun: string; placeholder: string; className?: string
  /** a class for one row (e.g. an inactive user drawn faded) */
  rowClass?: (row: T) => string | undefined
}) {
  const sortable = columns.filter((c) => c.sort)
  const compare = Object.fromEntries(sortable.map((c) => [c.key, c.sort!])) as Record<string, (a: T, b: T) => number>
  const first = initial ?? { column: sortable[0]?.key ?? '', dir: sortable[0]?.first ?? 'asc' }
  // the cards rebuild their row arrays on every render: keep the same array while the rows read the same, so the
  // table stays on its page (useDataTable goes back to page 1 when the rows change)
  const sig = rows.map((r) => rowKey(r) + '\u0001' + text(r)).join('\u0002')
  const [stable, setStable] = useState({ sig, rows })
  if (stable.sig !== sig) setStable({ sig, rows })
  const t = useDataTable<T, string>({
    rows: stable.sig === sig ? stable.rows : rows, text, compare: compare[first.column] ? compare : { [first.column]: () => 0, ...compare }, initial: first,
    firstDir: (c) => columns.find((x) => x.key === c)?.first ?? 'asc',
  })
  if (rows.length === 0) return <p className="sg-hint">{empty}</p>
  return (
    <>
      <TableSearch value={t.query} onChange={t.setQuery} placeholder={placeholder} label={`Cari ${noun}`} meta={`${t.matches.length.toLocaleString('id-ID')} ${noun}`} />
      {t.matches.length === 0 ? (
        <p className="sg-hint">Tidak ada {noun} yang cocok dengan pencarian ini.</p>
      ) : (
        <div className="odl-wrap">
          <table className={`odl cc-table ${className}`}>
            <thead><tr>
              {columns.map((c) => c.sort ? <SortTh key={c.key} t={t} c={c.key} right={c.right}>{c.label}</SortTh> : <th key={c.key} className={c.right ? 'r' : undefined}>{c.label}</th>)}
            </tr></thead>
            <tbody>
              {t.shown.map((r) => (
                <tr key={rowKey(r)} className={rowClass?.(r)}>{columns.map((c) => <td key={c.key} className={[c.right ? 'r' : '', c.className ?? ''].join(' ').trim() || undefined}>{c.cell(r)}</td>)}</tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <TablePager t={t} noun={noun} />
    </>
  )
}
