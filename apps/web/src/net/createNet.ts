// Port of the mockup's createNet(): one force-directed WhatsApp network map
// (sales numbers <-> contacts) per container. Renders in 3D with three.js when
// WebGL is available, otherwise falls back to a 2D canvas projection of the same
// simulation. All data comes from the Network API shape (no globals).
import type * as THREE_NS from 'three'
import type { NetContact, NetSales, Network } from '../api/types'
import { hb } from '../lib/format'

type Three = typeof THREE_NS
type NodeMesh = THREE_NS.Mesh<THREE_NS.SphereGeometry, THREE_NS.MeshStandardMaterial>
type EdgeMesh = THREE_NS.Mesh<THREE_NS.CylinderGeometry, THREE_NS.MeshBasicMaterial>
type Edge = [string, string, number]

interface NodeBase {
  id: string
  r: number
  x: number; y: number; z: number
  vx: number; vy: number; vz: number
  tot: number
  label: HTMLDivElement
  mesh?: NodeMesh
}
export interface SalesNode extends NodeBase { type: 'sales'; d: NetSales }
export interface ContactNode extends NodeBase { type: 'contact'; d: NetContact }
export type NetNode = SalesNode | ContactNode

export interface Spring { a: NetNode; b: NetNode; w: number; len: number; k?: number; mesh?: EdgeMesh }

export interface NetOptions {
  stage: HTMLElement
  canvas: HTMLCanvasElement
  labelsEl: HTMLElement
  tipEl: HTMLElement
  network: Network
  contactFilter?: (c: NetContact) => boolean
  small?: boolean
  filterFn?: (id: string) => boolean
  onPick?: (n: NetNode | null) => void
  /** Skip three.js entirely (tests, low-power). */
  force2D?: boolean
}

export interface NetInstance {
  readonly has3D: boolean
  nodes: NetNode[]
  byNid: Record<string, NetNode>
  springs: Spring[]
  edges: Edge[]
  focus: string | null
  hover: string | null
  network: Network
  /** True when `net` yields the same node set, so update() can be used instead of rebuilding. */
  sameNodes: (net: Network) => boolean
  /** Port of I.refresh: new period/edges -> totals, contact radius, spring weights. */
  update: (net: Network) => void
  start: () => void
  stop: () => void
  destroy: () => void
  /** Draw one frame at the current stage size (no simulation step). */
  render: () => void
  step: (dt: number) => void
}

const cssVar = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()
const dpr = () => Math.min(window.devicePixelRatio || 1, 2)
const esc = (s: string | number) => String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)

function nodeKey(n: NetNode): 'accent' | 'good' | 'warn' | 'bad' | 'neutral' {
  if (n.type === 'sales') return 'accent'
  if (n.d.acc && n.d.health != null) return hb(n.d.health) as 'good' | 'warn' | 'bad'
  return 'neutral'
}

/** Subset of the network a map shows: contacts (after filter), their edges and the sales involved. */
export function selectGraph(net: Network, contactFilter?: (c: NetContact) => boolean) {
  const contacts = net.contacts.filter(contactFilter || (() => true))
  const cids = new Set(contacts.map(c => c.id))
  const salesIds = new Set(net.sales.map(s => s.id))
  const edges = net.edges.filter(([a, b]) => cids.has(b) && salesIds.has(a))
  const sids = new Set(edges.map(e => e[0]))
  const sales = contactFilter ? net.sales.filter(s => sids.has(s.id)) : net.sales
  return { contacts, cids, edges, sales }
}

function webglAvailable(): boolean {
  try {
    const c = document.createElement('canvas')
    const gl = (c.getContext('webgl2') || c.getContext('webgl')) as WebGLRenderingContext | null
    if (!gl) return false
    gl.getExtension('WEBGL_lose_context')?.loseContext()
    return true
  } catch {
    return false
  }
}

async function loadThree(force2D?: boolean): Promise<Three | null> {
  if (force2D || !webglAvailable()) return null
  try {
    return await import('three')
  } catch {
    return null
  }
}

const contactR = (tot: number, k: number) => 0.3 + Math.sqrt(tot / k) / 11
const springLen = (w: number, k: number) => 1.3 + 7 / (1 + Math.log(1 + w / k) * 0.9)

