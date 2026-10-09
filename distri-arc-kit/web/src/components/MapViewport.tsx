import { useCallback, useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type MouseEvent, type PointerEvent } from 'react'

// Zoom is the camera (1 = whole map fits, up to 8×); dot size is separate. Markers keep their on-screen size while
// zooming, so zooming in pulls stacked dots apart; shrinking the dots puts every dot back on its true position.
export const MIN_ZOOM = 1
export const MAX_ZOOM = 8
export const MIN_DOT = 0.2
export const MAX_DOT = 2

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

export interface MapView { cx: number; cy: number; zoom: number }

/** The visible part of a width×height map at `zoom`, for a viewport of the given aspect ratio (width / height). */
export function viewRect(view: MapView, width: number, height: number, aspect = width / height) {
  const fitW = aspect >= width / height ? height * aspect : width
  const w = fitW / view.zoom
  const h = w / aspect
  return { x: view.cx - w / 2, y: view.cy - h / 2, w, h }
}

/** Map coordinate under a client point, for an SVG drawn with preserveAspectRatio="xMidYMid meet". */
export function clientToMap(rect: { left: number; top: number; width: number; height: number }, vb: { x: number; y: number; w: number; h: number }, clientX: number, clientY: number) {
  const s = Math.min(rect.width / vb.w, rect.height / vb.h) || 1
  const offX = (rect.width - vb.w * s) / 2
  const offY = (rect.height - vb.h * s) / 2
  return { x: vb.x + (clientX - rect.left - offX) / s, y: vb.y + (clientY - rect.top - offY) / s, s }
}

/** Zoom by `factor` keeping map point (px, py) where it is on screen; the centre stays on the map. */
export function zoomAround(view: MapView, factor: number, px: number, py: number, width: number, height: number): MapView {
  const zoom = clamp(view.zoom * factor, MIN_ZOOM, MAX_ZOOM)
  const k = view.zoom / zoom
  return { zoom, cx: clamp(px - (px - view.cx) * k, 0, width), cy: clamp(py - (py - view.cy) * k, 0, height) }
}

/** How far dots are spread apart from their true position: 0 at the smallest dot size, 1 from normal size up. */
export const spreadOf = (dot: number) => clamp((dot - MIN_DOT) / (1 - MIN_DOT), 0, 1)

/** Marker scale rounded to small steps so a smooth wheel zoom re-runs the collision layout only now and then. */
export const layoutStep = (s: number) => 2 ** (Math.round(Math.log2(s) * 6) / 6)

type Gesture =
  | { kind: 'pan'; id: number; x: number; y: number; cx: number; cy: number; s: number; moved: boolean }
  | { kind: 'pinch'; dist: number; zoom: number; px: number; py: number }

