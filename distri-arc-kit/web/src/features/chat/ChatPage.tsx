import { memo, useCallback, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate, useOutletContext, useParams, useSearchParams } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Items } from '../../api/client'
import type { MediaView, Mention, MessageView, QuickReply, ThreadDetail, ThreadView, WANumber, WATeam } from '../../api/types'
import { Icon } from '../../components/Icon'
import { ActBtn, ProposalSheet } from '../../components/actions'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtRp, hhmm, shortDate, wib, sowText } from '../../lib/format'
import { useChatContext, useNow, useThread, useThreads, useWAStatus } from '../../app/queries'
import { ConnectPanel, holderOf, labelOf, NumberRail, NumberSheet, PairSheet, phoneOf, teamName } from './Connect'
import { countFilters, displayTitle, FILTERS, fmtPhone, inFilter, lookupLinks, matchThread, searchKey, waitLabel, type Filter } from './inbox'
import { AttachMenu, ContactSheet, FilePreview, MAX_FILE, Mic, PollSheet, RecordingBar, useVoiceRecorder } from './Composer'
import { bubbleLine, callLine, contactCard, deletableNow, editableNow, fmtDuration, fmtSize, forwardable, mediaCaption, mediaName } from './bubble'
import { ForwardSheet } from './Forward'
import { ChatSearch, Marked } from './ChatSearch'
import { LabelChips, LabelSheet, PinnedBar, useChatLabels } from './Labels'
import { hasLabel, pinnedFirst } from './chatLabels'
import { ChatRowMenu } from './ChatRowMenu'
import { pastedImage } from './paste'
import { TriggerCard, type DraftAI } from '../aitrigger/TriggerCard'
import type { ChatHit } from './findInChat'
import { insertMention, matchMembers, mentionParts, mentionQuery, toWire, type Member } from './mentions'
import { insertQuick, matchQuick, slashQuery } from './quickReplyText'
import { countQuickUse, QuickReplySheet, useQuickReplies } from './QuickReplies'
import { linkParts } from './links'
import { formatParts, quoteLines, shortcutMark, wrapSelection, type Mark } from './format'
import { MembersSheet, openMemberChat, useMembers } from './Members'
import { ListResizer, listWidthStyle, savedListWidth } from './ListResizer'
import { NumberLocked } from './NumberDownBanner'
import { getDraft, setDraft, useDrafts } from './drafts'
import { useSse } from '../../app/SseProvider'
import { AIOverview } from './AIOverview'
import { ChatStats } from './ChatStats'
import { Dock } from '../../app/Dock'
import { OrchPill } from '../../app/orch'
import type { ShellContext } from '../../app/Shell'

/** Plain text with its tags ("@Budi", WhatsApp's colour), its links (open in a new tab) and the searched words marked. */
function RichText({ text, mentions, mark }: { text: string; mentions?: Mention[] | null; mark?: string }) {
  const plain = (t: string, key: string | number) => (mark ? <Marked key={key} text={t} q={mark} /> : <span key={key}>{t}</span>)
  return <>{mentionParts(text, mentions).map((p, i) => (p.tag
    ? <span key={i} className="bub-tag">{p.t}</span>
    : linkParts(p.t).map((l, k) => (l.href
      ? <a key={`${i}-${k}`} className="bub-link" href={l.href} target="_blank" rel="noopener noreferrer" onClick={(e) => e.stopPropagation()}>{mark ? <Marked text={l.t} q={mark} /> : l.t}</a>
      : plain(l.t, `${i}-${k}`)))))}</>
}

/** One line's WhatsApp formatting (*bold*, _italic_, ~strikethrough~, ```monospace```) around its rich text. */
function FormattedLine({ text, mentions, mark }: { text: string; mentions?: Mention[] | null; mark?: string }) {
  return <>{formatParts(text).map((f, i) => {
    let el: ReactNode = f.m ? (mark ? <Marked text={f.t} q={mark} /> : f.t) : <RichText text={f.t} mentions={mentions} mark={mark} />
    if (f.m) el = <code className="bub-mono">{el}</code>
    if (f.s) el = <s>{el}</s>
    if (f.i) el = <em>{el}</em>
    if (f.b) el = <strong>{el}</strong>
    return <span key={i}>{el}</span>
  })}</>
}

/** A bubble's text as WhatsApp shows it: formatting, "> " quote lines, tags, links, and the searched words marked. */
function BubbleText({ text, mentions, mark }: { text: string; mentions?: Mention[] | null; mark?: string }) {
  const lines = quoteLines(text)
  if (!lines.some((l) => l.quote)) return <FormattedLine text={text} mentions={mentions} mark={mark} />
  return <>{lines.map((l, i) => (l.quote
    ? <span key={i} className="bub-q"><FormattedLine text={l.t} mentions={mentions} mark={mark} /></span>
    : <span key={i}><FormattedLine text={l.t} mentions={mentions} mark={mark} />{i < lines.length - 1 && !lines[i + 1].quote ? '\n' : ''}</span>))}</>
}


function initials(t: Pick<ThreadView, 'kind' | 'title'>) {
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
  all: 'chat', unread: 'bell', unreplied: 'send', starred: 'star', dealer: 'building', group_internal: 'people', group_external: 'net', new: 'spark', internal: 'lock', supplier: 'box',
}

// the short chat list (one line of name, one of message), remembered in this browser
const COMPACT_KEY = 'chat.compact'
function readCompact(): boolean {
  try { return localStorage.getItem(COMPACT_KEY) === 'on' } catch { return false }
}

// the slim chat header (Sam 22 Oct): one short line — name and the action icons — so the messages get the room
const HEAD_KEY = 'chat.head-compact'
function useCompactHead(): [boolean, () => void] {
  const [on, setOn] = useState(() => {
    try { return localStorage.getItem(HEAD_KEY) === 'on' } catch { return false }
  })
  const toggle = useCallback(() => setOn((v) => {
    try { localStorage.setItem(HEAD_KEY, v ? 'off' : 'on') } catch { /* not remembered */ }
    return !v
  }), [])
  return [on, toggle]
}

/** A message found by Cari chat in any conversation (GET /chat/messages/search). */
type MessageHit = ChatHit & { thread_id: string; kind: ThreadView['kind']; group_kind?: string; title: string; phone_name?: string }

