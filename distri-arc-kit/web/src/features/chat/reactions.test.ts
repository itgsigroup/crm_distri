import { describe, expect, it } from 'vitest'
import { withMine } from './Reactions'

describe('withMine (a reaction shown before WhatsApp confirms it)', () => {
  it('adds, moves and takes back this number\'s reaction, keeping the others', () => {
    const rs = [{ emoji: '👍', count: 2 }, { emoji: '❤️', count: 1, mine: true }]
    expect(withMine(rs, '👍')).toEqual([{ emoji: '👍', count: 3, mine: true }])
    expect(withMine(rs, '')).toEqual([{ emoji: '👍', count: 2 }])
    expect(withMine(rs, '😂')).toEqual([{ emoji: '👍', count: 2 }, { emoji: '😂', count: 1, mine: true }])
    expect(withMine(undefined, '🙏')).toEqual([{ emoji: '🙏', count: 1, mine: true }])
  })
})
