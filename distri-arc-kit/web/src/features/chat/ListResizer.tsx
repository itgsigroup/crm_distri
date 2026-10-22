import { useEffect, useRef, useState, type CSSProperties, type RefObject } from 'react'

// The chat list widens like a VS Code panel (Sam 22 Oct): near the line between the list and the conversation the
// cursor turns ↔, a drag sets the width, a double-click goes back to the default; the width is kept per browser.

const KEY = 'chat.list-w'
export const LIST_MIN = 240
export const LIST_MAX = 640
/** the conversation keeps at least this much room */
const THREAD_MIN = 420

/** The width a drag asks for, kept between the minimum and what leaves the conversation its room. */
export function clampWidth(w: number, container: number): number {
  const max = Math.max(LIST_MIN, Math.min(LIST_MAX, container - THREAD_MIN))
  return Math.round(Math.min(max, Math.max(LIST_MIN, w)))
}

export function savedListWidth(): number | null {
  try {
    const v = Number(localStorage.getItem(KEY))
    return v >= LIST_MIN && v <= LIST_MAX ? v : null
  } catch {
    return null
  }
}

function save(w: number | null) {
  try {
    if (w == null) localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, String(w))
  } catch { /* not remembered */ }
}

/** The style that sets the list's width on the chat grid (--list-w), none for the default. */
export const listWidthStyle = (w: number | null): CSSProperties | undefined => (w ? ({ '--list-w': `${w}px` } as CSSProperties) : undefined)

/** The handle in the gap right of the list pane: drag, double-click, or ← → when focused. */
export function ListResizer({ box, width, onWidth }: { box: RefObject<HTMLDivElement | null>; width: number | null; onWidth: (w: number | null) => void }) {
  const [pos, setPos] = useState<{ left: number; top: number; height: number } | null>(null)
  const [dragging, setDragging] = useState(false)
  const start = useRef<{ x: number; w: number } | null>(null)
  const pane = () => box.current?.querySelector<HTMLElement>(':scope > .list-pane') ?? null

  // follow the list pane's right edge: its size, the window, the panels around it — observers set once, the position
  // updated only when it moved (no render loop). A passive effect: the parent's ref is set only after this child's
  // layout effects, so a layout effect would find no box on the first mount
  useEffect(() => {
    const el = box.current
    if (!el) return
    const place = () => {
      const p = el.querySelector<HTMLElement>(':scope > .list-pane')
      if (!p || p.offsetParent === null) { setPos((o) => (o ? null : o)); return }
      const c = el.getBoundingClientRect()
      const r = p.getBoundingClientRect()
      const next = { left: Math.round(r.right - c.left), top: Math.round(r.top - c.top), height: Math.round(r.height) }
      setPos((o) => (o && o.left === next.left && o.top === next.top && o.height === next.height ? o : next))
    }
    place()
    const ro = new ResizeObserver(place)
    ro.observe(el)
    const p = el.querySelector(':scope > .list-pane')
    if (p) ro.observe(p)
    const mo = new MutationObserver(place) // the list pane shown or hidden (phone layout, an empty chat page)
    mo.observe(el, { childList: true, attributes: true, attributeFilter: ['class', 'style'] })
    window.addEventListener('resize', place)
    return () => { ro.disconnect(); mo.disconnect(); window.removeEventListener('resize', place) }
  }, [box])

  useEffect(() => {
    if (!dragging) return
    document.body.classList.add('col-resizing')
    return () => document.body.classList.remove('col-resizing')
  }, [dragging])

  if (!pos) return null
  const containerW = () => box.current?.getBoundingClientRect().width ?? 1200
  const set = (w: number) => onWidth(clampWidth(w, containerW()))
  return (
    <div
      className={`list-resizer${dragging ? ' on' : ''}`}
      style={{ left: pos.left, top: pos.top, height: pos.height }}
      role="separator"
      aria-orientation="vertical"
      aria-label="Ubah lebar daftar chat"
      aria-valuemin={LIST_MIN}
      aria-valuemax={LIST_MAX}
      aria-valuenow={width ?? Math.round(pane()?.getBoundingClientRect().width ?? 0)}
      tabIndex={0}
      title="Tarik untuk mengubah lebar daftar chat · klik dua kali: lebar awal"
      onPointerDown={(e) => {
        const p = pane()
        if (!p || e.button !== 0) return
        e.preventDefault()
        e.currentTarget.setPointerCapture(e.pointerId)
        start.current = { x: e.clientX, w: p.getBoundingClientRect().width }
        setDragging(true)
      }}
      onPointerMove={(e) => {
        if (!start.current) return
        set(start.current.w + e.clientX - start.current.x)
      }}
      onPointerUp={(e) => {
        if (!start.current) return
        e.currentTarget.releasePointerCapture(e.pointerId)
        start.current = null
        setDragging(false)
        save(clampWidth(Math.round(pane()?.getBoundingClientRect().width ?? LIST_MIN), containerW()))
      }}
      onDoubleClick={() => { onWidth(null); save(null) }}
      onKeyDown={(e) => {
        if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
        e.preventDefault()
        const now = pane()?.getBoundingClientRect().width ?? LIST_MIN
        const w = clampWidth(now + (e.key === 'ArrowRight' ? 16 : -16), containerW())
        onWidth(w)
        save(w)
      }}
    />
  )
}