function ThreadList({ list, loaded, filter, setFilter, account, active, onPick, onHit, onLeave }: {
  list: ThreadView[]; loaded: boolean; filter: Filter; setFilter: (f: Filter) => void; account: string; active?: string; onPick: (id: string) => void
  onHit: (h: MessageHit) => void; onLeave: () => void
}) {
  const [menu, setMenu] = useState<string | null>(null)
  const drafts = useDrafts() // a chat with an unsent draft shows "Draf: …" (WhatsApp)
  const { stale } = useSse() // the realtime stream down: say so (the chats poll meanwhile)
  const [compact, setCompact] = useState(readCompact)
  const toggleCompact = useCallback(() => setCompact((v) => {
    try { localStorage.setItem(COMPACT_KEY, v ? 'off' : 'on') } catch { /* not remembered */ }
    return !v
  }), [])
  // Alt+R anywhere on Chat (outside a text box): the short list (Sam 17 Oct)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.altKey || e.ctrlKey || e.metaKey || e.code !== 'KeyR') return
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
      e.preventDefault()
      toggleCompact()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [toggleCompact])
  const { data: wa } = useWAStatus()
  const now = useNow()
  const qc = useQueryClient()
  const { openSheet, toast } = useFeedback()
  const [q, setQ] = useState('')
  const [label, setLabel] = useState('')
  const { data: labels } = useChatLabels()
  const n = wa?.items.find((x) => x.wa_number === account)
  const team = account.startsWith('team:') ? (account.slice(5) as WATeam) : undefined
  const nums = (wa?.items ?? []).filter((x) => !team || (x.team ?? 'sales') === team)
  // Pesan berbintang (Sam 22 Oct): the person's starred messages of this number or team — the "Berbintang" filter
  const { data: starred = [] } = useQuery({
    queryKey: ['chat', 'starred', account],
    staleTime: 15_000,
    queryFn: () => {
      const p = new URLSearchParams()
      if (team) p.set('team', team)
      else if (account) p.set('account', account)
      return api.get<Items<MessageHit>>('/chat/starred?' + p.toString()).then((r) => r.items)
    },
  })
  const counts = useMemo(() => ({ ...countFilters(list), starred: starred.length }), [list, starred.length])
  // the list holds the chat window (14 days); a search of 2+ letters asks the server across every stored
  // conversation, by name (spellings forgiven) or number — and, like WhatsApp, the words of every stored message
  // (the "Pesan" results under the chats, Sam 22 Oct)
  const [dq, setDq] = useState('')
  useEffect(() => {
    const h = setTimeout(() => setDq(q.trim()), 350)
    return () => clearTimeout(h)
  }, [q])
  const deep = dq.length >= 2
  // each letter typed is a new search: the previous results of this number stay on screen until the new ones arrive,
  // so the list does not empty and refill (jump up and down) while typing
  const sameAccount = <T,>(prev: T | undefined, prevQuery?: { queryKey: readonly unknown[] }) => (prevQuery?.queryKey[3] === account ? prev : undefined)
  const { data: found, isFetching: searching } = useQuery({
    queryKey: ['chat', 'search', dq, account],
    enabled: deep,
    staleTime: 30_000,
    placeholderData: sameAccount,
    queryFn: () => {
      const p = new URLSearchParams({ all: '1', q: dq })
      if (team) p.set('team', team)
      else if (account) p.set('account', account)
      return api.get<Items<ThreadView>>('/chat/threads?' + p.toString()).then((r) => r.items)
    },
  })
  const { data: hits, isFetching: searchingMsgs } = useQuery({
    queryKey: ['chat', 'search-messages', dq, account],
    enabled: deep,
    staleTime: 30_000,
    placeholderData: sameAccount,
    queryFn: () => {
      const p = new URLSearchParams({ q: dq })
      if (team) p.set('team', team)
      else if (account) p.set('account', account)
      return api.get<Items<MessageHit>>('/chat/messages/search?' + p.toString()).then((r) => r.items)
    },
  })
  // "Mencari…" sits in the search box, never above the list, so it cannot push the chats down and back up
  const busy = deep && (searching || searchingMsgs)
  const firstSearch = deep && ((searching && !found) || (searchingMsgs && !hits))
  const starring = filter === 'starred'
  // the list filters on a deferred copy of what is typed: the letters show at once, the (long) list follows
  const lq = useDeferredValue(q)
  const starKey = searchKey(lq)
  const msgs = starring
    ? starred.filter((h) => !starKey || searchKey(`${displayTitle(h)} ${h.from_name} ${h.snippet}`).includes(starKey))
    : deep && lq.trim() ? hits ?? [] : []
  const base = deep && found ? found : list
  // the open conversation stays in "Belum dibaca" / "Belum dibalas" after reading or answering it, as in WhatsApp
  const shown = useMemo(() => pinnedFirst(base).filter((t) => (inFilter(t, filter) || ((filter === 'unread' || filter === 'unreplied') && t.id === active)) && (deep && found ? true : matchThread(t, lq)) && hasLabel(t, label)), [base, filter, active, deep, found, lq, label])
  // the rows are kept while typing in the search box and redrawn only when the (deferred) results change
  const nowMs = now.getTime()
  const rows = useMemo(() => {
    const now = new Date(nowMs)
    return shown.map((c) => (
      <div key={c.id} className={`ibr-w${menu === c.id ? ' menu-open' : ''}`} onContextMenu={(e) => { e.preventDefault(); setMenu(c.id) }}>
      <button className={`ibr ${c.id === active ? 'is-active' : ''}`} onClick={() => onPick(c.id)}>
        <span className={`av ${c.kind === 'group' ? 'grp' : ''}`}>{c.kind === 'group' ? <Icon name="people" /> : initials(c)}</span>
        <div className="ibr-m">
          <div className="ibr-top">
            <b>{displayTitle(c)}</b>
            {c.kind === 'group' && <span className="ibr-tag grp">{c.group_kind === 'external' ? 'GRUP PELANGGAN' : 'GRUP'}</span>}
            {c.kind === 'internal' && <span className="ibr-tag grp" title="Chat antar karyawan GSI · tidak masuk analisa">INTERNAL</span>}
            {c.contact_type === 'supplier' && <span className="ibr-tag grp">PEMASOK</span>}
            <Waiting since={c.waiting_since} now={now} />
          </div>
          {c.kind !== 'group' && c.subtitle && <span className="ibr-src">≈ {c.subtitle}</span>}
          <LabelChips ids={c.labels} className="ibr-lbls" />
          {drafts[c.id] && c.id !== active
            ? <span className="ibr-lm"><b className="ibr-draft">Draf:</b> {drafts[c.id].replace(/\s+/g, ' ')}</span>
            : <span className="ibr-lm">{c.last_direction === 'out' ? 'Anda: ' : c.last_from ? c.last_from + ': ' : ''}{c.last_body}</span>}
        </div>
        <div className="ibr-rt">
          <span>{timeLabel(c.last_message_at, now)}</span>
          <span className="ibr-marks">
            {c.pinned_at && <Icon name="pin" className="i ibr-pin" />}
            {!!c.mentioned && <span className="un at" title="Anda ditandai di chat ini">@</span>}
            {c.unread > 0 && <span className="un">{c.unread > 99 ? '99+' : c.unread}</span>}
          </span>
          {!account && <span className="via">via {(c.account_label || c.sales).replace(/^Nomor\s+/, '')}</span>}
        </div>
      </button>
      <ChatRowMenu t={c} open={menu === c.id} setOpen={(v) => setMenu(v ? c.id : null)} onLeave={() => { if (c.id === active) onLeave() }} />
      </div>
    ))
  }, [shown, menu, active, drafts, nowMs, account, onPick, onLeave])
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['chat'] })
    qc.invalidateQueries({ queryKey: ['wa'] })
    toast('Daftar chat diperbarui')
  }
  return (
    <div className={`pane list-pane ibl${compact ? ' compact' : ''}`}>
      <div className="ibl-head">
        <button type="button" className="ibl-acc" onClick={() => n && openSheet(<NumberSheet wa={n.wa_number} />)} disabled={!n} title={n ? 'Detail nomor' : undefined}>
          <span className="ibl-ic"><Icon name="chat" /></span>
          <span className="ibl-who">
            <b>{n ? `${labelOf(n)}${n.user_role || n.sales_id ? ` - ${n.user_role || 'Sales'}` : ''}` : team ? `Tim ${teamName(team)}` : 'Semua nomor'}</b>
            <span>{n ? `${phoneOf(n)} · Tim ${teamName(n.team)}` : `${nums.length} nomor · ${nums.filter((x) => x.state === 'connected').length} terhubung`}</span>
          </span>
          {n && <Icon name="chev" className="i ibl-chev" />}
        </button>
        <button type="button" className={`ibl-btn${compact ? ' on' : ''}`} onClick={toggleCompact} aria-pressed={compact}
          title={compact ? 'Tampilan lengkap (Alt+R)' : 'Ringkas tampilan (Alt+R)'} aria-label="Ringkas tampilan"><Icon name="kanban" /></button>
        <button type="button" className="ibl-btn" onClick={refresh} title="Muat ulang" aria-label="Muat ulang"><Icon name="refresh" /></button>
      </div>
      <div className="ibl-search"><Icon name="search" /><input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Cari chat" aria-label="Cari chat" />{busy && !starring && <span className="ibl-busy" role="status">Mencari…</span>}</div>
      {stale && <div className="ibl-live" role="status"><i />Menyambung ulang ke server… chat tetap diperbarui tiap 10 detik</div>}
      {n && <NumberDown n={n} />}
      <div className="ibl-chips" role="tablist">
        {FILTERS.map(([k, l]) => (
          <button key={k} role="tab" aria-selected={filter === k} aria-label={l} title={l} className={filter === k ? 'is-active' : ''} onClick={() => setFilter(k)}>
            <Icon name={FILTER_ICONS[k]} />{filter === k && <em>{l}</em>}<span>{counts[k].toLocaleString('id-ID')}</span>
          </button>
        ))}
        {!!labels?.items.length && (
          <select className={`ibl-lbl${label ? ' on' : ''}`} value={label} onChange={(e) => setLabel(e.target.value)} aria-label="Filter label" title="Filter label">
            <option value="">Label: semua</option>
            {labels.items.map((l) => <option key={l.id} value={l.id}>{l.name} ({l.chats})</option>)}
          </select>
        )}
      </div>
      <div className="chat-list ibl-list">
        {starring && msgs.length === 0 && <div className="chat-none">{q ? `Tidak ada pesan berbintang yang cocok dengan "${q}"` : 'Belum ada pesan berbintang. Beri bintang lewat menu ⌄ di bubble pesan — hanya Anda yang melihatnya.'}</div>}
        {msgs.length > 0 && shown.length > 0 && <div className="ibl-sec">Chat</div>}
        {loaded && !starring && shown.length === 0 && msgs.length === 0 && !firstSearch && <div className="chat-none">{q ? `Tidak ada chat yang cocok dengan "${q}"${deep ? ' di semua chat Orbit — chat lama di HP yang belum pernah berbalas pesan sejak nomor ditautkan belum ada di sini' : ''}` : filter === 'unread' ? 'Semua chat sudah dibaca' : filter === 'unreplied' ? 'Semua chat pelanggan sudah dibalas' : 'Belum ada percakapan di sini'}</div>}
        {rows}
        {msgs.length > 0 && <div className="ibl-sec">{starring ? 'Pesan berbintang' : 'Pesan'}</div>}
        {msgs.map((h) => (
          <button key={h.id} className="ibr ibr-hit" onClick={() => onHit(h)} title="Buka chat di pesan ini">
            <span className={`av ${h.kind === 'group' ? 'grp' : ''}`}>{h.kind === 'group' ? <Icon name="people" /> : initials(h)}</span>
            <div className="ibr-m">
              <div className="ibr-top"><b>{displayTitle(h)}</b></div>
              <span className="ibr-lm"><Marked text={(h.direction === 'out' ? 'Anda: ' : h.kind === 'group' && h.from_name ? h.from_name + ': ' : '') + h.snippet} q={starring ? q.trim() : dq} /></span>
            </div>
            <div className="ibr-rt"><span>{timeLabel(h.sent_at, now)}</span>{starring && <Icon name="star" className="i ibr-star" />}</div>
          </button>
        ))}
      </div>
      <footer className="ibl-footer">CRM GSI ORBIT</footer>
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
function BubbleMenu({ m, edit, del, onReply, onEdit, onDelete, onForward, onPin, onStar, onLabel }: {
  m: MessageView; edit: boolean; del: boolean; onReply: () => void; onEdit: () => void; onDelete: () => void; onForward?: () => void
  onPin?: () => void; onStar?: () => void; onLabel?: () => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLSpanElement>(null)
  const { toast } = useFeedback()
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.preventDefault(); setOpen(false) } }
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
          {onStar && <button type="button" role="menuitem" onClick={pick(onStar)}><Icon name="star" />{m.starred ? 'Hapus bintang' : 'Beri bintang'}</button>}
          {onPin && <button type="button" role="menuitem" onClick={pick(onPin)}><Icon name="pin" />{m.pinned ? 'Lepas sematan' : 'Sematkan'}</button>}
          {onLabel && <button type="button" role="menuitem" onClick={pick(onLabel)}><Icon name="tag" />Label</button>}
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
/** A WhatsApp call (ADR 0062): the phone rang and talked; GSI Orbit records how it ended. */
function CallRow({ md }: { md: MediaView }) {
  const l = callLine(md)
  return <div className={`bub-call ${l.tone}`} title={l.tone === 'bad' ? 'Pelanggan menelepon dan belum terlayani — telepon balik atau balas chat' : 'Panggilan diangkat di HP pemegang nomor'}><Icon name="phone" />{l.text}</div>
}

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

/** The AI Trigger cards under a customer's bubble (ADR 0038, 0041, 0051): the draft written (or rewritten) with ChatGPT or Claude. */
function TriggerCards({ m, thread, asOf, onUse }: { m: MessageView; thread: string; asOf?: string | null; onUse: (m: MessageView, text: string) => void }) {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const write = useMutation({
    mutationFn: (v: { rule: string; ai: DraftAI }) => api.post(`/chat/threads/${thread}/messages/${m.id}/ai-draft`, { rule_id: v.rule, provider: v.ai }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['chat', 'thread', thread] }),
    onError: (e: Error) => toast(e.message),
  })
  return <>{m.suggest!.map((s) => (
    <TriggerCard key={s.rule_id} s={s} asOf={asOf} onUse={(text) => onUse(m, text)} onAI={(ai) => write.mutate({ rule: s.rule_id, ai })} aiBusy={write.isPending && write.variables?.rule === s.rule_id ? write.variables.ai : null} />
  ))}</>
}

