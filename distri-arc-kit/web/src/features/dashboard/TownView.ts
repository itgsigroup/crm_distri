import type { Shop, TaskKind, TownPlan } from './town'

// Kota Distribusi: a small animated town drawn on a canvas. Two streets with dealer shops, the GSI warehouse and
// office on the left; trucks deliver parcels, the collector rides a scooter to collect, sales walk to follow up.
// Everything people do comes from the plan (real data); clicking a shop opens the dealer.

export type TownTarget = { kind: 'shop'; shop: Shop } | { kind: 'warehouse' } | { kind: 'office' }

export interface TownOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  plan: TownPlan
  /** aging stock lots (crates in the yard) and whether some SKU runs out soon */
  stock: { aging: number; critical: boolean }
  unread: number
  onPick: (t: TownTarget) => void
  onHover: (t: TownTarget | null, x: number, y: number) => void
}

export const TOWN_H = 432
const SKY = 86
const ROW_BASE = [176, 318] // bottom of the buildings on each street
const STREET = [197, 339] // street centre lines
const LINK_X = 166 // the side road between both streets
const SHOP_X0 = 196
const DEPOT = { x: 86, y: STREET[0] }
const OFFICE_DOOR = { x: 86, y: ROW_BASE[1] + 8 }

const TONE: Record<Shop['tone'], string> = { good: '#2fb36a', warn: '#e9a23b', bad: '#e5534b', neutral: '#8b95a7' }

type Pt = { x: number; y: number }
interface Actor {
  kind: TaskKind
  tasks: string[]
  next: number
  state: 'out' | 'act' | 'back' | 'rest'
  path: Pt[]
  dist: number
  timer: number
  speed: number
  shop: string
  color: string
}
interface Coin { x: number; y: number; vx: number; vy: number; life: number }

function along(path: Pt[], d: number): Pt & { dir: number } {
  let left = d
  for (let i = 1; i < path.length; i++) {
    const a = path[i - 1]
    const b = path[i]
    const seg = Math.hypot(b.x - a.x, b.y - a.y)
    if (left <= seg) {
      const t = seg ? left / seg : 0
      return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t, dir: b.x >= a.x ? 1 : -1 }
    }
    left -= seg
  }
  const e = path[path.length - 1]
  const p = path[path.length - 2] ?? e
  return { ...e, dir: e.x >= p.x ? 1 : -1 }
}
const lengthOf = (path: Pt[]) => path.slice(1).reduce((a, b, i) => a + Math.hypot(b.x - path[i].x, b.y - path[i].y), 0)

/** Hour in WIB (UTC+7). */
const wibHour = () => (new Date().getUTCHours() + 7) % 24

export class TownView {
  private o: TownOptions
  private ctx: CanvasRenderingContext2D | null
  private w = 0
  private raf = 0
  private last = 0
  private t = 0
  private actors: Actor[] = []
  private coins: Coin[] = []
  private clouds: { x: number; y: number; s: number; v: number }[] = []
  private hits: { x: number; y: number; w: number; h: number; t: TownTarget }[] = []
  private hover: string | null = null
  private reduced: boolean
  private off: (() => void)[] = []

  constructor(o: TownOptions) {
    this.o = o
    this.ctx = o.canvas.getContext('2d')
    this.reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.clouds = [0.1, 0.35, 0.62, 0.85].map((x, i) => ({ x, y: 18 + (i % 2) * 22, s: 0.8 + (i % 3) * 0.25, v: 0.006 + i * 0.002 }))
    this.setPlan(o.plan)
    const pos = (e: MouseEvent) => {
      const r = o.canvas.getBoundingClientRect()
      return { x: e.clientX - r.left, y: e.clientY - r.top }
    }
    const hit = (p: Pt) => [...this.hits].reverse().find((h) => p.x >= h.x && p.x <= h.x + h.w && p.y >= h.y && p.y <= h.y + h.h)?.t ?? null
    const move = (e: MouseEvent) => {
      const p = pos(e)
      const t = hit(p)
      this.hover = t ? (t.kind === 'shop' ? t.shop.id : t.kind) : null
      o.canvas.style.cursor = t ? 'pointer' : ''
      o.onHover(t, p.x, p.y)
    }
    const leave = () => { this.hover = null; o.onHover(null, 0, 0) }
    const click = (e: MouseEvent) => { const t = hit(pos(e)); if (t) o.onPick(t) }
    o.canvas.addEventListener('mousemove', move)
    o.canvas.addEventListener('mouseleave', leave)
    o.canvas.addEventListener('click', click)
    this.off.push(() => {
      o.canvas.removeEventListener('mousemove', move)
      o.canvas.removeEventListener('mouseleave', leave)
      o.canvas.removeEventListener('click', click)
    })
  }

