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
export function layoutOrbit(list: BoardItem[], drift = 1.2, dense = false, named?: Set<string>): OrbitNode[] {
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
  if (named) { // context dots stay small and in place; only the named ones are spread apart
    const dots = nodes.filter((n) => !named.has(n.d.id))
    dots.forEach((n) => { n.size = Math.max(3, n.size * 0.4); n.name = '' })
    const lab = layoutNamed(nodes.filter((n) => named.has(n.d.id)), box)
    return [...dots, ...lab.sort((a, b) => b.size - a.size)]
  }
  return layoutNamed(nodes, box).sort((a, b) => b.size - a.size)
}

function layoutNamed(nodes: OrbitNode[], box: (a: OrbitNode) => number[]): OrbitNode[] {
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
  return nodes
}

/** A dealer on the readable orbit (real data, ADR 0023 era): the dot stays on its ring; the label may move. */
export interface ReadableNode extends OrbitNode {
  lx: number
  ly: number
  label: string
  dim: boolean
}

/** Above this many dealers the orbit switches to the readable layout (the mockup's 18 keep the mockup layout). */
export const READABLE_FROM = 40
export const READABLE_NAMED = 12

const clip = (s: string, n = 18) => (s.length > n ? s.slice(0, n - 1).trimEnd() + '…' : s)

/** Readable layout for hundreds of dealers. Glossary meaning is kept — ring = status, angle = position in the order
 * cycle, size = share of wallet, colour = sisa limit — but: (1) overlapping dots are spread only along their own
 * ring (never pushed into another ring), (2) an estimated share of wallet (the same 50% for everyone) gives a small
 * uniform dot instead of 150 equal large ones, (3) only `named` dealers get a short label, placed outside the dot
 * and stacked without overlap, with a leader line, (4) `focus` dims every other ring. */
export function layoutReadable(list: BoardItem[], named: Set<string>, focus: string | null = null, drift = 1.2): ReadableNode[] {
  const ns = list.map((d) => {
    const p = place(d, drift)
    const ring = ringOf(d)
    const est = d.metrics.sow_source !== 'confirmed'
    const size = est ? (ring === 'Key account' ? 6.5 : 5) : 4 + Math.sqrt(d.metrics.sow) * 0.8
    return { d, ring, rad: p.rad, ang: p.ang, size }
  })
  // spread along the ring: per ring, pack the angles so neighbours keep their radii apart, each packed run centred
  // on where its dealers belong (1-D label placement). Angles run −π…π so the jadwal line (top) is not a seam.
  const byRing = new Map<string, typeof ns>()
  for (const n of ns) {
    if (n.ang > Math.PI) n.ang -= Math.PI * 2
    byRing.set(n.ring, [...(byRing.get(n.ring) ?? []), n])
  }
  for (const group of byRing.values()) {
    group.sort((a, b) => a.ang - b.ang)
    const want = group.map((n) => n.ang)
    const gap = (i: number) => (group[i - 1].size + group[i].size + 1.5) / group[i].rad
    for (let pass = 0; pass < 12; pass++) {
      for (let i = 1; i < group.length; i++) group[i].ang = Math.max(group[i].ang, group[i - 1].ang + gap(i))
      // centre every packed run on the mean of where its members want to be
      let i = 0
      while (i < group.length) {
        let j = i
        while (j + 1 < group.length && group[j + 1].ang - group[j].ang <= gap(j + 1) + 1e-9) j++
        let shift = 0
        for (let k = i; k <= j; k++) shift += want[k] - group[k].ang
        shift /= j - i + 1
        for (let k = i; k <= j; k++) group[k].ang += shift
        i = j + 1
      }
    }
    for (let i = 1; i < group.length; i++) group[i].ang = Math.max(group[i].ang, group[i - 1].ang + gap(i))
  }
  const out: ReadableNode[] = ns.map(({ d, ring, rad, ang, size }) => {
    const x = CX + rad * Math.sin(ang)
    const y = CY - rad * Math.cos(ang)
    const side: 1 | -1 = x >= CX ? 1 : -1
    const label = named.has(d.id) ? clip(shortName(d.name)) : ''
    const dim = !!focus && ring !== focus
    return { d, x, y, ox: x, oy: y, size, tone: toneOf(d.metrics.credit.state), ring, name: label, sm: false, side, tw: label.length * 6.4 + 6, lx: x + side * (size + 5), ly: y + 4, label, dim }
  })
  // labels: outside the dot, stacked per side so none overlap; the dot never moves
  for (const side of [1, -1] as const) {
    const ls = out.filter((n) => n.label && !n.dim && n.side === side).sort((a, b) => a.ly - b.ly)
    for (let it = 0; it < 40; it++) {
      let moved = false
      for (let i = 1; i < ls.length; i++) {
        const a = ls[i - 1]
        const b = ls[i]
        const overlapX = side > 0 ? Math.min(a.lx + a.tw, b.lx + b.tw) - Math.max(a.lx, b.lx) : Math.min(a.lx, b.lx) - Math.max(a.lx - a.tw, b.lx - b.tw)
        if (overlapX > 0 && b.ly - a.ly < 15) {
          const p = (15 - (b.ly - a.ly)) / 2
          a.ly -= p
          b.ly += p
          moved = true
        }
      }
      if (!moved) break
    }
    ls.forEach((n) => { n.ly = Math.max(24, Math.min(H - 12, n.ly)) })
  }
  return out.sort((a, b) => Number(b.dim) - Number(a.dim) || Number(!!a.label) - Number(!!b.label))
}
