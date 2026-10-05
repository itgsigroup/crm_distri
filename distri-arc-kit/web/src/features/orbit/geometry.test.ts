import { expect, test } from 'vitest'
import type { BoardItem } from '../../api/types'
import { CX, CY, RING_R, layoutOrbit, place } from './geometry'

const dealer = (over: Partial<BoardItem['metrics']>, name = 'Toko Uji'): BoardItem =>
  ({ id: name, name, metrics: { cyc: 0.5, rhythm_days: 14, status: 'Aktif', sow: 50, avg_order: 30e6, credit: { state: 'aman' }, ...over } }) as unknown as BoardItem

test('key account sits inside its ring, shrinking radius with share of wallet', () => {
  const p = place(dealer({ status: 'Key account', sow: 72, cyc: 6 / 14 }))
  expect(p.rad).toBeCloseTo(92 - 12 - (22 / 50) * 22, 6)
  expect(p.ang).toBeCloseTo((6 / 14) * Math.PI * 2, 6)
  expect(p.size).toBeCloseTo(7 + Math.sqrt(72) * 1.3, 6)
})

test('at risk interpolates between Aktif and At risk rings by cyc', () => {
  const p = place(dealer({ status: 'At risk', cyc: 30 / 21, sow: 40 }))
  expect(p.rad).toBeCloseTo(170 + 14 + ((30 / 21 - 1.2) / 0.8) * (248 - 170 - 26), 6)
  expect(p.ang).toBeCloseTo(Math.PI * 2, 6) // cyc ≥ 1 → top (jadwal order)
})

test('churn is drawn just inside the dashed outer ring', () => {
  expect(place(dealer({ status: 'Churn', cyc: 2.5 })).rad).toBe(RING_R.Churn - 8)
})

test('baru has no rhythm: Aktif ring at 0.1 turn', () => {
  const p = place(dealer({ status: 'Baru', rhythm_days: null, cyc: 0, sow: 20 }))
  expect(p.rad).toBeCloseTo(170 - 8 - 10, 6)
  expect(p.ang).toBeCloseTo(0.1 * Math.PI * 2, 6)
})

test('layout keeps nodes inside the board and separates overlapping ones', () => {
  const same = [dealer({}, 'A Satu'), dealer({}, 'B Dua')]
  const n = layoutOrbit(same)
  for (const x of n) {
    expect(x.x).toBeGreaterThan(0)
    expect(x.x).toBeLessThan(800)
    expect(x.y).toBeGreaterThanOrEqual(40)
    expect(x.y).toBeLessThanOrEqual(710)
  }
  expect(Math.hypot(n[0].x - n[1].x, n[0].y - n[1].y)).toBeGreaterThan(5)
  expect(Math.abs(n[0].ox - CX) + Math.abs(n[0].oy - CY)).toBeGreaterThan(0)
})
