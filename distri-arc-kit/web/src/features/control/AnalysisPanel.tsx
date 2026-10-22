import { useEffect, useState, type ReactNode } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { hhmm } from '../../lib/format'
import { STAGES, chipState, type CycleUI } from '../../app/cycle'
import { SYSTEM_LABEL, duration, useOrch } from '../../app/orch'
import { STATE_LABEL, ordered, summarize, type CycleProgress } from './progress'

// While an analysis runs: what the AI is reading, stage by stage, and at the end whether it worked — compact, no game
// (Sam 22 Oct: in a CRM time is everything). Used by the analysis pop-up and by Pusat kendali.

// each stage in short: its icon, its name, and the one line shown while it runs
export const READING: Record<string, { icon: string; name: string; line: string }> = {
  ingest: { icon: 'chat', name: 'Baca data', line: 'chat WhatsApp hari ini, order, invoice & stok' },
  analyze: { icon: 'spark', name: 'Analisa', line: '6 agen AI menganalisa dealer' },
  synthesize: { icon: 'net', name: 'Gabungkan', line: 'menggabungkan temuan agen' },
  decide: { icon: 'flag', name: 'Usulan', line: 'menyusun usulan untuk Anda' },
  execute: { icon: 'doc', name: 'Simpan', line: 'menyimpan hasil' },
  learn: { icon: 'trend', name: 'Belajar', line: 'mencatat pelajaran' },
}

type Last = NonNullable<CycleUI['last']>

/** "AI sedang membaca data…": the stage running, the six stages as chips with their icons, engine and time.
 * extra: a line under the stages (the MCP hint). */
export function AnalysisRunning({ extra }: { extra?: ReactNode }) {
  const { s } = useOrch()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  const cur = READING[STAGES[Math.min(s.shown, STAGES.length - 1)]]
  return (
    <div className="an-run">
      <div className="an-head">
        <span className="an-pulse" />
        <div>
          <h2>{s.via === SYSTEM_LABEL ? 'Sistem sedang membaca data' : 'AI sedang membaca data'}<span className="an-dots"><i>.</i><i>.</i><i>.</i></span></h2>
          <p><Icon name={cur.icon} />{cur.name} · {cur.line}</p>
        </div>
        <div className="an-meta">
          <span><Icon name="spark" /><b>{s.via === 'api' ? 'API' : s.via}</b></span>
          {s.startedAt && <span><Icon name="refresh" /><b>{duration(now - s.startedAt)}</b></span>}
          {s.cycleId && !s.finished && <CancelButton cycleId={s.cycleId} />}
        </div>
      </div>
      <ol className="an-chips">
        {STAGES.map((st) => {
          const state = chipState(s, STAGES.indexOf(st), true)
          return (
            <li key={st} className={state} title={READING[st].line}>
              <span className="an-ic">{state === 'run' ? <i className="an-spin" /> : <Icon name={state === 'done' ? 'check' : READING[st].icon} />}</span>
              {READING[st].name}
            </li>
          )
        })}
      </ol>
      {s.cycleId && <AnalysisProgress cycleId={s.cycleId} />}
      {extra}
    </div>
  )
}

/** "Batalkan analisis" (Sam 17 Oct): stops a cycle that waits — e.g. Claude was never opened — so Analisis ulang is
 * free again; the cycle ends as failed with who cancelled it. */
export function CancelButton({ cycleId }: { cycleId: string }) {
  const { toast } = useFeedback()
  const cancel = useMutation({
    mutationFn: () => api.post(`/cycles/${cycleId}/cancel`, {}),
    onSuccess: () => toast('Analisis dibatalkan'),
    onError: (e: Error) => toast(e.message),
  })
  return (
    <button className="btn quiet an-cancel" disabled={cancel.isPending || cancel.isSuccess}
      onClick={() => { if (window.confirm('Batalkan analisis ini? Data dan saran sebelumnya tetap dipakai.')) cancel.mutate() }}>
      <Icon name="x" />{cancel.isSuccess ? 'Membatalkan…' : 'Batalkan analisis'}
    </button>
  )
}

const AGENT_ICON: Record<string, string> = {
  'AI Order': 'doc', 'AI Follow-up': 'refresh', 'AI Kredit': 'shield', 'AI Stok': 'box', 'AI Penagihan': 'cash', 'AI Prospek': 'people', 'Analisa WhatsApp': 'chat',
}

/** What the AI is analysing now: the data read, each agent (antri · sedang dianalisa · selesai) with what it covers,
 * and the AI's last steps on this cycle — polled while the cycle runs. */
export function AnalysisProgress({ cycleId }: { cycleId: string }) {
  const { data: p } = useQuery({
    queryKey: ['cycle-progress', cycleId],
    queryFn: () => api.get<CycleProgress>(`/cycles/${cycleId}/progress`),
    refetchInterval: 2500,
  })
  if (!p) return null
  const sum = summarize(p)
  const read = p.stages.find((x) => x.stage === 'ingest' && x.text)?.text
  return (
    <div className="an-prog">
      {read && <p className="an-read"><Icon name="chat" />Data dibaca: {read}</p>}
      {sum.total > 0 && (
        <>
          <div className="an-prog-h">
            <b>{sum.now || 'Analisa agen'}</b>
            <span className="num">{sum.done}/{sum.total} selesai</span>
          </div>
          <div className="an-bar" role="progressbar" aria-valuenow={sum.pct} aria-valuemin={0} aria-valuemax={100}><i style={{ width: `${Math.max(sum.pct, 4)}%` }} /></div>
          <ul className="an-agents">
            {ordered(p.agents).map((a) => (
              <li key={a.agent} className={a.state}>
                <span className="an-ic">{a.state === 'reading' ? <i className="an-spin" /> : <Icon name={a.state === 'done' ? 'check' : a.state === 'failed' ? 'alert' : AGENT_ICON[a.agent] ?? 'spark'} />}</span>
                <span className="an-ag">
                  <b>{a.agent}</b>
                  {(a.covers || a.summary) && <small>{[a.covers, a.state === 'failed' ? '' : a.summary].filter(Boolean).join(' · ')}</small>}
                </span>
                <em>{a.state === 'done' && a.done_at ? `selesai ${hhmm(new Date(a.done_at))}` : STATE_LABEL[a.state]}</em>
              </li>
            ))}
          </ul>
        </>
      )}
      {p.steps.length > 0 && (
        <ol className="an-steps" aria-label="Langkah AI terakhir">
          {p.steps.slice(0, 4).map((x, i) => (
            <li key={x.at + i}><span className="num">{hhmm(new Date(x.at))}</span>{x.text}{x.client && <small> · {x.client}</small>}</li>
          ))}
        </ol>
      )}
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
