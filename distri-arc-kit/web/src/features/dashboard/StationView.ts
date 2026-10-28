import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RoomEnvironment } from 'three/examples/jsm/environments/RoomEnvironment.js'
import { CREW, CREW_OF, type CrewKey, type CrewMember, type CrewState, type CrewStatus } from './station'

// Stasiun Orbit: an office deck inside a space station in three.js, rendered HD (MSAA, ACES tone mapping, bloom, PBR
// with an environment map) — a 3D office of AI agents, with astronauts (Sam 28 Oct). The AI core (the Orchestrator)
// floats in the middle of a round deck with panorama windows on space; ten desks stand round it, one astronaut each
// at a console with a hologram of its work: 1 Analisa sales, 2 Analisa WhatsApp, 3 Analisa dealer, 4 Eksekusi order,
// 5 Follow-up, 6 Cek stok, 7 Cek kredit, 8 Cek penagihan, 9 Prospek, 10 Data siap. The work is a relay: an astronaut
// that finishes takes its result capsule, walks the deck to the next desk, hands it over and walks back. A real cycle
// drives it; "Putar alur kerja" plays the relay 1→10 as a show (no analysis runs). No customer is ever drawn — the
// desks say what the astronaut does and counts only. Clicking a desk opens its screen.

export type StationTarget = { kind: 'core' } | { kind: 'room'; key: CrewKey }

export interface CycleState { running: boolean; stage: number; hasCycle: boolean }

export interface StationOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  onPick: (t: StationTarget) => void
  onHover: (t: StationTarget | null, x: number, y: number) => void
  /** the relay show started (true) or ended (false) */
  onReplay?: (on: boolean) => void
}

export const STATION_H = 540

// world layout (units)
const CORE_R = 1.3
const ARC_R = 2.05 // the six stage arcs
const DECK_Y = -1.45 // the office floor, under the floating core
const DECK_R = 7.5 // the round deck, to the panorama windows
const ROOM_R = 5.3 // the ring of desks
const WALK_R = 4.15 // the walkway in front of the desks, where the astronauts walk
const DOME = 1.0 // a desk's floor pad
const FLOOR = 0.06 // top of a desk's floor pad
const ASTRO_S = 1.4
const WALK_SPEED = 1.35 // units per second
const GIVE_S = 1.1 // the hand-over at the next desk
const READ_S = 2.4 // a desk's analysis in the relay show
const BELT_IN = 13.6
const BELT_OUT = 14.8
const PITCH0 = 0.66

const onRing = (a: number, r: number, y = 0) => new THREE.Vector3(r * Math.cos(a), y, r * Math.sin(a))
/** Room i: 1 at the back left, round the front, 10 (Data siap) at the back in the middle. */
const roomAngle = (i: number) => -Math.PI / 2 - ((i + 1) * Math.PI * 2) / CREW.length

const clamp01 = (v: number) => Math.max(0, Math.min(1, v))
const lerp = (a: number, b: number, k: number) => a + (b - a) * k

function rng(seed: number) {
  let s = seed >>> 0 || 1
  return () => { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296 }
}

// ---------- a far planet for the backdrop (no data: scenery) ----------

/** 3D value noise with a seeded lattice, for seamless sphere textures. */
function noise3(seed: number) {
  const P = new Uint8Array(512)
  const r = rng(seed)
  const p = Array.from({ length: 256 }, (_, i) => i)
  for (let i = 255; i > 0; i--) { const j = Math.floor(r() * (i + 1)); [p[i], p[j]] = [p[j], p[i]] }
  for (let i = 0; i < 512; i++) P[i] = p[i & 255]
  const V = Float32Array.from({ length: 256 }, () => r())
  const f = (t: number) => t * t * (3 - 2 * t)
  return (x: number, y: number, z: number) => {
    const xi = Math.floor(x); const yi = Math.floor(y); const zi = Math.floor(z)
    const xf = f(x - xi); const yf = f(y - yi); const zf = f(z - zi)
    const X = xi & 255; const Y = yi & 255; const Z = zi & 255
    const v = (a: number, b: number, c: number) => V[P[P[P[X + a] + Y + b] + Z + c]]
    const x0 = lerp(lerp(v(0, 0, 0), v(1, 0, 0), xf), lerp(v(0, 1, 0), v(1, 1, 0), xf), yf)
    const x1 = lerp(lerp(v(0, 0, 1), v(1, 0, 1), xf), lerp(v(0, 1, 1), v(1, 1, 1), xf), yf)
    return lerp(x0, x1, zf)
  }
}

function ramp(cols: THREE.Color[], t: number, out: THREE.Color) {
  const x = clamp01(t) * (cols.length - 1)
  const i = Math.min(cols.length - 2, Math.floor(x))
  return out.copy(cols[i]).lerp(cols[i + 1], x - i)
}

function giantMap(seed: number, palette: string[]): THREE.CanvasTexture {
  const n = noise3(seed)
  const W = 512
  const H = 256
  const c = document.createElement('canvas')
  c.width = W; c.height = H
  const g = c.getContext('2d')!
  const img = g.createImageData(W, H)
  const cols = palette.map((h) => new THREE.Color(h))
  const out = new THREE.Color()
  const fbm = (x: number, y: number, z: number, o: number) => { let a = 0.5; let s = 0; let f = 1; for (let i = 0; i < o; i++) { s += a * n(x * f, y * f, z * f); f *= 2.03; a *= 0.5 } return s / (1 - Math.pow(0.5, o)) }
  for (let y = 0; y < H; y++) {
    const th = (y / H) * Math.PI
    const sy = Math.cos(th)
    const st = Math.sin(th)
    for (let x = 0; x < W; x++) {
      const ph = (x / W) * Math.PI * 2
      const px = Math.cos(ph) * st; const pz = Math.sin(ph) * st
      const t = fbm(px * 2.5, sy * 2.5, pz * 2.5, 4)
      const v = (sy * 0.5 + 0.5) * 8 + t * 1.4
      ramp(cols, (Math.sin(v * Math.PI) * 0.5 + 0.5) * 0.7 + fbm(px * 6, sy * 18, pz * 6, 3) * 0.3, out)
      const i = (y * W + x) * 4
      img.data[i] = out.r * 255; img.data[i + 1] = out.g * 255; img.data[i + 2] = out.b * 255; img.data[i + 3] = 255
    }
  }
  g.putImageData(img, 0, 0)
  const map = new THREE.CanvasTexture(c)
  map.colorSpace = THREE.SRGBColorSpace
  return map
}

// ---------- shaders ----------

const FRESNEL_V = `varying vec3 vN; varying vec3 vV;
void main(){ vec4 mv = modelViewMatrix * vec4(position, 1.0); vN = normalize(normalMatrix * normal); vV = normalize(-mv.xyz); gl_Position = projectionMatrix * mv; }`
const FRESNEL_F = `uniform vec3 uColor; uniform float uPower; uniform float uAlpha; varying vec3 vN; varying vec3 vV;
void main(){ float f = pow(1.0 - abs(dot(vN, vV)), uPower); gl_FragColor = vec4(uColor, f * uAlpha); }`

const CORE_V = `varying vec3 vP; varying vec3 vN; varying vec3 vV;
void main(){ vP = position; vec4 mv = modelViewMatrix * vec4(position, 1.0); vN = normalize(normalMatrix * normal); vV = normalize(-mv.xyz); gl_Position = projectionMatrix * mv; }`
const CORE_F = `uniform float uTime; uniform float uBoost; varying vec3 vP; varying vec3 vN; varying vec3 vV;
float hash(vec3 p){ p = fract(p * 0.3183099 + 0.1); p *= 17.0; return fract(p.x * p.y * p.z * (p.x + p.y + p.z)); }
float noise(vec3 x){ vec3 i = floor(x); vec3 f = fract(x); f = f * f * (3.0 - 2.0 * f);
  return mix(mix(mix(hash(i), hash(i + vec3(1,0,0)), f.x), mix(hash(i + vec3(0,1,0)), hash(i + vec3(1,1,0)), f.x), f.y),
             mix(mix(hash(i + vec3(0,0,1)), hash(i + vec3(1,0,1)), f.x), mix(hash(i + vec3(0,1,1)), hash(i + vec3(1,1,1)), f.x), f.y), f.z); }
float fbm(vec3 p){ float a = 0.5; float s = 0.0; for (int i = 0; i < 5; i++){ s += a * noise(p); p *= 2.03; a *= 0.5; } return s; }
void main(){
  vec3 p = vP * 1.7;
  float n = fbm(p + vec3(0.0, uTime * 0.22, uTime * 0.1));
  float m = fbm(p * 2.2 - vec3(uTime * 0.18));
  float f = pow(1.0 - max(dot(vN, vV), 0.0), 2.2);
  vec3 deep = vec3(0.02, 0.18, 0.6);
  vec3 hot = vec3(0.3, 1.0, 1.45);
  vec3 white = vec3(1.5, 1.7, 1.9);
  vec3 c = mix(deep, hot, smoothstep(0.32, 0.68, n));
  c = mix(c, white, smoothstep(0.55, 0.85, m * n * 1.7));
  c += f * vec3(0.3, 0.8, 1.2);
  gl_FragColor = vec4(c * (0.8 + uBoost * 0.6), 1.0);
}`

// ---------- view types ----------

