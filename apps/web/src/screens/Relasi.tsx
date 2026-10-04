// Relasi — account list + full account page (mockup #screen-rel: renderList, renderAccount,
// smap, ledgerRow, mountAccNet). All content comes from /api/accounts.
import { useEffect, useRef, useState } from 'react'
import { api, useApi } from '../api/client'
import type { AccountListItem, AccountPage, Action, LedgerRow, Network, Stakeholder } from '../api/types'
import { Icon, Loading, Ring } from '../components/ui'
import { fmtRp, hb, hcol } from '../lib/format'
import { createNet, type NetInstance } from '../net/createNet'
import { useUI } from '../state/ui'

// Static UI vocabulary (same as the mockup's TAG_LABEL / VIA_ICON).
const TAG_LABEL: Record<string, string> = { decision: 'Pengambil keputusan', champion: 'Champion', influencer: 'Pemengaruh', user: 'Pengguna', ghost: 'Mutasi' }
const VIA_ICON: Record<string, string> = { mail: 'i-mail', chat: 'i-chat', people: 'i-people', doc: 'i-doc', form: 'i-form', box: 'i-box', phone: 'i-phone' }
const ANCHORS: [string, string][] = [['sec-next', 'Langkah'], ['sec-exp', 'Ekspansi'], ['sec-memo', 'Memori'], ['sec-people', 'Orang'], ['sec-deal', 'Deal'], ['sec-net', 'Peta'], ['sec-commit', 'Komitmen'], ['sec-tl', 'Timeline']]

type Mutation = { action?: Action; toast: string }

function useMutate() {
  const { toast, refresh } = useUI()
  return async (path: string, body?: unknown) => {
    try {
      const r = await api.post<Mutation>(path, body)
      toast(r.toast)
      refresh()
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e))
    }
  }
}

export function RelasiScreen() {
  const { route, go } = useUI()
  const [q, setQ] = useState('')
  const { data: list, error: listError } = useApi<{ items: AccountListItem[]; total_active: number }>('/api/accounts?q=')
  const items = list?.items ?? []
  const activeId = route.param || items[0]?.id || ''
  const f = q.toLowerCase()
  const shown = items.filter(d => !f || (d.name + d.opp + d.owner).toLowerCase().includes(f))

  return (
    <section className="screen" id="screen-rel">
      <div className="rel">
        <div className="card acc-list">
          <div className="search">
            <Icon n="i-search" />
            <input id="acc-search" type="text" placeholder="Cari akun, orang, proyek…" value={q} onChange={e => setQ(e.target.value)} />
          </div>
          <div className="list" id="acc-list">
            {listError && !list && <div style={{ color: 'var(--bad)', fontSize: 12.5, padding: 8 }}>Gagal memuat: {listError}</div>}
            {shown.map(d => (
              <button key={d.id} className={'acc-item' + (d.id === activeId ? ' is-active' : '')} onClick={() => go('rel:' + d.id)}>
                <span className={'dot ' + (d.band || hb(d.health))} />
                <div>
                  <b>{d.name}</b>
                  <span>{d.opp}</span>
                  <div className="row2"><span>{d.last} · {d.owner}</span><em className="num">{fmtRp(d.value)}</em></div>
                </div>
              </button>
            ))}
          </div>
        </div>
        <div className="stack" id="acc-page">
          {activeId ? <AccountView id={activeId} /> : list ? null : <Loading error={listError} />}
        </div>
      </div>
    </section>
  )
}

function AccountView({ id }: { id: string }) {
  const { data: d, error } = useApi<AccountPage>('/api/accounts/' + encodeURIComponent(id))
  if (!d || d.id !== id) return <Loading error={error} />
  return <AccountBody d={d} />
}

