import { describe, expect, it } from 'vitest'
import { createLayout, positions, reweight, settle, springLen } from './layout'

const input = {
  nodes: [
    { id: 's-rizky', type: 'sales' as const, total: 134 },
    { id: 's-dewi', type: 'sales' as const, total: 82 },
    { id: 'indo', type: 'dealer' as const, total: 104 },
    { id: 'jaya', type: 'dealer' as const, total: 30 },
    { id: 'sarana', type: 'dealer' as const, total: 82 },
  ],
  edges: [
    { sales: 's-rizky', dealer: 'indo', w: 104 },
    { sales: 's-rizky', dealer: 'jaya', w: 30 },
    { sales: 's-dewi', dealer: 'sarana', w: 82 },
  ],
  months: 1,
}

describe('Peta relasi layout', () => {
  it('is deterministic for the same data and seed', () => {
    const a = positions(settle(createLayout({ ...input, seed: 7 })))
    const b = positions(settle(createLayout({ ...input, seed: 7 })))
    expect(a).toEqual(b)
    const c = positions(settle(createLayout({ ...input, seed: 8 })))
    expect(c).not.toEqual(a)
  })

  it('pulls strong pairs closer than weak ones', () => {
    const l = settle(createLayout(input))
    const p = new Map(l.nodes.map((n) => [n.id, n]))
    const dist = (a: string, b: string) => Math.hypot(p.get(a)!.x - p.get(b)!.x, p.get(a)!.y - p.get(b)!.y, p.get(a)!.z - p.get(b)!.z)
    expect(dist('s-rizky', 'indo')).toBeLessThan(dist('s-rizky', 'jaya'))
    expect(springLen(104, 1)).toBeLessThan(springLen(30, 1))
  })

  it('node size grows with interactions per month and settles finite', () => {
    const l = settle(createLayout(input))
    const indo = l.nodes.find((n) => n.id === 'indo')!
    const jaya = l.nodes.find((n) => n.id === 'jaya')!
    expect(indo.r).toBeGreaterThan(jaya.r)
    expect(l.nodes.every((n) => Number.isFinite(n.x) && Number.isFinite(n.y) && Number.isFinite(n.z))).toBe(true)
  })

  it('a new period reweights without moving nodes', () => {
    const l = settle(createLayout(input))
    const before = positions(l)
    reweight(l, [{ sales: 's-rizky', dealer: 'indo', w: 553 }], new Map([['indo', 553]]), 6)
    expect(positions(l)).toEqual(before)
    expect(l.springs.find((s) => l.nodes[s.b].id === 'jaya')!.w).toBe(0)
    expect(l.nodes.find((n) => n.id === 'indo')!.r).toBeCloseTo(0.3 + Math.sqrt(553 / 6) / 11)
  })
})
