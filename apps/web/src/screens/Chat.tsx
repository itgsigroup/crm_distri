// Chat (WhatsApp) screen — port of the mockup's #screen-chat (renderChatList / renderThread /
// renderChatCtx / sendChat). Replies are never sent directly: the backend turns each reply into
// an approval Action and returns a pending outbound message plus a toast explaining it.
import { useLayoutEffect, useRef, useState, type FormEvent } from 'react'
import { api, useApi } from '../api/client'
import type { Action, ChatContext, ChatListItem, ChatMessage, ChatThread, ChatType } from '../api/types'
import { Icon, Loading, Pill, Ring } from '../components/ui'
import { useUI } from '../state/ui'

const TABS: [string, string][] = [
  ['all', 'Semua'],
  ['cust', 'Pelanggan'],
  ['gext', 'Grup ekst.'],
  ['gint', 'Grup int.'],
  ['internal', 'Internal'],
]
const TYPE_ORDER: ChatType[] = ['cust', 'gext', 'gint', 'internal']
const CTYPE: Record<ChatType, string> = {
  cust: 'Pelanggan',
  gext: 'Grup eksternal · dengan pelanggan',
  gint: 'Grup internal',
  internal: 'Internal · pribadi',
}

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e))
const avClass = (t: ChatType) => (t === 'gext' || t === 'gint' ? 'grp' : t === 'internal' ? 'int' : '')

interface ReplyResponse { action: Action; toast: string; message: ChatMessage }

export function ChatScreen() {
  const { route, go } = useUI()
  const [tab, setTab] = useState('all')
  const [showThread, setShowThread] = useState(() => !!route.param)
  const list = useApi<{ items: ChatListItem[]; unread: number }>('/api/chat/threads?type=' + tab)
  const items = list.data?.items ?? []
  const activeId = (route.screen === 'chat' && route.param) || items[0]?.id || ''
  const thread = useApi<ChatThread>(activeId ? '/api/chat/threads/' + activeId : null)

  const select = (id: string) => {
    // Opening a thread marks it read on the server; mirror that locally like the mockup.
    list.setData(d => (d ? { ...d, items: d.items.map(c => (c.id === id ? { ...c, unread: 0 } : c)) } : d))
    setShowThread(true)
    go('chat:' + id)
  }

  if (!list.data) return <section className="screen"><Loading error={list.error} /></section>

  const sorted = [...items].sort((a, b) => TYPE_ORDER.indexOf(a.type) - TYPE_ORDER.indexOf(b.type))
  const t = thread.data && thread.data.id === activeId ? thread.data : null

  return (
    <section className="screen" id="screen-chat">
      <div className={'chat' + (showThread ? ' show-thread' : '')} id="chat">
        <div className="pane list-pane">
          <div className="chat-tabs" id="chat-tabs">
            {TABS.map(([k, l]) => (
              <button key={k} className={tab === k ? 'is-active' : ''} onClick={() => setTab(k)}>{l}</button>
            ))}
          </div>
          <div className="chat-list" id="chat-list">
            {sorted.map((c, i) => (
              <ChatRow key={c.id} c={c} active={c.id === activeId} header={tab === 'all' && (i === 0 || sorted[i - 1].type !== c.type)} onSelect={select} />
            ))}
          </div>
        </div>
        <div className="pane th-pane" id="th-pane">
          {t ? <ThreadPane key={t.id} t={t} onBack={() => setShowThread(false)} /> : activeId && <div className="th-body"><Loading error={thread.error} /></div>}
        </div>
        <div className="pane ctx-pane ctx" id="chat-ctx">
          {t && <ContextPane ctx={t.context} />}
        </div>
      </div>
    </section>
  )
}

function ChatRow({ c, active, header, onSelect }: { c: ChatListItem; active: boolean; header: boolean; onSelect: (id: string) => void }) {
  return (
    <>
      {header && <div className="chat-sec">{CTYPE[c.type]}</div>}
      <button className={'ci' + (active ? ' is-active' : '')} data-chat={c.id} onClick={() => onSelect(c.id)}>
        <span className={'av ' + avClass(c.type)}>{c.initials}</span>
        <div>
          <b>{c.name}</b>
          <span className="sn">{c.sub}</span>
          <span className="lm">{c.private && !c.last ? 'Chat pribadi karyawan · tidak dibaca' : c.last}</span>
          <div className="tg"><Pill p={c.tag} style={{ fontSize: 10 }} /></div>
        </div>
        <div className="rt">
          <span>{c.time}</span>
          {c.unread > 0 && <span className="un">{c.unread}</span>}
          <span style={{ fontSize: 10 }}>via {c.via}</span>
        </div>
      </button>
    </>
  )
}

