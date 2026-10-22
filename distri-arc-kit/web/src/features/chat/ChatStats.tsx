import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import { Icon } from '../../components/Icon'
import { shortDate } from '../../lib/format'
import { useNow } from '../../app/queries'

// Statistik on the chat's lead panel (Sam 22 Oct: clearer, more detail, many insights): the whole stored
// conversation — insights first, then the numbers, the activity by month, hour and weekday, who starts the talks,
// what is asked and who writes in a group. Computed by the server from the messages (GET /chat/threads/{id}/stats).

interface Count { name: string; n: number; qty?: number }
export interface ChatStatsData {
  messages: number; inbound: number; outbound: number; media: number
  first_at: string | null; last_at: string | null; last_in_at: string | null; active_days: number
  last_30: number; prev_30: number; trend_pct: number | null
  reply: { turns: number; answered: number; median_min: number | null; p90_min: number | null; within_1h_pct: number | null; waiting_since: string | null }
  sessions: number; customer_opens: number
  months: { month: string; in: number; out: number }[]
  hours: number[]; weekdays: number[]; off_hours_pct: number
  intents: Count[]; items: Count[]; senders: Count[]
  insights: { tone: 'good' | 'warn' | 'bad' | 'info'; text: string }[]
}

const DAYS = ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des']
const TONE_ICON = { good: 'check', warn: 'alert', bad: 'alert', info: 'spark' } as const

/** "45 mnt", "5 jam", "3 hari" */
export function minutes(m: number | null): string {
  if (m == null) return '—'
  if (m < 60) return `${Math.max(1, m)} mnt`
  if (m < 48 * 60) return `${Math.round(m / 60)} jam`
  return `${Math.round(m / (24 * 60))} hari`
}

/** "Okt" from "2026-10" */
export const monthLabel = (k: string) => MONTHS[Number(k.slice(5, 7)) - 1] ?? k

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
const num = (n: number) => n.toLocaleString('id-ID')

