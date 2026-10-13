import { describe, expect, it } from 'vitest'
import { countFilters, filterOf, fmtPhone, inFilter, lookupLinks, matchThread, waitLabel } from './inbox'

const W = '2026-10-09T10:00:00Z'

describe('chat inbox', () => {
  it('splits groups into internal and external', () => {
    expect(filterOf({ kind: 'group', group_kind: 'external' })).toBe('group_external')
    expect(filterOf({ kind: 'group', group_kind: 'internal' })).toBe('group_internal')
    expect(filterOf({ kind: 'group' })).toBe('group_internal')
    expect(filterOf({ kind: 'new' })).toBe('new')
    expect(countFilters([{ kind: 'dealer', unread: 0, waiting_since: W }, { kind: 'dealer', unread: 2 }, { kind: 'group', group_kind: 'external', unread: 1, waiting_since: W }, { kind: 'new', unread: 0, waiting_since: W }]))
      .toEqual({ all: 4, unread: 2, unreplied: 2, dealer: 2, group_internal: 0, group_external: 1, new: 1 })
  })

  it('filters unread conversations of every kind', () => {
    expect(inFilter({ kind: 'group', group_kind: 'internal', unread: 3 }, 'unread')).toBe(true)
    expect(inFilter({ kind: 'dealer', unread: 0 }, 'unread')).toBe(false)
    expect(inFilter({ kind: 'dealer', unread: 0 }, 'all')).toBe(true)
    expect(inFilter({ kind: 'new', unread: 1 }, 'dealer')).toBe(false)
  })

  it('filters conversations waiting for our reply, not groups', () => {
    expect(inFilter({ kind: 'dealer', unread: 0, waiting_since: W }, 'unreplied')).toBe(true)
    expect(inFilter({ kind: 'new', unread: 0, waiting_since: W }, 'unreplied')).toBe(true)
    expect(inFilter({ kind: 'dealer', unread: 3, waiting_since: null }, 'unreplied')).toBe(false)
    expect(inFilter({ kind: 'group', group_kind: 'external', unread: 0, waiting_since: W }, 'unreplied')).toBe(false)
  })

  it('searches name, subtitle and last message', () => {
    const t = { title: 'Toko Sinar', subtitle: 'Semarang', last_body: 'minta harga DVR' }
    expect(matchThread(t, 'dvr')).toBe(true)
    expect(matchThread(t, 'SINAR')).toBe(true)
    expect(matchThread(t, 'jakarta')).toBe(false)
    expect(matchThread(t, '  ')).toBe(true)
  })

  it('counts the wait for a reply', () => {
    const now = new Date('2026-10-09T12:00:00Z')
    expect(waitLabel(null, now)).toBeNull()
    expect(waitLabel('2026-10-09T12:05:00Z', now)).toBeNull()
    expect(waitLabel('2026-10-09T11:48:00Z', now)).toEqual({ text: '12 mnt', tone: 'good' })
    expect(waitLabel('2026-10-08T14:16:00Z', now)).toEqual({ text: '21 jam 44 mnt', tone: 'warn' })
    expect(waitLabel('2026-10-06T10:00:00Z', now)).toEqual({ text: '3 hr 2 jam', tone: 'bad' })
  })

  it('formats numbers and lookup links', () => {
    expect(fmtPhone('6281234504471')).toBe('+62 812-3450-4471')
    expect(fmtPhone('')).toBe('')
    expect(lookupLinks('Shinta', '6289536767').map((l) => l.key)).toEqual(['google', 'maps', 'truecaller', 'wa'])
    expect(lookupLinks('Grup Gudang', undefined).map((l) => l.key)).toEqual(['google', 'maps'])
    expect(lookupLinks('Shinta', '6289536767')[3].href).toBe('https://wa.me/6289536767')
  })
})
