import { expect, test } from 'vitest'
import { B, H, L, R, T, W, clampY, px, py, xOf } from './scale'

test('x is linear: 0 at the left axis, 3.5×/bln at the right edge', () => {
  expect(px(0)).toBe(L)
  expect(px(3.5)).toBeCloseTo(W - R, 9)
  expect(px(1.75)).toBeCloseTo((L + W - R) / 2, 9)
})

test('y is logarithmic between 4 jt (bottom) and 200 jt (top)', () => {
  expect(py(200e6)).toBeCloseTo(T, 9)
  expect(py(4e6)).toBeCloseTo(H - B, 9)
  // equal ratios → equal distances
  expect(py(10e6) - py(20e6)).toBeCloseTo(py(50e6) - py(100e6), 9)
})

test('clamping and the Baru column', () => {
  expect(clampY(1e6)).toBeCloseTo(py(4e6 * 1.15), 9)
  expect(clampY(1e9)).toBeCloseTo(py(200e6 / 1.1), 9)
  expect(xOf(null)).toBeCloseTo(px(0.22), 9)
  expect(xOf(9)).toBeCloseTo(px(3.4), 9)
})

test('axis ticks follow the zoom: inverse scales, finer steps for a smaller range', async () => {
  const { freqAt, valueAt, xTicks, yTicks, fmtTick } = await import('./scale')
  expect(freqAt(px(1.3))).toBeCloseTo(1.3, 9)
  expect(valueAt(py(37e6))).toBeCloseTo(37e6, 0)
  expect(xTicks(0, 3.5)).toEqual([0.5, 1, 1.5, 2, 2.5, 3, 3.5])
  expect(xTicks(1, 1.5)).toEqual([1, 1.1, 1.2, 1.3, 1.4, 1.5])
  expect(yTicks(4e6, 200e6)).toEqual([5e6, 10e6, 20e6, 50e6, 100e6, 200e6])
  expect(yTicks(12e6, 25e6).length).toBeGreaterThanOrEqual(4)
  expect(fmtTick(5e6)).toBe('5 jt')
  expect(fmtTick(1.5e6)).toBe('1,5 jt')
  expect(fmtTick(150e6)).toBe('150 jt')
})

test('data beyond 3,5×/bln or 200 jt widens the range instead of piling dealers on the edge', async () => {
  const { domainOf, BASE, XSPLIT, freqAt, valueAt, xTicks } = await import('./scale')
  expect(domainOf([{ freq: 2, avg_order: 30e6 }])).toEqual(BASE) // mockup range unchanged
  const d = domainOf([{ freq: 12, avg_order: 400e6 }, { freq: 5.5, avg_order: 2e6 }, { freq: 1, avg_order: 30e6 }])
  expect(d.xtop).toBeGreaterThan(12)
  expect(px(3.5, d)).toBeCloseTo(L + (W - L - R) * XSPLIT, 9) // 0–3,5× keeps the first 80%
  expect(xOf(5.5, d)).toBeLessThan(xOf(12, d)) // no longer the same edge
  expect(xOf(12, d)).toBeLessThan(W - R)
  expect(freqAt(px(7, d), d)).toBeCloseTo(7, 6)
  expect(clampY(400e6, d)).toBeGreaterThan(T)
  expect(valueAt(py(2e6, d), d)).toBeCloseTo(2e6, -1)
  expect(xTicks(3, 13)).toEqual(expect.arrayContaining([3, 3.5, 4, 5, 10, 12]))
})
