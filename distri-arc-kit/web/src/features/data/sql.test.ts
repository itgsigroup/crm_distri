import { describe, expect, it } from 'vitest'
import { buildSQL, missingRequired } from './sql'

const contract = [{ name: 'number', required: true, desc: '' }, { name: 'customer_code', required: true, desc: '' }, { name: 'total', required: true, desc: '' }, { name: 'residual', required: false, desc: '' }]

describe('buildSQL', () => {
  it('matches the Go builder', () => {
    expect(buildSQL('gsi-data', 'invoices', contract, 'accurate.faktur', { number: 'nomor_faktur', customer_code: 'kode_pelanggan', total: 'total_faktur', residual: 'IFNULL(sisa, 0)' }, 'tanggal >= "2024-01-01"'))
      .toBe('SELECT CAST(`nomor_faktur` AS STRING) AS number,\n       CAST(`kode_pelanggan` AS STRING) AS customer_code,\n       `total_faktur` AS total,\n       IFNULL(sisa, 0) AS residual\nFROM `gsi-data.accurate.faktur`\nWHERE tanggal >= "2024-01-01"')
  })
  it('lists required columns without a source', () => {
    expect(missingRequired(contract, { number: 'x', total: ' ' })).toEqual(['customer_code', 'total'])
  })
})