/** The first letter of the admin who sent a message: the small round chip on the label. */
export const senderInitial = (name: string) => (name.trim().replace(/^(Pak|Bu|Mbak|Mas)\s+/i, '')[0] ?? '?').toUpperCase()

function Bubble({ m, via, group, analysis, thread, now, onEdit, onReply, onDelete, onJump, onZoom, onUse, asOf, mark, onPin, onStar, onLabel, onMember }: {
  m: MessageView; via: string; group: boolean; analysis: boolean; thread: string; now: Date; mark?: string
  onPin?: (m: MessageView) => void; onStar?: (m: MessageView) => void; onLabel?: (m: MessageView) => void; onMember?: (number: string) => void
  onEdit: (m: MessageView) => void; onReply: (m: MessageView) => void; onDelete: (m: MessageView) => void; onJump: (id: string) => void; onZoom: (url: string) => void
  onUse?: (m: MessageView, text: string) => void; asOf?: string | null
}) {
  const { openSheet } = useFeedback()
  const caption = mediaCaption(m.body, m.media)
  const edit = editableNow(m, now)
  const del = deletableNow(m, now)
  const forward = forwardable(m)
  if (m.media?.kind === 'system') {
    // a group's own line (ADR 0063): centred like WhatsApp, nobody's bubble, nothing to answer
    return <div className="m sys" data-mid={m.id}><div className="bub-sys">{m.body}<span className="tm">{hhmm(m.sent_at)}</span></div></div>
  }
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
      {group && m.direction === 'in' && (
        <span className="who">
          {m.from_number && onMember
            ? <button type="button" className="who-link" onClick={() => onMember(m.from_number!)} title={`Chat dengan ${m.from_name || fmtPhone(m.from_number)} · ${fmtPhone(m.from_number)}`}>{m.from_name || fmtPhone(m.from_number)}</button>
            : m.from_name}
          <span className={`pill ${m.internal ? 'neutral' : 'accent'}`}>{m.internal ? 'internal' : 'eksternal'}</span>
        </span>
      )}
      {m.direction === 'out' && !m.internal && (m.sent_by || m.from_phone) && (
        <span className={`who out-by${m.sent_by ? '' : ' phone'}`} title={m.sent_by ? `Dikirim dari GSI Orbit oleh ${m.sent_by} (user yang login)` : 'Dikirim langsung dari HP pemegang nomor, bukan dari GSI Orbit'}>
          {m.sent_by ? <i className="out-by-av">{senderInitial(m.sent_by)}</i> : <Icon name="phone" />}{m.sent_by || 'dari HP'}
        </span>
      )}
      <div className="bb">
        {m.quote && <QuoteBlock q={m.quote} mine={via} onJump={onJump} />}
        {m.media?.kind === 'call' ? <CallRow md={m.media} /> : m.media && <BubbleMedia md={m.media} url={`/api/chat/threads/${thread}/messages/${m.id}/media`} pending={m.status === 'pending'} onZoom={onZoom} />}
        {m.media?.kind === 'call' ? null : m.media?.kind === 'poll' ? <PollBody text={caption} /> : m.media?.kind === 'contact' ? <ContactBody text={caption} /> : <BubbleText text={caption} mentions={m.mentions} mark={mark} />}
        <span className="tm">{m.starred && <Icon name="star" className="i tm-star" />}{m.pinned && <Icon name="pin" className="i tm-pin" />}{m.edited_at ? 'diedit · ' : ''}{m.direction === 'out' ? `${via}${m.status === 'sent' && m.signal_id == null ? ' · terkirim' : m.status === 'pending' ? (m.held ? ' · tertahan' : ' · mengirim…') : m.status === 'failed' ? ' · gagal' : ''} · ` : ''}{hhmm(m.sent_at)}</span>
        {m.status === 'pending' && m.held && <span className="bub-held" role="note"><Icon name="shield" />{m.held} · terkirim otomatis begitu diizinkan</span>}
        <BubbleMenu m={m} edit={edit} del={del} onReply={() => onReply(m)} onEdit={() => onEdit(m)} onDelete={() => onDelete(m)} onForward={forward ? () => openSheet(<ForwardSheet m={m} from={thread} />) : undefined}
          onPin={onPin && m.status !== 'pending' && m.status !== 'failed' ? () => onPin(m) : undefined} onStar={onStar ? () => onStar(m) : undefined} onLabel={onLabel ? () => onLabel(m) : undefined} />
        {m.can_reply && <span className="bub-acts"><button type="button" onClick={() => onReply(m)} title="Balas pesan ini" aria-label="Balas"><ReplyIcon /></button></span>}
      </div>
      <LabelChips ids={m.labels} className="bub-lbls" />
      {m.auto_reply && <span className="bub-hold" title="Pesan otomatis WhatsApp Business — bukan jawaban dan bukan permintaan">balasan otomatis · tidak dihitung</span>}
      {m.holding && !m.auto_reply && m.direction === 'out' && !(analysis && m.analysis) && (
        <span className="bub-hold" title="Pesan ini hanya meminta pelanggan menunggu — waktu respon tetap dihitung sampai pelanggan mendapat jawaban">jawaban sementara · waktu respon tetap berjalan</span>
      )}
      {m.suggest && m.suggest.length > 0 && onUse && <TriggerCards m={m} thread={thread} asOf={asOf} onUse={onUse} />}
      {m.proposal && <span className="ann"><span className="ai" />Usulan Claude · <AnnProposal p={m.proposal} /></span>}
      {analysis && m.analysis && <BubbleAnalysis a={m.analysis} dir={m.direction} />}
    </div>
  )
}

type BubbleActs = Pick<Parameters<typeof Bubble>[0], 'onEdit' | 'onReply' | 'onDelete' | 'onJump' | 'onZoom' | 'onUse' | 'onPin' | 'onStar' | 'onLabel' | 'onMember'>

/** The bubbles of a chat. Memoised with stable actions: typing in the reply box does not redraw a long chat (a
 * year of messages), which made typing lag on slower laptops. */
