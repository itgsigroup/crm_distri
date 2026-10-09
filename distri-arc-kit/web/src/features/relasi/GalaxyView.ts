import type { RelasiEdge, RelasiNode } from '../../api/types'
import { bindControls } from './controls'
import { layoutGalaxy, type Galaxy, type Satellite } from './galaxy'
import { tipHtml } from './tip'

// Galaxy view of Peta relasi: sales numbers are planets, dealers satellites on their orbits (see galaxy.ts).
// Drawn on a canvas with a perspective camera over the tilted galactic plane: drag to rotate/tilt, Shift/right-drag or
// two fingers to pan, wheel / touchpad / pinch to zoom, click a planet or satellite to focus, click the focused
// dealer again or double-click it to open it. Same public surface as NetView, so RelasiPage can switch views.

export interface GalaxyOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  tip: HTMLElement
  monthLabels: string[]
  periodLabel: string
  onOpen: (dealerId: string) => void
  onFocus?: (id: string | null) => void
  onZoom?: (zoom: number) => void
}

type Tone = 'accent' | 'good' | 'warn' | 'bad' | 'neutral'
const TONE_RGB: Record<Tone, [number, number, number]> = {
  accent: [96, 140, 255],
  good: [52, 199, 120],
  warn: [245, 166, 35],
  bad: [240, 82, 82],
  neutral: [150, 160, 180],
}
const TAN = Math.tan((42 * Math.PI) / 180 / 2)

interface Body {
  id: string
  d: RelasiNode
  planet: boolean
  sat?: Satellite
  x: number
  y: number
  z: number
  r: number
  // projected
  sx: number
  sy: number
  sr: number
  depth: number
  vis: boolean
}

export class GalaxyView {
  private o: GalaxyOptions
  private g: Galaxy
  private nodes: RelasiNode[]
  private edges: RelasiEdge[]
  private byId = new Map<string, RelasiNode>()
  private bodies: Body[] = []
  private bodyById = new Map<string, Body>()
  private ctx: CanvasRenderingContext2D | null
  private stars: { x: number; y: number; z: number; b: number }[] = []
  private yaw = 0.5
  private pitch = 0.62
  private dist = 60
  private fitDist = 60
  private minDist = 4
  private maxDist = 120
  private tx = 0
  private tz = 0
  private ty = 0
  private auto: boolean
  private reduced: boolean
  private raf = 0
  private t0 = performance.now()
  private hover: string | null = null
  /** camera flight towards a clicked planet; any manual move cancels it */
  private goal: { tx: number; tz: number; dist: number } | null = null
  focus: string | null = null
  filter: (id: string) => boolean = () => true
  private w = 0
  private h = 0
  private off: (() => void)[] = []

  constructor(o: GalaxyOptions, nodes: RelasiNode[], edges: RelasiEdge[], months: number) {
    this.o = o
    this.nodes = nodes
    this.edges = edges
    this.reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.auto = !this.reduced
    this.ctx = o.canvas.getContext('2d')
    this.g = layoutGalaxy(nodes, edges, months)
    this.build()
    // a fixed starfield on a far sphere (deterministic)
    let s = 1234567
    const rnd = () => (s = (s * 16807) % 2147483647) / 2147483647
    for (let i = 0; i < 700; i++) {
      const u = rnd() * 2 - 1
      const th = rnd() * Math.PI * 2
      const q = Math.sqrt(1 - u * u)
      this.stars.push({ x: q * Math.cos(th), y: u, z: q * Math.sin(th), b: 0.25 + rnd() * 0.75 })
    }
    this.off.push(bindControls(o.canvas, {
      rotate: (dx, dy) => {
        this.goal = null
        this.yaw += dx * 0.006
        this.pitch = Math.max(0.05, Math.min(1.5, this.pitch + dy * 0.005))
      },
      pan: (dx, dy) => { this.goal = null; this.pan(dx, dy) },
      zoomBy: (f) => { this.goal = null; this.zoomBy(f) },
      click: (e) => {
        const b = this.pickAt(e)
        if (b && !b.planet && this.focus === b.id) this.o.onOpen(b.id)
        else {
          this.focus = b ? b.id : null
          this.o.onFocus?.(this.focus)
          const p = b?.planet ? this.g.planets.find((x) => x.id === b.id) : null
          if (p) this.goal = { tx: p.x, tz: p.z, dist: Math.max(this.minDist, (p.sr / TAN) * 1.25) } // fly to the system
        }
      },
      dblclick: (e) => {
        const b = this.pickAt(e)
        if (b && !b.planet) this.o.onOpen(b.id)
      },
      hover: (e) => this.hoverAt(e),
      leave: () => {
        this.o.tip.classList.remove('show')
        this.hover = null
      },
      setAuto: (on) => { this.auto = on },
    }, this.reduced))
  }