  /** New data: shops and the town's to-do lists; people already on the way finish their trip. */
  setPlan(plan: TownPlan) {
    this.o.plan = plan
    const mk = (kind: TaskKind, n: number, speed: number, color: string, offset: number): Actor[] => {
      const list = plan.tasks[kind].map((t) => t.shop)
      return Array.from({ length: Math.min(n, list.length) }, (_, i) => ({ kind, tasks: list, next: i, state: 'rest' as const, path: [], dist: 0, timer: offset + i * 1.6, speed, shop: '', color }))
    }
    this.actors = [...mk('deliver', 2, 70, '#3b6ef5', 0.2), ...mk('collect', 1, 95, '#e5534b', 1.1), ...mk('visit', 2, 34, '#7c5cf0', 0.6)]
  }

  setStock(stock: TownOptions['stock'], unread: number) {
    this.o.stock = stock
    this.o.unread = unread
  }

  private shopBox(s: Shop) {
    const w = s.look === 'key' ? 92 : 78
    const span = Math.max(0, this.w - 22 - SHOP_X0 - 92)
    const cx = SHOP_X0 + 46 + s.slot * span
    return { x: cx - w / 2, w, h: s.look === 'key' ? 60 : 50, base: ROW_BASE[s.row], cx }
  }

  /** Route from the depot (vehicles) or the office (people) to the shop's door, along the streets. */
  private route(kind: TaskKind, s: Shop): Pt[] {
    const { cx } = this.shopBox(s)
    if (kind === 'visit') { // walkers use the pavement in front of the shops
      const walk = [ROW_BASE[0] + 8, ROW_BASE[1] + 8]
      return s.row === 1 ? [OFFICE_DOOR, { x: cx, y: walk[1] }] : [OFFICE_DOOR, { x: LINK_X, y: walk[1] }, { x: LINK_X, y: walk[0] }, { x: cx, y: walk[0] }]
    }
    return s.row === 0 ? [DEPOT, { x: cx, y: STREET[0] }] : [DEPOT, { x: LINK_X, y: STREET[0] }, { x: LINK_X, y: STREET[1] }, { x: cx, y: STREET[1] }]
  }

  private step(dt: number) {
    for (const c of this.clouds) c.x = (c.x + c.v * dt + 1.2) % 1.2
    for (const a of this.actors) {
      if (a.state === 'rest') {
        a.timer -= dt
        if (a.timer > 0) continue
        const shop = this.o.plan.shops.find((s) => s.id === a.tasks[a.next % a.tasks.length])
        a.next += 1
        if (!shop) { a.timer = 1; continue }
        a.shop = shop.id
        a.path = this.route(a.kind, shop)
        a.dist = 0
        a.state = 'out'
      } else if (a.state === 'out' || a.state === 'back') {
        a.dist += a.speed * dt
        if (a.dist >= lengthOf(a.path)) {
          a.dist = lengthOf(a.path)
          if (a.state === 'out') { a.state = 'act'; a.timer = 2.4 }
          else { a.state = 'rest'; a.timer = 1.4; if (a.kind === 'collect') this.burst(DEPOT.x, DEPOT.y - 40) }
        }
      } else if (a.state === 'act') {
        a.timer -= dt
        if (a.timer <= 0) { a.state = 'back'; a.path = [...a.path].reverse(); a.dist = 0 }
      }
    }
    for (const c of this.coins) { c.x += c.vx * dt; c.y += c.vy * dt; c.vy += 160 * dt; c.life -= dt }
    this.coins = this.coins.filter((c) => c.life > 0)
  }

