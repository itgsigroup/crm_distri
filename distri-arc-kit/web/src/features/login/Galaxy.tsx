import { useEffect, useRef } from 'react'

// Animated "orbit galaxy" behind the login: a starfield with nebulae drawn once per resize, and tilted elliptical
// orbits whose planets (dealers) circle a glowing core. Canvas 2D, ~60 fps, a single frame with reduced motion.

interface Star { x: number; y: number; r: number; a: number; tw: number; ph: number }
interface Planet { ring: number; angle: number; speed: number; size: number; color: string }

const COLORS = ['#FF9F5A', '#F45D9C', '#7B61FF', '#34C759', '#2E8CFF', '#FFD166']

export function Galaxy() {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const bg = document.createElement('canvas')
    let w = 0, h = 0, dpr = 1, raf = 0
    let twinkle: Star[] = []
    let planets: Planet[] = []
    const rings = 7

    const layout = () => {
      dpr = Math.min(window.devicePixelRatio || 1, 2)
      w = canvas.clientWidth
      h = canvas.clientHeight
      for (const c of [canvas, bg]) { c.width = w * dpr; c.height = h * dpr }
      const b = bg.getContext('2d')!
      b.setTransform(dpr, 0, 0, dpr, 0, 0)
      b.fillStyle = '#04050D'
      b.fillRect(0, 0, w, h)
      // nebulae
      for (const [x, y, r, c] of [[0.15, 0.2, 0.55, 'rgba(123,97,255,.20)'], [0.85, 0.85, 0.6, 'rgba(244,93,156,.16)'], [0.65, 0.35, 0.45, 'rgba(46,140,255,.14)'], [0.3, 0.9, 0.4, 'rgba(255,159,90,.10)']] as [number, number, number, string][]) {
        const g = b.createRadialGradient(x * w, y * h, 0, x * w, y * h, r * Math.max(w, h))
        g.addColorStop(0, c)
        g.addColorStop(1, 'rgba(0,0,0,0)')
        b.fillStyle = g
        b.fillRect(0, 0, w, h)
      }
      // static stars
      const n = Math.round((w * h) / 1800)
      for (let i = 0; i < n; i++) {
        b.globalAlpha = 0.15 + Math.random() * 0.55
        b.fillStyle = Math.random() < 0.12 ? '#CFC8FF' : '#FFFFFF'
        b.beginPath()
        b.arc(Math.random() * w, Math.random() * h, Math.random() * 1.1 + 0.2, 0, Math.PI * 2)
        b.fill()
      }
      b.globalAlpha = 1
      twinkle = Array.from({ length: Math.round(n / 6) }, () => ({ x: Math.random() * w, y: Math.random() * h, r: Math.random() * 1.4 + 0.5, a: 0.4 + Math.random() * 0.6, tw: 0.6 + Math.random() * 2, ph: Math.random() * 6.28 }))
      planets = []
      for (let ring = 0; ring < rings; ring++) {
        const count = ring < 2 ? 1 : ring < 5 ? 2 : 3
        for (let k = 0; k < count; k++) {
          planets.push({ ring, angle: (k / count) * Math.PI * 2 + Math.random(), speed: (0.22 / (ring + 1.6)) * (Math.random() * 0.4 + 0.8), size: 2.2 + Math.random() * 2.8, color: COLORS[(ring + k * 2) % COLORS.length] })
        }
      }
    }

    const draw = (t: number) => {
      const s = t / 1000
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.drawImage(bg, 0, 0, w, h)
      for (const st of twinkle) {
        ctx.globalAlpha = st.a * (0.55 + 0.45 * Math.sin(s * st.tw + st.ph))
        ctx.fillStyle = '#fff'
        ctx.beginPath()
        ctx.arc(st.x, st.y, st.r, 0, Math.PI * 2)
        ctx.fill()
      }
      ctx.globalAlpha = 1
      // the orbit system: wide tilted ellipses around a core right of centre
      const cx = w * (w > 900 ? 0.56 : 0.5), cy = h * 0.54
      const maxR = Math.max(w, h) * 0.62
      const tilt = -0.32, squash = 0.36
      ctx.save()
      ctx.translate(cx, cy)
      ctx.rotate(tilt)
      const core = ctx.createRadialGradient(0, 0, 0, 0, 0, 170)
      const pulse = 0.85 + 0.15 * Math.sin(s * 1.3)
      core.addColorStop(0, `rgba(255,255,255,${0.85 * pulse})`)
      core.addColorStop(0.12, `rgba(255,190,140,${0.55 * pulse})`)
      core.addColorStop(0.4, 'rgba(244,93,156,.18)')
      core.addColorStop(1, 'rgba(123,97,255,0)')
      ctx.fillStyle = core
      ctx.beginPath()
      ctx.ellipse(0, 0, 170, 170 * 0.7, 0, 0, Math.PI * 2)
      ctx.fill()
      for (let ring = 0; ring < rings; ring++) {
        const rx = maxR * (0.16 + ring * 0.13), ry = rx * squash
        ctx.strokeStyle = `rgba(190,180,255,${0.3 - ring * 0.025})`
        ctx.lineWidth = ring % 2 ? 1 : 1.3
        ctx.setLineDash(ring % 2 ? [3, 7] : [])
        ctx.beginPath()
        ctx.ellipse(0, 0, rx, ry, 0, 0, Math.PI * 2)
        ctx.stroke()
      }
      ctx.setLineDash([])
      for (const p of planets) {
        const a = p.angle + s * p.speed
        const rx = maxR * (0.16 + p.ring * 0.13), ry = rx * squash
        // trail
        for (let k = 1; k <= 14; k++) {
          const ta = a - k * 0.025
          ctx.globalAlpha = 0.28 * (1 - k / 14)
          ctx.fillStyle = p.color
          ctx.beginPath()
          ctx.arc(Math.cos(ta) * rx, Math.sin(ta) * ry, p.size * (1 - k / 20), 0, Math.PI * 2)
          ctx.fill()
        }
        const x = Math.cos(a) * rx, y = Math.sin(a) * ry
        const depth = 0.65 + 0.35 * Math.sin(a) // nearer (lower) planets are brighter and larger
        ctx.globalAlpha = 1
        const g = ctx.createRadialGradient(x, y, 0, x, y, p.size * 5)
        g.addColorStop(0, p.color)
        g.addColorStop(1, 'rgba(0,0,0,0)')
        ctx.globalAlpha = 0.45 * depth
        ctx.fillStyle = g
        ctx.beginPath()
        ctx.arc(x, y, p.size * 5, 0, Math.PI * 2)
        ctx.fill()
        ctx.globalAlpha = depth
        ctx.fillStyle = p.color
        ctx.beginPath()
        ctx.arc(x, y, p.size * (0.8 + 0.4 * depth), 0, Math.PI * 2)
        ctx.fill()
      }
      ctx.globalAlpha = 1
      ctx.restore()
    }

    const loop = (t: number) => { draw(t); raf = requestAnimationFrame(loop) }
    layout()
    if (still) draw(0)
    else raf = requestAnimationFrame(loop)
    const onResize = () => { layout(); if (still) draw(0) }
    window.addEventListener('resize', onResize)
    return () => { cancelAnimationFrame(raf); window.removeEventListener('resize', onResize) }
  }, [])
  return <canvas ref={ref} className="lp-galaxy" aria-hidden="true" />
}
