import { Fragment, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { NextAction, PlanStep } from '../../api/types'
import { ActBtn } from '../../components/actions'
import { useFeedback } from '../../components/feedback'

// a run of dealer links ("A, B (3 / 2), C dan D") longer than this shows its first links and "+N dealer lainnya"
const RUN_SHOW = 5
const SEP = /^\s*(\([^)]*\))?\s*(,|;|dan)?\s*$/i

/** Renders plan text with [[dealer:<slug>|Label]] links as the mockup's dealer buttons; long lists of dealers are
 * collapsed so a sentence stays readable (expand with "+N dealer lainnya"). */
export function PlanText({ text, collapse = true }: { text: string; collapse?: boolean }) {
  const nav = useNavigate()
  const [open, setOpen] = useState(false)
  const re = /\[\[dealer:([^|\]]+)\|([^\]]+)\]\]/g
  const texts: string[] = []
  const links: { slug: string; label: string }[] = []
  let last = 0
  for (const m of text.matchAll(re)) {
    texts.push(text.slice(last, m.index))
    links.push({ slug: m[1], label: m[2] })
    last = (m.index ?? 0) + m[0].length
  }
  texts.push(text.slice(last))
  // texts[i] precedes links[i]; texts[i+1] follows it. A run continues while the text between two links is a separator.
  const hidden = new Set<number>()
  const tails = new Map<number, number>() // last shown link of a run → links hidden after it
  if (collapse && !open) {
    let start = 0
    for (let i = 1; i <= links.length; i++) {
      if (i < links.length && SEP.test(texts[i])) continue
      const len = i - start
      if (len > RUN_SHOW + 1) {
        for (let j = start + RUN_SHOW; j < i; j++) hidden.add(j)
        tails.set(start + RUN_SHOW - 1, len - RUN_SHOW)
      }
      start = i
    }
  }
  const parts: ReactNode[] = []
  // text after a hidden link starts with that link's "(17 / 14)": drop it
  const after = (i: number) => (hidden.has(i - 1) ? texts[i].replace(/^\s*\([^)]*\)/, '') : texts[i])
  links.forEach((l, i) => {
    if (!hidden.has(i)) parts.push(after(i))
    if (!hidden.has(i)) parts.push(<button key={'l' + i} className="ev" onClick={() => nav('/dealer/' + l.slug)}>{l.label}</button>)
    const more = tails.get(i)
    if (more) parts.push(<Fragment key={'m' + i}>{' '}<button className="ev-more" onClick={() => setOpen(true)}>+{more} dealer lainnya</button></Fragment>)
  })
  parts.push(after(texts.length - 1))
  return (
    <>
      {parts.map((p, i) => <Fragment key={i}>{p}</Fragment>)}
      {open && <> <button className="ev-more" onClick={() => setOpen(false)}>ringkas</button></>}
    </>
  )
}

const STATUS: Record<string, string> = { running: 'berjalan', waiting: 'menunggu pembayaran' }

function stepAction(it: PlanStep): NextAction | null {
  const p = it.proposals[0]
  if (!p) return null
  return { id: p.id, kind: p.kind, title: '', button: p.button, icon: p.icon, agent: it.agent ?? '', due_label: '', status: p.status, why: '', decided_at: null, executed_at: null, autonomy: p.autonomy }
}

/** The status pill of a plan step (Rencana hari ini). */
export function planStatus(it: PlanStep): { tone: string; text: string } {
  const auto = it.autonomy === 'auto'
  if (it.status === 'done') return { tone: 'good', text: '✓ ' + (it.link?.startsWith('chat:') ? 'dibalas' : 'dijalankan') }
  if (it.status === 'skipped') return { tone: 'bad', text: 'ditolak' }
  return { tone: 'neutral', text: STATUS[it.status] ?? (auto ? 'terjadwal' : 'menunggu Anda') }
}

/** The action of a plan step: open the chat, decide its proposal, or run an automatic step now. */
export function PlanAct({ it }: { it: PlanStep }) {
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
  const auto = it.autonomy === 'auto'
  const done = it.status === 'done'
  const rej = it.status === 'skipped'
  const next = stepAction(it)
  if (it.link?.startsWith('chat:')) return done ? null : <button className="btn ghost" onClick={() => nav('/chat/' + it.link!.slice(5))}>Buka chat</button>
  if (it.link === 'chat' && !done && !rej && next?.status === 'proposed') return <span className="cc-acts"><button className="btn ghost" onClick={() => nav('/chat')}>Lihat</button><ActBtn next={next} small /></span>
  if (!auto && next) return <ActBtn next={next} small />
  if (auto && !done && !rej && it.status !== 'waiting' && it.proposals.length > 0) return <button className="btn quiet" disabled={runNow.isPending} onClick={() => runNow.mutate(it.id)}>Jalankan sekarang</button>
  return null
}
