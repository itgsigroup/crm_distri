import { Link } from 'react-router'
import { Icon } from '../../components/Icon'
import { fmtNum, hhmm, shortDate } from '../../lib/format'
import { CREW, CREW_STATE_LABEL } from './station'
import type { Crew } from './useCrew'

const VIA: Record<string, string> = { system: 'Sistem', mcp: 'MCP Claude', api: 'API' }

/** Hasil overview AI: what each of the ten astronauts found, live while a cycle runs, else from the last cycle and
 * the current counts. Counts only — no customer names. */
export function AiOverview({ crew, can }: { crew: Crew; can: (screen: string) => boolean }) {
  const { status, lines, lastDone, lastSaid, prog, running, ended, fresh, waiting } = crew
  const live = !!prog && !ended
  const meta = live
    ? `Siklus #${fmtNum(prog!.number ?? 0)} sedang berjalan`
    : prog && ended
      ? `Siklus #${fmtNum(prog.number ?? 0)} · ${prog.status === 'failed' ? 'gagal' : 'selesai'}${prog.via ? ` · ${VIA[prog.via] ?? prog.via}` : ''}`
      : lastDone
        ? `Siklus terakhir #${fmtNum(lastDone.number ?? 0)} · ${shortDate(lastDone.started_at)} ${hhmm(lastDone.finished_at ?? lastDone.started_at)} WIB${lastDone.via ? ` · ${VIA[lastDone.via] ?? lastDone.via}` : ''}`
        : 'Belum ada siklus analisis'
  return <div className="card db-panel ai-ov">
    <div className="card-h"><h2>Hasil overview AI</h2><span className="meta">{meta}</span></div>
    <div className="ai-ov-head">
      <div>
        <b>{running ? 'Kru AI sedang bekerja…' : ended && fresh ? `${fmtNum(fresh)} usulan baru dari siklus ini` : `${fmtNum(waiting.length)} usulan menunggu keputusan Anda`}</b>
        <span>Sepuluh astronot membaca data berurutan — sales, WhatsApp, dealer, lalu enam agen — sampai data siap diputuskan manusia.</span>
      </div>
      {can('today') && <Link className="btn primary ai-ov-go" to="/">Putuskan di Pusat kendali</Link>}
    </div>
    <ol className="ai-ov-grid">
      {CREW.map((m) => {
        const w = status[m.key]
        const working = w.state !== 'idle'
        const said = lastSaid[m.key]
        const body = <>
          <span className="ai-ov-n" style={{ background: m.color, color: m.key === 'prospect' || m.key === 'ready' || m.key === 'followup' ? '#161200' : '#fff' }}>{m.n}</span>
          <div>
            <div className="ai-ov-t"><b>{m.title}</b><em className={`ai-ov-st ${w.state}`}>{CREW_STATE_LABEL[w.state]}</em></div>
            <p>{lines[m.key]}</p>
            {working && w.text !== lines[m.key] && !w.text.startsWith(CREW_STATE_LABEL[w.state]) ? <small className="ai-ov-now">{w.text}</small> : said ? <small className="ai-ov-now">Siklus terakhir: {said}</small> : null}
          </div>
        </>
        return <li key={m.key} className={`ai-ov-item ${w.state}`}>
          {can(m.screen) ? <Link to={m.to}>{body}<Icon name="chev" className="i ai-ov-arrow" /></Link> : <div>{body}</div>}
        </li>
      })}
    </ol>
  </div>
}
