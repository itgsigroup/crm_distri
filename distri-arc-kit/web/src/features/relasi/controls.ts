// Pointer / wheel / touchpad controls shared by the relasi views (network and galaxy): drag rotates, Shift- or
// right-drag and two fingers pan, wheel / touchpad pinch / two-finger pinch zoom, click focuses, double-click opens.

export interface ControlTarget {
  rotate(dx: number, dy: number): void
  pan(dx: number, dy: number): void
  /** with a pointer position, zoom towards that point */
  zoomBy(factor: number, clientX?: number, clientY?: number): void
  click(e: PointerEvent): void
  dblclick(e: MouseEvent): void
  hover(e: PointerEvent): void
  leave(): void
  /** auto-rotation off while the user handles the view, back on a few seconds later */
  setAuto(on: boolean): void
}

export function bindControls(cv: HTMLElement, t: ControlTarget, reduced: boolean): () => void {
  const pts = new Map<number, { x: number; y: number }>()
  let drag: { x: number; y: number; moved: boolean; pan: boolean } | null = null
  let pinch: { dist: number; mx: number; my: number } | null = null
  let resume: ReturnType<typeof setTimeout> | undefined
  const pause = () => {
    t.setAuto(false)
    clearTimeout(resume)
  }
  const later = () => {
    clearTimeout(resume)
    resume = setTimeout(() => {
      if (!drag && !pinch && !reduced) t.setAuto(true)
    }, 4000)
  }
  const startPinch = () => {
    const [a, b] = [...pts.values()]
    pinch = { dist: Math.hypot(a.x - b.x, a.y - b.y) || 1, mx: (a.x + b.x) / 2, my: (a.y + b.y) / 2 }
  }
  const down = (e: PointerEvent) => {
    pts.set(e.pointerId, { x: e.clientX, y: e.clientY })
    pause()
    cv.setPointerCapture?.(e.pointerId)
    if (pts.size === 2) {
      drag = null
      startPinch()
      t.leave()
      return
    }
    if (pts.size === 1) drag = { x: e.clientX, y: e.clientY, moved: false, pan: e.shiftKey || e.button === 1 || e.button === 2 }
  }
  const move = (e: PointerEvent) => {
    if (pts.has(e.pointerId)) pts.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (pinch && pts.size >= 2) {
      const [a, b] = [...pts.values()]
      const d = Math.hypot(a.x - b.x, a.y - b.y) || 1
      const mx = (a.x + b.x) / 2
      const my = (a.y + b.y) / 2
      t.zoomBy(d / pinch.dist)
      t.pan(mx - pinch.mx, my - pinch.my)
      pinch = { dist: d, mx, my }
      return
    }
    if (drag) {
      const dx = e.clientX - drag.x
      const dy = e.clientY - drag.y
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      if (drag.pan) t.pan(dx, dy)
      else t.rotate(dx, dy)
      drag.x = e.clientX
      drag.y = e.clientY
    } else if (!pts.size) t.hover(e)
  }
  const up = (e: PointerEvent) => {
    if (!pts.delete(e.pointerId)) return
    if (pinch) {
      if (pts.size < 2) pinch = null
      if (pts.size === 1) { // the finger that stays rotates on; lifting it is not a click
        const [p] = [...pts.values()]
        drag = { x: p.x, y: p.y, moved: true, pan: false }
      }
      later()
      return
    }
    if (drag && !drag.moved && !drag.pan) t.click(e)
    if (!pts.size) drag = null
    later()
  }
  const dbl = (e: MouseEvent) => t.dblclick(e)
  const leave = () => t.leave()
  const wheel = (e: WheelEvent) => {
    e.preventDefault()
    const dy = e.deltaMode === 1 ? e.deltaY * 16 : e.deltaMode === 2 ? e.deltaY * 400 : e.deltaY
    t.zoomBy(Math.exp(-dy * (e.ctrlKey ? 0.01 : 0.0015)), e.clientX, e.clientY) // ctrl+wheel = touchpad pinch (Chrome, Edge, Firefox)
  }
  // Safari (macOS) reports a touchpad pinch as gesture* events with a cumulative scale
  let gScale = 1
  const gStart = (e: Event) => { e.preventDefault(); gScale = 1 }
  const gChange = (e: Event) => {
    e.preventDefault()
    const g = e as Event & { scale: number; clientX: number; clientY: number }
    if (!g.scale) return
    t.zoomBy(g.scale / gScale, g.clientX, g.clientY)
    gScale = g.scale
  }
  const menu = (e: MouseEvent) => e.preventDefault() // right-drag pans
  const on: [string, EventListener, AddEventListenerOptions?][] = [
    ['pointerdown', down as EventListener], ['pointermove', move as EventListener], ['pointerup', up as EventListener],
    ['pointercancel', up as EventListener], ['dblclick', dbl as EventListener], ['pointerleave', leave],
    ['wheel', wheel as EventListener, { passive: false }], ['gesturestart', gStart, { passive: false }],
    ['gesturechange', gChange, { passive: false }], ['contextmenu', menu as EventListener],
  ]
  for (const [ev, fn, opt] of on) cv.addEventListener(ev, fn, opt)
  return () => {
    for (const [ev, fn] of on) cv.removeEventListener(ev, fn)
    clearTimeout(resume)
  }
}
