import type { BoardItem, ThreadView } from '../../api/types'
import { dueSoon, mustCollect } from '../orbit/filters'

// Stasiun Orbit (dashboard game scene): a space miniature with the AI core (the Orchestrator) in the middle, dealers
// as planets around it and the six agents as astronauts. Which dealers get a planet, how far out they orbit and what
// each astronaut is busy with all come from real data. The system does not hunt for leads: the only thing that comes
// in from deep space is a comet — a new number that wrote to a sales number first. Pure, so it is tested.

export type PlanetLook = 'key' | 'normal' | 'risk' | 'closed'
export type Tone = 'good' | 'warn' | 'bad' | 'neutral'
/** the four agents that fly out to planets; AI Stok works the cargo bay and AI Prospek the radar */
export type FlyKind = 'order' | 'collect' | 'credit' | 'followup'
export type AgentKey = FlyKind | 'stock' | 'prospect'

export interface Planet {
  id: string
  name: string
  look: PlanetLook
  tone: Tone
  omzet: number
  dueIn: number | null
  /** orbit radius, 0 (next to the core: jadwal order now or late) … 1 (far out: no jadwal or far away) */
  orbit: number
  /** planet size, 0 (smallest omzet) … 1 (biggest) */
  size: number
  /** starting angle on the orbit (radians) */
  angle: number
}

export interface Comet { id: string; name: string; at: string | null }

export interface StationPlan {
  planets: Planet[]
  tasks: Record<FlyKind, string[]>
  comets: Comet[]
}

/** The agents as the server names them (proposals.agent) → astronaut. */
export const AGENT_OF: Record<string, AgentKey> = {
  'AI Order': 'order', 'AI Penagihan': 'collect', 'AI Kredit': 'credit', 'AI Follow-up': 'followup', 'AI Stok': 'stock', 'AI Prospek': 'prospect',
}
export const AGENT_NAME: Record<AgentKey, string> = {
  order: 'AI Order', collect: 'AI Penagihan', credit: 'AI Kredit', followup: 'AI Follow-up', stock: 'AI Stok', prospect: 'AI Prospek',
}
export const AGENT_COLOR: Record<AgentKey, string> = {
  order: '#3b8cff', collect: '#ff4d5e', credit: '#a26bff', followup: '#ff9a2e', stock: '#2fd17a', prospect: '#e9eef7',
}

// days of jadwal order that map onto the orbit radius; further (or no jadwal) = the outer orbit
const FAR_DAYS = 45
const GOLDEN = Math.PI * (3 - Math.sqrt(5))

const short = (s: string) => s.replace(/^(PT|CV|UD|Toko|TB)\.?\s+/i, '').trim()
const toneOf = (d: BoardItem): Tone => {
  const s = d.metrics.credit.state
  return s === 'aman' ? 'good' : s === 'tipis' ? 'warn' : s === 'cash' ? 'neutral' : 'bad'
}
const lookOf = (d: BoardItem): PlanetLook =>
  d.metrics.status === 'Churn' ? 'closed' : d.metrics.status === 'At risk' ? 'risk' : d.metrics.status === 'Key account' ? 'key' : 'normal'

/** Orbit radius from the days to the next jadwal order: late or due today next to the core, none at the edge. */
export function orbitOf(dueIn: number | null, churn = false): number {
  if (churn || dueIn == null) return 1
  return Math.max(0, Math.min(1, dueIn / FAR_DAYS)) * 0.92
}

/** Pick up to `max` dealers: first some with a task (so the astronauts have work), then the biggest by omzet. */
export function planStation(board: BoardItem[], threads: ThreadView[], max: number, maxComets = 4): StationPlan {
  const active = board.filter((d) => d.metrics.segment !== 'Prospek' && d.metrics.status !== 'Prospek')
  const byOmzet = [...active].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln || a.name.localeCompare(b.name))
  const picked = new Map<string, BoardItem>()
  const take = (list: BoardItem[], n: number) => {
    for (const d of list) {
      if (picked.size >= max || n <= 0) break
      if (!picked.has(d.id)) { picked.set(d.id, d); n-- }
    }
  }
  const quota = Math.max(1, Math.floor(max / 5))
  take(byOmzet.filter(dueSoon), quota)
  take(byOmzet.filter(mustCollect), quota)
  take(byOmzet.filter((d) => d.metrics.status === 'At risk'), quota)
  take(byOmzet.filter((d) => d.metrics.credit.state === 'tipis'), quota)
  take(byOmzet, max)
  const chosen = [...picked.values()].sort((a, b) => b.metrics.omzet_bln - a.metrics.omzet_bln || a.name.localeCompare(b.name))
  const top = Math.sqrt(Math.max(1, chosen[0]?.metrics.omzet_bln ?? 1))
  const planets: Planet[] = chosen.map((d, i) => ({
    id: d.id, name: short(d.short_name || d.name), look: lookOf(d), tone: toneOf(d), omzet: d.metrics.omzet_bln, dueIn: d.metrics.due_in ?? null,
    orbit: orbitOf(d.metrics.due_in ?? null, d.metrics.status === 'Churn'),
    size: Math.sqrt(Math.max(0, d.metrics.omzet_bln)) / top,
    angle: (i * GOLDEN) % (Math.PI * 2),
  }))
  const tasks: Record<FlyKind, string[]> = { order: [], collect: [], credit: [], followup: [] }
  for (const d of chosen) {
    if (dueSoon(d)) tasks.order.push(d.id)
    if (mustCollect(d)) tasks.collect.push(d.id)
    if (d.metrics.credit.state === 'tipis') tasks.credit.push(d.id)
    if (d.metrics.status === 'At risk') tasks.followup.push(d.id)
  }
  // comets: new numbers that wrote in first (never suppliers), newest first
  const comets = threads
    .filter((t) => t.kind === 'new' && t.contact_type !== 'supplier' && t.last_direction !== 'out')
    .sort((a, b) => (b.last_message_at ?? '').localeCompare(a.last_message_at ?? ''))
    .slice(0, maxComets)
    .map((t) => ({ id: t.id, name: t.title, at: t.last_message_at }))
  return { planets, tasks, comets }
}

/** The agents of the proposals waiting for a decision → one capsule each at the Dek Keputusan (max shown). */
export function capsules(agents: string[], max = 12): AgentKey[] {
  return agents.slice(0, max).map((a) => AGENT_OF[a] ?? 'order')
}
