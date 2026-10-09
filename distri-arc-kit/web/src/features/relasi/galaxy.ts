// Galaxy layout of Peta relasi: every sales number is a planet, every dealer a satellite orbiting the sales it is
// closest to (most interactions). Orbit distance = closeness (strongest relation on the innermost orbit), satellite
// size = interactions per month, orbital speed falls with distance (Kepler-like). Planets are scattered through 3D
// space around the galaxy core at seeded-random spots, each system on its own tilted plane, never overlapping.
// Dealers without any interaction drift in an outer belt. Prospects (leads that never ordered) fly through as comets
// (a sales owns them: a long elliptical orbit around that planet) or meteors (nobody owns them: straight drift).
// Pure and deterministic: the same data always gives the same galaxy.

export interface GNodeIn { id: string; type: 'sales' | 'dealer'; total: number }
export interface GEdgeIn { sales: string; dealer: string; w: number }

export interface Planet {
  id: string
  x: number
  y: number
  z: number
  /** orientation of the system's orbital plane: tilt from horizontal and the direction it tilts to */
  tilt: number
  node: number
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

/** mulberry32 seeded from a string: random-looking but the same for the same data. */
export function rng(seed: string) {
  let a = 0
  for (let i = 0; i < seed.length; i++) a = (Math.imul(a ^ seed.charCodeAt(i), 2654435761) + i) | 0
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** Point of an orbit: angle a on a circle (or ellipse) of radius r in a plane tilted by `tilt` towards `node`. */
export function onPlane(r: number, a: number, tilt: number, node: number) {
  const lx = Math.cos(a) * r
  const lz = Math.sin(a) * r
  const y = -lz * Math.sin(tilt)
  const z0 = lz * Math.cos(tilt)
  return { x: lx * Math.cos(node) - z0 * Math.sin(node), y, z: lx * Math.sin(node) + z0 * Math.cos(node) }
}
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
    const rnd = rng('tilt:' + s.id)
    return { id: s.id, x: 0, y: 0, z: 0, tilt: 0.15 + rnd() * 0.9, node: rnd() * Math.PI * 2, r, sr: inner + spread + 1.2, sats: list.map((d) => d.id) }
  })

  // planets scattered at random through a flattened sphere around the core (biggest systems placed first), each
  // spot tried until it overlaps neither the core nor another system; the space grows when it gets crowded
  const order = [...planets].sort((a, b) => (total.get(b.id) ?? 0) - (total.get(a.id) ?? 0) || a.id.localeCompare(b.id))
  const maxSr = Math.max(4, ...planets.map((p) => p.sr))
  const coreClear = CORE + maxSr * 0.7
  const vol = planets.reduce((a, p) => a + (p.sr + 1) ** 3, 0)
  let R = Math.max(coreClear + maxSr * 2, Math.cbrt(vol * 6))
  const rnd = rng('galaxy:' + order.map((p) => p.id).join(','))
  const placed: Planet[] = []
  for (const p of order) {
    for (let tries = 0; ; tries++) {
      if (tries && tries % 60 === 0) R *= 1.12
      const u = rnd() * 2 - 1
      const th = rnd() * Math.PI * 2
      const q = Math.cbrt(rnd()) * R
      const k = Math.sqrt(1 - u * u)
      const x = q * k * Math.cos(th)
      const y = q * u * 0.55 // flattened like a galaxy disc, but far from a single plane
      const z = q * k * Math.sin(th)
      if (Math.hypot(x, y, z) < p.sr + coreClear) continue
      if (placed.some((o) => Math.hypot(o.x - x, o.y - y, o.z - z) < o.sr + p.sr + 1.5)) continue
      p.x = x
      p.y = y
      p.z = z
      placed.push(p)
      break
    }
  }

  let extent = Math.max(6, ...planets.map((p) => Math.hypot(p.x, p.y, p.z) + p.sr))
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

/** A prospect: a lead that never ordered. owner = sales node id that owns it, or '' when nobody does. */
export interface LeadIn { id: string; owner: string }

export interface Comet {
  id: string
  planet: string
  /** ellipse: semi-major axis, eccentricity; the planet sits in a focus */
  a: number
  e: number
  phase: number
  speed: number
  tilt: number
  node: number
  /** direction of the ellipse's long axis within its plane */
  peri: number
}

export interface Meteor {
  id: string
  x: number
  y: number
  z: number
  vx: number
  vy: number
  vz: number
}

/** Comets for owned prospects (around their sales planet), meteors for the rest (drifting through the galaxy). */
export function layoutLeads(g: Galaxy, leads: LeadIn[]): { comets: Comet[]; meteors: Meteor[] } {
  const byId = new Map(g.planets.map((p) => [p.id, p]))
  const comets: Comet[] = []
  const meteors: Meteor[] = []
  for (const l of leads) {
    const r = rng('lead:' + l.id)
    const p = l.owner ? byId.get(l.owner) : undefined
    if (p) {
      const a = p.sr * (1.3 + r() * 1.3) // long ellipses that swing far outside the system and dive back in
      comets.push({ id: l.id, planet: p.id, a, e: 0.62 + r() * 0.25, phase: r() * Math.PI * 2, speed: 0.5 / Math.pow(a, 1.2), tilt: p.tilt + (r() - 0.5) * 1.2, node: p.node + (r() - 0.5) * 1.5, peri: r() * Math.PI * 2 })
    } else {
      const E = g.extent
      const u = r() * 2 - 1
      const th = r() * Math.PI * 2
      const k = Math.sqrt(1 - u * u)
      const sp = 0.6 + r() * 1.4
      meteors.push({ id: l.id, x: (r() * 2 - 1) * E, y: (r() * 2 - 1) * E * 0.5, z: (r() * 2 - 1) * E, vx: k * Math.cos(th) * sp, vy: u * sp * 0.4, vz: k * Math.sin(th) * sp })
    }
  }
  return { comets, meteors }
}

/** Comet position at time t (seconds): Kepler's equation, so it races past the planet and lingers far out. */
export function cometAt(c: Comet, t: number) {
  const M = c.phase + c.speed * t
  let E = M
  for (let i = 0; i < 6; i++) E -= (E - c.e * Math.sin(E) - M) / (1 - c.e * Math.cos(E))
  const th = 2 * Math.atan2(Math.sqrt(1 + c.e) * Math.sin(E / 2), Math.sqrt(1 - c.e) * Math.cos(E / 2))
  const rr = c.a * (1 - c.e * Math.cos(E))
  return { ...onPlane(rr, th + c.peri, c.tilt, c.node), rr }
}
