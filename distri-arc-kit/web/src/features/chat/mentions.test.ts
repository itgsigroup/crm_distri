import { describe, expect, it } from 'vitest'
import { insertMention, matchMembers, mentionParts, mentionQuery, toWire } from './mentions'

describe('chat tags', () => {
  it('shows tags by name and leaves other @digits as written', () => {
    const ms = [{ token: '6281234504471', number: '6281234504471', name: 'Dewi' }, { token: '77889', name: '' }]
    expect(mentionParts('@6281234504471 cek stok, @77889 juga @123456 ya', ms)).toEqual([
      { t: '@Dewi', tag: true }, { t: ' cek stok, ' }, { t: '@77889', tag: true }, { t: ' juga @123456 ya' },
    ])
    expect(mentionParts('tanpa tag', ms)).toEqual([{ t: 'tanpa tag' }])
    expect(mentionParts('@6281234504471', null)).toEqual([{ t: '@6281234504471' }])
  })
  it('finds the @ being typed and puts the picked member in its place', () => {
    expect(mentionQuery('halo @de', 8)).toEqual({ start: 5, q: 'de' })
    expect(mentionQuery('@', 1)).toEqual({ start: 0, q: '' })
    expect(mentionQuery('email a@b', 9)).toBeNull()
    expect(mentionQuery('halo @dewi cek', 14)).toBeNull()
    const r = insertMention('halo @de cek', { start: 5, q: 'de' }, { number: '628123', name: 'Dewi Sales' })
    expect(r).toEqual({ text: 'halo @Dewi Sales  cek', caret: 17 })
  })
  it('offers matching members and sends their numbers', () => {
    const ms = [{ number: '628111', name: 'Budi' }, { number: '628222', name: 'Budiman' }, { number: '628333', name: 'Dewi' }]
    expect(matchMembers(ms, 'bud').map((m) => m.name)).toEqual(['Budi', 'Budiman'])
    expect(matchMembers(ms, '333').map((m) => m.name)).toEqual(['Dewi'])
    expect(toWire('@Budiman dan @Budi cek', [ms[0], ms[1]])).toBe('@628222 dan @628111 cek')
  })
})
