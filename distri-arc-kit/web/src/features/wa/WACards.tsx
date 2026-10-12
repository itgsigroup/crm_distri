import { useState } from 'react'
import { useNavigate } from 'react-router'
import type { WAAnalysis, WAInsight, WAIntent, WASalesRow } from '../../api/types'
import { Icon } from '../../components/Icon'
import { useMore } from '../../components/More'
import { Pill, Prov } from '../../components/ui'
import { fmtNum } from '../../lib/format'
import { INTENTS, intentLabel, intentTone, itemsText, waitLabel, who } from './wa'

// Analisa WhatsApp in the original Pusat kendali / Orchestrator layout: one WhatsApp card for every former BigQuery card.

const replyText = (m: number | null | undefined) => (m == null ? '—' : waitLabel(m))
const initials = (name: string) => name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]!.toUpperCase()).join('') || '?'

/** Waiting conversations first (longest wait on top), then the rest by priority. */
export function byWait(rows: WAInsight[]): WAInsight[] {
  return [...rows].sort((a, b) => Number(!!b.waiting_since) - Number(!!a.waiting_since) || (b.waiting_since ? b.wait_min - a.wait_min : 0) || b.priority - a.priority)
}

export const waWaiting = (d: WAAnalysis | undefined) => byWait((d?.items ?? []).filter((r) => !!r.waiting_since))
export const waIntent = (d: WAAnalysis | undefined, ...ks: WAIntent[]) => byWait((d?.items ?? []).filter((r) => ks.includes(r.intent)))

/** One conversation per row (the row-list of the original Jadwal order / Lewat jadwal cards). */
export function WAChatList({ items, empty, limit = 8 }: { items: WAInsight[]; empty: string; limit?: number }) {
  const nav = useNavigate()
  const [shown, more] = useMore(items, limit)
  if (items.length === 0) return <p className="sg-hint">{empty}</p>
  return (
    <>
      <ul className="row-list">
        {shown.map((r) => (
          <li key={r.thread_id}>
            <span className="who" style={{ width: 'auto' }}>
              {r.waiting_since ? <Pill tone={r.needs_reply ? 'bad' : 'warn'}>{waitLabel(r.wait_min)}</Pill> : r.answered ? <Pill tone="good" icon="check">dibalas</Pill> : <Pill tone="neutral">—</Pill>}
            </span>
            <div>
              <div className="t">
                {r.dealer_id ? <button className="ev" onClick={() => nav('/dealer/' + r.dealer_id)}>{who(r)}</button> : <b>{who(r)}</b>}
                {r.kind === 'new' && <> · <Pill tone="indigo">nomor baru</Pill></>}
                {' · '}<span className={`pill ${intentTone(r.intent)}`}>{intentLabel(r.intent)}</span>
                {r.sales && <> · {r.sales}</>}
              </div>
              <div className="s">
                {r.items.length > 0 && <>{itemsText(r.items)} · </>}
                {r.summary ? <><span className="ai" />{r.summary}</> : r.last_text ? <q>{r.last_text}</q> : 'hanya pesan sales'}
                {r.proposal_ids.length > 0 && <> · <Icon name="spark" />{r.proposal_ids.length} usulan agen</>}
              </div>
            </div>
            <button className="btn ghost" style={{ height: 28, fontSize: 12 }} onClick={() => nav('/chat/' + r.thread_id)}>Buka chat</button>
          </li>
        ))}
      </ul>
      {more}
    </>
  )
}

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
          {scoped ? 'Ringkasan AI mencakup semua sales — di halaman sales ini hanya angkanya.' : <>Belum ada ringkasan dari AI pada siklus ini — angka dihitung GSI Orbit dari chat. Minta Claude lewat MCP: <i>"jalankan analisis GSI Orbit"</i>, atau tekan <b>Analisis ulang → Lewat MCP</b>.</>}
        </p>
      )}
      <div className="foot">
        <Prov icon="chat">{fmtNum(t?.threads ?? 0)} percakapan</Prov>
        <Prov icon="building">{t?.dealers ?? 0} dealer</Prov>
        <Prov icon="people">{t?.new_numbers ?? 0} nomor baru</Prov>
        <Prov icon="doc">{fmtNum(t?.inbound ?? 0)} pesan masuk</Prov>
        <Prov style={{ marginLeft: 'auto' }}>{data?.source === 'ai' ? 'ditulis AI' : data?.cycle_number ? 'belum dianalisis AI' : '—'}</Prov>
      </div>
    </>
  )
}

