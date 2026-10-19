import { useNavigate } from 'react-router'
import type { AgingItem, NextAction, Plan, PlanStep, Proposal, WAAnalysis, WAInsight } from '../../api/types'
import { useStockProposals } from '../../app/queries'
import { ActBtn, ProposalSheet, useDecide } from '../../components/actions'
import { ColumnTable, type Column } from '../../components/DataTable'
import { useFeedback } from '../../components/feedback'
import { Icon } from '../../components/Icon'
import { Pill } from '../../components/ui'
import { fmtRp, shortName } from '../../lib/format'
import { byWait } from '../wa/WACards'
import { waitLabel, who } from '../wa/wa'
import { PlanAct, PlanText, planStatus } from './Plan'

// Every list card of Pusat kendali as a data table — search, sort asc/desc per column, pages of 10/25/50 (Sam 19 Oct).

const txt = (a: string | null | undefined, b: string | null | undefined) => (a ?? '').localeCompare(b ?? '', 'id')
const strip = (html: string | null) => (html ?? '').replace(/\[\[dealer:[^|\]]+\|([^\]]+)\]\]/g, '$1').replace(/<[^>]+>/g, '')

/** Rencana hari ini. */
export function PlanTable({ plan }: { plan: Plan | undefined }) {
  const cols: Column<PlanStep>[] = [
    { key: 'jam', label: 'Jam', cell: (it) => <span className="num">{it.time_label ?? '—'}</span>, sort: (a, b) => txt(a.time_label, b.time_label) || a.seq - b.seq, className: 'nowrap' },
    { key: 'langkah', label: 'Langkah', cell: (it) => <PlanText text={it.text_html ?? ''} />, sort: (a, b) => txt(strip(a.text_html), strip(b.text_html)) },
    { key: 'agen', label: 'Agen', cell: (it) => <span className="ai">{it.agent}</span>, sort: (a, b) => txt(a.agent, b.agent), className: 'nowrap' },
    { key: 'status', label: 'Status', cell: (it) => {
      const st = planStatus(it)
      return <span className="cc-acts" style={{ justifyContent: 'flex-start' }}><span className={`pill ${it.autonomy === 'auto' ? 'good' : 'warn'}`}>{it.autonomy === 'auto' ? 'otonom' : 'butuh approve'}</span><span className={`pill ${st.tone}`}>{st.text}</span></span>
    }, sort: (a, b) => txt(a.autonomy, b.autonomy) || txt(a.status, b.status) },
    { key: 'aksi', label: '', cell: (it) => <span className="cc-acts"><PlanAct it={it} /></span> },
  ]
  return (
    <ColumnTable rows={plan?.items ?? []} columns={cols} rowKey={(it) => it.id} noun="langkah" placeholder="Cari langkah, dealer, agen…"
      text={(it) => `${it.time_label ?? ''} ${strip(it.text_html)} ${it.agent ?? ''} ${planStatus(it).text} ${it.autonomy === 'auto' ? 'otonom' : 'butuh approve'}`}
      empty="Belum ada rencana. Orchestrator menyusun Rencana hari ini dari usulan atas chat WhatsApp, diurutkan per jam." />
  )
}

/** Agenda sales: per sales, the conversations still waiting for their reply. */
export function AgendaTable({ data }: { data: WAAnalysis | undefined }) {
  const nav = useNavigate()
  const rows = byWait((data?.items ?? []).filter((r) => !!r.waiting_since))
  const cols: Column<WAInsight>[] = [
    { key: 'sales', label: 'Sales', cell: (r) => <b>{r.sales || 'Tanpa sales'}</b>, sort: (a, b) => txt(a.sales, b.sales) || b.wait_min - a.wait_min, className: 'nowrap' },
    { key: 'chat', label: 'Chat', cell: (r) => <>{r.dealer_id ? <button className="ev" onClick={() => nav('/dealer/' + r.dealer_id)}>{who(r)}</button> : <b>{who(r)}</b>}{r.kind === 'new' && <> · <Pill tone="indigo">nomor baru</Pill></>}</>, sort: (a, b) => txt(who(a), who(b)) },
    { key: 'menunggu', label: 'Menunggu', cell: (r) => <Pill tone={r.needs_reply ? 'bad' : 'warn'}>{waitLabel(r.wait_min)}</Pill>, sort: (a, b) => a.wait_min - b.wait_min, first: 'desc', right: true },
    { key: 'aksi', label: '', cell: (r) => <span className="cc-acts"><button className="btn ghost" onClick={() => nav('/chat/' + r.thread_id)}>Buka chat</button></span> },
  ]
  return (
    <ColumnTable rows={rows} columns={cols} rowKey={(r) => r.thread_id} noun="chat" placeholder="Cari sales atau dealer…" initial={{ column: 'sales', dir: 'asc' }}
      text={(r) => `${r.sales} ${who(r)} ${r.kind === 'new' ? 'nomor baru' : ''}`} empty="Tidak ada chat yang menunggu balasan." />
  )
}

