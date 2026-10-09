import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MessageView, ThreadDetail, ThreadView } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn, ProposalSheet } from '../../components/actions'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtRp, hhmm, shortDate, wib } from '../../lib/format'
import { useChatContext, useNow, useThread, useThreads, useWAStatus } from '../../app/queries'
import { useOrch } from '../../app/orch'
import { ConnectPanel, holderOf, labelOf, NumberRail, NumberSheet, phoneOf } from './Connect'
import { countFilters, FILTERS, filterOf, fmtPhone, lookupLinks, matchThread, waitLabel, type Filter } from './inbox'


// Proposal buttons on annotations get their real labels from AI proposals (stage 05); until then the label
// comes from the mockup's action of the same id.
const ACT_LABEL: Record<string, [string, string]> = {
  so_sinar: ['Buat SO', 'check'], credit_graha: ['Setujui DP 50%', 'shield'], price_indo: ['Setujui counter', 'cash'],
  reorder_mitra: ['Kirim', 'chat'], transfer_cam: ['Usulkan transfer', 'refresh'],
}

function initials(t: ThreadView | ThreadDetail['thread']) {
  if (t.kind === 'group') return '#'
  if (t.kind === 'new' && !/[a-z]/i.test(t.title)) return '?'
  return t.title.replace(/^(Pak|Bu|Mbak|Mas)\s+/, '').split(/\s+/).filter((w) => /^[\p{L}\d]/u.test(w)).map((w) => w[0]).join('').slice(0, 2).toUpperCase() || '#'
}

function dayKey(d: string) {
  const p = wib(d)
  return `${p.y}-${p.m}-${p.d}`
}

function dayLabel(d: string, now: Date) {
  const k = dayKey(d)
  if (k === dayKey(now.toISOString())) return 'Hari ini'
  if (k === dayKey(new Date(now.getTime() - 86400000).toISOString())) return 'Kemarin'
  return shortDate(d)
}

function timeLabel(d: string | null, now: Date) {
  if (!d) return ''
  const l = dayLabel(d, now)
  return l === 'Hari ini' ? hhmm(d) : l
}

function Clock() {
  return <svg className="i" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="2" /><path d="M12 7v5l3 2" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
}

/** "21 jam 44 mnt" since the customer's message that nobody answered yet. */
function Waiting({ since, now }: { since?: string | null; now: Date }) {
  const w = waitLabel(since, now)
  if (!w) return null
  return <span className={`ibw ${w.tone}`} title="Menunggu balasan sejak pesan pelanggan yang belum dijawab"><Clock />{w.text}</span>
}

