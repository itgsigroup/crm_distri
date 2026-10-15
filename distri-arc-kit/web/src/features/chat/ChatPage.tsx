import { useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Items } from '../../api/client'
import type { MediaView, MessageView, ReplySuggestion, ThreadDetail, ThreadView, WATeam } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn, ProposalSheet } from '../../components/actions'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtRp, hhmm, shortDate, wib, sowText } from '../../lib/format'
import { useChatContext, useNow, useThread, useThreads, useWAStatus } from '../../app/queries'
import { useOrch } from '../../app/orch'
import { ConnectPanel, holderOf, labelOf, NumberRail, NumberSheet, phoneOf, teamName } from './Connect'
import { countFilters, displayTitle, FILTERS, fmtPhone, inFilter, lookupLinks, matchThread, waitLabel, type Filter } from './inbox'
import { AttachMenu, FilePreview, MAX_FILE, Mic, PollSheet, RecordingBar, useVoiceRecorder } from './Composer'
import { bubbleLine, deletableNow, editableNow, fmtDuration, fmtSize, forwardable, mediaCaption, mediaName } from './bubble'
import { ForwardSheet } from './Forward'
import { ChatSearch, Marked } from './ChatSearch'
import type { ChatHit } from './findInChat'


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

// Compact filter chips: an icon and the count; the active chip also shows its name, every chip names itself on hover.
const FILTER_ICONS: Record<Filter, string> = {
  all: 'chat', unread: 'bell', unreplied: 'send', dealer: 'building', group_internal: 'people', group_external: 'net', new: 'spark', supplier: 'box',
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
  const team = account.startsWith('team:') ? (account.slice(5) as WATeam) : undefined
  const nums = (wa?.items ?? []).filter((x) => !team || (x.team ?? 'sales') === team)
  const counts = useMemo(() => countFilters(list), [list])
  // the list holds the chat window (14 days); a search of 3+ letters asks the server across every stored
  // conversation, by name (spellings forgiven) or number
  const [dq, setDq] = useState('')
  useEffect(() => {
    const h = setTimeout(() => setDq(q.trim()), 350)
    return () => clearTimeout(h)
  }, [q])
  const deep = dq.length >= 3
  const { data: found, isFetching: searching } = useQuery({
    queryKey: ['chat', 'search', dq, account],
    enabled: deep,
    staleTime: 30_000,
    queryFn: () => {
      const p = new URLSearchParams({ all: '1', q: dq })
      if (team) p.set('team', team)
      else if (account) p.set('account', account)
      return api.get<Items<ThreadView>>('/chat/threads?' + p.toString()).then((r) => r.items)
    },
  })
  const base = deep && found ? found : list
  // the open conversation stays in "Belum dibaca" / "Belum dibalas" after reading or answering it, as in WhatsApp
  const shown = base.filter((t) => (inFilter(t, filter) || ((filter === 'unread' || filter === 'unreplied') && t.id === active)) && (deep && found ? true : matchThread(t, q)))
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
            <b>{n ? `${labelOf(n)}${n.user_role || n.sales_id ? ` - ${n.user_role || 'Sales'}` : ''}` : team ? `Tim ${teamName(team)}` : 'Semua nomor'}</b>
            <span>{n ? `${phoneOf(n)} · Tim ${teamName(n.team)}` : `${nums.length} nomor · ${nums.filter((x) => x.state === 'connected').length} terhubung`}</span>
          </span>
          {n && <Icon name="chev" className="i ibl-chev" />}
        </button>
        <button type="button" className="ibl-btn" onClick={refresh} title="Muat ulang" aria-label="Muat ulang"><Icon name="refresh" /></button>
      </div>
      <div className="ibl-search"><Icon name="search" /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari chat" aria-label="Cari chat" /></div>
      <div className="ibl-chips" role="tablist">
        {FILTERS.map(([k, l]) => (
          <button key={k} role="tab" aria-selected={filter === k} aria-label={l} title={l} className={filter === k ? 'is-active' : ''} onClick={() => setFilter(k)}>
            <Icon name={FILTER_ICONS[k]} />{filter === k && <em>{l}</em>}<span>{counts[k].toLocaleString('id-ID')}</span>
          </button>
        ))}
      </div>
      <div className="chat-list ibl-list">
        {deep && searching && <div className="chat-none">Mencari di semua chat…</div>}
        {loaded && shown.length === 0 && !(deep && searching) && <div className="chat-none">{q ? `Tidak ada chat yang cocok dengan "${q}"${deep ? ' di semua chat Orbit — chat lama di HP yang belum pernah berbalas pesan sejak nomor ditautkan belum ada di sini' : ''}` : filter === 'unread' ? 'Semua chat sudah dibaca' : filter === 'unreplied' ? 'Semua chat pelanggan sudah dibalas' : 'Belum ada percakapan di sini'}</div>}
        {shown.map((c) => (
          <button key={c.id} className={`ibr ${c.id === active ? 'is-active' : ''}`} onClick={() => onPick(c.id)}>
            <span className={`av ${c.kind === 'group' ? 'grp' : ''}`}>{c.kind === 'group' ? <Icon name="people" /> : initials(c)}</span>
            <div className="ibr-m">
              <div className="ibr-top">
                <b>{displayTitle(c)}</b>
                {c.kind === 'group' && <span className="ibr-tag grp">{c.group_kind === 'external' ? 'GRUP PELANGGAN' : 'GRUP'}</span>}
                {c.contact_type === 'supplier' && <span className="ibr-tag grp">PEMASOK</span>}
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

/** Analisa WhatsApp under one bubble: the request, the reply state, the AI note (ADR 0027). */
function BubbleAnalysis({ a, dir }: { a: NonNullable<MessageView['analysis']>; dir: MessageView['direction'] }) {
  const l = bubbleLine(a)
  return (
    <div className={`bub-an ${dir}`}>
      <div className="bub-an-row">
        {l.state && <span className={`bub-an-st ${l.tone}`}>{l.state}</span>}
        {!a.analyzed && <span className="bub-an-new">belum dianalisa</span>}
      </div>
      {a.note && <div className="bub-an-note" title="ditulis Claude lewat MCP"><span className="ai" />{a.note}</div>}
    </div>
  )
}

/** Loads a media file as a blob (video, voice note): seekable, and the API's error message when it fails. */
function useMediaBlob(url: string) {
  const [state, setState] = useState<{ src?: string; loading?: boolean; error?: string }>({})
  useEffect(() => () => { if (state.src) URL.revokeObjectURL(state.src) }, [state.src])
  const load = () => {
    if (state.src || state.loading) return
    setState({ loading: true })
    fetch(url, { credentials: 'same-origin' }).then(async (r) => {
      if (!r.ok) {
        const e = await r.json().catch(() => null)
        throw new Error(e?.error?.message ?? 'Gagal memuat media')
      }
      return r.blob()
    }).then((b) => setState({ src: URL.createObjectURL(b) }), (e: Error) => setState({ error: e.message }))
  }
  return { ...state, load }
}

function ReplyIcon() {
  return <svg className="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M10 9V5l-7 7 7 7v-4.1c5 0 8.5 1.6 11 5.1-1-5-4-10-11-11z" fill="currentColor" /></svg>
}

/** WhatsApp-style menu of one bubble (⌄ in its corner): Balas, Salin, Edit, Hapus untuk semua orang. */
function BubbleMenu({ m, edit, del, onReply, onEdit, onDelete, onForward }: {
  m: MessageView; edit: boolean; del: boolean; onReply: () => void; onEdit: () => void; onDelete: () => void; onForward?: () => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLSpanElement>(null)
  const { toast } = useFeedback()
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', esc)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', esc) }
  }, [open])
  const pick = (f: () => void) => () => { setOpen(false); f() }
  const text = mediaCaption(m.body, m.media)
  const copy = () => navigator.clipboard?.writeText(text).then(() => toast('Pesan disalin'), () => toast('Gagal menyalin'))
  return (
    <span className={`bub-menu${open ? ' open' : ''}`} ref={ref}>
      <button type="button" className="bub-menu-btn" onClick={() => setOpen(!open)} aria-label="Menu pesan" aria-haspopup="menu" aria-expanded={open}><Icon name="chev" /></button>
      {open && (
        <span className="bub-menu-list" role="menu">
          {m.can_reply && <button type="button" role="menuitem" onClick={pick(onReply)}><ReplyIcon />Balas</button>}
          {text && <button type="button" role="menuitem" onClick={pick(copy)}><Icon name="doc" />Salin</button>}
          {onForward && <button type="button" role="menuitem" onClick={pick(onForward)}><Icon name="send" />Teruskan</button>}
          {edit && <button type="button" role="menuitem" onClick={pick(onEdit)}><Icon name="edit" />Edit</button>}
          {del && <button type="button" role="menuitem" className="bad" onClick={pick(onDelete)}><Icon name="x" />Hapus untuk semua orang</button>}
        </span>
      )}
    </span>
  )
}

/** The quoted bubble above a reply; a click jumps to it. */
function QuoteBlock({ q, mine, onJump }: { q: NonNullable<MessageView['quote']>; mine: string; onJump: (id: string) => void }) {
  const who = q.direction === 'out' ? mine || 'Anda' : q.from_name || 'Pelanggan'
  const text = q.media_kind ? mediaCaption(q.body, { kind: q.media_kind, viewable: false }) || mediaName({ kind: q.media_kind, viewable: false }) : q.body
  return (
    <button type="button" className={`bub-quote ${q.direction === 'out' ? 'out' : 'in'}`} disabled={!q.id} onClick={() => q.id && onJump(q.id)} title={q.id ? 'Lihat pesan yang dibalas' : undefined}>
      <b>{q.id ? who : 'Pesan'}</b>
      <span>{text}</span>
    </button>
  )
}

function Play() {
  return <svg className="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5v14l11-7z" fill="currentColor" /></svg>
}

/** A bubble's photo, video, voice note or document, fetched from WhatsApp when shown (ADR 0029). */
function BubbleMedia({ md, url, pending, onZoom }: { md: MediaView; url: string; pending?: boolean; onZoom: (url: string) => void }) {
  const blob = useMediaBlob(url)
  const [imgFailed, setImgFailed] = useState(false)
  const name = mediaName(md)
  if (md.kind === 'poll') return <div className="bub-poll-h"><Icon name="chart" />Polling</div>
  if (pending && !md.viewable) {
    return <div className="bub-media none"><Icon name="refresh" />{md.kind === 'audio' && md.ptt ? `Pesan suara ${fmtDuration(md.seconds)}` : name}{md.size ? ` · ${fmtSize(md.size)}` : ''} · mengirim…</div>
  }
  if (!md.viewable || md.kind === 'location' || md.kind === 'contact') {
    if (md.kind === 'location' || md.kind === 'contact') return null
    return <div className="bub-media none" title="Media dari sebelum pembaruan ini — buka di WhatsApp ponsel"><Icon name="doc" />{name} · buka di ponsel</div>
  }
  if (md.kind === 'image' || md.kind === 'sticker') {
    if (imgFailed) return <div className="bub-media none"><Icon name="alert" />{name} tidak bisa dimuat</div>
    return (
      <button type="button" className={`bub-media ${md.kind === 'sticker' ? 'sticker' : 'img'}`} onClick={() => md.kind === 'image' && onZoom(url)} aria-label={`Buka ${name}`}>
        <img src={url} loading="lazy" alt={name} onError={() => setImgFailed(true)} />
      </button>
    )
  }
  if (md.kind === 'video' || md.kind === 'gif') {
    if (blob.src) return <video className="bub-media vid" src={blob.src} controls={md.kind === 'video'} autoPlay playsInline loop={md.kind === 'gif'} muted={md.kind === 'gif'} />
    return (
      <button type="button" className="bub-media vid-ph" onClick={blob.load} disabled={blob.loading}>
        <span className="bub-play">{blob.loading ? <Icon name="refresh" /> : <Play />}</span>
        <span>{blob.error ?? `${name}${md.seconds ? ' · ' + fmtDuration(md.seconds) : ''}${md.size ? ' · ' + fmtSize(md.size) : ''}`}</span>
      </button>
    )
  }
  if (md.kind === 'audio') {
    if (blob.src) return <audio className="bub-media aud" src={blob.src} controls autoPlay />
    return (
      <button type="button" className="bub-media vn" onClick={blob.load} disabled={blob.loading} aria-label={`Putar ${name}`}>
        <span className="bub-play">{blob.loading ? <Icon name="refresh" /> : <Play />}</span>
        <span className="vn-bar" aria-hidden="true" />
        <span className="vn-t"><Icon name="mic" />{blob.error ?? (fmtDuration(md.seconds) || name)}</span>
      </button>
    )
  }
  return (
    <a className="bub-media doc" href={url} target="_blank" rel="noreferrer noopener" download={md.file_name || undefined}>
      <Icon name="doc" /><span><b>{name}</b><small>{[md.mime?.split('/')[1]?.split(';')[0]?.toUpperCase(), fmtSize(md.size)].filter(Boolean).join(' · ')}</small></span>
    </a>
  )
}

/** Rekomendasi balasan under a customer's bubble (ADR 0038): the stock of the product the message names and a draft
 * the sales can use — it goes into the box to edit; the sales sends it. */
function SuggestBox({ s, asOf, onUse }: { s: ReplySuggestion; asOf?: string | null; onUse: (text: string) => void }) {
  const where = s.branch || 'semua cabang'
  return (
    <div className={`sug ${s.product ? (s.ready ? 'ready' : 'empty') : 'unknown'}`}>
      <div className="sug-h">
        <Icon name="box" />
        <b>{s.product ? (s.ready ? `Ready · ${s.qty.toLocaleString('id-ID')} di ${where}` : `Kosong di ${where}`) : s.rule}</b>
        {s.product && <span>{s.product}</span>}
      </div>
      {s.others.length > 0 && <div className="sug-o">Cabang lain: {s.others.slice(0, 4).map((o) => `${o.branch} ${o.qty.toLocaleString('id-ID')}`).join(' · ')}</div>}
      {s.candidates && s.candidates.length > 0 && <div className="sug-o">Mungkin: {s.candidates.join(' · ')}</div>}
      {s.note && <div className="sug-n">{s.note}</div>}
      {s.draft && (
        <div className="sug-d">
          <span>“{s.draft}”</span>
          <button type="button" className="btn ghost" onClick={() => onUse(s.draft!)}><Icon name="edit" />Pakai jawaban</button>
        </div>
      )}
      <small>Rekomendasi sistem dari data stok{asOf ? ` · sinkron ${shortDate(asOf)} ${hhmm(asOf)}` : ''} · dikirim hanya bila Anda menekan kirim</small>
    </div>
  )
}

function Bubble({ m, via, group, analysis, thread, now, onEdit, onReply, onDelete, onJump, onZoom, onUse, asOf, mark }: {
  m: MessageView; via: string; group: boolean; analysis: boolean; thread: string; now: Date; mark?: string
  onEdit: (m: MessageView) => void; onReply: (m: MessageView) => void; onDelete: (m: MessageView) => void; onJump: (id: string) => void; onZoom: (url: string) => void
  onUse?: (m: MessageView, text: string) => void; asOf?: string | null
}) {
  const { openSheet } = useFeedback()
  const caption = mediaCaption(m.body, m.media)
  const edit = editableNow(m, now)
  const del = deletableNow(m, now)
  const forward = forwardable(m)
  if (m.deleted) {
    return (
      <div className={`m ${m.direction}`} data-mid={m.id}>
        {group && m.direction === 'in' && <span className="who">{m.from_name}</span>}
        <div className="bb bub-deleted"><Icon name="lock" />{m.direction === 'out' ? 'Anda menghapus pesan ini' : 'Pesan ini dihapus'}<span className="tm">{hhmm(m.sent_at)}</span></div>
      </div>
    )
  }
  return (
    <div className={`m ${m.direction}${m.media ? ' has-media' : ''}`} data-mid={m.id}>
      {group && m.direction === 'in' && <span className="who">{m.from_name}<span className={`pill ${m.internal ? 'neutral' : 'accent'}`}>{m.internal ? 'internal' : 'eksternal'}</span></span>}
      <div className="bb">
        {m.quote && <QuoteBlock q={m.quote} mine={via} onJump={onJump} />}
        {m.media && <BubbleMedia md={m.media} url={`/api/chat/threads/${thread}/messages/${m.id}/media`} pending={m.status === 'pending'} onZoom={onZoom} />}
        {m.media?.kind === 'poll' ? <PollBody text={caption} /> : mark ? <Marked text={caption} q={mark} /> : caption}
        <span className="tm">{m.edited_at ? 'diedit · ' : ''}{m.direction === 'out' ? `${via}${m.status === 'sent' && m.signal_id == null ? ' · terkirim' : m.status === 'pending' ? ' · mengirim…' : m.status === 'failed' ? ' · gagal' : ''} · ` : ''}{hhmm(m.sent_at)}</span>
        {(m.can_reply || edit || del || caption || forward) && <BubbleMenu m={m} edit={edit} del={del} onReply={() => onReply(m)} onEdit={() => onEdit(m)} onDelete={() => onDelete(m)} onForward={forward ? () => openSheet(<ForwardSheet m={m} from={thread} />) : undefined} />}
        {m.can_reply && <span className="bub-acts"><button type="button" onClick={() => onReply(m)} title="Balas pesan ini" aria-label="Balas"><ReplyIcon /></button></span>}
      </div>
      {m.auto_reply && <span className="bub-hold" title="Pesan otomatis WhatsApp Business — bukan jawaban dan bukan permintaan">balasan otomatis · tidak dihitung</span>}
      {m.holding && !m.auto_reply && m.direction === 'out' && !(analysis && m.analysis) && (
        <span className="bub-hold" title="Pesan ini hanya meminta pelanggan menunggu — waktu respon tetap dihitung sampai pelanggan mendapat jawaban">jawaban sementara · waktu respon tetap berjalan</span>
      )}
      {m.suggest && onUse && <SuggestBox s={m.suggest} asOf={asOf} onUse={(text) => onUse(m, text)} />}
      {m.proposal && <span className="ann"><span className="ai" />Usulan Claude · <AnnProposal p={m.proposal} /></span>}
      {analysis && m.analysis && <BubbleAnalysis a={m.analysis} dir={m.direction} />}
    </div>
  )
}

/** A poll's question and options ("Q\n• a\n• b"); votes are counted on the phones (OPEN-QUESTIONS). */
function PollBody({ text }: { text: string }) {
  const [q, ...opts] = text.split('\n')
  return (
    <span className="bub-poll">
      <b>{q}</b>
      {opts.map((o) => <span key={o} className="bub-poll-o"><i />{o.replace(/^•\s*/, '')}</span>)}
    </span>
  )
}

/** A photo opened full size; Esc or a click closes it. */
function Lightbox({ url, onClose }: { url: string; onClose: () => void }) {
  useEffect(() => {
    const k = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', k)
    return () => window.removeEventListener('keydown', k)
  }, [onClose])
  return (
    <div className="bub-lightbox" role="dialog" aria-label="Foto" onClick={onClose}>
      <img src={url} alt="Foto" onClick={(e) => e.stopPropagation()} />
      <div className="bub-lb-bar">
        <a href={url} download onClick={(e) => e.stopPropagation()}>Unduh</a>
        <button type="button" onClick={onClose} aria-label="Tutup"><Icon name="x" /></button>
      </div>
    </div>
  )
}

function AnnProposal({ p }: { p: NonNullable<MessageView['proposal']> }) {
  const { openSheet } = useFeedback()
  const done = p.status === 'executed' || p.status === 'approved' || p.status === 'edited'
  return <button onClick={() => openSheet(<ProposalSheet id={p.id} />)}>{done ? 'Dijalankan ✓' : p.status === 'rejected' ? 'Ditolak' : p.button || 'Lihat'}</button>
}

const PER_MSG_KEY = 'chat.analysis.per-message'

/** Show the analysis under every bubble (on by default, remembered on this browser). */
function usePerMessageAnalysis(): [boolean, (v: boolean) => void] {
  const [on, setOn] = useState(() => {
    try {
      return localStorage.getItem(PER_MSG_KEY) !== 'off'
    } catch {
      return true
    }
  })
  const set = (v: boolean) => {
    setOn(v)
    try {
      localStorage.setItem(PER_MSG_KEY, v ? 'on' : 'off')
    } catch {
      /* private window: not remembered */
    }
  }
  return [on, set]
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
  const [olderMore, setOlderMore] = useState(true)
  const [loading, setLoading] = useState(false)
  const [perMsg, setPerMsg] = usePerMessageAnalysis()
  const [editing, setEditing] = useState<MessageView | null>(null)
  const [replying, setReplying] = useState<MessageView | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const { openSheet } = useFeedback()
  const [zoom, setZoom] = useState<string | null>(null)
  const [searching, setSearching] = useState(false)
  const [mark, setMark] = useState('')
  const pendingJump = useRef<string | null>(null)
  const body = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLTextAreaElement>(null)
  const from = (data?.thread.account_label || data?.thread.sales || '').replace(/^Nomor\s+/, '')
  const send = useMutation({
    mutationFn: (v: { body: string; reply_to?: string }) => api.post(`/chat/threads/${id}/messages`, v),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast(`Masuk antrean kirim ke ${data?.thread.title} dari ${from} · lewat penjaga anti-blokir`)
    },
    onError: (e: Error) => toast(e.message),
  })
  const edit = useMutation({
    mutationFn: (v: { id: string; body: string }) => api.patch<{ status: string }>(`/chat/threads/${id}/messages/${v.id}`, { body: v.body }),
    onSuccess: (r, v) => {
      setOlder((o) => o.map((m) => (m.id === v.id && r.status === 'pending' ? { ...m, body: v.body } : m)))
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast(r.status === 'pending' ? 'Pesan di antrean diubah sebelum dikirim' : 'Edit dikirim ke WhatsApp — pesan pelanggan ikut berubah')
    },
    onError: (e: Error) => toast(e.message),
  })
  const upload = useMutation({
    mutationFn: (form: FormData) => api.upload(`/chat/threads/${id}/messages`, form),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast(`Masuk antrean kirim ke ${data?.thread.title} dari ${from} · lewat penjaga anti-blokir`)
    },
    onError: (e: Error) => toast(e.message),
  })
  const sendForm = (f: Blob, name: string, extra: Record<string, string>) => {
    const form = new FormData()
    form.append('file', f, name)
    for (const [k, v] of Object.entries(extra)) form.append(k, v)
    if (replying) form.append('reply_to', replying.id)
    setReplying(null)
    upload.mutate(form)
  }
  const voice = useVoiceRecorder(
    (blob, seconds) => sendForm(blob, blob.type.includes('ogg') ? 'pesan-suara.ogg' : blob.type.includes('mp4') ? 'pesan-suara.m4a' : 'pesan-suara.webm', { voice: '1', seconds: String(seconds) }),
    toast,
  )
  const pickFile = (f: File) => {
    if (f.size > MAX_FILE) return toast('File lebih dari 25 MB — kirim lewat HP atau perkecil dulu')
    setEditing(null)
    setFile(f)
    requestAnimationFrame(() => input.current?.focus())
  }
  const openPoll = () => openSheet(<PollSheet onSend={(p) => {
    const body: Record<string, unknown> = { poll: p }
    if (replying) body.reply_to = replying.id
    return api.post(`/chat/threads/${id}/messages`, body).then(() => {
      setReplying(null)
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast('Polling masuk antrean kirim · lewat penjaga anti-blokir')
    }, (e: Error) => { toast(e.message); throw e })
  }} />)
  const remove = useMutation({
    mutationFn: (mid: string) => api.del<{ status: string }>(`/chat/threads/${id}/messages/${mid}`),
    onSuccess: (r, mid) => {
      setOlder((o) => o.map((m) => (m.id === mid && r.status === 'deleted' ? { ...m, deleted: true, body: '' } : m)))
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast(r.status === 'deleted' ? 'Pesan dihapus' : 'Dihapus untuk semua orang lewat WhatsApp — pesan di HP pelanggan ikut terhapus')
    },
    onError: (e: Error) => toast(e.message),
  })
  const deleteMsg = (m: MessageView) => {
    if (window.confirm(m.status === 'pending' ? 'Batalkan pesan ini sebelum terkirim?' : 'Hapus pesan ini untuk semua orang? Pesan di HP pelanggan ikut terhapus.')) remove.mutate(m.id)
  }
  const startReply = (m: MessageView) => {
    setEditing(null)
    setReplying(m)
    requestAnimationFrame(() => input.current?.focus())
  }
  // Rekomendasi balasan: the draft goes into the box as a reply to that message, for the sales to edit and send
  const useSuggestion = (m: MessageView, draft: string) => {
    setEditing(null)
    if (m.can_reply) setReplying(m)
    setText(draft)
    requestAnimationFrame(() => input.current?.focus())
  }
  const flash = (mid: string) => {
    const el = body.current?.querySelector<HTMLElement>(`[data-mid="${mid}"]`)
    if (!el) return false
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el.classList.remove('flash')
    void el.offsetWidth
    el.classList.add('flash')
    return true
  }
  const jumpTo = (mid: string) => {
    if (!flash(mid)) toast('Pesan yang dibalas ada di riwayat lebih lama — gulir ke atas')
  }
  // a search result older than the loaded messages: load every message from it up to them (the chat stays
  // continuous), then go to it once the bubbles are on the page
  const jumpToHit = (h: ChatHit) => {
    if (flash(h.id)) return
    const first = [...older, ...(data?.messages ?? [])][0]
    if (!first) return
    setLoading(true)
    api.get<ThreadDetail>(`/chat/threads/${id}?from=${encodeURIComponent(h.sent_at)}&before=${encodeURIComponent(first.sent_at)}`).then((r) => {
      pendingJump.current = h.id
      setOlder((o) => [...r.messages, ...o])
      setOlderMore(!!r.has_more)
    }, (e: Error) => toast(e.message)).finally(() => setLoading(false))
  }
  const startEdit = (m: MessageView) => {
    setReplying(null)
    setEditing(m)
    setText(m.body)
    requestAnimationFrame(() => input.current?.focus())
  }
  const cancelEdit = () => {
    setEditing(null)
    setText('')
  }
  useEffect(() => {
    if (body.current && older.length === 0) body.current.scrollTop = body.current.scrollHeight
  }, [data, older.length])
  const keepOffset = useRef<number | null>(null)
  useLayoutEffect(() => {
    // after older bubbles are prepended, keep the bubble the user was reading in place — or go to the search result
    const el = body.current
    if (pendingJump.current) {
      const mid = pendingJump.current
      pendingJump.current = null
      if (!flash(mid)) toast('Pesan ini di luar riwayat yang bisa dimuat sekaligus — persempit pencarian')
      return
    }
    if (el && keepOffset.current != null) {
      el.scrollTop = el.scrollHeight - keepOffset.current
      keepOffset.current = null
    }
  }, [older, toast])
  useEffect(() => {
    if (!(picked && data && data.thread.unread > 0)) return
    // opened = read: the count and the "Belum dibaca" tag drop at once, then the lists, the numbers' badges and
    // the other numbers' copies of a group come back from the server
    qc.setQueriesData<ThreadView[]>({ queryKey: ['chat', 'threads'] }, (list) => list?.map((t) => (t.id === id ? { ...t, unread: 0 } : t)))
    qc.setQueryData<ThreadDetail>(['chat', 'thread', id], (d) => (d ? { ...d, thread: { ...d.thread, unread: 0 } } : d))
    api.post(`/chat/threads/${id}/read`).finally(() => {
      qc.invalidateQueries({ queryKey: ['chat', 'threads'] })
      qc.invalidateQueries({ queryKey: ['wa'] })
    })
  }, [data, id, qc, picked])
  // the box grows with the text, up to about six lines, and shrinks back after sending
  useLayoutEffect(() => {
    const el = input.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 140) + 'px'
  }, [text, data])
  if (!data) return <div className="pane th-pane ibc" />
  const t = data.thread
  const messages = [...older, ...data.messages]
  // The first page already holds the whole retention window (a year); a very busy chat pages on by itself.
  const more = (older.length ? olderMore : !!data.has_more) && messages.length > 0
  const loadOlder = () => {
    if (!more || loading) return
    setLoading(true)
    if (body.current) keepOffset.current = body.current.scrollHeight - body.current.scrollTop
    api.get<ThreadDetail>(`/chat/threads/${id}?before=${encodeURIComponent(messages[0].sent_at)}`).then((r) => {
      setOlder((o) => [...r.messages, ...o])
      setOlderMore(!!r.has_more)
    }, (e: Error) => toast(e.message)).finally(() => setLoading(false))
  }
  const onScroll = () => {
    if (body.current && body.current.scrollTop < 200) loadOlder()
  }
  // Enter sends, Shift+Enter starts a new line (WhatsApp Web); Enter while an IME is composing a word does neither
  const composerKey = (e: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Escape') {
      if (editing) cancelEdit()
      setReplying(null)
      setFile(null)
      return
    }
    if (e.key !== 'Enter' || e.shiftKey || e.nativeEvent.isComposing) return
    e.preventDefault()
    if (text.trim() || file || editing) e.currentTarget.form?.requestSubmit()
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const v = text.trim()
    if (file && !editing) {
      setText('')
      sendForm(file, file.name, { body: v })
      setFile(null)
      return
    }
    if (!v) return
    setText('')
    if (editing) {
      setEditing(null)
      if (v !== editing.body) edit.mutate({ id: editing.id, body: v })
      return
    }
    send.mutate(replying ? { body: v, reply_to: replying.id } : { body: v })
    setReplying(null)
  }
  // Nama panggilan: what the team calls this number ("Pak Yohanes"), kept with the number and shown with the master name
  const rename = () => {
    const v = window.prompt(`Nama panggilan untuk ${t.phone ? fmtPhone(t.phone) : t.title} — tampil sebagai "Nama - ${t.title}" dan bisa dicari. Kosongkan untuk menghapus.`, t.phone_name ?? '')
    if (v === null) return
    api.put<{ name: string }>(`/chat/threads/${id}/alias`, { name: v }).then(
      (r) => { toast(r.name ? `Nama panggilan: ${r.name}` : 'Nama panggilan dihapus'); qc.invalidateQueries({ queryKey: ['chat'] }) },
      (e) => toast(e instanceof Error ? e.message : 'Gagal menyimpan nama'),
    )
  }
  const links = t.kind === 'group' ? [] : lookupLinks(t.title, t.phone, t.dealer_city)
  // an internal group is never read by Analisa WhatsApp; a customer group is, like a one-to-one chat
  const readable = t.kind !== 'group' || t.group_kind === 'external'
  return (
    <div className="pane th-pane ibc">
      <div className="ibc-h">
        <button className="back" onClick={onBack} aria-label="Kembali"><Icon name="arrow" /></button>
        <span className={`av ${t.kind === 'group' ? 'grp' : ''}`}>{t.kind === 'group' ? <Icon name="people" /> : initials(t)}</span>
        <div className="ibc-who">
          <div className="ibc-name">
            <b>{displayTitle(t)}</b>
            {t.kind !== 'group' && <button type="button" className="ibc-alias" onClick={rename} title={t.phone_name ? 'Ubah nama panggilan' : 'Beri nama panggilan (seperti di HP)'} aria-label="Nama panggilan"><Icon name="edit" /></button>}
            {t.subtitle && <span className="ibr-src">≈ {t.subtitle}</span>}
          </div>
          <div className="ibc-sub">
            {t.phone && <span className="ibc-phone">{fmtPhone(t.phone)}</span>}
            {links.map((l) => <a key={l.key} className="ibc-link" href={l.href} target="_blank" rel="noreferrer noopener">{l.label}</a>)}
            {t.kind === 'group' && <span className="ibc-phone">{from} · {t.account_masked}</span>}
          </div>
        </div>
        <div className="ibc-acts">
          <Waiting since={t.waiting_since} now={now} />
          <button type="button" className={`ibl-btn ibc-find${searching ? ' on' : ''}`} onClick={() => { setSearching(!searching); if (searching) setMark('') }} title="Cari di chat ini" aria-label="Cari di chat ini" aria-pressed={searching}><Icon name="search" /></button>
          {readable && (
            <button type="button" className={`ibc-an-tg${perMsg ? ' on' : ''}`} aria-pressed={perMsg} aria-label="Analisa per pesan" onClick={() => setPerMsg(!perMsg)}
              title={t.analyzed_at ? `Analisa per pesan · terakhir dianalisa ${shortDate(t.analyzed_at)} ${hhmm(t.analyzed_at)}` : 'Analisa per pesan · belum pernah dianalisa — tekan Analisa lanjut'}>
              <Icon name="spark" /><span className="ibc-an-l">Per pesan</span>
            </button>
          )}
          {readable && <button className="btn ibc-ai" disabled={orch.running} onClick={() => reanalyze(t.dealer_id ? `dealer:${t.dealer_id}` : `thread:${t.id}`, 'mcp')} title="Claude (MCP) menganalisa percakapan ini — catatan per pesan muncul di bawah tiap bubble"><span className="ai" style={{ fontSize: 0 }} />{orch.running ? 'Menganalisis…' : 'Analisa lanjut'}</button>}
          {t.kind !== 'group' && <button className="btn ibc-acc" onClick={onAccurate}><Icon name="send" />Accurate</button>}
          {!lead && <button className="ibl-btn ibc-lead-btn" onClick={() => setLead(true)} title="Tampilkan panel lead" aria-label="Tampilkan panel lead"><Icon name="doc" /></button>}
        </div>
      </div>
      {searching && <ChatSearch thread={id} onJump={jumpToHit} onQuery={setMark} onClose={() => { setSearching(false); setMark('') }} />}
      <div className="th-body ibc-body" ref={body} onScroll={onScroll}>
        <div className="ibc-start">
          {more
            ? <span className="ibc-pill dark"><Icon name="refresh" />{loading ? 'Memuat pesan sebelumnya…' : 'Gulir ke atas untuk pesan sebelumnya'}</span>
            : <span className="ibc-pill" title="Pesan yang lebih lama tidak ada di HP atau sudah lewat masa simpan">{data.history_from ? `Riwayat sejak ${shortDate(data.history_from)}` : 'Awal percakapan'} · chat disimpan {data.history_days || 365} hari</span>}
        </div>
        {messages.map((m, i) => {
          const sep = i === 0 || dayKey(messages[i - 1].sent_at) !== dayKey(m.sent_at) ? <div className="day">{dayLabel(m.sent_at, now)}</div> : null
          return (
            <div key={m.id} style={{ display: 'contents' }}>
              {sep}
              <Bubble m={m} via={from} group={t.kind === 'group'} analysis={perMsg} thread={id} now={now} onEdit={startEdit} onReply={startReply} onDelete={deleteMsg} onJump={jumpTo} onZoom={setZoom} onUse={useSuggestion} asOf={data?.stock_as_of} mark={mark} />
            </div>
          )
        })}
      </div>
      <div className="th-c ibc-c">
        {replying && (
          <div className="ibc-editing reply">
            <ReplyIcon />
            <span><b>Membalas {replying.direction === 'out' ? from || 'Anda' : replying.from_name || t.title}</b><small>{mediaCaption(replying.body, replying.media) || (replying.media ? mediaName(replying.media) : '')}</small></span>
            <button type="button" onClick={() => setReplying(null)} aria-label="Batal balas"><Icon name="x" /></button>
          </div>
        )}
        {editing && (
          <div className="ibc-editing">
            <Icon name="edit" />
            <span><b>Edit pesan</b><small>{editing.body}</small></span>
            <button type="button" onClick={cancelEdit} aria-label="Batal edit"><Icon name="x" /></button>
          </div>
        )}
        {file && <FilePreview file={file} onRemove={() => setFile(null)} />}
        {voice.recording ? (
          <RecordingBar secs={voice.secs} onCancel={voice.cancel} onSend={voice.send} />
        ) : (
          <form onSubmit={submit}>
            {!editing && <AttachMenu onFile={pickFile} onPoll={openPoll} disabled={upload.isPending} />}
            <textarea ref={input} rows={1} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={composerKey}
              placeholder={editing ? 'Ubah pesan…' : file ? 'Tambahkan keterangan…' : 'Tulis balasan… (Shift+Enter: baris baru)'} autoComplete="off" aria-label={editing ? 'Ubah pesan' : file ? 'Keterangan file' : 'Tulis balasan'} />
            {text.trim() || file || editing
              ? <button type="submit" className="send" aria-label={editing ? 'Simpan edit' : 'Kirim'} disabled={send.isPending || edit.isPending || upload.isPending}><Icon name={editing ? 'check' : 'send'} /></button>
              : <button type="button" className="send" onClick={voice.start} aria-label="Rekam pesan suara" title="Rekam pesan suara"><Mic /></button>}
          </form>
        )}
        <span className="pol"><Icon name="lock" />Dikirim dari {from} ({t.account_masked}) · jeda & "mengetik…" seperti manusia · penjaga anti-blokir</span>
      </div>
      {zoom && <Lightbox url={zoom} onClose={() => setZoom(null)} />}
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
/** The tabs the slim (ringkas) lead panel offers as icons; a click opens the panel on that tab. */
const MINI_TABS: [LeadTab, string, string][] = [['ai', 'spark', 'Analisa Claude'], ['src', 'net', 'Sumber'], ['accurate', 'doc', 'Data Accurate'], ['stats', 'chart', 'Statistik']]

/** A group with customers is read by Analisa WhatsApp and Claude; an internal group (stock, schedules, tasks) never
 * is. GSI Orbit marks a group with a customer of the master by itself; a person marks the others here. */
function GroupKind({ id, customer }: { id: string; customer: boolean }) {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const set = useMutation({
    mutationFn: (kind: 'internal' | 'external') => api.patch(`/wa/groups/${id}`, { kind, read_enabled: true }),
    onSuccess: (_, kind) => {
      qc.invalidateQueries({ queryKey: ['chat'] })
      qc.invalidateQueries({ queryKey: ['wa', 'groups'] })
      toast(kind === 'external' ? 'Ditandai grup pelanggan · ikut dibaca Analisa WhatsApp' : 'Ditandai grup internal · tidak dibaca analisa')
    },
    onError: (e: Error) => toast(e.message),
  })
  return customer ? (
    <div className="ibp-grp">
      <b><Icon name="people" />Grup pelanggan</b>
      <span>Dibaca Analisa WhatsApp dan Claude (Analisa lanjut) seperti chat pelanggan; pesan karyawan GSI di grup ini dihitung sebagai balasan.</span>
      <button type="button" className="btn quiet" disabled={set.isPending} onClick={() => set.mutate('internal')}>Ini grup internal</button>
    </div>
  ) : (
    <div className="ibp-grp internal">
      <b><Icon name="lock" />Grup internal</b>
      <span>Tidak dibaca analisa (hanya stok, jadwal, tugas). Ada pelanggan di grup ini? Tandai agar Claude ikut membacanya.</span>
      <button type="button" className="btn primary" disabled={set.isPending} onClick={() => set.mutate('external')}><Icon name="check" />Tandai grup pelanggan</button>
    </div>
  )
}

/** Who the number is (ADR 0035): a customer, a supplier (kept, but no lead and out of "Belum dibalas"), or a personal
 * chat — removed from the CRM on every GSI number, later messages not kept. */
function ContactKind({ id, supplier }: { id: string; supplier: boolean }) {
  const qc = useQueryClient()
  const nav = useNavigate()
  const { toast } = useFeedback()
  const set = useMutation({
    mutationFn: (type: 'customer' | 'supplier' | 'personal') => api.put<{ message: string; removed: boolean }>(`/chat/threads/${id}/contact-type`, { type }),
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast(r.message)
      if (r.removed) nav('/chat')
    },
    onError: (e: Error) => toast(e.message),
  })
  const personal = () => {
    if (window.confirm('Tandai obrolan pribadi? Seluruh chat nomor ini dihapus dari CRM (WhatsApp di HP tidak berubah) dan pesan berikutnya tidak disimpan.')) set.mutate('personal')
  }
  return (
    <div className="ibp-grp internal">
      <b><Icon name="people" />{supplier ? 'Pemasok' : 'Tipe kontak'}</b>
      <span>{supplier ? 'Bukan pelanggan: tidak masuk antrean "Belum dibalas" dan tidak dihitung sebagai lead.' : 'Bukan pelanggan? Tandai pemasok (tetap tersimpan) atau obrolan pribadi (dihapus dari CRM).'}</span>
      <div className="ibp-row">
        {supplier
          ? <button type="button" className="btn quiet" disabled={set.isPending} onClick={() => set.mutate('customer')}>Ini pelanggan</button>
          : <button type="button" className="btn quiet" disabled={set.isPending} onClick={() => set.mutate('supplier')}>Pemasok</button>}
        <button type="button" className="btn quiet" disabled={set.isPending} onClick={personal}>Obrolan pribadi</button>
      </div>
    </div>
  )
}