const MessageList = memo(function MessageList({ messages, nowMs, via, group, analysis, thread, asOf, mark, acts }: {
  messages: MessageView[]; nowMs: number; via: string; group: boolean; analysis: boolean; thread: string; asOf?: string | null; mark: string; acts: BubbleActs
}) {
  const now = useMemo(() => new Date(nowMs), [nowMs])
  return messages.map((m, i) => {
    const sep = i === 0 || dayKey(messages[i - 1].sent_at) !== dayKey(m.sent_at) ? <div className="day">{dayLabel(m.sent_at, now)}</div> : null
    return (
      <div key={m.id} style={{ display: 'contents' }}>
        {sep}
        <Bubble m={m} via={via} group={group} analysis={analysis} thread={thread} now={now} asOf={asOf} mark={mark} {...acts} />
      </div>
    )
  })
})

/** A contact card: the person's name and number, with a link to chat with the number on WhatsApp. */
function ContactBody({ text }: { text: string }) {
  const c = contactCard(text)
  const digits = c.phone.replace(/\D/g, '')
  return (
    <span className="bub-contact">
      <span className="bub-contact-av"><Icon name="people" /></span>
      <span className="bub-contact-m"><b>{c.name || 'Kontak'}</b>{c.phone && <small>{fmtPhone(digits)}</small>}</span>
      {digits && <a className="bub-contact-go" href={`https://wa.me/${digits}`} target="_blank" rel="noopener noreferrer" title="Buka chat di WhatsApp">Chat</a>}
    </span>
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

/** A photo opened full size; Esc or a click closes it. Zoom with the − / + buttons, the wheel, the + / - keys or a double-click; drag to pan when zoomed. */
const ZOOM_MIN = 1
const ZOOM_MAX = 5
function Lightbox({ url, onClose }: { url: string; onClose: () => void }) {
  const [z, setZ] = useState(1)
  const [pan, setPan] = useState({ x: 0, y: 0 })
  const drag = useRef<{ x: number; y: number; px: number; py: number } | null>(null)
  const zoomTo = useCallback((v: number) => {
    const n = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(v * 100) / 100))
    setZ(n)
    if (n === 1) setPan({ x: 0, y: 0 })
  }, [])
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      else if (e.key === '+' || e.key === '=') setZ((v) => Math.min(ZOOM_MAX, v + 0.5))
      else if (e.key === '-') setZ((v) => { const n = Math.max(ZOOM_MIN, v - 0.5); if (n === 1) setPan({ x: 0, y: 0 }); return n })
      else if (e.key === '0') { setZ(1); setPan({ x: 0, y: 0 }) }
    }
    window.addEventListener('keydown', k)
    return () => window.removeEventListener('keydown', k)
  }, [onClose])
  const stop = (e: { stopPropagation: () => void }) => e.stopPropagation()
  return (
    <div className="bub-lightbox" role="dialog" aria-label="Foto" onClick={onClose} onWheel={(e) => zoomTo(z * (e.deltaY < 0 ? 1.15 : 1 / 1.15))}>
      <img src={url} alt="Foto" draggable={false} className={z > 1 ? 'zoomed' : ''}
        style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${z})` }}
        onClick={stop}
        onDoubleClick={() => zoomTo(z > 1 ? 1 : 2)}
        onPointerDown={(e) => { if (z > 1) { drag.current = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y }; e.currentTarget.setPointerCapture(e.pointerId) } }}
        onPointerMove={(e) => { const d = drag.current; if (d) { setPan({ x: d.px + e.clientX - d.x, y: d.py + e.clientY - d.y }) } }}
        onPointerUp={() => { drag.current = null }} />
      <div className="bub-lb-bar" onClick={stop}>
        <div className="bub-lb-zoom" role="group" aria-label="Zoom">
          <button type="button" onClick={() => zoomTo(z - 0.5)} disabled={z <= ZOOM_MIN} aria-label="Perkecil" title="Perkecil (−)">−</button>
          <button type="button" className="bub-lb-pct" onClick={() => zoomTo(1)} title="Ukuran asli (0)">{Math.round(z * 100)}%</button>
          <button type="button" onClick={() => zoomTo(z + 0.5)} disabled={z >= ZOOM_MAX} aria-label="Perbesar" title="Perbesar (+)">+</button>
        </div>
        <a href={url} download>Unduh</a>
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
// Zoom of the conversation (20 Oct): 50%–160%, remembered on this browser for every chat
const CHAT_ZOOM_KEY = 'chat.zoom'
const CHAT_ZOOMS = [0.5, 0.6, 0.7, 0.8, 0.9, 1, 1.1, 1.25, 1.4, 1.6]
function useChatZoom(): [number, (step: number) => void, () => void] {
  const [z, setZ] = useState(() => {
    try {
      const v = Number(localStorage.getItem(CHAT_ZOOM_KEY))
      return CHAT_ZOOMS.includes(v) ? v : 1
    } catch {
      return 1
    }
  })
  const save = (v: number) => {
    setZ(v)
    try {
      localStorage.setItem(CHAT_ZOOM_KEY, String(v))
    } catch {
      /* private window: not remembered */
    }
  }
  const step = (d: number) => save(CHAT_ZOOMS[Math.min(CHAT_ZOOMS.length - 1, Math.max(0, CHAT_ZOOMS.indexOf(z) + d))])
  return [z, step, () => save(1)]
}

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
  // a draft handed over in the address (?draft=, the dealer page's Analisa Dealer button) fills the reply box once;
  // the sales reads, edits and sends it (19 Oct)
  const [params, setParams] = useSearchParams()
  // the chat's own draft stays when another chat is opened (Sam 22 Oct): restored here, kept as it is typed
  const [text, setText] = useState(() => params.get('draft') ?? getDraft(id))
  useEffect(() => {
    if (params.has('draft')) {
      const p = new URLSearchParams(params)
      p.delete('draft')
      setParams(p, { replace: true })
    }
  }, [params, setParams])
  const [older, setOlder] = useState<MessageView[]>([])
  const [olderMore, setOlderMore] = useState(true)
  const [loading, setLoading] = useState(false)
  const [perMsg, setPerMsg] = usePerMessageAnalysis()
  const [editing, setEditing] = useState<MessageView | null>(null)
  const [replying, setReplying] = useState<MessageView | null>(null)
  const [file, setFile] = useState<File | null>(null)
  // tags ("@"): the members picked into the text, and the "@…" being typed with the highlighted choice
  const [picks, setPicks] = useState<Member[]>([])
  const [tagging, setTagging] = useState<{ start: number; q: string } | null>(null)
  const [tagSel, setTagSel] = useState(0)
  // balasan cepat ("/", ADR 0065): the "/…" being typed and the highlighted choice
  const [slash, setSlash] = useState<{ start: number; q: string } | null>(null)
  const [quickSel, setQuickSel] = useState(0)
  // the composer's selection: the format bar (B I S </>) shows while some text is selected
  const [sel, setSel] = useState<[number, number]>([0, 0])
  const { openSheet } = useFeedback()
  const [zoom, setZoom] = useState<string | null>(null)
  const [searching, setSearching] = useState(false)
  const [zoom2, zoomStep, zoomReset] = useChatZoom()
  const [slim, toggleSlim] = useCompactHead()
  const [mark, setMark] = useState('')
  const pendingJump = useRef<string | null>(null)
  const body = useRef<HTMLDivElement>(null)
  // follow the newest message only while the reader is at the bottom (Sam 22 Oct, like WhatsApp): scrolled up to
  // read, a new message keeps the place and shows "↓ N pesan baru"; sending goes back down
  const stick = useRef(true)
  const lastSeen = useRef<string | null>(null)
  const [unseen, setUnseen] = useState(0)
  const [fromPhone, setFromPhone] = useState(false) // asking the phone for older messages
  const input = useRef<HTMLTextAreaElement>(null)
  // who a reply goes out as: the holder of the number (sender_name), not the chat's sales
  const from = (data?.thread.sender_name || data?.thread.account_label || data?.thread.sales || '').replace(/^Nomor\s+/, '')
  const group = data?.thread.kind === 'group'
  const { data: roster } = useMembers(id, group)
  const members = roster?.items
  const tagOptions = tagging && members ? matchMembers(members, tagging.q) : []
  const { data: quick } = useQuickReplies()
  const quickOptions = slash && quick ? matchQuick(quick.items, slash.q) : []
  // "/" shows the list while something matches; with no quick reply saved yet, a hint to make one
  const quickOpen = !!slash && !!quick && tagOptions.length === 0 && (quickOptions.length > 0 || quick.items.length === 0)
  const navigate = useNavigate()
  // a group member's name or number: their one-to-one chat from this group's number (made when there is none yet)
  const openMember = (number: string) => openMemberChat(id, number).then((tid) => { navigate('/chat/' + tid) }, (e: Error) => { toast(e.message); throw e })
  const typed = (value: string, caret: number) => {
    setText(value)
    const at = group && !editing ? mentionQuery(value, caret) : null
    setTagging(at)
    if (at?.q !== tagging?.q) setTagSel(0)
    const sl = !editing ? slashQuery(value, caret) : null
    setSlash(sl)
    if (sl?.q !== slash?.q) setQuickSel(0)
  }
  // WhatsApp formatting: the selection wrapped in *, _, ~ or ``` (again: unwrapped); the selection is kept
  const applyMark = (mk: Mark) => {
    const el = input.current
    if (!el) return
    const r = wrapSelection(text, el.selectionStart, el.selectionEnd, mk)
    setText(r.text)
    setSel([r.start, r.end])
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(r.start, r.end)
    })
  }
  // a quick reply in place of the "/…" typed — it only fills the box, the person still presses Kirim
  const pickQuick = (q: QuickReply) => {
    if (!slash) return
    const r = insertQuick(text, slash, q.body)
    setText(r.text)
    setSlash(null)
    countQuickUse(q.id)
    requestAnimationFrame(() => {
      input.current?.focus()
      input.current?.setSelectionRange(r.caret, r.caret)
    })
  }
  // picked in the Balasan cepat sheet: added to what is already typed
  const addQuick = (q: QuickReply) => {
    setText((cur) => (cur && !/\s$/.test(cur) ? cur + ' ' : cur) + q.body)
    countQuickUse(q.id)
    requestAnimationFrame(() => input.current?.focus())
  }
  const openQuick = () => openSheet(<QuickReplySheet onPick={addQuick} />)
  const pickTag = (m: Member) => {
    if (!tagging) return
    const r = insertMention(text, tagging, m)
    setText(r.text)
    setPicks((p) => (p.some((x) => x.number === m.number) ? p : [...p, m]))
    setTagging(null)
    requestAnimationFrame(() => {
      input.current?.focus()
      input.current?.setSelectionRange(r.caret, r.caret)
    })
  }
  const send = useMutation({
    mutationFn: (v: { body: string; reply_to?: string }) => api.post(`/chat/threads/${id}/messages`, v),
    onMutate: () => { stick.current = true }, // my own message: show it
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
  const openContact = () => openSheet(<ContactSheet onSend={(c) => {
    const body: Record<string, unknown> = { contact: c }
    if (replying) body.reply_to = replying.id
    return api.post(`/chat/threads/${id}/messages`, body).then(() => {
      setReplying(null)
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast('Kontak masuk antrean kirim · lewat penjaga anti-blokir')
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
  const jumpToHit = (h: Pick<ChatHit, 'id' | 'sent_at'>) => {
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
  // Sematkan: kept in GSI Orbit for the team (no limit per chat) — nothing goes to WhatsApp
  const pin = useMutation({
    mutationFn: (m: MessageView) => api.put(`/chat/threads/${id}/messages/${m.id}/pin`, { pinned: !m.pinned }),
    onSuccess: (_, m) => {
      setOlder((o) => o.map((x) => (x.id === m.id ? { ...x, pinned: !m.pinned } : x)))
      qc.invalidateQueries({ queryKey: ['chat', 'thread', id] })
      toast(m.pinned ? 'Sematan dilepas' : 'Pesan disematkan untuk tim — tidak terkirim ke WhatsApp')
    },
    onError: (e: Error) => toast(e.message),
  })
  // Pesan berbintang (Sam 22 Oct): the person's own star — shown at once, then the chat and the Berbintang list
  const star = useMutation({
    mutationFn: (m: MessageView) => api.put(`/chat/threads/${id}/messages/${m.id}/star`, { starred: !m.starred }),
    onSuccess: (_, m) => {
      const flip = (x: MessageView) => (x.id === m.id ? { ...x, starred: !m.starred } : x)
      setOlder((o) => o.map(flip))
      qc.setQueryData<ThreadDetail>(['chat', 'thread', id], (d) => (d ? { ...d, messages: d.messages.map(flip) } : d))
      qc.invalidateQueries({ queryKey: ['chat', 'starred'] })
      toast(m.starred ? 'Bintang dihapus' : 'Pesan diberi bintang — lihat di filter Berbintang')
    },
    onError: (e: Error) => toast(e.message),
  })
  const unpin = (mid: string) => api.put(`/chat/threads/${id}/messages/${mid}/pin`, { pinned: false }).then(() => {
    setOlder((o) => o.map((x) => (x.id === mid ? { ...x, pinned: false } : x)))
    qc.invalidateQueries({ queryKey: ['chat', 'thread', id] })
    toast('Sematan dilepas')
  }, (e: Error) => toast(e.message))
  const labelMsg = (m: MessageView) => openSheet(<LabelSheet title="Label pesan" sub={mediaCaption(m.body, m.media).slice(0, 80) || 'Pesan'} selected={m.labels ?? []}
    save={(ids) => api.put(`/chat/threads/${id}/messages/${m.id}/labels`, { label_ids: ids }).then((r) => { setOlder((o) => o.map((x) => (x.id === m.id ? { ...x, labels: ids } : x))); return r })} />)
  const startEdit = (m: MessageView) => {
    saveDraft() // Batal edit brings this draft back
    setReplying(null)
    setEditing(m)
    setText(m.body)
    requestAnimationFrame(() => input.current?.focus())
  }
  const cancelEdit = () => {
    setEditing(null)
    setText(getDraft(id)) // back to the draft that was there before the edit
  }
  // keep the draft once typing pauses — not on every letter: saving it redraws the chat list ("Draf: …"), which
  // made typing lag on slower laptops — and not while the box holds a message being edited
  const unsaved = useRef<string | null>(null)
  useEffect(() => {
    if (editing) return
    unsaved.current = text
    const h = setTimeout(() => { setDraft(id, text); unsaved.current = null }, 400)
    return () => clearTimeout(h)
  }, [id, text, editing])
  const saveDraft = useCallback(() => {
    if (unsaved.current !== null) setDraft(id, unsaved.current)
    unsaved.current = null
  }, [id])
  useEffect(() => saveDraft, [saveDraft]) // leaving the chat keeps what was typed last
  useEffect(() => {
    const el = body.current
    if (!el || !data) return
    const last = data.messages[data.messages.length - 1]?.id ?? null
    const first = lastSeen.current === null
    const arrived = !first && last !== lastSeen.current
    lastSeen.current = last
    if (first || stick.current) { // opened, or reading the newest: follow it
      el.scrollTop = el.scrollHeight
      setUnseen(0)
    } else if (arrived) { // reading older messages: keep the place, count what came
      setUnseen((n) => n + 1)
    }
  }, [data])
  // a message picked in Cari chat's "Pesan" results (?m=&at=): go to it once the chat is loaded
  const hitParam = params.get('m')
  const hitAt = params.get('at') ?? ''
  const jumpRef = useRef(jumpToHit)
  useEffect(() => { jumpRef.current = jumpToHit })
  useEffect(() => {
    if (!hitParam || !data) return
    jumpRef.current({ id: hitParam, sent_at: hitAt })
    const p = new URLSearchParams(params)
    p.delete('m')
    p.delete('at')
    setParams(p, { replace: true })
  }, [hitParam, hitAt, data, params, setParams])
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
    if (!(picked && data && (data.thread.unread > 0 || !!data.thread.mentioned))) return
    // opened = read: the count and the "Belum dibaca" tag drop at once, then the lists, the numbers' badges and
    // the other numbers' copies of a group come back from the server
    qc.setQueriesData<ThreadView[]>({ queryKey: ['chat', 'threads'] }, (list) => list?.map((t) => (t.id === id ? { ...t, unread: 0, mentioned: 0 } : t)))
    qc.setQueryData<ThreadDetail>(['chat', 'thread', id], (d) => (d ? { ...d, thread: { ...d.thread, unread: 0, mentioned: 0 } } : d))
    api.post(`/chat/threads/${id}/read`).finally(() => {
      qc.invalidateQueries({ queryKey: ['chat', 'threads'] })
      qc.invalidateQueries({ queryKey: ['wa'] })
    })
  }, [data, id, qc, picked])
  // Esc closes the chat, like WhatsApp Web ("Pilih chat") — once nothing else took it first (a menu, a sheet, the
  // picture viewer, the "/" or "@" list, a reply or edit being cancelled); not from another text box (Cari chat, notes)
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented || zoom || e.isComposing) return
      const t = e.target as HTMLElement | null
      if (t && t !== input.current && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return
      onBack()
    }
    window.addEventListener('keydown', k)
    return () => window.removeEventListener('keydown', k)
  }, [zoom, onBack])
  // the box grows with the text, up to about six lines, and shrinks back after sending
  useLayoutEffect(() => {
    const el = input.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 140) + 'px'
  }, [text, data])
  // the bubbles' actions read the latest handlers through a ref, so the memoised list keeps the same props while typing
  const handlers = useRef({ startEdit, startReply, deleteMsg, jumpTo, useSuggestion, pin: pin.mutate, star: star.mutate, labelMsg, openMember })
  useLayoutEffect(() => {
    handlers.current = { startEdit, startReply, deleteMsg, jumpTo, useSuggestion, pin: pin.mutate, star: star.mutate, labelMsg, openMember }
  })
  const acts = useMemo<BubbleActs>(() => ({
    onEdit: (m) => handlers.current.startEdit(m),
    onReply: (m) => handlers.current.startReply(m),
    onDelete: (m) => handlers.current.deleteMsg(m),
    onJump: (mid) => handlers.current.jumpTo(mid),
    onZoom: setZoom,
    onUse: (m, draft) => handlers.current.useSuggestion(m, draft),
    onPin: (m) => handlers.current.pin(m),
    onStar: (m) => handlers.current.star(m),
    onLabel: (m) => handlers.current.labelMsg(m),
    onMember: (n) => { handlers.current.openMember(n).catch(() => {}) },
  }), [])
  const messages = useMemo(() => [...older, ...(data?.messages ?? [])], [older, data?.messages])
  if (!data) return <div className="pane th-pane ibc" />
  const t = data.thread
  // The first page holds the last month (CHAT_OPEN_DAYS); older messages come in pages on scrolling up.
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
  // "Muat pesan lama dari HP" (chats kept 90 days, Sam 22 Oct): the phone sends the 50 messages before the oldest one
  // shown within seconds; they are fetched then (a few tries) and put on top, the place kept
  const askPhone = () => {
    const first = messages[0]
    if (!first || fromPhone) return
    setFromPhone(true)
    api.post<{ requested: number }>(`/chat/threads/${id}/history`, {}).then(() => {
      toast('Diminta ke HP — pesan lama muncul dalam beberapa detik')
      let tries = 0
      const pull = () => {
        tries++
        api.get<ThreadDetail>(`/chat/threads/${id}?before=${encodeURIComponent(first.sent_at)}`).then((r) => {
          if (r.messages.length) {
            if (body.current) keepOffset.current = body.current.scrollHeight - body.current.scrollTop
            setOlder((o) => [...r.messages.filter((m) => !o.some((x) => x.id === m.id)), ...o])
            setOlderMore(!!r.has_more)
            setFromPhone(false)
          } else if (tries < 6) setTimeout(pull, 2500)
          else { setFromPhone(false); toast('HP tidak mengirim pesan yang lebih lama — mungkin memang awal percakapan') }
        }, () => setFromPhone(false))
      }
      setTimeout(pull, 2500)
    }, (e: Error) => { setFromPhone(false); toast(e.message) })
  }
  const onScroll = () => {
    const el = body.current
    if (!el) return
    if (el.scrollTop < 200) loadOlder()
    stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
    if (stick.current && unseen) setUnseen(0)
  }
  const toNewest = () => {
    stick.current = true
    setUnseen(0)
    body.current?.scrollTo({ top: body.current.scrollHeight, behavior: 'smooth' })
  }
  // Enter sends, Shift+Enter starts a new line (WhatsApp Web); Enter while an IME is composing a word does neither
  const composerKey = (e: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    const mk = shortcutMark(e)
    if (mk) { // Ctrl/⌘+B bold, +I italic, +Shift+X strikethrough, +Shift+M monospace (WhatsApp Web)
      e.preventDefault()
      applyMark(mk)
      return
    }
    if (quickOpen) {
      // choosing a quick reply, like WhatsApp Business: ↑ ↓, Enter or Tab puts it in the box, Esc closes the list
      if (quickOptions.length > 0 && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
        e.preventDefault()
        setQuickSel((i) => (i + (e.key === 'ArrowDown' ? 1 : quickOptions.length - 1)) % quickOptions.length)
        return
      }
      if (quickOptions.length > 0 && ((e.key === 'Enter' && !e.shiftKey) || e.key === 'Tab')) {
        e.preventDefault()
        pickQuick(quickOptions[Math.min(quickSel, quickOptions.length - 1)])
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setSlash(null)
        return
      }
    }
    if (tagOptions.length > 0) {
      // choosing whom to tag, like WhatsApp: ↑ ↓, Enter or Tab picks, Esc closes the list
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault()
        setTagSel((i) => (i + (e.key === 'ArrowDown' ? 1 : tagOptions.length - 1)) % tagOptions.length)
        return
      }
      if ((e.key === 'Enter' && !e.shiftKey) || e.key === 'Tab') {
        e.preventDefault()
        pickTag(tagOptions[Math.min(tagSel, tagOptions.length - 1)])
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setTagging(null)
        return
      }
    }
    if (e.key === 'Escape') {
      // a reply, an edit or a file is cancelled first; with none, Esc goes on to close the chat
      if (editing || replying || file) e.preventDefault()
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
    const v = editing ? text.trim() : toWire(text.trim(), picks) // tags go to WhatsApp as "@<number>"
    setPicks([])
    setTagging(null)
    setSlash(null)
    if (file && !editing) {
      setText('')
      sendForm(file, file.name, { body: v })
      setFile(null)
      return
    }
    if (!v) return
    if (editing) {
      setText(getDraft(id)) // the draft typed before the edit comes back
      setEditing(null)
      if (v !== editing.body) edit.mutate({ id: editing.id, body: v })
      return
    }
    setText('')
    send.mutate(replying ? { body: v, reply_to: replying.id } : { body: v })
    setReplying(null)
  }
  // Nama panggilan: what the team calls this number ("Pak Yohanes"), kept with the number and shown with the master name
  const rename = () => {
    const v = window.prompt(`Nama panggilan untuk ${t.phone ? fmtPhone(t.phone) : t.title} — tampil sebagai "Nama - ${t.title}" dan bisa dicari. Kosongkan untuk menghapus.`, t.phone_name ?? '')
    if (v === null) return
    api.put<{ name: string; phone_synced?: boolean; phone_note?: string }>(`/chat/threads/${id}/alias`, { name: v }).then(
      (r) => {
        toast(!r.name ? 'Nama panggilan dihapus' : r.phone_synced ? `Nama panggilan: ${r.name} · tersimpan juga di kontak WhatsApp HP` : `Nama panggilan: ${r.name}${r.phone_note ? ' · ' + r.phone_note : ''}`)
        qc.invalidateQueries({ queryKey: ['chat'] })
      },
      (e) => toast(e instanceof Error ? e.message : 'Gagal menyimpan nama'),
    )
  }
  const links = t.kind === 'group' ? [] : lookupLinks(t.title, t.phone, t.dealer_city)
  // an internal group is never read by Analisa WhatsApp; a customer group is, like a one-to-one chat
  const readable = t.kind !== 'group' || t.group_kind === 'external'
  return (
    <div className="pane th-pane ibc">
      <div className={`ibc-h${slim ? ' slim' : ''}`}>
        <button className="back" onClick={onBack} aria-label="Kembali"><Icon name="arrow" /></button>
        <span className={`av ${t.kind === 'group' ? 'grp' : ''}`}>{t.kind === 'group' ? <Icon name="people" /> : initials(t)}</span>
        <div className="ibc-who">
          <div className="ibc-name">
            <b>{displayTitle(t)}</b>
            {t.kind !== 'group' && <button type="button" className="ibc-alias" onClick={rename} title={t.phone_name ? 'Ubah nama panggilan' : 'Beri nama panggilan (seperti di HP)'} aria-label="Nama panggilan"><Icon name="edit" /></button>}
            {t.subtitle && <span className="ibr-src">≈ {t.subtitle}</span>}
            <Dock screen="chat" head />
            <OrchPill />
          </div>
          <div className="ibc-sub">
            {t.phone && <span className="ibc-phone">{fmtPhone(t.phone)}</span>}
            {links.map((l) => <a key={l.key} className="ibc-link" href={l.href} target="_blank" rel="noreferrer noopener">{l.label}</a>)}
            {t.kind === 'group' && <span className="ibc-phone">{from} · {t.account_masked}</span>}
            <LabelChips ids={t.labels} className="ibc-lbls" />
          </div>
        </div>
        <div className="ibc-acts">
          <Waiting since={t.waiting_since} now={now} />
          {t.kind === 'group' && (
            <button type="button" className="ibl-btn ibc-find ibc-mem" onClick={() => openSheet(<MembersSheet thread={id} group={displayTitle(t)} onOpen={(n) => openMember(n)} />)} title="Anggota grup — klik nama untuk chat" aria-label="Anggota grup">
              <Icon name="people" />{roster ? <span>{roster.items.length + roster.hidden}</span> : null}
            </button>
          )}
          {t.kind !== 'group' && t.phone && (
            <a className="ibl-btn ibc-find" href={`tel:+${t.phone.replace(/\D/g, '')}`} title={`Telepon ${fmtPhone(t.phone)} dari perangkat ini (HP, atau laptop dengan aplikasi telepon). Panggilan WhatsApp: dari HP pemegang nomor.`} aria-label="Telepon"><Icon name="phone" /></a>
          )}
          <button type="button" className="ibl-btn ibc-find" onClick={() => openSheet(<LabelSheet title="Label chat" sub={displayTitle(t)} selected={t.labels ?? []}
            save={(ids) => api.put(`/chat/threads/${id}/labels`, { label_ids: ids })} />)} title="Label chat" aria-label="Label chat"><Icon name="tag" /></button>
          <div className="ibc-zoom" role="group" aria-label="Zoom chat">
            <button type="button" onClick={() => zoomStep(-1)} disabled={zoom2 <= CHAT_ZOOMS[0]} aria-label="Perkecil chat" title="Perkecil chat">−</button>
            <button type="button" className="ibc-zoom-pct" onClick={zoomReset} title="Kembali ke 100%">{Math.round(zoom2 * 100)}%</button>
            <button type="button" onClick={() => zoomStep(1)} disabled={zoom2 >= CHAT_ZOOMS[CHAT_ZOOMS.length - 1]} aria-label="Perbesar chat" title="Perbesar chat">+</button>
          </div>
          <button type="button" className={`ibl-btn ibc-find${searching ? ' on' : ''}`} onClick={() => { setSearching(!searching); if (searching) setMark('') }} title="Cari di chat ini" aria-label="Cari di chat ini" aria-pressed={searching}><Icon name="search" /></button>
          {readable && (
            <button type="button" className={`ibc-an-tg${perMsg ? ' on' : ''}`} aria-pressed={perMsg} aria-label="Analisa per pesan" onClick={() => setPerMsg(!perMsg)}
              title={t.analyzed_at ? `Analisa per pesan · terakhir dianalisa ${shortDate(t.analyzed_at)} ${hhmm(t.analyzed_at)}` : 'Analisa per pesan · belum pernah dianalisa — analisa lewat Orchestrator di samping nama'}>
              <Icon name="spark" /><span className="ibc-an-l">Per pesan</span>
            </button>
          )}
          {t.kind !== 'group' && <button className="btn ibc-acc" onClick={onAccurate}><Icon name="send" />Accurate</button>}
          <button type="button" className={`ibl-btn ibc-find ibc-slim${slim ? ' on' : ''}`} onClick={toggleSlim} aria-pressed={slim}
            title={slim ? 'Tampilkan header lengkap' : 'Ringkas header'} aria-label="Ringkas header"><Icon name="chev" /></button>
          {!lead && <button className="ibl-btn ibc-lead-btn" onClick={() => setLead(true)} title="Tampilkan panel lead" aria-label="Tampilkan panel lead"><Icon name="doc" /></button>}
        </div>
      </div>
      <PinnedBar pins={data.pins ?? []} who={displayTitle(t)} onJump={(p) => jumpToHit({ id: p.message_id, sent_at: p.sent_at })} onUnpin={(p) => unpin(p.message_id)} />
      {searching && <ChatSearch thread={id} onJump={jumpToHit} onQuery={setMark} onClose={() => { setSearching(false); setMark('') }} />}
      {unseen > 0 && <button type="button" className="ibc-new" onClick={toNewest} title="Ke pesan terbaru"><Icon name="arrow" style={{ transform: 'rotate(90deg)' }} />{unseen} pesan baru</button>}
      <div className="th-body ibc-body" ref={body} onScroll={onScroll} style={zoom2 !== 1 ? { zoom: zoom2 } : undefined}>
        <div className="ibc-start">
          {more
            ? <span className="ibc-pill dark"><Icon name="refresh" />{loading ? 'Memuat pesan sebelumnya…' : 'Gulir ke atas untuk pesan sebelumnya'}</span>
            : (
              <span className="ibc-start-old">
                <span className="ibc-pill" title="GSI Orbit menyimpan chat selama masa simpan; yang lebih lama masih ada di HP pemegang nomor">{data.history_from ? `Riwayat sejak ${shortDate(data.history_from)}` : 'Awal percakapan'} · chat disimpan {data.history_days || 90} hari</span>
                <button type="button" className="ibc-pill dark ibc-fromphone" disabled={fromPhone} onClick={askPhone} title="Minta 50 pesan sebelumnya ke HP pemegang nomor, seperti WhatsApp Web">
                  <Icon name="phone" />{fromPhone ? 'Meminta ke HP…' : 'Muat pesan lama dari HP'}
                </button>
              </span>
            )}
        </div>
        <MessageList messages={messages} nowMs={now.getTime()} via={from} group={t.kind === 'group'} analysis={perMsg} thread={id} asOf={data.stock_as_of} mark={mark} acts={acts} />
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
            {!editing && <AttachMenu onFile={pickFile} onPoll={openPoll} onContact={openContact} onQuick={openQuick} disabled={upload.isPending} />}
            {tagOptions.length === 0 && sel[1] > sel[0] && (
              <div className="cmp-fmt" role="toolbar" aria-label="Format teks">
                {([['*', 'B', 'Tebal (Ctrl/⌘+B)'], ['_', 'I', 'Miring (Ctrl/⌘+I)'], ['~', 'S', 'Coret (Ctrl/⌘+Shift+X)'], ['```', '</>', 'Monospace (Ctrl/⌘+Shift+M)']] as [Mark, string, string][]).map(([mk, label, title]) => (
                  <button key={mk} type="button" className={`fmt-${label === '</>' ? 'm' : label.toLowerCase()}`} title={title} aria-label={title} onMouseDown={(e) => { e.preventDefault(); applyMark(mk) }}>{label}</button>
                ))}
              </div>
            )}
            {quickOpen && quick && (
              <div className="cmp-tags cmp-quick" role="listbox" aria-label="Balasan cepat">
                {quickOptions.map((q, i) => (
                  <button key={q.id} type="button" role="option" aria-selected={i === quickSel} className={i === quickSel ? 'on' : ''}
                    onMouseDown={(e) => { e.preventDefault(); pickQuick(q) }} onMouseEnter={() => setQuickSel(i)}>
                    <b>/{q.shortcut}</b><small>{q.body}</small>
                  </button>
                ))}
                {quick.items.length === 0 && <p className="cmp-quick-empty">Belum ada balasan cepat — simpan teks yang sering dikirim (rekening, alamat, salam) lalu ketik / untuk memakainya.</p>}
                <button type="button" className="cmp-quick-manage" onMouseDown={(e) => { e.preventDefault(); setSlash(null); openQuick() }}><Icon name="gear" />Kelola balasan cepat</button>
              </div>
            )}
            {tagOptions.length > 0 && (
              <div className="cmp-tags" role="listbox" aria-label="Tandai anggota grup">
                {tagOptions.map((m, i) => (
                  <button key={m.number} type="button" role="option" aria-selected={i === tagSel} className={i === tagSel ? 'on' : ''}
                    onMouseDown={(e) => { e.preventDefault(); pickTag(m) }} onMouseEnter={() => setTagSel(i)}>
                    <b>{m.name}</b><small>{fmtPhone(m.number)}</small>
                  </button>
                ))}
              </div>
            )}
            <textarea ref={input} rows={1} value={text} onChange={(e) => typed(e.target.value, e.target.selectionStart ?? e.target.value.length)} onKeyDown={composerKey}
              onBlur={() => { setTagging(null); setSlash(null); setSel([0, 0]) }}
              onSelect={(e) => setSel([e.currentTarget.selectionStart, e.currentTarget.selectionEnd])}
              onPaste={(e) => {
                // a copied screenshot or picture: it becomes the file to send, the box takes its caption (WhatsApp Web)
                const img = !editing && pastedImage(e.clipboardData.files, e.clipboardData.getData('text/plain'))
                if (img) { e.preventDefault(); pickFile(img) }
              }}
              placeholder={editing ? 'Ubah pesan…' : file ? 'Tambahkan keterangan…' : group ? 'Tulis balasan… (@ menandai, / balasan cepat, Shift+Enter: baris baru)' : 'Tulis balasan… (/ balasan cepat, Shift+Enter: baris baru)'} autoComplete="off" aria-label={editing ? 'Ubah pesan' : file ? 'Keterangan file' : 'Tulis balasan'} />
            {text.trim() || file || editing
              ? <button type="submit" className="send" aria-label={editing ? 'Simpan edit' : 'Kirim'} disabled={send.isPending || edit.isPending || upload.isPending}><Icon name={editing ? 'check' : 'send'} /></button>
              : <button type="button" className="send" onClick={voice.start} aria-label="Rekam pesan suara" title="Rekam pesan suara"><Mic /></button>}
          </form>
        )}
        <span className="pol" title={'Dikirim dari ' + from + ' (' + t.account_masked + ') · jeda & "mengetik…" seperti manusia · penjaga anti-blokir'}><Icon name="lock" />Dikirim dari {from} ({t.account_masked}) · anti-blokir</span>
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
      <span>Dibaca Analisa WhatsApp dan Orchestrator seperti chat pelanggan; pesan karyawan GSI di grup ini dihitung sebagai balasan.</span>
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
  const qc = useQueryClient()
  const nav = useNavigate()
  const { openSheet, toast } = useFeedback()
  const [tab, setTab] = useState<LeadTab>('main')
  if (!ctx || !th) return <div className={`pane ibp${mini ? ' mini' : ''}`} />
  const t = th.thread
  const d = ctx.dealer
  const idf = ctx.identification
  const group = t.kind === 'group'
  const customerGroup = t.group_kind === 'external'
  const groupLabel = customerGroup ? 'Grup pelanggan' : 'Grup internal'
  const kindLabel = t.kind === 'dealer' ? 'Dealer' : group ? groupLabel : 'Lead'
  const stage = d ? d.metrics.status : t.kind === 'new' ? (idf ? 'Teridentifikasi' : 'Lead baru') : groupLabel
  const tone = d ? STATUS_TONE[d.metrics.status] ?? 'accent' : t.kind === 'new' ? 'accent' : 'neutral'
  const filled = d ? Math.max(1, Math.round((d.metrics.score / 100) * 7)) : t.kind === 'new' ? (idf ? Math.max(2, Math.round((idf.score / 100) * 7)) : 1) : 0
  const num = wa?.items.find((n) => n.wa_number === t.account)
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
      {notes.length === 0 && <li className="muted">Belum ada catatan Claude — analisa lewat Orchestrator di samping nama chat</li>}
      {notes.slice(-12).map((m) => (
        <li key={m.id}><span className="ai" style={{ fontSize: 0 }} /><span>{m.analysis!.note}<small>{m.from_name} · {hhmm(m.sent_at)}</small></span></li>
      ))}
    </ul>
  )
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
          <div><b>{d?.name ?? idf?.best_name ?? displayTitle(t)}</b><span>{t.phone ? fmtPhone(t.phone) : group ? `${from(t)} · ${t.account_masked}` : t.subtitle}</span></div>
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
            {(!group || customerGroup) && <AIOverview thread={id} group={group} />}
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
            <p className="ibp-note">Analisa di panel ini ditulis Claude lewat Orchestrator (tombol di samping nama chat).</p>
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
            <ChatStats thread={id} group={group} />
            {d && <h4>Dealer</h4>}
            {d && <Kv rows={[
              ...(d ? ([
                ['Skor dealer', `${d.metrics.score}/100`],
                ['Omzet / bulan', fmtRp(d.metrics.omzet_bln)],
                ['Rata-rata order', fmtRp(d.metrics.avg_order)],
                ['Ritme order', d.metrics.rhythm_days ? `${d.metrics.rhythm_days} hari` : null],
                ['Share of wallet', sowText(d.metrics)],
              ] as [string, ReactNode][]) : []),
            ]} />}
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

