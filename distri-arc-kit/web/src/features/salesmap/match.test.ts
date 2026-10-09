import { describe, expect, it } from 'vitest'
import { bestTarget, fullKey, nameKey, suggestions, type SalesProfile } from './match'

const p = (id: string, name: string, extra: Partial<SalesProfile> = {}): SalesProfile => ({
  id, name, branch: 'Semarang', source_system: 'import', source_id: id, active: true, merged_into: null, wa_number: null,
  dealers: 0, login_email: null, merged_count: 0, ...extra,
})

describe('mapping sales', () => {
  it('spells names alike', () => {
    expect(nameKey('Granike Monica M.')).toBe('granike')
    expect(fullKey('Granike Monica')).toBe(fullKey('Granike Monika'))
    expect(nameKey('Lucy, Enny Wulandari')).toBe('luky')
    expect(nameKey('  ')).toBe('')
  })

  it('suggests spellings of the same person, never merged ones', () => {
    const rows = [p('1', 'Granike Monica'), p('2', 'Granike Monica M.'), p('3', 'Granike Monika', { source_system: null }), p('4', 'Elisa'), p('5', 'Dewi'), p('6', 'Granike X', { merged_into: '3' })]
    const s = suggestions(rows)
    expect(s.get('1')?.map((x) => x.id)).toEqual(['2', '3'])
    expect(s.has('4')).toBe(false)
    expect(s.has('6')).toBe(false)
  })

  it('prefers the GSI Orbit profile, then a login, then most dealers', () => {
    expect(bestTarget([p('1', 'A', { dealers: 300 }), p('2', 'B', { source_system: null })])?.id).toBe('2')
    expect(bestTarget([p('1', 'A', { dealers: 300 }), p('2', 'B', { login_email: 'b@x' })])?.id).toBe('2')
    expect(bestTarget([p('1', 'A', { dealers: 3 }), p('2', 'B', { dealers: 300 })])?.id).toBe('2')
  })
})
