import { Fragment, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { Markdown } from '../../components/Markdown'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { hhmm, shortDate } from '../../lib/format'
import { useMe } from '../../app/queries'
import './hermes-chat.css'

export interface HermesMessage { id: string; role: 'user' | 'assistant'; status: 'pending' | 'done' | 'error'; content: string; demo: boolean; created_at: string; finished_at: string | null }
export interface HermesConfig { configured: boolean; url: string; key_hint: string }
interface HermesChatData { config: HermesConfig; messages: HermesMessage[] }

const KEY = ['mcp', 'hermes', 'chat']

/** The conversation; polls while Lyra is still answering. */
export const useHermesChat = () => useQuery({
  queryKey: KEY,
  queryFn: () => api.get<HermesChatData>('/mcp/hermes/chat'),
  refetchInterval: (q) => (q.state.data?.messages.some((m) => m.status === 'pending') ? 1500 : false),
})

const STARTERS = [
  'Ringkas dealer yang perlu ditindaklanjuti hari ini',
  'Produk apa yang paling laku 90 hari terakhir?',
  'Barang mana yang stoknya menumpuk lebih dari 90 hari?',
  'Dealer mana yang piutangnya berisiko macet?',
]

const day = (iso: string) => new Date(iso).toDateString()

function Bubble({ m }: { m: HermesMessage }) {
  const { toast } = useFeedback()
  const mine = m.role === 'user'
  if (m.status === 'pending') {
    return <div className="hc-row"><div className="hc-bubble is-typing" aria-label="Lyra sedang mengetik"><i /><i /><i /></div></div>
  }
  const copy = () => void navigator.clipboard?.writeText(m.content).then(() => toast('Disalin'), () => undefined)
  return (
    <div className={`hc-row ${mine ? 'is-mine' : ''}`}>
      <div className={`hc-bubble ${m.status === 'error' ? 'is-error' : ''}`}>
        {m.status === 'error' && <b className="hc-err"><Icon name="alert" />Lyra gagal menjawab</b>}
        {mine ? <p>{m.content}</p> : m.status === 'error' ? <p>{m.content.charAt(0).toUpperCase() + m.content.slice(1)}</p> : <Markdown text={m.content} />}
        <span className="hc-meta">
          {!mine && m.status === 'done' && <button onClick={copy} title="Salin jawaban"><Icon name="doc" /></button>}
          {m.demo && !mine && 'demo lama · '}{hhmm(m.finished_at ?? m.created_at)}{mine && <Icon name="check" />}
        </span>
      </div>
    </div>
  )
}

/** Hermes Agent → Chat: a window to Lyra, the company's Hermes Agent (ADR 0067). Each question goes to Lyra as is;
 * Lyra keeps the conversation, memory and tools, like in Telegram. Without a connected Lyra nothing can be asked. */
export function HermesChat({ onSettings }: { onSettings: () => void }) {
  const { data, isLoading } = useHermesChat()
  const { data: me } = useMe()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [text, setText] = useState('')
  const list = useRef<HTMLDivElement>(null)
  const box = useRef<HTMLTextAreaElement>(null)
  const msgs = data?.messages ?? []
  const pending = msgs.some((m) => m.status === 'pending')
  const cfg = data?.config
  const off = !isLoading && !cfg?.configured

  const ask = useMutation({
    mutationFn: (t: string) => api.post('/mcp/hermes/chat', { text: t }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
    onError: (e: Error, t) => { setText(t); toast(e.message) },
  })
  const clear = useMutation({
    mutationFn: () => api.del('/mcp/hermes/chat'),
    onSuccess: () => { toast('Riwayat chat dihapus'); qc.invalidateQueries({ queryKey: KEY }) },
    onError: (e: Error) => toast(e.message),
  })

  useEffect(() => {
    const el = list.current
    if (el) el.scrollTop = el.scrollHeight
  }, [msgs.length, pending])

  useEffect(() => {
    const el = box.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 160) + 'px'
  }, [text])

  const send = (t = text) => {
    const v = t.trim()
    if (!v || off || pending || ask.isPending) return
    setText('')
    ask.mutate(v)
  }

  const status = pending ? 'sedang mengetik…' : cfg?.configured ? 'online · Hermes Agent' : 'belum disambungkan'
  return (
    <div className="card hc">
      <header className="hc-head">
        <span className="hc-avatar"><Icon name="spark" /></span>
        <div>
          <b>Lyra</b>
          <span className={pending ? 'is-typing' : ''}>{status}</span>
        </div>
        {off && <Pill tone="warn">Belum tersambung</Pill>}
        <button className="btn ghost" disabled={!msgs.length || clear.isPending} onClick={() => { if (window.confirm('Hapus semua riwayat chat Anda dengan Lyra?')) clear.mutate() }}><Icon name="x" />Hapus riwayat</button>
      </header>

      <div className="hc-list" ref={list} aria-live="polite">
        {off && (
          <div className="hc-note">
            Lyra belum disambungkan ke GSI Orbit, jadi pertanyaan belum bisa dikirim.{' '}
            {me?.role === 'ceo' ? <button className="link" onClick={onSettings}>Sambungkan Lyra</button> : 'Minta CEO menyambungkan Lyra di tab Sambungan.'}
          </div>
        )}
        {!msgs.length && !isLoading && (
          <div className="hc-empty">
            <span className="hc-avatar is-big"><Icon name="spark" /></span>
            <b>Mulai chat dengan Lyra</b>
            <span>Pertanyaan Anda dikirim apa adanya ke Lyra, dan jawabannya tampil di sini, sama seperti di Telegram.</span>
            <div className="hc-starters">
              {STARTERS.map((s) => <button key={s} className="chip" disabled={off} onClick={() => send(s)}>{s}</button>)}
            </div>
          </div>
        )}
        {msgs.map((m, i) => (
          <Fragment key={m.id}>
            {(i === 0 || day(m.created_at) !== day(msgs[i - 1].created_at)) && <div className="hc-day"><span>{shortDate(m.created_at)}</span></div>}
            <Bubble m={m} />
          </Fragment>
        ))}
      </div>

      <form className="hc-compose" onSubmit={(e) => { e.preventDefault(); send() }}>
        <textarea
          ref={box}
          rows={1}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); send() } }}
          disabled={off}
          placeholder={off ? 'Lyra belum tersambung' : pending ? 'Lyra sedang menjawab…' : 'Tulis pesan ke Lyra'}
          aria-label="Pesan ke Lyra"
          maxLength={8000}
        />
        <button type="submit" className="hc-send" disabled={off || !text.trim() || pending || ask.isPending} aria-label="Kirim"><Icon name="send" /></button>
      </form>
      <p className="hc-foot">Lyra menyimpan percakapannya sendiri, sama seperti di Telegram. Lyra hanya membaca dan mengusulkan. Keputusan dan pengiriman ke dealer tetap dilakukan manusia di GSI Orbit. Riwayat chat hanya terlihat oleh Anda.</p>
    </div>
  )
}