const from = (t: ThreadDetail['thread']) => (t.sender_name || t.account_label || t.sales || '').replace(/^Nomor\s+/, '')

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

const ACCOUNT_KEY = 'chat-account'

/** The number Chat opens on: the one picked last on this browser, else the user's own, else the first linked one. */
function defaultAccount(items: WANumber[]): string {
  let last = ''
  try { last = localStorage.getItem(ACCOUNT_KEY) ?? '' } catch { /* private window */ }
  return (items.find((n) => n.wa_number === last) ?? items.find((n) => n.mine && n.state === 'connected') ?? items.find((n) => n.state === 'connected') ?? items[0])?.wa_number ?? ''
}

export function ChatPage() {
  const { threadId } = useParams()
  const nav = useNavigate()
  // the chat list's width, dragged like a VS Code panel (Sam 22 Oct); null = the layout's default
  const [listW, setListW] = useState<number | null>(savedListWidth)
  const chatBox = useRef<HTMLDivElement>(null)
  // the page header over the list column takes the list's width (a dragged width included)
  useLayoutEffect(() => {
    const stage = document.querySelector<HTMLElement>('.stage')
    if (!stage || !listW) return
    stage.style.setProperty('--list-w', `${listW}px`)
    return () => { stage.style.removeProperty('--list-w') }
  }, [listW])
  const [filter, setFilter] = useState<Filter>('all')
  // one number at a time (Sam 19 Oct: "Semua" loaded every number's chats and made Chat slow)
  const { data: wa } = useWAStatus()
  const [picked, setPicked] = useState('')
  const account = picked || defaultAccount(wa?.items ?? [])
  const [kept, setKept] = useState<string | undefined>(undefined)
  const accountChanged = useRef(false)
  const setAccount = (a: string) => {
    if (a !== account) {
      accountChanged.current = true
      setKept(undefined)
      if (threadId) nav('/chat', { replace: true })
    }
    setPicked(a)
    try { localStorage.setItem(ACCOUNT_KEY, a) } catch { /* not remembered */ }
  }
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
  const { data: list, isSuccess } = useThreads('all', account, !!account)
  // Keep a selected chat while filters change, but never carry it over to another WhatsApp account.
  if (threadId && kept !== threadId && !accountChanged.current) setKept(threadId)
  const candidate = threadId ?? kept
  const { data: selectedThread } = useThread(candidate)
  const selectedTeam = account.startsWith('team:') ? account.slice(5) : ''
  const accountMismatch = !!account && !!selectedThread && (selectedTeam
    ? !!selectedThread.thread.account_team && selectedThread.thread.account_team !== selectedTeam
    : !!selectedThread.thread.account && selectedThread.thread.account !== account)
  const active = accountChanged.current || accountMismatch ? undefined : candidate
  useEffect(() => {
    if (!candidate || !accountMismatch) return
    accountChanged.current = true
    setKept(undefined)
    if (threadId) nav('/chat', { replace: true })
  }, [accountMismatch, candidate, nav, threadId])
  const leave = () => { setKept(undefined); nav('/chat') }
  // the account picker goes in the page header next to "Chat", when the header has its slot
  const slot = useOutletContext<ShellContext | undefined>()?.topbarSlot ?? null
  const numbers = <NumberRail account={account} setAccount={setAccount} />
  const rail = slot ? createPortal(numbers, slot) : numbers
  // No number linked yet: show how to connect the team's numbers.
  if (!threadId && wa && wa.items.length === 0) {
    return (
      <div className={`chat multi chat-empty ${slot ? 'rail-out' : ''}`}>
        {rail}
        <div className="pane connect-pane"><ConnectPanel /></div>
      </div>
    )
  }
  // a logged-out number's history stays locked for everyone until it is linked again
  const locked = wa?.items.find((x) => x.wa_number === account && x.state === 'logged_out')
  if (locked) {
    return (
      <div className={`chat multi chat-empty ${slot ? 'rail-out' : ''}`}>
        {rail}
        <div className="pane connect-pane"><NumberLocked n={locked} /></div>
      </div>
    )
  }
  const accurate = () => active && openSheet(<AccurateSheet id={active} />)
  return (
    <div ref={chatBox} style={listWidthStyle(listW)} className={`chat multi komo ${slot ? 'rail-out' : ''} ${threadId ? 'show-thread' : ''} ${active ? 'with-lead' : ''} ${active && !lead ? 'lead-mini' : ''}`}>
      <ListResizer box={chatBox} width={listW} onWidth={setListW} />
      {rail}
      <ThreadList list={list ?? []} loaded={isSuccess} filter={filter} setFilter={setFilter} account={account} active={active} onPick={(id) => { accountChanged.current = false; nav('/chat/' + id) }}
        onHit={(h) => { accountChanged.current = false; nav(`/chat/${h.thread_id}?${new URLSearchParams({ m: h.id, at: h.sent_at })}`) }} onLeave={leave} />
      {active && <LeadPane key={'l' + active} id={active} mini={!lead} setMini={(v) => setLead(!v)} onAccurate={accurate} />}
      {active ? <ThreadPane key={active} id={active} picked={!!threadId} onBack={() => nav('/chat')} lead={lead} setLead={setLead} onAccurate={accurate} /> : (
        <div className="pane th-pane th-none"><Icon name="chat" /><b>Pilih chat</b><span>Klik salah satu percakapan di daftar untuk membukanya.</span></div>
      )}
    </div>
  )
}

