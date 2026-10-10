import { describe, expect, it } from 'vitest'
import { candidates, looksLike, nameKey, namesByUser, unlinked, type MapUser, type SalesProfile } from './match'

const p = (id: string, name: string, extra: Partial<SalesProfile> = {}): SalesProfile => ({
  id, name, branch: 'Semarang', source_system: 'import', source_id: id, active: true, merged_into: null, wa_number: null,
  dealers: 0, login_email: null, merged_count: 0, user_id: null, main: false, ...extra,
})
const user: MapUser = { id: 'u1', name: 'Granike Monika', email: 'g@x', role_name: 'Sales', role: 'sales', sales_user_id: 'm', branch: 'Semarang' }

describe('mapping sales', () => {
  it('spells names alike', () => {
    expect(nameKey('Granike Monica M.')).toBe('granike')
    expect(looksLike('Granike Monika', 'Granike Monica')).toBe(true)
    expect(looksLike('Granike Monika', 'Elisa')).toBe(false)
    expect(looksLike('Al', 'Al Fatah')).toBe(false)
  })

  it('groups names per user, main profile first', () => {
    const rows = [p('2', 'Granike Monica M.', { user_id: 'u1', merged_into: 'm' }), p('m', 'Granike Monika', { user_id: 'u1', main: true, source_system: null }), p('3', 'Elisa')]
    expect(namesByUser(rows).get('u1')?.map((x) => x.id)).toEqual(['m', '2'])
    expect(unlinked(rows).map((x) => x.id)).toEqual(['3'])
  })

  it('lists unlinked names that look like the user first', () => {
    const rows = [p('1', 'Aini', { dealers: 158 }), p('2', 'Granike Monica', { dealers: 0 }), p('3', 'Granike Monica M.', { dealers: 171 }), p('4', 'Lucy', { merged_into: '1' })]
    expect(candidates(rows, user).map((x) => x.id)).toEqual(['3', '2', '1'])
  })
})
