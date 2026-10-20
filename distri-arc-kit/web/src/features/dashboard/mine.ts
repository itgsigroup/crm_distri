import type { BoardItem } from '../../api/types'
import { dueSoon, mustCollect } from '../orbit/filters'

// Tambang Distribusi (dashboard game scene): which dealers get a dig site along the haul road of the open pit and
// what the crew is busy with. Every task comes from real data — excavators and dump trucks serve dealers whose
// jadwal order is near, the collector truck rides to dealers over limit / overdue, the surveyor walks to At risk
// dealers for a follow-up. Pure, so it is tested.

export type SiteLook = 'key' | 'normal' | 'risk' | 'closed'
export type TaskKind = 'deliver' | 'collect' | 'visit'

export interface Site {
  id: string
  name: string
  look: SiteLook
  tone: 'good' | 'warn' | 'bad' | 'neutral'
  omzet: number
  dueIn: number | null
  /** position along the haul road, from the rim (0) to the pit floor (1) */
  t: number
}

export interface Task { kind: TaskKind; site: string }

export interface MinePlan {
  sites: Site[]
  tasks: Record<TaskKind, Task[]>
}

// the first and last stretch of the road stay free: the rim holds the warehouse, the floor the stockpile
const ROAD_FROM = 0.14
const ROAD_TO = 0.94

const short = (s: string) => s.replace(/^(PT|CV|UD|Toko|TB)\.?\s+/i, '').trim()
const toneOf = (d: BoardItem): Site['tone'] => {
  const s = d.metrics.credit.state
  return s === 'aman' ? 'good' : s === 'tipis' ? 'warn' : s === 'cash' ? 'neutral' : 'bad'
}
const lookOf = (d: BoardItem): SiteLook =>
  d.metrics.status === 'Churn' ? 'closed' : d.metrics.status === 'At risk' ? 'risk' : d.metrics.status === 'Key account' ? 'key' : 'normal'

/** Pick up to `max` dealers: first some with a task (so the mine has a story), then the biggest by omzet. */
export function planMine(board: BoardItem[], max: number): MinePlan {
  const active = board.filter((d) => d.metrics.segment !== 'Prospek' && d.metrics.status !== 'Prospek')
  const byOmzet = [...active].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln || a.name.localeCompare(b.name))
  const picked = new Map<string, BoardItem>()
  const take = (list: BoardItem[], n: number) => {
    for (const d of list) {
      if (picked.size >= max || n <= 0) break
      if (!picked.has(d.id)) { picked.set(d.id, d); n-- }
    }
  }
  const quota = Math.max(1, Math.floor(max / 4))
  take(byOmzet.filter(dueSoon), quota)
  take(byOmzet.filter(mustCollect), quota)
  take(byOmzet.filter((d) => d.metrics.status === 'At risk'), quota)
  take(byOmzet, max)
  // biggest dealers near the rim, in omzet order, evenly spread down the road
  const chosen = [...picked.values()].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln || a.name.localeCompare(b.name))
  const n = chosen.length
  const sites: Site[] = chosen.map((d, i) => ({
    id: d.id, name: short(d.short_name || d.name), look: lookOf(d), tone: toneOf(d), omzet: d.metrics.omzet_bln, dueIn: d.metrics.due_in ?? null,
    t: n > 1 ? ROAD_FROM + (i / (n - 1)) * (ROAD_TO - ROAD_FROM) : (ROAD_FROM + ROAD_TO) / 2,
  }))
  const tasks: Record<TaskKind, Task[]> = { deliver: [], collect: [], visit: [] }
  for (const d of chosen) {
    if (dueSoon(d)) tasks.deliver.push({ kind: 'deliver', site: d.id })
    if (mustCollect(d)) tasks.collect.push({ kind: 'collect', site: d.id })
    if (d.metrics.status === 'At risk') tasks.visit.push({ kind: 'visit', site: d.id })
  }
  return { sites, tasks }
}
