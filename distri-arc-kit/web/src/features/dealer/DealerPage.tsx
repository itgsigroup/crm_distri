import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import type { BoardItem, Commitment, DealerDetail } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { Pill, Prov, ScoreRing } from '../../components/ui'
import { contactIni, fmtRp, hb, hcol, hhmm, shortDate } from '../../lib/format'
import { KAT, KUAD } from '../../lib/i18n/id'
import { useOrch, useOrchStatus } from '../../app/orch'
import { MemoText } from './Memo'
import { SOWSheet } from './SOWSheet'
import { useFeedback } from '../../components/feedback'
import { useDealer, useDealers } from '../../app/queries'
import { creditTone } from '../control/lists'

const VIA_ICON: Record<string, string> = { mail: 'mail', chat: 'chat', people: 'people', doc: 'doc', form: 'form', box: 'box', phone: 'phone' }
const DEALERS_PER_PAGE = 10

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

/** Search list on the left (mockup renderList): sorted by position in cycle, furthest first. */
function DealerList({ active }: { active?: string }) {
  const [q, setQ] = useState('')
  const [term, setTerm] = useState('')
  useEffect(() => {
    const t = window.setTimeout(() => setTerm(q.trim()), 200)
    return () => window.clearTimeout(t)
  }, [q])
  const [type, setType] = useState('')
  const [page, setPage] = useState(1)
  const [params, setParams] = useSearchParams()
  const status = params.get('status') ?? ''
  const { data: all = [], isPending, isError } = useDealers(term, type)
  const list = useMemo(() => (status ? all.filter((d) => d.metrics.status === status) : all), [all, status])
  const pageCount = Math.max(1, Math.ceil(list.length / DEALERS_PER_PAGE))
  const currentPage = Math.min(page, pageCount)
  const pageNumbers = [...new Set([1, currentPage - 1, currentPage, currentPage + 1, pageCount])].filter((n) => n >= 1 && n <= pageCount).sort((a, b) => a - b)
  const shown = list.slice((currentPage - 1) * DEALERS_PER_PAGE, currentPage * DEALERS_PER_PAGE)
  const nav = useNavigate()
  const { openSheet } = useFeedback()
  return (
    <div className="card acc-list">
      <div className="acc-list-title"><h2>Semua dealer</h2><span>{list.length.toLocaleString('id-ID')} dealer</span></div>
      <div className="search"><Icon name="search" /><input value={q} onChange={(e) => { setQ(e.target.value); setPage(1) }} type="text" placeholder="Cari dealer, kota, produk…" aria-label="Cari dealer" /></div>
      <div className="seg" style={{ margin: '10px 0 2px', alignSelf: 'flex-start' }} role="radiogroup" aria-label="Jenis pelanggan">
        {[['', 'Semua'], ['reseller', 'Dealer'], ['si', 'Freelance / SI']].map(([k, l]) => <button key={k} role="radio" aria-checked={type === k} className={type === k ? 'is-active' : ''} onClick={() => { setType(k); setPage(1) }}>{l}</button>)}
      </div>
      <select className="st-filter" value={status} onChange={(e) => { setParams(e.target.value ? { status: e.target.value } : {}); setPage(1) }} aria-label="Status dealer">
        <option value="">Semua status</option>
        {['Key account', 'Aktif', 'Baru', 'At risk', 'Churn', 'Prospek'].map((s) => <option key={s} value={s}>{s === 'Prospek' ? 'Prospek (belum pernah order)' : s}</option>)}
      </select>
      {status && <span className="st-count">{list.length.toLocaleString('id-ID')} dealer {status}</span>}
      <button className="btn quiet" style={{ height: 28, fontSize: 12, margin: '8px 0 4px', alignSelf: 'flex-start' }} onClick={() => openSheet(<SOWSheet />)}><Icon name="check" />Konfirmasi share of wallet</button>
      <div className="list">
        {shown.map((d) => {
          const m = d.metrics
          return (
            <button key={d.id} className={`acc-item ${d.id === active ? 'is-active' : ''}`} onClick={() => nav('/dealer/' + d.id + (status ? '?status=' + encodeURIComponent(status) : ''))}>
              <span className={`dot ${hb(m.score)}`} />
              <div>
                <b>{d.name}</b>
                <span>{m.status === 'Baru' ? 'Aktif' : m.status} · {d.customer_type === 'si' ? 'SI' : 'Dealer'} · {KUAD[m.segment].n} · {d.city}{d.tier ? ` · tier ${d.tier}` : ''}</span>
                <div className="row2">
                  <span>{m.rhythm_days ? (m.due_in! >= 0 ? `jadwal order ${m.due_in} hr` : `lewat ${-m.due_in!} hr`) : m.status === 'Prospek' ? 'belum pernah order' : m.status === 'Churn' ? `order terakhir ${m.last_order_days} hr lalu` : 'baru'} · {d.owner.name}</span>
                  <em className="num">{fmtRp(m.avg_order)}/order</em>
                </div>
              </div>
            </button>
          )
        })}
        {!isPending && !isError && list.length === 0 && <p className="acc-list-empty">Tidak ada dealer yang sesuai.</p>}
        {isPending && <p className="acc-list-empty">Memuat dealer…</p>}
        {isError && <p className="acc-list-empty">Daftar dealer gagal dimuat.</p>}
      </div>
      {!isPending && !isError && list.length > 0 && <div className="acc-list-pagination">
        <span>{(currentPage - 1) * DEALERS_PER_PAGE + 1}–{Math.min(currentPage * DEALERS_PER_PAGE, list.length)} dari {list.length.toLocaleString('id-ID')}</span>
        <div aria-label="Paginasi dealer">
          <button type="button" disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)} aria-label="Halaman sebelumnya"><Icon name="chev" /></button>
          {pageNumbers.map((n, index) => <span className="acc-page-slot" key={n}>{index > 0 && n - pageNumbers[index - 1] > 1 && <span aria-hidden="true">…</span>}<button type="button" className={currentPage === n ? 'is-active' : ''} aria-label={`Halaman ${n}`} aria-current={currentPage === n ? 'page' : undefined} onClick={() => setPage(n)}>{n}</button></span>)}
          <button type="button" disabled={currentPage === pageCount} onClick={() => setPage(currentPage + 1)} aria-label="Halaman berikutnya"><Icon name="chev" /></button>
        </div>
      </div>}
    </div>
  )
}