  private burst(x: number, y: number) {
    for (let i = 0; i < 6; i++) this.coins.push({ x, y, vx: (i - 2.5) * 22, vy: -120 - (i % 3) * 30, life: 1.1 })
  }

  private frame = (now: number) => {
    this.raf = requestAnimationFrame(this.frame)
    const st = this.o.stage
    if (!st.isConnected || st.getClientRects().length === 0 || !this.ctx) return
    const w = st.clientWidth
    if (!w) return
    const dpr = Math.min(devicePixelRatio, 2)
    if (w !== this.w) {
      this.w = w
      this.o.canvas.width = w * dpr
      this.o.canvas.height = TOWN_H * dpr
      this.o.canvas.style.height = TOWN_H + 'px'
    }
    const dt = this.last ? Math.min(0.05, (now - this.last) / 1000) : 0
    this.last = now
    if (!this.reduced) { this.t += dt; this.step(dt) }
    this.draw(dpr)
  }

  private draw(dpr: number) {
    const ctx = this.ctx!
    const w = this.w
    const h = TOWN_H
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    this.hits = []
    const hour = wibHour()
    const night = hour >= 18 || hour < 6
    const dusk = hour === 17 || hour === 6

    // sky, sun or moon, stars, clouds
    const sky = ctx.createLinearGradient(0, 0, 0, SKY + 30)
    if (night) { sky.addColorStop(0, '#0e1633'); sky.addColorStop(1, '#2a3566') }
    else if (dusk) { sky.addColorStop(0, '#f6a96b'); sky.addColorStop(1, '#ffd9a8') }
    else { sky.addColorStop(0, '#7ec8f5'); sky.addColorStop(1, '#cfeefd') }
    ctx.fillStyle = sky
    ctx.fillRect(0, 0, w, SKY + 30)
    if (night) {
      ctx.fillStyle = '#fff'
      for (let i = 0; i < 40; i++) { const x = (i * 97.3) % w; const y = (i * 37.7) % SKY; ctx.globalAlpha = 0.4 + 0.4 * Math.sin(this.t * 2 + i); ctx.fillRect(x, y, 1.5, 1.5) }
      ctx.globalAlpha = 1
      ctx.fillStyle = '#f4f1d0'
      ctx.beginPath(); ctx.arc(w - 60, 34, 14, 0, Math.PI * 2); ctx.fill()
      ctx.fillStyle = '#0e1633'
      ctx.beginPath(); ctx.arc(w - 54, 30, 12, 0, Math.PI * 2); ctx.fill()
    } else {
      ctx.fillStyle = dusk ? '#ff8a3d' : '#ffd43b'
      ctx.beginPath(); ctx.arc(w - 60, 36, 16, 0, Math.PI * 2); ctx.fill()
    }
    for (const c of this.clouds) this.cloud(ctx, c.x * w - 60, c.y, c.s, night)

    // ground
    ctx.fillStyle = night ? '#2f5a3a' : '#8fd18a'
    ctx.fillRect(0, SKY, w, h - SKY)
    // streets and the side road
    for (const y of STREET) this.street(ctx, 150, y, w, night)
    ctx.fillStyle = night ? '#3a3f4b' : '#5b6170'
    ctx.fillRect(LINK_X - 13, STREET[0], 26, STREET[1] - STREET[0])
    // pavements
    ctx.fillStyle = night ? '#6b6f78' : '#d9d4c7'
    for (const b of ROW_BASE) ctx.fillRect(150, b, w - 150, 5)

    // warehouse (row 0) and office (row 1)
    this.warehouse(ctx, night)
    this.office(ctx, night)

    // shops, back row first
    for (const s of [...this.o.plan.shops].sort((a, b) => a.row - b.row)) this.shop(ctx, s, night)
    if (!this.o.plan.shops.length) {
      ctx.fillStyle = night ? '#e8ecf8' : '#1f2937'
      ctx.font = '600 13px var(--font-body, sans-serif)'
      ctx.textAlign = 'center'
      ctx.fillText('Belum ada dealer di kota — data dealer sedang dimuat atau kosong.', (w + 150) / 2, STREET[0] - 60)
    }

    // trees and lamps along the bottom
    for (let x = 30; x < w; x += 120) this.tree(ctx, x, h - 22, night)
    if (night) for (let x = 210; x < w; x += 160) for (const y of [ROW_BASE[0] + 3, ROW_BASE[1] + 3]) this.lamp(ctx, x, y)

    // the people at work
    for (const a of this.actors) this.actor(ctx, a)
    // coins
    for (const c of this.coins) {
      ctx.globalAlpha = Math.max(0, Math.min(1, c.life * 1.5))
      ctx.fillStyle = '#ffcf33'
      ctx.beginPath(); ctx.ellipse(c.x, c.y, 5, 6, 0, 0, Math.PI * 2); ctx.fill()
      ctx.fillStyle = '#e0a800'
      ctx.fillRect(c.x - 1, c.y - 3, 2, 6)
    }
    ctx.globalAlpha = 1
  }

