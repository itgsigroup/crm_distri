import { Link } from 'react-router'
import type { WAAnalysis, WAInsight, WAIntent, WASalesRow } from '../../api/types'
import { Icon } from '../../components/Icon'
import { Prov } from '../../components/ui'
import { fmtNum, hhmm, shortDate } from '../../lib/format'
import { useNow } from '../../app/queries'
import { waitLabel } from './wa'

// Analisa WhatsApp in the Pusat kendali layout of 08-frontend.md: numbers measured from the chats, the request each chat
// makes (order, rilis kredit, retur) and what Claude wrote through MCP (summary per conversation, overview) — ADR 0045.

const replyText = (m: number | null | undefined) => (m == null ? '—' : waitLabel(m))

/** Waiting conversations first (longest wait on top), then the rest by priority. */
export function byWait(rows: WAInsight[]): WAInsight[] {
  return [...rows].sort((a, b) => Number(!!b.waiting_since) - Number(!!a.waiting_since) || (b.waiting_since ? b.wait_min - a.wait_min : 0) || b.priority - a.priority)
}

export const waWaiting = (d: WAAnalysis | undefined) => byWait((d?.items ?? []).filter((r) => !!r.waiting_since))

/** The conversations whose customer asks one of these (order, release, return…), waiting ones first. */
export const waIntent = (d: WAAnalysis | undefined, ...ks: WAIntent[]) => byWait((d?.items ?? []).filter((r) => ks.includes(r.intent)))

/** Ringkasan Orchestrator from WhatsApp: the AI overview, else what the numbers say. */
export function WABrief({ data, scoped }: { data: WAAnalysis | undefined; scoped: boolean }) {
  const t = data?.totals
  return (
    <>
      {!data?.cycle_number ? (
        <p className="sg-hint">Orchestrator membaca chat WhatsApp di setiap siklus penuh (tiap jam 06.00–20.00 atau lewat Analisis ulang). Ringkasan muncul di sini setelah siklus pertama.</p>
      ) : data.overview && !scoped ? (
        <p className="wa-brief">{data.overview}</p>
      ) : (
        <p className="cl-empty" style={{ margin: '4px 0 8px' }}>
          {scoped ? 'Ringkasan Claude mencakup semua sales — di halaman sales ini hanya angkanya.' : <>Belum ada ringkasan dari Claude (MCP) pada siklus ini — yang tampil hanya angka dari chat. Tekan <b>Analisis ulang</b>, atau minta di Claude <i>"jalankan analisis GSI Orbit"</i>.</>}
        </p>
      )}
      <div className="foot">
        <Prov icon="chat">{fmtNum(t?.threads ?? 0)} percakapan</Prov>
        <Prov icon="building">{t?.dealers ?? 0} dealer</Prov>
        <Prov icon="people">{t?.new_numbers ?? 0} nomor baru</Prov>
        <Prov icon="doc">{fmtNum(t?.inbound ?? 0)} pesan masuk</Prov>
        <Prov style={{ marginLeft: 'auto' }}>{data?.source === 'ai' ? 'ditulis Claude (MCP)' : data?.cycle_number ? 'belum dianalisis Claude' : '—'}</Prov>
      </div>
    </>
  )
}

