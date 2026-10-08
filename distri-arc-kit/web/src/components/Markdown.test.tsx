import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Markdown } from './Markdown'

describe('Markdown (analysis reports)', () => {
  it('renders headings, lists, tables and inline marks', () => {
    const { container } = render(<Markdown text={'## Judul\n\n> catatan\n\n### Kondisi\n- **Dealer**: 18\n- DSO `30`\n\n| Cabang | Omzet |\n|---|---:|\n| Jakarta | Rp1 M |\n\n1. satu\n2. dua'} />)
    expect(container.querySelector('h3')?.textContent).toBe('Judul')
    expect(container.querySelector('blockquote')?.textContent).toBe('catatan')
    expect(container.querySelectorAll('ul li')).toHaveLength(2)
    expect(container.querySelector('ul b')?.textContent).toBe('Dealer')
    expect(container.querySelector('td:last-child')?.getAttribute('style')).toContain('right')
    expect(container.querySelectorAll('ol li')).toHaveLength(2)
  })
  it('never renders model output as HTML', () => {
    const { container } = render(<Markdown text={'<img src=x onerror=alert(1)> **<script>x</script>**'} />)
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(container.textContent).toContain('<img src=x')
  })
})