  private cloud(ctx: CanvasRenderingContext2D, x: number, y: number, s: number, night: boolean) {
    ctx.fillStyle = night ? 'rgba(200,210,240,0.25)' : 'rgba(255,255,255,0.9)'
    for (const [dx, dy, r] of [[0, 8, 12], [14, 2, 16], [32, 8, 12], [20, 12, 12]] as const) {
      ctx.beginPath(); ctx.arc(x + dx * s, y + dy * s, r * s, 0, Math.PI * 2); ctx.fill()
    }
  }

  private street(ctx: CanvasRenderingContext2D, x0: number, y: number, w: number, night: boolean) {
    ctx.fillStyle = night ? '#3a3f4b' : '#5b6170'
    ctx.fillRect(x0, y - 16, w - x0, 32)
    ctx.strokeStyle = night ? '#c9c27a' : '#f3e7a1'
    ctx.lineWidth = 2
    ctx.setLineDash([12, 10])
    ctx.lineDashOffset = 0
    ctx.beginPath(); ctx.moveTo(x0, y); ctx.lineTo(w, y); ctx.stroke()
    ctx.setLineDash([])
  }

  private box(ctx: CanvasRenderingContext2D, x: number, base: number, w: number, h: number, wall: string, roof: string) {
    // wall
    ctx.fillStyle = wall
    ctx.fillRect(x, base - h, w, h)
    // roof (slanted, a little wider)
    ctx.fillStyle = roof
    ctx.beginPath()
    ctx.moveTo(x - 6, base - h)
    ctx.lineTo(x + w + 6, base - h)
    ctx.lineTo(x + w - 4, base - h - 14)
    ctx.lineTo(x + 4, base - h - 14)
    ctx.closePath()
    ctx.fill()
    ctx.fillStyle = 'rgba(0,0,0,0.12)'
    ctx.fillRect(x, base - 4, w, 4)
  }

  private label(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, bg: string, fg = '#fff') {
    ctx.font = '700 10px var(--font-body, sans-serif)'
    const tw = Math.min(ctx.measureText(text).width, 140)
    ctx.fillStyle = bg
    const r = 4
    const bx = x - tw / 2 - 5
    ctx.beginPath()
    ctx.roundRect?.(bx, y - 10, tw + 10, 14, r)
    if (!ctx.roundRect) ctx.rect(bx, y - 10, tw + 10, 14)
    ctx.fill()
    ctx.fillStyle = fg
    ctx.textAlign = 'center'
    ctx.fillText(text, x, y + 1, 140)
  }

