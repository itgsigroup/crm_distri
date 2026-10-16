import { useEffect, useRef, useState } from 'react'
import { Icon } from '../../components/Icon'
import { hhmm } from '../../lib/format'
import { STAGES, chipState } from '../../app/cycle'
import { duration, useOrch } from '../../app/orch'
import { START_SPEED, advance, arcPath, newGame, point, press, type Game } from './tapgame'

// Pusat kendali while an analysis runs: what the AI is reading, stage by stage, and at the end whether it worked —
// with a small game to play while waiting (Sam 16 Oct). The panel announces the result itself (no pop-up).

const READING: Record<string, [string, string]> = {
  ingest: ['Membaca data', 'chat WhatsApp, order, invoice, pembayaran dan stok'],
  analyze: ['Agen AI menganalisa', 'jadwal order, limit kredit, stok, prospek dan penagihan per dealer'],
  synthesize: ['Menggabungkan temuan', 'hasil keenam agen dan konflik di antaranya'],
  decide: ['Menyusun usulan', 'keputusan yang menunggu persetujuan Anda'],
  execute: ['Menyimpan hasil', 'usulan, Analisa Stok, Analisa Kas dan Analisa WhatsApp'],
  learn: ['Mencatat pelajaran', 'keputusan sebelumnya untuk siklus berikutnya'],
}

const BEST_KEY = 'tapgame-best'
function savedBest(): number {
  try { return Number(localStorage.getItem(BEST_KEY)) || 0 } catch { return 0 }
}

export function AnalysisPanel() {
  const { s, showPanel } = useOrch()
  useEffect(() => showPanel(), [showPanel]) // the result shows here, not as a pop-up
  const [closed, setClosed] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!s.running) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [s.running])

  const last = s.last
  const showResult = !s.running && !!last && last.mine && closed !== last.endedAt
  if (!s.running && !showResult) return null

  if (showResult && last) {
    const failed = last.status === 'failed'
    const partial = last.status === 'partial'
    const took = last.startedAt ? duration(last.endedAt - last.startedAt) : '—'
    return (
      <div className={`card an-panel ${failed ? 'bad' : partial ? 'warn' : 'good'}`} role="status">
        <div className="an-res">
          <span className="an-res-ic"><Icon name={failed ? 'alert' : 'check'} /></span>
          <div>
            <h2>{failed ? 'Analisis gagal' : partial ? 'Analisis selesai sebagian' : 'Analisis berhasil'}</h2>
            <p>
              {failed
                ? `Analisis ulang ${last.label} tidak selesai. Data dan saran sebelumnya tetap dipakai — coba lagi beberapa saat lagi.`
                : partial
                ? `Analisis ulang ${last.label} selesai, tetapi sebagian agen tidak berjalan.`
                : `Analisis ulang ${last.label} selesai. Halaman ini sudah diperbarui dengan hasilnya.`}
            </p>
            <div className="an-meta">
              <span>Siklus <b>{last.number ? '#' + last.number.toLocaleString('id-ID') : '—'}</b></span>
              <span>Selesai <b>{hhmm(new Date(last.endedAt))} WIB</b></span>
              <span>Durasi <b>{took}</b></span>
              {!failed && <span>Saran diperbarui <b>{last.updated}</b></span>}
              {last.note && <span className="an-note">{failed ? 'Penyebab' : 'Catatan'}: {last.note}</span>}
            </div>
          </div>
          <button type="button" className="ibl-btn" onClick={() => setClosed(last.endedAt)} aria-label="Tutup" title="Tutup"><Icon name="x" /></button>
        </div>
      </div>
    )
  }

  const cur = Math.min(s.shown, STAGES.length - 1)
  const [what, detail] = READING[STAGES[cur]]
  return (
    <div className="card an-panel running" role="status" aria-live="polite">
      <div className="an-grid">
        <div className="an-read">
          <div className="an-head">
            <span className="an-pulse" />
            <div>
              <h2>AI sedang membaca data<span className="an-dots"><i>.</i><i>.</i><i>.</i></span></h2>
              <p>{what}: {detail}</p>
            </div>
          </div>
          <ol className="an-steps">
            {STAGES.map((st, i) => {
              const state = chipState(s, i, true)
              return (
                <li key={st} className={state}>
                  <span className="an-st">{state === 'done' ? <Icon name="check" /> : state === 'run' ? <i className="an-spin" /> : i + 1}</span>
                  <span><b>{READING[st][0]}</b><small>{READING[st][1]}</small></span>
                </li>
              )
            })}
          </ol>
          <div className="an-meta">
            <span>Lingkup <b>{s.label || 'semua'}</b></span>
            <span>Oleh <b>{s.via === 'api' ? 'API' : s.via}</b></span>
            {s.startedAt && <span>Berjalan <b>{duration(now - s.startedAt)}</b></span>}
          </div>
        </div>
        {/* the MCP sheet (instructions for Claude) keeps the keyboard; otherwise Enter goes straight to the game */}
        <TapGame autoFocus={s.mine && !/mcp/i.test(s.via)} />
      </div>
    </div>
  )
}

/** Tap tap Enter: press Enter or Space (or tap) when the dot is inside the green arc. */
export function TapGame({ autoFocus }: { autoFocus: boolean }) {
  const [g, setG] = useState<Game>(() => newGame(savedBest()))
  const box = useRef<HTMLDivElement>(null)
  const prev = useRef<number | null>(null)
  useEffect(() => {
    let raf = 0
    const step = (t: number) => {
      const dt = prev.current == null ? 0 : Math.min(64, t - prev.current)
      prev.current = t
      setG((x) => advance(x, dt))
      raf = requestAnimationFrame(step)
    }
    raf = requestAnimationFrame(step)
    return () => cancelAnimationFrame(raf)
  }, [])
  useEffect(() => {
    if (autoFocus) box.current?.focus({ preventScroll: true })
  }, [autoFocus])
  useEffect(() => {
    try { localStorage.setItem(BEST_KEY, String(g.best)) } catch { /* private mode */ }
  }, [g.best])
  const hit = () => setG((x) => press(x, Math.random()))
  const [dx, dy] = point(g.angle, 52, 64, 64)
  return (
    <div className="an-game" ref={box} tabIndex={0} role="application" aria-label="Permainan tap tap Enter: tekan Enter atau Spasi saat titik di busur hijau"
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (!e.repeat) hit() } }}
      onPointerDown={(e) => { e.preventDefault(); box.current?.focus({ preventScroll: true }); hit() }}>
      <svg viewBox="0 0 128 128" aria-hidden>
        <circle cx="64" cy="64" r="52" className="an-ring" />
        <path d={arcPath(g.target, g.width, 52, 64, 64)} className="an-target" />
        <circle cx={dx} cy={dy} r="7" className={`an-ball ${g.last ?? ''}`} />
        <text x="64" y="60" className="an-score">{g.score}</text>
        <text x="64" y="78" className="an-best">terbaik {g.best}</text>
      </svg>
      <div className="an-game-t">
        <b>{g.last === 'hit' ? 'Tepat!' : g.last === 'miss' ? 'Meleset — coba lagi' : 'Tap tap Enter'}</b>
        <small>Tekan <kbd>Enter</kbd> / <kbd>Spasi</kbd> atau ketuk saat titik di busur hijau{g.speed > START_SPEED ? ' · makin cepat' : ''}</small>
      </div>
    </div>
  )
}
