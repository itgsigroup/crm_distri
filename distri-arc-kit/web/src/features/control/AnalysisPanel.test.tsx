import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { JumpGame } from './AnalysisPanel'

describe('Lompat!', () => {
  it('starts with Space and takes the keyboard', () => {
    render(<JumpGame autoFocus />)
    const game = screen.getByRole('application')
    expect(document.activeElement).toBe(game)
    expect(screen.getByText('Tekan Spasi untuk mulai')).toBeTruthy()
    fireEvent.keyDown(game, { key: ' ' })
    expect(screen.queryByText('Tekan Spasi untuk mulai')).toBeNull()
  })
})