  private build() {
    this.byId = new Map(this.nodes.map((n) => [n.id, n]))
    this.bodies = []
    for (const p of this.g.planets) {
      const d = this.byId.get(p.id)
      if (d) this.bodies.push({ id: p.id, d, planet: true, x: p.x, y: 0, z: p.z, r: p.r, sx: 0, sy: 0, sr: 0, depth: 0, vis: true })
    }
    for (const s of this.g.sats) {
      const d = this.byId.get(s.id)
      if (d) this.bodies.push({ id: s.id, d, planet: false, sat: s, x: 0, y: 0, z: 0, r: s.r, sx: 0, sy: 0, sr: 0, depth: 0, vis: true })
    }
    this.bodyById = new Map(this.bodies.map((b) => [b.id, b]))
  }

  /** New period: orbits are recomputed from the new weights. */
  update(nodes: RelasiNode[], edges: RelasiEdge[], months: number, monthLabels: string[], periodLabel: string) {
    this.nodes = nodes
    this.edges = edges
    this.o.monthLabels = monthLabels
    this.o.periodLabel = periodLabel
    this.g = layoutGalaxy(nodes, edges, months)
    this.build()
    this.focus = null
  }

  zoomBy(factor: number) {
    this.dist = Math.max(this.minDist, Math.min(this.maxDist, this.dist / factor))
    this.o.onZoom?.(this.fitDist / this.dist)
  }

  pan(dx: number, dy: number) {
    const perPx = (2 * this.dist * TAN) / (this.h || 1)
    const cy = Math.cos(this.yaw), sy = Math.sin(this.yaw), cp = Math.cos(this.pitch), sp = Math.sin(this.pitch)
    // screen right / up in world space for the camera below
    this.tx -= cy * dx * perPx
    this.tz -= -sy * dx * perPx
    this.tx += -sy * sp * dy * perPx
    this.ty += cp * dy * perPx
    this.tz += -cy * sp * dy * perPx
  }

  reset() {
    this.goal = null
    this.dist = this.fitDist
    this.tx = this.ty = this.tz = 0
    this.yaw = 0.5
    this.pitch = 0.62
    this.o.onZoom?.(1)
  }

  private fit() {
    this.fitDist = Math.max(12, this.g.extent / TAN / Math.max(1, Math.min(1.6, (this.w || 1) / (this.h || 1))) * 1.05)
    this.dist = this.fitDist
    this.minDist = Math.max(3, this.fitDist / 12)
    this.maxDist = this.fitDist * 2.2
    this.o.onZoom?.(1)
  }

  private connected(a: string, b: string) {
    return this.edges.some((e) => e.w > 0 && ((e.sales === a && e.dealer === b) || (e.dealer === a && e.sales === b)))
  }

  private isOn(id: string) {
    return this.filter(id) && (!this.focus || this.focus === id || this.connected(this.focus, id))
  }

  /** World → screen: rotate by yaw around the vertical axis, tilt by pitch, perspective divide. */
  private project(x: number, y: number, z: number) {
    const X = x - this.tx, Y = y - this.ty, Z = z - this.tz
    const cy = Math.cos(this.yaw), sy = Math.sin(this.yaw), cp = Math.cos(this.pitch), sp = Math.sin(this.pitch)
    const rx = X * cy - Z * sy
    const rz0 = X * sy + Z * cy
    const ry = Y * cp - rz0 * sp
    const rz = Y * sp + rz0 * cp
    const S = this.h / (2 * this.dist * TAN)
    const f = this.dist / Math.max(0.01, this.dist + rz)
    return { sx: this.w / 2 + rx * f * S, sy: this.h / 2 - ry * f * S, f: f * S, depth: rz, vis: this.dist + rz > 0.3 }
  }

  private colorOf(b: Body): [number, number, number] {
    return TONE_RGB[(b.planet ? 'accent' : b.d.tone) as Tone] ?? TONE_RGB.neutral
  }

