import type { RelasiEdge, RelasiNode } from '../../api/types'
import { bindControls } from './controls'
import { layoutGalaxy, layoutLeads, onPlane, rng, type Galaxy, type Meteor, type Planet, type Satellite } from './galaxy'
import { esc, tipHtml } from './tip'

// Galaxy view of Peta relasi: sales numbers are planets scattered through space, dealers satellites on their orbits,
// prospects (leads that never ordered) appear now and then as a comet streaking down the screen, and the ones no
// sales owns drift through as a few meteors (see galaxy.ts). Starfield with a faint Milky Way behind everything.
// Drawn on a canvas with a perspective camera: drag to rotate/tilt, Shift/right-drag or two fingers to pan, wheel /
// touchpad / pinch to zoom towards the pointer (fly through the galaxy), click a planet to fly to its system,
// click the focused dealer again or double-click it to open it. Same public surface as NetView.

/** A prospect: shown as a passing comet (any prospect, one at a time) or a meteor (owner = '', at most MAX_METEORS). */
export interface GalaxyLead { id: string; name: string; sub: string; owner: string; ownerName: string }

export interface GalaxyOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  tip: HTMLElement
  monthLabels: string[]
  periodLabel: string
  leads?: GalaxyLead[]
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

type Kind = 'planet' | 'sat' | 'meteor'
const MAX_METEORS = 30

interface Star { x: number; y: number; z: number; b: number; size: number; tint: string; tw: number }
/** A comet streaking across the screen (screen space), carrying one prospect. */
interface Streak { lead: GalaxyLead; x0: number; y0: number; vx: number; vy: number; start: number; dur: number; x: number; y: number }

