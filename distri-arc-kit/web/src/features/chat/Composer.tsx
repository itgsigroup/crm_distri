import { useEffect, useMemo, useRef, useState } from 'react'
import { Icon } from '../../components/Icon'
import { SheetHead, useFeedback } from '../../components/feedback'
import { fmtDuration, fmtSize } from './bubble'

// Composer extras like WhatsApp (ADR 0031): attach a document, photo or video, record a voice note, create a poll.

/** The largest file Chat sends (the API refuses more). */
export const MAX_FILE = 25 * 1024 * 1024

export function Clip() {
  return <svg className="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M16.5 6.5v10a4.5 4.5 0 0 1-9 0V6a3 3 0 0 1 6 0v10a1.5 1.5 0 0 1-3 0V7" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" /></svg>
}

function PollIcon() {
  return <svg className="i" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 20V10M12 20V4M19 20v-7" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" /></svg>
}

/** 📎: Dokumen, Foto & video, Polling. */
export function AttachMenu({ onFile, onPoll, disabled }: { onFile: (f: File) => void; onPoll: () => void; disabled?: boolean }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLSpanElement>(null)
  const doc = useRef<HTMLInputElement>(null)
  const media = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])
  const picked = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = ''
    setOpen(false)
    if (f) onFile(f)
  }
  return (
    <span className="cmp-attach" ref={ref}>
      <button type="button" className="cmp-ic" onClick={() => setOpen(!open)} disabled={disabled} aria-label="Lampirkan" aria-haspopup="menu" aria-expanded={open} title="Lampirkan file, foto, video, atau polling"><Clip /></button>
      {open && (
        <span className="cmp-menu" role="menu">
          <button type="button" role="menuitem" onClick={() => doc.current?.click()}><span className="cmp-dot doc"><Icon name="doc" /></span>Dokumen</button>
          <button type="button" role="menuitem" onClick={() => media.current?.click()}><span className="cmp-dot media"><Icon name="box" /></span>Foto &amp; video</button>
          <button type="button" role="menuitem" onClick={() => { setOpen(false); onPoll() }}><span className="cmp-dot poll"><PollIcon /></span>Polling</button>
        </span>
      )}
      <input ref={doc} type="file" hidden onChange={picked} />
      <input ref={media} type="file" hidden accept="image/jpeg,image/png,image/webp,video/mp4,video/3gpp,video/quicktime" onChange={picked} />
    </span>
  )
}

/** The file chosen, above the input (the input becomes its caption). */
export function FilePreview({ file, onRemove }: { file: File; onRemove: () => void }) {
  const url = useMemo(() => (file.type.startsWith('image/') || file.type.startsWith('video/') ? URL.createObjectURL(file) : null), [file])
  useEffect(() => () => { if (url) URL.revokeObjectURL(url) }, [url])
  return (
    <div className="cmp-file">
      {url && file.type.startsWith('image/') ? <img src={url} alt="" /> : url ? <video src={url} muted /> : <span className="cmp-file-ic"><Icon name="doc" /></span>}
      <span><b>{file.name}</b><small>{fmtSize(file.size)}{file.size > MAX_FILE ? ' · lebih dari 25 MB' : ' · tulis keterangan di bawah (opsional)'}</small></span>
      <button type="button" onClick={onRemove} aria-label="Batalkan lampiran"><Icon name="x" /></button>
    </div>
  )
}

