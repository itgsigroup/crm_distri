import * as THREE from 'three'
import type { RelasiEdge, RelasiNode } from '../../api/types'
import { createLayout, reweight, step, type Layout, type Positions } from './layout'

// Peta relasi engine — the mockup's createNet in TypeScript: three.js spheres + cylinders (or a canvas 2D
// projection when WebGL is unavailable), DOM labels, tooltip, drag to rotate, wheel to zoom, click to focus,
// click the focused dealer again (or double-click) to open it.

export interface NetOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  labels: HTMLElement
  tip: HTMLElement
  monthLabels: string[]
  periodLabel: string
  onOpen: (dealerId: string) => void
  onFocus?: (id: string | null) => void
}

type Key = 'accent' | 'good' | 'warn' | 'bad' | 'neutral'

interface VNode {
  d: RelasiNode
  i: number
  label: HTMLDivElement
  mesh?: THREE.Mesh
}

const css = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()

function webgl(canvas: HTMLCanvasElement): THREE.WebGLRenderer | null {
  try {
    return new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true })
  } catch {
    return null
  }
}

export class NetView {
  private o: NetOptions
  private l: Layout
  private nodes: VNode[] = []
  private edges: RelasiEdge[] = []
  private byId = new Map<string, VNode>()
  private springMeshes: THREE.Mesh[] = []
  private renderer: THREE.WebGLRenderer | null
  private ctx: CanvasRenderingContext2D | null = null
  private scene?: THREE.Scene
  private camera?: THREE.PerspectiveCamera
  private col: Record<Key | 'line' | 'bg', string> = { accent: '', good: '', warn: '', bad: '', neutral: '', line: '', bg: '' }
  private yaw = 0.6
  private pitch = 0.32
  private dist = 40
  private maxDist = 70
  private auto: boolean
  private reduced: boolean
  private raf = 0
  private hover: string | null = null
  focus: string | null = null
  filter: (id: string) => boolean = () => true
  private w = 0
  private h = 0
  private off: (() => void)[] = []
  readonly is3D: boolean

  constructor(o: NetOptions, nodes: RelasiNode[], edges: RelasiEdge[], months: number, settled?: Positions) {
    this.o = o
    this.edges = edges
    this.reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.auto = !this.reduced
    this.l = createLayout({ nodes, edges, months })
    if (settled) this.l.nodes.forEach((n, i) => { [n.x, n.y, n.z] = settled[i] ?? [n.x, n.y, n.z] })
    this.renderer = webgl(o.canvas)
    this.is3D = !!this.renderer
    nodes.forEach((d, i) => {
      // createLayout orders sales first, then dealers; map by id
      const idx = this.l.nodes.findIndex((n) => n.id === d.id)
      const label = document.createElement('div')
      label.className = 'net-lbl' + (d.type === 'sales' ? ' sales' : '')
      label.innerHTML = `${esc(d.name)}<small>${esc(d.sub)}</small>`
      o.labels.appendChild(label)
      const v: VNode = { d, i: idx >= 0 ? idx : i, label }
      this.nodes.push(v)
      this.byId.set(d.id, v)
    })
    if (this.renderer) {
      this.renderer.setPixelRatio(Math.min(devicePixelRatio, 2))
      this.scene = new THREE.Scene()
      this.camera = new THREE.PerspectiveCamera(42, 1, 0.1, 4000)
      this.scene.add(new THREE.HemisphereLight(0xffffff, 0x8899aa, 1.1))
      const dl = new THREE.DirectionalLight(0xffffff, 0.55)
      dl.position.set(4, 8, 6)
      this.scene.add(dl)
      const geo = new THREE.SphereGeometry(1, 28, 20)
      const cyl = new THREE.CylinderGeometry(1, 1, 1, 8, 1, true)
      for (const v of this.nodes) {
        const m = new THREE.Mesh(geo, new THREE.MeshStandardMaterial({ color: 0xffffff, roughness: 0.55, metalness: 0.05, transparent: true }))
        m.userData.id = v.d.id
        v.mesh = m
        this.scene.add(m)
      }
      for (let k = 0; k < this.l.springs.length; k++) {
        const m = new THREE.Mesh(cyl, new THREE.MeshBasicMaterial({ color: 0x888888, transparent: true, opacity: 0.3 }))
        this.springMeshes.push(m)
        this.scene.add(m)
      }
    } else {
      this.ctx = o.canvas.getContext('2d')
    }
    this.bind()
    this.theme()
    const mq = matchMedia('(prefers-color-scheme: dark)')
    const th = () => this.theme()
    mq.addEventListener('change', th)
    const mo = new MutationObserver(th)
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    this.off.push(() => mq.removeEventListener('change', th), () => mo.disconnect())
  }

