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
