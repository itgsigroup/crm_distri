import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { api } from '../api/client'
import { Icon } from '../components/Icon'
import { SheetHead, useFeedback } from '../components/feedback'
import { PlanText } from '../features/control/Plan'
import { hhmm, shortDate } from '../lib/format'

interface AskSource { id: string; kind: string; at: string; text: string; dealer?: string }
interface AskAnswer { q: string; intent: string; text: string; dealer?: string; sources: AskSource[] | null; confidence: number; via: string; at: string }

const ICON: Record<string, string> = { wa: 'chat', so: 'doc', invoice: 'doc', payment: 'cash', stock: 'box' }

/** Answer of a command-bar question (mockup askSheet), with the signals it rests on. */
export function AskSheet({ q }: { q: string }) {
  const { closeSheet } = useFeedback()
  const nav = useNavigate()
  const { data: a, error } = useQuery({ queryKey: ['ask', q], queryFn: () => api.post<AskAnswer>('/ask', { q }), staleTime: 60_000 })
  useEffect(() => {
    if (a?.dealer) {
      closeSheet()
      nav('/dealer/' + a.dealer)
    }
  }, [a?.dealer, closeSheet, nav])
  return (
    <>
      <SheetHead icon="spark" title={q} sub={`Distri ARC · ${a ? hhmm(a.at) : 'menjawab…'}`} onClose={closeSheet} />
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