interface Astro { root: THREE.Group; body: THREE.Group; arms: [THREE.Group, THREE.Group]; legs: [THREE.Group, THREE.Group]; flames: THREE.Mesh[]; hand: THREE.Group }
interface Room {
  m: CrewMember
  i: number
  g: THREE.Group
  a: Astro
  holo: THREE.Group
  screen: THREE.MeshBasicMaterial
  status: THREE.MeshBasicMaterial
  lamp: THREE.Sprite
  ring: THREE.Mesh // hover
  label: THREE.Sprite | null
  labelText: string
  beam: THREE.Line
  carry: THREE.Object3D | null
  work: { state: CrewState; text: string }
  was: CrewState
  walk: Walk | null
  /** the hand-over reached this desk (the relay show waits for it) */
  gave: boolean
  home: THREE.Vector3
}
/** An astronaut walking the deck: out to the next desk with the result, the hand-over, back home. */
interface Walk { to: Room; pts: THREE.Vector3[]; at: number[]; d: number; phase: 'go' | 'give' | 'back'; wait: number }
interface Pulse { s: THREE.Sprite; from: () => THREE.Vector3; to: () => THREE.Vector3; t: number; dur: number; lift: number }
interface Bit { m: THREE.Object3D; v: THREE.Vector3; life: number; max: number }

const STATE_WORD: Record<CrewState, string> = { idle: '', waiting: 'antri', reading: 'menganalisa', done: 'selesai', failed: 'gagal' }

function webgl(canvas: HTMLCanvasElement): THREE.WebGLRenderer | null {
  try {
    return new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: 'high-performance' })
  } catch {
    return null
  }
}

export class StationView {
  readonly ok: boolean
  private o: StationOptions
  private renderer: THREE.WebGLRenderer | null
  private composer: EffectComposer | null = null
  private bloom: UnrealBloomPass | null = null
  private scene = new THREE.Scene()
  private overlay = new THREE.Scene() // labels: drawn after bloom, always readable
  private camera = new THREE.PerspectiveCamera(32, 2, 0.3, 900)
  private target = new THREE.Vector3(0, -0.75, 0.5)
  private yaw = 0
  private pitch = PITCH0
  private zoom = 1
  private dist = 30
  private moved = false
  private geos = new Map<string, THREE.BufferGeometry>()
  private mats = new Map<string, THREE.Material>()
  private texs: THREE.Texture[] = []
  private glowTex: THREE.Texture
  private hits: THREE.Object3D[] = []
  private hover: string | null = null
  private rooms: Room[] = []
  private roomBy = new Map<CrewKey, Room>()
  private caps = new THREE.Group() // proposals waiting, as capsules on the Data siap floor
  private pulses: Pulse[] = []
  private bits: Bit[] = []
  private core = { mat: null as THREE.ShaderMaterial | null, mesh: null as THREE.Mesh | null, corona: null as THREE.Sprite | null, wire: null as THREE.Mesh | null, rings: [] as THREE.Mesh[], arcs: [] as THREE.Mesh[], ring: null as THREE.Mesh | null, light: null as THREE.PointLight | null, conduits: [] as THREE.Mesh[], wave: null as THREE.Mesh | null, waveT: 99 }
  private giant: THREE.Mesh | null = null
  private belt: THREE.InstancedMesh | null = null
  private stars: THREE.Points[] = []
  private cycle: CycleState = { running: false, stage: 0, hasCycle: false }
  /** the work from the data (setCrew); the relay show plays over it and gives it back at the end */
  private live: CrewStatus | null = null
  private show: { i: number; t: number; phase: 'read' | 'walk' | 'end' } | null = null
  private t = 0
  private last = 0
  private raf = 0
  private w = 0
  private h = 0
  private pulseClock = 0
  private readyClock = 0
  private perf = { n: 0, sum: 0, level: 2 }
  // the HD path (bloom on an MSAA render target) draws nothing on some GPUs/browsers: checked on the first frames,
  // then the scene renders straight to the canvas (still PBR, ACES and MSAA, without bloom)
  private direct = false
  private frames = 0
  private reduced: boolean
  private off: (() => void)[] = []

