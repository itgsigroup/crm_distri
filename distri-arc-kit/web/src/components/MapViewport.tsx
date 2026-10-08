import { useEffect, useRef, useState, type MouseEvent, type PointerEvent } from 'react'

const MIN_SCALE = 1
const MAX_SCALE = 8

export function useMapViewport(width: number, height: number) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [view, setView] = useState({ x: 0, y: 0, scale: 1 })
  const [dragging, setDragging] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const drag = useRef<{ id: number; x: number; y: number; viewX: number; viewY: number; viewWidth: number; viewHeight: number; rectWidth: number; rectHeight: number; moved: boolean; captured: boolean } | null>(null)
  const suppressClick = useRef(false)

  const zoom = (factor: number) => setView((v) => {
    const scale = Math.max(MIN_SCALE, Math.min(MAX_SCALE, v.scale * factor))
    const nextWidth = width / scale
    const nextHeight = height / scale
    const centerX = v.x + width / v.scale / 2
    const centerY = v.y + height / v.scale / 2
    return {
      scale,
      x: Math.max(0, Math.min(width - nextWidth, centerX - nextWidth / 2)),
      y: Math.max(0, Math.min(height - nextHeight, centerY - nextHeight / 2)),
    }
  })

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
      viewWidth: width / view.scale,
      viewHeight: height / view.scale,
      rectWidth: rect.width,
      rectHeight: rect.height,
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
      x: Math.max(0, Math.min(width - start.viewWidth, start.viewX - dx * start.viewWidth / start.rectWidth)),
      y: Math.max(0, Math.min(height - start.viewHeight, start.viewY - dy * start.viewHeight / start.rectHeight)),
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

  const viewBox = `${view.x} ${view.y} ${width / view.scale} ${height / view.scale}`
  const controls = (
    <div className="map-controls" role="toolbar" aria-label="Kontrol peta" onPointerDown={(e) => e.stopPropagation()}>
      <button type="button" aria-label="Perkecil peta" title="Perkecil" disabled={view.scale <= MIN_SCALE} onClick={() => zoom(1 / 1.25)}>−</button>
      <span aria-live="polite">{Math.round(view.scale * 100)}%</span>
      <button type="button" aria-label="Perbesar peta" title="Perbesar" disabled={view.scale >= MAX_SCALE} onClick={() => zoom(1.25)}>+</button>
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
    dragging,
    controls,
    svgProps: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel: onPointerUp, onClickCapture },
  }
}