  private warehouse(ctx: CanvasRenderingContext2D, night: boolean) {
    const x = 14
    const base = ROW_BASE[0]
    const w = 128
    const h = 66
    const hot = this.hover === 'warehouse'
    this.box(ctx, x, base, w, h, night ? '#7a6a58' : '#c9a77c', night ? '#5a3a2a' : '#9c4f2e')
    // big door
    ctx.fillStyle = night ? '#3b3530' : '#6b5440'
    ctx.fillRect(x + 40, base - 40, 48, 40)
    ctx.strokeStyle = 'rgba(0,0,0,0.25)'
    for (let yy = base - 36; yy < base; yy += 6) { ctx.beginPath(); ctx.moveTo(x + 40, yy); ctx.lineTo(x + 88, yy); ctx.stroke() }
    this.label(ctx, 'GUDANG GSI', x + w / 2, base - h + 12, '#1f2937')
    // crates of aging stock in the yard
    const n = Math.min(9, this.o.stock.aging)
    for (let i = 0; i < n; i++) {
      const cx = x + 4 + (i % 3) * 12
      const cy = base - 10 - Math.floor(i / 3) * 10
      ctx.fillStyle = '#c58b4a'; ctx.fillRect(cx, cy, 11, 9)
      ctx.strokeStyle = '#8a5a2b'; ctx.strokeRect(cx + 0.5, cy + 0.5, 10, 8)
    }
    if (this.o.stock.critical) this.alert(ctx, x + w - 10, base - h - 22, '#e5534b')
    if (hot) { ctx.strokeStyle = '#fff'; ctx.lineWidth = 2; ctx.strokeRect(x - 3, base - h - 17, w + 6, h + 19); ctx.lineWidth = 1 }
    this.hits.push({ x, y: base - h - 16, w, h: h + 16, t: { kind: 'warehouse' } })
  }

  private office(ctx: CanvasRenderingContext2D, night: boolean) {
    const x = 24
    const base = ROW_BASE[1]
    const w = 110
    const h = 56
    this.box(ctx, x, base, w, h, night ? '#56637a' : '#e8edf5', night ? '#2b3a55' : '#3b6ef5')
    for (let i = 0; i < 4; i++) {
      ctx.fillStyle = night ? '#ffe28a' : '#9fc3ff'
      ctx.fillRect(x + 10 + i * 24, base - h + 14, 16, 12)
    }
    ctx.fillStyle = night ? '#2d2a26' : '#5b6170'
    ctx.fillRect(x + w / 2 - 9, base - 22, 18, 22)
    this.label(ctx, 'KANTOR SALES', x + w / 2, base - h + 8, '#3b6ef5')
    if (this.o.unread > 0) { // a ringing phone: unread chats
      const ring = Math.sin(this.t * 14) * 2
      this.label(ctx, `${this.o.unread} chat`, x + w - 4 + ring, base - h - 22, '#2fb36a')
    }
    if (this.hover === 'office') { ctx.strokeStyle = '#fff'; ctx.lineWidth = 2; ctx.strokeRect(x - 3, base - h - 17, w + 6, h + 19); ctx.lineWidth = 1 }
    this.hits.push({ x, y: base - h - 16, w, h: h + 16, t: { kind: 'office' } })
  }

  private shop(ctx: CanvasRenderingContext2D, s: Shop, night: boolean) {
    const b = this.shopBox(s)
    const closed = s.look === 'closed'
    const risk = s.look === 'risk'
    const wall = closed ? (night ? '#4b4f57' : '#b8bcc4') : risk ? (night ? '#7b6f60' : '#e6dccb') : night ? '#8a7f70' : '#fff6e5'
    const roof = closed ? '#6b7280' : TONE[s.tone]
    this.box(ctx, b.x, b.base, b.w, b.h, wall, roof)
    // awning stripes
    if (!closed) for (let i = 0; i < b.w; i += 12) { ctx.fillStyle = i % 24 ? '#ffffff' : roof; ctx.fillRect(b.x + i, b.base - b.h, Math.min(12, b.w - i), 7) }
    // window and door
    const lit = night && !closed && !risk
    ctx.fillStyle = lit ? '#ffe28a' : closed ? '#6b7280' : risk ? '#a7b0be' : '#9fd3ff'
    ctx.fillRect(b.x + 8, b.base - b.h + 14, b.w * 0.42, b.h * 0.38)
    ctx.fillStyle = closed ? '#4b5563' : '#7a4b2a'
    ctx.fillRect(b.x + b.w - 26, b.base - 28, 16, 28)
    if (closed) { // boarded up
      ctx.strokeStyle = '#8a6a44'; ctx.lineWidth = 3
      ctx.beginPath(); ctx.moveTo(b.x + 6, b.base - b.h + 12); ctx.lineTo(b.x + b.w - 6, b.base - 6); ctx.moveTo(b.x + b.w - 6, b.base - b.h + 12); ctx.lineTo(b.x + 6, b.base - 6); ctx.stroke()
      ctx.lineWidth = 1
    }
    // name sign on the roof
    this.label(ctx, s.name.length > 18 ? s.name.slice(0, 17) + '…' : s.name, b.cx, b.base - b.h - 18, closed ? '#4b5563' : '#1f2937')
    if (s.look === 'key') this.star(ctx, b.x + b.w - 4, b.base - b.h - 30)
    if (risk) this.alert(ctx, b.x + 6, b.base - b.h - 32 + Math.sin(this.t * 4) * 2, '#e9a23b')
    if (s.dueIn != null && s.dueIn >= 0 && s.dueIn <= 14 && !closed) this.label(ctx, s.dueIn === 0 ? 'order hari ini' : `order ${s.dueIn} hr lagi`, b.cx, b.base - 34, '#2fb36a')
    if (this.hover === s.id) { ctx.strokeStyle = '#fff'; ctx.lineWidth = 2; ctx.strokeRect(b.x - 7, b.base - b.h - 28, b.w + 14, b.h + 30); ctx.lineWidth = 1 }
    this.hits.push({ x: b.x - 6, y: b.base - b.h - 28, w: b.w + 12, h: b.h + 28, t: { kind: 'shop', shop: s } })
  }