function AccountBody({ d }: { d: AccountPage }) {
  const { openSheet, toast, go } = useUI()
  const mutate = useMutate()
  const base = '/api/accounts/' + encodeURIComponent(d.id)
  const scrollTo = (id: string) => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  const a = d.next_action
  const exp = d.expansion
  const peluang = exp ? exp.whitespace.filter(w => w.st === 'peluang') : []

  const override = () => {
    if (!d.deal) return
    const reason = window.prompt('Alasan override stage (tercatat di audit log):')
    if (!reason || !reason.trim()) return
    mutate('/api/opportunities/' + encodeURIComponent(d.deal.opportunity_id) + '/override', { reason: reason.trim() })
  }

  return (
    <>
      <div className="card acc-head">
        <div className="ttl">
          <h2>{d.name}</h2>
          <div className="tags">
            <span className="pill neutral">{d.sector}</span>
            <span className="pill neutral"><Icon n="i-building" />{d.branch}</span>
            <span className="pill neutral">Owner {d.owner}</span>
          </div>
        </div>
        <div className="kpis">
          <div className="kpi h">
            <Ring h={d.health} />
            <div>
              <small>Health</small>
              <b className="num" style={{ color: d.trend >= 0 ? 'var(--good)' : 'var(--bad)', fontSize: 15 }}>
                {d.trend >= 0 ? '↑' : '↓'}{Math.abs(d.trend)}<span className="tr" style={{ color: 'var(--text-3)' }}>30 hari</span>
              </b>
            </div>
          </div>
          <div className="kpi"><small>Nilai terbuka</small><b className="num">{fmtRp(d.value)}</b></div>
          <div className="kpi"><small>Interaksi terakhir</small><b style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 15 }}><Icon n={d.last_icon || 'i-mail'} />{d.last}</b></div>
        </div>
      </div>

      <div className="anchors">
        {ANCHORS.map(([id, l]) => <button key={id} onClick={() => scrollTo(id)}>{l}</button>)}
      </div>

      {a && (
        <div className="card next" id="sec-next">
          <div className="card-h"><h2>Langkah berikutnya</h2><span className="ai" style={{ marginLeft: 6 }}>disarankan ARC</span><span className="meta">{d.next_meta}</span></div>
          <div className="next-row">
            <span className="ni"><Icon n={a.icon} /></span>
            <div><b>{a.title}</b><span>{a.why}</span></div>
            <div className="btns">
              {a.status === 'executed' ? (
                <span className="pill good"><Icon n="i-check" />Selesai</span>
              ) : a.status === 'rejected' ? (
                <span className="pill bad"><Icon n="i-x" />Ditolak · {(a.reject_reason || '').split(' — ')[0]}</span>
              ) : (
                <>
                  <button className="btn primary" onClick={() => openSheet(a)}><Icon n={a.icon} />{a.button_label}</button>
                  <button className="btn ghost" onClick={() => toast('Alternatif lain ditampilkan di bawah')}>Lihat alternatif</button>
                </>
              )}
            </div>
          </div>
          {d.alternatives.length > 0 && (
            <div className="next-alt">
              <span style={{ fontSize: 11.5, color: 'var(--text-3)', alignSelf: 'center' }}>Alternatif:</span>
              {d.alternatives.map(l => <button key={l} className="btn ghost" onClick={() => mutate(base + '/alternative', { label: l })}>{l}</button>)}
            </div>
          )}
        </div>
      )}

      {exp && (
        <div className="card" id="sec-exp">
          <div className="card-h">
            <h2>Sistem terpasang &amp; ruang ekspansi</h2>
            <span className="meta">Potensi ekspansi <b className="num" style={{ color: 'var(--text)' }}>{fmtRp(exp.potential)}</b> · {exp.lines} lini produk</span>
          </div>
          {exp.installed.length ? (
            <>
              <ul className="inst">
                {exp.installed.map((x, i) => (
                  <li key={i}>
                    <div><b style={{ fontWeight: 600 }}>{x.s}</b><span> · {x.y}</span></div>
                    <span>{x.w}</span>
                    <span className={'pill ' + (x.warn ? 'warn' : 'neutral')}>{x.c}</span>
                  </li>
                ))}
              </ul>
              <div className="hr" />
            </>
          ) : (
            <p style={{ fontSize: 12.5, color: 'var(--text-3)', marginBottom: 12 }}>Belum ada sistem GSI terpasang — akun baru.</p>
          )}
          <div className="ws">
            {exp.whitespace.map(w => (
              <div key={w.p} className={w.st === '-' ? '' : w.st}>
                <b>{w.p}</b>
                {w.st === 'ada' ? <small>Terpasang</small>
                  : w.st === 'proses' ? <><small>Dalam pipeline</small><em className="num">{fmtRp(w.v || 0)}</em></>
                  : w.st === 'peluang' ? <><small>{w.why}</small><em className="num">{fmtRp(w.v || 0)}</em></>
                  : <small style={{ color: 'var(--text-3)' }}>Belum relevan</small>}
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
            {peluang.map(w => (
              <button key={w.p} className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => mutate(base + '/whitespace', { line: w.p })}>+ Opportunity {w.p}</button>
            ))}
          </div>
        </div>
      )}

      <div className="card" id="sec-memo">
        <div className="card-h"><h2>Memori akun</h2><span className="ai" style={{ marginLeft: 6 }}>brief hidup</span><span className="meta">{d.memory.updated}</span></div>
        <p className="memo">{d.memory.text}</p>
        <div className="prov-row">
          {d.memory.provenance.map((p, i) => <span key={i} className="prov"><Icon n={p.icon || 'i-doc'} />{p.label}</span>)}
          <span className="prov" style={{ marginLeft: 'auto' }}>semua klaim bisa dilacak ke sumber</span>
        </div>
      </div>

      <div className="grid-2">
        <div className="card" id="sec-people">
          <div className="card-h"><h2>Peta stakeholder</h2><span className="meta">Dari pola komunikasi, bukan field manual</span></div>
          <StakeholderMap name={d.name} people={d.stakeholders} />
          <div className="legend">
            <span><i className="decision" />Pengambil keputusan</span>
            <span><i className="champion" />Champion</span>
            <span><i />Pemengaruh / pengguna</span>
            <span><i className="ghost" />Belum ada kontak</span>
            <span style={{ marginLeft: 'auto' }}>Tebal garis = intensitas</span>
          </div>
          {d.single_threaded && (
            <div className="stage-line" style={{ margin: '12px 0 0' }}>
              <span className="pill warn"><Icon n="i-alert" />Single-threaded</span>
              <span className="why">Hubungan bertumpu pada satu orang. Deal seperti ini kalah 3,1× lebih sering di riwayat GSI.</span>
            </div>
          )}
        </div>
        <div className="card" id="sec-deal">
          <div className="card-h"><h2>Deal intelligence</h2><span className="meta">{d.deal?.opp ?? ''}</span></div>
          {d.deal ? (
            <>
              <div className="stage-line">
                <span className="pill accent"><Icon n="i-target" />Odoo · {d.deal.odoo_stage}</span>
                {d.deal.signal && <span className="pill indigo">ARC: {d.deal.signal}</span>}
                <span className="why">{d.deal.stage_why}</span>
                <button className="btn quiet" style={{ marginLeft: 'auto', height: 26, fontSize: 12 }} onClick={override}>Override</button>
              </div>
              <div className="hb">
                {d.deal.breakdown.map(b => (
                  <div key={b.label} className="row">
                    <span className="lbl">{b.label}</span>
                    <div className="bar"><i style={{ width: b.value + '%', background: hcol(b.value) }} /></div>
                    <span className="v num">{b.value}</span>
                  </div>
                ))}
              </div>
              <div className="flags">
                {d.deal.flags.map(fl => (
                  <div key={fl.id} className="flag">
                    <span className="fi" style={{ background: `var(--${fl.k}-soft)`, color: `var(--${fl.k})` }}><Icon n={fl.k === 'good' ? 'i-check' : 'i-flag'} /></span>
                    <div><b>{fl.t}</b><span>{fl.s}</span></div>
                    <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => mutate('/api/signals/' + fl.id + '/act')}>{fl.act}</button>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p style={{ fontSize: 12.5, color: 'var(--text-3)' }}>Belum ada opportunity aktif.</p>
          )}
        </div>
      </div>

      <div className="card" id="sec-net">
        <div className="card-h">
          <h2>Peta koneksi WhatsApp akun ini</h2>
          <span className="meta">Ukuran = pesan/bulan · jarak = kedekatan · seret untuk memutar</span>
          <button className="btn quiet" style={{ marginLeft: 8, height: 26, fontSize: 12 }} onClick={() => go('net')}>Peta lengkap <Icon n="i-arrow" /></button>
        </div>
        <AccNet key={d.id} accountId={d.id} hasNetwork={d.has_network} />
      </div>

      <div className="card" id="sec-commit">
        <div className="card-h"><h2>Commitment ledger</h2><span className="meta">Janji dua arah, diekstrak dari percakapan</span></div>
        <div className="ledger">
          <div>
            <h3>Kami berjanji</h3>
            <ul>
              {d.commitments.kami.length ? d.commitments.kami.map((c, i) => <Ledger key={i} c={c} />)
                : <li><span /><div className="t" style={{ color: 'var(--text-3)' }}>Tidak ada komitmen terbuka</div></li>}
            </ul>
          </div>
          <div>
            <h3>Mereka berjanji</h3>
            <ul>
              {d.commitments.mereka.length ? d.commitments.mereka.map((c, i) => <Ledger key={i} c={c} />)
                : <li><span /><div className="t" style={{ color: 'var(--text-3)' }}>Belum ada janji dari pihak mereka — sinyal engagement rendah</div></li>}
            </ul>
          </div>
        </div>
      </div>

      <div className="card" id="sec-tl">
        <div className="card-h"><h2>Timeline</h2><span className="meta">Setiap interaksi + apa yang ARC simpulkan darinya</span></div>
        <ul className="tl">
          {d.timeline.map((t, i) => (
            <li key={i}>
              <span className={'n' + (t.hot ? ' hot' : '')}><Icon n={t.icon || VIA_ICON[t.via] || 'i-mail'} /></span>
              <div className="d"><span>{t.d}</span><span>{t.who}</span></div>
              <div className="t">{t.t}</div>
              <div className="x"><span className="ai" />{t.x}</div>
            </li>
          ))}
        </ul>
      </div>
    </>
  )
}

function Ledger({ c }: { c: LedgerRow }) {
  const k = c.st === 'done' ? 'done' : c.st === 'late' ? 'late' : ''
  return (
    <li>
      <span className={'ck ' + k}>{c.st === 'done' ? <Icon n="i-check" /> : c.st === 'late' ? <Icon n="i-alert" /> : null}</span>
      <div><div className="t">{c.t}</div><div className="s">{c.s}</div></div>
      {c.d ? <span className="pill bad">{c.d}</span> : c.st === 'done' ? <span className="pill good">Selesai</span> : <span className="pill neutral">Terbuka</span>}
    </li>
  )
}

// Port of smap(): stakeholders on a ring around "GSI", line width = interaction intensity.
function StakeholderMap({ name, people }: { name: string; people: Stakeholder[] }) {
  const W = 600, H = 372, cx = 300, cy = 186, R = 100
  const n = people.length
  return (
    <svg className="smap" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={'Peta stakeholder ' + name}>
      {people.map((s, i) => {
        const a = -Math.PI / 2 + i * ((2 * Math.PI) / n)
        const x = cx + R * Math.cos(a), y = cy + R * Math.sin(a)
        const vert = Math.abs(Math.cos(a)) < 0.3
        const off = vert ? R + 32 : R + 30
        const lx = cx + off * Math.cos(a), ly = cy + off * Math.sin(a)
        const anchor = vert ? 'middle' : Math.cos(a) > 0 ? 'start' : 'end'
        const dy = vert ? (Math.sin(a) > 0 ? 14 : -26) : -6
        const ini = s.initials || s.n.replace(/^(Pak|Bu|dr\.)\s+/, '').split(' ').map(w => w[0]).join('').slice(0, 2).toUpperCase()
        const X = x.toFixed(1), Y = y.toFixed(1), LX = lx.toFixed(1)
        return (
          <g key={i}>
            <line className={'ln ' + (s.s ? '' : 'ghost')} x1={cx} y1={cy} x2={X} y2={Y} strokeWidth={s.s ? 1.2 + s.s * 1.4 : 1.2} />
            <circle className={'c ' + s.tag} cx={X} cy={Y} r="21" />
            <text className="ini" x={X} y={(y + 4.5).toFixed(1)} textAnchor="middle">{ini}</text>
            <text className="nm" x={LX} y={(ly + dy).toFixed(1)} textAnchor={anchor}>{s.n}</text>
            <text className="rl" x={LX} y={(ly + dy + 15).toFixed(1)} textAnchor={anchor}>{(TAG_LABEL[s.tag] || '').toUpperCase()}</text>
            <text className="rl" style={{ fontWeight: 500, letterSpacing: 0 }} x={LX} y={(ly + dy + 30).toFixed(1)} textAnchor={anchor}>{s.role} · {s.note}</text>
          </g>
        )
      })}
      <circle className="me" cx={cx} cy={cy} r="24" />
      <text className="me-t" x={cx} y={cy + 4} textAnchor="middle">GSI</text>
    </svg>
  )
}

// Port of mountAccNet(): small createNet instance limited to this account's contacts.
function AccNet({ accountId, hasNetwork }: { accountId: string; hasNetwork: boolean }) {
  const { data } = useApi<Network>(hasNetwork ? '/api/network?period=1&account=' + encodeURIComponent(accountId) : null)
  const stageRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const labelsRef = useRef<HTMLDivElement>(null)
  const tipRef = useRef<HTMLDivElement>(null)
  const netRef = useRef<NetInstance | null>(null)
  const empty = !hasNetwork || (!!data && !data.contacts.some(c => c.acc === accountId))

  useEffect(() => {
    if (empty || !data || !stageRef.current || !canvasRef.current || !labelsRef.current || !tipRef.current) return
    const cur = netRef.current
    if (cur && cur.sameNodes(data)) { cur.update(data); return }
    cur?.destroy()
    netRef.current = null
    let cancelled = false
    createNet({
      stage: stageRef.current, canvas: canvasRef.current, labelsEl: labelsRef.current, tipEl: tipRef.current,
      network: data, contactFilter: c => c.acc === accountId, small: true,
    }).then(inst => {
      if (cancelled) { inst.destroy(); return }
      netRef.current = inst
      inst.start()
    }).catch(() => { /* leave the stage blank */ })
    return () => { cancelled = true }
  }, [data, empty, accountId])

  useEffect(() => () => { netRef.current?.destroy(); netRef.current = null }, [])

  if (empty) {
    return (
      <div className="acc-net" id="acc-net">
        <div className="th-note" style={{ height: '100%', justifyContent: 'center' }}>Belum ada percakapan WhatsApp untuk akun ini.</div>
      </div>
    )
  }
  return (
    <div className="acc-net" id="acc-net" ref={stageRef}>
      <canvas id="acc-net-canvas" ref={canvasRef} />
      <div id="acc-net-labels" ref={labelsRef} />
      <div className="tip" id="acc-net-tip" ref={tipRef} />
    </div>
  )
}