/** Records a voice note with the browser's microphone. */
export function useVoiceRecorder(onDone: (blob: Blob, seconds: number) => void, onError: (msg: string) => void) {
  const [recording, setRecording] = useState(false)
  const [secs, setSecs] = useState(0)
  const rec = useRef<MediaRecorder | null>(null)
  const chunks = useRef<Blob[]>([])
  const started = useRef(0)
  const keep = useRef(true)
  const timer = useRef<number | undefined>(undefined)
  const stopTracks = () => rec.current?.stream.getTracks().forEach((t) => t.stop())
  useEffect(() => () => { keep.current = false; window.clearInterval(timer.current); stopTracks() }, [])
  const start = async () => {
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') return onError('Browser ini tidak bisa merekam suara')
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const type = ['audio/ogg;codecs=opus', 'audio/webm;codecs=opus', 'audio/mp4'].find((t) => MediaRecorder.isTypeSupported(t))
      const r = new MediaRecorder(stream, type ? { mimeType: type } : undefined)
      chunks.current = []
      keep.current = true
      r.ondataavailable = (e) => e.data.size && chunks.current.push(e.data)
      r.onstop = () => {
        stream.getTracks().forEach((t) => t.stop())
        const seconds = Math.max(1, Math.round((Date.now() - started.current) / 1000))
        if (keep.current && chunks.current.length) onDone(new Blob(chunks.current, { type: r.mimeType || 'audio/webm' }), seconds)
      }
      rec.current = r
      started.current = Date.now()
      setSecs(0)
      r.start(250)
      setRecording(true)
      timer.current = window.setInterval(() => setSecs(Math.round((Date.now() - started.current) / 1000)), 500)
    } catch {
      onError('Mikrofon tidak diizinkan — izinkan di pengaturan browser')
    }
  }
  const finish = (send: boolean) => {
    keep.current = send
    window.clearInterval(timer.current)
    setRecording(false)
    if (rec.current && rec.current.state !== 'inactive') rec.current.stop()
    else stopTracks()
  }
  return { recording, secs, start, send: () => finish(true), cancel: () => finish(false) }
}

/** The recording bar that replaces the input while a voice note is being recorded. */
export function RecordingBar({ secs, onCancel, onSend }: { secs: number; onCancel: () => void; onSend: () => void }) {
  return (
    <div className="cmp-rec" role="status">
      <button type="button" className="cmp-ic" onClick={onCancel} aria-label="Batalkan rekaman" title="Batalkan"><Icon name="x" /></button>
      <span className="cmp-rec-dot" /><span className="cmp-rec-t">Merekam {fmtDuration(secs) || '0:00'}</span>
      <span className="cmp-rec-wave" aria-hidden="true" />
      <button type="button" className="send" onClick={onSend} aria-label="Kirim pesan suara"><Icon name="send" /></button>
    </div>
  )
}

export function Mic() {
  return <svg className="i" viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="3" width="6" height="11" rx="3" fill="currentColor" /><path d="M5.5 11a6.5 6.5 0 0 0 13 0M12 17.5V21" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
}

/** "Buat polling": a question, 2–12 options, one or several answers. */
export function PollSheet({ onSend }: { onSend: (p: { name: string; options: string[]; multi: boolean }) => Promise<unknown> }) {
  const { closeSheet } = useFeedback()
  const [name, setName] = useState('')
  const [options, setOptions] = useState(['', ''])
  const [multi, setMulti] = useState(false)
  const [busy, setBusy] = useState(false)
  const filled = options.map((o) => o.trim()).filter(Boolean)
  const ok = name.trim() && filled.length >= 2 && new Set(filled).size === filled.length
  const set = (i: number, v: string) => {
    const next = options.map((o, j) => (j === i ? v : o))
    if (i === next.length - 1 && v.trim() && next.length < 12) next.push('')
    setOptions(next)
  }
  const send = () => {
    if (!ok) return
    setBusy(true)
    onSend({ name: name.trim(), options: filled, multi }).then(closeSheet, () => setBusy(false))
  }
  return (
    <>
      <SheetHead icon="chart" title="Buat polling" sub="Dikirim ke chat ini seperti polling WhatsApp — pelanggan memilih di HP-nya" onClose={closeSheet} />
      <div className="sec poll-form">
        <label>Pertanyaan<input value={name} onChange={(e) => setName(e.target.value)} placeholder="mis. Jadwal kirim yang cocok?" maxLength={255} autoFocus /></label>
        <span className="poll-h">Pilihan</span>
        {options.map((o, i) => (
          <input key={i} value={o} onChange={(e) => set(i, e.target.value)} placeholder={`Pilihan ${i + 1}`} maxLength={100} aria-label={`Pilihan ${i + 1}`} />
        ))}
        <label className="poll-multi"><input type="checkbox" checked={multi} onChange={(e) => setMulti(e.target.checked)} />Izinkan beberapa jawaban</label>
        {filled.length !== new Set(filled).size && <small className="poll-err">Pilihan tidak boleh sama</small>}
      </div>
      <div className="ft"><button className="btn quiet" onClick={closeSheet}>Batal</button><span className="spacer" /><button className="btn primary" disabled={!ok || busy} onClick={send}><Icon name="send" />Kirim polling</button></div>
    </>
  )
}
