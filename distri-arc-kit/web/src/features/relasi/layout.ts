// Force layout of Peta relasi — the physics of the mockup's createNet (repulsion, weighted springs, gravity),
// with a seeded random start so the same data always settles into the same picture (testable, no jumps between
// visits). Runs in a Web Worker for the initial settle; the view keeps stepping it lightly per frame.

export interface LNode {
  id: string
  type: 'sales' | 'dealer'
  r: number
  tot: number
  x: number
  y: number
  z: number
  vx: number
  vy: number
  vz: number
}

export interface LSpring {
  a: number // node index
  b: number
  w: number
  len: number
}

export interface LayoutInput {
  nodes: { id: string; type: 'sales' | 'dealer'; total: number }[]
  edges: { sales: string; dealer: string; w: number }[]
  months: number
  small?: boolean
  seed?: number
}

export interface Layout {
  nodes: LNode[]
  springs: LSpring[]
  small: boolean
  months: number
}

/** mulberry32: tiny deterministic PRNG. */
export function rng(seed: number) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export const nodeRadius = (type: 'sales' | 'dealer', tot: number, months: number, small = false) =>
  type === 'sales' ? (small ? 0.9 : 1) : 0.3 + Math.sqrt(tot / months) / 11

export const springLen = (w: number, months: number) => 1.3 + 7 / (1 + Math.log(1 + w / months) * 0.9)

export function createLayout(inp: LayoutInput): Layout {
  const small = !!inp.small
  const rand = rng(inp.seed ?? 42)
  const seedR = () => (rand() - 0.5) * (small ? 8 : 16)
  const sales = inp.nodes.filter((n) => n.type === 'sales')
  const nodes: LNode[] = []
  sales.forEach((s, i) => {
    const a = (i / Math.max(1, sales.length)) * Math.PI * 2
    nodes.push({ id: s.id, type: 'sales', r: nodeRadius('sales', 0, inp.months, small), tot: s.total, x: Math.cos(a) * (small ? 3 : 6), y: (rand() - 0.5) * 3, z: Math.sin(a) * (small ? 3 : 6), vx: 0, vy: 0, vz: 0 })
  })
  inp.nodes.filter((n) => n.type === 'dealer').forEach((d) => {
    nodes.push({ id: d.id, type: 'dealer', r: nodeRadius('dealer', d.total, inp.months, small), tot: d.total, x: seedR(), y: seedR(), z: seedR(), vx: 0, vy: 0, vz: 0 })
  })
  const index = new Map(nodes.map((n, i) => [n.id, i]))
  const springs: LSpring[] = []
  for (const e of inp.edges) {
    const a = index.get(e.sales)
    const b = index.get(e.dealer)
    if (a === undefined || b === undefined) continue
    springs.push({ a, b, w: e.w, len: springLen(e.w, inp.months) })
  }
  return { nodes, springs, small, months: inp.months }
}

/** One physics step (mockup I.step). */
export function step(l: Layout, dt: number) {
  const N = l.nodes
  const rep = l.small ? 4 : 6
  for (let i = 0; i < N.length; i++) {
    const a = N[i]
    for (let j = i + 1; j < N.length; j++) {
      const b = N[j]
      let dx = a.x - b.x
      let dy = a.y - b.y
      let dz = a.z - b.z
      const d2 = dx * dx + dy * dy + dz * dz + 0.05
      const f = rep / d2
      const d = Math.sqrt(d2)
      dx /= d
      dy /= d
      dz /= d
      a.vx += dx * f
      a.vy += dy * f
      a.vz += dz * f
      b.vx -= dx * f
      b.vy -= dy * f
      b.vz -= dz * f
    }
  }
  for (const sp of l.springs) {
    const a = N[sp.a]
    const b = N[sp.b]
    let dx = b.x - a.x
    let dy = b.y - a.y
    let dz = b.z - a.z
    const d = Math.sqrt(dx * dx + dy * dy + dz * dz) + 0.001
    const f = (d - sp.len) * 0.06
    dx /= d
    dy /= d
    dz /= d
    a.vx += dx * f
    a.vy += dy * f
    a.vz += dz * f
    b.vx -= dx * f
    b.vy -= dy * f
    b.vz -= dz * f
  }
  for (const n of N) {
    n.vx -= n.x * 0.02
    n.vy -= n.y * 0.03
    n.vz -= n.z * 0.02
    n.vx *= 0.82
    n.vy *= 0.82
    n.vz *= 0.82
    n.x += n.vx * dt
    n.y += n.vy * dt
    n.z += n.vz * dt
  }
}

/** Initial settle (420 steps like the mockup). */
export function settle(l: Layout, steps = 420) {
  for (let i = 0; i < steps; i++) step(l, 1)
  return l
}

/** New period: weights, spring lengths and node sizes change; positions stay. */
export function reweight(l: Layout, edges: LayoutInput['edges'], totals: Map<string, number>, months: number) {
  l.months = months
  const w = new Map(edges.map((e) => [e.sales + '|' + e.dealer, e.w]))
  for (const sp of l.springs) {
    sp.w = w.get(l.nodes[sp.a].id + '|' + l.nodes[sp.b].id) ?? 0
    sp.len = springLen(sp.w, months)
  }
  for (const n of l.nodes) {
    n.tot = totals.get(n.id) ?? 0
    n.r = nodeRadius(n.type, n.tot, months, l.small)
  }
}

export type Positions = [number, number, number][]

export const positions = (l: Layout): Positions => l.nodes.map((n) => [n.x, n.y, n.z])