function ThreadList({ list, loaded, filter, setFilter, account, active, onPick }: {
  list: ThreadView[]; loaded: boolean; filter: Filter; setFilter: (f: Filter) => void; account: string; active?: string; onPick: (id: string) => void
}) {
  const { data: wa } = useWAStatus()
  const now = useNow()
  const qc = useQueryClient()
  const { openSheet, toast } = useFeedback()
  const [q, setQ] = useState('')
  const n = wa?.items.find((x) => x.wa_number === account)
  const counts = useMemo(() => countFilters(list), [list])
  const shown = list.filter((t) => (filter === 'all' || filterOf(t) === filter) && matchThread(t, q))
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['chat'] })
    qc.invalidateQueries({ queryKey: ['wa'] })
    toast('Daftar chat diperbarui')
  }
  return (
    <div className="pane list-pane ibl">
      <div className="ibl-head">
        <button type="button" className="ibl-acc" onClick={() => n && openSheet(<NumberSheet wa={n.wa_number} />)} disabled={!n} title={n ? 'Detail nomor' : undefined}>
          <span className="ibl-ic"><Icon name="chat" /></span>
          <span className="ibl-who">
            <small>WHATSAPP {wa?.transport === 'cloudapi' ? 'CLOUD API' : 'BAILEYS'}</small>
            <b>{n ? `${labelOf(n)}${n.user_role || n.sales_id ? ` - ${n.user_role || 'Sales'}` : ''}` : 'Semua nomor'}</b>
            <span>{n ? phoneOf(n) : `${wa?.items.length ?? 0} nomor · ${wa?.items.filter((x) => x.state === 'connected').length ?? 0} terhubung`}</span>
          </span>
          {n && <Icon name="chev" className="i ibl-chev" />}
        </button>
        <button type="button" className="ibl-btn" onClick={refresh} title="Muat ulang" aria-label="Muat ulang"><Icon name="refresh" /></button>
      </div>
      <div className="ibl-search"><Icon name="search" /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari chat" aria-label="Cari chat" /></div>
      <div className="ibl-chips" role="tablist">
        {FILTERS.map(([k, l]) => (
          <button key={k} role="tab" aria-selected={filter === k} className={filter === k ? 'is-active' : ''} onClick={() => setFilter(k)}>{l}<span>{counts[k].toLocaleString('id-ID')}</span></button>
        ))}
      </div>
      <div className="chat-list ibl-list">
        {loaded && shown.length === 0 && <div className="chat-none">{q ? `Tidak ada chat yang cocok dengan "${q}"` : 'Belum ada percakapan di sini'}</div>}
        {shown.map((c) => (
          <button key={c.id} className={`ibr ${c.id === active ? 'is-active' : ''}`} onClick={() => onPick(c.id)}>
            <span className={`av ${c.kind === 'group' ? 'grp' : ''}`}>{c.kind === 'group' ? <Icon name="people" /> : initials(c)}</span>
            <div className="ibr-m">
              <div className="ibr-top">
                <b>{c.title}</b>
                {c.kind === 'group' && <span className="ibr-tag grp">{c.group_kind === 'external' ? 'GRUP EKS' : 'GRUP'}</span>}
                {c.tag && <span className={`ibr-tag ${c.tag.k}`}>{c.tag.t}</span>}
                <Waiting since={c.waiting_since} now={now} />
              </div>
              {c.kind !== 'group' && c.subtitle && <span className="ibr-src">≈ {c.subtitle}</span>}
              <span className="ibr-lm">{c.last_direction === 'out' ? 'Anda: ' : c.last_from ? c.last_from + ': ' : ''}{c.last_body}</span>
            </div>
            <div className="ibr-rt">
              <span>{timeLabel(c.last_message_at, now)}</span>
              {c.unread > 0 && <span className="un">{c.unread > 99 ? '99+' : c.unread}</span>}
              {!account && <span className="via">via {(c.account_label || c.sales).replace(/^Nomor\s+/, '')}</span>}
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}

function Bubble({ m, via, group }: { m: MessageView; via: string; group: boolean }) {
  const a = m.annotation
  return (
    <div className={`m ${m.direction}`}>
      {group && m.direction === 'in' && <span className="who">{m.from_name}<span className={`pill ${m.internal ? 'neutral' : 'accent'}`}>{m.internal ? 'internal' : 'eksternal'}</span></span>}
      <div className="bb">{m.body}<span className="tm">{m.direction === 'out' ? `${via}${m.status === 'sent' && m.signal_id == null ? ' · terkirim' : m.status === 'pending' ? ' · mengirim…' : m.status === 'failed' ? ' · gagal' : ''} · ` : ''}{hhmm(m.sent_at)}</span></div>
      {a && (
        <span className={`ann ${a.k}`}>
          <span className="ai" />
          {a.t}
          {m.proposal ? <> · <AnnProposal p={m.proposal} /></> : a.act && ACT_LABEL[a.act] && <> · <AnnAction act={a.act} /></>}
        </span>
      )}
    </div>
  )
}

function AnnProposal({ p }: { p: NonNullable<MessageView['proposal']> }) {
  const { openSheet } = useFeedback()
  const done = p.status === 'executed' || p.status === 'approved' || p.status === 'edited'
  return <button onClick={() => openSheet(<ProposalSheet id={p.id} />)}>{done ? 'Dijalankan ✓' : p.status === 'rejected' ? 'Ditolak' : p.button || 'Lihat'}</button>
}

function AnnAction({ act }: { act: string }) {
  const [label] = ACT_LABEL[act]
  const { toast } = useFeedback()
  return <button onClick={() => toast(`${label}: proposal disusun agen setelah Stage 05`)}>{label}</button>
}

function ThreadPane({ id, onBack, picked, lead, setLead, onAccurate }: {
  id: string; onBack: () => void; picked: boolean; lead: boolean; setLead: (v: boolean) => void; onAccurate: () => void
}) {
  const { data } = useThread(id)
  const now = useNow()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const { reanalyze, s: orch } = useOrch()
  const [text, setText] = useState('')
  const [older, setOlder] = useState<MessageView[]>([])
  const [done, setDone] = useState(false)
  const [loading, setLoading] = useState(false)
  const body = useRef<HTMLDivElement>(null)
  const from = (data?.thread.account_label || data?.thread.sales || '').replace(/^Nomor\s+/, '')
  const send = useMutation({
    mutationFn: (b: string) => api.post(`/chat/threads/${id}/messages`, { body: b }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast(`Masuk antrean kirim ke ${data?.thread.title} dari ${from} · lewat penjaga anti-blokir`)
    },
    onError: (e: Error) => toast(e.message),
  })
  useEffect(() => {
    if (body.current && older.length === 0) body.current.scrollTop = body.current.scrollHeight
  }, [data, older.length])
  useEffect(() => {
    if (picked && data && data.thread.unread > 0) api.post(`/chat/threads/${id}/read`).then(() => qc.invalidateQueries({ queryKey: ['chat', 'threads'] }))
  }, [data, id, qc, picked])
  if (!data) return <div className="pane th-pane ibc" />
  const t = data.thread
  const messages = [...older, ...data.messages]
  const more = !done && data.messages.length >= 30
  const loadOlder = () => {
    if (!messages.length) return
    setLoading(true)
    api.get<ThreadDetail>(`/chat/threads/${id}?before=${encodeURIComponent(messages[0].sent_at)}`).then((r) => {
      setOlder((o) => [...r.messages, ...o])
      if (r.messages.length < 30) setDone(true)
    }, (e: Error) => toast(e.message)).finally(() => setLoading(false))
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const v = text.trim()
    if (!v) return
    setText('')
    send.mutate(v)
  }
  const links = t.kind === 'group' ? [] : lookupLinks(t.title, t.phone, t.dealer_city)
  return (
    <div className="pane th-pane ibc">
      <div className="ibc-h">
        <button className="back" onClick={onBack} aria-label="Kembali"><Icon name="arrow" /></button>
        <span className={`av ${t.kind === 'group' ? 'grp' : ''}`}>{t.kind === 'group' ? <Icon name="people" /> : initials(t)}</span>
        <div className="ibc-who">
          <div className="ibc-name"><b>{t.title}</b>{t.subtitle && <span className="ibr-src">≈ {t.subtitle}</span>}</div>
          <div className="ibc-sub">
            {t.phone && <span className="ibc-phone">{fmtPhone(t.phone)}</span>}
            {links.map((l) => <a key={l.key} className="ibc-link" href={l.href} target="_blank" rel="noreferrer noopener">{l.label}</a>)}
            {t.kind === 'group' && <span className="ibc-phone">{from} · {t.account_masked}</span>}
          </div>
        </div>
        <div className="ibc-acts">
          <Waiting since={t.waiting_since} now={now} />
          <button className="btn ibc-ai" disabled={orch.running} onClick={() => reanalyze(t.dealer_id ? `dealer:${t.dealer_id}` : 'screen:chat')} title="Jalankan Orchestrator untuk percakapan ini"><span className="ai" style={{ fontSize: 0 }} />{orch.running ? 'Menganalisis…' : 'Analisa lanjut'}</button>
          {t.kind !== 'group' && <button className="btn ibc-acc" onClick={onAccurate}><Icon name="send" />Accurate</button>}
          {!lead && <button className="ibl-btn" onClick={() => setLead(true)} title="Tampilkan panel lead" aria-label="Tampilkan panel lead"><Icon name="doc" /></button>}
        </div>
      </div>
      <div className="th-body ibc-body" ref={body}>
        <div className="ibc-start">
          {more
            ? <button type="button" className="ibc-pill dark" disabled={loading} onClick={loadOlder}><Icon name="refresh" />{loading ? 'Memuat…' : 'Muat pesan sebelumnya'}</button>
            : <span className="ibc-pill">Awal percakapan — pesan yang lebih lama tidak tersimpan di HP</span>}
        </div>
        {messages.map((m, i) => {
          const sep = i === 0 || dayKey(messages[i - 1].sent_at) !== dayKey(m.sent_at) ? <div className="day">{dayLabel(m.sent_at, now)}</div> : null
          return (
            <div key={m.id} style={{ display: 'contents' }}>
              {sep}
              <Bubble m={m} via={from} group={t.kind === 'group'} />
            </div>
          )
        })}
      </div>
      <div className="th-c ibc-c">
        {(t.suggestions ?? []).length > 0 && (
          <div className="ibc-sug">
            <span className="ibc-sug-h"><span className="ai" style={{ fontSize: 0 }} />Rekomendasi balasan (AI) — klik untuk pakai</span>
            <div className="sug">{(t.suggestions ?? []).map((x) => <button key={x} type="button" className="chip" onClick={() => setText(x)}>{x}</button>)}</div>
          </div>
        )}
        <form onSubmit={submit}>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Tulis balasan…" autoComplete="off" aria-label="Tulis balasan" />
          <button type="submit" className="send" aria-label="Kirim" disabled={send.isPending}><Icon name="send" /></button>
        </form>
        <span className="pol"><Icon name="lock" />Dikirim dari {from} ({t.account_masked}) · jeda & "mengetik…" seperti manusia · penjaga anti-blokir</span>
      </div>
    </div>
  )
}

const LEAD_TABS = [['main', 'Main'], ['ai', 'AI'], ['src', 'Sumber'], ['accurate', 'Accurate'], ['acc', 'Akun'], ['stats', 'Statistik']] as const
type LeadTab = (typeof LEAD_TABS)[number][0]
const STATUS_TONE: Record<string, string> = { 'Key account': 'good', Aktif: 'good', Baru: 'accent', Prospek: 'accent', 'At risk': 'warn', Churn: 'bad' }

function Kv({ rows }: { rows: [string, ReactNode][] }) {
  return <dl className="ibp-kv">{rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v || '—'}</dd></div>)}</dl>
}

/** The lead panel (Komo-style): who this is, where they came from, what AI suggests, and Accurate. */
function LeadPane({ id, onClose, onAccurate }: { id: string; onClose: () => void; onAccurate: () => void }) {
  const { data: ctx } = useChatContext(id)
  const { data: th } = useThread(id)
  const { data: wa } = useWAStatus()
  const { data: list = [] } = useThreads('all')
  const qc = useQueryClient()
  const nav = useNavigate()
  const { openSheet, toast } = useFeedback()
  const [tab, setTab] = useState<LeadTab>('main')
  if (!ctx || !th) return <div className="pane ibp" />
  const t = th.thread
  const tv = list.find((x) => x.id === id)
  const d = ctx.dealer
  const idf = ctx.identification
  const group = t.kind === 'group'
  const groupLabel = tv?.group_kind === 'external' ? 'Grup eksternal' : 'Grup internal'
  const kindLabel = t.kind === 'dealer' ? 'Dealer' : group ? groupLabel : 'Lead'
  const stage = d ? d.metrics.status : t.kind === 'new' ? (idf ? 'Teridentifikasi' : 'Lead baru') : groupLabel
  const tone = d ? STATUS_TONE[d.metrics.status] ?? 'accent' : t.kind === 'new' ? 'accent' : 'neutral'
  const filled = d ? Math.max(1, Math.round((d.metrics.score / 100) * 7)) : t.kind === 'new' ? (idf ? Math.max(2, Math.round((idf.score / 100) * 7)) : 1) : 0
  const num = wa?.items.find((n) => n.wa_number === tv?.account)
  const recommendation = d?.next?.title ?? idf?.potential ?? t.suggestions?.[0] ?? null
  const warn = d
    ? d.metrics.credit.state === 'over limit' || d.metrics.credit.state === 'overdue'
      ? { h: `Kredit ${d.metrics.credit.state}`, t: `Exposure ${fmtRp(d.metrics.credit.exposure)} dari limit ${fmtRp(d.credit_limit)}${d.metrics.credit.late_days ? ` · telat ${d.metrics.credit.late_days} hari` : ''}. Order baru perlu persetujuan kredit.` }
      : d.metrics.status === 'At risk' || d.metrics.status === 'Churn'
        ? { h: `Dealer ${d.metrics.status}`, t: `Order terakhir ${d.metrics.last_order_days ?? '—'} hari lalu (ritme ${d.metrics.rhythm_days ?? '—'} hari). Prioritaskan follow-up.` }
        : null
    : t.kind === 'new'
      ? { h: 'Belum ada di master pelanggan', t: 'Nomor ini belum ditemukan di data pelanggan Accurate; bisa jadi calon dealer baru atau kontak dari dealer yang sudah ada.' }
      : null
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['chat', 'context', id] })
    qc.invalidateQueries({ queryKey: ['chat', 'thread', id] })
    toast('Panel lead diperbarui')
  }
  const extracted = (
    <ul className="ibp-ext">
      {ctx.extracted.length === 0 && <li className="muted">Belum ada yang diekstrak dari chat ini</li>}
      {ctx.extracted.map((e) => (
        <li key={e.sent_at + e.annotation.t}><Icon name={e.annotation.k === 'bad' ? 'flag' : e.annotation.k === 'warn' ? 'alert' : 'check'} /><span>{e.annotation.t}<small>{e.from_name} · {hhmm(e.sent_at)}</small></span></li>
      ))}
    </ul>
  )
  const ins = th.messages.filter((m) => m.direction === 'in').length
  return (
    <div className="pane ibp">
      <div className="ibp-dark">
        <div className="ibp-bar">
          <Icon name={t.kind === 'dealer' ? 'building' : group ? 'people' : 'user-x'} />
          <b>{kindLabel}</b><span>· Analisa AI</span>
          <span className="sp" />
          {d && <button type="button" onClick={() => nav('/dealer/' + d.id)} title="Buka halaman dealer" aria-label="Buka halaman dealer"><Icon name="arrow" /></button>}
          <button type="button" onClick={refresh} title="Muat ulang" aria-label="Muat ulang panel"><Icon name="refresh" /></button>
          <button type="button" onClick={onClose} title="Tutup panel" aria-label="Tutup panel"><Icon name="x" /></button>
        </div>
        <div className="ibp-id">
          <div><b>{d?.name ?? idf?.best_name ?? t.title}</b><span>{t.phone ? fmtPhone(t.phone) : group ? `${from(t)} · ${t.account_masked}` : t.subtitle}</span></div>
          <em className={tone}>{stage}</em>
        </div>
        {filled > 0 && <div className="ibp-prog" aria-label={`Tahap ${stage}`}>{Array.from({ length: 7 }, (_, i) => <i key={i} className={i < filled ? `on ${tone}` : ''} />)}</div>}
      </div>
      <div className="ibp-tabs" role="tablist">
        {LEAD_TABS.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'is-active' : ''} onClick={() => setTab(k)}>{l}</button>)}
      </div>
      <div className="ibp-body">
        {tab === 'main' && (
          <>
            <div className="ibp-who">
              <span className={`av ${group ? 'grp' : ''}`}>{group ? <Icon name="people" /> : initials(t)}</span>
              <div><b>{t.title}</b><span>{d ? `Dealer tier ${d.tier} · ${d.city}` : group ? groupLabel : 'Pelanggan WhatsApp'}</span></div>
            </div>
            <Kv rows={[
              ['Telepon', t.phone ? fmtPhone(t.phone) : null],
              ['Tahap', stage],
              ['Minat', t.tag?.t ?? tv?.tag?.t ?? null],
              ['Sumber', t.subtitle ? `≈ ${t.subtitle}` : null],
              ['Sales', t.sales || null],
            ]} />
            {warn && <div className="ibp-warn"><b>{warn.h}</b><span>{warn.t}</span></div>}
            {recommendation && <div className="ibp-rec"><b><span className="ai" style={{ fontSize: 0 }} />REKOMENDASI AI</b><span>{recommendation}</span></div>}
            {d?.next && <ActBtn next={d.next} />}
            {t.kind === 'new' && (
              <div className="ibp-row">
                <ActBtn next={ctx.proposals?.new_dealer} label="Buat dealer tier C" icon="building" small />
                <ActBtn next={ctx.proposals?.price_list} label="Kirim harga" icon="send" ghost small />
              </div>
            )}
            <div className="ibp-grid">
              <button type="button" onClick={() => setTab('ai')}>Analisa AI <Icon name="arrow" /></button>
              <button type="button" onClick={() => setTab('src')}>Asal lead <Icon name="arrow" /></button>
              <button type="button" onClick={() => setTab('accurate')}>Data Accurate <Icon name="arrow" /></button>
              {d && <button type="button" onClick={() => nav('/dealer/' + d.id)}>Halaman dealer <Icon name="arrow" /></button>}
            </div>
            {!group && <button type="button" className="ibp-send" onClick={onAccurate}><Icon name="send" />Kirim ke Accurate</button>}
          </>
        )}
        {tab === 'ai' && (
          <>
            <h4>Diekstrak dari chat ini <span className="ai" style={{ marginLeft: 4, fontSize: 0 }} /></h4>
            {extracted}
            {d?.next && <><h4>Langkah berikutnya</h4><div className="ibp-rec"><b>{d.next.agent}</b><span>{d.next.title}{d.next.why ? ` — ${d.next.why}` : ''}</span></div><ActBtn next={d.next} /></>}
            {(t.suggestions ?? []).length > 0 && <><h4>Rekomendasi balasan</h4><ul className="ibp-ext">{t.suggestions!.map((x) => <li key={x}><Icon name="chat" /><span>{x}</span></li>)}</ul></>}
          </>
        )}
        {tab === 'src' && (
          <>
            <Kv rows={[
              ['Masuk lewat', `${from(t)} · ${t.account_masked}`],
              ['Jenis', kindLabel],
              ['Sumber (perkiraan)', t.subtitle || null],
              ['Pesan pertama', th.messages[0] ? `${shortDate(th.messages[0].sent_at)} · ${hhmm(th.messages[0].sent_at)}` : null],
            ]} />
            {idf && (
              <>
                <h4>Identifikasi nomor</h4>
                <div className="ibp-rec plain"><b>{idf.best_name}</b><span>{idf.best_org} · skor {idf.score}</span></div>
                <ul className="ibp-ext">{idf.sources.map((x) => <li key={x.source}><Icon name={x.ok === 'true' ? 'check' : 'alert'} /><span>{x.value}<small>{x.source}</small></span></li>)}</ul>
              </>
            )}
            {!idf && t.kind === 'new' && <p className="ibp-note">AI Prospek mengidentifikasi nomor ini setelah ia mengirim pesan.</p>}
          </>
        )}
        {tab === 'accurate' && (
          <>
            {d ? (
              <Kv rows={[
                ['Kode pelanggan', t.dealer_code || null],
                ['Nama di Accurate', d.name],
                ['Kota · cabang', `${d.city}${d.branch ? ` · ${d.branch}` : ''}`],
                ['Sales', d.owner?.name ?? t.sales],
                ['Limit kredit', d.credit_limit ? fmtRp(d.credit_limit) : 'Cash'],
                ['Piutang berjalan', fmtRp(d.metrics.credit.exposure)],
                ['Rata-rata bayar', d.metrics.credit.pay_days ? `${d.metrics.credit.pay_days} hari` : 'cash'],
                ['Order terakhir', d.metrics.last_order_days != null ? `${d.metrics.last_order_days} hari lalu` : null],
              ]} />
            ) : (
              <div className="ibp-warn"><b>Belum ada di Accurate</b><span>{group ? 'Grup WhatsApp tidak dicatat sebagai pelanggan.' : 'Nomor ini belum tertaut ke pelanggan Accurate. Kirim datanya agar admin membuat pelanggan baru.'}</span></div>
            )}
            <p className="ibp-note">Data pelanggan, faktur, dan piutang dibaca dari Accurate lewat impor BigQuery. GSI Orbit tidak mengubah data Accurate.</p>
            {!group && <button type="button" className="ibp-send" onClick={onAccurate}><Icon name="send" />Kirim ke Accurate</button>}
          </>
        )}
        {tab === 'acc' && (
          <>
            <Kv rows={[
              ['Nomor WhatsApp', num ? labelOf(num) : from(t)],
              ['Pemegang', num ? holderOf(num) : null],
              ['Nomor', num ? phoneOf(num) : t.account_masked],
              ['Status', num ? (num.state === 'connected' ? 'Terhubung' : num.state) : null],
            ]} />
            {num && <button type="button" className="btn ghost" onClick={() => openSheet(<NumberSheet wa={num.wa_number} />)}><Icon name="phone" />Detail nomor</button>}
          </>
        )}
        {tab === 'stats' && (
          <>
            <Kv rows={[
              ['Pesan dimuat', `${th.messages.length} · ${ins} masuk · ${th.messages.length - ins} keluar`],
              ['Belum dibaca', String(t.unread)],
              ...(d ? ([
                ['Skor dealer', `${d.metrics.score}/100`],
                ['Omzet / bulan', fmtRp(d.metrics.omzet_bln)],
                ['Rata-rata order', fmtRp(d.metrics.avg_order)],
                ['Ritme order', d.metrics.rhythm_days ? `${d.metrics.rhythm_days} hari` : null],
                ['Share of wallet', `${d.metrics.sow}%`],
              ] as [string, ReactNode][]) : []),
            ]} />
            {d && d.composition.length > 0 && (
              <>
                <h4>Produk favorit</h4>
                <div className="hb">
                  {d.composition.slice(0, 4).map((c) => (
                    <div className="row" key={c.product} style={{ gridTemplateColumns: '100px 1fr 34px' }}><span className="lbl">{c.product}</span><div className="bar"><i style={{ width: c.pct + '%', background: 'var(--indigo)' }} /></div><span className="v num">{c.pct}%</span></div>
                  ))}
                </div>
              </>
            )}
          </>
        )}
      </div>
    </div>
  )
}

