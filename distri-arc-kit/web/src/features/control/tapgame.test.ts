import { describe, expect, it } from 'vitest'
import { START_SPEED, advance, gap, newGame, press } from './tapgame'

describe('tap tap Enter', () => {
  it('moves the dot round the ring', () => {
    expect(advance(newGame(), 500).angle).toBe(START_SPEED / 2)
    expect(advance({ ...newGame(), angle: 350 }, 1000).angle).toBe((350 + START_SPEED) % 360)
    expect(gap(350, 10)).toBe(20)
    expect(gap(10, 350)).toBe(20)
  })
  it('scores a press inside the target, faster and narrower after', () => {
    const g = { ...newGame(3), angle: 190 } // target 180 ± 25
    const h = press(g, 0)
    expect(h).toMatchObject({ score: 1, best: 3, last: 'hit' })
    expect(h.speed).toBeGreaterThan(g.speed)
    expect(h.width).toBeLessThan(g.width)
    expect(gap(h.target, h.angle)).toBeGreaterThanOrEqual(90)
  })
  it('a miss ends the streak and keeps the best', () => {
    let g = { ...newGame(), angle: 180 }
    g = press(g, 0.5)
    g = press({ ...g, angle: g.angle }, 0) // far from the new target
    expect(g).toMatchObject({ score: 0, best: 1, last: 'miss', speed: START_SPEED })
  })
})
