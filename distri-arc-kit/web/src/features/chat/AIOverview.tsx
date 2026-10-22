import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { WAIntent, WAItem } from '../../api/types'
import { shortDate } from '../../lib/format'
import { intentLabel, intentTone, itemsText } from '../wa/wa'

// Analisa Claude on the chat's tab Main (Sam 22 Oct): what Claude wrote through the Orchestrator about this
// customer, start to end — the company, the conversation or customer group, and the cooperation. Only AI-written
// text, each line with its cycle, date and confidence (GET /chat/threads/{id}/ai-overview).

export interface AIOverviewData {
  company: { status: 'bermasalah' | 'perlu_perhatian' | 'aman'; overview: string; reasons: string[]; confidence: number | null; cycle_number: number | null; at: string } | null
  chat: { messages: number; inbound: number; first_at: string | null; last_at: string | null; notes: number }
  timeline: { cycle_number: number | null; at: string; intent: WAIntent; items: WAItem[] | null; summary: string; confidence: number | null }[]
  proposals: { id: string; agent: string; title: string; why: string; status: string; created_at: string; decided_at?: string; decided_by?: string; cycle_number: number | null }[]
}

const COMPANY: Record<string, [string, string]> = { bermasalah: ['Bermasalah', 'bad'], perlu_perhatian: ['Perlu perhatian', 'warn'], aman: ['Aman', 'good'] }
export const DECISION: Record<string, [string, string]> = {
  proposed: ['Menunggu keputusan', 'accent'], approved: ['Disetujui', 'good'], edited: ['Disetujui · diedit', 'good'], executed: ['Dijalankan', 'good'],
  rejected: ['Ditolak', 'bad'], expired: ['Kedaluwarsa', 'neutral'],
}

/** "Siklus #1.204 · 12 Okt · yakin 80%" — where a line came from. */
export function provenance(cycle: number | null, at: string, confidence: number | null): string {
  return [cycle ? `Siklus #${cycle.toLocaleString('id-ID')}` : null, shortDate(at), confidence != null ? `yakin ${confidence}%` : null].filter(Boolean).join(' · ')
}

export function AIOverview({ thread, group }: { thread: string; group: boolean }) {
  const { data: a, isLoading } = useQuery({
    queryKey: ['chat', 'ai-overview', thread],
    queryFn: () => api.get<AIOverviewData>(`/chat/threads/${thread}/ai-overview`),
    staleTime: 60_000,
  })
  if (isLoading || !a) return <div className="aio"><h4>Analisa Claude · dari awal sampai akhir</h4><p className="ibp-note">Memuat analisa…</p></div>
  const empty = !a.company && a.timeline.length === 0 && a.proposals.length === 0
  const c = a.chat
  const decided = a.proposals.filter((p) => p.status !== 'proposed').length
  return (
    <div className="aio">
      <h4>Analisa Claude · dari awal sampai akhir <span className="ai" style={{ marginLeft: 4, fontSize: 0 }} /></h4>
      {empty && <p className="ibp-note">Belum ada analisa Claude untuk {group ? 'grup' : 'chat'} ini — jalankan Orchestrator di samping nama chat. Hanya hasil Claude yang tampil di sini.</p>}

      {a.company && (
        <section className="aio-sec">
          <div className="aio-h"><b>Overview perusahaan</b><em className={COMPANY[a.company.status]?.[1] ?? 'neutral'}>{COMPANY[a.company.status]?.[0] ?? a.company.status}</em></div>
          <p>{a.company.overview}</p>
          {a.company.reasons.length > 0 && <ul className="aio-list">{a.company.reasons.map((r) => <li key={r}>{r}</li>)}</ul>}
          <small>{provenance(a.company.cycle_number, a.company.at, a.company.confidence)}</small>
        </section>
      )}

      {(a.timeline.length > 0 || c.messages > 0) && (
        <section className="aio-sec">
          <div className="aio-h"><b>{group ? 'Perjalanan grup' : 'Perjalanan percakapan'}</b></div>
          {c.first_at && c.last_at && (
            <p className="aio-facts">
              Sejak <b>{shortDate(c.first_at)}</b> sampai <b>{shortDate(c.last_at)}</b> · {c.messages.toLocaleString('id-ID')} pesan ({c.inbound.toLocaleString('id-ID')} masuk)
              {c.notes > 0 && <> · {c.notes.toLocaleString('id-ID')} dicatat Claude</>}
            </p>
          )}
          {a.timeline.length > 0 && (
            <ol className="aio-tl">
              {a.timeline.map((l, i) => (
                <li key={l.at + i}>
                  <span className="aio-dot" />
                  <div>
                    <span className="aio-tl-h"><em className={intentTone(l.intent)}>{intentLabel(l.intent)}</em>{l.items && l.items.length > 0 && <span>{itemsText(l.items)}</span>}</span>
                    <p>{l.summary}</p>
                    <small>{provenance(l.cycle_number, l.at, l.confidence)}</small>
                  </div>
                </li>
              ))}
            </ol>
          )}
        </section>
      )}

      {a.proposals.length > 0 && (
        <section className="aio-sec">
          <div className="aio-h"><b>Kerjasama</b><span>{a.proposals.length} usulan Claude · {decided} diputuskan</span></div>
          <ul className="aio-props">
            {a.proposals.map((p) => {
              const [label, tone] = DECISION[p.status] ?? [p.status, 'neutral']
              return (
                <li key={p.id}>
                  <div className="aio-tl-h"><b>{p.title}</b><em className={tone}>{label}</em></div>
                  {p.why && <p>{p.why}</p>}
                  <small>{p.agent} · {provenance(p.cycle_number, p.created_at, null)}{p.decided_by ? ` · diputuskan ${p.decided_by}${p.decided_at ? ' ' + shortDate(p.decided_at) : ''}` : ''}</small>
                </li>
              )
            })}
          </ul>
        </section>
      )}
      {!empty && <p className="ibp-note">Ditulis Claude lewat Orchestrator · AI hanya mengusulkan, keputusan tetap di tim.</p>}
    </div>
  )
}