/** Chats from WhatsApp (Jadwal order, Lewat jadwal, Limit tipis). */
export function WAChatTable({ items, empty, noun = 'chat' }: { items: WAInsight[]; empty: string; noun?: string }) {
  const nav = useNavigate()
  const cols: Column<WAInsight>[] = [
    { key: 'dealer', label: 'Dealer / kontak', cell: (r) => (
      <>
        {r.dealer_id ? <button className="ev" onClick={() => nav('/dealer/' + r.dealer_id)}>{who(r)}</button> : <b>{who(r)}</b>}
        {r.kind === 'new' && <> · <Pill tone="indigo">nomor baru</Pill></>}
        <span className="cc-sub">{r.sales && <b>{r.sales} · </b>}{r.summary ? <><span className="ai" title="ditulis Claude lewat MCP" />{r.summary}</> : r.last_text ? <q>{r.last_text}</q> : 'hanya pesan sales'}{r.proposal_ids.length > 0 && ` · ${r.proposal_ids.length} usulan`}</span>
      </>
    ), sort: (a, b) => txt(who(a), who(b)) },
    { key: 'menunggu', label: 'Status', cell: (r) => (r.waiting_since ? <Pill tone={r.needs_reply ? 'bad' : 'warn'}>{waitLabel(r.wait_min)}</Pill> : r.answered ? <Pill tone="good" icon="check">dibalas</Pill> : <Pill tone="neutral">—</Pill>),
      sort: (a, b) => (a.waiting_since ? a.wait_min : -1) - (b.waiting_since ? b.wait_min : -1), first: 'desc', right: true },
    { key: 'aksi', label: '', cell: (r) => <span className="cc-acts"><ChatBtn id={r.thread_id} /></span> },
  ]
  return (
    <ColumnTable rows={items} columns={cols} rowKey={(r) => r.thread_id} noun={noun} placeholder="Cari dealer, sales, pesan…" initial={{ column: 'menunggu', dir: 'desc' }}
      text={(r) => `${who(r)} ${r.sales} ${r.summary} ${r.last_text} ${r.kind === 'new' ? 'nomor baru' : ''}`} empty={empty} />
  )
}

const QUEUE_STATUS: Record<string, [string, string]> = {
  proposed: ['warn', 'menunggu Anda'], approved: ['good', 'disetujui'], edited: ['good', 'diedit & disetujui'], executed: ['good', 'dijalankan'],
  rejected: ['bad', 'ditolak'], expired: ['neutral', 'kedaluwarsa'], suppressed: ['neutral', 'ditahan'],
}