  private star(ctx: CanvasRenderingContext2D, x: number, y: number) {
    ctx.fillStyle = '#ffcf33'
    ctx.beginPath()
    for (let i = 0; i < 10; i++) {
      const r = i % 2 ? 4 : 9
      const a = (i / 10) * Math.PI * 2 - Math.PI / 2
      ctx.lineTo(x + Math.cos(a) * r, y + Math.sin(a) * r)
    }
    ctx.closePath()
    ctx.fill()
  }

  private alert(ctx: CanvasRenderingContext2D, x: number, y: number, color: string) {
    ctx.fillStyle = color
    ctx.beginPath(); ctx.arc(x, y, 8, 0, Math.PI * 2); ctx.fill()
    ctx.fillStyle = '#fff'
    ctx.font = '800 12px var(--font-body, sans-serif)'
    ctx.textAlign = 'center'
    ctx.fillText('!', x, y + 4)
  }

  private tree(ctx: CanvasRenderingContext2D, x: number, y: number, night: boolean) {
    ctx.fillStyle = '#7a5230'
    ctx.fillRect(x - 2, y - 4, 4, 12)
    ctx.fillStyle = night ? '#1f4a2c' : '#3e9a4f'
    ctx.beginPath(); ctx.arc(x, y - 10, 11, 0, Math.PI * 2); ctx.fill()
    ctx.fillStyle = night ? '#2a5c38' : '#57b866'
    ctx.beginPath(); ctx.arc(x - 4, y - 13, 6, 0, Math.PI * 2); ctx.fill()
  }

  private lamp(ctx: CanvasRenderingContext2D, x: number, y: number) {
    const g = ctx.createRadialGradient(x, y - 22, 0, x, y - 22, 26)
    g.addColorStop(0, 'rgba(255,226,138,0.55)')
    g.addColorStop(1, 'rgba(255,226,138,0)')
    ctx.fillStyle = g
    ctx.fillRect(x - 26, y - 48, 52, 52)
    ctx.fillStyle = '#2d2a26'
    ctx.fillRect(x - 1, y - 22, 2, 22)
    ctx.fillStyle = '#ffe28a'
    ctx.beginPath(); ctx.arc(x, y - 23, 3, 0, Math.PI * 2); ctx.fill()
  }

  private actor(ctx: CanvasRenderingContext2D, a: Actor) {
    if (a.state === 'rest' || !a.path.length) return
    const p = along(a.path, a.dist)
    const lane = a.kind === 'visit' ? 0 : a.state === 'back' ? 7 : -7
    const x = p.x
    const y = p.y + lane
    const moving = a.state !== 'act'
    if (a.kind === 'deliver') this.truck(ctx, x, y, p.dir, a.state === 'out' || (a.state === 'act' && a.timer > 1.2))
    else if (a.kind === 'collect') this.scooter(ctx, x, y, p.dir, a.color, a.state === 'back')
    else this.person(ctx, x, y, p.dir, a.color, moving)
    if (a.state === 'act') {
      const shop = this.o.plan.shops.find((s) => s.id === a.shop)
      const text = a.kind === 'deliver' ? `Antar paket · ${shop?.name ?? ''}` : a.kind === 'collect' ? `Tagih · ${shop?.name ?? ''}` : `Follow-up · ${shop?.name ?? ''}`
      this.label(ctx, text.length > 26 ? text.slice(0, 25) + '…' : text, x, y - 30, a.kind === 'deliver' ? '#3b6ef5' : a.kind === 'collect' ? '#e5534b' : '#7c5cf0')
      if (a.kind === 'deliver' && a.timer < 1.6) { // the parcel goes to the door
        const k = 1 - a.timer / 1.6
        ctx.fillStyle = '#c58b4a'
        ctx.fillRect(x - 5, y - 12 - k * 20, 10, 9)
      }
    }
  }