/** Order-to-cash ring (mockup siklusSvg). */
function Siklus({ phase, days }: { phase: number; days: number }) {
  const ph = ['Order', 'Siap', 'Kirim', 'Invoice', 'Bayar']
  const c = 2 * Math.PI * 44
  const seg = c / 5
  const gap = 6
  return (
    <svg viewBox="0 0 120 120" className="siklus">
      {ph.map((p, i) => {
        const on = i <= phase
        return <circle key={p} cx="60" cy="60" r="44" fill="none" stroke={on ? (i === phase ? 'var(--accent)' : 'var(--good)') : 'var(--surface-3)'} strokeWidth="10" strokeDasharray={`${seg - gap} ${c - seg + gap}`} strokeDashoffset={-i * seg} transform="rotate(-90 60 60)" strokeLinecap="butt" />
      })}
      <text x="60" y="56" textAnchor="middle" className="sk-t">{ph[phase]}</text>
      <text x="60" y="72" textAnchor="middle" className="sk-s">{days} hr/putaran</text>
    </svg>
  )
}

function LedgerRow({ c }: { c: Commitment }) {
  const k = c.status === 'done' ? 'done' : c.status === 'late' ? 'late' : ''
  return (
    <li>
      <span className={`ck ${k}`}>{c.status === 'done' ? <Icon name="check" /> : c.status === 'late' ? <Icon name="alert" /> : null}</span>
      <div><div className="t">{c.title}</div><div className="s">{c.detail}</div></div>
      {c.status === 'late' ? <span className="pill bad">Lewat {c.late_days} hari</span> : c.status === 'done' ? <span className="pill good">Selesai</span> : <span className="pill neutral">Terbuka</span>}
    </li>
  )
}

function statusTone(s: string) {
  return s === 'Key account' ? 'good' : s === 'Aktif' || s === 'Baru' ? 'accent' : s === 'At risk' ? 'warn' : s === 'Prospek' ? 'neutral' : 'bad'
}

