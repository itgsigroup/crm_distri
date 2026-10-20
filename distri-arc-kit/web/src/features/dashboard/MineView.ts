import * as THREE from 'three'
import type { MinePlan, Site, TaskKind } from './mine'

// Tambang Distribusi: an open-pit mine in three.js. A terraced pit with a spiral haul road; every dealer is a dig
// site along the road where a miner swings a pickaxe. Excavators dig at dealers whose jadwal order is near, dump
// trucks drive down from the GSI warehouse to deliver, the red pickup collects (coins at the warehouse), surveyors
// walk to At risk dealers. Everything people do comes from the plan (real data); clicking a site opens the dealer.

export type MineTarget = { kind: 'site'; site: Site } | { kind: 'warehouse' } | { kind: 'office' }

export interface MineOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  /** aging stock lots (crates in the yard) and whether some SKU runs out soon */
  stock: { aging: number; critical: boolean }
  unread: number
  onPick: (t: MineTarget) => void
  onHover: (t: MineTarget | null, x: number, y: number) => void
}

export const MINE_H = 460

// the pit: an ellipse (x stretched by SX), benches stepping down from the rim to the floor
const SX = 1.5
const R_RIM = 11
const R_FLOOR = 2.6
const DEPTH = 5
const BENCHES = 5
const WALL = 0.5 // share of each bench that is wall
// the haul road: a spiral from just outside the rim (where the warehouse is) down to the floor
const TURNS = 1.25
const THETA0 = Math.PI
const R0 = R_RIM + 1.4
const R1 = 2.1
const SAMPLES = 480
const HALF = 0.85 // flat half width
const BLEND = 0.8 // cut/fill slope beside the road
const K = 1.35 // model scale
const LANE = { out: -0.2, back: -0.6, walk: 0.6 }

const COLOR = {
  deliver: '#f2a900', collect: '#e5534b', visit: '#7c5cf0',
  good: '#2fb36a', warn: '#e9a23b', bad: '#e5534b', neutral: '#8b95a7', gold: '#ffcf33',
}

const clamp01 = (v: number) => Math.max(0, Math.min(1, v))
const smooth = (e0: number, e1: number, v: number) => { const x = clamp01((v - e0) / (e1 - e0)); return x * x * (3 - 2 * x) }
const lerp = (a: number, b: number, k: number) => a + (b - a) * k

/** Terraced pit height at a normalised radius. */
function benchY(rn: number) {
  const k = clamp01((R_RIM - rn) / (R_RIM - R_FLOOR)) * BENCHES
  const i = Math.min(BENCHES, Math.floor(k))
  return -(i + smooth(0, WALL, k - i)) * (DEPTH / BENCHES)
}
/** Haul road height: a steady grade from the rim to the floor. */
const roadY = (r: number) => -DEPTH * clamp01((R_RIM - r) / (R_RIM - R_FLOOR))

/** Hour in WIB (UTC+7). */
const wibHour = () => (new Date().getUTCHours() + 7) % 24

/** Deterministic pseudo random, so the scenery is the same on every visit. */
function rng(seed: number) {
  let s = seed
  return () => { s = (s * 1664525 + 1013904223) % 4294967296; return s / 4294967296 }
}

function webgl(canvas: HTMLCanvasElement): THREE.WebGLRenderer | null {
  try {
    return new THREE.WebGLRenderer({ canvas, antialias: true })
  } catch {
    return null
  }
}

interface Worker { root: THREE.Group; arms: THREE.Group; legs: [THREE.Group, THREE.Group] }
interface Truck { root: THREE.Group; wheels: THREE.Mesh[]; bed: THREE.Group; load: THREE.Object3D }
interface Excavator { root: THREE.Group; turret: THREE.Group; boom: THREE.Group; stick: THREE.Group; bucket: THREE.Group; dirt: THREE.Mesh; phase: number; full: boolean }
interface SiteView { site: Site; s: number; pile: THREE.Vector3; miner?: Worker & { phase: number; prev: number }; exc?: Excavator; ring: THREE.Mesh }
interface Actor {
  kind: TaskKind
  truck?: Truck
  walker?: Worker
  obj: THREE.Group
  tasks: string[]
  next: number
  state: 'out' | 'act' | 'back' | 'rest'
  s: number
  goal: number
  timer: number
  speed: number
  site: string
  label?: THREE.Sprite
}
interface Bit { mesh: THREE.Mesh; v: THREE.Vector3; life: number }

// excavator cycle: [time, turret yaw, boom, stick, bucket]
const DIG: [number, number, number, number, number][] = [
  [0, 0, -0.6, 0.6, 0.3],
  [1.2, 0, 0.3, -0.6, 0.6],
  [2.4, 0, 0.2, 0.5, -1.1],
  [3.2, 0, -0.7, 0.5, -1.1],
  [4.4, 1.7, -0.7, 0.5, -1.1],
  [5.2, 1.7, -0.6, 0.2, 1.0],
  [6.4, 0, -0.6, 0.6, 0.3],
]
const DIG_T = 6.4

export class MineView {
  readonly ok: boolean
  private o: MineOptions
  private renderer: THREE.WebGLRenderer | null
  private scene = new THREE.Scene()
  private camera = new THREE.PerspectiveCamera(32, 2, 0.5, 400)
  private target = new THREE.Vector3(-3.5, -2.2, 0.8)
  private yaw = 0
  private pitch = 0.62
  private dist = 30
  private moved = false
  private geos = new Map<string, THREE.BufferGeometry>()
  private mats = new Map<string, THREE.Material>()
  private road: { p: THREE.Vector3[]; side: THREE.Vector3[]; cum: number[] } = { p: [], side: [], cum: [] }
  private sites = new Map<string, SiteView>()
  private siteGroup = new THREE.Group()
  private actorGroup = new THREE.Group()
  private actors: Actor[] = []
  private bits: Bit[] = []
  private coins: Bit[] = []
  private clouds: THREE.Group[] = []
  private hits: THREE.Object3D[] = []
  private hover: string | null = null
  private wh = { pos: new THREE.Vector3(), crates: new THREE.Group(), beacon: null as THREE.Mesh | null, ring: null as THREE.Mesh | null }
  private office = { pos: new THREE.Vector3(), chat: null as THREE.Sprite | null, chatText: '', windows: [] as THREE.Mesh[], ring: null as THREE.Mesh | null }
  private lights = { hemi: new THREE.HemisphereLight(0xffffff, 0x8a6a4a, 1), sun: new THREE.DirectionalLight(0xffffff, 1.6), towers: [] as THREE.PointLight[], heads: [] as THREE.Mesh[] }
  private night: boolean | null = null
  private clock = 0
  private t = 0
  private last = 0
  private raf = 0
  private w = 0
  private reduced: boolean
  private rand = rng(7)
  private off: (() => void)[] = []

