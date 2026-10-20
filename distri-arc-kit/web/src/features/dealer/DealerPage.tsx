import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import type { BoardItem, Commitment, DealerDetail } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn } from '../../components/actions'
import { Pill, Prov, ScoreRing } from '../../components/ui'
import { contactIni, fmtRp, hb, hcol, hhmm, shortDate, sowText } from '../../lib/format'
import { KAT, KUAD } from '../../lib/i18n/id'
import { useOrch, useOrchStatus } from '../../app/orch'
import { MemoText } from './Memo'
import { SOWSheet } from './SOWSheet'
import { useFeedback } from '../../components/feedback'
import { useDealer, useDealerAnalysis, useDealerThreads, useDealers, useMe } from '../../app/queries'
import { creditTone } from '../control/lists'

const VIA_ICON: Record<string, string> = { mail: 'mail', chat: 'chat', people: 'people', doc: 'doc', form: 'form', box: 'box', phone: 'phone' }
const DEALERS_PER_PAGE = 10

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// Klasifikasi dealer (Sam 20 Oct): how many dealers are active and in which status (glossary › Status dealer), each a
// filter. "Aktif" counts Key account + Aktif + Baru, out of the dealers that ever ordered; Prospek is counted apart.
const CLASSES: { s: string; label: string; tone: string; hint: string }[] = [
  { s: 'Key account', label: 'Key account', tone: 'good', hint: 'share of wallet ≥ 50%, bayar tepat ≥ 85%, omzet besar' },
  { s: 'Aktif', label: 'Aktif', tone: 'accent', hint: 'order sesuai siklus' },
  { s: 'Baru', label: 'Baru', tone: 'indigo', hint: 'baru 1 order dalam 90 hari' },
  { s: 'At risk', label: 'At risk', tone: 'warn', hint: 'lewat 1,2× siklus order' },
  { s: 'Churn', label: 'Churn', tone: 'bad', hint: 'lewat 2× siklus, atau > 365 hari tanpa order' },
]
const ACTIVE = new Set(['Key account', 'Aktif', 'Baru'])
const pct = (n: number, of: number) => (of ? Math.round((n / of) * 100) : 0)

function DealerClasses({ all, status, onPick }: { all: BoardItem[]; status: string; onPick: (s: string) => void }) {
  const n: Record<string, number> = {}
  for (const d of all) n[d.metrics.status] = (n[d.metrics.status] ?? 0) + 1
  const prospek = n.Prospek ?? 0
  const ordered = all.length - prospek
  const active = CLASSES.filter((c) => ACTIVE.has(c.s)).reduce((t, c) => t + (n[c.s] ?? 0), 0)
  const pick = (s: string) => onPick(status === s ? '' : s)
  return (
    <div className="dl-cls" aria-label="Klasifikasi dealer">
      <button type="button" className={`dl-cls-head${status === 'aktif' ? ' on' : ''}`} onClick={() => pick('aktif')} title="Tampilkan dealer aktif (Key account + Aktif + Baru)">
        <span>Dealer aktif</span><b className="num">{pct(active, ordered)}%</b><small>{active.toLocaleString('id-ID')} dari {ordered.toLocaleString('id-ID')} yang pernah order</small>
      </button>
      <div className="dl-cls-bar" aria-hidden="true">
        {CLASSES.map((c) => <i key={c.s} style={{ width: `${pct(n[c.s] ?? 0, ordered)}%`, background: `var(--${c.tone})` }} />)}
      </div>
      <div className="dl-cls-chips" role="group" aria-label="Filter status dealer">
        {CLASSES.map((c) => (
          <button key={c.s} type="button" className={status === c.s ? 'on' : ''} aria-pressed={status === c.s} onClick={() => pick(c.s)} title={c.hint}>
            <i style={{ background: `var(--${c.tone})` }} />{c.label}<b className="num">{pct(n[c.s] ?? 0, ordered)}%</b><small className="num">{(n[c.s] ?? 0).toLocaleString('id-ID')}</small>
          </button>
        ))}
        <button type="button" className={status === 'Prospek' ? 'on' : ''} aria-pressed={status === 'Prospek'} onClick={() => pick('Prospek')} title="Belum pernah order — tidak dihitung dalam persentase">
          <i style={{ background: 'var(--text-3)' }} />Prospek<small className="num">{prospek.toLocaleString('id-ID')}</small>
        </button>
      </div>
    </div>
  )
}