/** Sambungan tab: the CEO connects this page to Lyra's Hermes API server (URL + API_SERVER_KEY). */
export function HermesChatConnect() {
  const { data } = useHermesChat()
  const { data: me } = useMe()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const cfg = data?.config
  const [url, setUrl] = useState('')
  const [key, setKey] = useState('')
  const ceo = me?.role === 'ceo'
  const save = useMutation({
    mutationFn: () => api.put<HermesConfig>('/mcp/hermes/config', { url: url || cfg?.url, api_key: key }),
    onSuccess: () => { toast('Lyra tersambung'); setKey(''); setUrl(''); qc.invalidateQueries({ queryKey: KEY }) },
    onError: (e: Error) => toast(e.message),
  })
  const drop = useMutation({
    mutationFn: () => api.del('/mcp/hermes/config'),
    onSuccess: () => { toast('Sambungan ke Lyra diputus'); qc.invalidateQueries({ queryKey: KEY }) },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <div className="card">
      <div className="card-h"><h2>Sambungan ke Lyra</h2><Pill tone={cfg?.configured ? 'good' : 'warn'} icon={cfg?.configured ? 'check' : 'alert'}>{cfg?.configured ? 'Tersambung' : 'Belum tersambung'}</Pill></div>
      <p className="hc-cfg">Sambungan ini supaya GSI Orbit bisa bertanya ke Lyra (chat di halaman ini dan kartu Lyra di Chat WhatsApp). Token MCP yang dipakai Lyra untuk membaca data GSI Orbit adalah hal terpisah, diatur di <b>MCP Claude → Hermes Agent</b>.</p>
      {cfg?.configured && <p className="hc-cfg">Server: <code>{cfg.url}</code> · API key <code>{cfg.key_hint}</code></p>}
      <ol className="cl-steps">
        <li><b>Nyalakan API server Hermes</b> di tempat Lyra berjalan: tambahkan ke <code>~/.hermes/.env</code>, lalu restart <code>hermes gateway</code> (Telegram tetap jalan):
          <pre className="hc-pre">{'API_SERVER_ENABLED=true\nAPI_SERVER_KEY=<rahasia panjang acak>\nAPI_SERVER_HOST=127.0.0.1'}</pre></li>
        <li><b>Isi URL dan API key</b> di bawah. Bila Lyra berjalan di server yang sama dengan GSI Orbit, URL-nya <code>http://127.0.0.1:8642</code>. GSI Orbit memeriksanya ke Lyra sebelum disimpan (terenkripsi).</li>
      </ol>
      {ceo ? (
        <form className="hc-form" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <input className="cl-input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder={cfg?.url || 'http://127.0.0.1:8642'} aria-label="URL API server Lyra" />
          <input className="cl-input" type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder={cfg?.configured ? `API key (kosongkan = tetap ${cfg.key_hint})` : 'API_SERVER_KEY'} aria-label="API key Lyra" />
          <button className="btn primary" disabled={save.isPending || !(url || cfg?.url) || (!key && !cfg?.configured)}><Icon name="plug" />{save.isPending ? 'Memeriksa…' : 'Periksa & simpan'}</button>
          {cfg?.configured && <button type="button" className="btn ghost" onClick={() => { if (window.confirm('Putuskan sambungan ke Lyra? Chat tidak bisa dipakai, dan kartu Lyra kembali ke mode demo.')) drop.mutate() }}>Putuskan</button>}
        </form>
      ) : <p className="hc-cfg">Hanya CEO yang bisa menyambungkan Lyra.</p>}
    </div>
  )
}
