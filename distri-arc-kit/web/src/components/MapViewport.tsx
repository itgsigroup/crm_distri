import { useEffect, useRef, useState, type MouseEvent, type PointerEvent } from 'react'

const MIN_SCALE = 0.2
const MAX_SCALE = 2

const clampPan = (value: number, extent: number) => Math.max(-extent * 3, Math.min(extent * 3, value))

export function useMapViewport(width: number, height: number) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [view, setView] = useState({ x: 0, y: 0, scale: 1 })
  const [dragging, setDragging] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const drag = useRef<{ id: number; x: number; y: number; viewX: number; viewY: number; pixelsPerUnit: number; moved: boolean; captured: boolean } | null>(null)
  const suppressClick = useRef(false)

  const zoom = (factor: number) => setView((v) => ({ ...v, scale: Math.max(MIN_SCALE, Math.min(MAX_SCALE, v.scale * factor)) }))

  const reset = () => setView({ x: 0, y: 0, scale: 1 })
  const onPointerDown = (e: PointerEvent<SVGSVGElement>) => {
    if (e.button !== 0) return
    const rect = e.currentTarget.getBoundingClientRect()
    drag.current = {
      id: e.pointerId,
      x: e.clientX,
      y: e.clientY,
      viewX: view.x,
      viewY: view.y,
      pixelsPerUnit: Math.min(rect.width / width, rect.height / height),
      moved: false,
      captured: false,
    }
  }
  const onPointerMove = (e: PointerEvent<SVGSVGElement>) => {
    const start = drag.current
    if (!start || start.id !== e.pointerId) return
    const dx = e.clientX - start.x
    const dy = e.clientY - start.y
    if (Math.abs(dx) + Math.abs(dy) > 3) start.moved = true
    if (!start.moved) return
    if (!start.captured) {
      e.currentTarget.setPointerCapture(e.pointerId)
      start.captured = true
    }
    setDragging(true)
    setView({
      x: clampPan(start.viewX - dx / start.pixelsPerUnit, width),
      y: clampPan(start.viewY - dy / start.pixelsPerUnit, height),
      scale: view.scale,
    })
  }
  const onPointerUp = (e: PointerEvent<SVGSVGElement>) => {
    const start = drag.current
    if (!start || start.id !== e.pointerId) return
    drag.current = null
    setDragging(false)
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId)
    if (start.moved) {
      suppressClick.current = true
      window.setTimeout(() => { suppressClick.current = false }, 0)
    }
  }
  const onClickCapture = (e: MouseEvent<SVGSVGElement>) => {
    if (!suppressClick.current) return
    e.preventDefault()
    e.stopPropagation()
    suppressClick.current = false
  }

  useEffect(() => {
    const sync = () => setFullscreen(document.fullscreenElement === containerRef.current)
    document.addEventListener('fullscreenchange', sync)
    return () => document.removeEventListener('fullscreenchange', sync)
  }, [])

  const toggleFullscreen = () => {
    const el = containerRef.current
    if (!el) return
    if (document.fullscreenElement === el) void document.exitFullscreen().catch(() => {})
    else void el.requestFullscreen().catch(() => {})
  }

  const viewBox = `${view.x} ${view.y} ${width} ${height}`
  const positionBlend = Math.max(0, Math.min(1, (view.scale - MIN_SCALE) / (1 - MIN_SCALE)))
  const controls = (
    <div className="map-controls" role="toolbar" aria-label="Kontrol peta" onPointerDown={(e) => e.stopPropagation()}>
      <button type="button" aria-label="Perkecil titik" title="Perkecil titik" disabled={view.scale <= MIN_SCALE} onClick={() => zoom(1 / 1.25)}>−</button>
      <span aria-live="polite" title="Ukuran titik">Titik {Math.round(view.scale * 100)}%</span>
      <button type="button" aria-label="Perbesar titik" title="Perbesar titik" disabled={view.scale >= MAX_SCALE} onClick={() => zoom(1.25)}>+</button>
      <button type="button" aria-label="Atur ulang peta" title="Atur ulang" onClick={reset}>↺</button>
      <button type="button" className="map-fullscreen" aria-label={fullscreen ? 'Keluar dari layar penuh' : 'Layar penuh'} title={fullscreen ? 'Keluar dari layar penuh' : 'Layar penuh'} onClick={toggleFullscreen}>
        <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
          {fullscreen
            ? <path d="M2 6h4V2M10 2v4h4M14 10h-4v4M6 14v-4H2" />
            : <path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4" />}
        </svg>
        <span>{fullscreen ? 'Keluar layar penuh' : 'Layar penuh'}</span>
      </button>
    </div>
  )

  return {
    containerRef,
    viewBox,
    markerScale: view.scale,
    positionBlend,
    dragging,
    controls,
    svgProps: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel: onPointerUp, onClickCapture },
  }
}
