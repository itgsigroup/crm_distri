import { useState } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { Pill } from '../../components/ui'
import { fmtPhone } from '../chat/inbox'
import { DAYS, FACTS, KIND_LABEL, dotted, fill, windowsError, type AutoReplyInfo, type AutoReplySettings, type HourWindow } from './autoreply'

// Pengaturan → Chat otomatis (ADR 0066, ADR 0077): during the hours set here a customer's message is answered without
// the sales — a product question by Lyra (Hermes Agent) with the stock and DPP price from the system, another fact
// question by AI Trigger, anything else by the template once. Saving is the CEO's approval: every automatic message
// is recorded as decided by them.

const useAutoReply = () => useQuery({ queryKey: ['auto-reply'], queryFn: () => api.get<AutoReplyInfo>('/auto-reply') })

const STATUS: Record<string, string> = { pending: 'mengirim', sent: 'terkirim', delivered: 'diterima', read: 'dibaca', failed: 'gagal', skipped: 'tidak dikirim' }

const editable = (s: AutoReplySettings): AutoReplySettings => ({
  enabled: s.enabled, lyra: s.lyra, breaks: s.breaks, numbers: s.numbers ?? [], template: s.template, facts: s.facts ?? [],
})

function Hours({ breaks, set, edit, from, to }: { breaks: HourWindow[]; set: (b: HourWindow[]) => void; edit: boolean; from: string; to: string }) {
  const change = (i: number, w: Partial<HourWindow>) => set(breaks.map((x, j) => (j === i ? { ...x, ...w } : x)))
  const toggleDay = (i: number, d: number) => {
    const days = breaks[i].days.includes(d) ? breaks[i].days.filter((x) => x !== d) : [...breaks[i].days, d].sort()
    change(i, { days })
  }
  return (
    <div className="card">
      <div className="card-h">
        <h2>Jam aktif</h2>
        <span className="meta">WIB · di antara {dotted(from)}–{dotted(to)}</span>
      </div>
      <ul className="ar-breaks">
        {breaks.map((w, i) => (
          <li key={i}>
            <div className="ar-time">
              <label>Dari <input type="time" value={w.start} min={from} max={to} step={300} disabled={!edit} onChange={(e) => change(i, { start: e.target.value })} aria-label="Jam mulai" /></label>
              <label>sampai <input type="time" value={w.end} min={from} max={to} step={300} disabled={!edit} onChange={(e) => change(i, { end: e.target.value })} aria-label="Jam selesai" /></label>
              {edit && breaks.length > 1 && <button type="button" className="btn quiet" onClick={() => set(breaks.filter((_, j) => j !== i))}><Icon name="x" />Hapus</button>}
            </div>
            <div className="seg ar-days" role="group" aria-label="Hari">
              {DAYS.map(([d, name]) => (
                <button key={d} type="button" aria-pressed={w.days.includes(d)} className={w.days.includes(d) ? 'is-active' : ''} disabled={!edit} onClick={() => toggleDay(i, d)}>{name}</button>
              ))}
            </div>
          </li>
        ))}
      </ul>
      {edit && breaks.length < 7 && (
        <button type="button" className="btn ghost ar-add" onClick={() => set([...breaks, { days: [1, 2, 3, 4, 5, 6], start: '12:00', end: '13:00' }])}><Icon name="check" />Tambah jam aktif</button>
      )}
      <p className="ar-help">Di luar jam ini tidak ada pesan otomatis. Batas {dotted(from)}–{dotted(to)} WIB adalah jendela kirim WhatsApp otomatis (penjaga anti-blokir): pesan di luarnya baru terkirim besok pagi.</p>
    </div>
  )
}

