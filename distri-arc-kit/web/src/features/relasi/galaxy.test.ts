import { expect, test } from 'vitest'
import { CORE, layoutGalaxy } from './galaxy'

const nodes = [
  { id: 's-a', type: 'sales' as const, total: 300 },
  { id: 's-b', type: 'sales' as const, total: 120 },
  { id: 'd1', type: 'dealer' as const, total: 90 },
  { id: 'd2', type: 'dealer' as const, total: 30 },
  { id: 'd3', type: 'dealer' as const, total: 60 },
  { id: 'd4', type: 'dealer' as const, total: 0 },
]
const edges = [
  { sales: 's-a', dealer: 'd1', w: 80 },
  { sales: 's-a', dealer: 'd2', w: 10 },
  { sales: 's-b', dealer: 'd2', w: 25 }, // d2 is closer to s-b
  { sales: 's-a', dealer: 'd3', w: 40 },
  { sales: 's-b', dealer: 'd4', w: 0 }, // no interaction → belt
]

test('each dealer orbits the sales it is closest to; no interaction → outer belt', () => {
  const g = layoutGalaxy(nodes, edges, 6)
  const of = (id: string) => g.sats.find((s) => s.id === id)!
  expect(of('d1').planet).toBe('s-a')
  expect(of('d2').planet).toBe('s-b')
  expect(of('d3').planet).toBe('s-a')
  expect(of('d4').planet).toBe('')
  expect(of('d4').orbit).toBeGreaterThan(Math.max(...g.planets.map((p) => Math.hypot(p.x, p.z) + p.sr)))
})

test('closer relation → inner, faster orbit', () => {
  const g = layoutGalaxy(nodes, edges, 6)
  const d1 = g.sats.find((s) => s.id === 'd1')!
  const d3 = g.sats.find((s) => s.id === 'd3')!
  expect(d1.orbit).toBeLessThan(d3.orbit)
  expect(d1.speed).toBeGreaterThan(d3.speed)
})

test('planet systems never overlap and the layout is deterministic', () => {
  const many = Array.from({ length: 12 }, (_, i) => ({ id: 's-' + i, type: 'sales' as const, total: 50 + i * 10 }))
  const ds = Array.from({ length: 200 }, (_, i) => ({ id: 'd' + i, type: 'dealer' as const, total: i % 40 }))
  const es = ds.map((d, i) => ({ sales: 's-' + (i % 12), dealer: d.id, w: 1 + (i % 17) }))
  const g = layoutGalaxy([...many, ...ds], es, 6)
  for (let i = 0; i < g.planets.length; i++)
    for (let j = i + 1; j < g.planets.length; j++) {
      const a = g.planets[i]
      const b = g.planets[j]
      expect(Math.hypot(a.x - b.x, a.z - b.z)).toBeGreaterThanOrEqual(a.sr + b.sr - 0.01)
    }
  for (const p of g.planets) expect(Math.hypot(p.x, p.z)).toBeGreaterThanOrEqual(p.sr + CORE - 0.5) // core stays clear
  expect(layoutGalaxy([...many, ...ds], es, 6)).toEqual(g)
})
