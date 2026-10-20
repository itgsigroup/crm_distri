import * as THREE from 'three'
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js'
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js'
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js'
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js'
import { RoomEnvironment } from 'three/examples/jsm/environments/RoomEnvironment.js'
import { AGENT_COLOR, AGENT_NAME, type AgentKey, type Comet, type FlyKind, type Planet, type StationPlan } from './station'

// Stasiun Orbit: a space miniature in three.js, rendered HD (MSAA, ACES tone mapping, bloom, PBR with an environment
// map). The AI core (the Orchestrator) burns in the middle inside the station ring; dealers are planets on orbits
// (the nearer the jadwal order, the closer to the core); the six agents are astronauts. Four of them fly out to
// planets for their tasks and bring a proposal capsule back to the Dek Keputusan, where only the human commander
// decides. AI Stok works the cargo bay, AI Prospek the passive radar that scans comets — new numbers that wrote in
// first (the system never hunts for leads). While an Orchestrator cycle runs, every astronaut gathers at the core and
// the six stage arcs light up in turn. Everything comes from the plan (real data); clicking opens the screen.

export type StationTarget =
  | { kind: 'planet'; planet: Planet }
  | { kind: 'core' }
  | { kind: 'dock' }
  | { kind: 'cargo' }
  | { kind: 'comms' }
  | { kind: 'comet'; comet: Comet }
  | { kind: 'agent'; agent: AgentKey; status: string }
  | { kind: 'commander' }

export interface CycleState { running: boolean; stage: number; hasCycle: boolean }

/** One agent's part in the cycle being followed (GET /cycles/{id}/progress): the astronaut acts it out. */
export type WorkState = 'waiting' | 'reading' | 'done' | 'failed'
export interface AgentWork { state: WorkState; text: string }
export type Work = Partial<Record<AgentKey, AgentWork>>

export interface StationOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  onPick: (t: StationTarget) => void
  onHover: (t: StationTarget | null, x: number, y: number) => void
}

export const STATION_H = 540

// world layout (units)
const CORE_R = 1.3
const ARC_R = 2.05 // the six stage arcs
const GATHER_R = 2.75 // astronauts during a cycle
const RING_R = 3.7 // the station ring
const ORBIT_IN = 6.1
const ORBIT_OUT = 12.4
const BELT_IN = 13.6
const BELT_OUT = 14.6
const COMET_A = 16.2
const COMET_B = 10.5
const ASTRO_S = 1.55
const SPIN = 0.028 // planets turn around the core together (rad/s), so they never run into each other
const PITCH0 = 0.46

const ANG = { radar: 0, dock: Math.PI / 2, cargo: Math.PI, comms: -Math.PI / 2 }
const onRing = (a: number, r = RING_R, y = 0) => new THREE.Vector3(r * Math.cos(a), y, r * Math.sin(a))
const HOME: Record<AgentKey, number> = { order: Math.PI / 4, collect: (3 * Math.PI) / 4, credit: -(3 * Math.PI) / 4, followup: -Math.PI / 4, stock: ANG.cargo, prospect: ANG.radar }

const TONE_GLOW: Record<Planet['tone'], string> = { good: '#4fd6ff', warn: '#ffb547', bad: '#ff4d4d', neutral: '#b6c1d6' }

const clamp01 = (v: number) => Math.max(0, Math.min(1, v))
const smooth = (v: number) => { const x = clamp01(v); return x * x * (3 - 2 * x) }
const lerp = (a: number, b: number, k: number) => a + (b - a) * k

function rng(seed: number) {
  let s = seed >>> 0 || 1
  return () => { s = (s * 1664525 + 1013904223) >>> 0; return s / 4294967296 }
}
function hash(str: string) {
  let h = 2166136261
  for (let i = 0; i < str.length; i++) { h ^= str.charCodeAt(i); h = Math.imul(h, 16777619) }
  return h >>> 0
}

// ---------- procedural planet surfaces ----------

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

type Style = 'ocean' | 'bands' | 'rock' | 'ice' | 'gold'
const PALETTE: Record<Style, string[]> = {
  ocean: ['#06224f', '#0b4d8c', '#1f8fc9', '#3aa86b', '#8a9a4e', '#e9f1f4'],
  bands: ['#5a3412', '#b8742e', '#e8b46a', '#f5dcaa', '#c98a3f', '#7d4a1c'],
  rock: ['#2a0d08', '#6b1f12', '#a8402a', '#d9774f', '#f0b08a'],
  ice: ['#8aa4c4', '#b9cde3', '#e4eef8', '#ffffff'],
  gold: ['#4a2f05', '#a8741a', '#f2c14e', '#fff0bf', '#d49b2c', '#7a5210'],
}
const NEUTRAL_BANDS = ['#3c4256', '#6c7591', '#a8b0c7', '#d9deea', '#8a93ad', '#4d556d']

function styleOf(p: Planet): Style {
  if (p.look === 'closed') return 'ice'
  if (p.look === 'key') return 'gold'
  return p.tone === 'good' ? 'ocean' : p.tone === 'bad' ? 'rock' : 'bands'
}

function ramp(cols: THREE.Color[], t: number, out: THREE.Color) {
  const x = clamp01(t) * (cols.length - 1)
  const i = Math.min(cols.length - 2, Math.floor(x))
  return out.copy(cols[i]).lerp(cols[i + 1], x - i)
}