function LeadPane({ id, mini, setMini, onAccurate }: { id: string; mini: boolean; setMini: (v: boolean) => void; onAccurate: () => void }) {
  const { data: ctx } = useChatContext(id)
  const { data: th } = useThread(id)
  const { data: wa } = useWAStatus()
  const { data: list = [] } = useThreads('all')
  const qc = useQueryClient()
  const nav = useNavigate()
  const { openSheet, toast } = useFeedback()
  const [tab, setTab] = useState<LeadTab>('main')
  if (!ctx || !th) return <div className={`pane ibp${mini ? ' mini' : ''}`} />
  const t = th.thread
  const tv = list.find((x) => x.id === id)
  const d = ctx.dealer
  const idf = ctx.identification
  const group = t.kind === 'group'
  const customerGroup = (t.group_kind ?? tv?.group_kind) === 'external'
  const groupLabel = customerGroup ? 'Grup pelanggan' : 'Grup internal'
  const kindLabel = t.kind === 'dealer' ? 'Dealer' : group ? groupLabel : 'Lead'
  const stage = d ? d.metrics.status : t.kind === 'new' ? (idf ? 'Teridentifikasi' : 'Lead baru') : groupLabel
  const tone = d ? STATUS_TONE[d.metrics.status] ?? 'accent' : t.kind === 'new' ? 'accent' : 'neutral'
  const filled = d ? Math.max(1, Math.round((d.metrics.score / 100) * 7)) : t.kind === 'new' ? (idf ? Math.max(2, Math.round((idf.score / 100) * 7)) : 1) : 0
  const num = wa?.items.find((n) => n.wa_number === tv?.account)
  // only a proposal (written by Claude through MCP, ADR 0032) — no rule or template text
  const recommendation = d?.next?.title ?? null
  const warn = d
    ? d.metrics.credit.state === 'over limit' || d.metrics.credit.state === 'overdue'
      ? { h: `Kredit ${d.metrics.credit.state}`, t: `Exposure ${fmtRp(d.metrics.credit.exposure)} dari limit ${fmtRp(d.credit_limit)}${d.metrics.credit.late_days ? ` · telat ${d.metrics.credit.late_days} hari` : ''}. Order baru perlu persetujuan kredit.` }
      : d.metrics.status === 'At risk' || d.metrics.status === 'Churn'
        ? { h: `Dealer ${d.metrics.status}`, t: `Order terakhir ${d.metrics.last_order_days ?? '—'} hari lalu (ritme ${d.metrics.rhythm_days ?? '—'} hari).` }
        : null
    : t.kind === 'new'
      ? { h: 'Belum ada di master pelanggan', t: 'Nomor ini belum ditemukan di data pelanggan Accurate.' }
      : null
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['chat', 'context', id] })
    qc.invalidateQueries({ queryKey: ['chat', 'thread', id] })
    toast('Panel lead diperbarui')
  }
  // Claude's notes per customer bubble (Analisa WhatsApp through MCP)
  const notes = th.messages.filter((m) => m.direction === 'in' && m.analysis?.note)
  const claudeNotes = (
    <ul className="ibp-ext">
      {notes.length === 0 && <li className="muted">Belum ada catatan Claude — tekan Analisa lanjut</li>}
      {notes.slice(-12).map((m) => (
        <li key={m.id}><span className="ai" style={{ fontSize: 0 }} /><span>{m.analysis!.note}<small>{m.from_name} · {hhmm(m.sent_at)}</small></span></li>
      ))}
    </ul>
  )
  const ins = th.messages.filter((m) => m.direction === 'in').length
  const name = d?.name ?? idf?.best_name ?? t.title
  if (mini) {
    const open = (k: LeadTab) => {
      setTab(k)
      setMini(false)
    }
    return (
      <div className="pane ibp mini" aria-label={`Panel ${kindLabel} (ringkas)`}>
        <button type="button" className="ibp-mini-open" onClick={() => setMini(false)} title="Perbesar panel" aria-label="Perbesar panel"><Icon name="chev" /></button>
        <button type="button" className={`ibp-mini-av ${tone}`} onClick={() => open('main')} title={`${name} · ${stage}`} aria-label={`${kindLabel} ${name}`}>
          {group ? <Icon name="people" /> : initials(t)}
        </button>
        <em className={`ibp-mini-st ${tone}`} title={stage}>{stage}</em>
        {warn && <button type="button" className="ibp-mini-ic warn" onClick={() => open('main')} title={warn.h} aria-label={warn.h}><Icon name="alert" /></button>}
        <span className="ibp-mini-sep" />
        {MINI_TABS.map(([k, icon, label]) => <button key={k} type="button" className="ibp-mini-ic" onClick={() => open(k)} title={label} aria-label={label}><Icon name={icon} /></button>)}
        {!group && <button type="button" className="ibp-mini-ic acc" onClick={onAccurate} title="Kirim ke Accurate" aria-label="Kirim ke Accurate"><Icon name="send" /></button>}
      </div>
    )
  }
  return (
    <div className="pane ibp">
      <div className="ibp-dark">
        <div className="ibp-bar">
          <Icon name={t.kind === 'dealer' ? 'building' : group ? 'people' : 'user-x'} />
          <b>{kindLabel}</b><span>· Claude (MCP)</span>
          <span className="sp" />
          {d && <button type="button" onClick={() => nav('/dealer/' + d.id)} title="Buka halaman dealer" aria-label="Buka halaman dealer"><Icon name="arrow" /></button>}
          <button type="button" onClick={refresh} title="Muat ulang" aria-label="Muat ulang panel"><Icon name="refresh" /></button>
          <button type="button" className="ibp-shrink" onClick={() => setMini(true)} title="Ringkas panel" aria-label="Ringkas panel"><Icon name="chev" /></button>
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
              <div><b>{displayTitle(t)}</b><span>{d ? `Dealer tier ${d.tier} · ${d.city}` : group ? groupLabel : 'Pelanggan WhatsApp'}</span></div>
            </div>
            <Kv rows={[
              ['Telepon', t.phone ? fmtPhone(t.phone) : null],
              ['Tahap', stage],
              ['Sumber', t.subtitle ? `≈ ${t.subtitle}` : null],
              ['Sales', t.sales || null],
            ]} />
            {warn && <div className="ibp-warn"><b>{warn.h}</b><span>{warn.t}</span></div>}
            {recommendation && <div className="ibp-rec"><b><span className="ai" style={{ fontSize: 0 }} />USULAN CLAUDE (MCP)</b><span>{recommendation}</span></div>}
            {d?.next && <ActBtn next={d.next} />}
            {t.kind === 'new' && (
              <div className="ibp-row">
                <ActBtn next={ctx.proposals?.new_dealer} label="Buat dealer tier C" icon="building" small />
                <ActBtn next={ctx.proposals?.price_list} label="Kirim harga" icon="send" ghost small />
              </div>
            )}
            <div className="ibp-grid">
              <button type="button" onClick={() => setTab('ai')}>Analisa Claude <Icon name="arrow" /></button>
              <button type="button" onClick={() => setTab('src')}>Asal lead <Icon name="arrow" /></button>
              <button type="button" onClick={() => setTab('accurate')}>Data Accurate <Icon name="arrow" /></button>
              {d && <button type="button" onClick={() => nav('/dealer/' + d.id)}>Halaman dealer <Icon name="arrow" /></button>}
            </div>
            {!group && <button type="button" className="ibp-send" onClick={onAccurate}><Icon name="send" />Kirim ke Accurate</button>}
            {group && t.group_id && <GroupKind id={t.group_id} customer={customerGroup} />}
            {!group && <ContactKind id={id} supplier={t.contact_type === 'supplier'} />}
          </>
        )}
        {tab === 'ai' && (
          <>
            <h4>Catatan Claude per pesan <span className="ai" style={{ marginLeft: 4, fontSize: 0 }} /></h4>
            {claudeNotes}
            {d?.next && <><h4>Usulan Claude</h4><div className="ibp-rec"><b>{d.next.agent}</b><span>{d.next.title}{d.next.why ? ` — ${d.next.why}` : ''}</span></div><ActBtn next={d.next} /></>}
            <p className="ibp-note">Analisa di panel ini hanya yang ditulis Claude lewat MCP (Analisa lanjut).</p>
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
            {!idf && t.kind === 'new' && <p className="ibp-note">Nomor ini diidentifikasi setelah ia mengirim pesan.</p>}
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
                ['Share of wallet', sowText(d.metrics)],
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
  const notes = th.messages.filter((m) => m.direction === 'in' && m.analysis?.note).slice(-3).map((m) => m.analysis!.note!)
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
  const firstShown = (list ?? []).find((t) => inFilter(t, filter))
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
    <div className={`chat multi komo ${threadId ? 'show-thread' : ''} ${active ? 'with-lead' : ''} ${active && !lead ? 'lead-mini' : ''}`}>
      {rail}
      <ThreadList list={list ?? []} loaded={isSuccess} filter={filter} setFilter={setFilter} account={account} active={active} onPick={(id) => nav('/chat/' + id)} />
      {active && <LeadPane key={'l' + active} id={active} mini={!lead} setMini={(v) => setLead(!v)} onAccurate={accurate} />}
      {active ? <ThreadPane key={active} id={active} picked={!!threadId} onBack={() => nav('/chat')} lead={lead} setLead={setLead} onAccurate={accurate} /> : <div className="pane th-pane" />}
    </div>
  )
}