  constructor(o: MineOptions) {
    this.o = o
    this.reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.renderer = webgl(o.canvas)
    this.ok = !!this.renderer
    if (!this.renderer) return
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 2))
    this.renderer.shadowMap.enabled = true
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap
    this.buildRoad()
    this.buildWorld()
    this.scene.add(this.siteGroup, this.actorGroup)
    this.setStock(o.stock, o.unread)
    this.bind()
  }

  // ---------- shared geometry and materials ----------

  private geo<T extends THREE.BufferGeometry>(key: string, make: () => T): T {
    let g = this.geos.get(key)
    if (!g) { g = make(); this.geos.set(key, g) }
    return g as T
  }
  private mat(color: string, extra: '' | 'flat' | 'glow' = ''): THREE.Material {
    const key = color + extra
    let m = this.mats.get(key)
    if (!m) {
      m = extra === 'glow' ? new THREE.MeshBasicMaterial({ color }) : new THREE.MeshLambertMaterial({ color, flatShading: extra === 'flat' })
      this.mats.set(key, m)
    }
    return m
  }
  private box(w: number, h: number, d: number, color: string, x = 0, y = 0, z = 0) {
    const m = new THREE.Mesh(this.geo(`b${w},${h},${d}`, () => new THREE.BoxGeometry(w, h, d)), this.mat(color))
    m.position.set(x, y, z)
    m.castShadow = true
    return m
  }
  private cyl(r: number, h: number, color: string, seg = 12) {
    const m = new THREE.Mesh(this.geo(`c${r},${h},${seg}`, () => new THREE.CylinderGeometry(r, r, h, seg)), this.mat(color))
    m.castShadow = true
    return m
  }
  private ball(r: number, color: string) {
    const m = new THREE.Mesh(this.geo(`s${r}`, () => new THREE.SphereGeometry(r, 14, 10)), this.mat(color))
    m.castShadow = true
    return m
  }

  private label(text: string, bg: string, fg = '#ffffff', h = 0.62) {
    const c = document.createElement('canvas')
    const g = c.getContext('2d')!
    const font = '700 56px "Plus Jakarta Sans", "Instrument Sans", system-ui, sans-serif'
    g.font = font
    const W = Math.ceil(g.measureText(text).width + 52)
    const H = 84
    c.width = W
    c.height = H
    g.font = font
    g.fillStyle = bg
    g.beginPath()
    if (g.roundRect) g.roundRect(0, 0, W, H, 20)
    else g.rect(0, 0, W, H)
    g.fill()
    g.fillStyle = fg
    g.textAlign = 'center'
    g.textBaseline = 'middle'
    g.fillText(text, W / 2, H / 2 + 3)
    const tex = new THREE.CanvasTexture(c)
    tex.colorSpace = THREE.SRGBColorSpace
    const s = new THREE.Sprite(new THREE.SpriteMaterial({ map: tex, depthTest: false, transparent: true }))
    s.scale.set((h * W) / H, h, 1)
    s.center.set(0.5, 0)
    s.renderOrder = 10
    return s
  }
  private dropSprite(s: THREE.Sprite | null | undefined) {
    if (!s) return
    s.removeFromParent()
    s.material.map?.dispose()
    s.material.dispose()
  }

  // ---------- the haul road and the terrain ----------

  private buildRoad() {
    const p: THREE.Vector3[] = []
    for (let i = 0; i <= SAMPLES; i++) {
      const t = i / SAMPLES
      const r = R0 - t * (R0 - R1)
      const a = THETA0 + t * TURNS * Math.PI * 2
      p.push(new THREE.Vector3(SX * r * Math.cos(a), roadY(r), r * Math.sin(a)))
    }
    const side = p.map((q, i) => {
      const n = p[Math.min(i + 1, SAMPLES)]
      const b = p[Math.max(i - 1, 0)]
      const s = new THREE.Vector3(-(n.z - b.z), 0, n.x - b.x).normalize()
      return s.x * q.x + s.z * q.z < 0 ? s.negate() : s // pointing out of the pit
    })
    const cum = [0]
    for (let i = 1; i <= SAMPLES; i++) cum.push(cum[i - 1] + p[i].distanceTo(p[i - 1]))
    this.road = { p, side, cum }
  }

  private sAt(t: number) { return this.road.cum[Math.round(clamp01(t) * SAMPLES)] }

  /** A point on the road `s` units from the warehouse, `lane` units to the outside; dir = the way up the s axis. */
  private roadAt(s: number, lane: number) {
    const { p, side, cum } = this.road
    let lo = 0
    let hi = SAMPLES
    while (hi - lo > 1) { const m = (lo + hi) >> 1; if (cum[m] <= s) lo = m; else hi = m }
    const k = clamp01((s - cum[lo]) / Math.max(1e-6, cum[hi] - cum[lo]))
    const pos = p[lo].clone().lerp(p[hi], k).addScaledVector(side[lo], lane)
    return { pos, dir: p[hi].clone().sub(p[lo]).normalize(), side: side[lo] }
  }

  /** Ground height and how much road is there (1 on the road, 0 away from it). */
  private ground(x: number, z: number) {
    const base = benchY(Math.hypot(x / SX, z))
    let best = Infinity
    let y = 0
    for (const q of this.road.p) {
      const d = (q.x - x) ** 2 + (q.z - z) ** 2
      if (d < best) { best = d; y = q.y }
    }
    const d = Math.sqrt(best)
    if (d <= HALF) return { y, road: 1 }
    if (d <= HALF + BLEND) return { y: lerp(y, base, smooth(0, 1, (d - HALF) / BLEND)), road: d < HALF + 0.12 ? 0.6 : 0 }
    return { y: base, road: 0 }
  }
  private yAt(x: number, z: number) { return this.ground(x, z).y }

  private buildWorld() {
    const sc = this.scene
    // terrain: a displaced grid, coloured by bench, wall, road and the grass around the pit
    const W = 60
    const D = 42
    const g = new THREE.PlaneGeometry(W, D, 240, 168)
    g.rotateX(-Math.PI / 2)
    const pos = g.attributes.position as THREE.BufferAttribute
    const roadness: number[] = []
    for (let i = 0; i < pos.count; i++) {
      const { y, road } = this.ground(pos.getX(i), pos.getZ(i))
      pos.setY(i, y)
      roadness.push(road)
    }
    g.computeVertexNormals()
    const nrm = g.attributes.normal as THREE.BufferAttribute
    const cols = new Float32Array(pos.count * 3)
    const c = new THREE.Color()
    const grass = new THREE.Color('#9cc27a')
    const grass2 = new THREE.Color('#86b065')
    const benchTop = new THREE.Color('#dcb98c')
    const benchLow = new THREE.Color('#a77b52')
    const wallTop = new THREE.Color('#b9804f')
    const wallLow = new THREE.Color('#7d5134')
    const roadC = new THREE.Color('#6f6355')
    const noise = rng(3)
    for (let i = 0; i < pos.count; i++) {
      const x = pos.getX(i)
      const y = pos.getY(i)
      const z = pos.getZ(i)
      const depth = clamp01(-y / DEPTH)
      const rn = Math.hypot(x / SX, z)
      if (roadness[i] >= 1) c.copy(roadC)
      else if (rn > R_RIM + 0.15 && y > -0.05) c.copy(grass).lerp(grass2, noise() * 0.8)
      else c.copy(wallTop).lerp(wallLow, depth).lerp(benchTop.clone().lerp(benchLow, depth), smooth(0.7, 0.95, nrm.getY(i)))
      if (roadness[i] > 0 && roadness[i] < 1) c.lerp(roadC, 0.5)
      c.offsetHSL(0, 0, (noise() - 0.5) * 0.04)
      cols.set([c.r, c.g, c.b], i * 3)
    }
    g.setAttribute('color', new THREE.BufferAttribute(cols, 3))
    this.geos.set('terrain', g)
    const terrainMat = new THREE.MeshLambertMaterial({ vertexColors: true })
    this.mats.set('terrain', terrainMat)
    const terrain = new THREE.Mesh(g, terrainMat)
    terrain.receiveShadow = true
    sc.add(terrain)
    // the land beyond the grid: a big plane with a hole where the grid (and the pit) is
    const far = new THREE.Mesh(this.geo('far', () => {
      const sh = new THREE.Shape().moveTo(-250, -250).lineTo(250, -250).lineTo(250, 250).lineTo(-250, 250).closePath()
      sh.holes.push(new THREE.Path().moveTo(-W / 2 + 0.1, -D / 2 + 0.1).lineTo(-W / 2 + 0.1, D / 2 - 0.1).lineTo(W / 2 - 0.1, D / 2 - 0.1).lineTo(W / 2 - 0.1, -D / 2 + 0.1).closePath())
      return new THREE.ShapeGeometry(sh).rotateX(-Math.PI / 2)
    }), this.mat('#93bb72'))
    far.position.y = -0.03
    far.receiveShadow = true
    sc.add(far)

    // water on the pit floor
    const pond = new THREE.Mesh(this.geo('pond', () => new THREE.CircleGeometry(1.1, 40).rotateX(-Math.PI / 2)), new THREE.MeshLambertMaterial({ color: '#4f9cc4', transparent: true, opacity: 0.85 }))
    this.mats.set('pond', pond.material)
    pond.scale.x = SX
    pond.position.set(0, -DEPTH + 0.03, 0)
    sc.add(pond)

    // light
    sc.add(this.lights.hemi)
    const sun = this.lights.sun
    sun.position.set(14, 24, 10)
    sun.castShadow = true
    sun.shadow.mapSize.set(2048, 2048)
    Object.assign(sun.shadow.camera, { left: -30, right: 30, top: 24, bottom: -24, near: 1, far: 70 })
    sun.shadow.bias = -0.0008
    sc.add(sun)

    this.buildWarehouse()
    this.buildOffice()

    // scenery: trees round the pit, light towers on the rim, clouds
    const r = rng(11)
    const near = (x: number, z: number) => Math.hypot(x - this.road.p[0].x, z - this.road.p[0].z) < 6
    for (let n = 0; n < 60; n++) {
      const a = r() * Math.PI * 2
      const rr = R_RIM + 2.2 + r() * 8
      const x = SX * rr * Math.cos(a)
      const z = rr * Math.sin(a)
      if (near(x, z) || Math.abs(x) > 29 || Math.abs(z) > 20) continue
      this.tree(x, z, 0.8 + r() * 0.6)
    }
    for (const a of [0.05, 1.25, 1.5, 1.8]) this.tower(SX * (R_RIM + 1) * Math.cos(a * Math.PI), (R_RIM + 1) * Math.sin(a * Math.PI))
    for (let i = 0; i < 5; i++) {
      const cl = new THREE.Group()
      for (const [dx, dy, s] of [[0, 0, 1.4], [1.5, 0.3, 1.1], [-1.4, 0.1, 1], [0.4, 0.7, 0.9]] as const) {
        const b = new THREE.Mesh(this.geo('cloud', () => new THREE.SphereGeometry(1, 12, 8)), this.mat('#ffffff', 'glow'))
        b.position.set(dx, dy, 0)
        b.scale.setScalar(s)
        cl.add(b)
      }
      cl.position.set(-40 + i * 18 + r() * 6, 11 + r() * 3, -18 - r() * 10)
      cl.scale.set(1.4, 0.7, 1)
      this.clouds.push(cl)
      sc.add(cl)
    }
  }

  private tree(x: number, z: number, s: number) {
    const t = new THREE.Group()
    const trunk = this.cyl(0.12, 0.6, '#7a5230', 6)
    trunk.position.y = 0.3
    const top = new THREE.Mesh(this.geo('cone', () => new THREE.ConeGeometry(0.6, 1.5, 7)), this.mat('#3e8f4c', 'flat'))
    top.position.y = 1.25
    top.castShadow = true
    t.add(trunk, top)
    t.position.set(x, this.yAt(x, z), z)
    t.scale.setScalar(s)
    this.scene.add(t)
  }

  private tower(x: number, z: number) {
    const g = new THREE.Group()
    const pole = this.cyl(0.07, 3.2, '#5b6170', 6)
    pole.position.y = 1.6
    const frame = this.box(0.9, 0.35, 0.12, '#374151', 0, 3.25, 0)
    g.add(pole, frame)
    for (const dx of [-0.28, 0, 0.28]) {
      const lamp = new THREE.Mesh(this.geo('lamp', () => new THREE.SphereGeometry(0.1, 8, 6)), new THREE.MeshBasicMaterial({ color: '#d7dbe2' }))
      lamp.position.set(dx, 3.25, 0.09)
      this.lights.heads.push(lamp)
      g.add(lamp)
    }
    const pl = new THREE.PointLight('#ffe9b0', 0, 22, 1.2)
    pl.position.set(0, 3.2, 0.6)
    this.lights.towers.push(pl)
    g.add(pl)
    g.position.set(x, this.yAt(x, z), z)
    g.lookAt(0, g.position.y, 0)
    this.scene.add(g)
  }

  /** Invisible box that the pointer hits. */
  private hitBox(w: number, h: number, d: number, target: MineTarget) {
    const m = new THREE.Mesh(this.geo(`b${w},${h},${d}`, () => new THREE.BoxGeometry(w, h, d)), this.mat('#ffffff'))
    m.visible = false
    m.userData.target = target
    this.hits.push(m)
    return m
  }
  private ring(r: number) {
    let mat = this.mats.get('ring')
    if (!mat) { mat = new THREE.MeshBasicMaterial({ color: '#ffffff', transparent: true, opacity: 0.85, depthTest: false }); this.mats.set('ring', mat) }
    const m = new THREE.Mesh(this.geo(`ring${r}`, () => new THREE.RingGeometry(r * 0.86, r, 40).rotateX(-Math.PI / 2)), mat)
    m.visible = false
    m.renderOrder = 5
    return m
  }

  private buildWarehouse() {
    const p0 = this.road.p[0]
    const g = new THREE.Group()
    g.position.set(p0.x - 2.6, 0, p0.z - 0.3)
    this.wh.pos.copy(g.position)
    g.add(this.box(3.2, 1.9, 3.8, '#c9a77c', 0, 0.95, 0))
    g.add(this.box(3.5, 0.18, 4.1, '#9c4f2e', 0, 1.98, 0))
    g.add(this.box(3.2, 0.42, 0.1, '#1f2937', 0, 2.3, 1.9)) // sign board
    g.add(this.box(0.08, 1.35, 2.0, '#6b5440', 1.61, 0.68, 0)) // big door towards the road
    for (let yy = 0.2; yy < 1.35; yy += 0.22) g.add(this.box(0.1, 0.03, 2.0, '#4f3d2e', 1.62, yy, 0))
    this.wh.crates.position.set(0.6, 0, 2.5)
    g.add(this.wh.crates)
    const beacon = new THREE.Mesh(this.geo('beacon', () => new THREE.SphereGeometry(0.18, 12, 8)), new THREE.MeshBasicMaterial({ color: '#e5534b' }))
    this.mats.set('beacon', beacon.material)
    beacon.position.set(1.3, 2.25, -1.6)
    beacon.visible = false
    this.wh.beacon = beacon
    g.add(beacon)
    const lbl = this.label('GUDANG GSI', '#1f2937', '#ffffff', 0.7)
    lbl.position.set(0, 2.7, 0)
    g.add(lbl)
    this.wh.ring = this.ring(3)
    this.wh.ring.position.set(0, 0.06, 0)
    const hb = this.hitBox(3.6, 2.6, 4.2, { kind: 'warehouse' })
    hb.position.y = 1.2
    g.add(this.wh.ring, hb)
    this.scene.add(g)
  }

  private buildOffice() {
    const p0 = this.road.p[0]
    const g = new THREE.Group()
    g.position.set(p0.x - 2.1, 0, p0.z + 4.2)
    this.office.pos.copy(g.position)
    g.add(this.box(2.0, 1.05, 2.9, '#3b6ef5', 0, 0.62, 0))
    g.add(this.box(2.2, 0.1, 3.1, '#e8edf5', 0, 1.2, 0))
    for (const z of [-0.9, 0, 0.9]) {
      const win = new THREE.Mesh(this.geo('win', () => new THREE.BoxGeometry(0.05, 0.36, 0.55)), new THREE.MeshBasicMaterial({ color: '#bfe0ff' }))
      win.position.set(1.01, 0.72, z)
      this.office.windows.push(win)
      g.add(win)
    }
    for (const [x, z] of [[-0.8, -1.2], [0.8, -1.2], [-0.8, 1.2], [0.8, 1.2]]) g.add(this.box(0.16, 0.12, 0.16, '#5b6170', x, 0.06, z))
    const lbl = this.label('KANTOR SALES', '#3b6ef5', '#ffffff', 0.6)
    lbl.position.set(0, 1.5, 0)
    g.add(lbl)
    this.office.ring = this.ring(2.3)
    this.office.ring.position.set(0, 0.06, 0)
    const hb = this.hitBox(2.3, 1.8, 3.2, { kind: 'office' })
    hb.position.y = 0.9
    g.add(this.office.ring, hb)
    this.scene.add(g)
  }

  // ---------- people and machines ----------

  private worker(vest: string, tool: 'pick' | 'clipboard'): Worker {
    const root = new THREE.Group()
    const legs: [THREE.Group, THREE.Group] = [new THREE.Group(), new THREE.Group()]
    legs.forEach((l, i) => {
      l.position.set(i ? 0.05 : -0.05, 0.2, 0)
      l.add(this.box(0.075, 0.2, 0.08, '#2f3b52', 0, -0.1, 0))
      root.add(l)
    })
    root.add(this.box(0.2, 0.22, 0.12, vest, 0, 0.31, 0))
    root.add(this.box(0.205, 0.035, 0.125, '#e8f0f8', 0, 0.33, 0)) // reflective stripe
    const head = this.ball(0.065, '#e0ac7a')
    head.position.y = 0.49
    const hat = new THREE.Mesh(this.geo('hat', () => new THREE.SphereGeometry(0.075, 12, 6, 0, Math.PI * 2, 0, Math.PI / 2)), this.mat('#ffcf33'))
    hat.position.y = 0.5
    hat.castShadow = true
    const brim = this.cyl(0.09, 0.012, '#ffcf33')
    brim.position.y = 0.505
    root.add(head, hat, brim)
    const arms = new THREE.Group()
    arms.position.set(0, 0.4, 0)
    arms.add(this.box(0.05, 0.19, 0.05, vest, -0.12, -0.09, 0), this.box(0.05, 0.19, 0.05, vest, 0.12, -0.09, 0))
    if (tool === 'pick') {
      const handle = this.cyl(0.013, 0.36, '#8a5a2b', 6)
      handle.position.set(0, -0.32, 0)
      arms.add(handle, this.box(0.04, 0.04, 0.3, '#6b7280', 0, -0.48, 0.02))
    } else {
      arms.add(this.box(0.16, 0.2, 0.02, '#ffffff', 0, -0.17, 0.08))
    }
    root.add(arms)
    root.scale.setScalar(K * 1.3)
    return { root, arms, legs }
  }

  private truck(color: string, small: boolean): Truck {
    const root = new THREE.Group()
    const body = new THREE.Group()
    root.add(body)
    body.add(this.box(0.5, 0.1, 1.1, '#2d2f36', 0, 0.2, 0))
    const cab = this.box(0.46, 0.3, 0.3, color, 0, 0.42, 0.38)
    body.add(cab, this.box(0.4, 0.14, 0.02, '#1e3a5f', 0, 0.47, 0.535))
    const wheels: THREE.Mesh[] = []
    for (const [x, z] of [[-0.26, 0.36], [0.26, 0.36], [-0.26, -0.3], [0.26, -0.3]]) {
      const w = new THREE.Mesh(this.geo('wheel', () => new THREE.CylinderGeometry(0.14, 0.14, 0.11, 14).rotateZ(Math.PI / 2)), this.mat('#1f2329'))
      w.position.set(x, 0.14, z)
      w.castShadow = true
      wheels.push(w)
      body.add(w)
    }
    const bed = new THREE.Group()
    bed.position.set(0, 0.26, -0.55)
    bed.add(this.box(0.54, small ? 0.12 : 0.26, 0.76, color, 0, small ? 0.06 : 0.13, 0.38))
    let load: THREE.Object3D
    if (small) {
      load = new THREE.Mesh(this.geo('coin', () => new THREE.CylinderGeometry(0.16, 0.16, 0.06, 18)), this.mat(COLOR.gold))
      load.position.set(0, 0.2, 0.38)
      ;(load as THREE.Mesh).rotation.x = Math.PI / 2
      body.add(this.box(0.08, 0.08, 0.08, '#ff7a1a', 0, 0.6, 0.38)) // beacon
    } else {
      const heap = new THREE.Group()
      for (const [x, z] of [[-0.12, 0.2], [0.12, 0.2], [0, 0.48], [-0.12, 0.6], [0.12, 0.5]]) heap.add(this.box(0.2, 0.18, 0.2, '#c58b4a', x, 0.33, z))
      load = heap
    }
    bed.add(load)
    body.add(bed)
    root.scale.setScalar(K * (small ? 0.85 : 1.05))
    return { root, wheels, bed, load }
  }

  private excavator(): Excavator {
    const root = new THREE.Group()
    for (const x of [-0.24, 0.24]) root.add(this.box(0.17, 0.16, 0.9, '#2d2f36', x, 0.08, 0))
    const turret = new THREE.Group()
    turret.position.y = 0.17
    root.add(turret)
    turret.add(this.box(0.56, 0.26, 0.62, '#f2a900', 0, 0.14, -0.04))
    turret.add(this.box(0.56, 0.2, 0.16, '#9a6b00', 0, 0.13, -0.42))
    turret.add(this.box(0.22, 0.3, 0.26, '#f2a900', -0.15, 0.41, 0.12))
    turret.add(this.box(0.2, 0.16, 0.02, '#1e3a5f', -0.15, 0.45, 0.255))
    const boom = new THREE.Group()
    boom.position.set(0.1, 0.3, 0.22)
    boom.add(this.box(0.1, 0.1, 0.75, '#f2a900', 0, 0, 0.37))
    turret.add(boom)
    const stick = new THREE.Group()
    stick.position.set(0, 0, 0.74)
    stick.add(this.box(0.08, 0.56, 0.08, '#e09b00', 0, -0.27, 0))
    boom.add(stick)
    const bucket = new THREE.Group()
    bucket.position.set(0, -0.55, 0)
    bucket.add(this.box(0.24, 0.16, 0.18, '#3a3f4b', 0, -0.06, 0.06))
    const dirt = this.box(0.2, 0.08, 0.15, '#9b6a42', 0, 0.04, 0.06)
    bucket.add(dirt)
    stick.add(bucket)
    root.scale.setScalar(K * 1.1)
    return { root, turret, boom, stick, bucket, dirt, phase: 0, full: false }
  }

  // ---------- data ----------

  /** New data: dig sites and the crew's to-do lists. */
  setPlan(plan: MinePlan) {
    if (!this.ok) return
    const old: THREE.Sprite[] = []
    this.siteGroup.traverse((o) => { if (o instanceof THREE.Sprite) old.push(o) })
    old.forEach((o) => this.dropSprite(o))
    this.siteGroup.clear()
    this.hits = this.hits.filter((h) => (h.userData.target as MineTarget).kind !== 'site')
    this.sites.clear()
    const r = rng(29)
    plan.sites.forEach((site, i) => {
      const s = this.sAt(site.t)
      const at = this.roadAt(s, 0)
      const pile = this.roadAt(s, 1.45).pos
      pile.y = this.yAt(pile.x, pile.z)
      const g = new THREE.Group()
      // ore pile: rocks, with crystals in the credit colour (gold for key accounts, grey when closed)
      const closed = site.look === 'closed'
      const ore = closed ? '#8b95a7' : site.look === 'key' ? COLOR.gold : COLOR[site.tone]
      for (let k = 0; k < 6; k++) {
        const rock = new THREE.Mesh(this.geo('rock', () => new THREE.DodecahedronGeometry(0.3, 0)), this.mat(closed ? '#7d7f86' : '#8d6a4b', 'flat'))
        rock.position.set(pile.x + (r() - 0.5) * 0.9, pile.y + 0.1 + r() * 0.25, pile.z + (r() - 0.5) * 0.9)
        rock.rotation.set(r() * 3, r() * 3, r() * 3)
        rock.scale.setScalar(0.8 + r() * 0.7)
        rock.castShadow = true
        rock.receiveShadow = true
        g.add(rock)
      }
      for (let k = 0; k < (site.look === 'key' ? 6 : 4); k++) {
        const gem = new THREE.Mesh(this.geo('gem', () => new THREE.OctahedronGeometry(0.13, 0)), this.mat(ore, 'flat'))
        gem.position.set(pile.x + (r() - 0.5) * 0.8, pile.y + 0.35 + r() * 0.3, pile.z + (r() - 0.5) * 0.8)
        gem.rotation.set(r() * 3, r() * 3, 0)
        g.add(gem)
      }
      const view: SiteView = { site, s, pile, ring: this.ring(1.4) }
      view.ring.position.set(pile.x, pile.y + 0.08, pile.z)
      g.add(view.ring)
      if (closed) { // barrier: two posts and a striped bar
        const bar = new THREE.Group()
        bar.add(this.box(0.06, 0.5, 0.06, '#e5e7eb', -0.5, 0.25, 0), this.box(0.06, 0.5, 0.06, '#e5e7eb', 0.5, 0.25, 0))
        for (let k = 0; k < 5; k++) bar.add(this.box(0.2, 0.08, 0.05, k % 2 ? '#ffffff' : '#e5534b', -0.4 + k * 0.2, 0.42, 0))
        const bp = this.roadAt(s, 0.8).pos
        bar.position.set(bp.x, this.yAt(bp.x, bp.z), bp.z)
        bar.lookAt(pile.x, bar.position.y, pile.z)
        bar.rotateY(Math.PI / 2)
        g.add(bar)
      } else {
        const m = this.worker(site.look === 'risk' ? COLOR.warn : '#ff7a1a', 'pick')
        const mp = this.roadAt(s, 0.68).pos
        m.root.position.set(mp.x, this.yAt(mp.x, mp.z), mp.z)
        m.root.lookAt(pile.x, m.root.position.y, pile.z)
        view.miner = { ...m, phase: (i * 0.37) % 1, prev: 0 }
        g.add(m.root)
      }
      // labels
      const top = pile.y + 1.25
      const name = this.label(site.name.length > 20 ? site.name.slice(0, 19) + '…' : site.name, closed ? '#4b5563' : '#1f2937')
      name.position.set(pile.x, top, pile.z)
      g.add(name)
      let up = top + 0.72
      if (site.dueIn != null && site.dueIn >= 0 && site.dueIn <= 14 && !closed) {
        const due = this.label(site.dueIn === 0 ? 'order hari ini' : `order ${site.dueIn} hr lagi`, COLOR.good, '#ffffff', 0.5)
        due.position.set(pile.x, up, pile.z)
        g.add(due)
        up += 0.6
      }
      if (site.look === 'key') { const st = this.label('★ Key', COLOR.gold, '#3b2a00', 0.5); st.position.set(pile.x, up, pile.z); g.add(st) }
      if (site.look === 'risk') { const al = this.label('!', COLOR.warn, '#ffffff', 0.55); al.position.set(pile.x, up, pile.z); al.userData.bob = up; g.add(al) }
      const hb = this.hitBox(2.4, 2.2, 2.4, { kind: 'site', site })
      const mid = at.pos.clone().lerp(pile, 0.6)
      hb.position.set(mid.x, pile.y + 0.6, mid.z)
      g.add(hb)
      this.siteGroup.add(g)
      this.sites.set(site.id, view)
    })
    // excavators dig at dealers whose jadwal order is near
    for (const task of plan.tasks.deliver.slice(0, 3)) {
      const v = this.sites.get(task.site)
      if (!v || v.site.look === 'closed') continue
      const e = this.excavator()
      const ep = this.roadAt(v.s + 1.5, 0.7).pos
      e.root.position.set(ep.x, this.yAt(ep.x, ep.z), ep.z)
      e.root.lookAt(v.pile.x, e.root.position.y, v.pile.z)
      e.phase = this.rand() * DIG_T
      v.exc = e
      this.siteGroup.add(e.root)
    }
    this.buildActors(plan)
  }

  private buildActors(plan: MinePlan) {
    for (const a of this.actors) this.dropSprite(a.label)
    this.actorGroup.clear()
    const mk = (kind: TaskKind, n: number, speed: number, offset: number): Actor[] => {
      const list = plan.tasks[kind].map((t) => t.site)
      return Array.from({ length: Math.min(n, list.length) }, (_, i) => {
        const truck = kind === 'visit' ? undefined : this.truck(COLOR[kind], kind === 'collect')
        const walker = kind === 'visit' ? this.worker(COLOR.visit, 'clipboard') : undefined
        const obj = (truck?.root ?? walker!.root)
        obj.visible = false
        this.actorGroup.add(obj)
        return { kind, truck, walker, obj, tasks: list, next: i, state: 'rest' as const, s: 0, goal: 0, timer: offset + i * 3.5, speed, site: '' }
      })
    }
    this.actors = [...mk('deliver', 2, 5.2, 0.3), ...mk('collect', 1, 7, 1.6), ...mk('visit', 2, 2.4, 0.8)]
  }

  setStock(stock: MineOptions['stock'], unread: number) {
    this.o.stock = stock
    this.o.unread = unread
    if (!this.ok) return
    const crates = this.wh.crates
    crates.clear()
    const n = Math.min(12, stock.aging)
    for (let i = 0; i < n; i++) crates.add(this.box(0.42, 0.36, 0.42, i % 3 ? '#c58b4a' : '#b07a3e', -1.1 + (i % 4) * 0.48, 0.18 + Math.floor(i / 8) * 0.37, Math.floor((i % 8) / 4) * 0.48))
    this.wh.beacon!.visible = stock.critical
    const text = unread > 0 ? `${unread} chat` : ''
    if (text !== this.office.chatText) {
      this.dropSprite(this.office.chat)
      this.office.chat = null
      this.office.chatText = text
      if (text) {
        this.office.chat = this.label(text, COLOR.good, '#ffffff', 0.55)
        this.office.chat.position.set(this.office.pos.x, 2.15, this.office.pos.z)
        this.scene.add(this.office.chat)
      }
    }
  }

  // ---------- animation ----------

  private spawn(list: Bit[], at: THREE.Vector3, n: number, color: string, kind: 'chip' | 'coin') {
    for (let i = 0; i < n; i++) {
      let b = list.find((x) => x.life <= 0)
      if (!b) {
        if (list.length > 60) return
        const mesh = kind === 'coin'
          ? new THREE.Mesh(this.geo('coin', () => new THREE.CylinderGeometry(0.16, 0.16, 0.06, 18)), this.mat(color))
          : new THREE.Mesh(this.geo('chip', () => new THREE.BoxGeometry(0.07, 0.07, 0.07)), this.mat(color, 'flat'))
        this.scene.add(mesh)
        b = { mesh, v: new THREE.Vector3(), life: 0 }
        list.push(b)
      }
      b.mesh.material = this.mat(color, kind === 'chip' ? 'flat' : '')
      b.mesh.position.copy(at)
      b.mesh.visible = true
      const a = this.rand() * Math.PI * 2
      const sp = kind === 'coin' ? 1.2 + this.rand() : 0.6 + this.rand() * 0.9
      b.v.set(Math.cos(a) * sp, (kind === 'coin' ? 4 : 2) + this.rand() * 1.5, Math.sin(a) * sp)
      b.life = kind === 'coin' ? 1.4 : 0.9
    }
  }

  private stepBits(list: Bit[], dt: number) {
    for (const b of list) {
      if (b.life <= 0) continue
      b.life -= dt
      b.v.y -= 9 * dt
      b.mesh.position.addScaledVector(b.v, dt)
      b.mesh.rotation.x += dt * 6
      b.mesh.rotation.y += dt * 4
      if (b.life <= 0) b.mesh.visible = false
    }
  }

  private step(dt: number) {
    this.t += dt
    const v3 = new THREE.Vector3()
    for (const c of this.clouds) { c.position.x += dt * 0.6; if (c.position.x > 45) c.position.x = -45 }
    for (const v of this.sites.values()) {
      const m = v.miner
      if (m) { // raise slowly, strike fast, hold
        const p = (this.t * 0.55 + m.phase) % 1
        const a = p < 0.6 ? lerp(-0.9, -2.9, smooth(0, 1, p / 0.6)) : p < 0.72 ? lerp(-2.9, -0.7, (p - 0.6) / 0.12) : -0.7
        m.arms.rotation.x = a
        if (m.prev < 0.72 && p >= 0.72) this.spawn(this.bits, v3.copy(v.pile).setY(v.pile.y + 0.35), 4, v.site.look === 'key' ? COLOR.gold : '#9b6a42', 'chip')
        m.prev = p
      }
      const e = v.exc
      if (e) {
        e.phase = (e.phase + dt) % DIG_T
        let k = 0
        while (k < DIG.length - 2 && DIG[k + 1][0] <= e.phase) k++
        const [t0, y0, b0, s0, k0] = DIG[k]
        const [t1, y1, b1, s1, k1] = DIG[k + 1]
        const f = smooth(0, 1, (e.phase - t0) / (t1 - t0))
        e.turret.rotation.y = lerp(y0, y1, f)
        e.boom.rotation.x = lerp(b0, b1, f)
        e.stick.rotation.x = lerp(s0, s1, f)
        e.bucket.rotation.x = lerp(k0, k1, f)
        const full = e.phase >= 2.3 && e.phase < 5.0
        if (e.full && !full) this.spawn(this.bits, e.bucket.getWorldPosition(v3), 6, '#9b6a42', 'chip')
        e.full = full
        e.dirt.visible = full
      }
    }
    for (const a of this.actors) this.stepActor(a, dt)
    this.stepBits(this.bits, dt)
    this.stepBits(this.coins, dt)
  }

  private stepActor(a: Actor, dt: number) {
    if (a.state === 'rest') {
      a.timer -= dt
      if (a.timer > 0) return
      const v = this.sites.get(a.tasks[a.next % a.tasks.length])
      a.next += 1
      if (!v) { a.timer = 1; return }
      a.site = v.site.id
      a.goal = v.s - (a.kind === 'visit' ? 0.6 : 0)
      a.s = 0
      a.state = 'out'
      a.obj.visible = true
      if (a.truck) a.truck.load.visible = a.kind === 'deliver'
    } else if (a.state === 'out' || a.state === 'back') {
      a.s += (a.state === 'out' ? 1 : -1) * a.speed * dt
      if (a.truck) for (const w of a.truck.wheels) w.rotation.x += (a.speed * dt) / 0.14
      if (a.state === 'out' && a.s >= a.goal) {
        a.s = a.goal
        a.state = 'act'
        a.timer = 2.8
        const name = this.sites.get(a.site)?.site.name ?? ''
        const text = (a.kind === 'deliver' ? 'Antar · ' : a.kind === 'collect' ? 'Tagih · ' : 'Follow-up · ') + name
        a.label = this.label(text.length > 28 ? text.slice(0, 27) + '…' : text, COLOR[a.kind], a.kind === 'deliver' ? '#2b1d00' : '#ffffff', 0.5)
        this.actorGroup.add(a.label)
      } else if (a.state === 'back' && a.s <= 0) {
        a.state = 'rest'
        a.timer = 1.6
        a.obj.visible = false
        if (a.kind === 'collect') this.spawn(this.coins, this.wh.pos.clone().setY(2.4), 8, COLOR.gold, 'coin')
      }
    } else {
      a.timer -= dt
      if (a.truck && a.kind === 'deliver') { // tip the bed: the goods slide off at the dealer
        const k = 1 - a.timer / 2.8
        a.truck.bed.rotation.x = -0.75 * Math.sin(Math.PI * k)
        if (k > 0.45) a.truck.load.visible = false
      }
      if (a.truck && a.kind === 'collect' && a.timer < 1.4) a.truck.load.visible = true
      if (a.timer <= 0) {
        a.state = 'back'
        if (a.truck) a.truck.bed.rotation.x = 0
        this.dropSprite(a.label)
        a.label = undefined
      }
    }
    this.placeActor(a)
  }

  private placeActor(a: Actor) {
    if (a.state === 'rest') return
    const walk = a.kind === 'visit'
    const lane = walk ? LANE.walk : a.state === 'back' ? LANE.back : LANE.out
    const at = this.roadAt(a.s, lane)
    const dir = at.dir.clone()
    if (a.state === 'back') dir.negate()
    if (walk) dir.setY(0).normalize()
    a.obj.position.copy(at.pos)
    if (a.state === 'act' && walk) {
      const v = this.sites.get(a.site)
      if (v) a.obj.lookAt(v.pile.x, at.pos.y, v.pile.z)
    } else a.obj.lookAt(at.pos.clone().add(dir))
    if (a.walker) {
      const sw = a.state === 'act' ? 0 : Math.sin(this.t * 9) * 0.6
      a.walker.legs[0].rotation.x = sw
      a.walker.legs[1].rotation.x = -sw
      a.walker.arms.rotation.x = a.state === 'act' ? -0.9 + Math.sin(this.t * 3) * 0.08 : -sw * 0.5
    }
    if (a.label) a.label.position.set(at.pos.x, at.pos.y + (walk ? 1.1 : 1.3), at.pos.z)
  }

  private daylight() {
    const hour = wibHour()
    const night = hour >= 18 || hour < 6
    if (night === this.night) return
    this.night = night
    const dusk = hour === 17 || hour === 6
    const sky = night ? '#141c3d' : dusk ? '#f3b27a' : '#9fd3f2'
    this.scene.background = new THREE.Color(sky)
    this.scene.fog = new THREE.Fog(sky, 55, 130)
    this.lights.hemi.intensity = night ? 0.45 : 1.05
    this.lights.hemi.color.set(night ? '#8ea2d8' : '#ffffff')
    this.lights.sun.intensity = night ? 0.25 : dusk ? 1.1 : 1.7
    this.lights.sun.color.set(night ? '#9fb2ff' : dusk ? '#ffc58a' : '#fff6e8')
    for (const l of this.lights.towers) l.intensity = night ? 40 : 0
    for (const h of this.lights.heads) (h.material as THREE.MeshBasicMaterial).color.set(night ? '#fff1b8' : '#d7dbe2')
    for (const w of this.office.windows) (w.material as THREE.MeshBasicMaterial).color.set(night ? '#ffe28a' : '#bfe0ff')
    for (const c of this.clouds) c.visible = !night
  }

  private frame = (now: number) => {
    this.raf = requestAnimationFrame(this.frame)
    const st = this.o.stage
    if (!this.renderer || !st.isConnected || st.getClientRects().length === 0) return
    const w = st.clientWidth
    if (!w) return
    if (w !== this.w) {
      this.w = w
      this.renderer.setSize(w, MINE_H)
      const aspect = w / MINE_H
      this.camera.aspect = aspect
      const tan = Math.tan(THREE.MathUtils.degToRad(this.camera.fov / 2))
      this.dist = Math.min(66, Math.max(9.5 / tan, 24 / (tan * aspect)))
      this.camera.updateProjectionMatrix()
    }
    const dt = this.last ? Math.min(0.05, (now - this.last) / 1000) : 0
    this.last = now
    this.clock -= dt
    if (this.clock <= 0) { this.clock = 60; this.daylight() }
    if (!this.reduced) this.step(dt)
    if (this.wh.beacon?.visible) this.wh.beacon.scale.setScalar(0.8 + 0.4 * Math.abs(Math.sin(this.t * 5)))
    if (this.office.chat) this.office.chat.position.y = 2.15 + Math.sin(this.t * 3) * 0.08
    const yaw = this.yaw + this.sway()
    this.camera.position.set(
      this.target.x + this.dist * Math.sin(yaw) * Math.cos(this.pitch),
      this.target.y + this.dist * Math.sin(this.pitch),
      this.target.z + this.dist * Math.cos(yaw) * Math.cos(this.pitch),
    )
    this.camera.lookAt(this.target)
    this.renderer.render(this.scene, this.camera)
  }

  /** The camera drifts a little left and right until someone turns it. */
  private sway() { return this.moved || this.reduced ? 0 : Math.sin(this.t * 0.07) * 0.22 }

  // ---------- pointer ----------

  private bind() {
    const cv = this.o.canvas
    cv.style.touchAction = 'pan-y'
    const ray = new THREE.Raycaster()
    const ndc = new THREE.Vector2()
    const pick = (e: PointerEvent | MouseEvent): MineTarget | null => {
      const r = cv.getBoundingClientRect()
      ndc.set(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1)
      ray.setFromCamera(ndc, this.camera)
      const hit = ray.intersectObjects(this.hits, false)[0]
      return (hit?.object.userData.target as MineTarget | undefined) ?? null
    }
    const keyOf = (t: MineTarget | null) => (t ? (t.kind === 'site' ? t.site.id : t.kind) : null)
    const highlight = (k: string | null) => {
      if (k === this.hover) return
      this.hover = k
      for (const v of this.sites.values()) v.ring.visible = v.site.id === k
      this.wh.ring!.visible = k === 'warehouse'
      this.office.ring!.visible = k === 'office'
    }
    let down: { x: number; y: number; yaw: number; pitch: number } | null = null
    let dragged = false
    const pdown = (e: PointerEvent) => { down = { x: e.clientX, y: e.clientY, yaw: this.yaw, pitch: this.pitch }; dragged = false }
    const pmove = (e: PointerEvent) => {
      const r = cv.getBoundingClientRect()
      if (down && (e.buttons & 1 || e.pointerType !== 'mouse')) {
        const dx = e.clientX - down.x
        const dy = e.clientY - down.y
        if (Math.hypot(dx, dy) > 5) dragged = true
        if (dragged) {
          if (!this.moved) { this.yaw += this.sway(); down.yaw = this.yaw + dx * 0.006; this.moved = true }
          this.yaw = down.yaw - dx * 0.006
          this.pitch = Math.max(0.3, Math.min(1.2, down.pitch + dy * 0.004))
          this.o.onHover(null, 0, 0)
          cv.style.cursor = 'grabbing'
          return
        }
      }
      const t = pick(e)
      highlight(keyOf(t))
      cv.style.cursor = t ? 'pointer' : 'grab'
      this.o.onHover(t, e.clientX - r.left, e.clientY - r.top)
    }
    const pup = (e: PointerEvent) => {
      if (down && !dragged) { const t = pick(e); if (t) this.o.onPick(t) }
      down = null
      cv.style.cursor = ''
    }
    const leave = () => { down = null; highlight(null); this.o.onHover(null, 0, 0) }
    const reset = () => { this.yaw = 0; this.pitch = 0.62; this.moved = false }
    cv.addEventListener('pointerdown', pdown)
    cv.addEventListener('pointermove', pmove)
    cv.addEventListener('pointerup', pup)
    cv.addEventListener('pointerleave', leave)
    cv.addEventListener('dblclick', reset)
    this.off.push(() => {
      cv.removeEventListener('pointerdown', pdown)
      cv.removeEventListener('pointermove', pmove)
      cv.removeEventListener('pointerup', pup)
      cv.removeEventListener('pointerleave', leave)
      cv.removeEventListener('dblclick', reset)
    })
  }

  start() {
    if (this.ok && !this.raf) this.raf = requestAnimationFrame(this.frame)
  }

  destroy() {
    cancelAnimationFrame(this.raf)
    this.raf = 0
    this.off.forEach((f) => f())
    this.scene.traverse((o) => {
      if (o instanceof THREE.Sprite) { o.material.map?.dispose(); o.material.dispose() }
      else if (o instanceof THREE.Mesh) (o.material as THREE.Material).dispose()
    })
    this.geos.forEach((g) => g.dispose())
    this.mats.forEach((m) => m.dispose())
    this.renderer?.dispose()
  }
}
