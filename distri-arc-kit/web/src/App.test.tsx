import { render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import App from './App'

afterEach(() => vi.restoreAllMocks())

test('renders the brand', () => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(JSON.stringify({ db: 'ok', queue: 'ok' })))))
  render(<App />)
  expect(screen.getByText(/Distri ARC Orbit/)).toBeTruthy()
})
