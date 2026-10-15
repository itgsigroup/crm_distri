import { describe, expect, it } from 'vitest'
import { countLabel, marks, step } from './findInChat'

describe('cari di dalam chat', () => {
  it('marks every match, any case, any spacing', () => {
    expect(marks('Kamera IP sudah, kamera  ip lagi', 'kamera ip')).toEqual([
      { t: 'Kamera IP', hit: true }, { t: ' sudah, ', hit: false }, { t: 'kamera  ip', hit: true }, { t: ' lagi', hit: false },
    ])
    expect(marks('diskon 10% (net)', '10% (')).toEqual([{ t: 'diskon ', hit: false }, { t: '10% (', hit: true }, { t: 'net)', hit: false }])
    expect(marks('halo', 'x')).toEqual([{ t: 'halo', hit: false }])
  })

  it('steps through results newest first and stops at the ends', () => {
    expect(step(-1, 3, 'up')).toBe(0)
    expect(step(0, 3, 'up')).toBe(1)
    expect(step(2, 3, 'up')).toBe(2)
    expect(step(1, 3, 'down')).toBe(0)
    expect(step(0, 3, 'down')).toBe(0)
    expect(step(0, 0, 'up')).toBe(-1)
  })

  it('counts', () => {
    expect(countLabel(2, 12, false)).toBe('3 dari 12')
    expect(countLabel(-1, 5, true)).toBe('5 dari 5+')
    expect(countLabel(-1, 0, false)).toBe('Tidak ada hasil')
  })
})
