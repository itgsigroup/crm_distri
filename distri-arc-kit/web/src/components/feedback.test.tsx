import { act, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { expect, test } from 'vitest'
import { FeedbackProvider, useFeedback } from './feedback'

function Counter() {
  const [n, setN] = useState(0)
  return <button onClick={() => setN(n + 1)}>klik {n}</button>
}

function Opener() {
  const { openSheet, closeSheet } = useFeedback()
  return <><button onClick={() => openSheet(<Counter />)}>buka</button><button onClick={closeSheet}>tutup</button></>
}

// Regression: Chat → + Nomor opened a second time showed the first number's finished link (the sheet kept its state).
test('every openSheet mounts a fresh sheet, even with the same component', () => {
  render(<FeedbackProvider><Opener /></FeedbackProvider>)
  fireEvent.click(screen.getByText('buka'))
  fireEvent.click(screen.getByText('klik 0'))
  expect(screen.getByText('klik 1')).toBeTruthy()
  fireEvent.click(screen.getByText('tutup'))
  act(() => { fireEvent.click(screen.getByText('buka')) })
  expect(screen.getByText('klik 0')).toBeTruthy()
})
