// Penjualan · Pipeline — ported from mockup <section id="screen-pipe"> plus
// setView / renderPlot / renderInference / naRow / kcard / renderKanban.
import { useRef, useState, type DragEvent, type MouseEvent as ReactMouseEvent } from 'react'
import { api, useApi } from '../api/client'
import type { Action, KanbanCard, Pipeline, Toast } from '../api/types'
import { Html, Icon, Loading, Pill, Stars } from '../components/ui'
import { fmtRp, fmtRp1, hb, hcol } from '../lib/format'
import { useUI } from '../state/ui'

type View = 'kanban' | 'field'
type FieldDeal = Pipeline['field'][number]

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Runs a mutating call: toast the server message (or the error), then refresh all data.
function useMutate() {
  const { toast, refresh } = useUI()
  return async (p: Promise<Toast>) => {
    try {
      const r = await p
      if (r?.toast) toast(r.toast)
    } catch (e) {
      toast(errMsg(e))
    } finally {
      refresh()
    }
  }
}

export function PipelineScreen() {
  const { data, error } = useApi<Pipeline>('/api/pipeline')
  const { toast } = useUI()
  const mutate = useMutate()
  const [view, setView] = useState<View>('kanban')

  if (!data) return <section className="screen" id="screen-pipe"><Loading error={error} /></section>

  return (
    <section className="screen" id="screen-pipe">
      <div className="sync">
        <span className="odoo">odoo</span>
        <div><b>{data.sync.title}</b><div className="m">{data.sync.meta}</div></div>
        <Pill p={data.sync.pill} />
        <div className="act">
          <button className="btn ghost" onClick={() => mutate(api.post<Toast>('/api/sync/odoo'))}><Icon n="i-refresh" />Sinkron sekarang</button>
          <button className="btn quiet" onClick={() => toast('Membuka pipeline di Odoo')}>Buka di Odoo</button>
        </div>
      </div>

      <div className="pipe-top">
        <div className="seg" role="tablist">
          <button className={view === 'kanban' ? 'is-active' : ''} role="tab" aria-selected={view === 'kanban'} onClick={() => setView('kanban')}><Icon n="i-kanban" />Pipeline Odoo</button>
          <button className={view === 'field' ? 'is-active' : ''} role="tab" aria-selected={view === 'field'} onClick={() => setView('field')}><Icon n="i-field" />Medan ARC</button>
        </div>
        <div className="filters">
          <span className="sel"><Icon n="i-building" />Semua cabang <Icon n="i-chev" /></span>
          <span className="sel"><Icon n="i-cal" />Q3 2026 <Icon n="i-chev" /></span>
          <span className="sel"><Icon n="i-people" />Semua owner <Icon n="i-chev" /></span>
        </div>
      </div>

      <div id="view-field" hidden={view !== 'field'}>
        <div className="card field">
          <div className="card-h"><h2>{data.field_title}</h2><span className="meta">Kanan = sehat · Atas = besar · Warna = health band</span></div>
          <Plot deals={data.field} />
          <div className="legend">
            <span><i style={{ background: 'var(--good)', borderColor: 'var(--good)' }} />Health ≥ 70</span>
            <span><i style={{ background: 'var(--warn)', borderColor: 'var(--warn)' }} />50–69</span>
            <span><i style={{ background: 'var(--bad)', borderColor: 'var(--bad)' }} />&lt; 50</span>
            <span style={{ marginLeft: 'auto' }}>Klik gelembung untuk membuka akun</span>
          </div>
        </div>
        <div className="pipe-grid">
          <div className="card">
            <div className="card-h"><h2>Odoo vs bukti ARC</h2><span className="meta">Stage tetap milik Odoo · ARC mengusulkan probabilitas</span></div>
            <ul className="inf" id="inf-list">
              {data.inference.map(d => (
                <li key={d.id}>
                  <div>
                    <b>{d.account}</b>
                    <div className="why">{d.why}{d.signal && <> <span className="pill indigo" style={{ verticalAlign: 1 }}>ARC: {d.signal}</span></>}</div>
                  </div>
                  <div className="stg">
                    <span className="pill accent">Odoo · {d.stage}</span>
                    <small>Odoo {d.manual}% → ARC {d.arc}</small>
                    {d.can_write && (
                      <button className="btn quiet" style={{ height: 24, fontSize: 11.5, padding: '0 8px' }}
                        onClick={() => mutate(api.post<Toast>('/api/opportunities/' + encodeURIComponent(d.id) + '/write-probability'))}>
                        Tulis {d.arc}% ke Odoo
                      </button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          </div>
          <div className="stack">
            <div className="card">
              <div className="card-h"><h2>Dua cara membaca pipeline</h2></div>
              <div className="cmp">
                <div><small>Weighted · input sales</small><b className="num">{fmtRp1(data.weighted.sales)}</b><span>Probabilitas manual di kartu deal</span></div>
                <div><small>Weighted · bukti ARC</small><b className="num">{fmtRp1(data.weighted.arc)}</b><span>Health × nilai, dari sinyal nyata</span></div>
              </div>
              {data.weighted.note && <p className="note" style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 12, lineHeight: 1.5 }}>{data.weighted.note}</p>}
            </div>
            <div className="card">
              <div className="card-h"><h2>Pola win/loss</h2><span className="meta">{data.winloss.meta}</span></div>
              <div className="html-go"><ul className="feed">
                {data.winloss.rows.map((r, i) => (
                  <li key={i}><span className="ag">{r.tag}</span><Html as="div" html={r.html} /></li>
                ))}
              </ul></div>
            </div>
          </div>
        </div>
      </div>

      <div id="view-kanban" hidden={view !== 'kanban'}>
        <Kanban stages={data.stages} />
        <p style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 10, display: 'flex', gap: 14, flexWrap: 'wrap' }}>
          <span>Kartu = opportunity Odoo apa adanya (revenue, probabilitas, expected closing, tag, prioritas, activity).</span>
          <span>Baris putus-putus = tambahan ARC: health dan sinyal dari bukti, tidak mengubah stage Odoo.</span>
        </p>
      </div>

      <div className="grid-2" style={{ marginTop: 20 }}>
        <div className="card">
          <div className="card-h"><h2>Tender radar</h2><span className="ai" style={{ marginLeft: 6 }}>Research agent</span><span className="meta">{data.tenders.meta}</span></div>
          <div className="html-go"><ul className="tender">
            {data.tenders.items.map(t => {
              const soft = t.tone === 'neutral' ? 'var(--surface-3)' : `var(--${t.tone}-soft)`
              const fg = t.tone === 'neutral' ? 'var(--text-3)' : `var(--${t.tone})`
              const path = '/api/tenders/' + encodeURIComponent(t.id)
              return (
                <li key={t.id}>
                  <span className="mt" style={{ background: soft, color: fg }}>{t.score}</span>
                  <div><b>{t.title}</b><Html html={t.detail_html} /></div>
                  {t.status !== 'new'
                    ? <span className="pill neutral">{t.status}</span>
                    : t.primary
                      ? <button className="btn primary" style={{ height: 28, fontSize: 12 }} onClick={() => mutate(api.post<Toast>(path + '/qualify'))}>Kualifikasi</button>
                      : <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => mutate(api.post<Toast>(path + '/skip'))}>Lewati</button>}
                </li>
              )
            })}
          </ul></div>
        </div>
        <div className="card">
          <div className="card-h"><h2>Tim &amp; disiplin eksekusi</h2><span className="meta">30 hari · dari bukti, bukan laporan</span></div>
          <div className="tbl-wrap">
            <table className="team">
              <thead><tr><th>Sales</th><th>Pipeline</th><th>Respons</th><th>Follow-up tepat</th><th>Multi-thread</th><th>Coaching ARC</th></tr></thead>
              <tbody>
                {data.team.map(r => (
                  <tr key={r.name}>
                    <td className="nm">{r.name}<small>{r.branch}</small></td>
                    <td className="num">{fmtRp1(r.pipeline)}</td>
                    <td className="num">{r.response}</td>
                    <td><Pill p={r.followup} /></td>
                    <td><Pill p={r.multithread} /></td>
                    <td className="coach">{r.coach}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </section>
  )
}

// ---------- Kanban ----------
function Kanban({ stages }: { stages: Pipeline['stages'] }) {
  const mutate = useMutate()
  const [dragId, setDragId] = useState<string | null>(null)
  const [over, setOver] = useState<number | null>(null)

  const fromStage = (cardId: string) => stages.find(s => s.cards.some(c => c.id === cardId))

  const onDrop = (e: DragEvent, stageId: number) => {
    e.preventDefault()
    setOver(null)
    const id = e.dataTransfer.getData('text/plain') || dragId
    setDragId(null)
    if (!id || fromStage(id)?.id === stageId) return
    mutate(api.post<Toast>('/api/opportunities/' + encodeURIComponent(id) + '/stage', { stage_id: stageId }))
  }

  return (
    <div className="kanban" id="kanban">
      {stages.map(st => {
        const droppable = !st.won
        return (
          <div key={st.id} className={'col ' + (st.won ? 'won' : '')}
            style={over === st.id ? { outline: '2px dashed var(--accent)', outlineOffset: -2 } : undefined}
            onDragOver={droppable ? e => { if (dragId) { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; setOver(st.id) } } : undefined}
            onDragLeave={droppable ? e => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOver(o => (o === st.id ? null : o)) } : undefined}
            onDrop={droppable ? e => onDrop(e, st.id) : undefined}>
            <h3>{st.name}<span className="num">{fmtRp(st.total)}</span></h3>
            {st.cards.map(c => (
              <KCard key={c.id} d={c} extra={c.won || st.won ? 'won' : ''}
                draggable={!st.won && !c.won}
                onDragStart={() => setDragId(c.id)}
                onDragEnd={() => { setDragId(null); setOver(null) }} />
            ))}
          </div>
        )
      })}
    </div>
  )
}

function KCard({ d, extra, draggable, onDragStart, onDragEnd }: {
  d: KanbanCard; extra: string; draggable: boolean; onDragStart: () => void; onDragEnd: () => void
}) {
  const { go, toast } = useUI()
  const open = () => (d.account_id ? go('rel:' + d.account_id) : toast('Lead baru · belum ada halaman akun'))
  return (
    <div className={'kc ' + extra} tabIndex={0} role="button"
      draggable={draggable}
      onDragStart={e => { e.dataTransfer.setData('text/plain', d.id); e.dataTransfer.effectAllowed = 'move'; onDragStart() }}
      onDragEnd={onDragEnd}
      onClick={open}
      onKeyDown={e => { if (e.key === 'Enter' && e.target === e.currentTarget) open() }}>
      <b>{d.opp}</b>
      <span>{d.account}</span>
      <div className="rev num">{fmtRp(d.value)} <small>{d.prob}</small></div>
      <span style={{ fontSize: 11.5 }}><Icon n="i-cal" /> {d.closing}</span>
      <div className="kt">{d.tags.map(t => <span key={t}>{t}</span>)}</div>
      <div className="kb">
        {d.prio != null && <Stars n={d.prio} />}
        {d.activity && <span className="actv" style={{ background: `var(--${d.activity})` }} title="Activity" />}
        <span className="av">{d.owner_initials || '--'}</span>
      </div>
      {d.health != null ? (
        <div className="arc">
          <span className="ai" style={{ fontSize: 10.5 }} />
          <span className={'pill ' + hb(d.health)} style={{ margin: 0 }}>Health {d.health}</span>
          {d.signal && <span className="pill indigo">{d.signal}</span>}
        </div>
      ) : d.note ? (
        <div className="arc">
          <span className="ai" style={{ fontSize: 10.5 }} />
          <span style={{ fontSize: 11.5, color: 'var(--text-2)' }}>{d.note}</span>
        </div>
      ) : null}
      {d.action && <NaRow a={d.action} />}
    </div>
  )
}

// Next-action row under a kanban card (mockup naRow).
function NaRow({ a }: { a: Action }) {
  const { openSheet } = useUI()
  if (a.status === 'rejected') {
    return (
      <div className="na rej">
        <span className="ai" style={{ fontSize: 10 }} />
        <div className="nt"><b>Ditolak · {(a.reject_reason || '').split(' — ')[0]}</b>agen tidak mengusulkan ulang 14 hari</div>
        <Icon n="i-x" />
      </div>
    )
  }
  if (a.status === 'executed') {
    return (
      <div className="na done">
        <span className="ai" style={{ fontSize: 10 }} />
        <div className="nt"><b>Dijalankan{a.decided_at_label ? ' · ' + a.decided_at_label : ''}</b></div>
        <Icon n="i-check" />
      </div>
    )
  }
  return (
    <div className="na">
      <span className="ai" style={{ fontSize: 10 }} />
      <div className="nt"><b>{a.title}</b>{a.due_label}</div>
      <button className="btn primary" draggable={false}
        onClick={e => { e.stopPropagation(); openSheet(a) }}
        onKeyDown={e => e.stopPropagation()}>
        <Icon n={a.icon} />{a.button_label}
      </button>
    </div>
  )
}

// ---------- Medan ARC (renderPlot) ----------
const W = 820, H = 430, L = 64, R = 24, T = 22, B = 48, STEP = 0.5e9

function Plot({ deals }: { deals: FieldDeal[] }) {
  const { go } = useUI()
  const wrap = useRef<HTMLDivElement>(null)
  const [tip, setTip] = useState<{ d: FieldDeal; x: number; y: number } | null>(null)

  // Mockup scale tops out at Rp 3,5 M; grow in 0,5 M steps if a deal is larger.
  const maxV = Math.max(3.5e9, Math.ceil(Math.max(0, ...deals.map(d => d.value)) / STEP) * STEP)
  const x = (h: number) => L + (W - L - R) * h / 100
  const y = (v: number) => T + (H - T - B) * (1 - v / maxV)
  const grid: number[] = []
  for (let v = STEP; v <= maxV + 1; v += STEP) grid.push(v)
  const sorted = [...deals].sort((a, b) => b.value - a.value)

  const move = (e: ReactMouseEvent, d: FieldDeal) => {
    const el = wrap.current
    if (!el) return
    const rct = el.getBoundingClientRect()
    let px = e.clientX - rct.left + 14
    const py = e.clientY - rct.top + 14
    if (px + 250 > rct.width) px -= 270
    setTip({ d, x: px, y: py })
  }

  return (
    <div id="plot-wrap" ref={wrap} style={{ position: 'relative' }} onMouseLeave={() => setTip(null)}>
      <svg className="plot" viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Medan pipeline: health vs nilai">
        <rect className="zone" x={L} y={T} width={x(50) - L} height={H - T - B} rx={8} />
        {grid.map(v => (
          <g key={v}>
            <line className="gl" x1={L} x2={W - R} y1={y(v)} y2={y(v)} />
            <text x={L - 10} y={y(v) + 4} textAnchor="end">{(v / 1e9).toFixed(1).replace('.', ',')} M</text>
          </g>
        ))}
        {[0, 25, 50, 75, 100].map(h => <text key={h} x={x(h)} y={H - B + 20} textAnchor="middle">{h}</text>)}
        <line className="ax" x1={L} x2={W - R} y1={H - B} y2={H - B} />
        <line className="ax" x1={x(50)} x2={x(50)} y1={T} y2={H - B} strokeDasharray="3 4" />
        <text className="zone-lbl" x={L + 12} y={T + 18}>Perlu perhatian</text>
        <text className="zone-lbl" x={W - R - 12} y={T + 18} textAnchor="end">Sehat</text>
        <text x={(L + W - R) / 2} y={H - 8} textAnchor="middle" style={{ fontWeight: 600 }}>Health score (bukti) →</text>
        <text transform={`translate(14 ${(T + H - B) / 2}) rotate(-90)`} textAnchor="middle" style={{ fontWeight: 600 }}>Nilai deal (Rp) →</text>
        {sorted.map(d => {
          const cx = x(d.health), cy = y(d.value)
          const r = 13 + Math.sqrt(d.value / 1e9) * 6
          const right = d.health < 70
          const lx = right ? cx + r + 8 : cx - r - 8
          return (
            <g key={d.id} className="b" tabIndex={0} role="link" aria-label={d.account}
              onClick={() => go('rel:' + d.id)}
              onKeyDown={e => { if (e.key === 'Enter') go('rel:' + d.id) }}
              onMouseMove={e => move(e, d)}>
              <circle className="o" cx={cx} cy={cy} r={r} fill={hcol(d.health)} />
              <text className="h" x={cx} y={cy + 4} textAnchor="middle">{d.health}</text>
              <text className="nm" x={lx} y={cy + 4} textAnchor={right ? 'start' : 'end'}>{d.account}</text>
            </g>
          )
        })}
      </svg>
      <div className={'tip' + (tip ? ' show' : '')} id="tip" style={tip ? { left: tip.x, top: tip.y } : undefined}>
        {tip && (
          <>
            <b>{tip.d.account}</b>
            <div className="r"><span>{tip.d.opp}</span></div>
            <div className="r"><span>Nilai</span><span>{fmtRp(tip.d.value)}</span></div>
            <div className="r"><span>Stage Odoo</span><span>{tip.d.stage}{tip.d.signal ? ' · ' + tip.d.signal : ''}</span></div>
            <div className="r"><span>Health</span><span>{tip.d.health} ({tip.d.trend >= 0 ? '+' : ''}{tip.d.trend})</span></div>
            <div className="r"><span>Sales menulis</span><span>{tip.d.manual_prob}%</span></div>
            {tip.d.next && <div className="r" style={{ marginTop: 4, opacity: 0.8 }}><span>Next: {tip.d.next}</span></div>}
          </>
        )}
      </div>
    </div>
  )
}