/** The status filter: one status, or "aktif" for Key account + Aktif + Baru. */
export function inStatus(d: BoardItem, status: string) {
  return !status || (status === 'aktif' ? ACTIVE.has(d.metrics.status) : d.metrics.status === status)
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
  const list = useMemo(() => all.filter((d) => inStatus(d, status)), [all, status])
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
      {!isPending && !isError && <DealerClasses all={all} status={status} onPick={(s) => { setParams(s ? { status: s } : {}); setPage(1) }} />}
      {status && <span className="st-count">{list.length.toLocaleString('id-ID')} dealer {status === 'aktif' ? 'aktif (Key account + Aktif + Baru)' : status}</span>}
      <button className="btn quiet" style={{ height: 28, fontSize: 12, margin: '8px 0 4px', alignSelf: 'flex-start' }} onClick={() => openSheet(<SOWSheet />)}><Icon name="check" />Konfirmasi share of wallet</button>
      <div className="list">
        {shown.map((d) => {
          const m = d.metrics
          return (
            <button key={d.id} className={`acc-item ${d.id === active ? 'is-active' : ''}`} onClick={() => nav('/dealer/' + d.id + (status ? '?status=' + encodeURIComponent(status) : ''))}>
              <span className={`dot ${hb(m.score)}`} />
              <div>
                <b>{d.name}</b>
                <span>{m.status} · {d.customer_type === 'si' ? 'SI' : 'Dealer'} · {KUAD[m.segment].n} · {d.city}{d.tier ? ` · tier ${d.tier}` : ''}</span>
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

/** WhatsApp CTA (Sam 17 Oct): opens the dealer's conversation in Chat — the most recent one, or a choice when several
 * PIC chat; replies still go through Chat's outbox and anti-ban guard. With a draft (Analisa Dealer), the reply box
 * opens with it filled in — the sales reads, edits and presses Kirim. The list comes from /dealers/{id}/threads, not
 * from loading every chat. */
function WhatsAppCTA({ dealerId, draft, label = 'WhatsApp', className = '' }: { dealerId: string; draft?: string; label?: string; className?: string }) {
  const nav = useNavigate()
  const { data: me } = useMe()
  const canChat = !!me?.screens.includes('chat')
  const { data: mine = [] } = useDealerThreads(dealerId, canChat)
  const [open, setOpen] = useState(false)
  if (!canChat) return null
  if (mine.length === 0) return <button className={`btn ghost dl-wa ${className}`} disabled title="Belum ada chat WhatsApp dengan dealer ini"><Icon name="chat" />{label}</button>
  const go = (id: string) => nav('/chat/' + id + (draft ? '?draft=' + encodeURIComponent(draft) : ''))
  return (
    <span className="dl-wa-w">
      <button className={`btn dl-wa ${className}`} onClick={() => (mine.length === 1 ? go(mine[0].id) : setOpen(!open))} title={mine.length === 1 ? `Buka chat ${mine[0].title}` : `${mine.length} chat dengan dealer ini`}>
        <Icon name="chat" />{label}{mine.length > 1 && <small>{mine.length}</small>}
      </button>
      {open && (
        <span className="dl-wa-menu" role="menu">
          {mine.map((t) => (
            <button key={t.id} role="menuitem" onClick={() => go(t.id)}><b>{t.title}</b><small>{t.last_message_at ? `${shortDate(t.last_message_at)} · ${hhmm(t.last_message_at)}` : '—'}</small></button>
          ))}
        </span>
      )}
    </span>
  )
}

const HEALTH: Record<string, { tone: string; label: string; cta: string; icon: string }> = {
  bermasalah: { tone: 'bad', label: 'Bermasalah', cta: 'Chat: selesaikan masalah', icon: 'alert' },
  perlu_perhatian: { tone: 'warn', label: 'Perlu perhatian', cta: 'Chat: sapa & tawarkan', icon: 'refresh' },
  aman: { tone: 'good', label: 'Aman', cta: 'Chat: sapa dealer', icon: 'check' },
}

/** Analisa Dealer (ADR 0055): what Claude read in this dealer's numbers and WhatsApp — a status, the reasons, and a
 * WhatsApp draft opened in Chat by the button (the sales sends it). */
function DealerAI({ d, onAnalyse, running }: { d: DealerDetail; onAnalyse: () => void; running: boolean }) {
  const { data: a } = useDealerAnalysis(d.id)
  if (!a) {
    return (
      <div className="card dai empty">
        <div className="card-h"><h2>Analisa AI</h2><span className="ai" style={{ marginLeft: 6 }}>Claude · angka &amp; chat WhatsApp</span></div>
        <p className="sg-hint">Belum dianalisa Claude. Tekan <b>Analisa AI</b>: Claude membaca siklus order, limit, invoice, dan chat WhatsApp dealer ini, lalu menilai apakah bermasalah dan menyiapkan draf pesan WhatsApp.</p>
        <button className="btn primary" disabled={running} onClick={onAnalyse}><Icon name="spark" />{running ? 'AI sedang menganalisa…' : 'Analisa AI'}</button>
      </div>
    )
  }
  const h = HEALTH[a.status] ?? HEALTH.aman
  return (
    <div className={`card dai dai-${h.tone}`}>
      <div className="card-h">
        <h2>Analisa AI</h2><span className="ai" style={{ marginLeft: 6 }}>Claude · angka &amp; chat WhatsApp</span>
        <span className="meta">{shortDate(a.generated_at)} · {hhmm(a.generated_at)}{a.cycle_number ? ` · siklus #${a.cycle_number.toLocaleString('id-ID')}` : ''}</span>
      </div>
      <div className="dai-row">
        <span className={`dai-status ${h.tone}`}><Icon name={h.icon} />{h.label}</span>
        <p className="dai-ov">{a.overview}</p>
      </div>
      {a.reasons.length > 0 && <ul className="dai-why">{a.reasons.map((r) => <li key={r}>{r}</li>)}</ul>}
      {a.wa_draft && (
        <div className="dai-wa">
          <small>Draf WhatsApp dari Claude</small>
          <q>{a.wa_draft}</q>
          <div className="dai-acts">
            <WhatsAppCTA dealerId={d.id} draft={a.wa_draft} label={h.cta} className={`dai-cta ${h.tone}`} />
            <button className="btn quiet" disabled={running} onClick={onAnalyse}><Icon name="refresh" />Analisa ulang</button>
          </div>
          <span className="dai-note">Draf terisi di kolom ketik Chat — Anda yang membaca, mengubah, dan menekan Kirim.</span>
        </div>
      )}
    </div>
  )
}

function DealerBody({ d }: { d: DealerDetail }) {
  const { reanalyze } = useOrch()
  const orch = useOrchStatus()
  // Analisa AI: Claude through MCP (the routine, ADR 0049/0055) reads this dealer; the progress is the pop-up
  const analyse = () => reanalyze('dealer:' + d.id, 'mcp', 'Claude lewat MCP')
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
            <span className="dl-cta">
              <WhatsAppCTA dealerId={d.id} />
              <button className="btn primary dl-ai" disabled={orch.running} onClick={analyse}
                title="Claude (MCP, lewat routine) menganalisa dealer ini dari angka dan chat WhatsApp-nya: status, alasan, dan draf pesan WhatsApp"><Icon name="spark" />{orch.running ? 'AI sedang menganalisa…' : 'Analisa AI'}</button>
            </span>
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
          <div className="kpi"><small>Share of wallet · product mix</small><b className="num">{sowText(m)} · {m.mix}/6</b><span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>share of wallet · kategori dibeli</span></div>
          <div className="kpi">
            <small>Sisa limit</small>
            <b className="num" style={{ color: `var(--${tone === 'neutral' ? 'text' : tone})` }}>{room == null ? 'cash' : room < 0 ? 'over limit' : Math.round(room * 100) + '%'}</b>
            <span style={{ fontSize: 11.5, color: 'var(--text-3)' }}>
              {room == null ? 'tanpa kredit' : (cr.exposure > cr.limit ? 'over limit · lewat limit ' + fmtRp(cr.exposure - cr.limit) : (cr.state === 'overdue' ? 'overdue · invoice lewat tempo' : cr.state) + ' · ' + fmtRp(cr.limit - cr.exposure) + ' tersisa') + ' · bayar ' + cr.pay_days + ' hr'}
            </span>
          </div>
        </div>
      </div>

      <DealerAI d={d} onAnalyse={analyse} running={orch.running} />

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
          <div className="card-h"><h2>Share of wallet &amp; product mix</h2><span className="meta">{m.sow_source === 'default' ? 'Share of wallet belum diketahui · riwayat < 6 bulan, minta sales konfirmasi' : `Share of wallet ${m.sow}% · ${m.sow_source === 'confirmed' ? 'dikonfirmasi sales' : m.sow_source === 'history' ? 'estimasi dari riwayat belanja (3 bln terakhir ÷ 3 bln terbaik)' : 'estimasi'}`}</span></div>
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
        <div className="card-h"><h2>Memori dealer</h2><span className="meta" style={{ marginLeft: 6 }}>dirangkum sistem dari sinyal</span><span className="meta">diperbarui {d.memo_updated_at ? shortDate(d.memo_updated_at) : '—'}</span></div>
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
  const list = all?.filter((d) => inStatus(d, status))
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
