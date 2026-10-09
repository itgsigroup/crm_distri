// Galaxy layout of Peta relasi: every sales number is a planet, every dealer a satellite orbiting the sales it is
// closest to (most interactions). Orbit distance = closeness (strongest relation on the innermost orbit), satellite
// size = interactions per month, orbital speed falls with distance (Kepler-like). Planets sit on spiral arms around
// the galaxy core, spaced so their systems never overlap. Dealers without any interaction drift in an outer belt.
// Pure and deterministic: the same data always gives the same galaxy.

export interface GNodeIn { id: string; type: 'sales' | 'dealer'; total: number }
export interface GEdgeIn { sales: string; dealer: string; w: number }

export interface Planet {
  id: string
  x: number
  z: number
  r: number
  /** radius of the whole system (outermost orbit + margin) */
  sr: number
  sats: string[]
}

export interface Satellite {
  id: string
  /** planet id, or '' for the outer belt (orbits the galaxy core) */
  planet: string
  orbit: number
  phase: number
  /** radians per second */
  speed: number
  /** orbit inclination, radians */
  incl: number
  r: number
  w: number
}

export interface Galaxy {
  planets: Planet[]
  sats: Satellite[]
  /** radius that contains everything */
  extent: number
}

const GOLDEN = Math.PI * (3 - Math.sqrt(5))
/** minimum clear radius around the galaxy core (grows with the largest system) */
export const CORE = 4

/** Small deterministic value in [-1, 1] from a string. */
function hash01(s: string) {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619)
  return ((h >>> 0) / 4294967295) * 2 - 1
}

export const planetRadius = (total: number, months: number) => Math.min(2.6, 0.9 + Math.sqrt(total / Math.max(1, months)) * 0.12)
export const satRadius = (total: number, months: number) => Math.min(0.9, 0.22 + Math.sqrt(total / Math.max(1, months)) * 0.07)

export function layoutGalaxy(nodes: GNodeIn[], edges: GEdgeIn[], months: number): Galaxy {
  const sales = nodes.filter((n) => n.type === 'sales')
  const dealers = nodes.filter((n) => n.type === 'dealer')
  const salesIds = new Set(sales.map((s) => s.id))
  // each dealer orbits its strongest sales relation
  const best = new Map<string, GEdgeIn>()
  for (const e of edges) {
    if (e.w <= 0 || !salesIds.has(e.sales)) continue
    const cur = best.get(e.dealer)
    if (!cur || e.w > cur.w || (e.w === cur.w && e.sales < cur.sales)) best.set(e.dealer, e)
  }
  const total = new Map(nodes.map((n) => [n.id, n.total]))
  const bySales = new Map<string, { id: string; w: number }[]>(sales.map((s) => [s.id, []]))
  const belt: string[] = []
  for (const d of dealers) {
    const e = best.get(d.id)
    if (e) bySales.get(e.sales)!.push({ id: d.id, w: e.w })
    else belt.push(d.id)
  }

  const sats: Satellite[] = []
  const planets: Planet[] = sales.map((s) => {
    const r = planetRadius(s.total, months)
    const list = bySales.get(s.id)!.sort((a, b) => b.w - a.w || a.id.localeCompare(b.id))
    const maxW = list[0]?.w || 1
    const spread = 2.5 + Math.sqrt(list.length) * 0.9
    const inner = r + 1.4
    list.forEach((d, i) => {
      const closeness = d.w / maxW // 1 = strongest relation
      const orbit = inner + Math.pow(1 - closeness, 0.6) * spread + (i % 3) * 0.12
      sats.push({ id: d.id, planet: s.id, orbit, phase: i * GOLDEN + hash01(d.id) * 0.3, speed: 0.9 / Math.pow(orbit, 1.5), incl: hash01(d.id + '#') * 0.22, r: satRadius(total.get(d.id) ?? 0, months), w: d.w })
    })
    return { id: s.id, x: 0, z: 0, r, sr: inner + spread + 1.2, sats: list.map((d) => d.id) }
  })

  // planets on spiral arms (largest systems nearest the core), then pushed apart so systems never overlap
  const order = [...planets].sort((a, b) => (total.get(b.id) ?? 0) - (total.get(a.id) ?? 0) || a.id.localeCompare(b.id))
  const maxSr = Math.max(4, ...planets.map((p) => p.sr))
  order.forEach((p, i) => {
    const arm = i % 2 ? Math.PI : 0
    const rad = maxSr * (1.1 + 1.25 * Math.sqrt(i))
    const ang = arm + i * 0.55 + rad * 0.04
    p.x = Math.cos(ang) * rad
    p.z = Math.sin(ang) * rad
  })
  const coreClear = CORE + maxSr * 0.7
  for (let it = 0; it < 120; it++) {
    let moved = false
    for (let i = 0; i < order.length; i++)
      for (let j = i + 1; j < order.length; j++) {
        const a = order[i]
        const b = order[j]
        let dx = b.x - a.x
        let dz = b.z - a.z
        let d = Math.hypot(dx, dz)
        const min = a.sr + b.sr + 1
        if (d >= min) continue
        if (d < 1e-6) { dx = 1; dz = 0; d = 1 }
        const push = (min - d) / 2
        a.x -= (dx / d) * push
        a.z -= (dz / d) * push
        b.x += (dx / d) * push
        b.z += (dz / d) * push
        moved = true
      }
    for (const p of order) { // keep the galaxy core clear
      const d = Math.hypot(p.x, p.z)
      const min = p.sr + coreClear
      if (d >= min) continue
      const k = d < 1e-6 ? 1 : d
      p.x = d < 1e-6 ? min : (p.x / k) * min
      p.z = d < 1e-6 ? 0 : (p.z / k) * min
      moved = true
    }
    if (!moved) break
  }

  let extent = Math.max(6, ...planets.map((p) => Math.hypot(p.x, p.z) + p.sr))
  if (belt.length) {
    const r0 = extent + 2
    belt.sort().forEach((id, i) => {
      const orbit = r0 + (i % 4) * 0.8 + Math.abs(hash01(id)) * 0.6
      sats.push({ id, planet: '', orbit, phase: i * GOLDEN, speed: 2.5 / Math.pow(orbit, 1.5), incl: hash01(id + '#') * 0.05, r: satRadius(total.get(id) ?? 0, months), w: 0 })
    })
    extent = r0 + 4
  }
  return { planets, sats, extent }
}