function planetMaps(p: Planet): { map: THREE.CanvasTexture; clouds: THREE.CanvasTexture | null } {
  const style = styleOf(p)
  const seed = hash(p.id)
  const n = noise3(seed)
  const W = p.size > 0.55 ? 384 : 256
  const H = W / 2
  const c = document.createElement('canvas')
  c.width = W; c.height = H
  const g = c.getContext('2d')!
  const img = g.createImageData(W, H)
  const cols = (style === 'bands' && p.tone === 'neutral' ? NEUTRAL_BANDS : PALETTE[style]).map((h) => new THREE.Color(h))
  const out = new THREE.Color()
  const r = rng(seed)
  const off = r() * 50
  const bands = 5 + Math.floor(r() * 6)
  const fbm = (x: number, y: number, z: number, o: number) => { let a = 0.5; let s = 0; let f = 1; for (let i = 0; i < o; i++) { s += a * n(x * f + off, y * f, z * f); f *= 2.03; a *= 0.5 } return s / (1 - Math.pow(0.5, o)) }
  for (let y = 0; y < H; y++) {
    const th = (y / H) * Math.PI
    const sy = Math.cos(th)
    const st = Math.sin(th)
    for (let x = 0; x < W; x++) {
      const ph = (x / W) * Math.PI * 2
      const px = Math.cos(ph) * st; const pz = Math.sin(ph) * st
      if (style === 'ocean') {
        const h = fbm(px * 1.8, sy * 1.8, pz * 1.8, 5)
        const pole = Math.abs(sy)
        if (pole > 0.86 - h * 0.12) out.copy(cols[5])
        else if (h > 0.53) ramp([cols[3], cols[4], cols[5]], (h - 0.53) * 3.2, out)
        else ramp([cols[0], cols[1], cols[2]], h / 0.53, out)
      } else if (style === 'bands' || style === 'gold') {
        const t = fbm(px * 2.5, sy * 2.5, pz * 2.5, 4)
        const v = (sy * 0.5 + 0.5) * bands + t * 1.4
        const k = (Math.sin(v * Math.PI) * 0.5 + 0.5) * 0.7 + fbm(px * 6, sy * 18, pz * 6, 3) * 0.3
        ramp(cols, k, out)
      } else if (style === 'rock') {
        const h = fbm(px * 2.2, sy * 2.2, pz * 2.2, 5)
        ramp(cols, h * 1.15 - 0.08, out)
      } else {
        const h = fbm(px * 3, sy * 3, pz * 3, 4)
        ramp(cols, 0.35 + h * 0.65, out)
      }
      const i = (y * W + x) * 4
      img.data[i] = out.r * 255; img.data[i + 1] = out.g * 255; img.data[i + 2] = out.b * 255; img.data[i + 3] = 255
    }
  }
  g.putImageData(img, 0, 0)
  if (style === 'rock' || style === 'ice') { // craters / cracks
    for (let k = 0; k < (style === 'rock' ? 26 : 18); k++) {
      const x = r() * W; const y = H * (0.15 + r() * 0.7); const rad = 2 + r() * (W / 40)
      if (style === 'rock') {
        g.fillStyle = 'rgba(20,4,2,0.35)'; g.beginPath(); g.arc(x, y, rad, 0, Math.PI * 2); g.fill()
        g.strokeStyle = 'rgba(255,200,170,0.25)'; g.lineWidth = 1; g.beginPath(); g.arc(x - 0.6, y - 0.6, rad, Math.PI, Math.PI * 1.8); g.stroke()
      } else {
        g.strokeStyle = 'rgba(70,110,160,0.35)'; g.lineWidth = 1
        g.beginPath(); g.moveTo(x, y)
        for (let s = 0; s < 6; s++) g.lineTo(x + (r() - 0.5) * 60, y + (r() - 0.5) * 20)
        g.stroke()
      }
    }
  }
  const map = new THREE.CanvasTexture(c)
  map.colorSpace = THREE.SRGBColorSpace
  map.anisotropy = 4
  let clouds: THREE.CanvasTexture | null = null
  if (style === 'ocean') {
    const cc = document.createElement('canvas')
    cc.width = W; cc.height = H
    const cg = cc.getContext('2d')!
    const ci = cg.createImageData(W, H)
    for (let y = 0; y < H; y++) {
      const th = (y / H) * Math.PI
      for (let x = 0; x < W; x++) {
        const ph = (x / W) * Math.PI * 2
        const v = fbm(Math.cos(ph) * Math.sin(th) * 3 + 9, Math.cos(th) * 3, Math.sin(ph) * Math.sin(th) * 3, 5)
        const a = smooth((v - 0.52) * 4)
        const i = (y * W + x) * 4
        ci.data[i] = 255; ci.data[i + 1] = 255; ci.data[i + 2] = 255; ci.data[i + 3] = a * 235
      }
    }
    cg.putImageData(ci, 0, 0)
    clouds = new THREE.CanvasTexture(cc)
    clouds.colorSpace = THREE.SRGBColorSpace
  }
  return { map, clouds }
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

const TAIL_V = `varying vec2 vUv; void main(){ vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }`
const TAIL_F = `uniform vec3 uColor; uniform float uAlpha; varying vec2 vUv;
void main(){ float a = pow(vUv.y, 1.8) * (0.55 + 0.45 * sin(vUv.x * 6.2832 * 3.0 + vUv.y * 9.0)); gl_FragColor = vec4(uColor, a * uAlpha); }`

// ---------- view types ----------

interface Astro { root: THREE.Group; body: THREE.Group; arms: [THREE.Group, THREE.Group]; legs: [THREE.Group, THREE.Group]; flames: THREE.Mesh[]; hand: THREE.Group }
interface PlanetView {
  planet: Planet
  g: THREE.Group
  mesh: THREE.Mesh
  r: number
  clouds?: THREE.Mesh
  warn?: THREE.Mesh
  ring: THREE.Mesh // hover
  hit: THREE.Mesh
  orbit: THREE.LineLoop
  label: THREE.Sprite
  maps: THREE.Texture[]
}
type AgentState = 'home' | 'out' | 'act' | 'back' | 'return' | 'togather' | 'gather'
interface Agent {
  key: AgentKey
  a: Astro
  tasks: string[]
  next: number
  state: AgentState
  u: number
  dur: number
  from: THREE.Vector3
  timer: number
  planet: string
  carry: THREE.Object3D | null
  fx: THREE.Object3D | null
  label: THREE.Sprite | null
  target: StationTarget & { kind: 'agent' }
  inWork: boolean
  workText: string
}
interface CometView { comet: Comet; g: THREE.Group; head: THREE.Mesh; tail: THREE.Mesh; hit: THREE.Mesh; label: THREE.Sprite; phase: number; incl: number; enter: number; seed: number }
interface Pulse { s: THREE.Sprite; from: () => THREE.Vector3; to: () => THREE.Vector3; t: number; dur: number; lift: number }
interface Bit { m: THREE.Object3D; v: THREE.Vector3; life: number; max: number; fall: boolean }

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
  private target = new THREE.Vector3(0, -0.4, 2.4)
  private yaw = 0
  private pitch = PITCH0
  private zoom = 1
  private dist = 34
  private moved = false
  private geos = new Map<string, THREE.BufferGeometry>()
  private mats = new Map<string, THREE.Material>()
  private texs: THREE.Texture[] = []
  private glowTex: THREE.Texture
  private hits: THREE.Object3D[] = []
  private hover: string | null = null
  private planetGroup = new THREE.Group()
  private planets = new Map<string, PlanetView>()
  private pending: { p: Planet; R: number; r: number }[] = [] // built one per frame: a planet texture takes tens of ms
  private mapCache = new Map<string, { map: THREE.CanvasTexture; clouds: THREE.CanvasTexture | null }>()
  private agents: Agent[] = []
  private commander: Astro | null = null
  private comets = new Map<string, CometView>()
  private pulses: Pulse[] = []
  private bits: Bit[] = []
  private core = { mat: null as THREE.ShaderMaterial | null, mesh: null as THREE.Mesh | null, corona: null as THREE.Sprite | null, wire: null as THREE.Mesh | null, rings: [] as THREE.Mesh[], arcs: [] as THREE.Mesh[], ring: null as THREE.Mesh | null, light: null as THREE.PointLight | null, conduits: [] as THREE.Mesh[], wave: null as THREE.Mesh | null, waveT: 99 }
  private dock = { pos: new THREE.Vector3(), caps: new THREE.Group(), holo: null as THREE.Sprite | null, holoText: '', ring: null as THREE.Mesh | null, count: 0 }
  private cargo = { pos: new THREE.Vector3(), crates: new THREE.Group(), beacon: null as THREE.Mesh | null, ring: null as THREE.Mesh | null, aging: 0, critical: false }
  private comms = { pos: new THREE.Vector3(), tip: null as THREE.Mesh | null, bubble: null as THREE.Sprite | null, bubbleText: '', ring: null as THREE.Mesh | null, unread: 0 }
  private radar = { pivot: new THREE.Group(), dish: new THREE.Group(), line: null as THREE.Line | null, focus: null as string | null, timer: 0, ping: 0 }
  private belt: THREE.InstancedMesh | null = null
  private stars: THREE.Points[] = []
  private beams: THREE.Line[] = []
  private bolt: THREE.Line | null = null
  private boltT = 0
  private cycle: CycleState = { running: false, stage: 0, hasCycle: false }
  private t = 0
  private last = 0
  private raf = 0
  private w = 0
  private h = 0
  private work: Work | null = null
  private pulseClock = 0
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
    this.buildStation()
    this.buildCrew()
    this.buildBelt()
    this.scene.add(this.planetGroup)
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
  private cylG(rt: number, rb: number, h: number, seg = 20) { return this.geo(`c${rt},${rb},${h},${seg}`, () => new THREE.CylinderGeometry(rt, rb, h, seg)) }
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

  private label(text: string, bg: string, fg = '#ffffff', h = 0.5, border = '') {
    const c = document.createElement('canvas')
    const g = c.getContext('2d')!
    const font = '700 54px "Plus Jakarta Sans", "Instrument Sans", system-ui, sans-serif'
    g.font = font
    const W = Math.ceil(g.measureText(text).width + 56)
    const H = 84
    c.width = W
    c.height = H
    g.font = font
    g.fillStyle = bg
    g.beginPath()
    if (g.roundRect) g.roundRect(3, 3, W - 6, H - 6, 22)
    else g.rect(3, 3, W - 6, H - 6)
    g.fill()
    if (border) { g.strokeStyle = border; g.lineWidth = 4; g.stroke() }
    g.fillStyle = fg
    g.textAlign = 'center'
    g.textBaseline = 'middle'
    g.fillText(text, W / 2, H / 2 + 3)
    const tex = new THREE.CanvasTexture(c)
    tex.colorSpace = THREE.SRGBColorSpace
    tex.anisotropy = 4
    const s = new THREE.Sprite(new THREE.SpriteMaterial({ map: tex, depthTest: false, transparent: true, toneMapped: false }))
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

  // ---------- sky: nebula, stars ----------

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
    sc.add(new THREE.HemisphereLight('#7f95c9', '#140d22', 0.42))
    const rim = new THREE.DirectionalLight('#8fb3ff', 0.55)
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

  // ---------- the station ring and its modules ----------

  private buildStation() {
    const sc = this.scene
    const hull = this.std('#c7cfdc', 0.32, 0.85, 1)
    const dark = this.std('#2c3445', 0.45, 0.7, 0.8)
    const ring = this.mesh(this.torusG(RING_R, 0.2, Math.PI * 2, 160), hull)
    ring.rotation.x = Math.PI / 2
    sc.add(ring)
    const inner = this.mesh(this.torusG(RING_R, 0.08, Math.PI * 2, 160), dark)
    inner.rotation.x = Math.PI / 2
    inner.position.y = 0.17
    sc.add(inner)
    // window lights around the ring
    const winGeo = this.boxG(0.12, 0.06, 0.03)
    const winMat = this.glow(0.9, 1.15, 1.5)
    for (let i = 0; i < 64; i++) {
      const a = (i / 64) * Math.PI * 2
      const w = this.mesh(winGeo, winMat)
      w.position.copy(onRing(a, RING_R + 0.2, 0.02))
      w.lookAt(onRing(a, RING_R + 3, 0.02))
      sc.add(w)
    }
    // energy conduits from the core to the ring
    for (let i = 0; i < 4; i++) {
      const a = Math.PI / 4 + (i * Math.PI) / 2
      const len = RING_R - CORE_R - 0.35
      const c = this.mesh(this.cylG(0.05, 0.05, len, 12), new THREE.MeshBasicMaterial({ color: new THREE.Color(0.4, 1.1, 1.5), toneMapped: false }))
      c.position.copy(onRing(a, CORE_R + 0.2 + len / 2, 0))
      c.rotation.z = Math.PI / 2
      c.rotation.y = -a
      sc.add(c)
      this.core.conduits.push(c)
      const strut = this.mesh(this.cylG(0.09, 0.09, len, 12), dark)
      strut.position.copy(c.position).setY(-0.12)
      strut.rotation.copy(c.rotation)
      sc.add(strut)
    }
    // solar arrays at the back
    const panelTex = (() => {
      const cv = document.createElement('canvas')
      cv.width = 256; cv.height = 64
      const g = cv.getContext('2d')!
      const gr = g.createLinearGradient(0, 0, 256, 64)
      gr.addColorStop(0, '#0a1f4d'); gr.addColorStop(1, '#173a86')
      g.fillStyle = gr; g.fillRect(0, 0, 256, 64)
      g.strokeStyle = 'rgba(140,180,255,0.55)'; g.lineWidth = 1.5
      for (let x = 0; x <= 256; x += 16) { g.beginPath(); g.moveTo(x, 0); g.lineTo(x, 64); g.stroke() }
      for (let y = 0; y <= 64; y += 16) { g.beginPath(); g.moveTo(0, y); g.lineTo(256, y); g.stroke() }
      const t = new THREE.CanvasTexture(cv)
      t.colorSpace = THREE.SRGBColorSpace
      t.anisotropy = 8
      this.texs.push(t)
      return t
    })()
    const panelMat = new THREE.MeshStandardMaterial({ map: panelTex, roughness: 0.25, metalness: 0.9, envMapIntensity: 1.3 })
    this.mats.set('panel', panelMat)
    for (const a of [-(3 * Math.PI) / 4, -Math.PI / 4]) {
      const arm = this.mesh(this.cylG(0.04, 0.04, 2.1, 8), hull)
      arm.position.copy(onRing(a, RING_R + 1.25, 0))
      arm.rotation.z = Math.PI / 2
      arm.rotation.y = -a
      sc.add(arm)
      for (const k of [0.75, 1.55]) {
        const p = this.mesh(this.boxG(0.72, 0.02, 1.25), panelMat)
        p.position.copy(onRing(a, RING_R + 0.25 + k, 0.02))
        p.rotation.y = -a
        p.rotation.x = 0.35
        sc.add(p)
      }
    }
    // Dek Keputusan (front): the deck, the capsules waiting, the commander and a hologram
    const dp = onRing(ANG.dock, RING_R + 0.55, 0.28)
    this.dock.pos.copy(dp)
    const deck = this.mesh(this.cylG(0.95, 1.02, 0.14, 48), hull, dp.x, dp.y, dp.z)
    sc.add(deck)
    const rim = this.mesh(this.torusG(0.98, 0.03, Math.PI * 2, 64), this.glow(1.8, 1.4, 0.4), dp.x, dp.y + 0.08, dp.z)
    rim.rotation.x = Math.PI / 2
    sc.add(rim)
    this.dock.caps.position.copy(dp).setY(dp.y + 0.07)
    sc.add(this.dock.caps)
    const dockHit = this.mesh(this.cylG(1.05, 1.05, 0.5, 16), new THREE.MeshBasicMaterial({ visible: false }), dp.x, dp.y + 0.2, dp.z)
    sc.add(dockHit)
    this.hit(dockHit, { kind: 'dock' })
    const dr = this.hoverRing(1.18)
    dr.position.copy(dp).setY(dp.y + 0.08)
    sc.add(dr)
    this.dock.ring = dr
    // cargo bay (AI Stok)
    const cp = onRing(ANG.cargo, RING_R + 0.35, 0.3)
    this.cargo.pos.copy(cp)
    const bay = this.mesh(this.boxG(1.1, 0.42, 1.3), hull, cp.x, cp.y, cp.z)
    sc.add(bay)
    const bayDoor = this.mesh(this.boxG(0.02, 0.28, 0.9), this.glow(1.5, 1.1, 0.4), cp.x - 0.56, cp.y, cp.z)
    sc.add(bayDoor)
    this.cargo.crates.position.copy(cp).setY(cp.y + 0.21)
    sc.add(this.cargo.crates)
    const beacon = this.mesh(this.sphere(0.07, 16), this.glow(4, 0.4, 0.3), cp.x + 0.4, cp.y + 0.33, cp.z - 0.5)
    sc.add(beacon)
    this.cargo.beacon = beacon
    const cargoHit = this.mesh(this.boxG(1.4, 1, 1.6), new THREE.MeshBasicMaterial({ visible: false }), cp.x, cp.y + 0.3, cp.z)
    sc.add(cargoHit)
    this.hit(cargoHit, { kind: 'cargo' })
    const cr = this.hoverRing(1.05)
    cr.position.copy(cp).setY(cp.y - 0.15)
    sc.add(cr)
    this.cargo.ring = cr
    // radar (AI Prospek): a dish on a mast that turns to the comets
    const rp = onRing(ANG.radar, RING_R + 0.3, 0.2)
    sc.add(this.mesh(this.cylG(0.62, 0.66, 0.08, 40), hull, rp.x, rp.y, rp.z + 0.2))
    const mast = this.mesh(this.cylG(0.05, 0.08, 0.7, 12), dark, rp.x, rp.y + 0.35, rp.z)
    sc.add(mast)
    this.radar.pivot.position.set(rp.x, rp.y + 0.75, rp.z)
    sc.add(this.radar.pivot)
    this.radar.pivot.add(this.radar.dish)
    const dishGeo = this.geo('dish', () => {
      const pts: THREE.Vector2[] = []
      for (let i = 0; i <= 16; i++) { const x = (i / 16) * 0.42; pts.push(new THREE.Vector2(x, x * x * 1.4)) }
      return new THREE.LatheGeometry(pts, 40)
    })
    const dishMat = new THREE.MeshStandardMaterial({ color: '#e8edf5', roughness: 0.3, metalness: 0.6, side: THREE.DoubleSide, envMapIntensity: 1 })
    this.mats.set('dish', dishMat)
    const dish = this.mesh(dishGeo, dishMat)
    dish.rotation.x = Math.PI / 2 // opens to +z of the pivot
    this.radar.dish.add(dish)
    const feed = this.mesh(this.cylG(0.012, 0.012, 0.32, 6), dark, 0, 0, 0.16)
    feed.rotation.x = Math.PI / 2
    this.radar.dish.add(feed)
    this.radar.dish.add(this.mesh(this.sphere(0.035, 12), this.glow(0.8, 3, 1.4), 0, 0, 0.33))
    // the scan: a faint line from the dish to the comet, with pings running along it
    const line = new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()]), new THREE.LineDashedMaterial({ color: new THREE.Color(0.45, 1.3, 0.95), dashSize: 0.35, gapSize: 0.25, transparent: true, opacity: 0.55, depthWrite: false, toneMapped: false }))
    line.visible = false
    sc.add(line)
    this.radar.line = line
    // comms antenna (chat)
    const mp = onRing(ANG.comms, RING_R + 0.25, 0.2)
    this.comms.pos.copy(mp)
    sc.add(this.mesh(this.cylG(0.025, 0.07, 1.7, 10), hull, mp.x, mp.y + 0.85, mp.z))
    for (const [y, l] of [[0.9, 0.55], [1.3, 0.38]] as const) {
      const bar = this.mesh(this.cylG(0.015, 0.015, l, 6), hull, mp.x, mp.y + y, mp.z)
      bar.rotation.z = Math.PI / 2
      sc.add(bar)
    }
    const tip = this.mesh(this.sphere(0.06, 14), this.glow(3.5, 0.6, 0.5), mp.x, mp.y + 1.75, mp.z)
    sc.add(tip)
    this.comms.tip = tip
    const commsHit = this.mesh(this.cylG(0.4, 0.4, 2.2, 8), new THREE.MeshBasicMaterial({ visible: false }), mp.x, mp.y + 1, mp.z)
    sc.add(commsHit)
    this.hit(commsHit, { kind: 'comms' })
    const mr = this.hoverRing(0.55)
    mr.position.copy(mp)
    sc.add(mr)
    this.comms.ring = mr
  }

  // ---------- astronauts ----------

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
    const visorMat = new THREE.MeshStandardMaterial({ color: visor, roughness: 0.08, metalness: 1, envMapIntensity: 1.8 })
    this.mats.set('visor' + visor, visorMat)
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

  private buildCrew() {
    const keys: AgentKey[] = ['order', 'collect', 'credit', 'followup', 'stock', 'prospect']
    for (const key of keys) {
      const a = this.astronaut(AGENT_COLOR[key], key === 'prospect' ? '#3a6fd8' : '#1c2a48')
      const target = { kind: 'agent' as const, agent: key, status: '' }
      const hitM = this.mesh(this.sphere(0.36, 8), new THREE.MeshBasicMaterial({ visible: false }), 0, 0.15, 0)
      hitM.scale.setScalar(1 / ASTRO_S)
      a.root.add(hitM)
      this.hit(hitM, target)
      this.scene.add(a.root)
      const ag: Agent = { key, a, tasks: [], next: 0, state: 'home', u: 0, dur: 1, from: new THREE.Vector3(), timer: 0.5 + keys.indexOf(key) * 0.7, planet: '', carry: null, fx: null, label: null, target, inWork: false, workText: '' }
      a.root.position.copy(this.homeOf(key))
      this.agents.push(ag)
    }
    // the commander: the human who decides, on the Dek Keputusan, gold visor
    const c = this.astronaut('#f2c14e', '#e3b04b', '#ffffff')
    c.root.position.copy(this.dock.pos).setY(this.dock.pos.y + 0.07 + 0.3 * ASTRO_S)
    c.root.rotation.y = 0 // faces outward (+z)
    const tablet = this.mesh(this.boxG(0.16, 0.11, 0.012), this.glow(0.5, 1.4, 2.2))
    tablet.position.set(0, 0.03, 0.06)
    tablet.rotation.x = -0.9
    c.hand.add(tablet)
    const hitM = this.mesh(this.sphere(0.36, 8), new THREE.MeshBasicMaterial({ visible: false }), 0, 0.15, 0)
    hitM.scale.setScalar(1 / ASTRO_S)
    c.root.add(hitM)
    this.hit(hitM, { kind: 'commander' })
    this.scene.add(c.root)
    this.commander = c
  }

  private homeOf(key: AgentKey) {
    if (key === 'stock') return this.cargo.pos.clone().add(new THREE.Vector3(0.15, 0.21 + 0.3 * ASTRO_S, 0.35))
    if (key === 'prospect') return onRing(ANG.radar, RING_R + 0.3, 0.24 + 0.3 * ASTRO_S).add(new THREE.Vector3(-0.05, 0, 0.5))
    return onRing(HOME[key], RING_R, 0.75)
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

  setPlan(plan: StationPlan) {
    if (!this.renderer) return
    const keep = new Set(plan.planets.map((p) => p.id))
    for (const [id, v] of this.planets) if (!keep.has(id)) this.dropPlanet(id, v)
    this.pending = []
    // place planets: radius from the jadwal order, pushed out a little when two would touch
    const placed: { x: number; z: number; r: number }[] = []
    for (const p of plan.planets) {
      const r = 0.42 + p.size * 0.8
      let R = ORBIT_IN + p.orbit * (ORBIT_OUT - ORBIT_IN)
      for (let k = 0; k < 8; k++) {
        const x = R * Math.cos(p.angle); const z = R * Math.sin(p.angle)
        if (!placed.some((q) => Math.hypot(q.x - x, q.z - z) < q.r + r + 0.45)) break
        R += 0.95
      }
      placed.push({ x: R * Math.cos(p.angle), z: R * Math.sin(p.angle), r })
      const v = this.planets.get(p.id)
      if (v && v.planet.look === p.look && v.planet.tone === p.tone && Math.abs(v.r - r) < 0.01) {
        v.planet = p
        v.g.position.set(R * Math.cos(p.angle), 0, R * Math.sin(p.angle))
        this.setOrbitLine(v, R)
        v.hit.userData.target = { kind: 'planet', planet: p }
      } else {
        if (v) this.dropPlanet(p.id, v)
        this.pending.push({ p, R, r })
      }
    }
    const comets = new Set(plan.comets.map((c) => c.id))
    for (const [id, v] of this.comets) if (!comets.has(id)) this.dropComet(id, v)
    plan.comets.forEach((c, i) => { if (!this.comets.has(c.id)) this.addComet(c, i) })
    for (const a of this.agents) {
      if (a.key === 'stock' || a.key === 'prospect') continue
      a.tasks = plan.tasks[a.key as FlyKind]
    }
  }

  private setOrbitLine(v: PlanetView, R: number) {
    const pts: THREE.Vector3[] = []
    for (let i = 0; i <= 160; i++) { const a = (i / 160) * Math.PI * 2; pts.push(new THREE.Vector3(R * Math.cos(a), 0, R * Math.sin(a))) }
    v.orbit.geometry.dispose()
    v.orbit.geometry = new THREE.BufferGeometry().setFromPoints(pts)
  }

  private addPlanet(p: Planet, R: number, r: number) {
    let maps = this.mapCache.get(p.id + p.look + p.tone)
    if (!maps) { maps = planetMaps(p); this.mapCache.set(p.id + p.look + p.tone, maps) }
    const g = new THREE.Group()
    g.position.set(R * Math.cos(p.angle), 0, R * Math.sin(p.angle))
    g.scale.setScalar(0.01) // grows in (step)
    const style = styleOf(p)
    const mat = new THREE.MeshStandardMaterial({
      map: maps.map, roughness: style === 'ocean' ? 0.55 : style === 'ice' ? 0.35 : 0.85, metalness: 0,
      bumpMap: style === 'rock' || style === 'ice' ? maps.map : null, bumpScale: 2, envMapIntensity: 0.12,
    })
    const mesh = this.mesh(this.sphere(r, 64), mat)
    mesh.rotation.z = 0.35 * ((hash(p.id) % 100) / 100 - 0.5)
    g.add(mesh)
    const bg = p.look === 'risk' ? 'rgba(160,24,30,0.86)' : p.look === 'key' ? 'rgba(122,82,8,0.88)' : p.look === 'closed' ? 'rgba(60,70,90,0.8)' : 'rgba(10,16,34,0.78)'
    const label = this.label(p.name.length > 22 ? p.name.slice(0, 21) + '…' : p.name, bg, '#ffffff', 0.42, 'rgba(255,255,255,0.18)')
    const hitM = this.mesh(this.sphere(Math.max(0.75, r * 1.25), 10), new THREE.MeshBasicMaterial({ visible: false }))
    const orbit = new THREE.LineLoop(new THREE.BufferGeometry(), new THREE.LineBasicMaterial({ color: '#6f84b8', transparent: true, opacity: 0.16, depthWrite: false }))
    const v: PlanetView = { planet: p, g, mesh, r, ring: this.hoverRing(r * 1.55), hit: hitM, orbit, label, maps: [] }
    if (maps.clouds) {
      const cm = new THREE.MeshStandardMaterial({ map: maps.clouds, transparent: true, depthWrite: false, roughness: 1, envMapIntensity: 0.1 })
      v.clouds = this.mesh(this.sphere(r * 1.025, 48), cm)
      g.add(v.clouds)
    }
    if (p.look !== 'closed') g.add(this.mesh(this.sphere(r * 1.05, 48), this.fresnel(TONE_GLOW[p.tone], 3.2, p.look === 'risk' ? 0.95 : 0.6)))
    if (p.look === 'key') { // golden rings
      const ringGeo = new THREE.RingGeometry(r * 1.45, r * 2.25, 96, 1)
      const pos = ringGeo.attributes.position as THREE.BufferAttribute
      const uv = ringGeo.attributes.uv as THREE.BufferAttribute
      for (let i = 0; i < pos.count; i++) { const d = Math.hypot(pos.getX(i), pos.getY(i)); uv.setXY(i, (d - r * 1.45) / (r * 0.8), 0.5) }
      const rc = document.createElement('canvas')
      rc.width = 256; rc.height = 4
      const rg = rc.getContext('2d')!
      const rr = rng(hash(p.id))
      for (let x = 0; x < 256; x++) { const k = 0.35 + rr() * 0.65; rg.fillStyle = `rgba(${230 + rr() * 25},${180 + rr() * 40},${90 + rr() * 60},${(x < 8 || x > 248 ? 0.2 : 0.75) * k})`; rg.fillRect(x, 0, 1, 4) }
      const rt = new THREE.CanvasTexture(rc)
      rt.colorSpace = THREE.SRGBColorSpace
      v.maps.push(rt)
      const ring = this.mesh(ringGeo, new THREE.MeshStandardMaterial({ map: rt, transparent: true, side: THREE.DoubleSide, roughness: 0.7, depthWrite: false, envMapIntensity: 0.3 }))
      ring.rotation.x = Math.PI / 2 - 0.42
      ring.rotation.y = 0.3
      g.add(ring)
    }
    if (p.look === 'risk') { // a red warning ring that pulses
      v.warn = this.mesh(this.torusG(r * 1.38, 0.02, Math.PI * 2, 72), this.glow(1.5, 0.18, 0.15, true, 0.9))
      v.warn.rotation.x = Math.PI / 2
      g.add(v.warn)
    }
    g.add(v.ring, hitM)
    this.hit(hitM, { kind: 'planet', planet: p })
    this.setOrbitLine(v, R)
    this.planetGroup.add(g, v.orbit)
    this.overlay.add(label)
    this.planets.set(p.id, v)
  }

  private dropPlanet(id: string, v: PlanetView) {
    v.g.removeFromParent()
    v.orbit.removeFromParent()
    v.orbit.geometry.dispose()
    ;(v.orbit.material as THREE.Material).dispose()
    this.dropSprite(v.label)
    this.hits = this.hits.filter((h) => h !== v.hit)
    v.g.traverse((o) => {
      if (o instanceof THREE.Mesh) {
        const m = o.material as THREE.Material
        if (![...this.mats.values()].includes(m)) m.dispose()
        if (o.geometry instanceof THREE.RingGeometry) o.geometry.dispose()
      }
    })
    v.maps.forEach((t) => t.dispose())
    this.planets.delete(id)
  }

  private addComet(c: Comet, i: number) {
    const seed = hash(c.id)
    const g = new THREE.Group()
    const head = this.mesh(this.sphere(0.13, 16), this.glow(2.2, 2.6, 3))
    g.add(head)
    g.add(this.glowSprite('#9fd8ff', 1.4, 0.9))
    const L = 3.2
    const tailMat = new THREE.ShaderMaterial({
      vertexShader: TAIL_V, fragmentShader: TAIL_F, transparent: true, depthWrite: false, blending: THREE.AdditiveBlending, side: THREE.DoubleSide,
      uniforms: { uColor: { value: new THREE.Color(0.5, 1.2, 2) }, uAlpha: { value: 0.85 } },
    })
    const tail = this.mesh(this.geo('tail', () => new THREE.CylinderGeometry(0.06, 0.42, L, 24, 1, true).translate(0, -L / 2, 0)), tailMat)
    this.scene.add(tail)
    const hitM = this.mesh(this.sphere(0.6, 8), new THREE.MeshBasicMaterial({ visible: false }))
    g.add(hitM)
    this.hit(hitM, { kind: 'comet', comet: c })
    this.scene.add(g)
    const name = c.name.length > 20 ? c.name.slice(0, 19) + '…' : c.name
    const label = this.label('Nomor baru · ' + name, 'rgba(14,60,96,0.86)', '#dff3ff', 0.4, 'rgba(140,210,255,0.5)')
    this.overlay.add(label)
    this.comets.set(c.id, { comet: c, g, head, tail, hit: hitM, label, phase: (i * 2.1 + (seed % 100) / 50) % (Math.PI * 2), incl: ((seed % 60) / 60 - 0.5) * 0.5, enter: 0, seed })
  }

  private dropComet(id: string, v: CometView) {
    v.g.removeFromParent()
    v.tail.removeFromParent()
    ;(v.tail.material as THREE.Material).dispose()
    this.dropSprite(v.label)
    this.hits = this.hits.filter((h) => h !== v.hit)
    v.g.traverse((o) => { if (o instanceof THREE.Sprite) o.material.dispose() })
    this.comets.delete(id)
    if (this.radar.focus === id) this.radar.focus = null
  }

  /** The proposals waiting for a decision: one capsule each on the deck, in the agent's colour. */
  setDock(caps: AgentKey[]) {
    if (!this.renderer) return
    const key = caps.join(',')
    if (this.dock.caps.userData.key === key) return
    this.dock.caps.userData.key = key
    this.dock.caps.clear()
    caps.forEach((k, i) => {
      const a = Math.PI + 0.35 + (i / Math.max(1, 11)) * (Math.PI - 0.7) // an arc behind the commander
      const ring = i < 6 ? 0.66 : 0.4
      const m = this.mesh(this.capsule(0.055, 0.1), this.hdr(AGENT_COLOR[k], 1.6))
      m.position.set(Math.cos(a + (i >= 6 ? 0.25 : 0)) * ring, 0.12, Math.sin(a + (i >= 6 ? 0.25 : 0)) * ring)
      m.userData.phase = i * 0.7
      this.dock.caps.add(m)
    })
    this.dock.count = caps.length
    const text = caps.length ? `Keputusan · ${caps.length} menunggu` : 'Keputusan · antrean kosong'
    if (text !== this.dock.holoText) {
      this.dock.holoText = text
      this.dropSprite(this.dock.holo)
      this.dock.holo = this.label(text, caps.length ? 'rgba(90,62,6,0.9)' : 'rgba(12,40,60,0.85)', caps.length ? '#ffe7a8' : '#bfe9ff', 0.46, caps.length ? 'rgba(255,205,90,0.7)' : 'rgba(140,210,255,0.5)')
      this.dock.holo.center.set(0.5, 1) // hangs under the deck, never over the core
      this.dock.holo.position.copy(this.dock.pos).add(new THREE.Vector3(0, -0.3, 1.2))
      this.overlay.add(this.dock.holo)
    }
  }

  setStock(aging: number, critical: boolean) {
    this.cargo.aging = aging
    this.cargo.critical = critical
    if (!this.renderer) return
    const n = Math.min(9, aging)
    if (this.cargo.crates.children.length === n) return
    this.cargo.crates.clear()
    const mat = this.std('#c98a2b', 0.6, 0.2, 0.6)
    const band = this.std('#4b3210', 0.6, 0.3, 0.5)
    for (let i = 0; i < n; i++) {
      const c = new THREE.Group()
      c.add(this.mesh(this.boxG(0.2, 0.18, 0.2), mat))
      c.add(this.mesh(this.boxG(0.205, 0.035, 0.205), band))
      c.position.set(-0.32 + (i % 3) * 0.24, 0.09 + Math.floor(i / 6) * 0.19, -0.42 + (Math.floor(i / 3) % 2) * 0.24)
      this.cargo.crates.add(c)
    }
  }

  setUnread(n: number) {
    this.comms.unread = n
    if (!this.renderer) return
    const text = n > 0 ? `${n > 999 ? '999+' : n} chat belum dibaca` : 'Chat terbaca semua'
    if (text === this.comms.bubbleText) return
    this.comms.bubbleText = text
    this.dropSprite(this.comms.bubble)
    this.comms.bubble = this.label(text, n > 0 ? 'rgba(18,110,72,0.9)' : 'rgba(20,30,50,0.8)', '#ffffff', 0.4, 'rgba(255,255,255,0.25)')
    this.comms.bubble.position.copy(this.comms.pos).setY(this.comms.pos.y + 1.95)
    this.overlay.add(this.comms.bubble)
  }

  setCycle(c: CycleState) {
    const was = this.cycle.running
    const stageWas = this.cycle.stage
    this.cycle = c
    if (!this.renderer) return
    if (c.running && !was) {
      for (const a of this.agents) if (!this.work?.[a.key]) this.fly(a, 'togather', a.a.root.position.clone())
    } else if (!c.running && was) {
      this.core.waveT = 0 // learn: a shock wave from the core
      for (const a of this.agents) if (a.state === 'gather' || a.state === 'togather') this.fly(a, 'return', a.a.root.position.clone())
    }
    if (c.running && c.stage === 5 && stageWas !== 5) this.core.waveT = 0
  }

  /** The cycle being followed: each astronaut shows its agent's state (antri, menganalisa, selesai with the result,
   * gagal); null sends them home. */
  setWork(w: Work | null) {
    this.work = w
  }

  // ---------- animation ----------

  private planetPos(id: string, out = new THREE.Vector3()) {
    const v = this.planets.get(id)
    return v ? v.g.getWorldPosition(out) : out.set(0, 0, 0)
  }

  /** Where an astronaut hovers to work a planet: on the core side, a little above. */
  private workSpot(id: string) {
    const p = this.planetPos(id)
    const v = this.planets.get(id)
    const toCore = p.clone().multiplyScalar(-1).setY(0).normalize()
    return p.addScaledVector(toCore, (v?.r ?? 0.6) + 0.55).setY(0.45)
  }
  private gatherSpot(a: Agent) {
    const i = this.agents.indexOf(a)
    const ang = this.t * 0.35 + (i / this.agents.length) * Math.PI * 2
    return new THREE.Vector3(GATHER_R * Math.cos(ang), 0.95 + Math.sin(this.t * 1.3 + i) * 0.08, GATHER_R * Math.sin(ang))
  }
  private dockSpot() { return this.dock.pos.clone().add(new THREE.Vector3(0.55, 0.6, 0.25)) }

  private goal(a: Agent): THREE.Vector3 {
    switch (a.state) {
      case 'out': return this.workSpot(a.planet)
      case 'back': return this.dockSpot()
      case 'togather': return this.gatherSpot(a)
      default: return this.homeOf(a.key)
    }
  }

  private fly(a: Agent, state: AgentState, from: THREE.Vector3) {
    this.endAct(a)
    a.state = state
    a.from.copy(from)
    a.u = 0
    const d = from.distanceTo(this.goal(a))
    a.dur = 0.8 + d / 3.4
    a.a.flames.forEach((f) => (f.visible = true))
  }

  private say(a: Agent) {
    const name = this.planets.get(a.planet)?.planet.name ?? ''
    const busy: Record<FlyKind, [string, string, string]> = {
      order: ['Terbang ke ' + name + ' — jadwal order dekat', 'Menyiapkan usulan order ' + name, 'Membawa usulan order ke Dek Keputusan'],
      collect: ['Terbang ke ' + name + ' — over limit / overdue', 'Menghitung tagihan ' + name, 'Membawa usulan penagihan ke Dek Keputusan'],
      credit: ['Terbang ke ' + name + ' — sisa limit tipis', 'Memasang perisai kredit ' + name, 'Membawa usulan kredit ke Dek Keputusan'],
      followup: ['Spacewalk ke ' + name + ' — At risk', 'Menyiapkan follow-up ' + name, 'Membawa usulan follow-up ke Dek Keputusan'],
    }
    const t = a.target
    if (this.cycle.running && (a.state === 'gather' || a.state === 'togather')) { t.status = 'Menganalisa bersama Inti AI (siklus Orchestrator berjalan)'; return }
    if (a.key === 'stock') { t.status = this.cargo.aging ? `Merapikan ${this.cargo.aging} stok menua di cargo bay${this.cargo.critical ? ' · ada stok kritis' : ''}` : 'Cargo bay rapi — tidak ada stok menua'; return }
    if (a.key === 'prospect') {
      const c = this.radar.focus ? this.comets.get(this.radar.focus)?.comet.name : null
      t.status = c ? `Memindai nomor baru: ${c} (WA Business · Truecaller · Odoo)` : 'Radar pasif — menunggu nomor baru yang menghubungi lebih dulu'
      return
    }
    const k = a.key as FlyKind
    if (a.state === 'out') t.status = busy[k][0]
    else if (a.state === 'act') t.status = busy[k][1]
    else if (a.state === 'back') t.status = busy[k][2]
    else t.status = a.tasks.length ? `Siaga · ${a.tasks.length} dealer menunggu` : 'Siaga — tidak ada dealer yang perlu ditangani'
  }

  private endAct(a: Agent) {
    if (a.fx) { a.fx.removeFromParent(); a.fx.traverse((o) => { if (o instanceof THREE.Line) { o.geometry.dispose(); (o.material as THREE.Material).dispose() } else if (o instanceof THREE.Mesh && o.userData.own) (o.material as THREE.Material).dispose() }); a.fx = null }
    this.dropSprite(a.label)
    a.label = null
  }

  private setCarry(a: Agent, what: 'crate' | 'capsule' | null) {
    if (a.carry) { a.carry.removeFromParent(); a.carry = null }
    if (!what) return
    const m = what === 'crate'
      ? this.mesh(this.boxG(0.13, 0.11, 0.13), this.std('#d9a441', 0.5, 0.3, 0.7))
      : this.mesh(this.capsule(0.045, 0.08), this.hdr(AGENT_COLOR[a.key], 1.8))
    a.a.hand.add(m)
    a.carry = m
  }

  private startAct(a: Agent) {
    a.state = 'act'
    a.timer = 3.2
    a.a.flames.forEach((f) => (f.visible = false))
    const v = this.planets.get(a.planet)
    const name = v?.planet.name ?? ''
    const word: Record<FlyKind, string> = { order: 'Order', collect: 'Tagih', credit: 'Kredit', followup: 'Follow-up' }
    const text = word[a.key as FlyKind] + ' · ' + name
    a.label = this.label(text.length > 28 ? text.slice(0, 27) + '…' : text, AGENT_COLOR[a.key], a.key === 'followup' ? '#2b1600' : '#ffffff', 0.4)
    this.overlay.add(a.label)
    if (!v) return
    if (a.key === 'credit') { // a hex shield around the planet
      const g = new THREE.Group()
      const wire = this.mesh(this.geo('shield', () => new THREE.IcosahedronGeometry(1, 2)), new THREE.MeshBasicMaterial({ color: new THREE.Color(1.2, 0.7, 2.6), wireframe: true, transparent: true, opacity: 0, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }))
      wire.userData.own = true
      const bub = this.mesh(this.sphere(1, 32), this.fresnel('#a26bff', 2, 0))
      bub.userData.own = true
      g.add(wire, bub)
      g.scale.setScalar(v.r * 1.7)
      v.g.add(g)
      a.fx = g
    } else if (a.key === 'followup') { // a tether from the astronaut to the planet
      const line = new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()]), new THREE.LineBasicMaterial({ color: new THREE.Color(2.4, 1.3, 0.4), transparent: true, opacity: 0.9, toneMapped: false }))
      this.scene.add(line)
      a.fx = line
    } else if (a.key === 'order') {
      this.setCarry(a, null)
      const crate = this.mesh(this.boxG(0.13, 0.11, 0.13), this.std('#d9a441', 0.5, 0.3, 0.7))
      crate.position.copy(a.a.hand.getWorldPosition(new THREE.Vector3()))
      this.scene.add(crate)
      a.fx = crate
    }
  }

  private stepWork(a: Agent, w: AgentWork, dt: number) {
    const A = a.a
    const pos = A.root.position
    if (!a.inWork) {
      this.endAct(a)
      this.setCarry(a, null)
      a.inWork = true
      a.workText = ''
      a.state = 'home'
    }
    const i = this.agents.indexOf(a)
    const finished = w.state === 'done' || w.state === 'failed'
    let goal: THREE.Vector3
    if (finished) { // in a row in front of the Dek Keputusan, showing the result
      const k = i - (this.agents.length - 1) / 2
      goal = this.dock.pos.clone().add(new THREE.Vector3(k * 0.85, 0.95 + Math.sin(this.t * 1.5 + i) * 0.04, 1.8 - Math.abs(k) * 0.18))
    } else { // around the core: reading close with a beam, waiting a little further out
      const reading = w.state === 'reading'
      const ang = this.t * (reading ? 0.35 : 0.12) + (i / this.agents.length) * Math.PI * 2
      const r = reading ? GATHER_R : GATHER_R + 0.6
      goal = new THREE.Vector3(r * Math.cos(ang), (reading ? 0.95 : 1.4) + Math.sin(this.t * 1.3 + i) * 0.08, r * Math.sin(ang))
    }
    const prev = pos.clone()
    const moving = pos.distanceTo(goal) > 0.3
    pos.lerp(goal, 1 - Math.exp(-dt * 2.2))
    A.flames.forEach((f) => { f.visible = moving; if (moving) f.scale.set(1, 0.8 + Math.random() * 0.5, 1) })
    const dir = pos.clone().sub(prev).setY(0)
    if (moving && dir.lengthSq() > 1e-8) { const l = pos.clone().add(dir); A.root.lookAt(l.x, pos.y, l.z) }
    else if (finished) A.root.rotation.set(0, lerp(A.root.rotation.y, 0, 0.1), 0) // faces the camera
    else A.root.lookAt(0, pos.y, 0)
    A.body.rotation.x = lerp(A.body.rotation.x, moving ? 0.9 : 0.1, 1 - Math.exp(-dt * 5))
    if (moving) { A.arms[1].rotation.x = -2.5; A.arms[0].rotation.x = -0.35 }
    else if (w.state === 'reading') A.arms[0].rotation.x = A.arms[1].rotation.x = -1.35 + Math.sin(this.t * 3 + i) * 0.12
    else if (w.state === 'done') { A.arms[1].rotation.x = -2.7 + Math.sin(this.t * 2 + i) * 0.1; A.arms[0].rotation.x = Math.sin(this.t * 2) * 0.1 } // holds the capsule up
    else if (w.state === 'failed') A.arms[0].rotation.x = A.arms[1].rotation.x = 0.15
    else A.arms[0].rotation.x = A.arms[1].rotation.x = Math.sin(this.t * 1.2 + i) * 0.15
    A.legs[0].rotation.x = Math.sin(this.t * 1.6 + i) * 0.12
    A.legs[1].rotation.x = -Math.sin(this.t * 1.6 + i) * 0.12
    if (w.state === 'done' && !a.carry) { this.setCarry(a, 'capsule'); this.spawnBits(pos.clone(), 10, AGENT_COLOR[a.key], 0.3) }
    if (w.state !== 'done' && a.carry) this.setCarry(a, null)
    const text = w.state === 'waiting' ? 'antri' : w.state === 'reading' ? 'menganalisa…' : w.state === 'failed' ? '✕ gagal' : '✓ ' + (w.text || 'selesai')
    if (text !== a.workText) {
      a.workText = text
      this.dropSprite(a.label)
      const bg = w.state === 'failed' ? 'rgba(170,30,36,0.92)' : w.state === 'done' ? AGENT_COLOR[a.key] : w.state === 'reading' ? 'rgba(20,70,120,0.9)' : 'rgba(10,16,34,0.82)'
      const fg = w.state === 'done' && (a.key === 'prospect' || a.key === 'followup') ? '#1a1300' : '#ffffff'
      a.label = this.label(AGENT_NAME[a.key].replace('AI ', '') + ' · ' + text, bg, fg, 0.36, 'rgba(255,255,255,0.25)')
      this.overlay.add(a.label)
    }
    // results in a row: labels on three steps so neighbours never cover each other
    a.label?.position.copy(pos).setY(pos.y + 0.85 + (finished ? (i % 3) * 0.42 : 0))
    a.target.status = w.state === 'waiting' ? 'Antri — menunggu giliran dianalisa'
      : w.state === 'reading' ? 'Sedang menganalisa' + (w.text ? ` (${w.text})` : '')
      : w.state === 'failed' ? 'Gagal' + (w.text ? `: ${w.text}` : '')
      : 'Selesai · ' + (w.text || 'tanpa saran baru') + ' — kapsul dibawa ke Dek Keputusan'
  }

  private stepAgent(a: Agent, dt: number) {
    const w = this.work?.[a.key]
    if (w) { this.stepWork(a, w, dt); return }
    if (a.inWork) { // the followed cycle was closed: back to the usual work
      a.inWork = false
      this.dropSprite(a.label)
      a.label = null
      this.setCarry(a, null)
      this.fly(a, this.cycle.running ? 'togather' : 'return', a.a.root.position.clone())
      return
    }
    const A = a.a
    const pos = A.root.position
    if (a.state === 'home') {
      const h = this.homeOf(a.key)
      pos.lerp(h, 1 - Math.exp(-dt * 3))
      pos.y = h.y + Math.sin(this.t * 1.6 + a.next) * 0.05
      A.body.rotation.x = lerp(A.body.rotation.x, 0, 1 - Math.exp(-dt * 4))
      if (a.key === 'stock') {
        A.root.rotation.y = Math.sin(this.t * 0.5) * 1.2 - Math.PI / 2
        A.arms[0].rotation.x = A.arms[1].rotation.x = this.cargo.aging ? -0.9 + Math.sin(this.t * 2.4) * 0.35 : Math.sin(this.t * 1.2) * 0.1
        if (this.cargo.aging && !a.carry) this.setCarry(a, 'crate')
        if (!this.cargo.aging && a.carry) this.setCarry(a, null)
      } else if (a.key === 'prospect') {
        const c = this.radar.focus ? this.comets.get(this.radar.focus) : null
        if (c) { const p = c.g.position; A.root.lookAt(p.x, pos.y, p.z) } else A.root.rotation.y = lerp(A.root.rotation.y, -Math.PI / 2, 0.02)
        A.arms[1].rotation.x = c ? -1.6 : Math.sin(this.t) * 0.1
        A.arms[0].rotation.x = Math.sin(this.t * 1.1) * 0.1
      } else {
        A.root.lookAt(0, pos.y, 0)
        A.root.rotateY(Math.PI) // face outward, ready to go
        A.arms[0].rotation.x = Math.sin(this.t * 1.4 + 1) * 0.15
        A.arms[1].rotation.x = Math.sin(this.t * 1.4) * 0.15
        a.timer -= dt
        const ready = a.tasks.filter((id) => this.planets.has(id))
        if (a.timer <= 0 && ready.length && !this.cycle.running) {
          a.planet = ready[a.next % ready.length]
          a.next += 1
          if (a.key === 'order') this.setCarry(a, 'crate')
          this.fly(a, 'out', pos.clone())
        } else if (a.timer <= 0) a.timer = 1.5
      }
      A.legs[0].rotation.x = Math.sin(this.t * 1.6) * 0.12
      A.legs[1].rotation.x = -Math.sin(this.t * 1.6) * 0.12
    } else if (a.state === 'gather') {
      const s = this.gatherSpot(a)
      pos.lerp(s, 1 - Math.exp(-dt * 5))
      A.root.lookAt(0, pos.y, 0)
      A.body.rotation.x = lerp(A.body.rotation.x, 0.15, 0.1)
      A.arms[0].rotation.x = A.arms[1].rotation.x = -1.35 + Math.sin(this.t * 3 + a.next) * 0.12
      A.flames.forEach((f) => (f.visible = false))
    } else if (a.state === 'act') {
      a.timer -= dt
      const spot = this.workSpot(a.planet)
      pos.lerp(spot, 1 - Math.exp(-dt * 4))
      pos.y += Math.sin(this.t * 2) * 0.004
      const pp = this.planetPos(a.planet)
      A.root.lookAt(pp.x, pos.y, pp.z)
      A.body.rotation.x = lerp(A.body.rotation.x, 0.2, 0.1)
      const k = 1 - a.timer / 3.2
      const v = this.planets.get(a.planet)
      if (a.key === 'credit' && a.fx) {
        const op = Math.sin(Math.PI * clamp01(k * 1.1))
        const [wire, bub] = a.fx.children as THREE.Mesh[]
        ;(wire.material as THREE.MeshBasicMaterial).opacity = op * 0.55
        ;(bub.material as THREE.ShaderMaterial).uniforms.uAlpha.value = op * 1.4
        a.fx.rotation.y += dt * 0.6
        A.arms[1].rotation.x = -1.5
      } else if (a.key === 'followup' && a.fx instanceof THREE.Line) {
        const hand = A.hand.getWorldPosition(new THREE.Vector3())
        const surf = pp.clone().add(hand.clone().sub(pp).normalize().multiplyScalar(v?.r ?? 0.6))
        const g = a.fx.geometry as THREE.BufferGeometry
        const at = g.attributes.position as THREE.BufferAttribute
        at.setXYZ(0, hand.x, hand.y, hand.z)
        at.setXYZ(1, surf.x, surf.y, surf.z)
        at.needsUpdate = true
        A.arms[1].rotation.x = -1.4 + Math.sin(this.t * 3) * 0.1
      } else if (a.key === 'order' && a.fx) {
        const from = A.hand.getWorldPosition(new THREE.Vector3())
        const kk = smooth(clamp01((k - 0.15) / 0.6))
        a.fx.position.lerpVectors(from, pp, kk)
        a.fx.scale.setScalar(1 - kk * 0.85)
        a.fx.rotation.y += dt * 3
        if (kk >= 1 && a.fx.visible) { a.fx.visible = false; this.spawnBits(pp, 10, '#ffd36b', v?.r ?? 0.6) }
        A.arms[1].rotation.x = -1.2 * (1 - kk)
      } else if (a.key === 'collect') {
        A.arms[1].rotation.x = -1.4
        if (Math.floor((k - dt / 3.2) * 10) !== Math.floor(k * 10) && k < 0.85) this.spawnCoin(pp, A.hand.getWorldPosition(new THREE.Vector3()), v?.r ?? 0.6)
      }
      A.arms[0].rotation.x = Math.sin(this.t * 2) * 0.2
      if (a.label) a.label.position.copy(pos).setY(pos.y + 0.95)
      if (a.timer <= 0) {
        this.setCarry(a, 'capsule')
        this.fly(a, 'back', pos.clone())
      }
    } else { // flying: out | back | return | togather
      a.u = Math.min(1, a.u + dt / a.dur)
      const to = this.goal(a)
      const e = smooth(a.u)
      const mid = a.from.clone().lerp(to, 0.5)
      mid.y += Math.min(3, a.from.distanceTo(to) * 0.22) + 0.4
      const p = a.from.clone().multiplyScalar((1 - e) ** 2).addScaledVector(mid, 2 * (1 - e) * e).addScaledVector(to, e * e)
      const dir = p.clone().sub(pos)
      if (dir.lengthSq() > 1e-6) { const look = pos.clone().add(dir.setY(0)); A.root.lookAt(look.x, pos.y, look.z) }
      pos.copy(p)
      A.body.rotation.x = lerp(A.body.rotation.x, a.state === 'togather' || a.u > 0.85 ? 0.2 : 1.05, 1 - Math.exp(-dt * 5))
      A.arms[1].rotation.x = a.carry ? -0.9 : -2.5
      A.arms[0].rotation.x = -0.35
      A.legs[0].rotation.x = 0.15 + Math.sin(this.t * 8) * 0.05
      A.legs[1].rotation.x = -0.05 - Math.sin(this.t * 8) * 0.05
      for (const f of A.flames) f.scale.set(1, 0.8 + Math.random() * 0.5, 1)
      if (a.u >= 1) {
        if (a.state === 'out') this.startAct(a)
        else if (a.state === 'back') {
          this.spawnBits(this.dockSpot(), 8, AGENT_COLOR[a.key], 0.2)
          this.setCarry(a, null)
          this.fly(a, 'return', pos.clone())
        } else if (a.state === 'togather') {
          a.state = this.cycle.running ? 'gather' : 'home'
          A.flames.forEach((f) => (f.visible = false))
        } else {
          a.state = 'home'
          a.timer = 2 + Math.random() * 2
          A.flames.forEach((f) => (f.visible = false))
          if (a.key === 'order') this.setCarry(a, null)
        }
      }
    }
  }

  private spawnBits(at: THREE.Vector3, n: number, color: string, r: number) {
    const mat = this.hdr(color, 2.2)
    for (let i = 0; i < n; i++) {
      const m = this.mesh(this.sphere(0.03, 6), mat)
      const v = new THREE.Vector3(Math.random() - 0.5, Math.random() * 0.8 + 0.2, Math.random() - 0.5).normalize()
      m.position.copy(at).addScaledVector(v, r * 0.9)
      this.scene.add(m)
      this.bits.push({ m, v: v.multiplyScalar(0.9 + Math.random() * 0.9), life: 0, max: 0.9 + Math.random() * 0.5, fall: true })
    }
  }
  private spawnCoin(from: THREE.Vector3, to: THREE.Vector3, r: number) {
    const m = this.mesh(this.cylG(0.05, 0.05, 0.015, 16), this.glow(2.6, 1.9, 0.5))
    m.rotation.x = Math.PI / 2
    const start = from.clone().add(to.clone().sub(from).normalize().multiplyScalar(r))
    m.position.copy(start)
    this.scene.add(m)
    this.bits.push({ m, v: to.clone().sub(start).divideScalar(0.7), life: 0, max: 0.7, fall: false })
  }
  private dishTip() { return this.radar.dish.localToWorld(new THREE.Vector3(0, 0, 0.33)) }

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
      if (b.fall) b.v.addScaledVector(g, dt)
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

  private randomPlanet(): string | null {
    const ids = [...this.planets.keys()]
    return ids.length ? ids[Math.floor(Math.random() * ids.length)] : null
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
    this.pulseClock -= dt
    if (this.pulseClock <= 0 && dt > 0) {
      if (!c.running) {
        this.pulseClock = 1.4
        const id = this.randomPlanet()
        if (id) this.spawnPulse(() => this.planetPos(id), core, '#5fc8ff', 2.2, 0.35)
      } else if (c.stage === 0) {
        this.pulseClock = 0.12
        const id = this.randomPlanet()
        if (id) this.spawnPulse(() => this.planetPos(id), core, '#7fe0ff', 1.2, 0.5)
      } else if (c.stage === 3) {
        this.pulseClock = 0.25
        this.spawnPulse(core, () => this.dock.pos.clone().setY(this.dock.pos.y + 0.3), '#ffcf5a', 1, 0.5)
      } else if (c.stage === 4) {
        this.pulseClock = 0.2
        const id = this.randomPlanet()
        if (id) this.spawnPulse(() => this.dock.pos.clone().setY(this.dock.pos.y + 0.3), () => this.planetPos(id), '#5cf2a0', 1.4, 0.45)
      } else this.pulseClock = 0.3
    }
    // analysis beams from every gathered astronaut to the core
    const reads = (a: Agent) => a.inWork && this.work?.[a.key]?.state === 'reading'
    const gathered = this.agents.filter((a) => a.state === 'gather' || reads(a))
    while (this.beams.length < this.agents.length) {
      const l = new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), new THREE.Vector3()]), new THREE.LineBasicMaterial({ color: new THREE.Color(0.6, 1.8, 2.4), transparent: true, opacity: 0.7, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }))
      l.visible = false
      this.scene.add(l)
      this.beams.push(l)
    }
    this.beams.forEach((l, i) => {
      const a = gathered[i]
      l.visible = !!a && ((c.running && (c.stage === 1 || c.stage === 2)) || reads(a))
      if (!l.visible) return
      const at = (l.geometry as THREE.BufferGeometry).attributes.position as THREE.BufferAttribute
      const h = a.a.hand.getWorldPosition(new THREE.Vector3())
      at.setXYZ(0, h.x, h.y, h.z)
      const k = CORE_R / Math.max(0.01, h.length())
      at.setXYZ(1, h.x * k, h.y * k, h.z * k)
      at.needsUpdate = true
      ;(l.material as THREE.LineBasicMaterial).opacity = 0.35 + Math.random() * 0.5
    })
    // Sintesis & konflik: a lightning bolt between two astronauts
    if (!this.bolt) {
      this.bolt = new THREE.Line(new THREE.BufferGeometry().setFromPoints(Array.from({ length: 10 }, () => new THREE.Vector3())), new THREE.LineBasicMaterial({ color: new THREE.Color(2.2, 1.4, 3), transparent: true, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }))
      this.scene.add(this.bolt)
    }
    this.boltT -= dt
    if (c.running && c.stage === 2 && gathered.length > 1 && this.boltT <= 0) {
      this.boltT = 0.35
      const i = Math.floor(Math.random() * gathered.length)
      const j = (i + 1 + Math.floor(Math.random() * (gathered.length - 1))) % gathered.length
      const a = gathered[i].a.hand.getWorldPosition(new THREE.Vector3())
      const b = gathered[j].a.hand.getWorldPosition(new THREE.Vector3())
      const at = (this.bolt.geometry as THREE.BufferGeometry).attributes.position as THREE.BufferAttribute
      for (let k = 0; k < 10; k++) {
        const p = a.clone().lerp(b, k / 9)
        if (k > 0 && k < 9) p.add(new THREE.Vector3(Math.random() - 0.5, Math.random() - 0.5, Math.random() - 0.5).multiplyScalar(0.35))
        at.setXYZ(k, p.x, p.y, p.z)
      }
      at.needsUpdate = true
    }
    this.bolt.visible = c.running && c.stage === 2 && this.boltT > 0.18
    // shock wave (Belajar)
    const w = this.core.wave!
    this.core.waveT += dt
    const wk = this.core.waveT / 1.6
    w.visible = wk < 1
    if (wk < 1) { w.scale.setScalar(CORE_R + wk * 9); (w.material as THREE.MeshBasicMaterial).opacity = (1 - wk) * 0.9 }
  }

  private stepComets(dt: number) {
    const ids = [...this.comets.keys()]
    // the radar scans the comets in turn
    this.radar.timer -= dt
    if (this.radar.timer <= 0 || (this.radar.focus && !this.comets.has(this.radar.focus))) {
      this.radar.timer = 6
      const i = this.radar.focus ? ids.indexOf(this.radar.focus) : -1
      this.radar.focus = ids.length ? ids[(i + 1) % ids.length] : null
    }
    for (const v of this.comets.values()) {
      v.phase += dt * 0.045
      v.enter = this.reduced ? 1 : Math.min(1, v.enter + dt / 4)
      const e = smooth(v.enter)
      const k = 1 + (1 - e) * 0.9 // comes in from deep space
      const x = COMET_A * Math.cos(v.phase) * k
      const z = COMET_B * Math.sin(v.phase) * k
      const y = Math.sin(v.phase) * COMET_B * Math.sin(v.incl) * k + 2.2
      v.g.position.set(x, y, z)
      const away = v.g.position.clone().normalize()
      v.tail.position.copy(v.g.position)
      v.tail.quaternion.setFromUnitVectors(new THREE.Vector3(0, -1, 0), away)
      const scanned = this.radar.focus === v.comet.id
      ;(v.tail.material as THREE.ShaderMaterial).uniforms.uAlpha.value = 0.65 + (scanned ? 0.35 + Math.sin(this.t * 6) * 0.2 : 0)
      v.head.scale.setScalar(scanned ? 1.25 + Math.sin(this.t * 8) * 0.15 : 1)
      v.label.position.set(x, y + 0.45, z)
    }
    // dish, scan line and pings
    const f = this.radar.focus ? this.comets.get(this.radar.focus) : null
    const line = this.radar.line!
    line.visible = !!f && !this.cycle.running
    if (f) {
      const target = f.g.position
      const piv = this.radar.pivot
      const m = new THREE.Matrix4().lookAt(target, piv.position, new THREE.Vector3(0, 1, 0))
      const q = new THREE.Quaternion().setFromRotationMatrix(m)
      piv.quaternion.slerp(q, 1 - Math.exp(-dt * 2.5))
      const from = this.dishTip()
      const at = (line.geometry as THREE.BufferGeometry).attributes.position as THREE.BufferAttribute
      at.setXYZ(0, from.x, from.y, from.z)
      at.setXYZ(1, target.x, target.y, target.z)
      at.needsUpdate = true
      line.computeLineDistances()
      ;(line.material as THREE.LineDashedMaterial).opacity = 0.35 + Math.abs(Math.sin(this.t * 3)) * 0.25
      this.radar.ping -= dt
      if (this.radar.ping <= 0 && !this.cycle.running) {
        this.radar.ping = 0.45
        const id = f.comet.id
        this.spawnPulse(() => this.dishTip(), () => this.comets.get(id)?.g.position.clone() ?? this.dishTip(), '#7dffc8', 1.3, 0.32, 0)
      }
    } else this.radar.pivot.rotation.y += dt * 0.25
  }

  private step(dt: number) {
    this.t += dt
    const mat = this.core.mat!
    mat.uniforms.uTime.value = this.t
    const boost = this.cycle.running ? 0.6 + Math.sin(this.t * 5) * 0.25 : 0.1 + Math.sin(this.t * 1.2) * 0.06
    mat.uniforms.uBoost.value = boost
    this.core.mesh!.scale.setScalar(1 + boost * 0.04)
    this.core.corona!.material.opacity = 0.22 + boost * 0.25
    this.core.light!.intensity = 26 + boost * 14
    this.core.wire!.rotation.y += dt * (this.cycle.running ? 0.5 : 0.12)
    this.core.wire!.rotation.x += dt * 0.05
    this.core.rings.forEach((r, i) => { r.rotation.z += dt * (0.3 + i * 0.17) * (i % 2 ? -1 : 1) * (this.cycle.running ? 3 : 1) })
    this.core.conduits.forEach((c, i) => { (c.material as THREE.MeshBasicMaterial).color.setRGB(0.3 + boost * 0.6, 0.9 + boost + Math.sin(this.t * 4 + i) * 0.15, 1.3 + boost * 0.6) })
    this.planetGroup.rotation.y -= dt * SPIN
    for (const v of this.planets.values()) {
      if (v.g.scale.x < 1) v.g.scale.setScalar(Math.min(1, v.g.scale.x + dt * 1.8))
      v.mesh.rotation.y += dt * 0.12
      if (v.clouds) v.clouds.rotation.y += dt * 0.16
      if (v.warn) { const s = 1 + Math.sin(this.t * 3) * 0.08; v.warn.scale.setScalar(s); (v.warn.material as THREE.MeshBasicMaterial).opacity = 0.55 + Math.sin(this.t * 3) * 0.35 }
    }
    if (this.belt) this.belt.rotation.y -= dt * 0.008
    this.stars[0].rotation.y += dt * 0.002
    for (const a of this.agents) { this.stepAgent(a, dt); if (!a.inWork) this.say(a) }
    // the commander taps the tablet while capsules wait
    const cm = this.commander
    if (cm) {
      const busy = this.dock.count > 0
      cm.arms[1].rotation.x = busy ? -1.15 + Math.sin(this.t * 6) * 0.05 : -0.2 + Math.sin(this.t * 0.8) * 0.05
      cm.arms[0].rotation.x = busy ? -0.9 + Math.sin(this.t * 3) * 0.12 : Math.sin(this.t * 0.8 + 1) * 0.08
      cm.body.rotation.y = Math.sin(this.t * 0.4) * 0.25
    }
    this.dock.caps.children.forEach((m) => { m.position.y = 0.12 + Math.sin(this.t * 2 + (m.userData.phase as number)) * 0.03; m.rotation.y += dt })
    if (this.cargo.beacon) { this.cargo.beacon.visible = this.cargo.critical && Math.sin(this.t * 6) > -0.2; }
    if (this.comms.tip) (this.comms.tip.material as THREE.MeshBasicMaterial).color.setRGB(this.comms.unread > 0 && Math.sin(this.t * 4) > 0 ? 4 : 0.8, this.comms.unread > 0 ? 0.6 : 0.8, 0.5)
    if (this.comms.bubble) this.comms.bubble.position.y = this.comms.pos.y + 1.95 + Math.sin(this.t * 2.5) * 0.05
    this.stepCycle(dt)
    this.stepComets(dt)
    this.stepFx(dt)
  }

  private place() {
    const v3 = new THREE.Vector3()
    for (const v of this.planets.values()) {
      v.g.getWorldPosition(v3)
      v.label.position.set(v3.x, v3.y - v.r - 0.5, v3.z)
    }
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
    const next = this.pending.shift()
    if (next) {
      this.addPlanet(next.p, next.R, next.r)
      if (this.reduced) this.planets.get(next.p.id)!.g.scale.setScalar(1)
    }
    this.step(this.reduced ? 0 : dt) // reduced motion: everything placed, nothing moves
    this.place()
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
    this.dist = Math.min(80, Math.max(9.2 / tan, 18.5 / (tan * aspect)))
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
  private sway() { return this.moved || this.reduced ? 0 : Math.sin(this.t * 0.06) * 0.35 }

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
    const keyOf = (t: StationTarget | null) => (!t ? null : t.kind === 'planet' ? 'p:' + t.planet.id : t.kind === 'comet' ? 'c:' + t.comet.id : t.kind === 'agent' ? 'a:' + t.agent : t.kind)
    const highlight = (k: string | null) => {
      if (k === this.hover) return
      this.hover = k
      for (const [id, v] of this.planets) {
        v.ring.visible = 'p:' + id === k
        ;(v.orbit.material as THREE.LineBasicMaterial).opacity = 'p:' + id === k ? 0.55 : 0.16
      }
      this.core.ring!.visible = k === 'core'
      this.dock.ring!.visible = k === 'dock' || k === 'commander'
      this.cargo.ring!.visible = k === 'cargo'
      this.comms.ring!.visible = k === 'comms'
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
      this.o.onHover(t ? (t.kind === 'agent' ? { ...t } : t) : null, e.clientX - r.left, e.clientY - r.top)
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
      this.zoom = Math.max(0.4, Math.min(1.5, this.zoom * Math.exp(e.deltaY * 0.004)))
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
  zoomBy(k: number) { this.zoom = Math.max(0.4, Math.min(1.5, this.zoom * k)) }
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
    this.mapCache.forEach((m) => { m.map.dispose(); m.clouds?.dispose() })
    this.geos.forEach((g) => g.dispose())
    this.mats.forEach((m) => m.dispose())
    this.texs.forEach((t) => t.dispose())
    this.composer?.dispose()
    this.renderer?.dispose()
  }

}