  private frame = () => {
    this.raf = requestAnimationFrame(this.frame)
    const st = this.o.stage
    if (!st.isConnected || st.getClientRects().length === 0) return
    const w = st.clientWidth
    const h = st.clientHeight
    if (!w || !h || !this.ctx) return
    const dpr = Math.min(devicePixelRatio, 2)
    if (this.w !== w || this.h !== h) {
      const first = !this.w
      this.w = w
      this.h = h
      this.o.canvas.width = w * dpr
      this.o.canvas.height = h * dpr
      if (first) this.fit()
    }
    const now = performance.now()
    const t = this.reduced ? 0 : (now - this.t0) / 1000
    if (this.auto) this.yaw += 0.0012
    if (this.goal) {
      const k = this.reduced ? 1 : 0.12
      this.tx += (this.goal.tx - this.tx) * k
      this.tz += (this.goal.tz - this.tz) * k
      this.ty += (0 - this.ty) * k
      this.dist += (this.goal.dist - this.dist) * k
      this.o.onZoom?.(this.fitDist / this.dist)
      if (Math.abs(this.goal.dist - this.dist) < 0.01 && Math.hypot(this.goal.tx - this.tx, this.goal.tz - this.tz) < 0.01) this.goal = null
    }
    const ctx = this.ctx
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

    // space
    const bg = ctx.createRadialGradient(w / 2, h / 2, 0, w / 2, h / 2, Math.hypot(w, h) / 1.6)
    bg.addColorStop(0, '#141a33')
    bg.addColorStop(1, '#05060d')
    ctx.fillStyle = bg
    ctx.fillRect(0, 0, w, h)
    const cy = Math.cos(this.yaw), sy = Math.sin(this.yaw), cp = Math.cos(this.pitch), sp = Math.sin(this.pitch)
    const fov = h / (2 * TAN)
    for (const s of this.stars) { // rotation only: stars are infinitely far away
      const rx = s.x * cy - s.z * sy
      const rz0 = s.x * sy + s.z * cy
      const ry = s.y * cp - rz0 * sp
      const rz = s.y * sp + rz0 * cp
      if (rz > -0.05) continue
      const px = w / 2 + (rx / -rz) * fov
      const py = h / 2 - (ry / -rz) * fov
      if (px < 0 || py < 0 || px > w || py > h) continue
      ctx.globalAlpha = s.b * 0.8
      ctx.fillStyle = '#dfe6ff'
      ctx.fillRect(px, py, s.b > 0.8 ? 1.6 : 1, s.b > 0.8 ? 1.6 : 1)
    }

    // galaxy core
    const core = this.project(0, 0, 0)
    if (core.vis) {
      const cr = Math.max(30, 5 * core.f)
      const cg = ctx.createRadialGradient(core.sx, core.sy, 0, core.sx, core.sy, cr * 3)
      cg.addColorStop(0, 'rgba(255,236,200,0.55)')
      cg.addColorStop(0.25, 'rgba(160,140,255,0.18)')
      cg.addColorStop(1, 'rgba(60,60,140,0)')
      ctx.globalAlpha = 1
      ctx.fillStyle = cg
      ctx.beginPath()
      ctx.arc(core.sx, core.sy, cr * 3, 0, Math.PI * 2)
      ctx.fill()
      ctx.fillStyle = 'rgba(255,240,215,0.85)'
      ctx.font = `800 ${Math.max(11, Math.min(18, core.f * 0.9))}px var(--font-display, sans-serif)`
      ctx.textAlign = 'center'
      ctx.fillText('GSI', core.sx, core.sy + 5)
    }

    // positions: planets fixed, satellites move along their orbits
    const planetPos = new Map(this.g.planets.map((p) => [p.id, p]))
    for (const b of this.bodies) {
      if (b.sat) {
        const s = b.sat
        const c = s.planet ? planetPos.get(s.planet)! : { x: 0, z: 0 }
        const a = s.phase + s.speed * t
        b.x = c.x + Math.cos(a) * s.orbit
        b.z = c.z + Math.sin(a) * s.orbit * Math.cos(s.incl)
        b.y = Math.sin(a) * s.orbit * Math.sin(s.incl)
      }
      const p = this.project(b.x, b.y, b.z)
      b.sx = p.sx
      b.sy = p.sy
      b.sr = b.r * p.f
      b.depth = p.depth
      b.vis = p.vis
    }

    // orbits (rings drawn in the inclined plane of each satellite)
    ctx.lineWidth = 1
    for (const b of this.bodies) {
      if (!b.sat) continue
      const s = b.sat
      const c = s.planet ? planetPos.get(s.planet)! : { x: 0, z: 0 }
      const on = this.isOn(b.id)
      const strong = !!this.focus && (this.focus === s.planet || this.focus === b.id)
      ctx.globalAlpha = strong ? 0.35 : on ? (s.planet ? 0.1 : 0.05) : 0.02
      ctx.strokeStyle = strong ? '#9fb6ff' : '#8090c0'
      ctx.beginPath()
      for (let k = 0; k <= 48; k++) {
        const a = (k / 48) * Math.PI * 2
        const p = this.project(c.x + Math.cos(a) * s.orbit, Math.sin(a) * s.orbit * Math.sin(s.incl), c.z + Math.sin(a) * s.orbit * Math.cos(s.incl))
        if (k) ctx.lineTo(p.sx, p.sy)
        else ctx.moveTo(p.sx, p.sy)
      }
      ctx.stroke()
    }

    // bridges: a dealer that is also close to another sales number
    for (const e of this.edges) {
      if (e.w <= 0) continue
      const d = this.bodyById.get(e.dealer)
      const s = this.bodyById.get(e.sales)
      if (!d || !s || d.sat?.planet === e.sales) continue
      const hot = !!this.focus && (this.focus === e.dealer || this.focus === e.sales)
      const on = this.filter(e.dealer) && this.filter(e.sales)
      ctx.globalAlpha = hot ? 0.75 : on && !this.focus ? 0.12 : 0.03
      ctx.strokeStyle = hot ? '#ffd27a' : '#b8c4ff'
      ctx.setLineDash(hot ? [] : [3, 4])
      ctx.beginPath()
      ctx.moveTo(d.sx, d.sy)
      ctx.lineTo(s.sx, s.sy)
      ctx.stroke()
    }
    ctx.setLineDash([])
    // the focused satellite's line to its own planet
    const fb = this.focus ? this.bodyById.get(this.focus) : null
    if (fb?.sat?.planet) {
      const p = this.bodyById.get(fb.sat.planet)
      if (p) {
        ctx.globalAlpha = 0.8
        ctx.strokeStyle = '#ffd27a'
        ctx.beginPath()
        ctx.moveTo(fb.sx, fb.sy)
        ctx.lineTo(p.sx, p.sy)
        ctx.stroke()
      }
    }

    // bodies, far to near
    const order = this.bodies.filter((b) => b.vis).sort((a, b) => b.depth - a.depth)
    for (const b of order) {
      const on = this.isOn(b.id)
      const [r, g, bl] = this.colorOf(b)
      const rad = Math.max(b.planet ? 3 : 1.4, b.sr)
      if (b.planet) { // glow
        const gl = ctx.createRadialGradient(b.sx, b.sy, rad * 0.6, b.sx, b.sy, rad * 2.6)
        gl.addColorStop(0, `rgba(${r},${g},${bl},0.45)`)
        gl.addColorStop(1, `rgba(${r},${g},${bl},0)`)
        ctx.globalAlpha = on ? 1 : 0.15
        ctx.fillStyle = gl
        ctx.beginPath()
        ctx.arc(b.sx, b.sy, rad * 2.6, 0, Math.PI * 2)
        ctx.fill()
      }
      const sg = ctx.createRadialGradient(b.sx - rad * 0.35, b.sy - rad * 0.35, rad * 0.1, b.sx, b.sy, rad)
      sg.addColorStop(0, `rgb(${Math.min(255, r + 90)},${Math.min(255, g + 90)},${Math.min(255, bl + 90)})`)
      sg.addColorStop(0.55, `rgb(${r},${g},${bl})`)
      sg.addColorStop(1, `rgb(${Math.round(r * 0.35)},${Math.round(g * 0.35)},${Math.round(bl * 0.4)})`)
      ctx.globalAlpha = on ? 1 : 0.14
      ctx.fillStyle = sg
      ctx.beginPath()
      ctx.arc(b.sx, b.sy, rad, 0, Math.PI * 2)
      ctx.fill()
      if (this.hover === b.id || this.focus === b.id) {
        ctx.globalAlpha = 1
        ctx.strokeStyle = '#ffffff'
        ctx.lineWidth = 1.5
        ctx.beginPath()
        ctx.arc(b.sx, b.sy, rad + 3, 0, Math.PI * 2)
        ctx.stroke()
        ctx.lineWidth = 1
      }
    }

    // labels: every planet; satellites of the focused planet, the focused / hovered satellite, and big ones up close
    ctx.textAlign = 'center'
    const placed: [number, number, number, number][] = []
    const free = (x0: number, y0: number, x1: number, y1: number) => !placed.some(([a0, b0, a1, b1]) => x0 < a1 && x1 > a0 && y0 < b1 && y1 > b0)
    const labelOrder = [...order].sort((a, b) => Number(b.planet) - Number(a.planet) || Number(b.id === this.focus || b.id === this.hover) - Number(a.id === this.focus || a.id === this.hover) || b.sr - a.sr)
    for (const b of labelOrder) {
      const on = this.isOn(b.id)
      if (!on) continue
      const focusSys = !!this.focus && !b.planet && (b.sat?.planet === this.focus || this.focus === b.id)
      const show = b.planet || focusSys || this.hover === b.id || (b.sr > 9 && !this.focus)
      if (!show) continue
      const rad = Math.max(b.planet ? 3 : 1.4, b.sr)
      ctx.globalAlpha = 1
      ctx.font = b.planet ? '700 12.5px var(--font-display, sans-serif)' : '600 11px var(--font-body, sans-serif)'
      ctx.fillStyle = b.planet ? '#cfe0ff' : '#ffffff'
      ctx.shadowColor = 'rgba(0,0,0,0.9)'
      ctx.shadowBlur = 4
      const ly = b.sy - rad - (b.planet ? 16 : 6)
      const tw = ctx.measureText(b.d.name).width
      // satellites' names never cover a planet's or another name; the hovered / focused one always shows
      if (!b.planet && this.hover !== b.id && this.focus !== b.id && !free(b.sx - tw / 2, ly - 11, b.sx + tw / 2, ly + 2)) continue
      placed.push([b.sx - tw / 2 - 2, ly - 12, b.sx + tw / 2 + 2, b.planet ? ly + 15 : ly + 3])
      ctx.fillText(b.d.name, b.sx, ly)
      if (b.planet) {
        ctx.font = '500 10px var(--font-body, sans-serif)'
        ctx.fillStyle = 'rgba(200,210,240,0.75)'
        ctx.fillText(`${b.d.sub} · ${this.g.planets.find((p) => p.id === b.id)?.sats.length ?? 0} dealer`, b.sx, b.sy - rad - 4)
      }
      ctx.shadowBlur = 0
    }
    ctx.globalAlpha = 1
  }