  private key(v: VNode): Key {
    return (v.d.type === 'sales' ? 'accent' : v.d.tone) as Key
  }

  theme() {
    this.col = { accent: css('--accent'), good: css('--good'), warn: css('--warn'), bad: css('--bad'), neutral: css('--text-3'), line: css('--text-2'), bg: css('--surface') }
    if (this.renderer) {
      this.renderer.setClearColor(new THREE.Color(this.col.bg || '#ffffff'), 1)
      for (const v of this.nodes) (v.mesh!.material as THREE.MeshStandardMaterial).color.set(this.col[this.key(v)] || '#888')
      for (const m of this.springMeshes) (m.material as THREE.MeshBasicMaterial).color.set(this.col.line || '#888')
    }
  }

  /** New period: weights and sizes change, positions stay. */
  update(nodes: RelasiNode[], edges: RelasiEdge[], months: number, monthLabels: string[], periodLabel: string) {
    this.edges = edges
    this.o.monthLabels = monthLabels
    this.o.periodLabel = periodLabel
    for (const n of nodes) {
      const v = this.byId.get(n.id)
      if (v) v.d = n
    }
    reweight(this.l, edges, new Map(nodes.map((n) => [n.id, n.total])), months)
    this.focus = null
    this.theme()
  }

  private resize() {
    const w = this.o.stage.clientWidth
    const h = this.o.stage.clientHeight
    if (!w || !h) return
    if (this.renderer && this.camera) {
      this.renderer.setSize(w, h, false)
      this.camera.aspect = w / h
      this.camera.updateProjectionMatrix()
    } else {
      const r = Math.min(devicePixelRatio, 2)
      this.o.canvas.width = w * r
      this.o.canvas.height = h * r
    }
  }

  private connected(a: string, b: string) {
    return this.edges.some((e) => e.w > 0 && ((e.sales === a && e.dealer === b) || (e.dealer === a && e.sales === b)))
  }

  private isOn(id: string) {
    return this.filter(id) && (!this.focus || this.focus === id || this.connected(this.focus, id))
  }

  private project(i: number) {
    const n = this.l.nodes[i]
    const cy = Math.cos(this.yaw), sy = Math.sin(this.yaw), cp = Math.cos(this.pitch), sp = Math.sin(this.pitch)
    const x = n.x * cy - n.z * sy
    let z = n.x * sy + n.z * cy
    const y = n.y * cp - z * sp
    z = n.y * sp + z * cp
    const S = this.h / (2 * this.dist * 0.3839)
    const f = this.dist / (this.dist + z)
    return { sx: this.w / 2 + x * f * S, sy: this.h / 2 - y * f * S, f, S, z }
  }

  private placeLabel(v: VNode, sx: number, sy: number, vis: boolean, on: boolean, z: number) {
    const n = this.l.nodes[v.i]
    const show = on && vis && (v.d.type === 'sales' || n.tot >= 30 || this.hover === v.d.id || this.focus === v.d.id || (!!this.focus && on))
    v.label.style.opacity = show ? '1' : '0'
    v.label.style.left = sx + 'px'
    v.label.style.top = sy - (v.d.type === 'sales' ? 40 : 34) + 'px'
    v.label.style.zIndex = String(z)
  }

