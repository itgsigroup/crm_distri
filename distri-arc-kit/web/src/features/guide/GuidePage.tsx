import { useEffect, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router'
import html from './panduan.html?raw'
import { ROUTES } from '../../app/Shell'
import type { ScreenKey } from '../../lib/i18n/id'

/**
 * Panduan Orbit: the mockup's "Konsep orbit" screen (same content as reference/panduan-orbit.html, SVG inline),
 * kept verbatim so it stays identical; its buttons become app navigation.
 */
export function GuidePage() {
  const ref = useRef<HTMLDivElement>(null)
  const nav = useNavigate()
  const { hash } = useLocation()

  useEffect(() => {
    if (!hash) return
    const t = setTimeout(() => document.getElementById(hash.slice(1))?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 50)
    return () => clearTimeout(t)
  }, [hash])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const onClick = (e: MouseEvent) => {
      const b = (e.target as HTMLElement).closest<HTMLElement>('[data-nav],[data-scroll],[data-go]')
      if (!b) return
      e.preventDefault()
      if (b.dataset.scroll) {
        document.getElementById(b.dataset.scroll)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      } else if (b.dataset.nav) {
        nav(ROUTES[b.dataset.nav as ScreenKey] ?? '/')
      } else if (b.dataset.go?.startsWith('dealer:')) {
        nav('/dealer/' + b.dataset.go.slice(7))
      }
    }
    el.addEventListener('click', onClick)
    return () => el.removeEventListener('click', onClick)
  }, [nav])

  return <div ref={ref} dangerouslySetInnerHTML={{ __html: html }} />
}
