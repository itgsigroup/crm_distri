import { Fragment, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { NextAction, Plan, PlanStep } from '../../api/types'
import { ActBtn } from '../../components/actions'
import { useFeedback } from '../../components/feedback'

/** Renders plan text with [[dealer:<slug>|Label]] links as the mockup's dealer buttons. */
export function PlanText({ text }: { text: string }) {
  const nav = useNavigate()
  const parts: ReactNode[] = []
  const re = /\[\[dealer:([^|\]]+)\|([^\]]+)\]\]/g
  let last = 0
  for (const m of text.matchAll(re)) {
    parts.push(text.slice(last, m.index))
    const slug = m[1]
    parts.push(<button key={m.index} className="ev" onClick={() => nav('/dealer/' + slug)}>{m[2]}</button>)
    last = (m.index ?? 0) + m[0].length
  }
  parts.push(text.slice(last))
  return <>{parts.map((p, i) => <Fragment key={i}>{p}</Fragment>)}</>
}

const STATUS: Record<string, string> = { running: 'berjalan', waiting: 'menunggu pembayaran' }

function stepAction(it: PlanStep): NextAction | null {
  const p = it.proposals[0]
  if (!p) return null
  return { id: p.id, kind: p.kind, title: '', button: p.button, icon: p.icon, agent: it.agent ?? '', due_label: '', status: p.status, why: '', decided_at: null, executed_at: null, autonomy: p.autonomy }
}

/** Rencana hari ini (mockup renderPlan). */
export function PlanList({ plan }: { plan: Plan | undefined }) {
  const nav = useNavigate()
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const runNow = useMutation({
    mutationFn: (id: string) => api.post<{ result: string }>(`/plan/${id}/run`),
    onSuccess: (r) => {
      toast(r.result)
      for (const k of ['plan', 'proposals', 'dealers', 'dealer', 'chat']) qc.invalidateQueries({ queryKey: [k] })
    },
    onError: (e: Error) => toast(e.message),
  })
  const items = plan?.items ?? []
  if (items.length === 0) {
    return (
      <ol className="plan">
        <li>
          <span className="pt">—</span>
          <div className="pb"><div className="px" style={{ color: 'var(--text-2)' }}>Belum ada rencana. Orchestrator menyusun Rencana hari ini dari siklus pertamanya: jadwal order, penagihan, dan push stok, diurutkan per jam.</div><div className="pm"><span className="ai">Orchestrator</span><span className="pill neutral">menunggu siklus</span></div></div>
          <div className="pa" />
        </li>
      </ol>
    )
  }
  return (
    <ol className="plan">
      {items.map((it) => {
        const auto = it.autonomy === 'auto'
        const done = it.status === 'done'
        const rej = it.status === 'skipped'
        const next = stepAction(it)
        return (
          <li key={it.id} className={done ? 'done' : rej ? 'rej' : ''}>
            <span className="pt">{it.time_label}</span>
            <div className="pb">
              <div className="px"><PlanText text={it.text_html ?? ''} /></div>
              <div className="pm">
                <span className="ai">{it.agent}</span>
                <span className={`pill ${auto ? 'good' : 'warn'}`}>{auto ? 'otonom' : 'butuh approve'}</span>
                {done ? <span className="pill good">✓ dijalankan</span> : rej ? <span className="pill bad">ditolak</span> : <span className="pill neutral">{STATUS[it.status] ?? (auto ? 'terjadwal' : 'menunggu Anda')}</span>}
              </div>
            </div>
            <div className="pa">
              {it.link === 'chat' && !done && !rej && next?.status === 'proposed' ? (
                <span style={{ display: 'flex', gap: 6 }}>
                  <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => nav('/chat')}>Lihat</button>
                  <ActBtn next={next} small />
                </span>
              ) : !auto && next ? (
                <ActBtn next={next} small />
              ) : auto && !done && !rej && it.status !== 'waiting' && it.proposals.length > 0 ? (
                <button className="btn quiet" style={{ height: 28, fontSize: 12 }} disabled={runNow.isPending} onClick={() => runNow.mutate(it.id)}>Jalankan sekarang</button>
              ) : null}
            </div>
          </li>
        )
      })}
    </ol>
  )
}

/** "8 langkah · 3 otonom · 5 butuh approve" */
export const planMeta = (p: Plan | undefined) => (p && p.total ? `${p.total} langkah · ${p.auto} otonom · ${p.approve} butuh approve` : '0 langkah')