  constructor(o: StationOptions) {
    this.o = o
    this.reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    this.renderer = webgl(o.canvas)
    this.ok = !!this.renderer
    this.glowTex = this.makeGlow()
    if (!this.renderer) return
    const r = this.renderer
    r.setPixelRatio(Math.min(2, Math.max(1.5, devicePixelRatio)))
    r.toneMapping = THREE.ACESFilmicToneMapping
    r.toneMappingExposure = 1.05
    r.outputColorSpace = THREE.SRGBColorSpace
    const pmrem = new THREE.PMREMGenerator(r)
    const env = pmrem.fromScene(new RoomEnvironment(r), 0.04).texture
    this.texs.push(env)
    pmrem.dispose()
    this.scene.environment = env
    this.scene.background = new THREE.Color('#02030a')
    const webgl2 = r.capabilities.isWebGL2
    const float = webgl2 && (r.extensions.has('EXT_color_buffer_half_float') || r.extensions.has('EXT_color_buffer_float'))
    this.composer = new EffectComposer(r, new THREE.WebGLRenderTarget(1, 1, { type: float ? THREE.HalfFloatType : THREE.UnsignedByteType, samples: webgl2 ? 4 : 0 }))
    this.composer.addPass(new RenderPass(this.scene, this.camera))
    this.bloom = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.55, 0.45, 0.9)
    this.composer.addPass(this.bloom)
    this.composer.addPass(new OutputPass())
    this.buildSky()
    this.buildCore()
    this.buildOffice()
    CREW.forEach((m, i) => this.buildRoom(m, i))
    this.buildBelt()
    this.bind()
  }

  // ---------- shared helpers ----------

  private geo<T extends THREE.BufferGeometry>(key: string, make: () => T): T {
    let g = this.geos.get(key)
    if (!g) { g = make(); this.geos.set(key, g) }
    return g as T
  }
  /** PBR material, shared by key. */
  private std(color: string, rough = 0.5, metal = 0, env = 0.7): THREE.MeshStandardMaterial {
    const key = `std${color}|${rough}|${metal}|${env}`
    let m = this.mats.get(key)
    if (!m) { m = new THREE.MeshStandardMaterial({ color, roughness: rough, metalness: metal, envMapIntensity: env }); this.mats.set(key, m) }
    return m as THREE.MeshStandardMaterial
  }
  /** Unlit glowing material; values above 1 bloom. */
  private glow(r: number, g: number, b: number, additive = false, opacity = 1): THREE.MeshBasicMaterial {
    const key = `glow${r},${g},${b},${additive},${opacity}`
    let m = this.mats.get(key)
    if (!m) {
      m = new THREE.MeshBasicMaterial({ color: new THREE.Color(r, g, b), transparent: additive || opacity < 1, opacity, blending: additive ? THREE.AdditiveBlending : THREE.NormalBlending, depthWrite: !additive, toneMapped: false })
      this.mats.set(key, m)
    }
    return m as THREE.MeshBasicMaterial
  }
  private hdr(hex: string, k: number) { const c = new THREE.Color(hex); return this.glow(c.r * k, c.g * k, c.b * k) }
  /** A glowing material of one room that changes colour with its state (not shared). */
  private own(key: string, opacity = 1, additive = false) {
    const m = new THREE.MeshBasicMaterial({ color: new THREE.Color(1, 1, 1), transparent: opacity < 1 || additive, opacity, blending: additive ? THREE.AdditiveBlending : THREE.NormalBlending, depthWrite: !additive, toneMapped: false })
    this.mats.set(key, m)
    return m
  }
  private fresnel(color: string, power: number, alpha: number) {
    return new THREE.ShaderMaterial({
      vertexShader: FRESNEL_V, fragmentShader: FRESNEL_F, transparent: true, depthWrite: false, blending: THREE.AdditiveBlending,
      uniforms: { uColor: { value: new THREE.Color(color) }, uPower: { value: power }, uAlpha: { value: alpha } },
    })
  }
  private mesh(geo: THREE.BufferGeometry, mat: THREE.Material, x = 0, y = 0, z = 0) {
    const m = new THREE.Mesh(geo, mat)
    m.position.set(x, y, z)
    return m
  }
  private sphere(r: number, seg = 24) { return this.geo(`s${r},${seg}`, () => new THREE.SphereGeometry(r, seg, Math.max(8, Math.round(seg * 0.7)))) }
  private capsule(r: number, l: number) { return this.geo(`cap${r},${l}`, () => new THREE.CapsuleGeometry(r, l, 6, 14)) }
  private boxG(w: number, h: number, d: number) { return this.geo(`b${w},${h},${d}`, () => new THREE.BoxGeometry(w, h, d)) }
  private cylG(rt: number, rb: number, h: number, seg = 20, open = false) { return this.geo(`c${rt},${rb},${h},${seg},${open}`, () => new THREE.CylinderGeometry(rt, rb, h, seg, 1, open)) }
  private torusG(r: number, tube: number, arc = Math.PI * 2, seg = 96) { return this.geo(`t${r},${tube},${arc},${seg}`, () => new THREE.TorusGeometry(r, tube, 12, seg, arc)) }

  private makeGlow() {
    const c = document.createElement('canvas')
    c.width = c.height = 128
    const g = c.getContext('2d')!
    const gr = g.createRadialGradient(64, 64, 0, 64, 64, 64)
    gr.addColorStop(0, 'rgba(255,255,255,1)')
    gr.addColorStop(0.18, 'rgba(255,255,255,0.75)')
    gr.addColorStop(0.45, 'rgba(255,255,255,0.18)')
    gr.addColorStop(1, 'rgba(255,255,255,0)')
    g.fillStyle = gr
    g.fillRect(0, 0, 128, 128)
    const t = new THREE.CanvasTexture(c)
    this.texs.push(t)
    return t
  }
  private glowSprite(color: string, scale: number, opacity = 1) {
    const s = new THREE.Sprite(new THREE.SpriteMaterial({ map: this.glowTex, color, transparent: true, opacity, blending: THREE.AdditiveBlending, depthWrite: false }))
    s.scale.setScalar(scale)
    return s
  }

  /** A room sign: a number badge in the room colour, the title, and under it what the astronaut is doing. */
  private sign(m: CrewMember, state: CrewState, text: string) {
    const c = document.createElement('canvas')
    const g = c.getContext('2d')!
    const fTitle = '800 50px "Plus Jakarta Sans", "Instrument Sans", system-ui, sans-serif'
    const fSub = '600 38px "Plus Jakarta Sans", "Instrument Sans", system-ui, sans-serif'
    const sub = (STATE_WORD[state] && !text.startsWith(STATE_WORD[state]) && state !== 'reading' ? STATE_WORD[state] + ' · ' : '') + text
    const subT = sub.length > 36 ? sub.slice(0, 35) + '…' : sub
    const badge = 64
    g.font = fTitle
    const wT = g.measureText(m.title).width + badge + 18
    g.font = fSub
    const wS = g.measureText((state === 'done' ? '✓ ' : state === 'failed' ? '✕ ' : '') + subT).width
    const W = Math.ceil(Math.max(wT, wS) + 48)
    const H = 150
    c.width = W
    c.height = H
    const bg = state === 'failed' ? 'rgba(90,14,20,0.9)' : state === 'reading' ? 'rgba(10,48,92,0.92)' : 'rgba(8,12,28,0.86)'
    g.fillStyle = bg
    g.beginPath()
    if (g.roundRect) g.roundRect(3, 3, W - 6, H - 6, 26)
    else g.rect(3, 3, W - 6, H - 6)
    g.fill()
    g.strokeStyle = state === 'reading' ? '#8fd8ff' : state === 'done' ? 'rgba(127,240,210,0.8)' : 'rgba(255,255,255,0.22)'
    g.lineWidth = 4
    g.stroke()
    // number badge
    g.fillStyle = m.color
    g.beginPath()
    g.arc(24 + badge / 2, 16 + badge / 2, badge / 2, 0, Math.PI * 2)
    g.fill()
    g.fillStyle = m.key === 'prospect' || m.key === 'ready' || m.key === 'followup' ? '#161200' : '#ffffff'
    g.font = '800 38px "Plus Jakarta Sans", system-ui, sans-serif'
    g.textAlign = 'center'
    g.textBaseline = 'middle'
    g.fillText(String(m.n), 24 + badge / 2, 16 + badge / 2 + 2)
    g.textAlign = 'left'
    g.fillStyle = '#ffffff'
    g.font = fTitle
    g.fillText(m.title, 24 + badge + 16, 16 + badge / 2 + 3)
    g.font = fSub
    g.fillStyle = state === 'done' ? '#7ff0d2' : state === 'failed' ? '#ff9a9a' : state === 'reading' ? '#bfe9ff' : state === 'waiting' ? '#ffd27a' : 'rgba(214,224,245,0.82)'
    g.fillText((state === 'done' ? '✓ ' : state === 'failed' ? '✕ ' : '') + subT, 24, 118)
    const tex = new THREE.CanvasTexture(c)
    tex.colorSpace = THREE.SRGBColorSpace
    tex.anisotropy = 4
    const s = new THREE.Sprite(new THREE.SpriteMaterial({ map: tex, depthTest: false, transparent: true, toneMapped: false }))
    const h = 0.86
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
  private hit(obj: THREE.Object3D, target: StationTarget) {
    obj.userData.target = target
    this.hits.push(obj)
  }
  private hoverRing(r: number) {
    const m = this.mesh(this.torusG(r, 0.035, Math.PI * 2, 72), this.glow(1.6, 1.8, 2.2, true, 0.9))
    m.rotation.x = Math.PI / 2
    m.visible = false
    return m
  }

  // ---------- sky: nebula, stars, a far planet ----------

  private buildSky() {
    const c = document.createElement('canvas')
    c.width = 2048; c.height = 1024
    const g = c.getContext('2d')!
    g.fillStyle = '#02030a'
    g.fillRect(0, 0, 2048, 1024)
    const r = rng(11)
    const clouds: [string, number][] = [['#3a1f7a', 0.5], ['#0f4a7a', 0.45], ['#7a1f5c', 0.32], ['#0d6b6b', 0.3], ['#1c2c8a', 0.4]]
    for (let i = 0; i < 70; i++) {
      const [col, a] = clouds[i % clouds.length]
      const x = r() * 2048
      const y = 512 + (r() - 0.5) * 560 * (0.4 + r() * 0.6)
      const rad = 80 + r() * 320
      const gr = g.createRadialGradient(x, y, 0, x, y, rad)
      const cc = new THREE.Color(col)
      const rgba = (k: number) => `rgba(${Math.round(cc.r * 255)},${Math.round(cc.g * 255)},${Math.round(cc.b * 255)},${k})`
      gr.addColorStop(0, rgba(a * 0.55 * r()))
      gr.addColorStop(1, rgba(0))
      g.fillStyle = gr
      g.fillRect(x - rad, y - rad, rad * 2, rad * 2)
      if (x < rad) { g.save(); g.translate(2048, 0); g.fillRect(x - rad, y - rad, rad * 2, rad * 2); g.restore() }
    }
    for (let i = 0; i < 900; i++) { // faint dust stars painted into the sky
      g.fillStyle = `rgba(255,255,255,${0.15 + r() * 0.35})`
      g.fillRect(r() * 2048, r() * 1024, 1, 1)
    }
    const tex = new THREE.CanvasTexture(c)
    tex.colorSpace = THREE.SRGBColorSpace
    this.texs.push(tex)
    const sky = new THREE.Mesh(this.geo('sky', () => new THREE.SphereGeometry(400, 48, 24)), new THREE.MeshBasicMaterial({ map: tex, side: THREE.BackSide, depthWrite: false, toneMapped: false }))
    sky.rotation.z = 0.35
    sky.rotation.x = 0.2
    this.scene.add(sky)
    // two star layers: many faint, a few bright ones that bloom
    const layer = (count: number, size: number, bright: number, seed: number) => {
      const rr = rng(seed)
      const pos = new Float32Array(count * 3)
      const col = new Float32Array(count * 3)
      const tint = [new THREE.Color('#ffffff'), new THREE.Color('#bcd4ff'), new THREE.Color('#ffe2b8'), new THREE.Color('#d6c8ff')]
      for (let i = 0; i < count; i++) {
        const u = rr() * 2 - 1
        const a = rr() * Math.PI * 2
        const rad = 180 + rr() * 150
        const s = Math.sqrt(1 - u * u)
        pos.set([rad * s * Math.cos(a), rad * u, rad * s * Math.sin(a)], i * 3)
        const c2 = tint[Math.floor(rr() * tint.length)]
        const k = bright * (0.5 + rr() * 0.5)
        col.set([c2.r * k, c2.g * k, c2.b * k], i * 3)
      }
      const geo = new THREE.BufferGeometry()
      geo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
      geo.setAttribute('color', new THREE.BufferAttribute(col, 3))
      this.geos.set('stars' + seed, geo)
      const mat = new THREE.PointsMaterial({ size, map: this.glowTex, vertexColors: true, transparent: true, depthWrite: false, blending: THREE.AdditiveBlending, sizeAttenuation: false, toneMapped: false })
      this.mats.set('stars' + seed, mat)
      const p = new THREE.Points(geo, mat)
      this.scene.add(p)
      this.stars.push(p)
    }
    layer(4200, 2.2, 1, 21)
    layer(260, 5, 2.4, 22)
    // a gas giant far behind the station, with a thin atmosphere and a moon
    const map = giantMap(7, ['#5a3412', '#b8742e', '#e8b46a', '#f5dcaa', '#c98a3f', '#7d4a1c'])
    this.texs.push(map)
    const gmat = new THREE.MeshStandardMaterial({ map, roughness: 0.85, metalness: 0, envMapIntensity: 0.3 })
    this.mats.set('giant', gmat)
    const giant = this.mesh(this.sphere(16, 64), gmat, 70, -34, -120)
    giant.rotation.z = 0.35
    this.scene.add(giant)
    this.giant = giant
    const atmo = this.mesh(this.sphere(16.6, 48), this.fresnel('#ffb46a', 3, 0.7), 70, -34, -120)
    this.scene.add(atmo)
    const moon = this.mesh(this.sphere(2.2, 32), this.std('#9aa3b8', 0.95, 0, 0.3), -48, -20, -90)
    this.scene.add(moon)
  }

  // ---------- the AI core ----------

  private buildCore() {
    const sc = this.scene
    const mat = new THREE.ShaderMaterial({ vertexShader: CORE_V, fragmentShader: CORE_F, uniforms: { uTime: { value: 0 }, uBoost: { value: 0 } }, toneMapped: false })
    this.mats.set('core', mat)
    this.core.mat = mat
    const m = this.mesh(this.sphere(CORE_R, 64), mat)
    sc.add(m)
    this.core.mesh = m
    this.hit(m, { kind: 'core' })
    const halo = this.mesh(this.sphere(CORE_R * 1.22, 48), this.fresnel('#5fd8ff', 2.6, 0.75))
    sc.add(halo)
    const corona = this.glowSprite('#3f9fe0', CORE_R * 4.6, 0.3)
    sc.add(corona)
    this.core.corona = corona
    const wire = this.mesh(this.geo('icoW', () => new THREE.IcosahedronGeometry(CORE_R * 1.42, 1)), new THREE.MeshBasicMaterial({ color: new THREE.Color(0.4, 1.4, 2), wireframe: true, transparent: true, opacity: 0.22, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }))
    sc.add(wire)
    this.core.wire = wire
    // three thin energy rings at different tilts
    for (let i = 0; i < 3; i++) {
      const ring = this.mesh(this.torusG(CORE_R * (1.6 + i * 0.13), 0.014, Math.PI * 2, 128), this.glow(0.35 + i * 0.25, 0.95, 1.4, true, 0.7))
      ring.rotation.set(Math.PI / 2 + (i - 1) * 0.55, i * 0.9, 0)
      sc.add(ring)
      this.core.rings.push(ring)
    }
    // six stage arcs: Ingest · Analisis · Sintesis · Keputusan · Eksekusi · Belajar
    const gap = 0.12
    for (let i = 0; i < 6; i++) {
      const arc = this.mesh(this.torusG(ARC_R, 0.06, (Math.PI * 2) / 6 - gap, 40), new THREE.MeshBasicMaterial({ color: new THREE.Color('#1d2a44'), toneMapped: false }))
      arc.rotation.x = Math.PI / 2
      arc.rotation.z = Math.PI / 2 - Math.PI / 6 + gap / 2 + i * ((Math.PI * 2) / 6) // Ingest centred at the front
      arc.position.y = -0.05
      sc.add(arc)
      this.core.arcs.push(arc)
    }
    const light = new THREE.PointLight('#bfe6ff', 34, 0, 1)
    sc.add(light)
    this.core.light = light
    sc.add(new THREE.HemisphereLight('#7f95c9', '#140d22', 0.5))
    const rim = new THREE.DirectionalLight('#8fb3ff', 0.7)
    rim.position.set(-20, 14, 30)
    sc.add(rim)
    const ring = this.hoverRing(CORE_R * 1.95)
    sc.add(ring)
    this.core.ring = ring
    const wave = this.mesh(this.torusG(1, 0.04, Math.PI * 2, 96), new THREE.MeshBasicMaterial({ color: new THREE.Color(0.8, 2, 2.4), transparent: true, opacity: 0, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }))
    wave.rotation.x = Math.PI / 2
    sc.add(wave)
    this.core.wave = wave
  }

  // ---------- the office deck ----------

  private buildOffice() {
    const sc = this.scene
    const hull = this.std('#c7cfdc', 0.32, 0.85, 1)
    const dark = this.std('#2c3445', 0.45, 0.7, 0.8)
    // the deck: a round floor of dark metal panels, its rim and the underside
    sc.add(this.mesh(this.cylG(DECK_R, DECK_R + 0.25, 0.3, 160), hull, 0, DECK_Y - 0.15, 0))
    sc.add(this.mesh(this.cylG(DECK_R - 0.05, DECK_R - 0.05, 0.02, 160), this.std('#161d2b', 0.5, 0.65, 0.7), 0, DECK_Y + 0.005, 0))
    const seam = this.glow(0.12, 0.2, 0.32)
    for (const r of [2.7, 6.3]) {
      const t = this.mesh(this.torusG(r, 0.012, Math.PI * 2, 160), seam, 0, DECK_Y + 0.02, 0)
      t.rotation.x = Math.PI / 2
      sc.add(t)
    }
    for (let k = 0; k < 20; k++) { // radial panel seams
      const a = (k / 20) * Math.PI * 2
      const l = this.mesh(this.boxG(DECK_R - 2.8, 0.004, 0.02), seam)
      l.position.copy(onRing(a, 2.8 + (DECK_R - 2.8) / 2, DECK_Y + 0.02))
      l.rotation.y = -a
      sc.add(l)
    }
    // the walkway: a lit lane in front of the desks
    const lane = this.mesh(this.torusG(WALK_R, 0.32, Math.PI * 2, 160), this.std('#222b3d', 0.35, 0.8, 0.9))
    lane.rotation.x = Math.PI / 2
    lane.scale.z = 0.04
    lane.position.y = DECK_Y + 0.01
    sc.add(lane)
    for (const r of [WALK_R - 0.34, WALK_R + 0.34]) {
      const edge = this.mesh(this.torusG(r, 0.018, Math.PI * 2, 160), this.glow(0.45, 1.25, 1.9), 0, DECK_Y + 0.03, 0)
      edge.rotation.x = Math.PI / 2
      sc.add(edge)
    }
    // the core's pedestal and four light conduits to the walkway
    sc.add(this.mesh(this.cylG(1.15, 1.45, 0.16, 64), dark, 0, DECK_Y + 0.08, 0))
    const ped = this.mesh(this.torusG(1.3, 0.035, Math.PI * 2, 96), this.glow(0.5, 1.4, 2), 0, DECK_Y + 0.17, 0)
    ped.rotation.x = Math.PI / 2
    sc.add(ped)
    for (let i = 0; i < 4; i++) {
      const a = Math.PI / 4 + (i * Math.PI) / 2
      const len = WALK_R - 0.4 - 1.45
      const c = this.mesh(this.boxG(len, 0.01, 0.06), new THREE.MeshBasicMaterial({ color: new THREE.Color(0.4, 1.1, 1.5), toneMapped: false }))
      c.position.copy(onRing(a, 1.45 + len / 2, DECK_Y + 0.02))
      c.rotation.y = -a
      sc.add(c)
      this.core.conduits.push(c)
    }
    // the outer wall: a low panel, panorama glass on space and its frame
    const wallMat = this.mats.get('wall') ?? (() => { const w = new THREE.MeshStandardMaterial({ color: '#3a4458', roughness: 0.4, metalness: 0.75, envMapIntensity: 0.9, side: THREE.DoubleSide }); this.mats.set('wall', w); return w })()
    sc.add(this.mesh(this.cylG(DECK_R, DECK_R, 0.42, 160, true), wallMat, 0, DECK_Y + 0.21, 0))
    const glass = this.mats.get('glass') ?? (() => { const gm = new THREE.MeshStandardMaterial({ color: '#a8d4ff', roughness: 0.05, metalness: 0.3, transparent: true, opacity: 0.06, depthWrite: false, envMapIntensity: 0.7, side: THREE.DoubleSide }); this.mats.set('glass', gm); return gm })()
    sc.add(this.mesh(this.cylG(DECK_R, DECK_R, 1.5, 160, true), glass, 0, DECK_Y + 1.17, 0))
    for (const y of [0.42, 1.92]) {
      const rim = this.mesh(this.torusG(DECK_R, 0.05, Math.PI * 2, 160), hull, 0, DECK_Y + y, 0)
      rim.rotation.x = Math.PI / 2
      sc.add(rim)
    }
    const run = this.mesh(this.torusG(DECK_R - 0.06, 0.016, Math.PI * 2, 160), this.glow(0.5, 1.2, 1.8), 0, DECK_Y + 0.38, 0)
    run.rotation.x = Math.PI / 2
    sc.add(run)
    for (let k = 0; k < 24; k++) { // mullions
      const a = (k / 24) * Math.PI * 2
      sc.add(this.mesh(this.cylG(0.035, 0.035, 1.5, 8), hull, DECK_R * Math.cos(a), DECK_Y + 1.17, DECK_R * Math.sin(a)))
    }
  }

  /** The point on the walkway at angle a. */
  private lanePoint(a: number) { return onRing(a, WALK_R, DECK_Y) }

  // ---------- rooms and astronauts ----------

  private astronaut(accent: string, visor: string, suit = '#eef1f6'): Astro {
    const root = new THREE.Group()
    const body = new THREE.Group()
    root.add(body)
    const white = this.std(suit, 0.6, 0.05, 0.5)
    const acc = this.std(accent, 0.38, 0.25, 0.8)
    const grey = this.std('#9aa3b2', 0.5, 0.4, 0.7)
    const darkM = this.std('#2a3140', 0.6, 0.3, 0.5)
    body.add(this.mesh(this.capsule(0.12, 0.14), white, 0, 0.34, 0))
    body.add(this.mesh(this.boxG(0.13, 0.09, 0.03), acc, 0, 0.37, 0.115))
    body.add(this.mesh(this.sphere(0.012, 8), this.hdr(accent, 3), 0.04, 0.39, 0.132))
    body.add(this.mesh(this.cylG(0.125, 0.125, 0.035, 20), darkM, 0, 0.25, 0))
    body.add(this.mesh(this.sphere(0.128, 32), white, 0, 0.585, 0))
    body.add(this.mesh(this.cylG(0.1, 0.11, 0.04, 20), grey, 0, 0.47, 0))
    const visorGeo = this.geo('visor', () => new THREE.SphereGeometry(0.131, 32, 16, Math.PI / 2 - 0.95, 1.9, 0.82, 1.15))
    let visorMat = this.mats.get('visor' + visor)
    if (!visorMat) { visorMat = new THREE.MeshStandardMaterial({ color: visor, roughness: 0.08, metalness: 1, envMapIntensity: 1.8 }); this.mats.set('visor' + visor, visorMat) }
    body.add(this.mesh(visorGeo, visorMat, 0, 0.585, 0)) // opens to +z: the front
    body.add(this.mesh(this.sphere(0.022, 10), this.hdr(accent, 3.2), 0.105, 0.64, 0.05))
    body.add(this.mesh(this.boxG(0.21, 0.25, 0.1), grey, 0, 0.38, -0.16))
    body.add(this.mesh(this.boxG(0.16, 0.05, 0.02), acc, 0, 0.45, -0.212))
    const flames: THREE.Mesh[] = []
    const flameMat = this.glow(2.6, 1.3, 0.5, true, 0.9)
    for (const s of [-1, 1]) {
      body.add(this.mesh(this.cylG(0.028, 0.035, 0.05, 10), darkM, s * 0.055, 0.235, -0.17))
      const f = this.mesh(this.geo('flame', () => new THREE.ConeGeometry(0.032, 0.2, 12).translate(0, -0.1, 0)), flameMat, s * 0.055, 0.21, -0.17)
      f.visible = false
      body.add(f)
      flames.push(f)
    }
    const arms = [new THREE.Group(), new THREE.Group()] as [THREE.Group, THREE.Group]
    const legs = [new THREE.Group(), new THREE.Group()] as [THREE.Group, THREE.Group]
    let hand = new THREE.Group()
    ;[-1, 1].forEach((s, i) => {
      const arm = arms[i]
      arm.position.set(s * 0.16, 0.45, 0)
      arm.add(this.mesh(this.capsule(0.042, 0.15), white, 0, -0.1, 0))
      arm.add(this.mesh(this.cylG(0.047, 0.047, 0.03, 14), acc, 0, -0.09, 0))
      arm.add(this.mesh(this.sphere(0.045, 12), grey, 0, -0.21, 0))
      body.add(arm)
      if (i === 1) { hand = new THREE.Group(); hand.position.set(0, -0.25, 0.02); arm.add(hand) }
      const leg = legs[i]
      leg.position.set(s * 0.066, 0.21, 0)
      leg.add(this.mesh(this.capsule(0.052, 0.11), white, 0, -0.08, 0))
      leg.add(this.mesh(this.cylG(0.056, 0.056, 0.025, 14), acc, 0, -0.06, 0))
      leg.add(this.mesh(this.boxG(0.09, 0.06, 0.13), darkM, 0, -0.17, 0.02))
      body.add(leg)
    })
    body.position.y = -0.3
    root.scale.setScalar(ASTRO_S)
    return { root, body, arms, legs, flames, hand }
  }

  /** The hologram over a room's console: what the astronaut works on, in the room colour. */
  private hologram(key: CrewKey, color: string): THREE.Group {
    const g = new THREE.Group()
    const solid = this.hdr(color, 2.2)
    const c = new THREE.Color(color)
    const wire = this.mats.get('holoW' + key) as THREE.MeshBasicMaterial | undefined
      ?? (() => { const m = new THREE.MeshBasicMaterial({ color: new THREE.Color(c.r * 2, c.g * 2, c.b * 2), wireframe: true, transparent: true, opacity: 0.85, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }); this.mats.set('holoW' + key, m); return m })()
    const add = (o: THREE.Object3D) => { g.add(o); return o }
    switch (key) {
      case 'sales': // a rising bar chart
        ;[0.12, 0.2, 0.3].forEach((h, i) => add(this.mesh(this.boxG(0.07, h, 0.07), solid, (i - 1) * 0.11, h / 2 - 0.12, 0)))
        break
      case 'wa': { // a chat bubble
        const b = add(this.mesh(this.sphere(0.14, 24), solid))
        b.scale.set(1.25, 0.9, 0.45)
        const tail = add(this.mesh(this.geo('tail', () => new THREE.ConeGeometry(0.05, 0.1, 10)), solid, -0.1, -0.12, 0))
        tail.rotation.z = -0.6
        break
      }
      case 'dealer': // a globe with an orbit
        add(this.mesh(this.sphere(0.13, 18), wire))
        add(this.mesh(this.torusG(0.2, 0.008, Math.PI * 2, 48), solid)).rotation.x = 1.2
        add(this.mesh(this.sphere(0.03, 10), solid, 0.2, 0, 0))
        break
      case 'order': // a crate with an arrow
        add(this.mesh(this.boxG(0.18, 0.18, 0.18), wire, 0, -0.04, 0))
        add(this.mesh(this.geo('arrow', () => new THREE.ConeGeometry(0.07, 0.12, 4)), solid, 0, 0.14, 0))
        break
      case 'followup': // a call: a ring and a dot with waves
        add(this.mesh(this.torusG(0.11, 0.012, Math.PI * 2, 40), solid))
        add(this.mesh(this.torusG(0.18, 0.008, Math.PI, 40), solid)).rotation.z = -Math.PI / 2
        add(this.mesh(this.sphere(0.045, 12), solid))
        break
      case 'stock': // stacked crates
        add(this.mesh(this.boxG(0.13, 0.11, 0.13), wire, -0.08, -0.08, 0))
        add(this.mesh(this.boxG(0.13, 0.11, 0.13), wire, 0.08, -0.08, 0))
        add(this.mesh(this.boxG(0.13, 0.11, 0.13), wire, 0, 0.04, 0))
        break
      case 'credit': // a shield
        add(this.mesh(this.geo('shield', () => new THREE.IcosahedronGeometry(0.16, 1)), wire))
        add(this.mesh(this.sphere(0.05, 12), solid))
        break
      case 'collect': // a stack of coins
        for (let i = 0; i < 4; i++) add(this.mesh(this.cylG(0.09, 0.09, 0.035, 20), this.glow(2.6, 1.9, 0.5), (i % 2) * 0.02, -0.1 + i * 0.05, 0))
        break
      case 'prospect': { // a radar dish
        const dish = add(this.mesh(this.geo('dishS', () => {
          const pts: THREE.Vector2[] = []
          for (let i = 0; i <= 10; i++) { const x = (i / 10) * 0.17; pts.push(new THREE.Vector2(x, x * x * 3)) }
          return new THREE.LatheGeometry(pts, 24)
        }), wire))
        dish.rotation.x = Math.PI / 2.6
        add(this.mesh(this.torusG(0.24, 0.006, Math.PI * 2, 48), solid)).rotation.x = Math.PI / 2
        break
      }
      case 'ready': // the data put together: a cube of cubes
        for (let x = -1; x <= 1; x += 2) for (let y = -1; y <= 1; y += 2) for (let z = -1; z <= 1; z += 2) add(this.mesh(this.boxG(0.08, 0.08, 0.08), solid, x * 0.06, y * 0.06, z * 0.06))
        add(this.mesh(this.boxG(0.26, 0.26, 0.26), wire))
        break
    }
    return g
  }

  private buildRoom(m: CrewMember, i: number) {
    const sc = this.scene
    const a = roomAngle(i)
    const g = new THREE.Group()
    g.position.copy(onRing(a, ROOM_R, DECK_Y))
    g.lookAt(0, DECK_Y, 0) // local +z faces the core and the walkway
    sc.add(g)
    g.updateMatrixWorld()
    const hull = this.std('#c7cfdc', 0.32, 0.85, 1)
    const dark = this.std('#2c3445', 0.45, 0.7, 0.8)
    // the desk's floor pad with the status light round it (its colour is the astronaut's state)
    g.add(this.mesh(this.cylG(DOME, DOME + 0.04, 0.06, 48), this.std('#1a2232', 0.55, 0.6, 0.6), 0, FLOOR / 2, 0))
    const status = this.own('status' + m.key)
    const ringM = this.mesh(this.torusG(DOME + 0.02, 0.03, Math.PI * 2, 72), status, 0, FLOOR, 0)
    ringM.rotation.x = Math.PI / 2
    g.add(ringM)
    // the desk, a side cabinet, the screen and the hologram over it
    g.add(this.mesh(this.boxG(0.78, 0.36, 0.3), dark, 0, FLOOR + 0.18, 0.5))
    g.add(this.mesh(this.boxG(0.84, 0.03, 0.36), hull, 0, FLOOR + 0.375, 0.5))
    g.add(this.mesh(this.boxG(0.2, 0.32, 0.26), dark, 0.52, FLOOR + 0.16, 0.1))
    const screen = this.own('screen' + m.key)
    const scr = this.mesh(this.boxG(0.62, 0.02, 0.28), screen, 0, FLOOR + 0.42, 0.5)
    scr.rotation.x = -0.55
    g.add(scr)
    const holo = this.hologram(m.key, m.color)
    holo.position.set(0, FLOOR + 1.05, 0.5)
    g.add(holo)
    const cone = this.mesh(this.geo('holoCone', () => new THREE.ConeGeometry(0.22, 0.42, 24, 1, true)), this.fresnel(m.color, 1.2, 0.35), 0, FLOOR + 0.66, 0.5)
    cone.rotation.x = Math.PI
    g.add(cone)
    // a light pole with the desk's colour
    g.add(this.mesh(this.cylG(0.02, 0.02, 1.2, 8), hull, -0.62, FLOOR + 0.6, -0.15))
    g.add(this.mesh(this.sphere(0.05, 12), this.hdr(m.color, 3), -0.62, FLOOR + 1.22, -0.15))
    const lamp = this.glowSprite(m.color, 1.6, 0.1)
    lamp.position.set(0, FLOOR + 0.9, 0)
    g.add(lamp)
    // the astronaut, at the desk
    const astro = this.astronaut(m.color, m.key === 'ready' ? '#e3b04b' : '#1c2a48', m.key === 'ready' ? '#ffffff' : '#eef1f6')
    const home = new THREE.Vector3(0, FLOOR + 0.3 * ASTRO_S, -0.12)
    astro.root.position.copy(home)
    g.add(astro.root)
    // hit area and hover ring
    const hitM = this.mesh(this.cylG(DOME + 0.1, DOME + 0.1, 1.7, 12), new THREE.MeshBasicMaterial({ visible: false }), 0, 0.85, 0)
    g.add(hitM)
    this.hit(hitM, { kind: 'room', key: m.key })
    const ring = this.hoverRing(DOME + 0.18)
    ring.position.y = 0.04
    g.add(ring)
    if (m.key === 'ready') { this.caps.position.set(0.55, FLOOR + 0.05, -0.45); g.add(this.caps) }
    const beam = new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()]), new THREE.LineBasicMaterial({ color: new THREE.Color(0.6, 1.8, 2.4), transparent: true, opacity: 0.7, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }))
    beam.visible = false
    sc.add(beam)
    const room: Room = { m, i, g, a: astro, holo, screen, status, lamp, ring, label: null, labelText: '', beam, carry: null, work: { state: 'idle', text: m.does }, was: 'idle', walk: null, gave: false, home }
    this.rooms.push(room)
    this.roomBy.set(m.key, room)
  }

  private buildBelt() {
    const n = 520
    const geo = this.geo('rock', () => new THREE.IcosahedronGeometry(0.09, 0))
    const mat = this.std('#8d8478', 0.95, 0.05, 0.2)
    const belt = new THREE.InstancedMesh(geo, mat, n)
    const r = rng(31)
    const m = new THREE.Matrix4()
    const q = new THREE.Quaternion()
    const e = new THREE.Euler()
    const s = new THREE.Vector3()
    const p = new THREE.Vector3()
    for (let i = 0; i < n; i++) {
      const a = r() * Math.PI * 2
      const rad = BELT_IN + r() * (BELT_OUT - BELT_IN)
      p.set(rad * Math.cos(a), (r() - 0.5) * 0.5, rad * Math.sin(a))
      e.set(r() * 6, r() * 6, r() * 6)
      q.setFromEuler(e)
      const k = 0.4 + r() * r() * 2.2
      s.set(k, k * (0.6 + r() * 0.5), k)
      m.compose(p, q, s)
      belt.setMatrixAt(i, m)
    }
    this.scene.add(belt)
    this.belt = belt
  }

  // ---------- data in ----------

  /** What every astronaut is doing (station.ts crewStatus). */
  setCrew(s: CrewStatus) {
    this.live = s
    if (this.show) return // the relay show gives the data back when it ends
    for (const r of this.rooms) r.work = s[r.m.key] ?? r.work
  }

  /** Plays the relay 1 → 10 as a show: each astronaut analyses, then walks its result to the next desk. Nothing is
   * analysed; the desks show what the data says (their lines). Not while a real cycle runs. */
  replay() {
    if (!this.renderer || this.show || this.cycle.running) return
    for (const r of this.rooms) {
      r.gave = false
      r.work = { state: 'waiting', text: 'antri' }
      r.was = 'waiting' // no relay from the state the show replaces
    }
    this.show = { i: 0, t: 0, phase: 'read' }
    this.o.onReplay?.(true)
  }

  get replaying() { return !!this.show }

  private endShow() {
    this.show = null
    for (const r of this.rooms) {
      r.work = this.live?.[r.m.key] ?? { state: 'idle', text: r.m.does }
      r.was = r.work.state
    }
    this.o.onReplay?.(false)
  }

  private stepShow(dt: number) {
    const sh = this.show
    if (!sh) return
    const r = this.rooms[sh.i]
    const line = (k: CrewKey) => { const w = this.live?.[k]; return w && w.state === 'idle' ? w.text : CREW_OF[k].does }
    sh.t += dt
    if (sh.phase === 'read') {
      if (r.work.state !== 'reading') r.work = { state: 'reading', text: 'menganalisa…' }
      if (sh.t >= READ_S) {
        r.work = { state: 'done', text: line(r.m.key) }
        sh.t = 0
        sh.phase = r.m.key === 'ready' ? 'end' : 'walk'
      }
    } else if (sh.phase === 'walk') {
      const next = this.rooms[sh.i + 1]
      if (next.gave || sh.t > 30) { sh.i++; sh.t = 0; sh.phase = 'read' }
    } else if (sh.t > 3.2) this.endShow()
  }

  /** The proposals waiting for a decision → a capsule each on the Data siap floor (max 12), in the colour of the
   * astronaut that made it. */
  setDock(keys: CrewKey[]) {
    if (!this.renderer) return
    const list = keys.slice(0, 12)
    const sig = list.join('|')
    if (sig === this.caps.userData.sig) return
    this.caps.userData.sig = sig
    this.caps.clear()
    list.forEach((k, i) => {
      const c = this.mesh(this.capsule(0.045, 0.08), this.hdr(CREW_OF[k].color, 1.8))
      c.position.set((i % 4) * 0.14 - 0.2, 0.12, Math.floor(i / 4) * 0.14)
      c.rotation.z = Math.PI / 2
      c.userData.phase = i * 0.7
      this.caps.add(c)
    })
  }

  setCycle(c: CycleState) {
    const was = this.cycle.running
    const stageWas = this.cycle.stage
    this.cycle = c
    if (!this.renderer) return
    if (c.running && this.show) this.endShow() // a real cycle takes over from the show
    if (!c.running && was) this.core.waveT = 0 // learn: a shock wave from the core
    if (c.running && c.stage === 5 && stageWas !== 5) this.core.waveT = 0
  }

  // ---------- animation ----------

  private holoPos(r: Room, out = new THREE.Vector3()) { return r.holo.getWorldPosition(out) }

  private setCarry(r: Room, on: boolean) {
    if (r.carry) {
      r.carry.traverse((o) => { if (o instanceof THREE.Sprite) o.material.dispose() }) // its glow (the capsule's are shared)
      r.carry.removeFromParent()
      r.carry = null
    }
    if (!on) return
    const m = this.mesh(this.capsule(0.05, 0.09), this.hdr(r.m.color, 2.4))
    m.add(this.glowSprite(r.m.color, 0.5, 0.55)) // seen from across the deck
    r.a.hand.add(m)
    r.carry = m
  }

  private stepRoom(r: Room, dt: number) {
    const { state, text } = r.work
    const A = r.a
    const i = r.i
    const t = this.t
    // status light, screen and lamp
    const c = new THREE.Color(r.m.color)
    const pulse = 0.5 + 0.5 * Math.sin(t * 5 + i)
    if (state === 'reading') r.status.color.setRGB(c.r * (1.6 + pulse * 1.4), c.g * (1.6 + pulse * 1.4), c.b * (1.6 + pulse * 1.4))
    else if (state === 'done') r.status.color.setRGB(0.3, 2, 1.25)
    else if (state === 'failed') r.status.color.setRGB(2.4 * (0.5 + pulse * 0.5), 0.25, 0.25)
    else if (state === 'waiting') r.status.color.setRGB(1.6 * (0.6 + 0.4 * Math.sin(t * 2 + i)), 1.05 * (0.6 + 0.4 * Math.sin(t * 2 + i)), 0.25)
    else r.status.color.setRGB(c.r * 0.9, c.g * 0.9, c.b * 0.9)
    const flick = state === 'reading' ? 1.4 + Math.random() * 0.6 : state === 'waiting' ? 0.45 : 0.9 + Math.sin(t * 1.5 + i) * 0.1
    r.screen.color.setRGB(c.r * flick, c.g * flick, c.b * flick)
    r.lamp.material.opacity = state === 'reading' ? 0.22 + pulse * 0.1 : state === 'waiting' ? 0.05 : 0.1
    // hologram
    const fast = state === 'reading'
    r.holo.rotation.y += dt * (fast ? 2.6 : state === 'waiting' ? 0.2 : 0.7)
    r.holo.position.y = FLOOR + 1.05 + Math.sin(t * 1.8 + i) * 0.03
    r.holo.scale.setScalar(fast ? 1.15 + Math.sin(t * 6) * 0.06 : state === 'waiting' ? 0.8 : 1)
    if (r.walk) this.stepWalk(r, dt)
    else { // at the desk: stands at the console and works it
      A.root.position.y = r.home.y + Math.sin(t * 1.4 + i) * 0.012
      A.body.rotation.y = state === 'waiting' ? Math.sin(t * 0.7 + i) * 0.6 : Math.sin(t * 0.5 + i) * 0.12
      A.body.rotation.x = lerp(A.body.rotation.x, state === 'failed' ? 0.25 : state === 'reading' ? 0.12 : 0.04, 1 - Math.exp(-dt * 4))
      if (state === 'done') { // finished: a thumbs-up over the desk, then rests
        A.arms[1].rotation.x = -1.6 + Math.sin(t * 2 + i) * 0.08
        A.arms[0].rotation.x = Math.sin(t * 2) * 0.06
      } else if (state === 'failed') A.arms[0].rotation.x = A.arms[1].rotation.x = 0.15
      else if (state === 'waiting') A.arms[0].rotation.x = A.arms[1].rotation.x = Math.sin(t * 1.2 + i) * 0.15
      else { // typing: fast while reading
        const sp = fast ? 14 : 5
        A.arms[0].rotation.x = -1.15 + Math.sin(t * sp + i) * (fast ? 0.12 : 0.06)
        A.arms[1].rotation.x = -1.15 + Math.sin(t * sp + i + 1.7) * (fast ? 0.12 : 0.06)
      }
      A.legs[0].rotation.x = lerp(A.legs[0].rotation.x, 0, 1 - Math.exp(-dt * 6))
      A.legs[1].rotation.x = lerp(A.legs[1].rotation.x, 0, 1 - Math.exp(-dt * 6))
    }
    // a desk that just finished: sparks, and the astronaut walks its result to the next desk (the last one: a burst)
    if (state !== r.was) {
      const at = r.g.localToWorld(new THREE.Vector3(0, 1.3, 0.3))
      if (state === 'done' && r.m.key !== 'ready') {
        this.spawnBits(at, 10, r.m.color, 0.3)
        this.startWalk(r, this.rooms[r.i + 1])
      } else if (state === 'done') this.spawnBits(at, 28, '#ffcf5a', 0.6)
      if (state === 'reading') r.gave = false
      r.was = state
    }
    // the reading beam to the core
    r.beam.visible = state === 'reading'
    if (r.beam.visible) {
      const h = this.holoPos(r)
      const at = (r.beam.geometry as THREE.BufferGeometry).attributes.position as THREE.BufferAttribute
      at.setXYZ(0, h.x, h.y, h.z)
      const k = CORE_R / Math.max(0.01, h.length())
      at.setXYZ(1, h.x * k, h.y * k, h.z * k)
      at.needsUpdate = true
      ;(r.beam.material as THREE.LineBasicMaterial).opacity = 0.35 + Math.random() * 0.5
    }
    // the sign
    const sig = state + '|' + text
    if (sig !== r.labelText) {
      r.labelText = sig
      this.dropSprite(r.label)
      r.label = this.sign(r.m, state, text)
      this.overlay.add(r.label)
    }
    if (r.label) r.label.position.copy(r.g.localToWorld(new THREE.Vector3(0, FLOOR + 1.75, 0.15)))
  }

  // ---------- the relay: walking a result to the next desk ----------

  /** The way from a desk to the next one: round its desk to the walkway, along it, to the front of the next desk. */
  private route(from: Room, to: Room): THREE.Vector3[] {
    const y = from.home.y
    const w = (r: Room, x: number, z: number) => r.g.localToWorld(new THREE.Vector3(x, y, z)).setY(DECK_Y + y)
    const goal = w(to, 0, ROOM_R - WALK_R - 0.05)
    const sides = [0.85, -0.85].map((x) => [w(from, x, -0.12), w(from, x, ROOM_R - WALK_R)])
    const [side, out] = sides.sort((p, q) => p[1].distanceTo(goal) - q[1].distanceTo(goal))[0]
    const pts = [w(from, 0, -0.12), side, out]
    let a0 = Math.atan2(out.z, out.x)
    const a1 = Math.atan2(goal.z, goal.x)
    let da = a1 - a0
    while (da > Math.PI) da -= Math.PI * 2
    while (da < -Math.PI) da += Math.PI * 2
    const n = Math.max(2, Math.ceil(Math.abs(da) / 0.12))
    for (let k = 1; k < n; k++) { a0 += da / n; pts.push(this.lanePoint(a0).setY(DECK_Y + y)) }
    pts.push(goal)
    return pts
  }

  private startWalk(r: Room, to: Room | undefined) {
    if (!to || r.walk || this.reduced) { if (to) this.handOver(r, to); return }
    const pts = this.route(r, to)
    const at = [0]
    for (let k = 1; k < pts.length; k++) at.push(at[k - 1] + pts[k].distanceTo(pts[k - 1]))
    this.scene.attach(r.a.root) // walks in the deck's frame, turned by its heading alone
    const ahead = new THREE.Vector3(0, 0, 1).applyQuaternion(r.a.root.quaternion)
    r.a.root.rotation.set(0, Math.atan2(ahead.x, ahead.z), 0)
    this.setCarry(r, true)
    r.walk = { to, pts, at, d: 0, phase: 'go', wait: 0 }
  }

  /** The result reaches the next desk: it flies from the hand into that desk's hologram. */
  private handOver(r: Room, to: Room) {
    const from = r.carry ? r.carry.getWorldPosition(new THREE.Vector3()) : this.holoPos(r)
    this.setCarry(r, false)
    this.spawnPulse(() => from.clone(), () => this.holoPos(to), r.m.color, 0.7, 0.5, 0.4)
    this.spawnBits(this.holoPos(to), 12, r.m.color, 0.25)
    to.gave = true
  }

  private stepWalk(r: Room, dt: number) {
    const w = r.walk!
    const A = r.a
    const t = this.t
    const total = w.at[w.at.length - 1]
    if (w.phase === 'give') {
      w.wait += dt
      A.arms[1].rotation.x = -1.5
      A.legs[0].rotation.x = A.legs[1].rotation.x = 0
      if (w.wait > GIVE_S * 0.45 && r.carry) this.handOver(r, w.to)
      if (w.wait > GIVE_S) w.phase = 'back'
      return
    }
    w.d += dt * WALK_SPEED * (w.phase === 'go' ? 1 : -1)
    const done = w.phase === 'go' ? w.d >= total : w.d <= 0
    w.d = Math.max(0, Math.min(total, w.d))
    let k = 1
    while (k < w.at.length - 1 && w.at[k] < w.d) k++
    const a = w.pts[k - 1]
    const b = w.pts[k]
    const seg = Math.max(1e-6, w.at[k] - w.at[k - 1])
    const pos = a.clone().lerp(b, (w.d - w.at[k - 1]) / seg)
    A.root.position.copy(pos)
    A.root.position.y += Math.abs(Math.sin(t * 9)) * 0.03 // a step's bounce
    const dir = (w.phase === 'go' ? b.clone().sub(a) : a.clone().sub(b))
    if (dir.lengthSq() > 1e-6) {
      const want = Math.atan2(dir.x, dir.z)
      let diff = want - A.root.rotation.y
      while (diff > Math.PI) diff -= Math.PI * 2
      while (diff < -Math.PI) diff += Math.PI * 2
      A.root.rotation.y += diff * (1 - Math.exp(-dt * 12))
    }
    A.body.rotation.y = 0
    A.body.rotation.x = 0.08
    const swing = Math.sin(t * 9) * 0.55
    A.legs[0].rotation.x = swing
    A.legs[1].rotation.x = -swing
    A.arms[0].rotation.x = -swing * 0.7
    A.arms[1].rotation.x = r.carry ? -1.2 : swing * 0.7 // carries the capsule in front
    // a trail of sparks in the desk's colour shows the way the result goes
    w.wait -= dt
    if (w.phase === 'go' && w.wait <= 0) { w.wait = 0.14; this.spawnBits(pos.clone().setY(DECK_Y + 0.08), 1, r.m.color, 0.04) }
    if (!done) return
    if (w.phase === 'go') { // face the next desk and hand over
      const face = this.holoPos(w.to).sub(A.root.position)
      A.root.rotation.y = Math.atan2(face.x, face.z)
      w.phase = 'give'
      w.wait = 0
      return
    }
    r.walk = null // home: back at its own desk
    r.g.attach(A.root)
    A.root.position.copy(r.home)
    A.root.rotation.set(0, 0, 0)
  }

  private spawnBits(at: THREE.Vector3, n: number, color: string, r: number) {
    const mat = this.hdr(color, 2.2)
    for (let i = 0; i < n; i++) {
      const m = this.mesh(this.sphere(0.03, 6), mat)
      const v = new THREE.Vector3(Math.random() - 0.5, Math.random() * 0.8 + 0.2, Math.random() - 0.5).normalize()
      m.position.copy(at).addScaledVector(v, r * 0.9)
      this.scene.add(m)
      this.bits.push({ m, v: v.multiplyScalar(0.9 + Math.random() * 0.9), life: 0, max: 0.9 + Math.random() * 0.5 })
    }
  }

  private spawnPulse(from: () => THREE.Vector3, to: () => THREE.Vector3, color: string, dur: number, size = 0.45, lift = -1) {
    let p = this.pulses.find((q) => !q.s.visible)
    if (!p) {
      const s = this.glowSprite(color, size)
      this.scene.add(s)
      p = { s, from, to, t: 0, dur, lift: 0 }
      this.pulses.push(p)
    }
    p.s.visible = true
    p.s.material.color.set(color)
    p.s.scale.setScalar(size)
    p.from = from; p.to = to; p.t = 0; p.dur = dur; p.lift = lift >= 0 ? lift : 0.5 + Math.random() * 1.2
  }

  private stepFx(dt: number) {
    const g = new THREE.Vector3(0, -0.6, 0)
    this.bits = this.bits.filter((b) => {
      b.life += dt
      b.m.position.addScaledVector(b.v, dt)
      b.v.addScaledVector(g, dt)
      b.m.scale.setScalar(Math.max(0.05, 1 - b.life / b.max))
      if (b.life >= b.max) { b.m.removeFromParent(); return false }
      return true
    })
    for (const p of this.pulses) {
      if (!p.s.visible) continue
      p.t += dt / p.dur
      if (p.t >= 1) { p.s.visible = false; continue }
      const a = p.from(); const b = p.to()
      const mid = a.clone().lerp(b, 0.5); mid.y += p.lift
      const e = p.t
      p.s.position.copy(a.multiplyScalar((1 - e) ** 2)).addScaledVector(mid, 2 * (1 - e) * e).addScaledVector(b, e * e)
    }
  }

  private stepCycle(dt: number) {
    const c = this.cycle
    const core = () => new THREE.Vector3(0, 0, 0)
    // stage arcs
    this.core.arcs.forEach((arc, i) => {
      const m = arc.material as THREE.MeshBasicMaterial
      if (c.running && i === c.stage) m.color.setRGB(2.2 + Math.sin(this.t * 8) * 0.8, 2.4 + Math.sin(this.t * 8) * 0.8, 2.6)
      else if (c.running ? i < c.stage : c.hasCycle) m.color.setRGB(0.25, 1.5, 1.15)
      else m.color.set('#1d2a44')
    })
    // data: from the rooms to the core (fast from rooms that read), from the core to Data siap
    const workers = this.rooms.filter((r) => r.m.key !== 'ready')
    const reading = workers.filter((r) => r.work.state === 'reading')
    this.pulseClock -= dt
    if (this.pulseClock <= 0 && dt > 0) {
      const busy = reading.length > 0
      this.pulseClock = busy ? 0.16 : c.running ? 0.35 : 0.9
      const pool = busy ? reading : workers.filter((r) => r.work.state !== 'waiting')
      const r = pool[Math.floor(Math.random() * pool.length)]
      if (r) this.spawnPulse(() => this.holoPos(r), core, r.m.color, busy ? 1.1 : 1.9, busy ? 0.45 : 0.32)
    }
    this.readyClock -= dt
    const ready = this.roomBy.get('ready')!
    if (this.readyClock <= 0 && dt > 0 && ready.work.state !== 'waiting') {
      this.readyClock = ready.work.state === 'reading' ? 0.3 : 2.4
      this.spawnPulse(core, () => this.holoPos(ready), '#ffcf5a', 1.3, 0.42, 1.2)
    }
    // shock wave (Belajar)
    const w = this.core.wave!
    this.core.waveT += dt
    const wk = this.core.waveT / 1.6
    w.visible = wk < 1
    if (wk < 1) { w.scale.setScalar(CORE_R + wk * 7); (w.material as THREE.MeshBasicMaterial).opacity = (1 - wk) * 0.9 }
  }

  private step(dt: number) {
    this.t += dt
    const mat = this.core.mat!
    mat.uniforms.uTime.value = this.t
    const busy = this.cycle.running || this.rooms.some((r) => r.work.state === 'reading')
    const boost = busy ? 0.6 + Math.sin(this.t * 5) * 0.25 : 0.1 + Math.sin(this.t * 1.2) * 0.06
    mat.uniforms.uBoost.value = boost
    this.core.mesh!.scale.setScalar(1 + boost * 0.04)
    this.core.corona!.material.opacity = 0.22 + boost * 0.25
    this.core.light!.intensity = 26 + boost * 14
    this.core.wire!.rotation.y += dt * (busy ? 0.5 : 0.12)
    this.core.wire!.rotation.x += dt * 0.05
    this.core.rings.forEach((r, i) => { r.rotation.z += dt * (0.3 + i * 0.17) * (i % 2 ? -1 : 1) * (busy ? 3 : 1) })
    this.core.conduits.forEach((c, i) => { (c.material as THREE.MeshBasicMaterial).color.setRGB(0.3 + boost * 0.6, 0.9 + boost + Math.sin(this.t * 4 + i) * 0.15, 1.3 + boost * 0.6) })
    if (this.giant) this.giant.rotation.y += dt * 0.01
    if (this.belt) this.belt.rotation.y -= dt * 0.008
    this.stars[0].rotation.y += dt * 0.002
    this.stepShow(dt)
    for (const r of this.rooms) this.stepRoom(r, dt)
    this.caps.children.forEach((m) => { m.position.y = 0.12 + Math.sin(this.t * 2 + (m.userData.phase as number)) * 0.03; m.rotation.y += dt })
    this.stepCycle(dt)
    this.stepFx(dt)
  }

  private frame = (now: number) => {
    this.raf = requestAnimationFrame(this.frame)
    const st = this.o.stage
    if (!this.renderer || !this.composer || !st.isConnected || st.getClientRects().length === 0) return
    const w = st.clientWidth
    if (!w) return
    const h = st.clientHeight || STATION_H
    if (w !== this.w || h !== this.h) this.resize(w, h)
    const dt = this.last ? Math.min(0.05, (now - this.last) / 1000) : 0
    this.last = now
    this.adapt(dt)
    this.step(this.reduced ? 0 : dt) // reduced motion: everything placed, nothing moves
    const yaw = this.yaw + this.sway()
    const d = this.dist * this.zoom
    this.camera.position.set(
      this.target.x + d * Math.sin(yaw) * Math.cos(this.pitch),
      this.target.y + d * Math.sin(this.pitch),
      this.target.z + d * Math.cos(yaw) * Math.cos(this.pitch),
    )
    this.camera.lookAt(this.target)
    this.render()
    const r = this.renderer
    r.autoClear = false
    r.clearDepth()
    r.render(this.overlay, this.camera)
    r.autoClear = true
  }

  private render() {
    const r = this.renderer!
    if (!this.direct) {
      try {
        this.composer!.render()
      } catch (e) {
        this.fallback(e)
      }
    }
    if (this.direct) {
      r.setRenderTarget(null)
      r.render(this.scene, this.camera)
      return
    }
    this.frames++
    if ((this.frames === 6 || this.frames === 30 || this.frames === 120) && this.blank()) this.fallback('canvas blank after the HD pass')
  }

  /** The core is always on screen and bright: black pixels there mean the HD pass drew nothing. */
  private blank() {
    const r = this.renderer!
    const gl = r.getContext()
    const p = new THREE.Vector3(0, 0, 0).project(this.camera)
    if (Math.abs(p.x) > 1 || Math.abs(p.y) > 1) return false
    const x = Math.round(((p.x + 1) / 2) * gl.drawingBufferWidth) - 2
    const y = Math.round(((p.y + 1) / 2) * gl.drawingBufferHeight) - 2
    const px = new Uint8Array(5 * 5 * 4)
    r.setRenderTarget(null)
    gl.readPixels(Math.max(0, x), Math.max(0, y), 5, 5, gl.RGBA, gl.UNSIGNED_BYTE, px)
    let max = 0
    for (let i = 0; i < px.length; i += 4) max = Math.max(max, px[i], px[i + 1], px[i + 2])
    return max < 24
  }

  private fallback(why: unknown) {
    if (this.direct) return
    this.direct = true
    console.warn('Stasiun Orbit: HD pass unavailable, rendering directly', why)
  }

  private resize(w: number, h: number) {
    this.w = w
    this.h = h
    this.renderer!.setSize(w, h)
    this.composer!.setPixelRatio(this.renderer!.getPixelRatio())
    this.composer!.setSize(w, h)
    const aspect = w / h
    this.camera.aspect = aspect
    const tan = Math.tan(THREE.MathUtils.degToRad(this.camera.fov / 2))
    this.dist = Math.min(80, Math.max(6.4 / tan, 9.0 / (tan * aspect)))
    this.camera.updateProjectionMatrix()
  }

  /** HD by default; step down (pixel ratio, then bloom) when this device cannot keep up. */
  private adapt(dt: number) {
    if (!dt || this.perf.level === 0) return
    this.perf.n++
    this.perf.sum += dt
    if (this.perf.n < 90) return
    const avg = this.perf.sum / this.perf.n
    this.perf.n = 0
    this.perf.sum = 0
    if (avg < 1 / 40) return
    this.perf.level--
    if (this.perf.level === 1) this.renderer!.setPixelRatio(1)
    else if (this.bloom) this.bloom.enabled = false
    this.w = 0 // resize on the next frame
  }

  /** The camera drifts a little until someone turns it. */
  private sway() { return this.moved || this.reduced ? 0 : Math.sin(this.t * 0.06) * 0.3 }

  // ---------- pointer ----------

  private bind() {
    const cv = this.o.canvas
    cv.style.touchAction = 'pan-y'
    const ray = new THREE.Raycaster()
    const ndc = new THREE.Vector2()
    const pick = (e: PointerEvent | MouseEvent): StationTarget | null => {
      const r = cv.getBoundingClientRect()
      ndc.set(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1)
      ray.setFromCamera(ndc, this.camera)
      const hit = ray.intersectObjects(this.hits, false)[0]
      return (hit?.object.userData.target as StationTarget | undefined) ?? null
    }
    const keyOf = (t: StationTarget | null) => (!t ? null : t.kind === 'room' ? 'r:' + t.key : t.kind)
    const highlight = (k: string | null) => {
      if (k === this.hover) return
      this.hover = k
      for (const r of this.rooms) r.ring.visible = 'r:' + r.m.key === k
      this.core.ring!.visible = k === 'core'
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
          if (!this.moved) { this.yaw += this.sway(); down.yaw = this.yaw + dx * 0.005; this.moved = true }
          this.yaw = down.yaw - dx * 0.005
          this.pitch = Math.max(0.12, Math.min(1.35, down.pitch + dy * 0.004))
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
    const reset = () => { this.yaw = 0; this.pitch = PITCH0; this.zoom = 1; this.moved = false }
    // pinch on a trackpad (ctrl + wheel) or ctrl/⌘ + wheel zooms; a plain wheel keeps scrolling the page
    const wheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return
      e.preventDefault()
      this.zoom = Math.max(0.35, Math.min(1.5, this.zoom * Math.exp(e.deltaY * 0.004)))
    }
    cv.addEventListener('pointerdown', pdown)
    cv.addEventListener('pointermove', pmove)
    cv.addEventListener('pointerup', pup)
    cv.addEventListener('pointerleave', leave)
    cv.addEventListener('dblclick', reset)
    cv.addEventListener('wheel', wheel, { passive: false })
    this.off.push(() => {
      cv.removeEventListener('pointerdown', pdown)
      cv.removeEventListener('pointermove', pmove)
      cv.removeEventListener('pointerup', pup)
      cv.removeEventListener('pointerleave', leave)
      cv.removeEventListener('dblclick', reset)
      cv.removeEventListener('wheel', wheel)
    })
  }

  /** Zoom from the buttons: k < 1 closer. */
  zoomBy(k: number) { this.zoom = Math.max(0.35, Math.min(1.5, this.zoom * k)) }
  resetView() { this.yaw = 0; this.pitch = PITCH0; this.zoom = 1; this.moved = false }

  start() {
    if (this.ok && !this.raf) this.raf = requestAnimationFrame(this.frame)
  }

  destroy() {
    cancelAnimationFrame(this.raf)
    this.raf = 0
    this.off.forEach((f) => f())
    for (const sc of [this.scene, this.overlay]) {
      sc.traverse((o) => {
        if (o instanceof THREE.Sprite) { if (o.material.map !== this.glowTex) o.material.map?.dispose(); o.material.dispose() }
        else if (o instanceof THREE.Mesh || o instanceof THREE.Line || o instanceof THREE.Points) {
          const m = o.material as THREE.Material & { map?: THREE.Texture | null }
          m.map?.dispose()
          m.dispose()
          o.geometry.dispose()
        }
      })
    }
    this.geos.forEach((g) => g.dispose())
    this.mats.forEach((m) => m.dispose())
    this.texs.forEach((t) => t.dispose())
    this.composer?.dispose()
    this.renderer?.dispose()
  }
}