function ThreadPane({ t, onBack }: { t: ChatThread; onBack: () => void }) {
  const { go, toast, refresh } = useUI()
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  // Pending outbound messages returned by /reply, kept until the server thread includes them.
  const [pending, setPending] = useState<ChatMessage[]>([])
  const body = useRef<HTMLDivElement>(null)

  const serverIds = new Set(t.messages.map(m => m.id).filter(id => id !== undefined))
  const localPending = pending.filter(m => m.id === undefined || !serverIds.has(m.id))

  useLayoutEffect(() => {
    const el = body.current
    if (el) el.scrollTop = el.scrollHeight
  }, [t.messages, pending])

  const head = (
    <div className="th-h">
      <button className="back" data-chat-back aria-label="Kembali" onClick={onBack}><Icon n="i-arrow" /></button>
      <span className="av">{t.initials}</span>
      <div><b>{t.name}</b><span>{t.sub}</span></div>
      <div className="via"><b>Nomor {t.via}</b><br />{t.via_note}</div>
    </div>
  )

  if (t.private) {
    return (
      <>
        {head}
        <div className="th-body">
          <div className="th-note">
            <Icon n="i-lock" />
            <b>{t.private_note?.title}</b>
            <span>{t.private_note?.body}</span>
            <button className="btn ghost" onClick={() => go('conn:sec-wa')}>Lihat aturan</button>
          </div>
        </div>
      </>
    )
  }

  const act = (id?: string) => {
    if (!id) return
    api.post<{ toast: string }>('/api/chat/annotations/' + encodeURIComponent(id) + '/act')
      .then(r => toast(r.toast))
      .catch(e => toast(errMsg(e)))
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const v = text.trim()
    if (!v || sending) return
    setSending(true)
    try {
      const r = await api.post<ReplyResponse>('/api/chat/threads/' + encodeURIComponent(t.id) + '/reply', { text: v })
      if (r.message) setPending(p => [...p, r.message])
      setText('')
      toast(r.toast)
      refresh()
    } catch (err) {
      toast(errMsg(err))
    } finally {
      setSending(false)
    }
  }

  const renderMsg = (m: ChatMessage, key: string, isPending: boolean) => {
    if (m.day) return <div key={key} className="day">{m.day}</div>
    return (
      <div key={key} className={'m ' + (m.f ?? 'in') + (isPending ? ' pending' : '')} style={isPending ? { opacity: 0.75 } : undefined}>
        {m.who && m.f === 'in' && (
          <span className="who">{m.who}<span className={'pill ' + (m.int ? 'neutral' : 'accent')}>{m.int ? 'internal' : 'eksternal'}</span></span>
        )}
        <div className="bb">{m.t}</div>
        <span className="tm">{m.tm}{m.f === 'out' ? ' · ' + (m.who || t.via) + (m.sent ? ' · ' + m.sent : '') : ''}</span>
        {m.ann && (
          <span className={'ann ' + m.ann.k}>
            <span className="ai" />
            {m.ann.t}
            {m.ann.act && <> · <button type="button" disabled={!m.ann.act_id} onClick={() => act(m.ann?.act_id)}>{m.ann.act}</button></>}
          </span>
        )}
      </div>
    )
  }

  return (
    <>
      {head}
      <div className="th-body" id="th-body" ref={body}>
        {t.messages.map((m, i) => renderMsg(m, 's' + (m.id ?? 'i' + i), false))}
        {localPending.map((m, i) => renderMsg(m, 'p' + (m.id ?? i), true))}
      </div>
      <div className="th-c">
        <div className="sug">
          {t.suggestions.map(x => (
            <button key={x} type="button" className="chip" data-sug={x} onClick={() => setText(x)}>
              <span className="ai" style={{ fontSize: 0 }} />{x}
            </button>
          ))}
        </div>
        <form id="chat-form" onSubmit={submit}>
          <span className="as"><Icon n="i-chat" />Balas sebagai {t.via}</span>
          <input id="chat-input" placeholder="Tulis balasan…" autoComplete="off" aria-label="Tulis balasan" value={text} onChange={e => setText(e.target.value)} />
          <button type="submit" className="send" aria-label="Kirim" disabled={sending}><Icon n="i-send" /></button>
        </form>
        <span className="pol"><Icon n="i-lock" />{t.policy_line}</span>
      </div>
    </>
  )
}

const extIcon = (k: string) => (k === 'bad' ? 'i-flag' : k === 'warn' ? 'i-alert' : 'i-check')
const TASK_STATUS: Record<string, string> = {
  proposed: 'Menunggu persetujuan',
  pending: 'Menunggu persetujuan',
  approved: 'Di Basecamp',
  sent: 'Di Basecamp',
  executed: 'Di Basecamp',
  done: 'Selesai',
  rejected: 'Ditolak',
}

