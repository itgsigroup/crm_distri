// Penjualan · Prospek & funnel — ported from mockup <section id="screen-pros">
// plus renderInbList / renderInbDetail / data-inb-act.
import { useState } from 'react'
import { api, useApi } from '../api/client'
import type { InboundDetail, InboundListItem, Prospects, Toast } from '../api/types'
import { Html, Icon, Loading } from '../components/ui'
import { fmtRp } from '../lib/format'
import { useUI } from '../state/ui'

type ScoreCls = 'good' | 'warn' | 'bad' | 'neutral'

// Same thresholds as the mockup's scoreCls.
function scoreCls(x: Pick<InboundListItem, 'status' | 'score'>): ScoreCls {
  if (x.status === 'not_prospect') return 'bad'
  if (x.score == null) return 'neutral'
  return x.score >= 60 ? 'good' : x.score >= 30 ? 'warn' : 'bad'
}
const scoreBg = (c: ScoreCls) => (c === 'neutral' ? 'var(--surface-3)' : `var(--${c}-soft)`)
const scoreFg = (c: ScoreCls) => (c === 'neutral' ? 'var(--text-3)' : `var(--${c})`)
const hasCompany = (c: string) => !!c && c !== '—'

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))

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

export function ProspekScreen() {
  const { data, error } = useApi<Prospects>('/api/prospects')
  const { toast, go } = useUI()
  const [selected, setSelected] = useState<string | null>(null)
  const activeId = (selected && data?.inbound.some(x => x.id === selected) ? selected : data?.inbound[0]?.id) ?? null
  const detail = useApi<InboundDetail>(activeId ? '/api/prospects/' + encodeURIComponent(activeId) : null)

  if (!data) return <section className="screen" id="screen-pros"><Loading error={error} /></section>
  const { funnel, sources, flywheel } = data

  return (
    <section className="screen" id="screen-pros">
      <div className="pros-top">
        <div className="card">
          <div className="card-h"><h2>Funnel calon pelanggan</h2><span className="meta">{funnel.meta}</span></div>
          <ul className="fun">
            {funnel.stages.map(s => (
              <li key={s.label}>
                <span className="lbl">{s.label}<small>{s.sub}</small></span>
                <div className="bar"><i className={s.soft ? 'soft' : undefined} style={{ width: s.width + '%', ...(s.won ? { background: 'var(--good)' } : {}) }} /></div>
                <span className="n num">{s.n}</span>
                <span className="cv">{s.conv && <b className={s.conv_tone === 'warn' ? 'warn' : undefined}>{s.conv}</b>}</span>
              </li>
            ))}
          </ul>
          <div className="fun-meta">
            {funnel.meta_cards.map(m => (
              <div key={m.label}><small>{m.label}</small><b className="num">{m.value}</b><span>{m.sub}</span></div>
            ))}
          </div>
        </div>
        <div className="stack">
          <div className="card">
            <div className="card-h"><h2>Sumber &amp; win rate</h2><span className="meta">{sources.meta}</span></div>
            <ul className="src">
              {sources.rows.map(r => (
                <li key={r.label}>
                  <span>{r.label}</span>
                  <div className="bar"><i style={{ width: r.win + '%', ...(r.tone ? { background: `var(--${r.tone})` } : {}) }} /></div>
                  <span className="n num">{r.n}</span>
                  <span className="wr"><b>{r.win}%</b> win</span>
                </li>
              ))}
            </ul>
            {sources.note && <p style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 10, lineHeight: 1.5 }}>{sources.note}</p>}
          </div>
        </div>
      </div>

      <div className="inb">
        <div className="card">
          <div className="card-h"><h2>Nomor baru masuk</h2><span className="ai" style={{ marginLeft: 6 }}>Identity agent</span><span className="meta">7 hari</span></div>
          <div className="inb-list" id="inb-list">
            {data.inbound.map(x => {
              const cls = scoreCls(x)
              return (
                <button key={x.id} className={'ib ' + (x.id === activeId ? 'is-active' : '')} onClick={() => setSelected(x.id)}>
                  <span className="sc" style={{ background: scoreBg(cls), color: scoreFg(cls) }}>
                    {x.status === 'not_prospect' ? '✕' : x.score == null ? '?' : x.score}
                  </span>
                  <div>
                    <b>{x.lead ? '✓ ' : ''}{x.name}{hasCompany(x.company) ? ' · ' + x.company : ''}</b>
                    <span className="no">{x.no} → {x.via}</span>
                    <div className="fm">“{x.first}”</div>
                  </div>
                  <span className="rt">{x.when}</span>
                </button>
              )
            })}
          </div>
          <p style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 12, lineHeight: 1.5 }}>
            Hanya nomor yang <b>menghubungi kita lebih dulu</b> yang diidentifikasi. Sumber: Getcontact, profil WhatsApp Business, Truecaller, Odoo, riwayat chat, dan web publik. Skor = seberapa yakin identitasnya × seberapa cocok dengan segmen GSI.
          </p>
        </div>
        <div className="stack" id="inb-detail">
          {detail.data && detail.data.id === activeId
            ? <InbDetail x={detail.data} />
            : activeId && <Loading error={detail.error} />}
        </div>
      </div>

      <div className="card">
        <div className="card-h"><h2>Funnel atau flywheel?</h2><span className="ai" style={{ marginLeft: 6 }}>analisis ARC</span><span className="meta">{flywheel.meta}</span></div>
        <div className="fw">
          <svg viewBox="0 0 400 300" role="img" aria-label="Flywheel GSI">
            <defs><marker id="arw" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto"><path d="M0 0L10 5 0 10z" fill="var(--text-3)" /></marker></defs>
            <circle cx="200" cy="150" r="92" fill="none" stroke="var(--surface-3)" strokeWidth="22" />
            <path d="M200 58 A92 92 0 0 1 279.7 196" fill="none" stroke="var(--accent)" strokeWidth="22" strokeLinecap="round" />
            <path d="M279.7 196 A92 92 0 0 1 120.3 196" fill="none" stroke="var(--indigo)" strokeWidth="22" strokeLinecap="round" />
            <path d="M120.3 196 A92 92 0 0 1 200 58" fill="none" stroke="var(--good)" strokeWidth="22" strokeLinecap="round" />
            <path d="M168 38 A118 118 0 0 1 230 36" fill="none" stroke="var(--text-3)" strokeWidth="2" markerEnd="url(#arw)" />
            <text className="ctr" x="200" y="146" textAnchor="middle">{flywheel.center}</text>
            <text className="ctr2" x="200" y="164" textAnchor="middle">PIPELINE DARI</text><text className="ctr2" x="200" y="177" textAnchor="middle">PELANGGAN LAMA</text>
            <text className="seg-lbl" x="310" y="118" textAnchor="start">Libatkan</text><text className="seg-sub" x="310" y="132" textAnchor="start">pain point → solusi</text><text className="seg-sub" x="310" y="144" textAnchor="start">lengkap, multi-thread</text>
            <text className="seg-lbl" x="200" y="272" textAnchor="middle">Puaskan</text><text className="seg-sub" x="200" y="286" textAnchor="middle">BAST cepat · maintenance · bayar mudah</text>
            <text className="seg-lbl" x="90" y="118" textAnchor="end">Tarik</text><text className="seg-sub" x="90" y="132" textAnchor="end">referral · ekspansi</text><text className="seg-sub" x="90" y="144" textAnchor="end">tender · nomor masuk</text>
          </svg>
          <div>
            <div className="fw-m">
              {flywheel.metrics.map(m => (
                <div key={m.label}><small>{m.label}</small><b className="num">{m.value}</b><span>{m.sub}</span></div>
              ))}
            </div>
            <div className="html-go"><Html as="div" className="verdict" html={flywheel.verdict_html} /></div>
            <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
              <button className="btn primary" onClick={() => toast('Tiga metrik flywheel ditambahkan ke Denyut bisnis · dipantau harian')}>Pantau 3 metrik ini</button>
              <button className="btn ghost" onClick={() => go('cash')}>Lihat friksi di Kas</button>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

