import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { api } from '../api/client'
import { Icon } from '../components/Icon'
import { SheetHead, useFeedback } from '../components/feedback'
import { PlanText } from '../features/control/Plan'
import { hhmm, shortDate } from '../lib/format'

interface AskSource { id: string; kind: string; at: string; text: string; dealer?: string }
/** The page that answers a question as a table (Sam 22 Oct): Cek stok with its branch, search and Analisa stok view. */
export interface AskShow { page: 'stockcheck'; branch?: string; search?: string; view?: string }
export interface AskAnswer { q: string; intent: string; text: string; dealer?: string; sources: AskSource[] | null; confidence: number; via: string; at: string; show?: AskShow }

/** The address of Cek stok answering a question: ?cabang=&cari=&filter=&tanya= */
export function showURL(s: AskShow, q: string): string {
  const p = new URLSearchParams({ tanya: q })
  if (s.branch) p.set('cabang', s.branch)
  if (s.search) p.set('cari', s.search)
  if (s.view) p.set('filter', s.view)
  return '/stok/cek?' + p.toString()
}

/** The question asked to the Orchestrator, answered from the cache the command bar filled (or asked again). */
export const askQuery = (q: string) => ({ queryKey: ['ask', q], queryFn: () => api.post<AskAnswer>('/ask', { q }), staleTime: 60_000 })

const ICON: Record<string, string> = { wa: 'chat', so: 'doc', invoice: 'doc', payment: 'cash', stock: 'box' }

/** Answer of a command-bar question (mockup askSheet), with the signals it rests on. */
export function AskSheet({ q }: { q: string }) {
  const { closeSheet } = useFeedback()
  const nav = useNavigate()
  const { data: a, error } = useQuery(askQuery(q))
  useEffect(() => {
    if (a?.dealer) {
      closeSheet()
      nav('/dealer/' + a.dealer)
    } else if (a?.show) { // the answer is a table: open it with its filters and the answer on top
      closeSheet()
      nav(showURL(a.show, q))
    }
  }, [a, q, closeSheet, nav])
  return (
    <>
      <SheetHead icon="spark" title={q} sub={`GSI Orbit · ${a ? hhmm(a.at) : 'menjawab…'}`} onClose={closeSheet} />
      <div className="sec">
        <p style={{ fontSize: 14.5 }}>{error ? (error as Error).message : a ? <PlanText text={a.text} /> : 'Membaca data dealer, SO, stok, dan pembayaran…'}</p>
      </div>
      {a && (
        <div className="sec" style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
          {(a.sources ?? []).map((s) => (
            <button key={s.id} className="prov" style={{ cursor: 'pointer' }} title={s.text} onClick={() => { closeSheet(); nav(s.kind === 'wa' ? '/chat' : '/dealer/' + s.dealer + '#sec-tl') }}>
              <Icon name={ICON[s.kind] ?? 'doc'} />{shortDate(s.at)} · {s.text.length > 40 ? s.text.slice(0, 40) + '…' : s.text}
            </button>
          ))}
          <span className="prov">conf {a.confidence.toFixed(2)}</span>
        </div>
      )}
      <div className="ft"><button className="btn ghost" onClick={closeSheet}>Tutup</button></div>
    </>
  )
}
