import { describe, expect, it } from 'vitest'
import type { Sales } from '../api/types'
import { splitSales } from './SalesPicker'

const s = (name: string, dealers: number, user?: string): Sales => ({ id: name, key: name.toLowerCase(), name, branch: 'Semarang', initials: name[0], wa_number: '', dealers, user_id: user, user_name: user ? name : undefined })

describe('filter sales', () => {
  it('shows the Pengguna sales as chips and unlinked BigQuery names in the list', () => {
    const { chips, rest } = splitSales([s('Elisa', 335), s('Granike Monika', 171, 'u1'), s('Andi', 6, 'u2'), s('Aini', 158)])
    expect(chips.map((x) => x.name)).toEqual(['Andi', 'Granike Monika'])
    expect(rest.map((x) => x.name)).toEqual(['Aini', 'Elisa'])
  })

  it('before any mapping: the 10 sales with most dealers as chips', () => {
    const list = Array.from({ length: 14 }, (_, i) => s(`S${String(i).padStart(2, '0')}`, i))
    const { chips, rest } = splitSales(list)
    expect(chips).toHaveLength(10)
    expect(chips[0].name).toBe('S13')
    expect(rest.map((x) => x.name)).toEqual(['S00', 'S01', 'S02', 'S03'])
  })
})