function Numbers({ info, s, set, edit }: { info: AutoReplyInfo; s: AutoReplySettings; set: (n: string[]) => void; edit: boolean }) {
  const sales = info.numbers.filter((n) => n.team === 'sales')
  const all = s.numbers.length === 0
  return (
    <div className="card">
      <div className="card-h">
        <h2>Nomor yang ikut</h2>
        <span className="meta">{all ? 'semua nomor' : `${s.numbers.length} nomor`}</span>
      </div>
      <div className="seg" role="radiogroup" aria-label="Nomor">
        <button type="button" role="radio" aria-checked={all} className={all ? 'is-active' : ''} disabled={!edit} onClick={() => set([])}>Semua nomor</button>
        <button type="button" role="radio" aria-checked={!all} className={!all ? 'is-active' : ''} disabled={!edit || sales.length === 0} onClick={() => all && set([sales[0].wa_number])}>Pilih nomor</button>
      </div>
      {!all && (
        <ul className="ar-nums" style={{ marginTop: 12 }}>
          {sales.map((n) => (
            <li key={n.wa_number}>
              <label>
                <input type="checkbox" checked={s.numbers.includes(n.wa_number)} disabled={!edit}
                  onChange={(e) => set(e.target.checked ? [...s.numbers, n.wa_number] : s.numbers.filter((x) => x !== n.wa_number))} />
                <span><b>{n.holder || n.label || fmtPhone(n.wa_number)}</b><small>{fmtPhone(n.wa_number)}{n.label && n.holder ? ` · ${n.label}` : ''}{n.state !== 'connected' ? ' · tidak tersambung' : ''}</small></span>
              </label>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function Log({ info }: { info: AutoReplyInfo }) {
  return (
    <div className="card">
      <div className="card-h"><h2>Riwayat chat otomatis</h2><span className="meta">30 terakhir</span></div>
      {info.recent.length === 0 ? <p className="cl-empty">Belum ada pesan otomatis.</p> : (
        <ul className="ar-log">
          {info.recent.map((r) => (
            <li key={r.message_id}>
              <div className="ar-log-h">
                <Pill tone={r.kind === 'lyra' ? 'indigo' : r.kind === 'fact' ? 'accent' : 'neutral'}>{KIND_LABEL[r.kind]}</Pill>
                <Link to={`/chat/${r.thread_id}`}>{r.title || fmtPhone(r.account)}</Link>
                <span>{new Date(r.created_at).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })} · {STATUS[r.status] ?? r.status}</span>
              </div>
              <p className="ar-q">“{r.question}”</p>
              {r.answer && <p className="ar-a">{r.answer}</p>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function AutoReplyPage() {
  const { data: info, isLoading, error } = useAutoReply()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [draft, setDraft] = useState<AutoReplySettings | null>(null)
  const save = useMutation({
    mutationFn: (s: AutoReplySettings) => api.put<AutoReplyInfo>('/auto-reply', s),
    onSuccess: (res) => { qc.setQueryData(['auto-reply'], res); setDraft(null); toast('Chat otomatis disimpan') },
    onError: (e: Error) => toast(e.message),
  })
  if (error) return <div className="card"><p className="cl-empty">{(error as Error).message}</p></div>
  if (isLoading || !info) return <div className="card"><p className="cl-empty">Memuat…</p></div>
  const s = draft ?? editable(info.settings) // unsaved edits, else what is saved
  const edit = info.can_edit
  const set = (p: Partial<AutoReplySettings>) => setDraft({ ...s, ...p })
  const dirty = JSON.stringify(s) !== JSON.stringify(editable(info.settings))
  const hoursErr = windowsError(s.breaks, info.send_from, info.send_to)
  const tplLen = s.template.trim().length
  const tplErr = tplLen < 20 || tplLen > 500 ? 'Pesan template 20–500 karakter' : ''
  const preview = fill(s.template, 'Toko Sinar Jaya', dotted(s.breaks[0]?.end ?? '13:00'), 'siang')
  const status = !info.settings.enabled
    ? <Pill tone="neutral">Mati</Pill>
    : info.active_now ? <Pill tone="good" icon="check">Menjawab sekarang · sampai {info.active_until}</Pill> : <Pill tone="accent">Aktif · menunggu jam aktif</Pill>
  return (
    <div className="stack ar">
      <div className="card">
        <div className="card-h">
          <h2>Cara kerja</h2>
          {status}
        </div>
        <ol className="at-steps">
          <li><b>Pelanggan chat</b> pada jam aktif di bawah, di nomor yang ikut. Hanya chat pribadi pelanggan — bukan grup, nomor internal, atau pemasok.</li>
          <li><b>Pertanyaan produk dijawab Lyra</b> (Hermes Agent): Lyra membaca barang yang ditanyakan, lalu stok per kota dan harga DPP sesuai tier dealer diambil dari sistem (data Accurate), sama seperti kartu Lyra di Chat tanpa diskon.</li>
          <li><b>Pertanyaan fakta lain</b> (status order, limit) dijawab AI Trigger dari data sistem, paling banyak {info.max_facts}× per chat per jam aktif.</li>
          <li><b>Pesan lainnya</b> mendapat pesan template, sekali per chat per jam aktif.</li>
          <li><b>Sales tetap memegang chat</b>: bila sales membalas dalam {info.human_quiet_min} menit terakhir, tidak ada jawaban otomatis. Chat tetap “Belum dibalas” supaya sales menindaklanjuti.</li>
        </ol>
      </div>

      <div className="card">
        <div className="card-h"><h2>Penjawab</h2></div>
        <ul className="ar-nums">
          <li>
            <label>
              <input type="checkbox" checked={s.enabled} disabled={!edit} onChange={(e) => set({ enabled: e.target.checked })} />
              <span><b>Chat otomatis aktif</b><small>Mati: tidak ada pesan otomatis sama sekali.</small></span>
            </label>
          </li>
          <li>
            <label>
              <input type="checkbox" checked={s.lyra} disabled={!edit} onChange={(e) => set({ lyra: e.target.checked })} />
              <span>
                <b>Pertanyaan produk dijawab Lyra (Hermes Agent)</b>
                <small>{info.lyra_ready ? 'Lyra tersambung.' : <>Lyra belum tersambung — sambungkan di <Link to="/pengaturan/hermes#sambungan">Hermes Agent → Sambungan</Link>. Sampai itu, pertanyaan produk dijawab AI Trigger.</>}</small>
              </span>
            </label>
          </li>
        </ul>
        <p className="ar-help">Cadangan AI Trigger — fakta yang boleh dijawab bila Lyra tidak menemukan produk{info.ai_ready ? '' : ' (AI belum aktif: isi kunci di API & model AI)'}:</p>
        <div className="seg ar-days" role="group" aria-label="Fakta AI Trigger" style={{ marginTop: 8 }}>
          {FACTS.map(([k, label]) => (
            <button key={k} type="button" aria-pressed={s.facts.includes(k)} className={s.facts.includes(k) ? 'is-active' : ''} disabled={!edit}
              onClick={() => set({ facts: s.facts.includes(k) ? s.facts.filter((x) => x !== k) : [...s.facts, k] })}>{label}</button>
          ))}
        </div>
      </div>

      <Hours breaks={s.breaks} set={(breaks) => set({ breaks })} edit={edit} from={info.send_from} to={info.send_to} />

      <div className="card">
        <div className="card-h"><h2>Pesan template</h2><span className="meta">{tplLen}/500</span></div>
        <textarea className="ar-text" rows={3} value={s.template} disabled={!edit} onChange={(e) => set({ template: e.target.value })} aria-label="Pesan template" />
        <p className="ar-help"><code>{'{salam}'}</code> Selamat pagi/siang/sore · <code>{'{nama}'}</code> nama pelanggan · <code>{'{kembali}'}</code> jam aktif selesai. Sertakan “(Pesan otomatis)” atau “terima kasih sudah menghubungi” supaya chat tetap “Belum dibalas”.</p>
        <div className="ar-bubble"><small>Contoh yang diterima pelanggan</small>{preview}</div>
      </div>

      <Numbers info={info} s={s} set={(numbers) => set({ numbers })} edit={edit} />
      <Log info={info} />

      {!edit && <small className="muted"><Icon name="lock" /> Hanya CEO yang bisa mengubah dan menyetujui chat otomatis.</small>}
      {edit && (dirty || !info.settings.approved_name) && (
        <div className="ar-save">
          <span>{hoursErr || tplErr || `Menyimpan = menyetujui aturan ini; setiap pesan otomatis tercatat atas nama Anda.${info.settings.approved_name ? ` Terakhir disetujui ${info.settings.approved_name}.` : ''}`}</span>
          {dirty && <button type="button" className="btn quiet" onClick={() => setDraft(null)}>Batal</button>}
          <button type="button" className="btn primary" disabled={!!hoursErr || !!tplErr || save.isPending} onClick={() => save.mutate(s)}><Icon name="check" />{save.isPending ? 'Menyimpan…' : 'Simpan & setujui'}</button>
        </div>
      )}
    </div>
  )
}
