import { describe, expect, it } from 'vitest'
import { countFilters, filterOf, fmtPhone, lookupLinks, matchThread, waitLabel } from './inbox'

describe('chat inbox', () => {
  it('splits groups into internal and external', () => {
    expect(filterOf({ kind: 'group', group_kind: 'external' })).toBe('group_external')
    expect(filterOf({ kind: 'group', group_kind: 'internal' })).toBe('group_internal')
    expect(filterOf({ kind: 'group' })).toBe('group_internal')
    expect(filterOf({ kind: 'new' })).toBe('new')
    expect(countFilters([{ kind: 'dealer' }, { kind: 'dealer' }, { kind: 'group', group_kind: 'external' }, { kind: 'new' }]))
      .toEqual({ all: 4, dealer: 2, group_internal: 0, group_external: 1, new: 1 })
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