/** KPI utama from WhatsApp: three wheels (measured from the chats). */
export function WAWheels({ data }: { data: WAAnalysis | undefined }) {
  const t = data?.totals
  const due = data?.reply_due_min ?? 120
  const threads = t?.threads ?? 0
  const answeredPct = threads ? Math.round(((threads - (t?.waiting ?? 0)) / threads) * 100) : 100
  const med = t?.reply_median_min ?? null
  const late = t?.needs_reply ?? 0
  const W: [string, string, string, number, string, string][] = [
    ['Chat terbalas', answeredPct + '%', '% percakapan yang sudah dibalas sales', answeredPct, 'target ≥ 90%', answeredPct >= 90 ? 'good' : 'warn'],
    ['Waktu balas', replyText(med), `median jawaban substantif di jam kerja · p90 ${replyText(t?.reply_p90_min ?? null)}`, med == null ? 0 : Math.round((1 - med / (2 * due)) * 100), `target ≤ ${waitLabel(due)}`, med == null ? 'text-3' : med <= due ? 'good' : 'warn'],
    ['Menunggu', String(late), `chat menunggu lebih dari ${waitLabel(due)}`, late ? Math.max(5, 100 - late * 20) : 100, 'target 0', late ? 'bad' : 'good'],
  ]
  const c = 2 * Math.PI * 26
  return (
    <>
      <div className="wheels">
        {W.map(([title, v, s, p, tg, k]) => (
          <div className="wheel" key={title}>
            <svg viewBox="0 0 64 64">
              <circle cx="32" cy="32" r="26" fill="none" stroke="var(--surface-3)" strokeWidth="7" />
              <circle cx="32" cy="32" r="26" fill="none" stroke={`var(--${k})`} strokeWidth="7" strokeLinecap="round" strokeDasharray={c} strokeDashoffset={c * (1 - Math.max(0.05, Math.min(1, p / 100)))} transform="rotate(-90 32 32)" />
              <text x="32" y="36" textAnchor="middle">{v}</text>
            </svg>
            <div><b>{title}</b><span>{s}</span><small>{tg}</small></div>
          </div>
        ))}
      </div>
    </>
  )
}

/** Orchestrator "Agen" grid from WhatsApp: one card per sales. */
export function WASalesGrid({ rows }: { rows: WASalesRow[] }) {
  if (rows.length === 0) return <p className="sg-hint">Belum ada percakapan.</p>
  return (
    <div className="agent-grid">
      {rows.map((s) => (
        <div className="agent-card" key={s.name}>
          <div className="ac-h"><span className="ni"><Icon name="chat" /></span><div><b>{s.name || 'Tanpa sales'}</b><span>{s.threads} percakapan · {s.inbound} pesan masuk</span></div></div>
          <div className="ac-kv"><span><small>Belum dibalas</small>{s.waiting}</span><span><small>Lewat 2 jam</small>{s.needs_reply}</span><span><small>Waktu balas</small>{replyText(s.reply_median_min)}</span><span><small>p90</small>{replyText(s.reply_p90_min ?? null)}</span><span><small>Luar jam</small>{replyText(s.reply_off_median_min ?? null)}</span></div>
        </div>
      ))}
    </div>
  )
}

/** The label of Claude's Analisa WhatsApp every 2 hours (ADR 0037): succeeded, failed (why), running, and the next
 * run; the history is on MCP Claude. */
export function WAClaudeStatus({ data }: { data?: WAAnalysis }) {
  const now = useNow()
  const c = data?.claude
  if (!c) return null
  const last = c.last
  const at = (d?: string | null) => (d ? `${shortDate(d)} ${hhmm(d)}` : '')
  const next = c.enabled && c.next_run_at ? `berikutnya ${hhmm(c.next_run_at)}` : 'jadwal dijeda'
  let tone = 'neutral'
  let text = 'Analisa Claude belum pernah berjalan'
  // a routine on claude.ai sends it every 2 hours: no success for 2½ hours means it did not come
  const late = c.runner === 'external' && c.enabled && (!c.last_ok_at || now.getTime() - new Date(c.last_ok_at).getTime() > 150 * 60_000)
  if (late && last?.status !== 'error') {
    tone = 'bad'
    text = c.last_ok_at ? `Analisa Claude terlambat · terakhir berhasil ${at(c.last_ok_at)}` : 'Analisa Claude belum masuk dari routine claude.ai'
  } else if (last?.status === 'running') {
    tone = 'accent'
    text = 'Claude sedang menganalisa WhatsApp…'
  } else if (last?.status === 'ok') {
    tone = 'good'
    text = `Analisa Claude berhasil · ${at(last.finished_at ?? last.started_at)}`
  } else if (last) {
    tone = 'bad'
    text = `Analisa Claude gagal · ${at(last.finished_at ?? last.started_at)}`
  }
  return (
    <div className={`wa-claude ${tone}`} title={last?.error ?? undefined}>
      <span className="ai" style={{ fontSize: 0 }} />
      <b>{text}</b>
      <span>tiap 2 jam lewat MCP{c.runner === 'external' ? ' · routine claude.ai' : ''} · {next}</span>
      {last?.status !== 'ok' && last?.error && <span className="wa-claude-err">{last.error}</span>}
      <Link to="/claude">Riwayat</Link>
    </div>
  )
}