function activityLabel(a: string) {
  return a === 'normal' ? 'Aktivitas normal' : a === 'menurun' ? 'Aktivitas menurun' : a === 'berhenti' ? 'Tidak aktif' : 'Belum ada siklus order'
}

function DealerBody({ d }: { d: DealerDetail }) {
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  const m = d.metrics
  const cr = m.credit
  const room = cr.room
  const status = m.status === 'Baru' ? 'Aktif' : m.status
  const months = d.orders.months
  const maxM = Math.max(...months.map((x) => x.total), 1)
  const parts: [string, number][] = [['Siklus order', m.score_parts.r], ['Share of wallet', m.score_parts.p], ['Product mix', m.score_parts.k], ['Sisa limit', m.score_parts.n], ['PIC aktif', m.score_parts.i]]
  const phase = d.orders.last?.phase ?? 0
  const tone = creditTone(cr.state)
  const issue = (m.rhythm_days != null && m.cyc > 1.2) || tone === 'bad' || (m.due_in != null && m.due_in >= 0 && m.due_in <= 7)
  const empty = KAT.filter((_, i) => !m.mix_cats[i])
  const active = d.contacts.filter((c) => c.active).length
  const late = (d.commitments.mereka ?? []).filter((c) => /Bayar/.test(c.title))
  return (
    <>
      <div className="card acc-head">
        <div className="ttl">
          <h2>{d.name}</h2>
          <div className="tags">
            <Pill tone={statusTone(m.status)} icon="target">{status}</Pill>
            <Pill tone="neutral" icon="people">{d.customer_type === 'si' ? 'Freelance / System Integrator' : 'Dealer (reseller)'}</Pill>
            <Pill tone="neutral" icon="building">{d.city} · cabang {d.branch}</Pill>
            {(d.tier || d.segment_desc) && <Pill tone="neutral">{[d.tier && `Tier ${d.tier}`, d.segment_desc].filter(Boolean).join(' · ')}</Pill>}
            <Pill tone="neutral" icon="chart">{KUAD[m.segment].n} · {KUAD[m.segment].s}</Pill>
            <Pill tone="neutral">Sales {d.owner.name}</Pill>
            <button className="btn ghost" style={{ height: 26, fontSize: 12, marginLeft: 'auto' }} disabled={orch.running} onClick={() => reanalyze('dealer:' + d.id)}><Icon name="refresh" />Analisis ulang</button>
          </div>
        </div>
        <div className="kpis" style={{ margin: '14px 0 0', flexBasis: '100%' }}>
          <div className="kpi h"><ScoreRing h={m.score} /><div><small>Skor dealer</small><b className="num" style={{ fontSize: 15 }}>{activityLabel(m.activity)}</b></div></div>
          <div className="kpi">
            <small>Siklus order · jadwal order</small>
            <b className="num">{m.rhythm_days ? m.rhythm_days + ' hr' : 'baru'}</b>
            <span style={{ fontSize: 11.5, color: m.rhythm_days && m.due_in! < 0 ? 'var(--bad)' : 'var(--text-3)' }}>
              {m.rhythm_days ? (m.due_in! >= 0 ? 'jadwal order ' + (m.due_in === 0 ? 'hari ini' : m.due_in + ' hari lagi') : 'lewat ' + -m.due_in! + ' hari') : '1 order · belum terbentuk'}
            </span>
          </div>
          <div className="kpi"><small>Share of wallet · product mix</small><b className="num">{m.sow}% · {m.mix}/6</b><span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>share of wallet · kategori dibeli</span></div>
          <div className="kpi">
            <small>Sisa limit</small>
            <b className="num" style={{ color: `var(--${tone === 'neutral' ? 'text' : tone})` }}>{room == null ? 'cash' : room < 0 ? 'over limit' : Math.round(room * 100) + '%'}</b>
            <span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>
              {room == null ? 'tanpa kredit' : (cr.exposure > cr.limit ? 'over limit · lewat limit ' + fmtRp(cr.exposure - cr.limit) : (cr.state === 'overdue' ? 'overdue · invoice lewat tempo' : cr.state) + ' · ' + fmtRp(cr.limit - cr.exposure) + ' tersisa') + ' · bayar ' + cr.pay_days + ' hr'}
            </span>
          </div>
        </div>
      </div>

      <div className="anchors">
        <button onClick={() => scrollTo('sec-next')}>Langkah</button>
        <button onClick={() => scrollTo('sec-siklus')}>Order-to-cash</button>
        <button onClick={() => scrollTo('sec-porsi')}>Share of wallet &amp; product mix</button>
        <button onClick={() => scrollTo('sec-napas')}>Sisa limit</button>
        <button onClick={() => scrollTo('sec-memo')}>Memori</button>
        <button onClick={() => scrollTo('sec-ikatan')}>PIC aktif</button>
        <button onClick={() => scrollTo('sec-tl')}>Timeline</button>
      </div>

      {d.next && d.next.status !== 'expired' ? (
        <div className="card next" id="sec-next">
          <div className="card-h"><h2>Langkah berikutnya</h2><span className="ai" style={{ marginLeft: 6 }}>disarankan {d.next.agent}</span><span className="meta">{d.next.status === 'proposed' ? 'Tenggat: ' + (d.next.due_label || '—') : d.next.status === 'rejected' ? 'Ditolak' : 'Dijalankan · ' + hhmm(d.next.executed_at ?? d.next.decided_at ?? '')}</span></div>
          <div className="next-row">
            <span className="ni"><Icon name={d.next.icon || 'spark'} /></span>
            <div><b>{d.next.title}</b><span>{d.next.why}</span></div>
            <div className="btns"><ActBtn next={d.next} /></div>
          </div>
        </div>
      ) : (
        <div className="card" id="sec-next">
          <div className="card-h"><h2>Langkah berikutnya</h2></div>
          <p style={{ fontSize: 13.5, color: 'var(--text-2)' }}>
            {issue
              ? `Belum ada saran hari ini untuk dealer ini. Analisis ulang dealer ini: agen menyusun langkah — tagih, follow-up, atau perluas product mix — dengan alasan dan sumbernya.`
              : `Tidak ada yang mendesak: siklus order terjaga, sisa limit ${cr.state}. AI Follow-up follow-up otomatis 1 hari sebelum jadwal order dengan rekomendasi order.`}
          </p>
        </div>
      )}

      <div className="grid-2">
        <div className="card" id="sec-siklus">
          <div className="card-h"><h2>Order-to-cash</h2><span className="meta">Order → Siap → Kirim → Invoice → Bayar → order lagi</span></div>
          <div style={{ display: 'grid', gridTemplateColumns: '140px 1fr', gap: 16, alignItems: 'center' }}>
            <Siklus phase={phase} days={d.orders.cycle_days || m.rhythm_days || 0} />
            <div className="hb">
              {parts.map(([l, v]) => (
                <div className="row" key={l}><span className="lbl">{l}</span><div className="bar"><i style={{ width: v + '%', background: hcol(v) }} /></div><span className="v num">{v}</span></div>
              ))}
              <p style={{ fontSize: 11.5, color: 'var(--text-3)', marginTop: 6 }}>Skor dealer = 30% siklus order · 25% share of wallet · 15% product mix · 15% sisa limit · 15% PIC aktif</p>
            </div>
          </div>
          <div className="hr" />
          <h3 style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)', marginBottom: 8 }}>Order 6 bulan</h3>
          <div className="aging">
            {months.map((x, i) => (
              <div className="row" key={x.month} style={{ gridTemplateColumns: '40px 1fr 70px' }}>
                <span className="lbl">{x.label}</span>
                <div className="bar"><i style={{ width: (x.total / maxM) * 100 + '%', background: i === months.length - 1 ? 'var(--accent-line)' : 'var(--accent)' }} /></div>
                <span className="v num">{x.total ? fmtRp(x.total) : '—'}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="card" id="sec-porsi">
          <div className="card-h"><h2>Share of wallet &amp; product mix</h2><span className="meta">Share of wallet {m.sow}% · {m.sow_source === 'confirmed' ? 'dikonfirmasi sales' : 'estimasi'}</span></div>
          <div className="sow"><div className="pbar"><i style={{ width: m.sow + '%' }} /></div><div className="pl"><span>GSI {m.sow}%</span><span>distributor lain {100 - m.sow}%</span></div></div>
          <div className="kr">
            {KAT.map((k, i) => (
              <div key={k} className={m.mix_cats[i] ? 'on' : ''}><b>{k}</b><small>{m.mix_cats[i] ? 'dibeli 6 bln' : 'belum pernah'}</small></div>
            ))}
          </div>
          <h3 style={{ fontSize: 11, fontWeight: 700, letterSpacing: '.06em', textTransform: 'uppercase', color: 'var(--text-3)', margin: '14px 0 8px' }}>Komposisi product mix</h3>
          <div className="hb">
            {d.composition.map((c) => (
              <div className="row" key={c.product}><span className="lbl">{c.product}</span><div className="bar"><i style={{ width: c.pct + '%', background: 'var(--indigo)' }} /></div><span className="v num">{c.pct}%</span></div>
            ))}
          </div>
          <p style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 10, lineHeight: 1.5 }}>Share of wallet naik dengan <b>meperluas product mix</b> ({empty.slice(0, 2).join(', ') || 'semua kategori terisi'}), bukan menurunkan harga.</p>
        </div>
      </div>

      <div className="card" id="sec-napas">
        <div className="card-h"><h2>Sisa limit</h2><span className="ai" style={{ marginLeft: 6 }}>AI Kredit</span><span className="meta">{cr.limit ? `Limit ${fmtRp(cr.limit)} · pola bayar ${cr.pay_days} hari · ${cr.on_time}% tepat` : 'Dealer cash'}</span></div>
        {cr.limit ? (
          <div className="aging">
            <div className="row" style={{ gridTemplateColumns: '90px 1fr 120px' }}>
              <span className="lbl">Exposure</span>
              <div className="bar"><i style={{ width: Math.min(100, Math.round((cr.exposure / cr.limit) * 100)) + '%', background: `var(--${tone})` }} /></div>
              <span className="v num">{Math.round((cr.exposure / cr.limit) * 100)}%<small>{fmtRp(cr.exposure)} / {fmtRp(cr.limit)}</small></span>
            </div>
          </div>
        ) : (
          <p style={{ fontSize: 13, color: 'var(--text-2)' }}>Tanpa limit kredit — order diproses saat pembayaran masuk.</p>
        )}
        <div className="flags" style={{ marginTop: 12 }}>
          {late.map((c) => (
            <div className="flag" key={c.title}>
              <span className="fi" style={{ background: `var(--${c.status === 'late' ? 'bad' : 'warn'}-soft)`, color: `var(--${c.status === 'late' ? 'bad' : 'warn'})` }}><Icon name="cash" /></span>
              <div><b>{c.title}</b><span>{c.detail}</span></div>
              {c.status === 'late' ? <span className="pill bad">Lewat {c.late_days} hari</span> : <span className="pill neutral">Terbuka</span>}
            </div>
          ))}
          {d.flags.includes('credit_blocked') && (
            <div className="flag"><span className="fi" style={{ background: 'var(--bad-soft)', color: 'var(--bad)' }}><Icon name="flag" /></span><div><b>Over limit / overdue</b><span>Order berikutnya tertahan sampai pembayaran masuk atau approve CEO</span></div></div>
          )}
          {d.flags.includes('limit_up_candidate') && (
            <div className="flag"><span className="fi" style={{ background: 'var(--good-soft)', color: 'var(--good)' }}><Icon name="check" /></span><div><b>Layak kenaikan limit</b><span>{cr.on_time}% tepat waktu · sisa limit sering tipis karena order tumbuh</span></div></div>
          )}
        </div>
      </div>

      <div className="card" id="sec-memo">
        <div className="card-h"><h2>Memori dealer</h2><span className="ai" style={{ marginLeft: 6 }}>brief otomatis</span><span className="meta">diperbarui {d.memo_updated_at ? shortDate(d.memo_updated_at) : '—'}</span></div>
        <MemoText d={d} />
        <div className="prov-row">
          <Prov icon="chat">WhatsApp</Prov><Prov icon="doc">SO &amp; invoice Odoo</Prov><Prov icon="box">pembayaran</Prov>
          <Prov style={{ marginLeft: 'auto' }}>{(d.memo_signals ?? []).length} sumber · semua klaim bisa dilacak ke sumber</Prov>
        </div>
      </div>

      <div className="grid-2">
        <div className="card" id="sec-ikatan">
          <div className="card-h"><h2>PIC aktif</h2><span className="meta">{active} kontak aktif</span></div>
          <ul style={{ display: 'flex', flexDirection: 'column' }}>
            {d.contacts.map((p) => (
              <li key={p.name} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 0', borderTop: '1px solid var(--line)', fontSize: 13 }}>
                <span className="avatar" style={{ width: 28, height: 28, fontSize: 10, background: 'var(--surface-3)', color: 'var(--text-2)' }}>{contactIni(p.name)}</span>
                <div><b style={{ fontWeight: 600 }}>{p.name}</b><span style={{ color: 'var(--text-3)' }}> · {p.role}</span></div>
                <span className={`pill ${p.level === 'utama' ? 'good' : p.level === 'aktif' ? 'neutral' : 'warn'}`} style={{ marginLeft: 'auto' }}>{p.level === 'utama' ? 'kontak utama' : p.level === 'aktif' ? 'aktif' : p.level === 'jarang' ? 'jarang' : 'belum kontak'}</span>
              </li>
            ))}
          </ul>
          {active <= 1 && (
            <div className="stage-line" style={{ marginTop: 10 }}>
              <span className="pill warn"><Icon name="alert" />Hanya 1 PIC</span>
              <span className="why">Satu orang pergi, dealer ikut pergi. Minta nomor admin/kasir saat kirim berikutnya.</span>
            </div>
          )}
        </div>
        <div className="card" id="sec-commit">
          <div className="card-h"><h2>Komitmen</h2><span className="meta">Dua arah</span></div>
          <div className="ledger" style={{ gridTemplateColumns: '1fr' }}>
            <div><h3>Kami</h3><ul>{d.commitments.kami.length ? d.commitments.kami.map((c) => <LedgerRow key={c.title} c={c} />) : <li><span /><div className="t" style={{ color: 'var(--text-3)' }}>Tidak ada</div></li>}</ul></div>
            <div><h3>Mereka</h3><ul>{d.commitments.mereka.length ? d.commitments.mereka.map((c) => <LedgerRow key={c.title} c={c} />) : <li><span /><div className="t" style={{ color: 'var(--text-3)' }}>Tidak ada</div></li>}</ul></div>
          </div>
        </div>
      </div>

      <div className="card" id="sec-tl">
        <div className="card-h"><h2>Timeline</h2><span className="meta">Interaksi + kesimpulan agen</span></div>
        <ul className="tl">
          {d.timeline.map((t) => (
            <li key={t.signal_id}>
              <span className="n"><Icon name={VIA_ICON[t.via] ?? 'chat'} /></span>
              <div className="d"><span>{shortDate(t.at)}</span><span>{t.who}</span></div>
              <div className="t">{t.text}</div>
              {t.conclusion && <div className="x"><span className="ai" />{t.conclusion}</div>}
            </li>
          ))}
        </ul>
      </div>
    </>
  )
}

export function DealerPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [params] = useSearchParams()
  const status = params.get('status') ?? ''
  const { data: all } = useDealers('')
  const list = status ? all?.filter((d) => d.metrics.status === status) : all
  const fallback: BoardItem | undefined = list?.find((d) => d.id === 'mitra') ?? list?.[0]
  const current = id ?? fallback?.id
  const search = params.toString()
  useEffect(() => {
    // keep the list filter (?status=Prospek) when opening the first dealer
    if (!id && fallback) nav('/dealer/' + fallback.id + (search ? '?' + search : ''), { replace: true })
  }, [id, fallback, nav, search])
  const { data: d } = useDealer(current)
  return (
    <div className="rel">
      <DealerList active={current} />
      <div className="stack" id="acc-page">{d && <DealerBody d={d} />}</div>
    </div>
  )
}
