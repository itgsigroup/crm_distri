import { describe, expect, it } from 'vitest'
import { LIST_MAX, LIST_MIN, clampWidth, listWidthStyle } from './ListResizer'

describe('chat list width', () => {
  it('stays between the minimum and what leaves the conversation room', () => {
    expect(clampWidth(100, 1600)).toBe(LIST_MIN)
    expect(clampWidth(400.4, 1600)).toBe(400)
    expect(clampWidth(900, 1600)).toBe(LIST_MAX)
    expect(clampWidth(600, 900)).toBe(900 - 420) // the conversation keeps 420px
    expect(clampWidth(600, 500)).toBe(LIST_MIN) // a narrow screen still shows the list
  })
  it('sets --list-w only when dragged', () => {
    expect(listWidthStyle(null)).toBeUndefined()
    expect(listWidthStyle(420)).toEqual({ '--list-w': '420px' })
  })
})
