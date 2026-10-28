import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MessageView } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { hhmm, shortDate } from '../../lib/format'
import { discounted, fmtPct, parsePct, quoteText, rupiah, sendable, startQty, totals } from './lyra'
import './lyra.css'

/** Lyra di bawah bubble (ADR 0070): the products a customer asks about — in words or with a photo — read by the
 * company's Hermes Agent, with the stock per city and warehouse and the DPP price from GSI Orbit. The sales sets a
 * discount (prices and total follow), then sends the message after editing and confirming it. */
export function LyraCard({ m, thread, onAsk }: { m: MessageView; thread: string; onAsk: () => void }) {
  const c = m.lyra!
  const qc = useQueryClient()
  const { openSheet } = useFeedback()
  const [pctText, setPctText] = useState('')
  const [qty, setQty] = useState<Record<string, number>>({})
  const pct = parsePct(pctText)
  // the card refreshes on the chat's live events; a slow poll covers a missed one while Lyra reads
  useEffect(() => {
    if (c.status !== 'pending') return
    const t = window.setInterval(() => qc.invalidateQueries({ queryKey: ['chat', 'thread', thread] }), 4000)
    return () => window.clearInterval(t)
  }, [c.status, qc, thread])

  if (c.status === 'pending') {
    return (
      <div className="lyra">
        <Tag />
        <div className="lyra-wait"><Icon name="spark" />Lyra sedang membaca {m.media?.kind === 'image' ? 'foto dan pesan' : 'pesan'} ini…</div>
      </div>
    )
  }
  const rows = c.items.map((line) => ({ line, qty: qty[line.sku] ?? startQty(line) }))
  const sum = totals(rows, pct)
  const send = () => openSheet(<LyraSendSheet m={m} thread={thread} text={quoteText(rows, pct)} />)
  return (
    <div className={`lyra${c.status === 'error' ? ' is-err' : ''}`}>
      <Tag onAsk={onAsk} />
      {c.status === 'error' && <div className="lyra-n bad"><Icon name="alert" />{c.note || 'Lyra gagal membaca pesan ini'}</div>}
      {c.status === 'done' && c.items.length === 0 && <div className="lyra-n">{c.note || 'Lyra tidak menemukan barang di katalog untuk pesan ini.'}</div>}
      {rows.map((r, i) => (
        <div className="lyra-it" key={r.line.sku}>
          <div className="lyra-it-h"><b>{r.line.product}</b><small>{r.line.sku}</small></div>
          <div className="lyra-stock">
            <span className={`lyra-tot ${r.line.stock > 0 ? (r.line.asked > r.line.stock ? 'warn' : 'good') : 'bad'}`}>
              {r.line.stock > 0 ? `Stok ${r.line.stock.toLocaleString('id-ID')}` : 'Stok kosong'}
            </span>
            {r.line.cities.map((ct) => (
              <span className="lyra-city" key={ct.city} title={ct.warehouses?.map((w) => `${w.name}: ${w.qty.toLocaleString('id-ID')}`).join('\n')}>
                <b>{ct.city}</b> {ct.qty.toLocaleString('id-ID')}
                {ct.warehouses && ct.warehouses.length > 0 && <small> ({ct.warehouses.map((w) => `${w.name} ${w.qty.toLocaleString('id-ID')}`).join(' · ')})</small>}
              </span>
            ))}
          </div>
          <div className="lyra-price">
            {r.line.price > 0 ? (
              <>
                <span>DPP {pct > 0 && <s>{rupiah(r.line.price)}</s>} <b>{rupiah(discounted(r.line.price, pct))}</b>/unit</span>
                <span className="lyra-calc">
                  <label className="lyra-qty">× <input type="number" min={1} max={100000} value={r.qty} aria-label={`Jumlah ${r.line.product}`}
                    onChange={(e) => setQty({ ...qty, [r.line.sku]: Math.max(1, Math.min(100000, Math.round(Number(e.target.value) || 1))) })} /> unit</label>
                  <span className="lyra-sub">= <b>{rupiah(sum.lines[i])}</b></span>
                </span>
              </>
            ) : <span className="lyra-n">Harga belum ada di data</span>}
          </div>
          {r.line.asked > 0 && <small className="lyra-asked">Diminta pelanggan: {r.line.asked.toLocaleString('id-ID')} unit</small>}
        </div>
      ))}
      {sendable(c) && (
        <div className="lyra-ft">
          <label className="lyra-disc">Diskon
            <input inputMode="decimal" value={pctText} placeholder="0" aria-label="Diskon dalam persen" onChange={(e) => setPctText(e.target.value)} />%
          </label>
          <span className="lyra-total">
            Total {pct > 0 && sum.before > 0 && <s>{rupiah(sum.before)}</s>} <b>{rupiah(sum.total)}</b>
            {pct > 0 && sum.before > 0 && <small>hemat {rupiah(sum.before - sum.total)} ({fmtPct(pct)}%)</small>}
          </span>
          <button type="button" className="btn primary lyra-send" onClick={send}><Icon name="send" />Kirim ke pelanggan</button>
        </div>
      )}
      {c.status === 'done' && c.items.length > 0 && c.note && <div className="lyra-n">Lyra: {c.note}</div>}
      <small className="lyra-meta">
        Harga DPP (belum PPN) · {c.tier ? `tier ${c.tier}` : 'harga list (tier A) — chat belum tertaut dealer'} · stok dari GSI Orbit
        {c.read_at ? ` · dibaca ${shortDate(c.read_at)} ${hhmm(c.read_at)}` : ''}{c.demo ? ' · mode demo' : ''} · terkirim hanya setelah Anda konfirmasi
      </small>
    </div>
  )
}

