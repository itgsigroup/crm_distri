import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { useFeedback } from '../../components/feedback'
import { fmtNum, hhmm, shortDate } from '../../lib/format'
import { fmtSize } from '../chat/bubble'

export interface ChatBackupFile { name: string; month: string; size: number; at: string; chats: number; messages: number }
interface ChatBackups { items: ChatBackupFile[]; keep_months: number; schedule: string; on: boolean }

/** "2026-10" → "Oktober 2026" */
export function monthLabel(m: string): string {
  const [y, mo] = m.split('-').map(Number)
  if (!y || !mo) return m
  const s = new Date(y, mo - 1, 1).toLocaleDateString('id-ID', { month: 'long', year: 'numeric' })
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/** Pengaturan → Cadangan chat (CEO/admin, ADR 0061), like WhatsApp's Setelan › Chat › Cadangan chat: the last
 * backup, "Cadangkan", and one zip per month of WhatsApp-format .txt chats that stays after the 90-day purge. */
export function ChatBackupCard() {
  const qc = useQueryClient()
  const { toast } = useFeedback()
  const { data, isError } = useQuery({ queryKey: ['chat', 'backups'], queryFn: () => api.get<ChatBackups>('/chat/backups'), retry: false, staleTime: 60_000 })
  const run = useMutation({
    mutationFn: () => api.post<{ chats: number; messages: number; ms: number }>('/chat/backups'),
    onSuccess: (r) => {
      toast(`Dicadangkan · ${fmtNum(r.chats)} chat · ${fmtNum(r.messages)} pesan · ${(r.ms / 1000).toLocaleString('id-ID', { maximumFractionDigits: 1 })} dtk`)
      qc.invalidateQueries({ queryKey: ['chat', 'backups'] })
    },
    onError: (e: Error) => toast(e.message),
  })
  if (isError || !data) return null
  const last = data.items.reduce<ChatBackupFile | null>((a, f) => (!a || f.at > a.at ? f : a), null)
  return (
    <div className="card">
      <div className="card-h"><h2>Cadangan chat</h2><span className="meta">Format teks WhatsApp · tanpa media</span></div>
      <ul className="rules">
        <li>
          <div>
            <b>Cadangan terakhir: {last ? `${shortDate(last.at)} ${hhmm(last.at)}` : 'belum ada'}</b>
            <span>{data.schedule}, sebelum chat di atas 90 hari dibersihkan · 1 zip per bulan, isinya 1 file .txt per chat persis seperti Ekspor chat WhatsApp · disimpan {data.keep_months > 0 ? `${data.keep_months} bulan` : 'selamanya'}</span>
          </div>
          <button className="btn primary" style={{ height: 30, fontSize: 12.5 }} disabled={run.isPending} onClick={() => run.mutate()}>
            <Icon name="shield" />{run.isPending ? 'Mencadangkan…' : 'Cadangkan'}
          </button>
        </li>
        {data.items.map((f) => (
          <li key={f.name}>
            <div><b>{monthLabel(f.month)}</b><span>{fmtNum(f.chats)} chat · {fmtNum(f.messages)} pesan · {fmtSize(f.size) || '0 B'} · ditulis {shortDate(f.at)} {hhmm(f.at)}</span></div>
            <a className="btn ghost" style={{ height: 26, fontSize: 12 }} href={`/api/chat/backups/${encodeURIComponent(f.name)}`} download><Icon name="doc" />Unduh</a>
          </li>
        ))}
      </ul>
      <p style={{ fontSize: 12, color: 'var(--text-3)', margin: '10px 0 0', lineHeight: 1.5 }}>Satu chat bisa juga diekspor dari Chat: menu ⌄ di daftar chat → <b>Ekspor chat</b>. Chat yang ditandai Obrolan pribadi ikut dihapus dari cadangan. Setiap unduhan tercatat di audit.</p>
    </div>
  )
}
