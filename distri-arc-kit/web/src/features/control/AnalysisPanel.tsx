import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Icon } from '../../components/Icon'
import { hhmm } from '../../lib/format'
import { STAGES, chipState, type CycleUI } from '../../app/cycle'
import { duration, useOrch } from '../../app/orch'
import { GROUND, PH, PX, W, jump, newRun, score, step, type Run } from './jumpgame'

// While an analysis runs: what the AI is reading, stage by stage, and at the end whether it worked — with a small game
// to play while waiting (Sam 16 Oct). Used by the analysis pop-up and by Pusat kendali.

const READING: Record<string, [string, string]> = {
  ingest: ['Membaca data', 'chat WhatsApp baru hari ini, order, invoice, pembayaran dan stok'],
  analyze: ['Agen AI menganalisa', 'jadwal order, limit kredit, stok, prospek dan penagihan per dealer'],
  synthesize: ['Menggabungkan temuan', 'hasil keenam agen dan konflik di antaranya'],
  decide: ['Menyusun usulan', 'keputusan yang menunggu persetujuan Anda'],
  execute: ['Menyimpan hasil', 'usulan, Analisa Stok, Analisa Kas dan Analisa WhatsApp'],
  learn: ['Mencatat pelajaran', 'keputusan sebelumnya untuk siklus berikutnya'],
}

const BEST_KEY = 'jumpgame-best'
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
      <JumpGame autoFocus={focusGame} />
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

const JUMP_KEYS = new Set([' ', 'Enter', 'ArrowUp', 'w', 'W'])

/** Lompat!: Space (or Enter, ↑, a tap) jumps over the pipes — twice in the air at most — and coins score extra. */
export function JumpGame({ autoFocus }: { autoFocus: boolean }) {
  const [r, setR] = useState<Run>(() => newRun(savedBest()))
  const box = useRef<HTMLDivElement>(null)
  const prev = useRef<number | null>(null)
  useEffect(() => {
    let raf = 0
    const tick = (t: number) => {
      const dt = prev.current == null ? 0 : Math.min(40, t - prev.current)
      prev.current = t
      setR((x) => step(x, dt, Math.random()))
      raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [])
  useEffect(() => {
    if (autoFocus) box.current?.focus({ preventScroll: true })
  }, [autoFocus])
  useEffect(() => {
    try { localStorage.setItem(BEST_KEY, String(r.best)) } catch { /* private mode */ }
  }, [r.best])
  const press = () => setR((x) => jump(x))
  const top = GROUND - r.y - PH // the runner's top
  const legs = r.y > 0 ? 0 : Math.floor(r.dist / 10) % 2 // the stride while on the ground
  return (
    <div className="an-game" ref={box} tabIndex={0} role="application" aria-label="Game Lompat: tekan Spasi untuk melompati pipa"
      onKeyDown={(e) => { if (JUMP_KEYS.has(e.key)) { e.preventDefault(); if (!e.repeat) press() } }}
      onPointerDown={(e) => { e.preventDefault(); box.current?.focus({ preventScroll: true }); press() }}>
      <svg viewBox={`0 0 ${W} 120`} aria-hidden className="jg">
        <rect x="0" y="0" width={W} height={GROUND} className="jg-sky" />
        {[40, 150, 250].map((cx, i) => {
          const x = ((cx - r.dist * 0.15) % (W + 60) + W + 60) % (W + 60) - 30
          return <g key={i} className="jg-cloud"><ellipse cx={x} cy={18 + i * 9} rx="16" ry="6" /><ellipse cx={x + 10} cy={14 + i * 9} rx="10" ry="6" /></g>
        })}
        {r.coins.filter((c) => !c.got).map((c) => <ellipse key={c.id} cx={c.x} cy={GROUND - c.y} rx="4" ry="5" className="jg-coin" />)}
        {r.pipes.map((p) => (
          <g key={p.id} className="jg-pipe">
            <rect x={p.x + 2} y={GROUND - p.h + 6} width={p.w - 4} height={p.h - 6} />
            <rect x={p.x} y={GROUND - p.h} width={p.w} height="7" rx="1" />
          </g>
        ))}
        <rect x="0" y={GROUND} width={W} height={120 - GROUND} className="jg-ground" />
        {Array.from({ length: 12 }, (_, i) => {
          const x = ((i * 30 - r.dist) % W + W) % W
          return <rect key={i} x={x} y={GROUND + 2} width="14" height="4" className="jg-brick" />
        })}
        {/* the courier: cap, face, shirt, overalls, two legs */}
        <g className={`jg-hero${r.over ? ' hurt' : ''}`}>
          <rect x={PX + 2} y={top} width="12" height="4" rx="1" className="jg-cap" />
          <rect x={PX + 11} y={top + 2} width="5" height="2" className="jg-cap" />
          <rect x={PX + 3} y={top + 4} width="10" height="5" className="jg-face" />
          <rect x={PX + 10} y={top + 5} width="2" height="2" className="jg-eye" />
          <rect x={PX + 2} y={top + 9} width="12" height="6" className="jg-shirt" />
          <rect x={PX + 4} y={top + 10} width="8" height="6" className="jg-overall" />
          <rect x={PX + (legs ? 3 : 4)} y={top + 16} width="4" height="4" className="jg-shoe" />
          <rect x={PX + (legs ? 10 : 9)} y={top + 16} width="4" height="4" className="jg-shoe" />
        </g>
        <text x={W - 8} y="14" className="jg-score">{String(score(r)).padStart(4, '0')}</text>
        <text x={W - 8} y="25" className="jg-best">terbaik {r.best}</text>
        {(!r.started || r.over) && (
          <g className="jg-msg">
            <rect x={W / 2 - 92} y="34" width="184" height="34" rx="8" />
            <text x={W / 2} y="49">{r.over ? `Kena pipa! Skor ${score(r)}` : 'Lompat!'}</text>
            <text x={W / 2} y="61" className="jg-sub">{r.over ? 'Spasi untuk main lagi' : 'Tekan Spasi untuk mulai'}</text>
          </g>
        )}
      </svg>
      <div className="an-game-t">
        <small>Tekan <kbd>Spasi</kbd> untuk melompat (dua kali = lompat lebih tinggi) · ambil koin · hindari pipa</small>
      </div>
    </div>
  )
}
