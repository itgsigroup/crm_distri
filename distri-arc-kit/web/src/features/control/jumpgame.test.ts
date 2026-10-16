import { describe, expect, it } from 'vitest'
import { PX, jump, newRun, score, step } from './jumpgame'

const frames = (r: ReturnType<typeof newRun>, n: number, rand = 0.5) => {
  for (let i = 0; i < n; i++) r = step(r, 16, rand)
  return r
}

describe('Lompat!', () => {
  it('waits for the first press, then runs and lands again', () => {
    expect(step(newRun(), 16, 0.5).dist).toBe(0)
    let r = jump(newRun())
    r = frames(r, 10)
    expect(r.y).toBeGreaterThan(20)
    r = frames(r, 80)
    expect(r.y).toBe(0)
    expect(r.jumps).toBe(0)
    expect(r.dist).toBeGreaterThan(0)
  })
  it('allows a double jump, not a third', () => {
    let r = frames(jump(newRun()), 5)
    r = jump(r)
    expect(r.jumps).toBe(2)
    expect(jump(r)).toBe(r)
  })
  it('a pipe ends the run unless jumped over; the next press starts again with the best kept', () => {
    let r = { ...newRun(), started: true, pipes: [{ x: PX + 30, w: 22, h: 30, id: 1 }], next: 999 }
    r = frames(r, 30)
    expect(r.over).toBe(true)
    const best = r.best
    r = jump(r)
    expect(r.over).toBe(false)
    expect(r.best).toBe(best)
    let ok = jump({ ...newRun(), started: false, pipes: [{ x: PX + 50, w: 22, h: 30, id: 1 }], next: 999 })
    ok = frames(ok, 60)
    expect(ok.over).toBe(false)
  })
  it('scores distance and coins, and makes pipes as it goes', () => {
    let r = { ...newRun(), started: true, coins: [{ x: PX + 8, y: 8, id: 1 }], next: 999 }
    r = step(r, 16, 0.5)
    expect(r.coinsGot).toBe(1)
    expect(score(r)).toBe(5)
    const far = frames({ ...newRun(), started: true, next: 1 }, 3)
    expect(far.pipes.length).toBe(1)
  })
})
