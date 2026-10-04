import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Network } from '../api/types'
import { createNet, type NetInstance } from './createNet'

function fakeCtx() {
  const fns = ['setTransform', 'fillRect', 'beginPath', 'moveTo', 'lineTo', 'stroke', 'arc', 'fill']
  const ctx: Record<string, unknown> = { fillStyle: '', strokeStyle: '', lineWidth: 1, globalAlpha: 1 }
  fns.forEach(f => { ctx[f] = vi.fn() })
  return ctx as unknown as CanvasRenderingContext2D & Record<string, ReturnType<typeof vi.fn>>
}

const net = (period: number, w: number): Network => ({
  months: ['Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep'],
  period,
  period_label: period === 1 ? '30 hari' : period * 30 + ' hari',
  sales: [{ id: 's1', n: 'Sales Satu', branch: 'Cabang A', no: '+62 1' }],
  contacts: [
    { id: 'c1', n: 'Kontak Satu', role: 'Peran', acc: 'a1', account_name: 'Akun A', health: 80, decision: false },
    { id: 'c2', n: 'Kontak Dua', role: 'Peran', acc: null, health: null, decision: false },
  ],
  edges: [['s1', 'c1', w], ['s1', 'c2', 2]],
  monthly: { c1: [0, 0, 0, 0, 0, w], c2: [0, 0, 0, 0, 0, 2] },
  pairs: [],
  insights: [],
  count: { connections: 2, messages: w + 2 },
})

describe('createNet (2D fallback)', () => {
  let ctx: ReturnType<typeof fakeCtx>
  let inst: NetInstance | null = null
  beforeEach(() => {
    ctx = fakeCtx()
    // jsdom has no WebGL: only a fake 2D context is available, so the engine must fall back.
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(((type: string) => (type === '2d' ? ctx : null)) as never)
  })
  afterEach(() => {
    inst?.destroy()
    inst = null
    vi.restoreAllMocks()
  })

  const mount = async () => {
    const stage = document.createElement('div')
    const canvas = document.createElement('canvas')
    const labelsEl = document.createElement('div')
    const tipEl = document.createElement('div')
    stage.append(canvas, labelsEl, tipEl)
    document.body.append(stage)
    Object.defineProperty(stage, 'clientWidth', { value: 400 })
    Object.defineProperty(stage, 'clientHeight', { value: 300 })
    inst = await createNet({ stage, canvas, labelsEl, tipEl, network: net(1, 40) })
    return { inst, labelsEl }
  }

  it('builds one node per sales and contact, with labels, without WebGL', async () => {
    const { inst, labelsEl } = await mount()
    expect(inst.has3D).toBe(false)
    expect(inst.nodes).toHaveLength(3)
    expect(inst.springs).toHaveLength(2)
    expect(labelsEl.querySelectorAll('.net-lbl')).toHaveLength(3)
    expect(labelsEl.querySelector('.net-lbl.sales')?.textContent).toContain('Sales Satu')
    inst.render()
    expect(ctx.arc).toHaveBeenCalled()
  })

  it('update() recomputes contact radius and spring weight for the new period', async () => {
    const { inst } = await mount()
    const c1 = inst.byNid.c1
    const r1 = c1.r
    inst.focus = 'c1'
    inst.update(net(6, 900))
    expect(c1.r).not.toBe(r1)
    expect(c1.tot).toBe(900)
    expect(inst.springs.find(s => s.b.id === 'c1')?.w).toBe(900)
    expect(inst.focus).toBeNull()
    expect(inst.sameNodes(net(3, 10))).toBe(true)
  })

  it('destroy() removes labels', async () => {
    const { inst, labelsEl } = await mount()
    inst.destroy()
    expect(labelsEl.children).toHaveLength(0)
  })
})