interface Body {
  id: string
  kind: Kind
  name: string
  d?: RelasiNode
  lead?: GalaxyLead
  planet?: Planet
  sat?: Satellite
  meteor?: Meteor
  x: number
  y: number
  z: number
  r: number
  // projected values
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
  private planetById = new Map<string, Planet>()
  private bodies: Body[] = []
  private bodyById = new Map<string, Body>()
  private ctx: CanvasRenderingContext2D | null
  private stars: Star[] = []
  private leadPool: GalaxyLead[] = []
  private leadNext = 0
  private streak: Streak | null = null
  private nextStreak = 0
  private dust: { x: number; y: number; z: number }[] = []
  private yaw = 0.5
  private pitch = 0.5
  private dist = 60
  private fitDist = 60
  private minDist = 2
  private maxDist = 120
  private tx = 0
  private ty = 0
  private tz = 0
  private auto: boolean
  private reduced: boolean
  private raf = 0
  private t0 = performance.now()
  private hover: string | null = null
  /** camera flight towards a clicked planet; any manual move cancels it */
  private goal: { tx: number; ty: number; tz: number; dist: number } | null = null
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
    this.stars = makeStars()
    this.off.push(bindControls(o.canvas, {
      rotate: (dx, dy) => {
        this.goal = null
        this.yaw += dx * 0.006
        this.pitch = Math.max(-1.45, Math.min(1.45, this.pitch + dy * 0.005))
      },
      pan: (dx, dy) => { this.goal = null; this.pan(dx, dy) },
      zoomBy: (f, cx, cy) => { this.goal = null; this.zoomBy(f, cx, cy) },
      click: (e) => {
        if (this.streakAt(e)) { this.o.onOpen(this.streak!.lead.id); return }
        const b = this.pickAt(e)
        if (b && b.kind !== 'planet' && this.focus === b.id) this.o.onOpen(b.id)
        else {
          this.focus = b ? b.id : null
          this.o.onFocus?.(this.focus)
          if (b?.planet) this.goal = { tx: b.planet.x, ty: b.planet.y, tz: b.planet.z, dist: Math.max(this.minDist, (b.planet.sr / TAN) * 1.25) } // fly to the system
        }
      },
      dblclick: (e) => {
        const b = this.pickAt(e)
        if (b && b.kind !== 'planet') this.o.onOpen(b.id)
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
    this.planetById = new Map(this.g.planets.map((p) => [p.id, p]))
    const blank = { x: 0, y: 0, z: 0, sx: 0, sy: 0, sr: 0, depth: 0, vis: false }
    this.bodies = []
    for (const p of this.g.planets) {
      const d = this.byId.get(p.id)
      if (d) this.bodies.push({ ...blank, id: p.id, kind: 'planet', name: d.name, d, planet: p, x: p.x, y: p.y, z: p.z, r: p.r })
    }
    for (const s of this.g.sats) {
      const d = this.byId.get(s.id)
      if (d) this.bodies.push({ ...blank, id: s.id, kind: 'sat', name: d.name, d, sat: s, r: s.r })
    }
    const leads = (this.o.leads ?? []).filter((l) => !this.byId.has(l.id))
    const { meteors } = layoutLeads(this.g, leads)
    const leadById = new Map(leads.map((l) => [l.id, l]))
    // comets take turns: every prospect gets its moment, in a shuffled but stable order
    const shuffle = rng('comets')
    this.leadPool = leads.map((l) => [shuffle(), l] as const).sort((a, b) => a[0] - b[0]).map(([, l]) => l)
    this.leadNext = 0
    for (const m of meteors.slice(0, MAX_METEORS)) this.bodies.push({ ...blank, id: m.id, kind: 'meteor', name: leadById.get(m.id)!.name, lead: leadById.get(m.id), meteor: m, r: 0.35 + Math.abs(Math.sin(m.vx * 9)) * 0.3 })
    this.bodyById = new Map(this.bodies.map((b) => [b.id, b]))
    // space dust spread through the galaxy: close to the camera it rushes past and gives the feeling of flying
    const rnd = rng('dust')
    const E = this.g.extent * 1.4
    this.dust = Array.from({ length: 600 }, () => ({ x: (rnd() * 2 - 1) * E, y: (rnd() * 2 - 1) * E * 0.6, z: (rnd() * 2 - 1) * E }))
  }

  /** Prospects arrived or changed: comets and meteors are rebuilt, everything else stays. */
  setLeads(leads: GalaxyLead[]) {
    this.o.leads = leads
    this.build()
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

  /** Zoom; with a pointer position the point under it stays under it, so zooming flies towards what you point at. */
  zoomBy(factor: number, clientX?: number, clientY?: number) {
    const before = this.dist
    this.dist = Math.max(this.minDist, Math.min(this.maxDist, this.dist / factor))
    const f = before / this.dist
    if (clientX != null && clientY != null && f !== 1) {
      const r = this.o.stage.getBoundingClientRect()
      const sx = clientX - r.left - this.w / 2
      const sy = clientY - r.top - this.h / 2
      this.pan(-sx * (f - 1), -sy * (f - 1))
    }
    this.o.onZoom?.(this.fitDist / this.dist)
  }

  /** Move what the camera looks at by a screen distance in pixels (the galaxy follows the pointer). */
  pan(dx: number, dy: number) {
    const k = (2 * this.dist * TAN) / (this.h || 1)
    const cy = Math.cos(this.yaw), sy = Math.sin(this.yaw), cp = Math.cos(this.pitch), sp = Math.sin(this.pitch)
    // screen right = (cy, 0, -sy), screen up = (-sy·sp, cp, -cy·sp) for the projection below
    this.tx += (-cy * dx - sy * sp * dy) * k
    this.ty += cp * dy * k
    this.tz += (sy * dx - cy * sp * dy) * k
  }

  reset() {
    this.goal = null
    this.dist = this.fitDist
    this.tx = this.ty = this.tz = 0
    this.yaw = 0.5
    this.pitch = 0.5
    this.o.onZoom?.(1)
  }

  private fit() {
    this.fitDist = Math.max(12, (this.g.extent / TAN / Math.max(1, Math.min(1.6, (this.w || 1) / (this.h || 1)))) * 1.0)
    this.dist = this.fitDist
    this.minDist = Math.max(1.2, this.fitDist / 40)
    this.maxDist = this.fitDist * 2.2
    this.o.onZoom?.(1)
  }

  private connected(a: string, b: string) {
    if (this.edges.some((e) => e.w > 0 && ((e.sales === a && e.dealer === b) || (e.dealer === a && e.sales === b)))) return true
    const la = this.bodyById.get(a)?.lead
    const lb = this.bodyById.get(b)?.lead
    return (!!la && la.owner === b) || (!!lb && lb.owner === a)
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
    const zc = this.dist + rz // distance in front of the camera
    const f = this.dist / Math.max(0.05, zc)
    return { sx: this.w / 2 + rx * f * S, sy: this.h / 2 - ry * f * S, f: f * S, depth: rz, vis: zc > 0.4 }
  }

  private rgbOf(b: Body): [number, number, number] {
    if (b.kind === 'planet') return TONE_RGB.accent
    if (b.kind === 'meteor') return [160, 140, 120]
    return TONE_RGB[(b.d?.tone ?? 'neutral') as Tone] ?? TONE_RGB.neutral
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
    const t = this.reduced ? 0 : (performance.now() - this.t0) / 1000
    if (this.auto) this.yaw += 0.0010
    if (this.goal) {
      const k = this.reduced ? 1 : 0.1
      this.tx += (this.goal.tx - this.tx) * k
      this.ty += (this.goal.ty - this.ty) * k
      this.tz += (this.goal.tz - this.tz) * k
      this.dist += (this.goal.dist - this.dist) * k
      this.o.onZoom?.(this.fitDist / this.dist)
      if (Math.abs(this.goal.dist - this.dist) < 0.01 && Math.hypot(this.goal.tx - this.tx, this.goal.ty - this.ty, this.goal.tz - this.tz) < 0.01) this.goal = null
    }
    const ctx = this.ctx
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)

    // deep space and far stars
    const bg = ctx.createRadialGradient(w / 2, h / 2, 0, w / 2, h / 2, Math.hypot(w, h) / 1.6)
    bg.addColorStop(0, '#121833')
    bg.addColorStop(1, '#04050b')
    ctx.globalAlpha = 1
    ctx.fillStyle = bg
    ctx.fillRect(0, 0, w, h)
    const cy = Math.cos(this.yaw), sy = Math.sin(this.yaw), cp = Math.cos(this.pitch), sp = Math.sin(this.pitch)
    const fov = h / (2 * TAN)
    const now = performance.now() / 1000
    for (const s of this.stars) { // far away: they only turn with the camera, and twinkle
      const rx = s.x * cy - s.z * sy
      const rz0 = s.x * sy + s.z * cy
      const ry = s.y * cp - rz0 * sp
      const rz = s.y * sp + rz0 * cp
      if (rz > -0.05) continue
      const px = w / 2 + (rx / -rz) * fov
      const py = h / 2 - (ry / -rz) * fov
      if (px < -4 || py < -4 || px > w + 4 || py > h + 4) continue
      const a = s.tw && !this.reduced ? s.b * (0.65 + 0.35 * Math.sin(now * s.tw + s.x * 40)) : s.b
      ctx.globalAlpha = a
      ctx.fillStyle = s.tint
      if (s.size > 2) { // a bright star: glow and a small cross
        const gl = ctx.createRadialGradient(px, py, 0, px, py, s.size * 2.4)
        gl.addColorStop(0, s.tint)
        gl.addColorStop(1, 'rgba(0,0,0,0)')
        ctx.fillStyle = gl
        ctx.fillRect(px - s.size * 2.4, py - s.size * 2.4, s.size * 4.8, s.size * 4.8)
        ctx.fillStyle = s.tint
        ctx.globalAlpha = a * 0.6
        ctx.fillRect(px - s.size * 2, py - 0.5, s.size * 4, 1)
        ctx.fillRect(px - 0.5, py - s.size * 2, 1, s.size * 4)
        ctx.globalAlpha = a
        ctx.beginPath()
        ctx.arc(px, py, s.size * 0.5, 0, Math.PI * 2)
        ctx.fill()
      } else ctx.fillRect(px - s.size / 2, py - s.size / 2, s.size, s.size)
    }
    // space dust: perspective points, the nearer the bigger and brighter
    ctx.fillStyle = '#aab8ff'
    for (const d of this.dust) {
      const p = this.project(d.x, d.y, d.z)
      if (!p.vis || p.sx < -5 || p.sy < -5 || p.sx > w + 5 || p.sy > h + 5) continue
      const sz = Math.min(3, 0.04 * p.f)
      if (sz < 0.35) continue
      ctx.globalAlpha = Math.min(0.55, sz * 0.25)
      ctx.fillRect(p.sx, p.sy, sz, sz)
    }

    // galaxy core
    const core = this.project(0, 0, 0)
    if (core.vis) {
      const cr = Math.max(26, 3.2 * core.f)
      const cg = ctx.createRadialGradient(core.sx, core.sy, 0, core.sx, core.sy, cr * 3)
      cg.addColorStop(0, 'rgba(255,236,200,0.55)')
      cg.addColorStop(0.25, 'rgba(170,140,255,0.16)')
      cg.addColorStop(1, 'rgba(60,60,140,0)')
      ctx.globalAlpha = 1
      ctx.fillStyle = cg
      ctx.beginPath()
      ctx.arc(core.sx, core.sy, cr * 3, 0, Math.PI * 2)
      ctx.fill()
      ctx.fillStyle = 'rgba(255,240,215,0.85)'
      ctx.font = `800 ${Math.max(11, Math.min(20, core.f * 0.9))}px var(--font-display, sans-serif)`
      ctx.textAlign = 'center'
      ctx.fillText('GSI', core.sx, core.sy + 5)
    }

    // positions: planets fixed in space, satellites on their tilted orbits, comets on ellipses, meteors drifting
    const E = this.g.extent * 1.3
    const wrap = (v: number, m: number) => ((((v + m) % (2 * m)) + 2 * m) % (2 * m)) - m
    for (const b of this.bodies) {
      if (b.sat) {
        const s = b.sat
        const c = s.planet ? this.planetById.get(s.planet)! : null
        const q = onPlane(s.orbit, s.phase + s.speed * t, (c?.tilt ?? 0) + s.incl, c?.node ?? 0)
        b.x = (c?.x ?? 0) + q.x
        b.y = (c?.y ?? 0) + q.y
        b.z = (c?.z ?? 0) + q.z
      } else if (b.meteor) {
        const m = b.meteor
        b.x = wrap(m.x + m.vx * t, E)
        b.y = wrap(m.y + m.vy * t, E * 0.6)
        b.z = wrap(m.z + m.vz * t, E)
      }
      const p = this.project(b.x, b.y, b.z)
      b.sx = p.sx
      b.sy = p.sy
      b.sr = b.r * p.f
      b.depth = p.depth
      b.vis = p.vis && p.sx > -200 && p.sy > -200 && p.sx < w + 200 && p.sy < h + 200
    }

    // orbit rings of the satellites
    ctx.lineWidth = 1
    for (const b of this.bodies) {
      if (!b.sat) continue
      const s = b.sat
      const c = s.planet ? this.planetById.get(s.planet)! : null
      const strong = !!this.focus && (this.focus === s.planet || this.focus === b.id)
      if (!strong && !b.vis && !(c && this.bodyById.get(c.id)?.vis)) continue
      const on = this.isOn(b.id)
      ctx.globalAlpha = strong ? 0.35 : on ? (s.planet ? 0.09 : 0.04) : 0.015
      ctx.strokeStyle = strong ? '#9fb6ff' : '#8090c0'
      ctx.beginPath()
      let started = false
      for (let k = 0; k <= 48; k++) {
        const q = onPlane(s.orbit, (k / 48) * Math.PI * 2, (c?.tilt ?? 0) + s.incl, c?.node ?? 0)
        const p = this.project((c?.x ?? 0) + q.x, (c?.y ?? 0) + q.y, (c?.z ?? 0) + q.z)
        if (!p.vis) { started = false; continue }
        if (started) ctx.lineTo(p.sx, p.sy)
        else ctx.moveTo(p.sx, p.sy)
        started = true
      }
      ctx.stroke()
    }

    // bridges: a dealer that is also close to another sales number
    for (const e of this.edges) {
      if (e.w <= 0) continue
      const d = this.bodyById.get(e.dealer)
      const s = this.bodyById.get(e.sales)
      if (!d || !s || d.sat?.planet === e.sales || (!d.vis && !s.vis)) continue
      const hot = !!this.focus && (this.focus === e.dealer || this.focus === e.sales)
      const on = this.filter(e.dealer) && this.filter(e.sales)
      ctx.globalAlpha = hot ? 0.75 : on && !this.focus ? 0.1 : 0.025
      ctx.strokeStyle = hot ? '#ffd27a' : '#b8c4ff'
      ctx.setLineDash(hot ? [] : [3, 4])
      ctx.beginPath()
      ctx.moveTo(d.sx, d.sy)
      ctx.lineTo(s.sx, s.sy)
      ctx.stroke()
    }
    ctx.setLineDash([])
    const fb = this.focus ? this.bodyById.get(this.focus) : null
    const home = fb?.sat?.planet
    if (fb && home) { // the focused satellite and its own planet
      const p = this.bodyById.get(home)
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
      const [r, g, bl] = this.rgbOf(b)
      const rad = Math.max(b.kind === 'planet' ? 3 : 1.2, b.sr)
      if (b.kind === 'meteor') this.drawMeteor(ctx, b, rad, on, t)
      if (b.kind === 'planet') { // glow
        const gl = ctx.createRadialGradient(b.sx, b.sy, rad * 0.6, b.sx, b.sy, rad * 2.6)
        gl.addColorStop(0, `rgba(${r},${g},${bl},0.45)`)
        gl.addColorStop(1, `rgba(${r},${g},${bl},0)`)
        ctx.globalAlpha = on ? 1 : 0.15
        ctx.fillStyle = gl
        ctx.beginPath()
        ctx.arc(b.sx, b.sy, rad * 2.6, 0, Math.PI * 2)
        ctx.fill()
      }
      if (b.kind === 'planet' || b.kind === 'sat') {
        const sg = ctx.createRadialGradient(b.sx - rad * 0.35, b.sy - rad * 0.35, rad * 0.1, b.sx, b.sy, rad)
        sg.addColorStop(0, `rgb(${Math.min(255, r + 90)},${Math.min(255, g + 90)},${Math.min(255, bl + 90)})`)
        sg.addColorStop(0.55, `rgb(${r},${g},${bl})`)
        sg.addColorStop(1, `rgb(${Math.round(r * 0.35)},${Math.round(g * 0.35)},${Math.round(bl * 0.4)})`)
        ctx.globalAlpha = on ? 1 : 0.14
        ctx.fillStyle = sg
        ctx.beginPath()
        ctx.arc(b.sx, b.sy, rad, 0, Math.PI * 2)
        ctx.fill()
      }
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

    // labels: every planet; satellites of the focused system, the focused / hovered body, big ones up close —
    // never on top of each other
    ctx.textAlign = 'center'
    const placed: [number, number, number, number][] = []
    const free = (x0: number, y0: number, x1: number, y1: number) => !placed.some(([a0, b0, a1, b1]) => x0 < a1 && x1 > a0 && y0 < b1 && y1 > b0)
    const special = (b: Body) => b.id === this.focus || b.id === this.hover
    const labelOrder = [...order].sort((a, b) => Number(b.kind === 'planet') - Number(a.kind === 'planet') || Number(special(b)) - Number(special(a)) || b.sr - a.sr)
    for (const b of labelOrder) {
      if (!this.isOn(b.id)) continue
      const sysFocus = !!this.focus && b.kind === 'sat' && b.sat?.planet === this.focus
      const show = b.kind === 'planet' || special(b) || sysFocus || (b.kind === 'sat' && b.sr > 9 && !this.focus)
      if (!show) continue
      const rad = Math.max(b.kind === 'planet' ? 3 : 1.2, b.sr)
      ctx.font = b.kind === 'planet' ? '700 12.5px var(--font-display, sans-serif)' : '600 11px var(--font-body, sans-serif)'
      const tw = ctx.measureText(b.name).width
      let ly = b.sy - rad - (b.kind === 'planet' ? 16 : 6)
      if (b.kind === 'planet') { // a planet's name goes above it, or below when another name is in the way
        const below = b.sy + rad + 14
        if (!free(b.sx - tw / 2, ly - 12, b.sx + tw / 2, ly + 15) && free(b.sx - tw / 2, below - 12, b.sx + tw / 2, below + 15)) ly = below
      } else if (!special(b) && !free(b.sx - tw / 2, ly - 11, b.sx + tw / 2, ly + 2)) continue
      placed.push([b.sx - tw / 2 - 2, ly - 12, b.sx + tw / 2 + 2, b.kind === 'planet' ? ly + 15 : ly + 3])
      ctx.globalAlpha = 1
      ctx.fillStyle = b.kind === 'planet' ? '#cfe0ff' : b.kind === 'sat' ? '#ffffff' : '#bfe9ff'
      ctx.shadowColor = 'rgba(0,0,0,0.9)'
      ctx.shadowBlur = 4
      ctx.fillText(b.name, b.sx, ly)
      if (b.kind === 'planet') {
        ctx.font = '500 10px var(--font-body, sans-serif)'
        ctx.fillStyle = 'rgba(200,210,240,0.75)'
        ctx.fillText(`${b.d?.sub ?? ''} · ${b.planet?.sats.length ?? 0} dealer`, b.sx, ly + 12)
      }
      ctx.shadowBlur = 0
    }
    ctx.globalAlpha = 1
    this.drawStreak(ctx, w, h)
  }

  /** Now and then one prospect streaks down the screen as a comet, with its name for a moment. */
  private drawStreak(ctx: CanvasRenderingContext2D, w: number, h: number) {
    const now = performance.now()
    if (!this.streak && !this.reduced && this.leadPool.length && now >= this.nextStreak) {
      const rnd = Math.random
      const lead = this.leadPool[this.leadNext++ % this.leadPool.length]
      const x0 = w * (0.1 + rnd() * 0.8)
      const ang = (Math.PI / 2) + (rnd() - 0.5) * 1.1 // mostly downwards, a little to either side
      const dur = 2600 + rnd() * 1600
      const len = h + 260
      this.streak = { lead, x0, y0: -60, vx: (Math.cos(ang) * len) / dur, vy: (Math.sin(ang) * len) / dur, start: now, dur, x: x0, y: -60 }
    }
    const k = this.streak
    if (!k) return
    const el = now - k.start
    if (el > k.dur) {
      this.streak = null
      this.nextStreak = now + 3000 + Math.random() * 4000
      return
    }
    k.x = k.x0 + k.vx * el
    k.y = k.y0 + k.vy * el
    const sp = Math.hypot(k.vx, k.vy) || 1
    const ux = k.vx / sp
    const uy = k.vy / sp
    const tail = 150
    const fade = Math.min(1, el / 300, (k.dur - el) / 400)
    const hot = this.hover === 'streak:' + k.lead.id
    const gr = ctx.createLinearGradient(k.x, k.y, k.x - ux * tail, k.y - uy * tail)
    gr.addColorStop(0, 'rgba(210,240,255,0.9)')
    gr.addColorStop(0.3, 'rgba(140,200,255,0.35)')
    gr.addColorStop(1, 'rgba(100,150,255,0)')
    ctx.globalAlpha = fade
    ctx.fillStyle = gr
    ctx.beginPath()
    ctx.moveTo(k.x + uy * 3, k.y - ux * 3)
    ctx.lineTo(k.x - ux * tail + uy * 9, k.y - uy * tail - ux * 9)
    ctx.lineTo(k.x - ux * tail - uy * 9, k.y - uy * tail + ux * 9)
    ctx.lineTo(k.x - uy * 3, k.y + ux * 3)
    ctx.closePath()
    ctx.fill()
    const hg = ctx.createRadialGradient(k.x, k.y, 0, k.x, k.y, 10)
    hg.addColorStop(0, 'rgba(255,255,255,1)')
    hg.addColorStop(0.35, 'rgba(190,230,255,0.8)')
    hg.addColorStop(1, 'rgba(120,180,255,0)')
    ctx.fillStyle = hg
    ctx.beginPath()
    ctx.arc(k.x, k.y, hot ? 13 : 10, 0, Math.PI * 2)
    ctx.fill()
    ctx.font = '600 11px var(--font-body, sans-serif)'
    ctx.textAlign = 'left'
    ctx.fillStyle = '#d8efff'
    ctx.shadowColor = 'rgba(0,0,0,0.9)'
    ctx.shadowBlur = 4
    ctx.globalAlpha = fade * 0.9
    ctx.fillText(`☄ ${k.lead.name} · prospek`, k.x + 12, k.y + 4)
    ctx.shadowBlur = 0
    ctx.globalAlpha = 1
  }

  private streakAt(e: PointerEvent | MouseEvent) {
    const k = this.streak
    if (!k) return false
    const r = this.o.stage.getBoundingClientRect()
    return Math.hypot(e.clientX - r.left - k.x, e.clientY - r.top - k.y) < 18
  }

  /** Rocky meteor with a short fiery streak behind it. */
  private drawMeteor(ctx: CanvasRenderingContext2D, b: Body, rad: number, on: boolean, t: number) {
    const m = b.meteor!
    const back = this.project(b.x - m.vx * 2.2, b.y - m.vy * 2.2, b.z - m.vz * 2.2)
    if (back.vis) {
      const gr = ctx.createLinearGradient(b.sx, b.sy, back.sx, back.sy)
      gr.addColorStop(0, 'rgba(255,170,90,0.75)')
      gr.addColorStop(1, 'rgba(255,120,60,0)')
      ctx.globalAlpha = on ? 1 : 0.12
      ctx.strokeStyle = gr
      ctx.lineWidth = Math.max(1, rad * 0.9)
      ctx.beginPath()
      ctx.moveTo(b.sx, b.sy)
      ctx.lineTo(back.sx, back.sy)
      ctx.stroke()
      ctx.lineWidth = 1
    }
    // an irregular rock that tumbles
    const spin = t * (0.6 + Math.abs(m.vx))
    ctx.globalAlpha = on ? 1 : 0.14
    ctx.fillStyle = '#8f7b68'
    ctx.beginPath()
    for (let k = 0; k < 7; k++) {
      const a = spin + (k / 7) * Math.PI * 2
      const rr = rad * (0.75 + 0.3 * Math.sin(k * 2.7 + m.vz * 5))
      if (k) ctx.lineTo(b.sx + Math.cos(a) * rr, b.sy + Math.sin(a) * rr)
      else ctx.moveTo(b.sx + Math.cos(a) * rr, b.sy + Math.sin(a) * rr)
    }
    ctx.closePath()
    ctx.fill()
    ctx.fillStyle = 'rgba(255,230,200,0.35)'
    ctx.beginPath()
    ctx.arc(b.sx - rad * 0.25, b.sy - rad * 0.25, rad * 0.3, 0, Math.PI * 2)
    ctx.fill()
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
      const rad = Math.max(b.kind === 'planet' ? 3 : 1.2, b.sr) + 5
      if (d < rad && d - (b.kind === 'planet' ? 2 : 0) < bd) {
        bd = d
        best = b
      }
    }
    return best
  }

  private hoverAt(e: PointerEvent) {
    const tip = this.o.tip
    if (this.streakAt(e)) {
      const l = this.streak!.lead
      this.hover = 'streak:' + l.id
      this.o.stage.style.cursor = 'pointer'
      const r = this.o.stage.getBoundingClientRect()
      tip.style.left = Math.min(r.width - 250, e.clientX - r.left + 14) + 'px'
      tip.style.top = e.clientY - r.top + 14 + 'px'
      tip.innerHTML = `<b>${esc(l.name)}</b><div class="r"><span>${esc(l.sub)}</span></div><div class="r"><span>Komet · prospek</span><span>belum pernah order</span></div><div class="r"><span>Sales</span><span>${l.ownerName ? esc(l.ownerName) : 'belum ada'}</span></div><div class="r" style="margin-top:4px;opacity:.7"><span>Klik untuk buka</span></div>`
      tip.classList.add('show')
      return
    }
    const b = this.pickAt(e)
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
    if (b.lead) {
      const l = b.lead
      tip.innerHTML = `<b>${esc(l.name)}</b><div class="r"><span>${esc(l.sub)}</span></div><div class="r"><span>Meteor · prospek tanpa sales</span><span>belum pernah order</span></div><div class="r"><span>Sales</span><span>${l.ownerName ? esc(l.ownerName) : 'belum ada'}</span></div><div class="r" style="margin-top:4px;opacity:.7"><span>Klik dua kali untuk buka</span></div>`
    } else if (b.d) tip.innerHTML = tipHtml(b.d, this.edges, (id) => this.byId.get(id)?.name ?? id, this.o.periodLabel, this.o.monthLabels)
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

/** The sky: stars of different size, colour and brightness (some twinkle), and a faint Milky Way band. */
function makeStars(): Star[] {
  const rnd = rng('stars')
  const tints = ['#ffffff', '#dfe8ff', '#cfe0ff', '#fff4e0', '#ffe9c8', '#d8f0ff']
  const out: Star[] = []
  const add = (x: number, y: number, z: number, faint: boolean) => {
    const r = rnd()
    const size = faint ? 0.8 + rnd() * 0.5 : r < 0.8 ? 0.8 + rnd() * 0.5 : r < 0.985 ? 1.4 + rnd() * 0.6 : 2.4 + rnd() * 1.2
    out.push({ x, y, z, size, b: faint ? 0.18 + rnd() * 0.3 : 0.5 + rnd() * 0.5, tint: tints[Math.floor(rnd() * tints.length)], tw: !faint && rnd() < 0.3 ? 1 + rnd() * 3 : 0 })
  }
  for (let i = 0; i < 14000; i++) {
    const u = rnd() * 2 - 1
    const th = rnd() * Math.PI * 2
    const q = Math.sqrt(1 - u * u)
    add(q * Math.cos(th), u, q * Math.sin(th), false)
  }
  // Milky Way: many faint stars close to one tilted great circle
  const tilt = 0.9
  for (let i = 0; i < 9000; i++) {
    const a = rnd() * Math.PI * 2
    const spread = (rnd() + rnd() + rnd() - 1.5) * 0.16
    let x = Math.cos(a)
    let y = spread
    let z = Math.sin(a)
    const y2 = y * Math.cos(tilt) - z * Math.sin(tilt)
    z = y * Math.sin(tilt) + z * Math.cos(tilt)
    y = y2
    const n = Math.hypot(x, y, z)
    x /= n
    add(x, y / n, z / n, true)
  }
  return out
}