  private pickAt(e: PointerEvent | MouseEvent): Body | null {
    const r = this.o.stage.getBoundingClientRect()
    const mx = e.clientX - r.left
    const my = e.clientY - r.top
    let best: Body | null = null
    let bd = 1e9
    for (const b of this.bodies) {
      if (!b.vis) continue
      const d = Math.hypot(b.sx - mx, b.sy - my)
      const rad = Math.max(b.planet ? 3 : 1.4, b.sr) + 5
      if (d < rad && d - (b.planet ? 2 : 0) < bd) {
        bd = d
        best = b
      }
    }
    return best
  }

  private hoverAt(e: PointerEvent) {
    const b = this.pickAt(e)
    const tip = this.o.tip
    this.hover = b ? b.id : null
    this.o.stage.style.cursor = b ? 'pointer' : ''
    if (!b) {
      tip.classList.remove('show')
      return
    }
    const r = this.o.stage.getBoundingClientRect()
    let px = e.clientX - r.left + 14
    const py = e.clientY - r.top + 14
    if (px + 240 > r.width) px -= 260
    tip.style.left = px + 'px'
    tip.style.top = py + 'px'
    tip.innerHTML = tipHtml(b.d, this.edges, (id) => this.byId.get(id)?.name ?? id, this.o.periodLabel, this.o.monthLabels)
    tip.classList.add('show')
  }

  start() {
    if (!this.raf) this.frame()
  }

  destroy() {
    cancelAnimationFrame(this.raf)
    this.raf = 0
    this.off.forEach((f) => f())
    this.o.tip.classList.remove('show')
    const ctx = this.ctx
    if (ctx) {
      ctx.setTransform(1, 0, 0, 1, 0, 0)
      ctx.clearRect(0, 0, this.o.canvas.width, this.o.canvas.height)
    }
  }
}
