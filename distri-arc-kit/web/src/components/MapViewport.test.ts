import { expect, test } from 'vitest'
import type { BoardItem } from '../api/types'
import { CX, CY, layoutReadable, place } from '../features/orbit/geometry'
import { MAX_ZOOM, MIN_DOT, clientToMap, spreadOf, viewRect, zoomAround } from './MapViewport'

const W = 800
const H = 740
const home = { cx: W / 2, cy: H / 2, zoom: 1 }

test('at zoom 1 the whole map is visible; at 2× half of it', () => {
  expect(viewRect(home, W, H)).toEqual({ x: 0, y: 0, w: W, h: H })
  const r = viewRect({ ...home, zoom: 2 }, W, H)
  expect(r.w).toBeCloseTo(W / 2)
  expect(r.h).toBeCloseTo(H / 2)
  expect(r.x).toBeCloseTo(W / 4)
})

test('full screen on a wide screen shows the whole map height and widens the view', () => {
  const r = viewRect(home, W, H, 16 / 9)
  expect(r.h).toBeCloseTo(H)
  expect(r.w).toBeCloseTo(H * (16 / 9))
})

test('client point maps to the map point under it, letterbox included', () => {
  const rect = { left: 100, top: 50, width: 1600, height: 740 } // 2.16:1 box around an 800×740 map → side bars
  const p = clientToMap(rect, { x: 0, y: 0, w: W, h: H }, 100 + 800, 50 + 370)
  expect(p.x).toBeCloseTo(400)
  expect(p.y).toBeCloseTo(370)
  expect(p.s).toBeCloseTo(1)
})

test('zooming keeps the point under the cursor in place and stays within limits', () => {
  const v = zoomAround(home, 2, 200, 185, W, H)
  expect(v.zoom).toBe(2)
  const before = { x: (200 - 0) / W, y: (185 - 0) / H } // fraction of the view where the point was
  const r = viewRect(v, W, H)
  expect((200 - r.x) / r.w).toBeCloseTo(before.x)
  expect((185 - r.y) / r.h).toBeCloseTo(before.y)
  expect(zoomAround(v, 100, 0, 0, W, H).zoom).toBe(MAX_ZOOM)
  expect(zoomAround(v, 0.01, 0, 0, W, H)).toMatchObject({ zoom: 1 })
})

test('smallest dots sit on their true position; normal dots may be spread', () => {
  expect(spreadOf(MIN_DOT)).toBe(0)
  expect(spreadOf(1)).toBe(1)
  expect(spreadOf(2)).toBe(1)
})

test('orbit spreads stacked dots less when they are smaller on screen (zoomed in)', () => {
  const dealer = (i: number) => ({ id: 'd' + i, name: 'Dealer ' + i, metrics: { cyc: 2.5, rhythm_days: 14, status: 'Churn', sow: 50, sow_source: 'estimated', avg_order: 30e6, credit: { state: 'aman' } } }) as unknown as BoardItem
  const many = Array.from({ length: 40 }, (_, i) => dealer(i))
  const shift = (scale: number) => {
    const n = layoutReadable(many, new Set(), null, 1.2, scale)
    return Math.max(...n.map((x) => Math.hypot(x.x - x.ox, x.y - x.oy)))
  }
  expect(shift(1 / 8)).toBeLessThan(shift(1) / 4)
  for (const x of layoutReadable(many, new Set(), null, 1.2, 1 / 8)) expect(Math.hypot(x.x - CX, x.y - CY)).toBeCloseTo(place(x.d).rad, 3)
})