/** KPI utama from WhatsApp: three wheels plus what dealers asked for. */
export function WAWheels({ data }: { data: WAAnalysis | undefined }) {
  const t = data?.totals
  const due = data?.reply_due_min ?? 120
  const threads = t?.threads ?? 0
  const answeredPct = threads ? Math.round(((threads - (t?.waiting ?? 0)) / threads) * 100) : 100
  const med = t?.reply_median_min ?? null
  const late = t?.needs_reply ?? 0
  const W: [string, string, string, number, string, string][] = [
    ['Chat terbalas', answeredPct + '%', '% percakapan yang sudah dibalas sales', answeredPct, 'target ≥ 90%', answeredPct >= 90 ? 'good' : 'warn'],
    ['Waktu balas', replyText(med), 'median waktu balas per percakapan', med == null ? 0 : Math.round((1 - med / (2 * due)) * 100), `target ≤ ${waitLabel(due)}`, med == null ? 'text-3' : med <= due ? 'good' : 'warn'],
    ['Menunggu', String(late), `chat menunggu lebih dari ${waitLabel(due)}`, late ? Math.max(5, 100 - late * 20) : 100, 'target 0', late ? 'bad' : 'good'],
  ]
  const c = 2 * Math.PI * 26
  const max = Math.max(1, ...INTENTS.map((i) => t?.by_intent[i.key] ?? 0))
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
      <div className="wa-intents one" aria-label="Permintaan di chat" style={{ marginTop: 14 }}>
        {INTENTS.map((i) => {
          const n = t?.by_intent[i.key] ?? 0
          return (
            <div className="wa-int" key={i.key}>
              <span className="l">{i.label}</span>
              <span className="bar"><i style={{ width: `${(n / max) * 100}%`, background: `var(--${i.tone === 'neutral' ? 'text-3' : i.tone})` }} /></span>
              <span className="v num">{n}</span>
            </div>
          )
        })}
      </div>
    </>
  )
}

/** Agenda sales from WhatsApp: per sales, the conversations still waiting for their reply. */
export function WAAgenda({ data }: { data: WAAnalysis | undefined }) {
  const nav = useNavigate()
  const [openRow, setOpenRow] = useState('')
  const rows = data?.sales ?? []
  const waitingOf = (s: WASalesRow) => byWait((data?.items ?? []).filter((r) => r.sales === s.name && !!r.waiting_since))
  const busy = rows.filter((s) => s.waiting > 0).sort((a, b) => b.needs_reply - a.needs_reply || b.waiting - a.waiting)
  const idle = rows.filter((s) => s.waiting === 0)
  const [shown, more] = useMore(busy, 6)
  return (
    <div className="agenda">
      {shown.map((s) => {
        const items = waitingOf(s)
        const all = openRow === s.name
        return (
          <div className="ag-row" key={s.name}>
            <div className="ag-who">
              <span className="avatar">{initials(s.name || '?')}</span>
              <div><b>{s.name || 'Tanpa sales'}</b><span>{s.threads} chat · balas {replyText(s.reply_median_min)}</span></div>
              <span className="ag-n" title="chat belum dibalas">{s.waiting}</span>
            </div>
            <ul>
              {(all ? items : items.slice(0, 3)).map((r) => (
                <li key={r.thread_id}>
                  <span className="dot" style={{ background: r.needs_reply ? 'var(--bad)' : 'var(--warn)' }} />
                  <button className="ev" onClick={() => nav('/chat/' + r.thread_id)}>{who(r)}</button>
                  <span className="ag-what">{intentLabel(r.intent).toLowerCase()} · {waitLabel(r.wait_min)}</span>
                </li>
              ))}
              {items.length > 3 && <li><button className="ev-more" onClick={() => setOpenRow(all ? '' : s.name)}>{all ? 'ringkas' : `+${items.length - 3} chat lainnya`}</button></li>}
            </ul>
          </div>
        )
      })}
      {more}
      {idle.length > 0 && <p className="ag-idle">{idle.length} sales tanpa chat menunggu: {idle.slice(0, 6).map((s) => s.name.split(' ')[0]).join(', ')}{idle.length > 6 ? `, +${idle.length - 6}` : ''}</p>}
      {rows.length === 0 && <p className="ag-idle">Belum ada percakapan.</p>}
    </div>
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
          <div className="ac-kv"><span><small>Belum dibalas</small>{s.waiting}</span><span><small>Lewat 2 jam</small>{s.needs_reply}</span><span><small>Waktu balas</small>{replyText(s.reply_median_min)}</span></div>
          <div className="ac-out"><span className="ai" />{s.needs_reply ? `${s.needs_reply} chat perlu dibalas sekarang` : s.waiting ? `${s.waiting} chat belum dibalas, masih dalam batas` : 'Semua chat sudah dibalas'}</div>
        </div>
      ))}
    </div>
  )
}

/** Orchestrator "Matriks" from WhatsApp: what dealers asked for, how much still waits, how much the agents answered. */
export function WAIntentMatrix({ data }: { data: WAAnalysis | undefined }) {
  const items = data?.items ?? []
  return (
    <div className="tbl-wrap">
      <table className="matrix">
        <thead><tr><th>Permintaan</th><th className="r">Chat</th><th className="r">Belum dibalas</th><th className="r">Lewat 2 jam</th><th className="r">Usulan agen</th></tr></thead>
        <tbody>
          {INTENTS.map((i) => {
            const rs = items.filter((r) => r.intent === i.key)
            return (
              <tr key={i.key}>
                <td><span className={`pill ${i.tone}`}>{i.label}</span></td>
                <td className="r num">{rs.length}</td>
                <td className="r num">{rs.filter((r) => !!r.waiting_since).length}</td>
                <td className="r num" style={rs.some((r) => r.needs_reply) ? { color: 'var(--bad)', fontWeight: 700 } : undefined}>{rs.filter((r) => r.needs_reply).length}</td>
                <td className="r num">{rs.reduce((n, r) => n + r.proposal_ids.length, 0)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
