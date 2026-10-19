import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'
import { ColumnTable, type Column } from './DataTable'

interface Row { id: string; name: string; wait: number }
const rows: Row[] = Array.from({ length: 23 }, (_, i) => ({ id: String(i), name: `Dealer ${String.fromCharCode(65 + i)}`, wait: (i * 7) % 23 }))
const cols: Column<Row>[] = [
  { key: 'name', label: 'Dealer', cell: (r) => r.name, sort: (a, b) => a.name.localeCompare(b.name) },
  { key: 'wait', label: 'Menunggu', cell: (r) => `${r.wait} mnt`, sort: (a, b) => a.wait - b.wait, first: 'desc', right: true },
]
const table = () => <ColumnTable rows={rows} columns={cols} rowKey={(r) => r.id} text={(r) => r.name} noun="chat" placeholder="Cari" empty="kosong" />
const names = () => screen.getAllByRole('row').slice(1).map((r) => r.querySelector('td')!.textContent)

// Pusat kendali cards (19 Oct): every list is a data table — search, sort asc/desc, pages of 10.
test('ColumnTable sorts both ways, searches and pages', () => {
  render(table())
  expect(names()).toHaveLength(10)
  expect(names()[0]).toBe('Dealer A')
  fireEvent.click(screen.getByRole('button', { name: /Dealer/ }))
  expect(names()[0]).toBe('Dealer W') // second click on the same column: Z→A
  fireEvent.click(screen.getByRole('button', { name: /Menunggu/ }))
  expect(names()[0]).toBe(rows.find((r) => r.wait === 22)!.name) // numbers start largest first
  fireEvent.click(screen.getByText('Berikutnya'))
  expect(screen.getByText('Halaman 2 dari 3')).toBeTruthy()
  fireEvent.change(screen.getByLabelText('Cari chat'), { target: { value: 'dealer c' } })
  expect(names()).toEqual(['Dealer C'])
})

test('ColumnTable shows the empty text without rows', () => {
  render(<ColumnTable rows={[] as Row[]} columns={cols} rowKey={(r) => r.id} text={(r) => r.name} noun="chat" placeholder="Cari" empty="kosong" />)
  expect(screen.getByText('kosong')).toBeTruthy()
})