function ContextPane({ ctx }: { ctx: ChatContext }) {
  const { go, openSheet, toast, refresh } = useUI()

  if (ctx.kind === 'internal') {
    return (
      <>
        <div>
          <h3>Konteks</h3>
          <div className="box" style={{ fontSize: 13, lineHeight: 1.55, color: 'var(--text-2)' }}>{ctx.note}</div>
        </div>
        {!!ctx.internal_members?.length && (
          <div>
            <h3>Anggota internal</h3>
            <ul className="mem">
              {ctx.internal_members.map((m, i) => <li key={m.n + i}>{m.n}<span className="pill neutral">{m.unit}</span></li>)}
            </ul>
          </div>
        )}
      </>
    )
  }

  const sendTask = (id: string) => {
    api.post<{ toast: string }>('/api/chat/tasks/' + encodeURIComponent(id) + '/send')
      .then(r => { toast(r.toast); refresh() })
      .catch(e => toast(errMsg(e)))
  }

  const a = ctx.account
  const accBox = a && (
    <div>
      <h3>Akun</h3>
      <div className="box acc">
        <Ring h={a.health} />
        <div><b>{a.name}</b><span>{a.line}</span></div>
      </div>
      {a.action && (
        <button className="btn ghost" style={{ width: '100%', justifyContent: 'center', marginTop: 8 }} onClick={() => openSheet(a.action!)}>
          <Icon n={a.action.icon} />{a.action.title}
        </button>
      )}
      <button className="btn quiet" style={{ width: '100%', justifyContent: 'center', marginTop: 4 }} onClick={() => go('rel:' + a.id)}>
        Buka halaman akun <Icon n="i-arrow" />
      </button>
    </div>
  )

  if (ctx.kind === 'group') {
    return (
      <>
        {accBox}
        {ctx.project && (
          <div>
            <h3>Project</h3>
            <div className="box">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <b style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{ctx.project.id}</b>
                <span className="pill accent">{ctx.project.stage}</span>
              </div>
              <div style={{ fontSize: 12.5, color: 'var(--text-2)', marginTop: 6 }}>Berikutnya: {ctx.project.next}</div>
            </div>
          </div>
        )}
        {!!ctx.summary?.length && (
          <div>
            <h3>Ringkasan hari ini <span className="ai" style={{ marginLeft: 4 }} /></h3>
            <ul className="sum">{ctx.summary.map((x, i) => <li key={i}>{x}</li>)}</ul>
          </div>
        )}
        {!!ctx.tasks?.length && (
          <div>
            <h3>Tugas terdeteksi</h3>
            <div className="box">
              {ctx.tasks.map(tk => (
                <div className="task" key={tk.id}>
                  <div><b>{tk.t}</b><span>{tk.who} · dari {tk.src}</span></div>
                  {tk.status === 'detected'
                    ? <button className="btn ghost" onClick={() => sendTask(tk.id)}>Ke Basecamp</button>
                    : <span className="pill neutral">{TASK_STATUS[tk.status] ?? tk.status}</span>}
                </div>
              ))}
            </div>
          </div>
        )}
        {!!ctx.members?.length && (
          <div>
            <h3>Anggota</h3>
            <ul className="mem">
              {ctx.members.map((m, i) => (
                <li key={m.n + i}>
                  {m.n} <span style={{ color: 'var(--text-3)' }}>· {m.r}</span>
                  <span className={'pill ' + (m.int ? 'neutral' : 'accent')}>{m.int ? 'internal' : 'eksternal'}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </>
    )
  }

  const extracted = ctx.extracted ?? []
  const commits = ctx.open_commitments ?? []
  return (
    <>
      {accBox}
      <div>
        <h3>Diekstrak dari chat ini <span className="ai" style={{ marginLeft: 4 }} /></h3>
        <ul className="ext">
          {extracted.length
            ? extracted.map((x, i) => (
                <li key={i}>
                  <Icon n={extIcon(x.k)} />
                  <span>{x.t}<br /><span style={{ color: 'var(--text-3)', fontSize: 11.5 }}>{x.tm}</span></span>
                </li>
              ))
            : <li><span style={{ color: 'var(--text-3)' }}>Belum ada</span></li>}
        </ul>
      </div>
      <div>
        <h3>Komitmen terbuka</h3>
        <div className="box">
          {commits.length
            ? commits.map((x, i) => (
                <div className="task" key={i}>
                  <div><b>{x.t}</b><span>{x.s}</span></div>
                  <Pill p={x.pill} />
                </div>
              ))
            : <span style={{ fontSize: 12.5, color: 'var(--text-3)' }}>Tidak ada</span>}
        </div>
      </div>
    </>
  )
}
