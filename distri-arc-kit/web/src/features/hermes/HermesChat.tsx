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

/** The conversation; polls while Hermes is still answering. */
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
    return <div className="hc-row"><div className="hc-bubble is-typing" aria-label="Hermes sedang mengetik"><i /><i /><i /></div></div>
  }
  const copy = () => void navigator.clipboard?.writeText(m.content).then(() => toast('Disalin'), () => undefined)
  return (
    <div className={`hc-row ${mine ? 'is-mine' : ''}`}>
      <div className={`hc-bubble ${m.status === 'error' ? 'is-error' : ''}`}>
        {m.status === 'error' && <b className="hc-err"><Icon name="alert" />Hermes gagal menjawab</b>}
        {mine ? <p>{m.content}</p> : m.status === 'error' ? <p>{m.content.charAt(0).toUpperCase() + m.content.slice(1)}</p> : <Markdown text={m.content} />}
        <span className="hc-meta">
          {!mine && m.status === 'done' && <button onClick={copy} title="Salin jawaban"><Icon name="doc" /></button>}
          {m.demo && !mine && 'demo · '}{hhmm(m.finished_at ?? m.created_at)}{mine && <Icon name="check" />}
        </span>
      </div>
    </div>
  )
}

/** Hermes Agent → Chat: talk to the company's Hermes Agent like in Telegram (ADR 0067). */
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
    if (!v || pending || ask.isPending) return
    setText('')
    ask.mutate(v)
  }

  const status = pending ? 'sedang mengetik…' : cfg?.configured ? 'online · terhubung lewat API server' : 'mode demo · belum disambungkan'
  return (
    <div className="card hc">
      <header className="hc-head">
        <span className="hc-avatar"><Icon name="spark" /></span>
        <div>
          <b>Hermes Agent</b>
          <span className={pending ? 'is-typing' : ''}>{status}</span>
        </div>
        {!cfg?.configured && <Pill tone="warn">Demo</Pill>}
        <button className="btn ghost" disabled={!msgs.length || clear.isPending} onClick={() => { if (window.confirm('Hapus semua riwayat chat Anda dengan Hermes?')) clear.mutate() }}><Icon name="x" />Hapus riwayat</button>
      </header>

      <div className="hc-list" ref={list} aria-live="polite">
        {!cfg?.configured && !isLoading && (
          <div className="hc-note">
            Hermes belum disambungkan ke halaman ini, jadi jawaban masih <b>mode demo</b>.{' '}
            {me?.role === 'ceo' ? <button className="link" onClick={onSettings}>Sambungkan Hermes</button> : 'Minta CEO menyambungkan Hermes di tab Pengaturan.'}
          </div>
        )}
        {!msgs.length && !isLoading && (
          <div className="hc-empty">
            <span className="hc-avatar is-big"><Icon name="spark" /></span>
            <b>Mulai chat dengan Hermes</b>
            <span>Tanya apa saja tentang dealer, penjualan, piutang, dan stok. Hermes membaca data GSI Orbit lewat MCP lalu menjawab di sini — sama seperti di Telegram.</span>
            <div className="hc-starters">
              {STARTERS.map((s) => <button key={s} className="chip" onClick={() => send(s)}>{s}</button>)}
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
          placeholder={pending ? 'Hermes sedang menjawab…' : 'Tulis pesan ke Hermes'}
          aria-label="Pesan ke Hermes"
          maxLength={8000}
        />
        <button type="submit" className="hc-send" disabled={!text.trim() || pending || ask.isPending} aria-label="Kirim"><Icon name="send" /></button>
      </form>
      <p className="hc-foot">Hermes hanya membaca dan mengusulkan. Keputusan dan pengiriman ke dealer tetap dilakukan manusia di GSI Orbit. Riwayat chat hanya terlihat oleh Anda.</p>
    </div>
  )
}

/** Pengaturan tab: the CEO connects this page to the Hermes API server (URL + API_SERVER_KEY). */
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
    onSuccess: () => { toast('Hermes tersambung — chat sekarang memakai Hermes sungguhan'); setKey(''); setUrl(''); qc.invalidateQueries({ queryKey: KEY }) },
    onError: (e: Error) => toast(e.message),
  })
  const drop = useMutation({
    mutationFn: () => api.del('/mcp/hermes/config'),
    onSuccess: () => { toast('Sambungan chat Hermes diputus'); qc.invalidateQueries({ queryKey: KEY }) },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <div className="card">
      <div className="card-h"><h2>Chat Hermes di halaman ini</h2><Pill tone={cfg?.configured ? 'good' : 'warn'} icon={cfg?.configured ? 'check' : 'alert'}>{cfg?.configured ? 'Tersambung' : 'Mode demo'}</Pill></div>
      {cfg?.configured && <p className="hc-cfg">Server: <code>{cfg.url}</code> · API key <code>{cfg.key_hint}</code></p>}
      <ol className="cl-steps">
        <li><b>Nyalakan API server Hermes</b> di device Hermes — tambahkan ke <code>~/.hermes/.env</code>, lalu restart <code>hermes gateway</code> (Telegram tetap jalan):
          <pre className="hc-pre">{'API_SERVER_ENABLED=true\nAPI_SERVER_KEY=<rahasia panjang acak>\nAPI_SERVER_HOST=0.0.0.0'}</pre></li>
        <li><b>Buka jalur dari server GSI Orbit ke device Hermes</b>. Disarankan <b>Tailscale</b> (gratis, jaringan privat): pasang di server dan device Hermes, lalu pakai alamat <code>http://100.x.x.x:8642</code>. Alternatif: Cloudflare Tunnel (URL https).</li>
        <li><b>Isi URL dan API key</b> di bawah. GSI Orbit memeriksanya ke Hermes sebelum disimpan (terenkripsi).</li>
      </ol>
      {ceo ? (
        <form className="hc-form" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <input className="cl-input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder={cfg?.url || 'http://100.64.0.5:8642'} aria-label="URL API server Hermes" />
          <input className="cl-input" type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder={cfg?.configured ? `API key (kosongkan = tetap ${cfg.key_hint})` : 'API_SERVER_KEY'} aria-label="API key Hermes" />
          <button className="btn primary" disabled={save.isPending || !(url || cfg?.url) || (!key && !cfg?.configured)}><Icon name="plug" />{save.isPending ? 'Memeriksa…' : 'Periksa & simpan'}</button>
          {cfg?.configured && <button type="button" className="btn ghost" onClick={() => { if (window.confirm('Putuskan chat Hermes? Halaman chat kembali ke mode demo.')) drop.mutate() }}>Putuskan</button>}
        </form>
      ) : <p className="hc-cfg">Hanya CEO yang bisa menyambungkan chat Hermes.</p>}
    </div>
  )
}
