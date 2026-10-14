import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ReplyRule, ReplySuggestion } from '../../api/types'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { useMe } from '../../app/queries'

// Pengaturan → Rekomendasi balasan (ADR 0038): a customer message with one of a rule's words gets a suggested
// answer from the data under its bubble in Chat. Nothing is sent automatically.

const useReplyRules = () => useQuery({ queryKey: ['reply-rules'], queryFn: () => api.get<{ items: ReplyRule[] }>('/reply-rules').then((r) => r.items) })

const PLACEHOLDERS = '{produk} nama produk · {jumlah} stok di cabang chat · {cabang} cabang chat · {cabang_lain} cabang lain yang ada stoknya'

function RuleSheet({ rule }: { rule?: ReplyRule }) {
  const { closeSheet, toast } = useFeedback()
  const qc = useQueryClient()
  const [name, setName] = useState(rule?.name ?? 'Tanya stok')
  const [keywords, setKeywords] = useState((rule?.keywords ?? ['stok', 'ready', 'ada barang', 'tersedia']).join(', '))
  const [ready, setReady] = useState(rule?.template_ready ?? 'Ready {jumlah} pcs untuk {produk} di {cabang}.')
  const [empty, setEmpty] = useState(rule?.template_empty ?? 'Mohon maaf, {produk} di {cabang} sedang kosong.')
  const [showQty, setShowQty] = useState(rule?.show_qty ?? true)
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const save = useMutation({
    mutationFn: () => {
      const body = { name, action: 'stock', keywords: keywords.split(',').map((k) => k.trim()).filter(Boolean), template_ready: ready, template_empty: empty, show_qty: showQty, enabled }
      return rule ? api.put(`/reply-rules/${rule.id}`, body) : api.post('/reply-rules', body)
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['reply-rules'] }); qc.invalidateQueries({ queryKey: ['chat'] }); toast('Aturan disimpan'); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const del = useMutation({
    mutationFn: () => api.del(`/reply-rules/${rule!.id}`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['reply-rules'] }); toast('Aturan dihapus'); closeSheet() },
    onError: (e: Error) => toast(e.message),
  })
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  return (
    <form onSubmit={submit}>
      <SheetHead icon="box" title={rule ? `Ubah · ${rule.name}` : 'Aturan baru'} sub="Pesan pelanggan yang memuat salah satu kata kunci mendapat rekomendasi jawaban dari data stok" onClose={closeSheet} />
      <div className="sec frm rr-form">
        <label>Nama aturan<input value={name} onChange={(e) => setName(e.target.value)} required /></label>
        <label>Jenis<select value="stock" disabled><option value="stock">Cek stok produk yang disebut di pesan</option></select></label>
        <label className="full">Kata kunci (pisahkan dengan koma)<input value={keywords} onChange={(e) => setKeywords(e.target.value)} placeholder="stok, ready, ada barang" required /></label>
        <label className="full">Draf bila stok ready<input value={ready} onChange={(e) => setReady(e.target.value)} required /></label>
        <label className="full">Draf bila stok kosong<input value={empty} onChange={(e) => setEmpty(e.target.value)} required /></label>
        <small className="full">{PLACEHOLDERS}</small>
        <label className="chk full"><input type="checkbox" checked={showQty} onChange={(e) => setShowQty(e.target.checked)} />Sebutkan jumlah stok di draf ke pelanggan (bila tidak: cukup "ready"). Sales tetap melihat jumlahnya.</label>
        <label className="chk full"><input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />Aktif</label>
      </div>
      <div className="ft">
        <button className="btn primary" type="submit" disabled={save.isPending}><Icon name="check" />Simpan</button>
        {rule && <button className="btn quiet" type="button" disabled={del.isPending} onClick={() => window.confirm(`Hapus aturan "${rule.name}"?`) && del.mutate()}>Hapus</button>}
        <span className="spacer" />
        <button className="btn quiet" type="button" onClick={closeSheet}>Batal</button>
      </div>
    </form>
  )
}

/** Try a customer sentence against the enabled rules and today's stock. */
function TryIt() {
  const [text, setText] = useState('Pak, kamera 4MP ready?')
  const [branch, setBranch] = useState('')
  const [res, setRes] = useState<ReplySuggestion | null | undefined>(undefined)
  const run = useMutation({
    mutationFn: () => api.post<{ suggestion: ReplySuggestion | null }>('/reply-rules/test', { text, branch }),
    onSuccess: (r) => setRes(r.suggestion),
  })
  return (
    <div className="rr-try">
      <b>Coba kalimat pelanggan</b>
      <div className="rr-try-row">
        <input value={text} onChange={(e) => setText(e.target.value)} aria-label="Kalimat pelanggan" />
        <input value={branch} onChange={(e) => setBranch(e.target.value)} placeholder="Cabang (opsional)" aria-label="Cabang" style={{ maxWidth: 160 }} />
        <button className="btn ghost" type="button" onClick={() => run.mutate()} disabled={run.isPending}><Icon name="search" />Coba</button>
      </div>
      {res === null && <p className="rr-res muted">Tidak ada aturan yang cocok — tidak ada rekomendasi.</p>}
      {res && (
        <p className="rr-res">
          <Pill tone={res.product ? (res.ready ? 'good' : 'bad') : 'neutral'}>{res.product ? (res.ready ? `Ready ${res.qty}` : 'Kosong') : 'Produk tidak dikenali'}</Pill>
          {res.product && <span> {res.product}{res.others.length ? ` · cabang lain: ${res.others.map((o) => `${o.branch} ${o.qty}`).join(', ')}` : ''}</span>}
          {res.draft && <span className="rr-draft">“{res.draft}”</span>}
          {res.note && <span className="muted"> {res.note}</span>}
        </p>
      )}
    </div>
  )
}

/** Pengaturan card: the rules, their keywords, and a try box. */
export function ReplyRulesCard() {
  const { data: me } = useMe()
  const { data: rules = [] } = useReplyRules()
  const { openSheet } = useFeedback()
  const edit = !!me && (me.screens.includes('users') || me.screens.includes('conn'))
  return (
    <div className="card">
      <div className="card-h">
        <h2>Rekomendasi balasan</h2>
        {edit && <button className="btn primary" style={{ marginLeft: 'auto', height: 32, fontSize: 12.5 }} onClick={() => openSheet(<RuleSheet />)}><Icon name="check" />Aturan baru</button>}
      </div>
      <p className="rr-intro">Pesan pelanggan yang memuat kata kunci mendapat rekomendasi jawaban dari data stok di bawah bubble Chat. Sales menekan <b>Pakai jawaban</b>, mengedit, lalu mengirim sendiri — tidak pernah terkirim otomatis.</p>
      {rules.length === 0 ? <p className="cl-empty">Belum ada aturan.</p> : (
        <ul className="rr-list">
          {rules.map((r) => (
            <li key={r.id} className={r.enabled ? '' : 'off'}>
              <div>
                <span className="rr-name"><b>{r.name}</b><Pill tone={r.enabled ? 'good' : 'neutral'}>{r.enabled ? 'Aktif' : 'Mati'}</Pill></span>
                <span className="rr-kw">{r.keywords.map((k) => <code key={k}>{k}</code>)}</span>
                <span className="rr-tpl">Ready: “{r.template_ready}” · Kosong: “{r.template_empty}”</span>
              </div>
              {edit && <button className="btn quiet" onClick={() => openSheet(<RuleSheet rule={r} />)}>Ubah</button>}
            </li>
          ))}
        </ul>
      )}
      <TryIt />
    </div>
  )
}