function Tag({ onAsk }: { onAsk?: () => void }) {
  return (
    <div className="lyra-tag">
      <Icon name="spark" />Lyra · Hermes Agent<span>· hanya terlihat tim</span>
      {onAsk && <button type="button" className="lyra-again" onClick={onAsk} title="Minta Lyra membaca pesan ini lagi"><Icon name="refresh" />Baca ulang</button>}
    </div>
  )
}

/** The message to the customer, editable; nothing is sent until the sales presses Kirim. */
function LyraSendSheet({ m, thread, text }: { m: MessageView; thread: string; text: string }) {
  const qc = useQueryClient()
  const { toast, closeSheet } = useFeedback()
  const [body, setBody] = useState(text)
  const send = useMutation({
    mutationFn: () => api.post(`/chat/threads/${thread}/messages`, m.can_reply ? { body: body.trim(), reply_to: m.id } : { body: body.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['chat'] })
      toast('Info produk masuk antrean kirim · lewat penjaga anti-blokir')
      closeSheet()
    },
    onError: (e: Error) => toast(e.message),
  })
  const who = m.from_name || 'pelanggan'
  return (
    <>
      <SheetHead icon="send" title={`Kirim info produk ke ${who}`} sub="Periksa dan ubah pesannya dulu — baru terkirim setelah Anda menekan Kirim" onClose={closeSheet} />
      <div className="sec lyra-sheet">
        <textarea value={body} onChange={(e) => setBody(e.target.value)} rows={14} aria-label="Pesan untuk pelanggan" autoFocus />
        <small>{m.can_reply ? 'Dikirim sebagai balasan pesan pelanggan ini' : 'Dikirim ke chat ini'} · *tebal* dan ~coret~ tampil seperti di WhatsApp</small>
      </div>
      <div className="ft">
        <button className="btn quiet" onClick={closeSheet}>Batal</button>
        <span className="spacer" />
        <button className="btn primary" disabled={!body.trim() || send.isPending} onClick={() => send.mutate()}><Icon name="send" />{send.isPending ? 'Mengirim…' : `Kirim ke ${who}`}</button>
      </div>
    </>
  )
}

/** Whether a bubble can be given to Lyra: a customer's text or photo. */
export const lyraAskable = (m: MessageView) => m.direction === 'in' && !m.internal && !m.deleted && (!m.media || m.media.kind === 'image')
