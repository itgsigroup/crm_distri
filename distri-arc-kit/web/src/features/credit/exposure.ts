import type { BoardItem, ExposureRow } from '../../api/types'
import type { Dir } from '../../components/DataTable'

// "Exposure vs limit" table: rows joined with the dealer board (full name, sales, cabang) and the compare functions.

export interface ExposureLine extends ExposureRow { name: string; owner: string; branch: string; room: number }
export type ExposureColumn = 'dealer' | 'exposure' | 'limit' | 'room' | 'pct'

export function exposureLines(ex: ExposureRow[], board: BoardItem[]): ExposureLine[] {
  const byId = new Map(board.map((d) => [d.id, d]))
  return ex.map((e) => {
    const d = byId.get(e.dealer_id)
    return { ...e, name: d?.name ?? e.short_name, owner: d?.owner.name ?? '', branch: d?.branch ?? '', room: e.limit - e.exposure }
  })
}

export const exposureText = (x: ExposureLine) => `${x.name} ${x.short_name} ${x.owner} ${x.branch}`

export const EXPOSURE_COMPARE: Record<ExposureColumn, (a: ExposureLine, b: ExposureLine) => number> = {
  dealer: (a, b) => a.short_name.localeCompare(b.short_name, 'id'),
  exposure: (a, b) => a.exposure - b.exposure,
  limit: (a, b) => a.limit - b.limit,
  room: (a, b) => a.room - b.room,
  pct: (a, b) => a.pct - b.pct,
}
export const exposureTie = (a: ExposureLine, b: ExposureLine) => a.short_name.localeCompare(b.short_name, 'id')
export const exposureFirstDir = (c: ExposureColumn): Dir => (c === 'dealer' ? 'asc' : c === 'room' ? 'asc' : 'desc')
