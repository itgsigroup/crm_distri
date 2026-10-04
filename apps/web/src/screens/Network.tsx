// Peta koneksi 3D — WhatsApp sales numbers <-> contacts (mockup #screen-net).
// The map is drawn by createNet; pairs, insights and counts come from the server.
import { useEffect, useRef, useState } from 'react'
import { useApi } from '../api/client'
import type { Network } from '../api/types'
import { Icon } from '../components/ui'
import { createNet, type NetInstance } from '../net/createNet'
import { useUI } from '../state/ui'

const PERIODS: [number, string][] = [[1, '30 hr'], [2, '60 hr'], [3, '90 hr'], [6, '180 hr']]

export function NetworkScreen() {
  const { go } = useUI()
  const [period, setPeriod] = useState(1)
  const [filter, setFilter] = useState('all')
  const { data, error } = useApi<Network>('/api/network?period=' + period + '&sales=' + encodeURIComponent(filter))

  const stageRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const labelsRef = useRef<HTMLDivElement>(null)
  const tipRef = useRef<HTMLDivElement>(null)
  const netRef = useRef<NetInstance | null>(null)
  const filterRef = useRef(filter)
  const goRef = useRef(go)
  useEffect(() => { goRef.current = go }, [go])

  // Build the engine on first data; afterwards refresh in place (port of setPeriod -> NET.refresh).
  useEffect(() => {
    if (!data || !stageRef.current || !canvasRef.current || !labelsRef.current || !tipRef.current) return
    const cur = netRef.current
    if (cur && cur.sameNodes(data)) { cur.update(data); return }
    cur?.destroy()
    netRef.current = null
    let cancelled = false
    createNet({
      stage: stageRef.current, canvas: canvasRef.current, labelsEl: labelsRef.current, tipEl: tipRef.current,
      network: data,
      filterFn: id => {
        const f = filterRef.current
        return f === 'all' || id === f || (netRef.current?.edges ?? []).some(([a, b]) => a === f && b === id)
      },
      onPick: n => {
        const inst = netRef.current
        if (!inst) return
        if (!n) { inst.focus = null; return }
        if (n.type === 'contact' && n.d.acc && inst.focus === n.id) { goRef.current('rel:' + n.d.acc); return }
        inst.focus = n.id
      },
    }).then(inst => {
      if (cancelled) { inst.destroy(); return }
      netRef.current = inst
      inst.start()
    }).catch(() => { /* map stays empty; panels still render */ })
    return () => { cancelled = true }
  }, [data])

  useEffect(() => () => { netRef.current?.destroy(); netRef.current = null }, [])

  const netFilter = (id: string) => {
    filterRef.current = id
    setFilter(id)
    if (netRef.current) netRef.current.focus = null
  }

  const name = (id: string) => data?.sales.find(s => s.id === id)?.n ?? data?.contacts.find(c => c.id === id)?.n ?? id
  const pairs = data?.pairs ?? []
  const max = pairs.reduce((m, p) => Math.max(m, p.w), 0) || 1

  return (
    <section className="screen" id="screen-net">
      <div className="net">
        <div>
          <div className="net-stage" id="net-stage" ref={stageRef}>
            <canvas id="net-canvas" ref={canvasRef} />
            <div id="net-labels" ref={labelsRef} />
            <div className="net-hud">
              <span className="pill accent"><Icon n="i-chat" />WhatsApp · <span id="net-period-lbl">{data ? data.period_label + ' terakhir' : ''}</span></span>
              <span className="pill neutral" id="net-count">{data ? `${data.count.connections} koneksi · ${data.count.messages.toLocaleString('id-ID')} pesan` : ''}</span>
            </div>
            <div className="net-legend">
              <span><i style={{ background: 'var(--accent)' }} />Nomor sales</span>
              <span><i style={{ background: 'var(--good)' }} />Kontak · akun sehat</span>
              <span><i style={{ background: 'var(--warn)' }} />50–69</span>
              <span><i style={{ background: 'var(--bad)' }} />&lt; 50</span>
              <span><i style={{ background: 'var(--text-3)' }} />Tanpa deal aktif</span>
              <span>Ukuran = rata-rata pesan/bulan dalam periode · Jarak = kedekatan</span>
            </div>
            <div className="net-hint">Seret untuk memutar · scroll untuk zoom · klik node untuk fokus</div>
            <div className="tip" id="net-tip" ref={tipRef} />
          </div>
        </div>
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Lihat dari</h2><span className="meta">Filter nomor sales</span></div>
            <div className="net-filters" id="net-filters">
              {[['all', 'Semua sales'] as const, ...(data?.sales ?? []).map(s => [s.id, s.n] as const)].map(([id, n]) => (
                <button key={id} className={filter === id ? 'is-active' : ''} onClick={() => netFilter(id)}>{n}</button>
              ))}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 12, flexWrap: 'wrap' }}>
              <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)' }}>Periode</span>
              <div className="seg" id="net-periods">
                {PERIODS.map(([k, l]) => (
                  <button key={k} className={period === k ? 'is-active' : ''} onClick={() => setPeriod(k)}>{l}</button>
                ))}
              </div>
            </div>
            <p style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 8, lineHeight: 1.45 }}>
              Ukuran node dan jarak dihitung ulang dari pesan dalam periode ini. Sejauh mana bisa ditarik ke belakang bergantung riwayat yang diambil di{' '}
              <button className="ev" onClick={() => go('conn:sec-wa')}>Pengaturan → WhatsApp</button>.
            </p>
            <div className="hr" />
            <ul className="pairs" id="net-pairs">
              <li style={{ border: 0, paddingTop: 0 }}>
                <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)' }}>Pasangan terkuat</span>
              </li>
              {error && !data && <li><span style={{ color: 'var(--bad)', fontSize: 12.5 }}>Gagal memuat: {error}</span></li>}
              {pairs.map(p => (
                <li key={p.a + '|' + p.b}>
                  <div>
                    <div>{name(p.a)} ↔ <b>{name(p.b)}</b> <span style={{ color: 'var(--text-3)' }}>· {p.account}</span></div>
                    <div className="pb"><i style={{ width: (p.w / max) * 100 + '%' }} /></div>
                  </div>
                  <em className="num">{p.w}</em>
                </li>
              ))}
            </ul>
          </div>
          <div className="card">
            <div className="card-h"><h2>Pola koneksi</h2><span className="ai" style={{ marginLeft: 6 }}>dibaca ARC</span></div>
            <ul className="ins" id="net-ins">
              {(data?.insights ?? []).slice(0, 6).map((x, i) => (
                <li key={i}>
                  <span className="ii" style={{ background: `var(--${x.k === 'neutral' ? 'surface-3' : x.k + '-soft'})`, color: `var(--${x.k === 'neutral' ? 'text-2' : x.k})` }}><Icon n={x.icon} /></span>
                  <div><b>{x.t}</b><span>{x.s}</span></div>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