  private frame = () => {
    this.raf = requestAnimationFrame(this.frame)
    const st = this.o.stage
    if (!st.isConnected || st.offsetParent === null) return
    const w = st.clientWidth
    const h = st.clientHeight
    if (!w || !h) return
    if (this.w !== w || this.h !== h) {
      this.w = w
      this.h = h
      this.resize()
    }
    step(this.l, 0.6)
    if (this.auto) this.yaw += 0.0022
    const months = this.l.months
    if (this.renderer && this.camera && this.scene) {
      const cam = this.camera
      cam.position.set(Math.cos(this.yaw) * Math.cos(this.pitch) * this.dist, Math.sin(this.pitch) * this.dist, Math.sin(this.yaw) * Math.cos(this.pitch) * this.dist)
      cam.lookAt(0, 0, 0)
      const up = new THREE.Vector3(0, 1, 0)
      const v3 = new THREE.Vector3()
      for (const v of this.nodes) {
        const n = this.l.nodes[v.i]
        const m = v.mesh!
        m.position.set(n.x, n.y, n.z)
        m.scale.setScalar(n.r)
        const on = this.isOn(v.d.id)
        ;(m.material as THREE.MeshStandardMaterial).opacity = on ? 1 : 0.12
        v3.set(n.x, n.y + n.r + 0.15, n.z).project(cam)
        this.placeLabel(v, ((v3.x + 1) / 2) * w, ((1 - v3.y) / 2) * h, v3.z < 1, on, Math.round((1 - v3.z) * 1000))
      }
      this.l.springs.forEach((sp, k) => {
        const a = this.l.nodes[sp.a]
        const b = this.l.nodes[sp.b]
        const m = this.springMeshes[k]
        m.position.set((a.x + b.x) / 2, (a.y + b.y) / 2, (a.z + b.z) / 2)
        v3.set(b.x - a.x, b.y - a.y, b.z - a.z)
        const len = v3.length()
        const rr = 0.035 + Math.sqrt(sp.w / months) * 0.012
        m.scale.set(rr, len, rr)
        m.quaternion.setFromUnitVectors(up, v3.normalize())
        const on = this.filter(a.id) && this.filter(b.id) && (!this.focus || this.focus === a.id || this.focus === b.id)
        ;(m.material as THREE.MeshBasicMaterial).opacity = !sp.w ? 0 : on ? 0.18 + Math.min(0.7, sp.w / months / 120) : 0.03
      })
      this.renderer.render(this.scene, cam)
      return
    }
    const ctx = this.ctx
    if (!ctx) return
    const r = Math.min(devicePixelRatio, 2)
    ctx.setTransform(r, 0, 0, r, 0, 0)
    ctx.fillStyle = this.col.bg
    ctx.fillRect(0, 0, w, h)
    const P = this.l.nodes.map((_, i) => this.project(i))
    for (const sp of this.l.springs) {
      if (!sp.w) continue
      const a = P[sp.a]
      const b = P[sp.b]
      const ia = this.l.nodes[sp.a].id
      const ib = this.l.nodes[sp.b].id
      const on = this.filter(ia) && this.filter(ib) && (!this.focus || this.focus === ia || this.focus === ib)
      ctx.globalAlpha = on ? 0.18 + Math.min(0.7, sp.w / months / 120) : 0.04
      ctx.strokeStyle = this.col.line
      ctx.lineWidth = Math.max(1, (0.035 + Math.sqrt(sp.w / months) * 0.012) * 2 * a.S * a.f)
      ctx.beginPath()
      ctx.moveTo(a.sx, a.sy)
      ctx.lineTo(b.sx, b.sy)
      ctx.stroke()
    }
    ;[...this.nodes].sort((x, y) => P[y.i].z - P[x.i].z).forEach((v) => {
      const p = P[v.i]
      const n = this.l.nodes[v.i]
      const on = this.isOn(v.d.id)
      const rad = n.r * p.S * p.f
      ctx.globalAlpha = on ? 1 : 0.15
      ctx.fillStyle = this.col[this.key(v)]
      ctx.beginPath()
      ctx.arc(p.sx, p.sy, rad, 0, Math.PI * 2)
      ctx.fill()
      ctx.globalAlpha = on ? 0.35 : 0.05
      ctx.fillStyle = '#fff'
      ctx.beginPath()
      ctx.arc(p.sx - rad * 0.3, p.sy - rad * 0.3, rad * 0.35, 0, Math.PI * 2)
      ctx.fill()
      this.placeLabel(v, p.sx, p.sy - rad, true, on, Math.round(1000 - p.z * 10))
    })
    ctx.globalAlpha = 1
  }