/** Keputusan: the proposals with their quick decision; Detail opens the full proposal (impact, options, reject). */
export function QueueTable({ items }: { items: Proposal[] }) {
  const decide = useDecide()
  const { openSheet } = useFeedback()
  const cols: Column<Proposal>[] = [
    { key: 'usulan', label: 'Usulan', cell: (q) => (
      <>
        <b>{q.title}</b>{q.pills.map(([k, t]) => <span key={t} className={`pill ${k}`} style={{ marginLeft: 6 }}>{t}</span>)}
        {(q.summary || q.why) && <span className="cc-sub">{q.summary || q.why}</span>}
      </>
    ), sort: (a, b) => txt(a.title, b.title) },
    { key: 'agen', label: 'Agen', cell: (q) => <span className="ai">{q.agent}</span>, sort: (a, b) => txt(a.agent, b.agent), className: 'nowrap' },
    { key: 'status', label: 'Status', cell: (q) => { const [k, t] = QUEUE_STATUS[q.status] ?? ['neutral', q.status]; return <span className={`pill ${k}`}>{t}</span> },
      sort: (a, b) => Number(b.status === 'proposed') - Number(a.status === 'proposed') || txt(a.status, b.status), className: 'nowrap' },
    { key: 'waktu', label: 'Masuk', cell: (q) => <span className="num">{new Date(q.created_at).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Jakarta' })}</span>,
      sort: (a, b) => Date.parse(a.created_at) - Date.parse(b.created_at), first: 'desc', className: 'nowrap' },
    { key: 'aksi', label: '', cell: (q) => {
      const quick = q.options[0]
      return (
        <span className="cc-acts">
          {q.status === 'proposed' && quick && <button className="btn primary" disabled={decide.isPending} onClick={() => quick.key === 'reject' ? openSheet(<ProposalSheet id={q.id} />) : decide.mutate({ id: q.id, body: quick.key === 'approve' ? { decision: 'approve' } : { decision: 'option', option: quick.key } })}>{q.button ?? quick.label}</button>}
          <button className="btn quiet" onClick={() => openSheet(<ProposalSheet id={q.id} />)}>Detail</button>
        </span>
      )
    } },
  ]
  return (
    <ColumnTable rows={items} columns={cols} rowKey={(q) => q.id} noun="usulan" placeholder="Cari usulan, dealer, agen…" initial={{ column: 'status', dir: 'asc' }}
      text={(q) => `${q.title} ${q.dealer_name ?? ''} ${q.agent} ${q.summary ?? ''} ${(QUEUE_STATUS[q.status] ?? ['', q.status])[1]}`}
      empty="Tidak ada keputusan hari ini. Usulan Claude dari chat WhatsApp — rilis kredit, harga, retur, limit — muncul di sini." />
  )
}

/** Push stok: aged stock (BigQuery) → dealers whose product mix fits. */
export function PushTable({ items }: { items: AgingItem[] }) {
  const { data: props = [] } = useStockProposals()
  const bundleOf = (name: string): NextAction | null => {
    const p = props.find((x) => x.kind === 'push_stock' && x.payload?.name === name && !x.payload?.parent)
    return p ? { id: p.id, kind: p.kind, title: p.title, button: p.button ?? 'Buat bundle', icon: p.icon ?? 'box', agent: p.agent, due_label: p.due_label ?? '', status: p.status, why: p.why, decided_at: p.decided_at, executed_at: p.executed_at, autonomy: p.autonomy } : null
  }
  const count = (x: AgingItem) => x.candidate_count ?? (x.candidates ?? []).length
  const cols: Column<AgingItem>[] = [
    { key: 'produk', label: 'Produk', cell: (x) => {
      const c = x.candidates ?? []
      const n = count(x)
      return <><b>{x.name}</b><span className="cc-sub">{n ? `→ ${n} dealer: ${c.slice(0, 3).map((k) => shortName(k.name)).join(', ')}${n > Math.min(3, c.length) ? `, +${n - Math.min(3, c.length)}` : ''}` : 'belum ada dealer yang cocok'}{x.due_this_week ? ` · ${x.due_this_week} jadwal order minggu ini` : ''}</span>{n > 0 && <span className="cc-sub" style={{ marginTop: 6 }}><ActBtn small next={bundleOf(x.name)} label="Buat bundle" icon="box" /></span>}</>
    }, sort: (a, b) => txt(a.name, b.name) },
    { key: 'umur', label: 'Umur', cell: (x) => `${x.age_days} hr`, sort: (a, b) => a.age_days - b.age_days, first: 'desc', right: true },
    { key: 'nilai', label: 'Nilai', cell: (x) => <span className="num">{fmtRp(x.value)}</span>, sort: (a, b) => a.value - b.value, first: 'desc', right: true },
  ]
  return (
    <ColumnTable rows={items} columns={cols} rowKey={(x) => x.id} noun="produk" placeholder="Cari produk, kategori, dealer…" initial={{ column: 'nilai', dir: 'desc' }}
      text={(x) => `${x.name} ${x.category ?? ''} ${x.branch ?? ''} ${(x.candidates ?? []).map((k) => k.name).join(' ')}`}
      empty="Tidak ada stok menua dengan dealer yang cocok." />
  )
}

/** "Buka chat" as an icon button: the narrow cards keep their columns. */
function ChatBtn({ id }: { id: string }) {
  const nav = useNavigate()
  return <button className="btn ghost cc-ico" title="Buka chat" aria-label="Buka chat" onClick={() => nav('/chat/' + id)}><Icon name="chat" /></button>
}
