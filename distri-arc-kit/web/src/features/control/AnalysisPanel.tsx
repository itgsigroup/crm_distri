import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon } from '../../components/Icon'
import { hhmm } from '../../lib/format'
import { STAGES, chipState, type CycleUI } from '../../app/cycle'
import { duration, useOrch } from '../../app/orch'
import { START_SPEED, advance, arcPath, newGame, point, press, type Game } from './tapgame'

// While an analysis runs: what the AI is reading, stage by stage, and at the end whether it worked — with a small game
// to play while waiting (Sam 16 Oct). Used by the analysis pop-up and by Pusat kendali.

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

type Last = NonNullable<CycleUI['last']>

/** Pusat kendali: a cycle this tab did not start (the hourly one, or another user's) — what the AI reads, and the
 * game. An analysis started here shows in its own pop-up (AnalysisSheet), so it is not shown twice. */
export function AnalysisPanel() {
  const { s } = useOrch()
  if (!s.running || s.mine) return null
  return (
    <div className="card an-panel running" role="status" aria-live="polite">
      <AnalysisRunning />
    </div>
  )
}

/** "AI sedang membaca data…": the stage in plain words, the six stages, scope, engine and time running — and the game
 * beside it. extra: a line under the stages (the MCP hint). */
export function AnalysisRunning({ extra, focusGame = false }: { extra?: ReactNode; focusGame?: boolean }) {
  const { s } = useOrch()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  const cur = Math.min(s.shown, STAGES.length - 1)
  const [what, detail] = READING[STAGES[cur]]
  return (
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
        {extra}
        <div className="an-meta">
          <span>Lingkup <b>{s.label || 'semua'}</b></span>
          <span>Oleh <b>{s.via === 'api' ? 'API' : s.via}</b></span>
          {s.startedAt && <span>Berjalan <b>{duration(now - s.startedAt)}</b></span>}
        </div>
      </div>
      <TapGame autoFocus={focusGame} />
    </div>
  )
}

/** The end of an analysis: berhasil, selesai sebagian or gagal, with the cycle, time, duration and cause. */
export function AnalysisResult({ last }: { last: Last }) {
  const failed = last.status === 'failed'
  const partial = last.status === 'partial'
  const took = last.startedAt ? duration(last.endedAt - last.startedAt) : '—'
  return (
    <div className={`an-res ${failed ? 'bad' : partial ? 'warn' : 'good'}`} role="status">
      <span className="an-res-ic"><Icon name={failed ? 'alert' : 'check'} /></span>
      <div>
        <h2>{failed ? 'Analisis gagal' : partial ? 'Analisis selesai sebagian' : 'Analisis berhasil'}</h2>
        <p>
          {failed
            ? `Analisis ulang ${last.label} tidak selesai. Data dan saran sebelumnya tetap dipakai — coba lagi beberapa saat lagi.`
            : partial
            ? `Analisis ulang ${last.label} selesai, tetapi sebagian agen tidak berjalan.`
            : `Analisis ulang ${last.label} selesai. Halaman sudah diperbarui dengan hasilnya.`}
        </p>
        <div className="an-meta">
          <span>Siklus <b>{last.number ? '#' + last.number.toLocaleString('id-ID') : '—'}</b></span>
          <span>Selesai <b>{hhmm(new Date(last.endedAt))} WIB</b></span>
          <span>Durasi <b>{took}</b></span>
          {!failed && <span>Saran diperbarui <b>{last.updated}</b></span>}
          {last.note && <span className="an-note">{failed ? 'Penyebab' : 'Catatan'}: {last.note}</span>}
        </div>
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
