import { describe, expect, it } from 'vitest'
import { hasLabel, labelsOf, nextPin, pinLine, pinnedFirst } from './chatLabels'

const all = [
  { id: 'a', name: 'Pelanggan baru', color: '#34B7F1', chats: 2 },
  { id: 'b', name: 'Pesanan baru', color: '#F5B400', chats: 0 },
]

describe('pins and labels', () => {
  it('keeps the team order and drops deleted labels', () => {
    expect(labelsOf(['b', 'x', 'a'], all).map((l) => l.id)).toEqual(['a', 'b'])
    expect(labelsOf(undefined, all)).toEqual([])
  })
  it('filters chats by label', () => {
    expect(hasLabel({ labels: ['a'] }, 'a')).toBe(true)
    expect(hasLabel({}, 'a')).toBe(false)
    expect(hasLabel({}, '')).toBe(true)
  })
  it('cycles pins and writes one line', () => {
    expect(nextPin(0, 3)).toBe(1)
    expect(nextPin(2, 3)).toBe(0)
    expect(nextPin(0, 0)).toBe(0)
    expect(pinLine({ body: 'Harga\n\nkamera  4MP', from_name: '', direction: 'in' })).toBe('Harga kamera 4MP')
    expect(pinLine({ body: '', media: { kind: 'image', viewable: true }, from_name: '', direction: 'in' })).toBe('[image]')
  })
  it('puts pinned chats on top, last pinned first', () => {
    const l = [{ id: 'a' }, { id: 'b', pinned_at: '2026-10-15T08:00:00Z' }, { id: 'c' }, { id: 'd', pinned_at: '2026-10-15T09:00:00Z' }]
    expect(pinnedFirst(l).map((t) => t.id)).toEqual(['d', 'b', 'a', 'c'])
  })
})