const from = (t: ThreadDetail['thread']) => (t.account_label || t.sales || '').replace(/^Nomor\s+/, '')

/** Kirim ke Accurate: the customer's data ready to paste into Accurate (no API connection yet — nothing is
 * written to Accurate by GSI Orbit; a person saves it there). */
function AccurateSheet({ id }: { id: string }) {
  const { closeSheet, toast } = useFeedback()
  const { data: th } = useThread(id)
  const { data: ctx } = useChatContext(id)
  if (!th) return <SheetHead icon="send" title="Kirim ke Accurate" onClose={closeSheet} />
  const t = th.thread
  const d = ctx?.dealer
  const idf = ctx?.identification
  const notes = (ctx?.extracted ?? []).slice(-3).map((e) => e.annotation.t)
  const fields: [string, string][] = [
    ['Nama pelanggan', d?.name ?? idf?.best_name ?? t.title],
    ['Perusahaan', idf?.best_org ?? ''],
    ['Kode pelanggan', t.dealer_code ?? ''],
    ['Telepon / WhatsApp', t.phone ? fmtPhone(t.phone) : ''],
    ['Kota', d?.city ?? t.dealer_city ?? ''],
    ['Sales', d?.owner?.name ?? t.sales ?? ''],
    ['Sumber', t.subtitle ? `WhatsApp · ${t.subtitle}` : 'WhatsApp'],
    ['Catatan', notes.join('; ')],
  ]
  const shown = fields.filter(([, v]) => v)
  const copy = () => {
    const txt = shown.map(([k, v]) => `${k}: ${v}`).join('\n')
    navigator.clipboard?.writeText(txt).then(() => toast('Data pelanggan disalin — tempel di Accurate'), () => toast('Gagal menyalin; pilih teks lalu salin manual'))
  }
  return (
    <>
      <SheetHead icon="send" title={d ? `Accurate · ${d.name}` : 'Kirim ke Accurate'} sub={d ? 'Pelanggan sudah ada di Accurate · data untuk diperbarui' : 'Pelanggan baru · data untuk dibuat di Accurate'} onClose={closeSheet} />
      <div className="sec">
        <dl className="num-detail">{shown.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
        <p className="ibp-note" style={{ marginTop: 12 }}>Integrasi API Accurate belum tersambung: GSI Orbit tidak menulis ke Accurate. Salin data ini dan simpan di Accurate Online; pelanggan otomatis tertaut ke chat pada impor BigQuery berikutnya.</p>
      </div>
      <div className="ft"><button className="btn quiet" onClick={closeSheet}>Tutup</button><span className="spacer" /><button className="btn primary" onClick={copy}><Icon name="doc" />Salin data</button></div>
    </>
  )
}

const LEAD_KEY = 'chat.lead'
const readLead = () => {
  try {
    const v = localStorage.getItem(LEAD_KEY)
    if (v) return v === 'on'
  } catch {
    /* private window */
  }
  return typeof window === 'undefined' || window.innerWidth >= 1280
}

export function ChatPage() {
  const { threadId } = useParams()
  const nav = useNavigate()
  const [filter, setFilter] = useState<Filter>('all')
  const [account, setAccount] = useState('')
  const [lead, setLeadState] = useState(readLead)
  const setLead = (v: boolean) => {
    setLeadState(v)
    try {
      localStorage.setItem(LEAD_KEY, v ? 'on' : 'off')
    } catch {
      /* not remembered */
    }
  }
  const { openSheet } = useFeedback()
  const { data: list, isSuccess } = useThreads('all', account)
  const { data: all } = useThreads('all')
  const { data: wa } = useWAStatus()
  const unread = useMemo(() => {
    const m: Record<string, number> = {}
    for (const t of all ?? []) if (t.account) m[t.account] = (m[t.account] ?? 0) + t.unread
    return m
  }, [all])
  const firstShown = (list ?? []).find((t) => filter === 'all' || filterOf(t) === filter)
  const active = threadId ?? firstShown?.id
  const rail = <NumberRail account={account} setAccount={setAccount} unread={unread} />
  // No conversation yet (no number linked, or history still syncing): show how to connect the team's numbers.
  if (!threadId && account === '' && list && wa && list.length === 0) {
    return (
      <div className="chat multi chat-empty">
        {rail}
        <div className="pane connect-pane"><ConnectPanel /></div>
      </div>
    )
  }
  const accurate = () => active && openSheet(<AccurateSheet id={active} />)
  return (
    <div className={`chat multi komo ${threadId ? 'show-thread' : ''} ${lead && active ? 'with-lead' : ''}`}>
      {rail}
      <ThreadList list={list ?? []} loaded={isSuccess} filter={filter} setFilter={setFilter} account={account} active={active} onPick={(id) => nav('/chat/' + id)} />
      {active && lead && <LeadPane key={'l' + active} id={active} onClose={() => setLead(false)} onAccurate={accurate} />}
      {active ? <ThreadPane key={active} id={active} picked={!!threadId} onBack={() => nav('/chat')} lead={lead} setLead={setLead} onAccurate={accurate} /> : <div className="pane th-pane" />}
    </div>
  )
}