/** The open number does not sync: logged out on the phone (WhatsApp removed the linked device), never linked, or
 * reconnecting. New chats do not arrive until it is linked again (Sam 23 Oct: Granike Monica logged out, the boss's
 * chat never came in and nothing in Chat said so). */
function NumberDown({ n }: { n: WANumber }) {
  const { openSheet } = useFeedback()
  if (n.state === 'connected' || n.state === 'pairing') return null
  const since = n.last_seen_at ? ` sejak ${shortDate(n.last_seen_at)} ${hhmm(n.last_seen_at)}` : ''
  const text = n.state === 'logged_out' ? `WhatsApp ${labelOf(n)} keluar dari perangkat tertaut${since} — chat baru tidak masuk ke Orbit`
    : n.state === 'unpaired' ? `WhatsApp ${labelOf(n)} belum ditautkan — chat tidak masuk ke Orbit`
    : `WhatsApp ${labelOf(n)} terputus${since} — menyambung ulang…`
  return (
    <div className={`ibl-down${n.state === 'disconnected' ? ' warn' : ''}`} role="alert">
      <Icon name="alert" /><span>{text}</span>
      {n.state !== 'disconnected' && <button type="button" className="btn ghost" onClick={() => openSheet(<PairSheet wa={n.wa_number} />)}><Icon name="qr" />Tautkan ulang</button>}
    </div>
  )
}
