import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { TapGame } from './AnalysisPanel'

describe('tap tap Enter', () => {
  it('answers Enter and Space with a hit or a miss', () => {
    render(<TapGame autoFocus />)
    const game = screen.getByRole('application')
    expect(document.activeElement).toBe(game)
    expect(screen.getByText('Tap tap Enter')).toBeTruthy()
    fireEvent.keyDown(game, { key: 'Enter' })
    expect(screen.getByText(/Tepat!|Meleset/)).toBeTruthy()
    fireEvent.keyDown(game, { key: ' ' })
    expect(screen.getByText(/Tepat!|Meleset/)).toBeTruthy()
  })
})
