import type { BoardItem } from '../../api/types'
import { dueSoon, mustCollect } from '../orbit/filters'

// Kota Distribusi (dashboard game scene): which dealers get a shop on the street and what the town's people are
// busy with. Every task comes from real data — trucks deliver to dealers whose jadwal order is near, the collector
// rides to dealers over limit / overdue, sales walk to At risk dealers for a follow-up. Pure, so it is tested.

export type ShopLook = 'key' | 'normal' | 'risk' | 'closed'
export type TaskKind = 'deliver' | 'collect' | 'visit'

export interface Shop {
  id: string
  name: string
  look: ShopLook
  tone: 'good' | 'warn' | 'bad' | 'neutral'
  omzet: number
  dueIn: number | null
  /** 0 = front street, 1 = back street */
  row: 0 | 1
  /** position along the street, 0…1 */
  slot: number
}

export interface Task { kind: TaskKind; shop: string }

export interface TownPlan {
  shops: Shop[]
  tasks: Record<TaskKind, Task[]>
}

const short = (s: string) => s.replace(/^(PT|CV|UD|Toko|TB)\.?\s+/i, '').trim()
const toneOf = (d: BoardItem): Shop['tone'] => {
  const s = d.metrics.credit.state
  return s === 'aman' ? 'good' : s === 'tipis' ? 'warn' : s === 'cash' ? 'neutral' : 'bad'
}
const lookOf = (d: BoardItem): ShopLook =>
  d.metrics.status === 'Churn' ? 'closed' : d.metrics.status === 'At risk' ? 'risk' : d.metrics.status === 'Key account' ? 'key' : 'normal'

/** Pick up to `max` dealers: first some with a task (so the town has a story), then the biggest by omzet. */
export function planTown(board: BoardItem[], max: number): TownPlan {
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
  // biggest shops on the front street, in omzet order, alternating so both streets look alive
  const chosen = [...picked.values()].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln || a.name.localeCompare(b.name))
  const perRow = [Math.ceil(chosen.length / 2), Math.floor(chosen.length / 2)]
  const idx = [0, 0]
  const shops: Shop[] = chosen.map((d, i) => {
    const row = (i % 2) as 0 | 1
    const k = idx[row]++
    return { id: d.id, name: short(d.short_name || d.name), look: lookOf(d), tone: toneOf(d), omzet: d.metrics.omzet_bln, dueIn: d.metrics.due_in ?? null, row, slot: perRow[row] > 1 ? k / (perRow[row] - 1) : 0.5 }
  })
  const onStreet = new Set(shops.map((s) => s.id))
  const tasks: Record<TaskKind, Task[]> = { deliver: [], collect: [], visit: [] }
  for (const d of chosen) {
    if (!onStreet.has(d.id)) continue
    if (dueSoon(d)) tasks.deliver.push({ kind: 'deliver', shop: d.id })
    if (mustCollect(d)) tasks.collect.push({ kind: 'collect', shop: d.id })
    if (d.metrics.status === 'At risk') tasks.visit.push({ kind: 'visit', shop: d.id })
  }
  return { shops, tasks }
}
