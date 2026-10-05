import { expect, test } from 'vitest'
import type { BoardItem } from '../api/types'
import { parseCommand } from './command'

const dealers = [{ id: 'mitra', name: 'CV Mitra Jaya Teknik' }, { id: 'sinar', name: 'Toko Sinar Elektronik' }] as BoardItem[]

test('command bar parser', () => {
  expect(parseCommand('analisis ulang Mitra Jaya', dealers)).toEqual({ kind: 'reanalyze', scope: 'dealer:mitra', via: 'auto' })
  expect(parseCommand('analisis ulang stok lewat mcp', dealers)).toEqual({ kind: 'reanalyze', scope: 'screen:stock', via: 'mcp' })
  expect(parseCommand('hitung ulang semua', dealers)).toEqual({ kind: 'reanalyze', scope: 'all', via: 'auto' })
  expect(parseCommand('sinar', dealers)).toEqual({ kind: 'dealer', id: 'sinar', name: 'Toko Sinar Elektronik' })
  expect(parseCommand('dealer mana yang berisiko?', dealers)).toEqual({ kind: 'ask', q: 'dealer mana yang berisiko?' })
})
