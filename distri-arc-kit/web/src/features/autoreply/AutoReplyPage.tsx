import { useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { fmtPhone } from '../chat/inbox'
import { DAYS, FACTS, TOPIC_HINT, KIND_LABEL, daysText, dotted, fill, halfHours, windowsError, type AutoReplyInfo, type AutoReplySettings, type HourWindow } from './autoreply'

// Pengaturan → Chat otomatis (ADR 0066, ADR 0077): during the hours set here a customer's message is answered without
// the sales — a product question by Lyra (Hermes Agent) with the stock and DPP price from the system, another fact
// question by AI Trigger, anything else by the template once. Saving is the CEO's approval: every automatic message
// is recorded as decided by them. The page reads in plain steps with one big on/off state (Sam 29 Oct). Step 2 sets what
// may be answered at all — Lyra and AI Trigger alike; without "Harga" no price reaches the customer (ADR 0080).

const useAutoReply = () => useQuery({ queryKey: ['auto-reply'], queryFn: () => api.get<AutoReplyInfo>('/auto-reply') })

const TEAM: Record<string, string> = { sales: 'Sales', admin: 'Admin', billing: 'Penagihan' }

const STATUS: Record<string, string> = { pending: 'mengirim', sent: 'terkirim', delivered: 'diterima', read: 'dibaca', failed: 'gagal', skipped: 'tidak dikirim' }

const editable = (s: AutoReplySettings): AutoReplySettings => ({
  enabled: s.enabled, lyra: s.lyra, sales_style: !!s.sales_style, breaks: s.breaks, numbers: s.numbers ?? [], template: s.template, facts: s.facts ?? [],
})

/** A green ✓ or red ✗ with a word: whether a step is fine. */
function Mark({ ok, children }: { ok: boolean; children: ReactNode }) {
  return <span className={`arx-mark ${ok ? 'ok' : 'no'}`}><Icon name={ok ? 'check' : 'x'} />{children}</span>
}

/** One numbered step: a big number, a question as title, its mark. */
function Step({ n, title, mark, children }: { n: number; title: string; mark?: ReactNode; children: ReactNode }) {
  return (
    <section className="card arx-step">
      <header><span className="arx-n">{n}</span><h2>{title}</h2>{mark}</header>
      {children}
    </section>
  )
}

/** A time as a plain list of half hours ("12.00"), not the browser's clock that may show 01:00 PM. */
function Clock({ value, from, to, edit, label, onChange }: { value: string; from: string; to: string; edit: boolean; label: string; onChange: (v: string) => void }) {
  const opts = halfHours(from, to)
  if (!opts.includes(value)) opts.push(value)
  opts.sort()
  return (
    <select className="arx-clock" value={value} disabled={!edit} onChange={(e) => onChange(e.target.value)} aria-label={label}>
      {opts.map((o) => <option key={o} value={o}>{dotted(o)}</option>)}
    </select>
  )
}

function Hours({ breaks, set, edit, from, to }: { breaks: HourWindow[]; set: (b: HourWindow[]) => void; edit: boolean; from: string; to: string }) {
  const change = (i: number, w: Partial<HourWindow>) => set(breaks.map((x, j) => (j === i ? { ...x, ...w } : x)))
  const toggleDay = (i: number, d: number) => change(i, { days: breaks[i].days.includes(d) ? breaks[i].days.filter((x) => x !== d) : [...breaks[i].days, d].sort() })
  return (
    <>
      <ul className="arx-hours">
        {breaks.map((w, i) => (
          <li key={i}>
            <p className="arx-say"><b>{w.days.length ? daysText(w.days) : 'Belum pilih hari'}</b>, jam <b>{dotted(w.start)}</b> sampai <b>{dotted(w.end)}</b></p>
            <div className="arx-row">
              <span className="arx-lbl">Hari</span>
              <div className="arx-chips" role="group" aria-label="Hari">
                {DAYS.map(([d, name]) => (
                  <button key={d} type="button" aria-pressed={w.days.includes(d)} className={w.days.includes(d) ? 'on' : ''} disabled={!edit} onClick={() => toggleDay(i, d)}>{w.days.includes(d) && <Icon name="check" />}{name}</button>
                ))}
              </div>
            </div>
            <div className="arx-row">
              <span className="arx-lbl">Jam</span>
              <Clock value={w.start} from={from} to={to} edit={edit} label="Mulai" onChange={(start) => change(i, { start })} />
              <Icon name="arrow" />
              <Clock value={w.end} from={from} to={to} edit={edit} label="Selesai" onChange={(end) => change(i, { end })} />
              {edit && breaks.length > 1 && <button type="button" className="btn quiet arx-del" onClick={() => set(breaks.filter((_, j) => j !== i))}><Icon name="x" />Hapus</button>}
            </div>
          </li>
        ))}
      </ul>
      {edit && breaks.length < 7 && (
        <button type="button" className="btn ghost arx-add" onClick={() => set([...breaks, { days: [1, 2, 3, 4, 5, 6], start: '12:00', end: '13:00' }])}>+ Tambah jadwal lain</button>
      )}
      <p className="arx-hint"><Icon name="alert" /><span>Jam bisa dipilih dari <b>{dotted(from)}</b> sampai <b>{dotted(to)}</b> WIB (batas aman WhatsApp).</span></p>
    </>
  )
}

/** The SweetAlert-style question before the switch flips (Sam 29 Oct: not the browser's confirm box). Portaled to <body>
 *  so the fixed overlay centres on the screen, not inside the page's transformed container. */
function ConfirmSwal({ on, busy, onOk, onCancel }: { on: boolean; busy: boolean; onOk: () => void; onCancel: () => void }) {
  return createPortal(
    <div className="swal-bg" onClick={onCancel}>
      <div className={`swal ${on ? 'swal-success' : 'swal-warning'}`} role="alertdialog" aria-modal="true" aria-labelledby="arx-swal-t" aria-describedby="arx-swal-p"
        onClick={(e) => e.stopPropagation()} onKeyDown={(e) => { if (e.key === 'Escape') { e.preventDefault(); onCancel() } }}>
        <div className="swal-icon" aria-hidden="true">
          <svg viewBox="0 0 52 52"><circle className="swal-ring" cx="26" cy="26" r="24" />{on
            ? <path className="swal-mark" d="M15 27l7 7 15-16" />
            : <path className="swal-mark" d="M26 14v16M26 37v1" />}</svg>
        </div>
        <h3 id="arx-swal-t">{on ? 'Nyalakan chat otomatis?' : 'Matikan chat otomatis?'}</h3>
        <p id="arx-swal-p">{on
          ? 'Pelanggan akan menerima balasan otomatis pada jam yang dipilih, tercatat atas nama Anda.'
          : 'Tidak ada lagi balasan otomatis ke pelanggan sampai dinyalakan lagi.'}</p>
        <div className="arx-swal-actions">
          <button type="button" className="btn ghost" onClick={onCancel} disabled={busy}>Batal</button>
          <button type="button" className={`btn primary ${on ? 'arx-go' : 'arx-stop'}`} onClick={onOk} disabled={busy} ref={(el) => { if (el && document.activeElement !== el) el.focus({ preventScroll: true }) }}>
            {busy ? 'Menyimpan…' : on ? 'Ya, nyalakan' : 'Ya, matikan'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}

export function AutoReplyPage() {
  const { data: info, isLoading, error } = useAutoReply()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const [draft, setDraft] = useState<AutoReplySettings | null>(null)
  const [asking, setAsking] = useState<boolean | null>(null) // the switch's confirmation: turning on (true) or off
  const save = useMutation({
    mutationFn: (s: AutoReplySettings) => api.put<AutoReplyInfo>('/auto-reply', s),
    onSuccess: (res) => {
      qc.setQueryData(['auto-reply'], res)
      setDraft(null)
      toast(res.settings.enabled ? 'Tersimpan — chat otomatis NYALA' : 'Tersimpan — chat otomatis MATI')
    },
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
  const tplErr = tplLen < 20 ? 'Pesan terlalu pendek (minimal 20 huruf)' : tplLen > 500 ? 'Pesan terlalu panjang (maksimal 500 huruf)' : ''
  const err = hoursErr || tplErr
  const preview = fill(s.template, 'Toko Sinar Jaya', dotted(s.breaks[0]?.end ?? '13:00'), 'siang')
  const on = info.settings.enabled
  const when = info.settings.breaks.map((w) => `${daysText(w.days)} ${dotted(w.start)}–${dotted(w.end)}`).join(' · ')
  // every connected number of any team (Sam 29 Oct); a chosen number that dropped stays listed so it can be unticked
  const sales = info.numbers.filter((n) => n.state === 'connected' || s.numbers.includes(n.wa_number))
  const allNums = s.numbers.length === 0
  const flip = () => setAsking(!on)
  const noPrice = !s.facts.includes('price')
  const productHint = [s.facts.includes('stock') && 'Stok', !noPrice && 'harga', 'barang'].filter(Boolean).join(', ')
  return (
    <div className="stack arx">
      {/* the one thing to see first: on, waiting, or off */}
      <div className={`arx-hero ${on ? (info.active_now ? 'live' : 'wait') : 'off'}`}>
        <span className="arx-dot" aria-hidden="true" />
        <div className="arx-hero-t">
          <b>{on ? (info.active_now ? 'NYALA — sedang menjawab' : 'NYALA — menunggu jamnya') : 'MATI'}</b>
          <span>
            {!on ? 'Tidak ada balasan otomatis ke pelanggan.'
              : info.active_now ? `Chat pelanggan dijawab otomatis sampai jam ${info.active_until}.`
                : `Akan menjawab pada: ${when}.`}
          </span>
        </div>
        {edit && (
          // a switch that shows its state: grey with the knob left = MATI, green with the knob right = NYALA
          <button type="button" className={`arx-switch ${on ? 'is-on' : ''}`} role="switch" aria-checked={on} aria-label="Chat otomatis"
            title={on ? 'Klik untuk mematikan' : 'Klik untuk menyalakan'} disabled={save.isPending || (!on && !!err)} onClick={flip}>
            <span className="knob" /><span className="arx-switch-t">{on ? 'NYALA' : 'MATI'}</span>
          </button>
        )}
      </div>
      {!edit && <p className="arx-lock"><Icon name="lock" />Hanya CEO yang bisa mengubah halaman ini.</p>}

      {/* what happens, in three pictures */}
      <div className="card arx-flow">
        <div><Icon name="chat" /><b>Pelanggan chat</b><span>“Stok kamera 4MP ada?” · “Order saya sampai mana?”</span></div>
        <Icon name="arrow" className="i arx-to" />
        <div><Icon name="radar" /><b>{s.sales_style ? 'Lyra membalas seperti sales' : 'Lyra cari barangnya'}</b><span>{noPrice ? 'Angka dari sistem · harga TIDAK diberikan' : 'Angka stok, harga, order dari sistem'}</span></div>
        <Icon name="arrow" className="i arx-to" />
        <div><Icon name="send" /><b>Balasan terkirim</b><span>Sales tetap bisa lanjut</span></div>
      </div>

      <Step n={1} title="Kapan menjawab?" mark={<Mark ok={!hoursErr}>{hoursErr ? 'Perlu dibetulkan' : 'Beres'}</Mark>}>
        <Hours breaks={s.breaks} set={(breaks) => set({ breaks })} edit={edit} from={info.send_from} to={info.send_to} />
        {hoursErr && <p className="arx-err"><Icon name="x" />{hoursErr}</p>}
      </Step>

      <Step n={2} title="Apa yang boleh dijawab?" mark={<Mark ok={s.facts.length > 0}>{s.facts.length ? `${s.facts.length} topik` : 'Hanya salam & template'}</Mark>}>
        <p className="arx-sub">Pilih topik yang boleh dijawab otomatis. Yang tidak dicentang <b>tidak pernah</b> disebut ke pelanggan — oleh Lyra maupun AI Trigger.</p>
        <div className="arx-topics" role="group" aria-label="Topik yang boleh dijawab">
          {FACTS.map(([k, label]) => {
            const on = s.facts.includes(k)
            return (
              <button key={k} type="button" aria-pressed={on} className={`arx-tile ${on ? 'on' : ''}`} disabled={!edit}
                onClick={() => set({ facts: on ? s.facts.filter((x) => x !== k) : [...s.facts, k] })}>
                <span className="arx-box"><Icon name={on ? 'check' : 'x'} /></span>
                <span><b>{label}</b><small>{on ? TOPIC_HINT[k][0] : TOPIC_HINT[k][1]}</small></span>
              </button>
            )
          })}
        </div>
        {noPrice && (
          <p className="arx-hint"><Icon name="lock" /><span>Harga <b>tidak</b> diberikan. Pelanggan yang menanyakan harga dijawab “harganya akan kami konfirmasi”, dan balasan yang menyebut nominal rupiah tidak dikirim.</span></p>
        )}
      </Step>

      <Step n={3} title="Siapa yang menjawab?" mark={<Mark ok={!s.lyra || info.lyra_ready}>{s.lyra ? (info.lyra_ready ? 'Lyra siap' : 'Lyra belum tersambung') : 'Tanpa Lyra'}</Mark>}>
        <button type="button" className={`arx-tile ${s.lyra ? 'on' : ''}`} aria-pressed={s.lyra} disabled={!edit} onClick={() => set({ lyra: !s.lyra })}>
          <span className="arx-box"><Icon name={s.lyra ? 'check' : 'x'} /></span>
          <span>
            <b>Lyra menjawab pertanyaan produk</b>
            <small>{productHint} — Lyra cari barangnya, angka dari sistem.</small>
          </span>
        </button>
        <button type="button" className={`arx-tile ${s.sales_style ? 'on' : ''}`} aria-pressed={s.sales_style} disabled={!edit} onClick={() => set({ sales_style: !s.sales_style })} style={{ marginTop: 10 }}>
          <span className="arx-box"><Icon name={s.sales_style ? 'check' : 'x'} /></span>
          <span>
            <b>Lyra membalas SEMUA chat seperti sales</b>
            <small>Menyapa dengan nama pelanggan dan menjawab sesuai pertanyaan — stok, harga, status order, salam, atau janji temu. Angka selalu dari sistem; balasan yang angkanya tidak cocok tidak dikirim. Diskon, kredit, dan pesanan tetap diserahkan ke sales.</small>
          </span>
        </button>
        {s.sales_style && (
          <div className="arx-wa" style={{ marginTop: 10 }}>
            <small>Contoh balasan Lyra</small>
            {noPrice
              ? <>Selamat siang Pak Budi, untuk <b>NVR 32ch</b> stok di Semarang ada 9 unit. Untuk harganya akan kami konfirmasi ya, Pak. Butuh berapa unit?</>
              : <>Selamat siang Pak Budi, untuk <b>NVR 32ch</b> stok di Semarang ada 9 unit. Harganya <b>Rp 2.500.000</b>/unit (harga DPP, belum termasuk PPN). Mau saya bantu siapkan penawarannya, Pak?</>}{'\n\n'}<i>— Lyra, asisten GSI (pesan otomatis)</i>
          </div>
        )}
        {(s.lyra || s.sales_style) && !info.lyra_ready && (
          <p className="arx-err"><Icon name="alert" />Lyra belum tersambung. <Link to="/pengaturan/hermes#sambungan">Sambungkan Lyra di sini →</Link></p>
        )}
        <p className="arx-sub">Kalau Lyra tidak menemukan barangnya, AI Trigger menjawab topik yang dicentang di langkah 2.</p>
      </Step>

      <Step n={4} title={s.sales_style ? 'Pesan cadangan' : 'Pesan untuk chat lainnya'} mark={<Mark ok={!tplErr}>{tplErr ? 'Perlu dibetulkan' : 'Beres'}</Mark>}>
        <p className="arx-sub">{s.sales_style
          ? 'Dipakai hanya bila Lyra tidak bisa membalas (belum tersambung, terlalu lama, atau balasannya tidak lolos cek angka), sekali saja.'
          : 'Chat yang bukan soal produk mendapat pesan ini, sekali saja.'}</p>
        <textarea className="ar-text" rows={3} value={s.template} disabled={!edit} onChange={(e) => set({ template: e.target.value })} aria-label="Pesan template" />
        <div className="arx-count">{tplLen}/500</div>
        {tplErr && <p className="arx-err"><Icon name="x" />{tplErr}</p>}
        <p className="arx-sub">Yang dilihat pelanggan:</p>
        <div className="arx-wa">{preview}</div>
        <p className="arx-hint"><span>Kata <code>{'{nama}'}</code> diganti nama pelanggan, <code>{'{kembali}'}</code> jam sales kembali, <code>{'{salam}'}</code> Selamat pagi/siang/sore.</span></p>
      </Step>

      <Step n={5} title="Nomor mana?" mark={<Mark ok={allNums || s.numbers.length > 0}>{allNums ? 'Semua nomor' : `${s.numbers.length} nomor`}</Mark>}>
        <div className="arx-chips big" role="radiogroup" aria-label="Nomor">
          <button type="button" role="radio" aria-checked={allNums} className={allNums ? 'on' : ''} disabled={!edit} onClick={() => set({ numbers: [] })}>{allNums && <Icon name="check" />}Semua nomor</button>
          <button type="button" role="radio" aria-checked={!allNums} className={!allNums ? 'on' : ''} disabled={!edit || sales.length === 0} onClick={() => allNums && set({ numbers: [sales[0].wa_number] })}>{!allNums && <Icon name="check" />}Pilih sendiri</button>
        </div>
        {!allNums && (
          <ul className="ar-nums arx-nums">
            {sales.map((n) => (
              <li key={n.wa_number}>
                <label>
                  <input type="checkbox" checked={s.numbers.includes(n.wa_number)} disabled={!edit}
                    onChange={(e) => set({ numbers: e.target.checked ? [...s.numbers, n.wa_number] : s.numbers.filter((x) => x !== n.wa_number) })} />
                  <span><b>{n.holder || n.label || fmtPhone(n.wa_number)}</b><small>{fmtPhone(n.wa_number)} · {TEAM[n.team] ?? n.team}{n.state !== 'connected' ? ' · tidak tersambung' : ''}</small></span>
                </label>
              </li>
            ))}
          </ul>
        )}
      </Step>

      <p className="arx-hint arx-rule"><Icon name="shield" /><span>Aman: hanya chat pribadi pelanggan (bukan grup). Kalau sales baru membalas dalam {info.human_quiet_min} menit terakhir, tidak ada balasan otomatis. Chat tetap “Belum dibalas” supaya sales menindaklanjuti.</span></p>

      <details className="card arx-log">
        <summary><Icon name="doc" />Riwayat balasan otomatis ({info.recent.length})</summary>
        {info.recent.length === 0 ? <p className="cl-empty">Belum ada balasan otomatis.</p> : (
          <ul className="ar-log">
            {info.recent.map((r) => (
              <li key={r.message_id}>
                <div className="ar-log-h">
                  <span className={`arx-kind ${r.kind}`}>{KIND_LABEL[r.kind]}</span>
                  <Link to={`/chat/${r.thread_id}`}>{r.title || fmtPhone(r.account)}</Link>
                  <span>{new Date(r.created_at).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' })} · {STATUS[r.status] ?? r.status}</span>
                </div>
                <p className="ar-q">“{r.question}”</p>
                {r.answer && <p className="ar-a">{r.answer}</p>}
              </li>
            ))}
          </ul>
        )}
      </details>

      {asking !== null && (
        <ConfirmSwal on={asking} busy={save.isPending} onCancel={() => setAsking(null)}
          onOk={() => save.mutate({ ...s, enabled: asking }, { onSettled: () => setAsking(null) })} />
      )}

      {edit && dirty && (
        <div className={`arx-save ${err ? 'bad' : ''}`}>
          <Icon name={err ? 'x' : 'alert'} />
          <span>{err ? `Belum bisa disimpan: ${err}` : 'Ada perubahan yang BELUM DISIMPAN'}</span>
          <button type="button" className="btn quiet" onClick={() => setDraft(null)}>Batal</button>
          <button type="button" className="btn primary" disabled={!!err || save.isPending} onClick={() => save.mutate(s)}><Icon name="check" />{save.isPending ? 'Menyimpan…' : 'Simpan'}</button>
        </div>
      )}
    </div>
  )
}
