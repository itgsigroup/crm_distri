import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { QuickReply } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { cleanShortcut, validShortcut } from './quickReplyText'

// Balasan cepat (ADR 0065), like WhatsApp Business: the team's saved texts, typed with "/" in Chat.

export const useQuickReplies = (enabled = true) => useQuery({
  queryKey: ['chat', 'quick-replies'],
  queryFn: () => api.get<{ items: QuickReply[] }>('/chat/quick-replies'),
  staleTime: 60_000,
  enabled,
})

/** A pick is counted so the most used come first (a miss changes nothing for the person). */
export const countQuickUse = (id: string) => { api.post(`/chat/quick-replies/${id}/use`, {}).catch(() => {}) }

/** The add / edit form of one quick reply. */
function QuickForm({ init, onDone, onCancel }: { init?: QuickReply; onDone: () => void; onCancel?: () => void }) {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [shortcut, setShortcut] = useState(init?.shortcut ?? '')
  const [body, setBody] = useState(init?.body ?? '')
  const sc = cleanShortcut(shortcut)
  const ok = validShortcut(sc) && body.trim().length > 0 && body.length <= 2000
  const save = useMutation({
    mutationFn: () => (init ? api.put(`/chat/quick-replies/${init.id}`, { shortcut: sc, body }) : api.post('/chat/quick-replies', { shortcut: sc, body })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['chat', 'quick-replies'] })
      toast(init ? `Balasan cepat /${sc} diubah` : `Balasan cepat /${sc} disimpan — ketik /${sc} di chat`)
      if (!init) { setShortcut(''); setBody('') }
      onDone()
    },
    onError: (e: Error) => toast(e.message),
  })
  return (
    <form className="qr-form" onSubmit={(e) => { e.preventDefault(); if (ok) save.mutate() }}>
      <label className="qr-sc">
        <span>/</span>
        <input value={shortcut} onChange={(e) => setShortcut(e.target.value)} maxLength={25} placeholder="pintasan, mis. rekening" aria-label="Pintasan" autoFocus={!!init} />
      </label>
      <textarea value={body} onChange={(e) => setBody(e.target.value)} rows={3} maxLength={2000} placeholder="Isi pesan, mis. Transfer ke BCA 123 456 7890 a.n. PT Gosyen Solusi Indonesia" aria-label="Isi pesan" />
      <div className="qr-form-ft">
        <small>{shortcut && !validShortcut(sc) ? 'Pintasan: huruf, angka, - atau _ (maks. 24)' : `${body.length}/2000`}</small>
        <span className="spacer" />
        {onCancel && <button type="button" className="btn quiet" onClick={onCancel}>Batal</button>}
        <button type="submit" className="btn primary" disabled={!ok || save.isPending}><Icon name="check" />{init ? 'Simpan' : 'Tambah'}</button>
      </div>
    </form>
  )
}

/** Kelola balasan cepat: the list (pick one to put it in the box), add, edit or delete one's own (admins: any). */
export function QuickReplySheet({ onPick }: { onPick?: (q: QuickReply) => void }) {
  const { data, isLoading } = useQuickReplies()
  const qc = useQueryClient()
  const { toast, closeSheet } = useFeedback()
  const [editing, setEditing] = useState<string | null>(null)
  const del = useMutation({
    mutationFn: (q: QuickReply) => api.del(`/chat/quick-replies/${q.id}`),
    onSuccess: (_, q) => { qc.invalidateQueries({ queryKey: ['chat', 'quick-replies'] }); toast(`Balasan cepat /${q.shortcut} dihapus`) },
    onError: (e: Error) => toast(e.message),
  })
  const items = data?.items ?? []
  return (
    <>
      <SheetHead icon="send" title="Balasan cepat" sub="Ketik / di kolom chat untuk memakai — teks hanya mengisi kolom, Anda tetap menekan Kirim" onClose={closeSheet} />
      <div className="sec qr-sheet">
        <QuickForm onDone={() => {}} />
        {isLoading ? <p className="qr-empty">Memuat…</p> : !items.length ? (
          <p className="qr-empty">Belum ada balasan cepat. Simpan teks yang sering dikirim — nomor rekening, alamat gudang, salam pembuka — lalu ketik <b>/</b> di chat.</p>
        ) : (
          <ul className="qr-list">
            {items.map((q) => (
              <li key={q.id}>
                {editing === q.id ? <QuickForm init={q} onDone={() => setEditing(null)} onCancel={() => setEditing(null)} /> : (
                  <>
                    <button type="button" className="qr-item" disabled={!onPick} onClick={() => { if (onPick) { onPick(q); closeSheet() } }} title={onPick ? 'Pakai di chat ini' : undefined}>
                      <b>/{q.shortcut}</b>
                      <span>{q.body}</span>
                      <small>{q.by ? `oleh ${q.by}` : ''}{q.uses ? ` · dipakai ${q.uses}×` : ''}</small>
                    </button>
                    {q.can_edit && (
                      <span className="qr-acts">
                        <button type="button" className="ibl-btn" onClick={() => setEditing(q.id)} title={`Ubah /${q.shortcut}`} aria-label={`Ubah /${q.shortcut}`}><Icon name="edit" /></button>
                        <button type="button" className="ibl-btn" onClick={() => { if (window.confirm(`Hapus balasan cepat /${q.shortcut} untuk semua tim?`)) del.mutate(q) }} title={`Hapus /${q.shortcut}`} aria-label={`Hapus /${q.shortcut}`}><Icon name="x" /></button>
                      </span>
                    )}
                  </>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  )
}