export function ChatStats({ thread, group }: { thread: string; group: boolean }) {
  const now = useNow().getTime()
  const { data: s, isLoading } = useQuery({
    queryKey: ['chat', 'stats', thread],
    queryFn: () => api.get<ChatStatsData>(`/chat/threads/${thread}/stats`),
    staleTime: 60_000,
  })
  if (isLoading || !s) return <p className="ibp-note">Menghitung statistik…</p>
  if (s.messages === 0) return <p className="ibp-note">Belum ada pesan tersimpan untuk dihitung.</p>
  const r = s.reply
  const maxMonth = Math.max(1, ...s.months.flatMap((m) => [m.in, m.out]))
  const maxHour = Math.max(1, ...s.hours)
  const maxDay = Math.max(1, ...s.weekdays)
  const openPct = s.sessions ? Math.round((s.customer_opens / s.sessions) * 100) : 0
  const intentTotal = s.intents.reduce((a, c) => a + c.n, 0)
  return (
    <div className="cst">
      {s.insights.length > 0 && (
        <section>
          <h4>Insight</h4>
          <ul className="cst-ins">
            {s.insights.map((x) => <li key={x.text} className={x.tone}><Icon name={TONE_ICON[x.tone]} /><span>{x.text}</span></li>)}
          </ul>
        </section>
      )}

      <section>
        <h4>Ringkasan {group ? 'grup' : 'percakapan'}</h4>
        <div className="cst-kpi">
          <div><span>Total pesan</span><b>{num(s.messages)}</b><small>{num(s.inbound)} masuk · {num(s.outbound)} keluar</small></div>
          <div><span>30 hari terakhir</span><b>{num(s.last_30)}</b>
            <small className={s.trend_pct == null ? '' : s.trend_pct >= 0 ? 'up' : 'down'}>{s.trend_pct == null ? `sebelumnya ${num(s.prev_30)}` : `${s.trend_pct >= 0 ? '▲' : '▼'} ${Math.abs(s.trend_pct)}% vs 30 hari sebelumnya`}</small></div>
          <div><span>Median balasan</span><b>{minutes(r.median_min)}</b><small>{r.p90_min != null ? `10% terlambat > ${minutes(r.p90_min)}` : 'belum ada balasan'}</small></div>
          <div><span>Dibalas &lt; 1 jam</span><b>{r.within_1h_pct != null ? `${r.within_1h_pct}%` : '—'}</b><small>{num(r.answered)} dari {num(r.turns)} giliran</small></div>
          <div className={r.waiting_since ? 'warn' : ''}><span>Belum dibalas</span><b>{r.waiting_since ? minutes(Math.round((now - new Date(r.waiting_since).getTime()) / 60000)) : 'Tidak ada'}</b><small>{r.waiting_since ? `sejak ${shortDate(r.waiting_since)}` : 'semua giliran terjawab'}</small></div>
          <div><span>Hari aktif</span><b>{num(s.active_days)}</b><small>{s.first_at ? `sejak ${shortDate(s.first_at)}` : ''}{s.media ? ` · ${num(s.media)} media` : ''}</small></div>
        </div>
      </section>

      <section>
        <h4>Aktivitas 6 bulan</h4>
        <div className="cst-legend"><span><i className="in" />Masuk</span><span><i className="out" />Keluar</span></div>
        <div className="cst-months" role="img" aria-label={s.months.map((m) => `${monthLabel(m.month)} ${m.in} masuk ${m.out} keluar`).join(', ')}>
          {s.months.map((m) => (
            <div key={m.month} className="cst-mo" title={`${monthLabel(m.month)} ${m.month.slice(0, 4)} · ${m.in} masuk · ${m.out} keluar`}>
              <div className="cst-bars">
                <i className="in" style={{ height: `${(m.in / maxMonth) * 100}%` }} />
                <i className="out" style={{ height: `${(m.out / maxMonth) * 100}%` }} />
              </div>
              <span>{monthLabel(m.month)}</span>
            </div>
          ))}
        </div>
      </section>

      {s.inbound > 0 && (
        <section>
          <h4>Kapan pelanggan menulis</h4>
          <div className="cst-hours" role="img" aria-label="Pesan pelanggan per jam (WIB)">
            {s.hours.map((n, h) => <i key={h} title={`${String(h).padStart(2, '0')}.00–${String(h + 1).padStart(2, '0')}.00 · ${n} pesan`} style={{ opacity: n ? 0.18 + 0.82 * (n / maxHour) : 0.06 }} />)}
          </div>
          <div className="cst-hours-ax"><span>00</span><span>06</span><span>12</span><span>18</span><span>24</span></div>
          <div className="cst-days">
            {s.weekdays.map((n, d) => (
              <div key={d} title={`${DAYS[d]} · ${n} pesan`}><span>{DAYS[d]}</span><div className="cst-track"><i style={{ width: `${(n / maxDay) * 100}%` }} /></div><b>{n}</b></div>
            ))}
          </div>
          <p className="cst-note">{s.off_hours_pct}% di luar jam kerja (sebelum 08.00, setelah 17.00, atau Minggu)</p>
        </section>
      )}

      {s.sessions > 0 && (
        <section>
          <h4>Siapa memulai percakapan</h4>
          <div className="cst-split" title={`${s.customer_opens} dari ${s.sessions} percakapan dimulai pelanggan`}>
            <i className="cust" style={{ width: `${openPct}%` }} /><i className="team" style={{ width: `${100 - openPct}%` }} />
          </div>
          <div className="cst-legend"><span><i className="cust" />Pelanggan {openPct}%</span><span><i className="team" />Tim {100 - openPct}%</span><span>{num(s.sessions)} percakapan</span></div>
        </section>
      )}

      {s.items.length > 0 && (
        <section>
          <h4>Barang paling sering disebut</h4>
          <ol className="cst-rank">
            {s.items.map((c) => <li key={c.name}><span>{c.name}</span><b>{c.n}×</b>{c.qty ? <small>{num(c.qty)} unit</small> : <small />}</li>)}
          </ol>
        </section>
      )}

      {s.intents.length > 0 && (
        <section>
          <h4>Permintaan pelanggan</h4>
          <div className="cst-days">
            {s.intents.map((c) => (
              <div key={c.name} title={`${cap(c.name)} · ${c.n} pesan`}><span className="wide">{cap(c.name)}</span><div className="cst-track"><i style={{ width: `${(c.n / intentTotal) * 100}%` }} /></div><b>{Math.round((c.n / intentTotal) * 100)}%</b></div>
            ))}
          </div>
        </section>
      )}

      {group && s.senders.length > 0 && (
        <section>
          <h4>Anggota pelanggan paling aktif</h4>
          <ol className="cst-rank">
            {s.senders.map((c) => <li key={c.name}><span>{c.name}</span><b>{num(c.n)}</b><small>pesan</small></li>)}
          </ol>
        </section>
      )}
      <p className="ibp-note">Dihitung sistem dari seluruh pesan tersimpan{s.last_at ? ` sampai ${shortDate(s.last_at)}` : ''}; permintaan dan barang dari bacaan Analisa WhatsApp.</p>
    </div>
  )
}
