import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MessageView, ThreadDetail, ThreadView } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn, ProposalSheet } from '../../components/actions'
import { useFeedback } from '../../components/feedback'
import { ScoreRing } from '../../components/ui'
import { fmtRp, hhmm, shortDate, wib } from '../../lib/format'
import { useChatContext, useNow, useThread, useThreads, useWAStatus } from '../../app/queries'
import { ConnectPanel, holderOf, labelOf, NumberRail, NumberSheet, phoneOf } from './Connect'

const TABS: [string, string][] = [['all', 'Semua'], ['dealer', 'Dealer'], ['group_internal', 'Grup internal'], ['new', 'Nomor baru']]
const SECTION: Record<string, string> = { dealer: 'Dealer', group: 'Grup internal', new: 'Nomor baru' }

// Proposal buttons on annotations get their real labels from AI proposals (stage 05); until then the label
// comes from the mockup's action of the same id.
const ACT_LABEL: Record<string, [string, string]> = {
  so_sinar: ['Buat SO', 'check'], credit_graha: ['Setujui DP 50%', 'shield'], price_indo: ['Setujui counter', 'cash'],
  reorder_mitra: ['Kirim', 'chat'], transfer_cam: ['Usulkan transfer', 'refresh'],
}

function initials(t: ThreadView | ThreadDetail['thread']) {
  if (t.kind === 'group') return '#'
  if (t.kind === 'new') return '?'
  return t.title.replace(/^(Pak|Bu|Mbak|Mas)\s+/, '').split(' ').map((w) => w[0]).join('').slice(0, 2).toUpperCase()
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

function ThreadList({ tab, setTab, account, active, onPick }: { tab: string; setTab: (t: string) => void; account: string; active?: string; onPick: (id: string) => void }) {
  const { data: list = [], isSuccess } = useThreads(tab, account)
  const { data: wa } = useWAStatus()
  const now = useNow()
  const n = wa?.items.find((x) => x.wa_number === account)
  const { openSheet } = useFeedback()
  return (
    <div className="pane list-pane">
      <div className="list-acc">{n ? <><b>{labelOf(n)}<button type="button" className="acc-info" onClick={() => openSheet(<NumberSheet wa={n.wa_number} />)} title="Detail nomor" aria-label="Detail nomor"><Icon name="doc" /></button></b><span>{[holderOf(n), phoneOf(n)].filter(Boolean).join(' · ')}</span></> : <><b>Semua nomor</b><span>{wa?.items.length ?? 0} nomor WhatsApp · {wa?.items.filter((x) => x.state === 'connected').length ?? 0} terhubung</span></>}</div>
      <div className="chat-tabs">
        {TABS.map(([k, l]) => <button key={k} className={tab === k ? 'is-active' : ''} onClick={() => setTab(k)}>{l}</button>)}
      </div>
      <div className="chat-list">
        {isSuccess && list.length === 0 && <div className="chat-none">Belum ada percakapan di sini</div>}
        {list.map((c, i) => {
          const hdr = tab === 'all' && (i === 0 || list[i - 1].kind !== c.kind) ? <div className="chat-sec">{SECTION[c.kind]}</div> : null
          return (
            <div key={c.id} style={{ display: 'contents' }}>
              {hdr}
              <button className={`ci ${c.id === active ? 'is-active' : ''}`} onClick={() => onPick(c.id)}>
                <span className={`av ${c.kind === 'group' ? 'grp' : c.kind === 'new' ? 'int' : ''}`}>{initials(c)}</span>
                <div>
                  <b>{c.title}</b>
                  <span className="sn">{c.subtitle}</span>
                  <span className="lm">{c.last_from ? c.last_from + ': ' : ''}{c.last_body}</span>
                  {c.tag && <div className="tg"><span className={`pill ${c.tag.k}`} style={{ fontSize: 10 }}>{c.tag.t}</span></div>}
                </div>
                <div className="rt">
                  <span>{timeLabel(c.last_message_at, now)}</span>
                  {c.unread > 0 && <span className="un">{c.unread}</span>}
                  <span style={{ fontSize: 10 }}>via {(c.account_label || c.sales).replace(/^Nomor\s+/, '')}</span>
                </div>
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function Bubble({ m, via, group }: { m: MessageView; via: string; group: boolean }) {
  const a = m.annotation
  return (
    <div className={`m ${m.direction}`}>
      {group && m.direction === 'in' && <span className="who">{m.from_name}<span className={`pill ${m.internal ? 'neutral' : 'accent'}`}>{m.internal ? 'internal' : 'eksternal'}</span></span>}
      <div className="bb">{m.body}</div>
      <span className="tm">{hhmm(m.sent_at)}{m.direction === 'out' ? ` · ${via}${m.status === 'sent' && m.signal_id == null ? ' · terkirim' : m.status === 'pending' ? ' · mengirim…' : m.status === 'failed' ? ' · gagal' : ''}` : ''}</span>
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

function ThreadPane({ id, onBack, picked }: { id: string; onBack: () => void; picked: boolean }) {
  const { data } = useThread(id)
  const now = useNow()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [text, setText] = useState('')
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
    if (body.current) body.current.scrollTop = body.current.scrollHeight
  }, [data])
  useEffect(() => {
    if (picked && data && data.thread.unread > 0) api.post(`/chat/threads/${id}/read`).then(() => qc.invalidateQueries({ queryKey: ['chat', 'threads'] }))
  }, [data, id, qc, picked])
  if (!data) return <div className="pane th-pane" />
  const t = data.thread
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const v = text.trim()
    if (!v) return
    setText('')
    send.mutate(v)
  }
  return (
    <div className="pane th-pane">
      <div className="th-h">
        <button className="back" onClick={onBack} aria-label="Kembali"><Icon name="arrow" /></button>
        <span className="av">{initials(t)}</span>
        <div><b>{t.title}</b><span>{t.subtitle}</span></div>
        <div className="via"><b>{from}</b> · {t.account_masked}<br />{t.kind === 'group' ? 'grup internal · stok & tugas' : t.kind === 'new' ? 'nomor baru · identifikasi' : 'dealer · dibaca agen'}</div>
      </div>
      <div className="th-body" ref={body}>
        {data.messages.map((m, i) => {
          const sep = i === 0 || dayKey(data.messages[i - 1].sent_at) !== dayKey(m.sent_at) ? <div className="day">{dayLabel(m.sent_at, now)}</div> : null
          return (
            <div key={m.id} style={{ display: 'contents' }}>
              {sep}
              <Bubble m={m} via={from} group={t.kind === 'group'} />
            </div>
          )
        })}
      </div>
      <div className="th-c">
        <div className="sug">
          {(t.suggestions ?? []).map((x) => <button key={x} className="chip" onClick={() => setText(x)}><span className="ai" style={{ fontSize: 0 }} />{x}</button>)}
        </div>
        <form onSubmit={submit}>
          <span className="as"><Icon name="chat" />Balas dari {from}</span>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Tulis balasan…" autoComplete="off" />
          <button type="submit" className="send" aria-label="Kirim" disabled={send.isPending}><Icon name="send" /></button>
        </form>
        <span className="pol"><Icon name="lock" />Dikirim dari {from} ({t.account_masked}) · jeda & "mengetik…" seperti manusia · penjaga anti-blokir</span>
      </div>
    </div>
  )
}

function ContextPane({ id }: { id: string }) {
  const { data } = useChatContext(id)
  const nav = useNavigate()
  const { toast } = useFeedback()
  if (!data) return <div className="pane ctx-pane ctx" />
  if (data.kind === 'group') {
    const tasks = data.extracted.filter((e) => e.annotation.k === 'indigo' || e.annotation.act)
    return (
      <div className="pane ctx-pane ctx">
        <div><h3>Konteks</h3><div className="box" style={{ fontSize: 13, lineHeight: 1.55, color: 'var(--text-2)' }}>Grup internal gudang: dibaca untuk stok, surat jalan, dan tugas. Tidak memengaruhi skor dealer.</div></div>
        <div>
          <h3>Tugas terdeteksi</h3>
          <div className="box">
            {tasks.map((e) => (
              <div className="task" key={e.sent_at + e.annotation.t}>
                <div><b>{e.annotation.t.replace(/^Tugas → /, '')}</b><span>{e.from_name} · dari pesan {hhmm(e.sent_at)}</span></div>
                {e.annotation.act ? <ActBtn small label={ACT_LABEL[e.annotation.act]?.[0] ?? 'Lihat'} icon={ACT_LABEL[e.annotation.act]?.[1] ?? 'check'} /> : <button className="btn ghost" onClick={() => toast('Tugas dibuat di Basecamp')}>Ke Basecamp</button>}
              </div>
            ))}
          </div>
        </div>
      </div>
    )
  }
  if (data.kind === 'new') {
    const idf = data.identification
    return (
      <div className="pane ctx-pane ctx">
        <div>
          <h3>Siapa ini</h3>
          <div className="box">
            <b style={{ fontFamily: 'var(--font-display)', fontSize: 14 }}>{idf?.best_name ?? 'Belum dikenali'}</b>
            <div style={{ fontSize: 12, color: 'var(--text-3)' }}>{idf ? `${idf.best_org} · skor ${idf.score}` : 'AI Prospek mengidentifikasi nomor ini setelah ia mengirim pesan'}</div>
            <div className="hr" />
            <ul className="ext">{(idf?.sources ?? []).map((s) => <li key={s.source}><Icon name={s.ok === 'true' ? 'check' : 'alert'} /><span>{s.value}</span></li>)}</ul>
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>
            <ActBtn next={data.proposals?.new_dealer} label="Buat dealer tier C" icon="building" />
            <ActBtn next={data.proposals?.price_list} label="Kirim harga" icon="send" ghost />
          </div>
        </div>
        {idf?.potential && <div><h3>Potensi</h3><div className="box" style={{ fontSize: 13, lineHeight: 1.5 }}>{idf.potential}</div></div>}
      </div>
    )
  }
  const d = data.dealer
  if (!d) return <div className="pane ctx-pane ctx" />
  const m = d.metrics
  return (
    <div className="pane ctx-pane ctx">
      <div>
        <h3>Dealer</h3>
        <div className="box acc">
          <ScoreRing h={m.score} />
          <div><b>{d.name}</b><span>Tier {d.tier} · {d.city} · {d.credit_limit ? `${fmtRp(m.credit.exposure)} / ${fmtRp(d.credit_limit)}` : 'cash'} · bayar {m.credit.pay_days ? m.credit.pay_days + ' hr' : 'cash'}</span></div>
        </div>
        {d.next && <div style={{ marginTop: 8 }}><ActBtn next={d.next} /></div>}
        <button className="btn quiet" style={{ width: '100%', justifyContent: 'center', marginTop: 4 }} onClick={() => nav('/dealer/' + d.id)}>Buka halaman dealer <Icon name="arrow" /></button>
      </div>
      <div>
        <h3>Diekstrak dari chat ini <span className="ai" style={{ marginLeft: 4 }} /></h3>
        <ul className="ext">
          {data.extracted.length === 0 && <li><Icon name="check" /><span style={{ color: 'var(--text-3)' }}>Belum ada yang diekstrak</span></li>}
          {data.extracted.map((e) => (
            <li key={e.sent_at + e.annotation.t}><Icon name={e.annotation.k === 'bad' ? 'flag' : e.annotation.k === 'warn' ? 'alert' : 'check'} /><span>{e.annotation.t}<br /><span style={{ color: 'var(--text-3)', fontSize: 11.5 }}>{hhmm(e.sent_at)}</span></span></li>
          ))}
        </ul>
      </div>
      <div>
        <h3>Produk favorit</h3>
        <div className="box">
          <div className="hb">
            {d.composition.slice(0, 3).map((c) => (
              <div className="row" key={c.product} style={{ gridTemplateColumns: '100px 1fr 34px' }}><span className="lbl">{c.product}</span><div className="bar"><i style={{ width: c.pct + '%', background: 'var(--indigo)' }} /></div><span className="v num">{c.pct}%</span></div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

export function ChatPage() {
  const { threadId } = useParams()
  const nav = useNavigate()
  const [tab, setTab] = useState('all')
  const [account, setAccount] = useState('')
  const { data: list } = useThreads('all', account)
  const { data: all } = useThreads('all')
  const { data: wa } = useWAStatus()
  const unread = useMemo(() => {
    const m: Record<string, number> = {}
    for (const t of all ?? []) if (t.account) m[t.account] = (m[t.account] ?? 0) + t.unread
    return m
  }, [all])
  const active = threadId ?? list?.[0]?.id
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
  return (
    <div className={`chat multi ${threadId ? 'show-thread' : ''}`}>
      {rail}
      <ThreadList tab={tab} setTab={setTab} account={account} active={active} onPick={(id) => nav('/chat/' + id)} />
      {active ? <ThreadPane id={active} picked={!!threadId} onBack={() => nav('/chat')} /> : <div className="pane th-pane" />}
      {active ? <ContextPane id={active} /> : <div className="pane ctx-pane ctx" />}
    </div>
  )
}
