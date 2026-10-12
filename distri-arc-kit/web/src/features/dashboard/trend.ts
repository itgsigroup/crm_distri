import type { TrendKey } from '../../api/types'
import { fmtNum, fmtRp } from '../../lib/format'

// Dashboard trend: how each card's points are written, and the change against the point before.

export const TREND_FMT: Record<TrendKey, (v: number) => string> = {
  dealers: fmtNum, at_risk: fmtNum, due: fmtNum, tight: fmtNum,
  omzet: fmtRp, receivable: fmtRp, overdue: fmtRp, cash_in: fmtRp,
  on_schedule: (v) => `${v}%`,
  dso: (v) => `${v} hari`,
}

/** Which direction is good news (colours the change); 'none' for counts that are neither. */
export const TREND_BETTER: Record<TrendKey, 'up' | 'down' | 'none'> = {
  dealers: 'up', omzet: 'up', on_schedule: 'up', cash_in: 'up',
  at_risk: 'down', dso: 'down', tight: 'down', overdue: 'down', receivable: 'down',
  due: 'none',
}

/** "+12%" / "−3 poin" against the previous point that has data; null when there is none or it was 0. */
export function trendChange(values: (number | null)[], i: number): { dir: 'up' | 'down' | 'flat'; text: string } | null {
  const v = values[i]
  if (v == null) return null
  let j = i - 1
  while (j >= 0 && values[j] == null) j--
  if (j < 0) return null
  const prev = values[j]!
  if (v === prev) return { dir: 'flat', text: 'sama' }
  const dir = v > prev ? 'up' : 'down'
  if (prev === 0) return { dir, text: 'dari 0' }
  const pct = Math.round(((v - prev) / prev) * 100)
  const sign = v > prev ? '+' : '−'
  return { dir, text: pct === 0 ? `${sign}<1%` : `${sign}${Math.abs(pct)}%` }
}

/** Names of the points above the chart: "5 Okt" per day; the running month "Okt 26 · s.d. 5 Okt". */
export function trendHeads(mode: 'months' | 'days', labels: string[], dates: string[], short: (iso: string) => string): string[] {
  if (mode === 'days') return dates.map(short)
  const monthEnd = (iso: string) => new Date(Number(iso.slice(0, 4)), Number(iso.slice(5, 7)), 0).getDate() === Number(iso.slice(8, 10))
  return labels.map((l, i) => (i === labels.length - 1 && dates[i] && !monthEnd(dates[i]) ? `${l} · s.d. ${short(dates[i])}` : l))
}
