// Orbit board geometry — a direct port of renderOrbit() in the approved mockup.
// SVG viewBox 800×740, GSI at the centre; ring radius per status; angle = position in the order cycle.
import type { BoardItem } from '../../api/types'
import { shortName } from '../../lib/format'

export const W = 800
export const H = 740
export const CX = 400
export const CY = 370
export const RING_R: Record<string, number> = { 'Key account': 92, Aktif: 170, 'At risk': 248, Churn: 310 }
export const RINGS = ['Key account', 'Aktif', 'At risk', 'Churn'] as const

export type Tone = 'good' | 'warn' | 'bad' | 'neutral'
export const toneOf = (state: string): Tone => (state === 'aman' ? 'good' : state === 'tipis' ? 'warn' : state === 'cash' ? 'neutral' : 'bad')

export interface OrbitNode {
  d: BoardItem
  x: number
  y: number
  ox: number
  oy: number
  size: number
  tone: Tone
  ring: string
  name: string
  sm: boolean
  side: 1 | -1
  tw: number
}

/** The ring a dealer is drawn on (Baru sits on Aktif). */
export const ringOf = (d: BoardItem) => (d.metrics.status === 'Baru' ? 'Aktif' : d.metrics.status)

/** Radius and angle of one dealer before label collision avoidance. */
export function place(d: BoardItem, drift = 1.2): { rad: number; ang: number; size: number } {
  const c = d.metrics.cyc
  const r = ringOf(d)
  const sow = d.metrics.sow
  let rad: number
  if (r === 'Key account') rad = RING_R['Key account'] - 12 - ((sow - 50) / 50) * 22
  else if (r === 'Aktif') rad = RING_R.Aktif - 8 - (sow / 100) * 50
  else if (r === 'At risk') rad = RING_R.Aktif + 14 + ((Math.min(c, 2) - drift) / (2 - drift)) * (RING_R['At risk'] - RING_R.Aktif - 26)
  else rad = RING_R.Churn - 8
  const ang = (d.metrics.rhythm_days ? Math.min(c, 1) : 0.1) * Math.PI * 2
  return { rad, ang, size: 7 + Math.sqrt(sow) * 1.3 }
}

/** Lay out all nodes: polar placement, then 80 rounds of node+label overlap avoidance pulled back gently. The
 * avoidance is quadratic: it only runs for a readable number of labelled nodes (dense = plain dots, no labels). */
export function layoutOrbit(list: BoardItem[], drift = 1.2, dense = false): OrbitNode[] {
  const nodes: OrbitNode[] = list.map((d) => {
    const { rad, ang, size } = place(d, drift)
    const x = CX + rad * Math.sin(ang)
    const y = CY - rad * Math.cos(ang)
    const r = ringOf(d)
    const name = shortName(d.name)
    const sm = d.metrics.avg_order < 10e6 && r !== 'At risk' && r !== 'Churn'
    const side: 1 | -1 = x > CX + 150 ? -1 : 1
    return { d, x, y, ox: x, oy: y, size, tone: toneOf(d.metrics.credit.state), ring: r, name, sm, side, tw: name.length * (sm ? 5.8 : 6.6) + 6 }
  })
  const box = (a: OrbitNode) => {
    const h = Math.max(a.size, 8)
    return a.side > 0 ? [a.x - a.size, a.x + a.size + a.tw, a.y - h, a.y + h] : [a.x - a.size - a.tw, a.x + a.size, a.y - h, a.y + h]
  }
  if (dense) {
    nodes.forEach((n) => { n.size = Math.max(2.5, n.size * 0.35) })
    return nodes.sort((a, b) => b.size - a.size)
  }
  for (let it = 0; it < 80; it++) {
    for (let i = 0; i < nodes.length; i++)
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i]
        const b = nodes[j]
        const A = box(a)
        const B = box(b)
        const ox = Math.min(A[1], B[1]) - Math.max(A[0], B[0])
        const oy = Math.min(A[3], B[3]) - Math.max(A[2], B[2])
        if (ox > 0 && oy > 0) {
          const dir = a.y <= b.y ? -1 : 1
          const p = (oy / 2 + 2) * 0.35
          a.y += dir * p
          b.y -= dir * p
          const dx = a.x - b.x
          const dy = a.y - b.y
          const dist = Math.hypot(dx, dy) || 1
          const min = a.size + b.size + 6
          if (dist < min) {
            const q = (min - dist) / 2
            a.x += (dx / dist) * q
            b.x -= (dx / dist) * q
            a.y += (dy / dist) * q
            b.y -= (dy / dist) * q
          }
        }
      }
    nodes.forEach((n) => {
      n.x += (n.ox - n.x) * 0.04
      n.y += (n.oy - n.y) * 0.04
      n.x = Math.max(n.size + 4, Math.min(W - n.size - 4, n.x))
      n.y = Math.max(40, Math.min(H - 30, n.y))
    })
  }
  return nodes.sort((a, b) => b.size - a.size)
}