  private pickAt(e: PointerEvent | MouseEvent): VNode | null {
    const r = this.o.stage.getBoundingClientRect()
    if (this.renderer && this.camera) {
      const m = new THREE.Vector2(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1)
      const rc = new THREE.Raycaster()
      rc.setFromCamera(m, this.camera)
      const hit = rc.intersectObjects(this.nodes.map((v) => v.mesh!))[0]
      return hit ? this.byId.get(hit.object.userData.id as string) ?? null : null
    }
    const mx = e.clientX - r.left
    const my = e.clientY - r.top
    let best: VNode | null = null
    let bd = 1e9
    for (const v of this.nodes) {
      const p = this.project(v.i)
      const d = Math.hypot(p.sx - mx, p.sy - my)
      const rad = this.l.nodes[v.i].r * p.S * p.f + 4
      if (d < rad && d < bd) {
        bd = d
        best = v
      }
    }
    return best
  }

  private hoverAt(e: PointerEvent) {
    const v = this.pickAt(e)
    const tip = this.o.tip
    this.hover = v ? v.d.id : null
    this.o.stage.style.cursor = v ? 'pointer' : ''
    if (!v) {
      tip.classList.remove('show')
      return
    }
    const r = this.o.stage.getBoundingClientRect()
    let px = e.clientX - r.left + 14
    const py = e.clientY - r.top + 14
    if (px + 240 > r.width) px -= 260
    tip.style.left = px + 'px'
    tip.style.top = py + 'px'
    const per = this.o.periodLabel
    const name = (id: string) => this.byId.get(id)?.d.name ?? id
    if (v.d.type === 'sales') {
      const es = this.edges.filter((e) => e.sales === v.d.id && e.w > 0).sort((a, b) => b.w - a.w)
      tip.innerHTML = `<b>${esc(v.d.name)} · ${esc(v.d.sub)}</b><div class="r"><span>Nomor</span><span>${esc(v.d.number ?? '')}</span></div><div class="r"><span>Kontak aktif</span><span>${es.length}</span></div><div class="r"><span>Pesan ${per}</span><span>${v.d.total}</span></div>${es[0] ? `<div class="r"><span>Terkuat</span><span>${esc(name(es[0].dealer))}</span></div>` : ''}`
    } else {
      const es = this.edges.filter((e) => e.dealer === v.d.id && e.w > 0).sort((a, b) => b.w - a.w)
      const series = this.edges.filter((e) => e.dealer === v.d.id).reduce((acc, e) => acc.map((x, i) => x + (e.monthly[i] ?? 0)), [0, 0, 0, 0, 0, 0])
      const ml = this.o.monthLabels
      tip.innerHTML = `<b>${esc(v.d.name)}</b><div class="r"><span>${esc(v.d.sub)}</span></div>${v.d.score != null ? `<div class="r"><span>Skor dealer</span><span>${v.d.score}</span></div>` : ''}<div class="r"><span>Pesan ${per}</span><span>${v.d.total}</span></div>${es.map((e) => `<div class="r"><span>↔ ${esc(name(e.sales))}</span><span>${e.w}</span></div>`).join('')}<div class="r" style="margin-top:4px;opacity:.75"><span>${ml[0]}→${ml[ml.length - 1]}</span><span>${series.join(' · ')}</span></div><div class="r" style="margin-top:4px;opacity:.7"><span>Klik dua kali untuk buka dealer</span></div>`
    }
    tip.classList.add('show')
  }