export function useMapViewport(width: number, height: number) {
  const home: MapView = { cx: width / 2, cy: height / 2, zoom: 1 }
  const containerRef = useRef<HTMLDivElement>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const [view, setViewState] = useState<MapView>(home)
  const viewRef = useRef(view)
  const setView = useCallback((v: MapView) => { viewRef.current = v; setViewState(v) }, [])
  const [dot, setDot] = useState(1)
  const [dragging, setDragging] = useState(false)
  const [fullscreen, setFullscreen] = useState(false) // native Fullscreen API
  const [maximized, setMaximized] = useState(false) // CSS fallback (iPhone Safari has no element fullscreen)
  const [aspect, setAspect] = useState(width / height)
  const big = fullscreen || maximized
  const vb = viewRect(view, width, height, big ? aspect : width / height)
  const vbRef = useRef(vb)
  useLayoutEffect(() => { vbRef.current = vb })
  const pointers = useRef(new Map<number, { x: number; y: number }>())
  const gesture = useRef<Gesture | null>(null)
  const suppressClick = useRef(false)

  const toMap = useCallback((clientX: number, clientY: number) => {
    const svg = svgRef.current
    if (!svg) return null
    return clientToMap(svg.getBoundingClientRect(), vbRef.current, clientX, clientY)
  }, [])

  const zoomBy = useCallback((factor: number, clientX?: number, clientY?: number) => {
    const v = viewRef.current
    const p = clientX != null && clientY != null ? toMap(clientX, clientY) : null
    setView(zoomAround(v, factor, p?.x ?? v.cx, p?.y ?? v.cy, width, height))
  }, [toMap, setView, width, height])

  // wheel / trackpad pinch: a native listener, React's onWheel is passive and cannot stop the page from scrolling
  useEffect(() => {
    const svg = svgRef.current
    if (!svg) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const dy = e.deltaMode === 1 ? e.deltaY * 16 : e.deltaMode === 2 ? e.deltaY * 400 : e.deltaY
      zoomBy(Math.exp(-dy * (e.ctrlKey ? 0.01 : 0.0012)), e.clientX, e.clientY)
    }
    // Safari (macOS) reports a touchpad pinch as gesture* events with a cumulative scale instead of ctrl+wheel
    let base = 1
    type SafariGesture = Event & { scale: number; clientX: number; clientY: number }
    const onGestureStart = (e: Event) => { e.preventDefault(); base = 1 }
    const onGestureChange = (e: Event) => {
      e.preventDefault()
      const g = e as SafariGesture
      if (!g.scale) return
      zoomBy(g.scale / base, g.clientX, g.clientY)
      base = g.scale
    }
    svg.addEventListener('wheel', onWheel, { passive: false })
    svg.addEventListener('gesturestart', onGestureStart, { passive: false })
    svg.addEventListener('gesturechange', onGestureChange, { passive: false })
    svg.addEventListener('gestureend', onGestureStart, { passive: false })
    return () => {
      svg.removeEventListener('wheel', onWheel)
      svg.removeEventListener('gesturestart', onGestureStart)
      svg.removeEventListener('gesturechange', onGestureChange)
      svg.removeEventListener('gestureend', onGestureStart)
    }
  }, [zoomBy])

  const startPan = (id: number, x: number, y: number) => {
    const p = toMap(x, y)
    const v = viewRef.current
    gesture.current = { kind: 'pan', id, x, y, cx: v.cx, cy: v.cy, s: p?.s ?? 1, moved: false }
  }
  const startPinch = () => {
    const [a, b] = [...pointers.current.values()]
    const p = toMap((a.x + b.x) / 2, (a.y + b.y) / 2)
    if (!p) return
    gesture.current = { kind: 'pinch', dist: Math.hypot(a.x - b.x, a.y - b.y) || 1, zoom: viewRef.current.zoom, px: p.x, py: p.y }
  }

  const onPointerDown = (e: PointerEvent<SVGSVGElement>) => {
    if (e.pointerType === 'mouse' && e.button !== 0) return
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (pointers.current.size === 1) startPan(e.pointerId, e.clientX, e.clientY)
    else if (pointers.current.size === 2) {
      for (const id of pointers.current.keys()) if (!e.currentTarget.hasPointerCapture(id)) try { e.currentTarget.setPointerCapture(id) } catch { /* pointer already gone */ }
      startPinch()
      setDragging(true)
    }
  }
  const onPointerMove = (e: PointerEvent<SVGSVGElement>) => {
    if (!pointers.current.has(e.pointerId)) return
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    const g = gesture.current
    if (!g) return
    if (g.kind === 'pinch') {
      const [a, b] = [...pointers.current.values()]
      if (!a || !b) return
      const zoom = clamp(g.zoom * (Math.hypot(a.x - b.x, a.y - b.y) / g.dist), MIN_ZOOM, MAX_ZOOM)
      // keep the map point that started under the fingers under their current midpoint
      const v = viewRef.current
      const tmp = { ...v, zoom }
      const r = svgRef.current!.getBoundingClientRect()
      const box = viewRect(tmp, width, height, big ? aspect : width / height)
      const at = clientToMap(r, box, (a.x + b.x) / 2, (a.y + b.y) / 2)
      setView({ zoom, cx: clamp(v.cx + g.px - at.x, 0, width), cy: clamp(v.cy + g.py - at.y, 0, height) })
      return
    }
    if (g.id !== e.pointerId) return
    const dx = e.clientX - g.x
    const dy = e.clientY - g.y
    if (!g.moved && Math.abs(dx) + Math.abs(dy) > 3) {
      g.moved = true
      try { e.currentTarget.setPointerCapture(e.pointerId) } catch { /* pointer already gone */ }
      setDragging(true)
    }
    if (!g.moved) return
    setView({ zoom: viewRef.current.zoom, cx: clamp(g.cx - dx / g.s, 0, width), cy: clamp(g.cy - dy / g.s, 0, height) })
  }
  const onPointerUp = (e: PointerEvent<SVGSVGElement>) => {
    if (!pointers.current.delete(e.pointerId)) return
    const g = gesture.current
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId)
    const moved = g?.kind === 'pinch' || (g?.kind === 'pan' && g.moved)
    if (moved) {
      suppressClick.current = true
      window.setTimeout(() => { suppressClick.current = false }, 0)
    }
    const rest = [...pointers.current.entries()]
    if (rest.length === 1) { // pinch → pan with the finger that stays
      const [id, p] = rest[0]
      startPan(id, p.x, p.y)
      ;(gesture.current as Extract<Gesture, { kind: 'pan' }>).moved = true
      return
    }
    if (rest.length === 0) {
      gesture.current = null
      setDragging(false)
    }
  }
  const onClickCapture = (e: MouseEvent<SVGSVGElement>) => {
    if (!suppressClick.current) return
    e.preventDefault()
    e.stopPropagation()
    suppressClick.current = false
  }
  const onDoubleClick = (e: MouseEvent<SVGSVGElement>) => {
    if ((e.target as Element).closest?.('.dn')) return // a dot: the click already opened the dealer
    zoomBy(e.shiftKey ? 1 / 2 : 2, e.clientX, e.clientY)
  }

  useEffect(() => {
    const sync = () => setFullscreen(!!containerRef.current && document.fullscreenElement === containerRef.current)
    document.addEventListener('fullscreenchange', sync)
    return () => document.removeEventListener('fullscreenchange', sync)
  }, [])

  useEffect(() => {
    if (!maximized) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setMaximized(false) }
    document.addEventListener('keydown', onKey)
    document.body.classList.add('map-maximized')
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.classList.remove('map-maximized')
    }
  }, [maximized])

  // full screen: the map fills the whole screen, so the visible area follows the screen's shape
  useLayoutEffect(() => {
    const svg = svgRef.current
    if (!big || !svg) return
    const measure = () => {
      const r = svg.getBoundingClientRect()
      if (r.width > 0 && r.height > 0) setAspect(r.width / r.height)
    }
    measure()
    const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    ro?.observe(svg)
    window.addEventListener('resize', measure)
    return () => {
      ro?.disconnect()
      window.removeEventListener('resize', measure)
    }
  }, [big])

  const toggleFullscreen = () => {
    const el = containerRef.current
    if (!el) return
    if (maximized) { setMaximized(false); return }
    if (document.fullscreenElement === el) { void document.exitFullscreen().catch(() => {}); return }
    if (typeof el.requestFullscreen !== 'function' || !document.fullscreenEnabled) { setMaximized(true); return }
    el.requestFullscreen().catch(() => setMaximized(true))
  }

  const reset = () => { setView(home); setDot(1) }
  const zoomed = view.zoom > MIN_ZOOM + 1e-6 || view.cx !== home.cx || view.cy !== home.cy
  const stop = (e: { stopPropagation: () => void }) => e.stopPropagation()
  const controls = (
    <div className="map-controls" role="toolbar" aria-label="Kontrol peta" onPointerDown={stop} onDoubleClick={stop}>
      <div className="map-ctl-group" aria-label="Zoom">
        <button type="button" aria-label="Zoom out" title="Zoom out (scroll ke bawah)" disabled={view.zoom <= MIN_ZOOM + 1e-6} onClick={() => zoomBy(1 / 1.5)}>−</button>
        <span aria-live="polite" title="Zoom">{Math.round(view.zoom * 100)}%</span>
        <button type="button" aria-label="Zoom in" title="Zoom in (scroll ke atas · klik ganda)" disabled={view.zoom >= MAX_ZOOM - 1e-6} onClick={() => zoomBy(1.5)}>+</button>
      </div>
      <div className="map-ctl-group" aria-label="Ukuran titik">
        <span className="map-ctl-l" title="Ukuran titik — kecilkan untuk melihat posisi asli tiap titik">Titik</span>
        <button type="button" aria-label="Kecilkan titik" title="Kecilkan titik · titik kembali ke posisi aslinya" disabled={dot <= MIN_DOT + 1e-6} onClick={() => setDot((d) => clamp(d / 1.4, MIN_DOT, MAX_DOT))}>
          <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><circle cx="8" cy="8" r="2.5" /></svg>
        </button>
        <button type="button" aria-label="Besarkan titik" title="Besarkan titik" disabled={dot >= MAX_DOT - 1e-6} onClick={() => setDot((d) => clamp(d * 1.4, MIN_DOT, MAX_DOT))}>
          <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><circle cx="8" cy="8" r="5.5" /></svg>
        </button>
      </div>
      <button type="button" aria-label="Atur ulang peta" title="Atur ulang: seluruh peta, titik normal" disabled={!zoomed && dot === 1} onClick={reset}>↺</button>
      <button type="button" className="map-fullscreen" aria-label={big ? 'Keluar dari layar penuh' : 'Layar penuh'} title={big ? 'Keluar dari layar penuh (Esc)' : 'Layar penuh'} onClick={toggleFullscreen}>
        <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
          {big
            ? <path d="M2 6h4V2M10 2v4h4M14 10h-4v4M6 14v-4H2" />
            : <path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4" />}
        </svg>
        <span>{big ? 'Keluar' : 'Layar penuh'}</span>
      </button>
    </div>
  )

  return {
    containerRef,
    /** class names for the map container */
    className: `map-viewport${dragging ? ' is-panning' : ''}${maximized ? ' is-max' : ''}${big ? ' is-full' : ''}`,
    viewBox: `${vb.x.toFixed(2)} ${vb.y.toFixed(2)} ${vb.w.toFixed(2)} ${vb.h.toFixed(2)}`,
    zoom: view.zoom,
    /** one screen pixel at fit size, in map units at the current zoom (for strokes and hit areas) */
    unit: 1 / view.zoom,
    /** dot size in map units relative to the base size: constant on screen while zooming */
    markerScale: dot / view.zoom,
    /** 0 = every dot on its true position, 1 = dots spread apart so none overlap */
    positionBlend: spreadOf(dot),
    dragging,
    controls,
    // --mu keeps labels at their normal size on screen while zooming (see .map-viewport svg text)
    svgProps: { ref: svgRef, preserveAspectRatio: 'xMidYMid meet', style: { '--mu': 1 / view.zoom } as CSSProperties, onPointerDown, onPointerMove, onPointerUp, onPointerCancel: onPointerUp, onClickCapture, onDoubleClick },
  }
}