export async function createNet(o: NetOptions): Promise<NetInstance> {
  const THREE = await loadThree(o.force2D)
  const small = !!o.small
  const filterFn = o.filterFn || (() => true)

  let net = o.network
  let periodK = net.period || 1
  const g = selectGraph(net, o.contactFilter)
  const { cids } = g

  const nodes: NetNode[] = []
  const byNid: Record<string, NetNode> = {}
  const cl: Spring[] = []
  let edges: Edge[] = g.edges

  const totalsOf = (es: Edge[]) => {
    const t: Record<string, number> = {}
    es.forEach(([a, b, w]) => { t[a] = (t[a] || 0) + w; t[b] = (t[b] || 0) + w })
    return t
  }
  const totals = totalsOf(net.edges)
  const seedR = () => (Math.random() - 0.5) * (small ? 8 : 16)
  const mkLabel = (name: string, sub: string, sales: boolean) => {
    const l = document.createElement('div')
    l.className = 'net-lbl' + (sales ? ' sales' : '')
    l.append(name)
    const s = document.createElement('small')
    s.textContent = sub
    l.append(s)
    o.labelsEl.appendChild(l)
    return l
  }

  g.sales.forEach((sv, i) => {
    const a = (i / Math.max(1, g.sales.length)) * Math.PI * 2
    nodes.push({
      id: sv.id, type: 'sales', d: sv, r: small ? 0.9 : 1,
      x: Math.cos(a) * (small ? 3 : 6), y: (Math.random() - 0.5) * 3, z: Math.sin(a) * (small ? 3 : 6),
      vx: 0, vy: 0, vz: 0, tot: totals[sv.id] || 0, label: mkLabel(sv.n, sv.branch, true),
    })
  })
  g.contacts.forEach(c => {
    const t = totals[c.id] || 0
    nodes.push({
      id: c.id, type: 'contact', d: c, r: contactR(t, periodK),
      x: seedR(), y: seedR(), z: seedR(), vx: 0, vy: 0, vz: 0, tot: t, label: mkLabel(c.n, c.role, false),
    })
  })
  nodes.forEach(n => { byNid[n.id] = n })
  const springs: Spring[] = edges.map(([a, b, w]) => ({ a: byNid[a], b: byNid[b], w, len: springLen(w, periodK) }))
  g.contacts.forEach((c, i) => g.contacts.slice(i + 1).forEach(x => {
    if (c.acc && c.acc === x.acc) cl.push({ a: byNid[c.id], b: byNid[x.id], w: 0, len: 3.2, k: 0.02 })
  }))

  // ---------- renderer ----------
  let renderer: THREE_NS.WebGLRenderer | null = null
  let scene: THREE_NS.Scene | null = null
  let camera: THREE_NS.PerspectiveCamera | null = null
  let sphereGeo: THREE_NS.SphereGeometry | null = null
  let cylGeo: THREE_NS.CylinderGeometry | null = null
  let ctx: CanvasRenderingContext2D | null = null
  if (THREE) {
    try {
      renderer = new THREE.WebGLRenderer({ canvas: o.canvas, antialias: true, alpha: true })
      renderer.setPixelRatio(dpr())
    } catch {
      renderer = null
    }
  }
  const has3D = !!(THREE && renderer)
  if (THREE && renderer) {
    scene = new THREE.Scene()
    camera = new THREE.PerspectiveCamera(42, 1, 0.1, 400)
    scene.add(new THREE.HemisphereLight(0xffffff, 0x8899aa, 1.1))
    const dl = new THREE.DirectionalLight(0xffffff, 0.55)
    dl.position.set(4, 8, 6)
    scene.add(dl)
    sphereGeo = new THREE.SphereGeometry(1, 28, 20)
    cylGeo = new THREE.CylinderGeometry(1, 1, 1, 8, 1, true)
    nodes.forEach(n => {
      const m = new THREE.Mesh(sphereGeo!, new THREE.MeshStandardMaterial({ color: 0xffffff, roughness: 0.55, metalness: 0.05, transparent: true }))
      m.scale.setScalar(n.r)
      m.userData = { nid: n.id }
      n.mesh = m
      scene!.add(m)
    })
  } else {
    try {
      ctx = o.canvas.getContext('2d')
    } catch {
      ctx = null
    }
  }
  const addEdgeMesh = (sp: Spring) => {
    if (!THREE || !scene || !cylGeo) return
    const m = new THREE.Mesh(cylGeo, new THREE.MeshBasicMaterial({ color: 0x888888, transparent: true, opacity: 0.3 }))
    if (col.line) m.material.color.set(col.line)
    sp.mesh = m
    scene.add(m)
  }

  let col: Record<string, string> = {}
  const theme = () => {
    col = { accent: cssVar('--accent'), good: cssVar('--good'), warn: cssVar('--warn'), bad: cssVar('--bad'), neutral: cssVar('--text-3'), line: cssVar('--text-2'), bg: cssVar('--surface') }
    if (THREE && renderer) {
      if (col.bg) renderer.setClearColor(new THREE.Color(col.bg), 1)
      nodes.forEach(n => { const c = col[nodeKey(n)]; if (c) n.mesh!.material.color.set(c) })
      springs.forEach(sp => { if (col.line) sp.mesh?.material.color.set(col.line) })
    }
  }
  springs.forEach(addEdgeMesh)

  let yaw = 0.6
  let pitch = 0.32
  let dist = small ? 20 : 40
  let auto = true
  let raf: number | null = null
  let lastW = 0
  let lastH = 0
  let destroyed = false
  const up = THREE ? new THREE.Vector3(0, 1, 0) : null
  const v = THREE ? new THREE.Vector3() : null

  const I: NetInstance = {
    has3D, nodes, byNid, springs, edges, focus: null, hover: null, network: net,
    sameNodes, update, start, stop, destroy, render, step,
  }

  function resize() {
    const w = o.stage.clientWidth, h = o.stage.clientHeight
    if (!w || !h) return
    if (renderer && camera) {
      renderer.setSize(w, h, false)
      camera.aspect = w / h
      camera.updateProjectionMatrix()
    } else {
      const r = dpr()
      o.canvas.width = w * r
      o.canvas.height = h * r
    }
  }

  function step(dt: number) {
    const N = nodes
    for (let i = 0; i < N.length; i++) {
      const a = N[i]
      for (let j = i + 1; j < N.length; j++) {
        const b = N[j]
        let dx = a.x - b.x, dy = a.y - b.y, dz = a.z - b.z
        const d2 = dx * dx + dy * dy + dz * dz + 0.05
        const f = (small ? 4 : 6) / d2
        const d = Math.sqrt(d2)
        dx /= d; dy /= d; dz /= d
        a.vx += dx * f; a.vy += dy * f; a.vz += dz * f
        b.vx -= dx * f; b.vy -= dy * f; b.vz -= dz * f
      }
    }
    const spring = (sp: Spring, k: number) => {
      let dx = sp.b.x - sp.a.x, dy = sp.b.y - sp.a.y, dz = sp.b.z - sp.a.z
      const d = Math.sqrt(dx * dx + dy * dy + dz * dz) + 0.001
      const f = (d - sp.len) * k
      dx /= d; dy /= d; dz /= d
      sp.a.vx += dx * f; sp.a.vy += dy * f; sp.a.vz += dz * f
      sp.b.vx -= dx * f; sp.b.vy -= dy * f; sp.b.vz -= dz * f
    }
    springs.forEach(sp => spring(sp, 0.06))
    cl.forEach(sp => spring(sp, sp.k!))
    N.forEach(n => {
      n.vx -= n.x * 0.02; n.vy -= n.y * 0.03; n.vz -= n.z * 0.02
      n.vx *= 0.82; n.vy *= 0.82; n.vz *= 0.82
      n.x += n.vx * dt; n.y += n.vy * dt; n.z += n.vz * dt
    })
  }

  const related = (id: string) => filterFn(id)
  const isOn = (n: NetNode) =>
    related(n.id) && (!I.focus || I.focus === n.id || I.edges.some(([a, b]) => (a === I.focus && b === n.id) || (b === I.focus && a === n.id)))
  const edgeOn = (sp: Spring) => related(sp.a.id) && related(sp.b.id) && (!I.focus || I.focus === sp.a.id || I.focus === sp.b.id)

  function project(n: NetNode) {
    const cy = Math.cos(yaw), sy = Math.sin(yaw), cp = Math.cos(pitch), spn = Math.sin(pitch)
    const x = n.x * cy - n.z * sy
    let z = n.x * sy + n.z * cy
    const y = n.y * cp - z * spn
    z = n.y * spn + z * cp
    const w = o.stage.clientWidth, h = o.stage.clientHeight
    const S = h / (2 * dist * 0.3839)
    const f = dist / (dist + z)
    return { sx: w / 2 + x * f * S, sy: h / 2 - y * f * S, f, S, z }
  }

  function placeLabel(n: NetNode, sx: number, sy: number, vis: boolean, on: boolean, z: number) {
    const show = on && vis && (small || n.type === 'sales' || n.tot >= 30 || I.hover === n.id || I.focus === n.id || (!!I.focus && on))
    n.label.style.opacity = show ? '1' : '0'
    n.label.style.left = sx + 'px'
    n.label.style.top = sy - (n.type === 'sales' ? 40 : 34) + 'px'
    n.label.style.zIndex = String(z)
  }

  function render() {
    const w = o.stage.clientWidth, h = o.stage.clientHeight
    if (!w || !h) return
    if (lastW !== w || lastH !== h) { lastW = w; lastH = h; resize() }
    if (THREE && renderer && scene && camera && up && v) {
      const cam = camera
      cam.position.set(Math.cos(yaw) * Math.cos(pitch) * dist, Math.sin(pitch) * dist, Math.sin(yaw) * Math.cos(pitch) * dist)
      cam.lookAt(0, 0, 0)
      nodes.forEach(n => {
        const m = n.mesh!
        m.position.set(n.x, n.y, n.z)
        const on = isOn(n)
        m.material.opacity = on ? 1 : 0.12
        v.set(n.x, n.y + n.r + 0.15, n.z).project(cam)
        placeLabel(n, ((v.x + 1) / 2) * w, ((1 - v.y) / 2) * h, v.z < 1, on, Math.round((1 - v.z) * 1000))
      })
      springs.forEach(sp => {
        const a = sp.a, b = sp.b, m = sp.mesh
        if (!m) return
        m.position.set((a.x + b.x) / 2, (a.y + b.y) / 2, (a.z + b.z) / 2)
        v.set(b.x - a.x, b.y - a.y, b.z - a.z)
        const len = v.length()
        const rr = 0.035 + Math.sqrt(sp.w / periodK) * 0.012
        m.scale.set(rr, len, rr)
        m.quaternion.setFromUnitVectors(up, v.normalize())
        m.material.opacity = !sp.w ? 0 : edgeOn(sp) ? 0.18 + Math.min(0.7, sp.w / periodK / 120) : 0.03
      })
      renderer.render(scene, cam)
    } else if (ctx) {
      const r = dpr()
      ctx.setTransform(r, 0, 0, r, 0, 0)
      ctx.fillStyle = col.bg
      ctx.fillRect(0, 0, w, h)
      const P: Record<string, ReturnType<typeof project>> = {}
      nodes.forEach(n => { P[n.id] = project(n) })
      springs.forEach(sp => {
        if (!sp.w) return
        const a = P[sp.a.id], b = P[sp.b.id]
        ctx!.globalAlpha = edgeOn(sp) ? 0.18 + Math.min(0.7, sp.w / periodK / 120) : 0.04
        ctx!.strokeStyle = col.line
        ctx!.lineWidth = Math.max(1, (0.035 + Math.sqrt(sp.w / periodK) * 0.012) * 2 * a.S * a.f)
        ctx!.beginPath()
        ctx!.moveTo(a.sx, a.sy)
        ctx!.lineTo(b.sx, b.sy)
        ctx!.stroke()
      })
      ;[...nodes].sort((a, b) => P[b.id].z - P[a.id].z).forEach(n => {
        const p = P[n.id]
        const on = isOn(n)
        const rad = n.r * p.S * p.f
        ctx!.globalAlpha = on ? 1 : 0.15
        ctx!.fillStyle = col[nodeKey(n)]
        ctx!.beginPath()
        ctx!.arc(p.sx, p.sy, rad, 0, Math.PI * 2)
        ctx!.fill()
        ctx!.globalAlpha = on ? 0.35 : 0.05
        ctx!.fillStyle = '#fff'
        ctx!.beginPath()
        ctx!.arc(p.sx - rad * 0.3, p.sy - rad * 0.3, rad * 0.35, 0, Math.PI * 2)
        ctx!.fill()
        placeLabel(n, p.sx, p.sy - rad, true, on, Math.round(1000 - p.z * 10))
      })
      ctx.globalAlpha = 1
    }
  }

  function frame() {
    raf = requestAnimationFrame(frame)
    if (!o.stage.isConnected || o.stage.offsetParent === null) return
    if (!o.stage.clientWidth || !o.stage.clientHeight) return
    step(0.6)
    if (auto) yaw += 0.0022
    render()
  }

  function pickAt(e: PointerEvent): NetNode | null {
    const r = o.stage.getBoundingClientRect()
    if (THREE && camera) {
      const m = new THREE.Vector2(((e.clientX - r.left) / r.width) * 2 - 1, -((e.clientY - r.top) / r.height) * 2 + 1)
      const rc = new THREE.Raycaster()
      rc.setFromCamera(m, camera)
      const hit = rc.intersectObjects(nodes.map(n => n.mesh!))[0]
      return hit ? byNid[(hit.object.userData as { nid: string }).nid] || null : null
    }
    const mx = e.clientX - r.left, my = e.clientY - r.top
    let best: NetNode | null = null, bd = 1e9
    nodes.forEach(n => {
      const p = project(n)
      const d = Math.hypot(p.sx - mx, p.sy - my)
      const rad = n.r * p.S * p.f + 4
      if (d < rad && d < bd) { bd = d; best = n }
    })
    return best
  }

  function hoverAt(e: PointerEvent) {
    const n = pickAt(e)
    const tip = o.tipEl
    I.hover = n ? n.id : null
    o.stage.style.cursor = n ? 'pointer' : ''
    if (!n) { tip.classList.remove('show'); return }
    const r = o.stage.getBoundingClientRect()
    let px = e.clientX - r.left + 14
    const py = e.clientY - r.top + 14
    if (px + 240 > r.width) px -= 260
    tip.style.left = px + 'px'
    tip.style.top = py + 'px'
    const per = net.period_label
    const nm = (id: string) => esc(byNid[id]?.d.n ?? id)
    if (n.type === 'sales') {
      const es = I.edges.filter(([a]) => a === n.id).sort((a, b) => b[2] - a[2])
      tip.innerHTML = `<b>${esc(n.d.n)} · ${esc(n.d.branch)}</b><div class="r"><span>Nomor</span><span>${esc(n.d.no)}</span></div><div class="r"><span>Kontak aktif</span><span>${es.length}</span></div><div class="r"><span>Pesan ${esc(per)}</span><span>${n.tot}</span></div>${es[0] ? `<div class="r"><span>Terkuat</span><span>${nm(es[0][1])}</span></div>` : ''}`
    } else {
      const c = n.d
      const hasAcc = !!c.acc
      const es = I.edges.filter(([, b]) => b === n.id).sort((a, b) => b[2] - a[2])
      const months = net.months || []
      const monthly = net.monthly?.[n.id] || []
      const range = months.length ? `${esc(months[0])}→${esc(months[months.length - 1])}` : ''
      tip.innerHTML = `<b>${esc(c.n)}</b><div class="r"><span>${esc(c.role)}</span></div>${hasAcc ? `<div class="r"><span>Akun</span><span>${esc(c.account_name ?? '')}</span></div><div class="r"><span>Health akun</span><span>${c.health ?? '–'}</span></div>` : ''}<div class="r"><span>Pesan ${esc(per)}</span><span>${n.tot}</span></div>${es.map(([a, , w]) => `<div class="r"><span>↔ ${nm(a)}</span><span>${w}</span></div>`).join('')}${monthly.length ? `<div class="r" style="margin-top:4px;opacity:.75"><span>${range}</span><span>${monthly.join(' · ')}</span></div>` : ''}${c.decision && n.tot < 5 ? `<div class="r" style="margin-top:4px;opacity:.8"><span>Pengambil keputusan tanpa jalur WA</span></div>` : ''}${hasAcc && !small ? '<div class="r" style="margin-top:4px;opacity:.7"><span>Klik untuk buka akun</span></div>' : ''}`
    }
    tip.classList.add('show')
  }

  // ---------- interaction ----------
  let drag: { x: number; y: number; moved: boolean } | null = null
  let autoTimer: ReturnType<typeof setTimeout> | null = null
  const cv = o.canvas
  const onDown = (e: PointerEvent) => {
    drag = { x: e.clientX, y: e.clientY, moved: false }
    auto = false
    try { cv.setPointerCapture(e.pointerId) } catch { /* not capturable */ }
  }
  const onMove = (e: PointerEvent) => {
    if (drag) {
      const dx = e.clientX - drag.x, dy = e.clientY - drag.y
      if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
      yaw += dx * 0.008
      pitch = Math.max(-1.2, Math.min(1.2, pitch + dy * 0.006))
      drag.x = e.clientX
      drag.y = e.clientY
    } else hoverAt(e)
  }
  const onUp = (e: PointerEvent) => {
    if (drag && !drag.moved) {
      const n = pickAt(e)
      if (o.onPick) o.onPick(n)
      else I.focus = n ? n.id : null
    }
    drag = null
    if (autoTimer) clearTimeout(autoTimer)
    autoTimer = setTimeout(() => { if (!drag) auto = true }, 4000)
  }
  const onLeave = () => { o.tipEl.classList.remove('show'); I.hover = null }
  const onWheel = (e: WheelEvent) => {
    e.preventDefault()
    dist = Math.max(small ? 12 : 18, Math.min(70, dist + e.deltaY * 0.03))
  }
  cv.addEventListener('pointerdown', onDown)
  cv.addEventListener('pointermove', onMove)
  cv.addEventListener('pointerup', onUp)
  cv.addEventListener('pointerleave', onLeave)
  cv.addEventListener('wheel', onWheel, { passive: false })

  // ---------- data refresh ----------
  function sameNodes(n2: Network): boolean {
    const g2 = selectGraph(n2, o.contactFilter)
    const ids = [...g2.sales.map(s => s.id), ...g2.contacts.map(c => c.id)]
    return ids.length === nodes.length && ids.every(id => id in byNid)
  }

  function update(n2: Network) {
    net = n2
    I.network = n2
    periodK = n2.period || 1
    // Refresh node data (names, health) from the new payload.
    n2.sales.forEach(s => { const n = byNid[s.id]; if (n && n.type === 'sales') n.d = s })
    n2.contacts.forEach(c => { const n = byNid[c.id]; if (n && n.type === 'contact') n.d = c })
    const t = totalsOf(n2.edges)
    edges = n2.edges.filter(([a, b]) => cids.has(b) && a in byNid && b in byNid)
    I.edges = edges
    nodes.forEach(n => {
      n.tot = t[n.id] || 0
      if (n.type === 'contact') {
        n.r = contactR(n.tot, periodK)
        n.mesh?.scale.setScalar(n.r)
      }
    })
    // Edges that had no spring yet (pair silent in the first period) get one now.
    edges.forEach(([a, b, w]) => {
      if (!springs.some(sp => sp.a.id === a && sp.b.id === b)) {
        const sp: Spring = { a: byNid[a], b: byNid[b], w, len: springLen(w, periodK) }
        springs.push(sp)
        addEdgeMesh(sp)
      }
    })
    springs.forEach(sp => {
      const e = edges.find(([a, b]) => a === sp.a.id && b === sp.b.id)
      sp.w = e ? e[2] : 0
      sp.len = springLen(sp.w, periodK)
    })
    nodes.forEach(n => {
      if (n.type === 'contact' && n.mesh) { const c = col[nodeKey(n)]; if (c) n.mesh.material.color.set(c) }
    })
    I.focus = null
  }

  function start() {
    if (destroyed) return
    theme()
    resize()
    if (!raf) frame()
  }
  function stop() {
    if (raf) { cancelAnimationFrame(raf); raf = null }
  }

  const mq = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-color-scheme: dark)') : null
  mq?.addEventListener?.('change', theme)
  const mo = new MutationObserver(theme)
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })

  function destroy() {
    if (destroyed) return
    destroyed = true
    stop()
    if (autoTimer) clearTimeout(autoTimer)
    mq?.removeEventListener?.('change', theme)
    mo.disconnect()
    cv.removeEventListener('pointerdown', onDown)
    cv.removeEventListener('pointermove', onMove)
    cv.removeEventListener('pointerup', onUp)
    cv.removeEventListener('pointerleave', onLeave)
    cv.removeEventListener('wheel', onWheel)
    o.tipEl.classList.remove('show')
    nodes.forEach(n => n.label.remove())
    if (scene) {
      nodes.forEach(n => n.mesh?.material.dispose())
      springs.forEach(sp => sp.mesh?.material.dispose())
      scene.clear()
    }
    sphereGeo?.dispose()
    cylGeo?.dispose()
    renderer?.dispose()
    renderer = null
  }

  theme()
  for (let i = 0; i < 420; i++) step(1)
  return I
}