function InbDetail({ x }: { x: InboundDetail }) {
  const mutate = useMutate()
  const cls = scoreCls(x)
  const base = '/api/prospects/' + encodeURIComponent(x.id)
  const post = (p: string, body?: unknown) => mutate(api.post<Toast>(base + p, body))
  const who = hasCompany(x.company) ? x.company : x.name

  return (
    <>
      <div className="card">
        <div className="card-h"><h2>Siapa ini</h2><span className="meta">{x.no} · menghubungi {x.via} · {x.when}</span></div>
        <div className="idh">
          <div>
            <div className="big">{x.name}</div>
            <div className="sub">{x.role}{hasCompany(x.company) && <> · <b>{x.company}</b></>}</div>
          </div>
          <div className="conf">
            <b className="num" style={{ color: scoreFg(cls) }}>{x.status === 'not_prospect' ? 'Bukan' : x.score == null ? '?' : x.score}</b>
            <small>{x.status === 'not_prospect' ? 'prospek' : 'skor identitas × kecocokan'}</small>
          </div>
        </div>
        <div className="hr" />
        <ul className="srcs">
          {x.sources.map((sv, i) => (
            <li key={i}><span className="s"><i className={sv.c} />{sv.s}</span><span className="v">{sv.v}</span></li>
          ))}
        </ul>
        <div className="hr" />
        <div className="card-h" style={{ marginBottom: 6 }}><h2 style={{ fontSize: 13 }}>Overview</h2><span className="ai" style={{ marginLeft: 6 }}>Research agent</span></div>
        <p style={{ fontSize: 13.5, lineHeight: 1.55 }}>{x.overview}</p>
        <div style={{ display: 'flex', gap: 8, marginTop: 14, flexWrap: 'wrap' }}>
          {x.status === 'not_prospect' ? (
            <button className="btn ghost" onClick={() => post('/undo')}>Ternyata prospek</button>
          ) : x.lead || x.status === 'lead' ? (
            <span className="pill good"><Icon n="i-check" />Lead dibuat di Odoo · {x.via} ditugaskan</span>
          ) : (
            <>
              <button className="btn primary" onClick={() => post('/lead')}><Icon n="i-check" />Buat lead di Odoo</button>
              <button className="btn ghost" onClick={() => post('/reply-first')}><Icon n="i-chat" />Balas dengan pertanyaan pertama</button>
              <button className="btn quiet" onClick={() => post('/not-prospect')}>Bukan prospek</button>
            </>
          )}
        </div>
      </div>

      {x.solutions.length > 0 && (
        <div className="card">
          <div className="card-h">
            <h2>Alternatif solusi untuk {who}</h2>
            <span className="meta">Diminta: {fmtRp(x.requested)} · Peluang tambahan: <b style={{ color: 'var(--text)' }}>{fmtRp(x.extra)}</b></span>
          </div>
          <ul className="sol">
            {x.solutions.map((q, i) => (
              <li key={i} className={q.k}><div><b>{q.t}</b><span>{q.why}</span></div><em className="num">{fmtRp(q.v)}</em></li>
            ))}
          </ul>
          <p style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 10, lineHeight: 1.5 }}>
            Urutan penawaran: jawab yang diminta dulu (cepat, tanpa syarat), gali pain point lewat pertanyaan di bawah, baru tawarkan paket saat ≥ 2 kebutuhan terkonfirmasi.
          </p>
        </div>
      )}

      {x.questions.length > 0 && (
        <div className="card">
          <div className="card-h">
            <h2>Pertanyaan pain point</h2>
            <span className="ai" style={{ marginLeft: 6 }}>disusun untuk {x.role.split(' ')[0]}</span>
            <span className="meta">Tiap jawaban membuka satu solusi</span>
          </div>
          <ul className="pq">
            {x.questions.map((q, i) => (
              <li key={i}>
                <span className="k">{i + 1}</span>
                <div><div className="qq">{q.q}</div><div className="un">Membuka: {q.u}</div></div>
                <button className="btn ghost" onClick={() => post('/question', { index: i })}>Pakai</button>
              </li>
            ))}
          </ul>
          <div style={{ display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' }}>
            <button className="btn ghost" onClick={() => post('/talking-points')}><Icon n="i-send" />Kirim sebagai talking points ke {x.via}</button>
          </div>
        </div>
      )}
    </>
  )
}