  private bind() {
    const cv = this.o.canvas
    let drag: { x: number; y: number; moved: boolean } | null = null
    let resume: ReturnType<typeof setTimeout> | undefined
    const down = (e: PointerEvent) => {
      drag = { x: e.clientX, y: e.clientY, moved: false }
      this.auto = false
      cv.setPointerCapture?.(e.pointerId)
    }
    const move = (e: PointerEvent) => {
      if (drag) {
        const dx = e.clientX - drag.x
        const dy = e.clientY - drag.y
        if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
        this.yaw += dx * 0.008
        this.pitch = Math.max(-1.2, Math.min(1.2, this.pitch + dy * 0.006))
        drag.x = e.clientX
        drag.y = e.clientY
      } else this.hoverAt(e)
    }
    const up = (e: PointerEvent) => {
      if (drag && !drag.moved) {
        const v = this.pickAt(e)
        if (v && v.d.type === 'dealer' && this.focus === v.d.id) this.o.onOpen(v.d.id)
        else {
          this.focus = v ? v.d.id : null
          this.o.onFocus?.(this.focus)
        }
      }
      drag = null
      clearTimeout(resume)
      resume = setTimeout(() => {
        if (!drag && !this.reduced) this.auto = true
      }, 4000)
    }
    const dbl = (e: MouseEvent) => {
      const v = this.pickAt(e)
      if (v && v.d.type === 'dealer') this.o.onOpen(v.d.id)
    }
    const leave = () => {
      this.o.tip.classList.remove('show')
      this.hover = null
    }
    const wheel = (e: WheelEvent) => {
      e.preventDefault()
      this.dist = Math.max(10, Math.min(this.maxDist, this.dist + e.deltaY * 0.03 * (this.maxDist / 70)))
    }
    cv.addEventListener('pointerdown', down)
    cv.addEventListener('pointermove', move)
    cv.addEventListener('pointerup', up)
    cv.addEventListener('dblclick', dbl)
    cv.addEventListener('pointerleave', leave)
    cv.addEventListener('wheel', wheel, { passive: false })
    this.off.push(() => {
      cv.removeEventListener('pointerdown', down)
      cv.removeEventListener('pointermove', move)
      cv.removeEventListener('pointerup', up)
      cv.removeEventListener('dblclick', dbl)
      cv.removeEventListener('pointerleave', leave)
      cv.removeEventListener('wheel', wheel)
      clearTimeout(resume)
    })
  }

  /** Camera distance that shows the whole network (real teams draw hundreds of dealers, the mockup 22). */
  private fit() {
    const ns = this.l.nodes
    if (!ns.length) return
    const cx = ns.reduce((a, n) => a + n.x, 0) / ns.length
    const cy = ns.reduce((a, n) => a + n.y, 0) / ns.length
    const cz = ns.reduce((a, n) => a + n.z, 0) / ns.length
    const d = ns.map((n) => Math.hypot(n.x - cx, n.y - cy, n.z - cz)).sort((a, b) => a - b)
    const r = d[Math.floor(d.length * 0.92)] ?? 10 // ignore a few far outliers
    this.dist = Math.max(40, r * 2.7)
    this.maxDist = Math.max(70, this.dist * 2)
  }

  start() {
    this.fit()
    this.theme()
    this.resize()
    if (!this.raf) this.frame()
  }

  destroy() {
    cancelAnimationFrame(this.raf)
    this.raf = 0
    this.off.forEach((f) => f())
    this.o.labels.innerHTML = ''
    this.renderer?.dispose()
  }
}

function esc(s: string) {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)
}