  private truck(ctx: CanvasRenderingContext2D, x: number, y: number, dir: number, loaded: boolean) {
    ctx.save()
    ctx.translate(x, y)
    ctx.scale(dir, 1)
    ctx.fillStyle = '#3b6ef5'
    ctx.fillRect(-18, -14, 24, 14)
    ctx.fillStyle = '#ffffff'
    ctx.fillRect(-16, -12, 20, 3)
    ctx.fillStyle = '#2b4fb8'
    ctx.fillRect(6, -10, 10, 10)
    ctx.fillStyle = '#bfe0ff'
    ctx.fillRect(9, -8, 5, 4)
    if (loaded) { ctx.fillStyle = '#c58b4a'; ctx.fillRect(-12, -21, 10, 7); ctx.fillRect(-1, -21, 7, 7) }
    ctx.fillStyle = '#1f2937'
    for (const wx of [-12, 10]) { ctx.beginPath(); ctx.arc(wx, 1, 3.5, 0, Math.PI * 2); ctx.fill() }
    ctx.restore()
  }

  private scooter(ctx: CanvasRenderingContext2D, x: number, y: number, dir: number, color: string, coin: boolean) {
    ctx.save()
    ctx.translate(x, y)
    ctx.scale(dir, 1)
    ctx.fillStyle = color
    ctx.fillRect(-9, -6, 18, 5)
    ctx.fillStyle = '#1f2937'
    for (const wx of [-7, 7]) { ctx.beginPath(); ctx.arc(wx, 1, 3, 0, Math.PI * 2); ctx.fill() }
    // rider
    ctx.fillStyle = '#374151'
    ctx.fillRect(-3, -15, 6, 9)
    ctx.fillStyle = '#f1c27d'
    ctx.beginPath(); ctx.arc(0, -18, 3.5, 0, Math.PI * 2); ctx.fill()
    ctx.fillStyle = color
    ctx.beginPath(); ctx.arc(0, -19, 3.8, Math.PI, 0); ctx.fill()
    if (coin) { ctx.fillStyle = '#ffcf33'; ctx.beginPath(); ctx.arc(-9, -12, 4, 0, Math.PI * 2); ctx.fill() }
    ctx.restore()
  }

  private person(ctx: CanvasRenderingContext2D, x: number, y: number, dir: number, color: string, moving: boolean) {
    const swing = moving ? Math.sin(this.t * 10) * 3 : 0
    ctx.strokeStyle = '#1f2937'
    ctx.lineWidth = 2
    ctx.beginPath(); ctx.moveTo(x, y - 6); ctx.lineTo(x - swing, y); ctx.moveTo(x, y - 6); ctx.lineTo(x + swing, y); ctx.stroke()
    ctx.lineWidth = 1
    ctx.fillStyle = color
    ctx.fillRect(x - 3, y - 14, 6, 9)
    ctx.fillStyle = '#f1c27d'
    ctx.beginPath(); ctx.arc(x, y - 17, 3.5, 0, Math.PI * 2); ctx.fill()
    // a folder in the hand, on the side it walks to
    ctx.fillStyle = '#ffffff'
    ctx.fillRect(x + dir * 3, y - 11, 4, 5)
  }

  start() {
    if (!this.raf) this.raf = requestAnimationFrame(this.frame)
  }

  destroy() {
    cancelAnimationFrame(this.raf)
    this.raf = 0
    this.off.forEach((f) => f())
  }
}
