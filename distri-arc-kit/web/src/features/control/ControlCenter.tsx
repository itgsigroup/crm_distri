import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Icon } from '../../components/Icon'
import { CardH, Pill } from '../../components/ui'
import { greeting, hhmm, shortDate } from '../../lib/format'
import { PipeChips } from '../../app/Dock'
import { useMe, useNow, useQueue, useWAAnalysis } from '../../app/queries'
import { useOrch, useOrchStatus } from '../../app/orch'
import { Queue } from './Queue'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { WALastRead, useCanWA } from '../wa/WAAnalysis'
import { WAAgenda, WABrief, WAChatList, WAWheels, waWaiting } from '../wa/WACards'
import { waitLabel } from '../wa/wa'

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// Pusat kendali: the original layout from WhatsApp (ADR 0026). Analysis only from Claude through MCP (ADR 0032): the
// WhatsApp overview, the line per conversation and the proposals; everything else is a count measured from the chats.
export function ControlCenter() {
  const nav = useNavigate()
  const { data: me } = useMe()
  const now = useNow()
  const page = useSalesPage()
  const sales = page.sales
  const orch = useOrchStatus()
  const { reanalyze } = useOrch()
  const canWA = useCanWA()
  const { data: wa } = useWAAnalysis(sales, canWA)
  const { data: queue = [] } = useQueue(sales)
  const scoped = !!sales || page.locked

  const t = wa?.totals
  const due = waitLabel(wa?.reply_due_min ?? 120)
  // the proposals tied to a conversation (written by Claude through MCP)
  const waIds = new Set((wa?.items ?? []).flatMap((r) => r.proposal_ids))
  const waQueue = queue.filter((q) => waIds.has(q.id))
  const pending = waQueue.filter((q) => q.status === 'proposed').length
  const waiting = waWaiting(wa)
  const first = me?.name.split(' ')[0] ?? ''

  return (
    <>
      <SalesPicker page={page} />
      <SalesBanner page={page} />
      <div className="cc-hero">
        <div className="cc-left">
          <div className="greet">
            <h2>{greeting(now)}, {first}.</h2>
            <p>
              {!canWA ? (
                <>Analisa WhatsApp mengutip pesan pelanggan, jadi hanya tampil untuk pengguna dengan akses halaman Chat.</>
              ) : wa?.cycle_number ? (
                <>Orchestrator sudah membaca {fmt(t?.threads)} percakapan WhatsApp ({t?.dealers ?? 0} dealer, {t?.new_numbers ?? 0} nomor baru) dengan {fmt(t?.inbound)} pesan masuk. {t?.needs_reply ?? 0} chat menunggu lebih dari {due}, {pending} menunggu keputusan{sales ? '' : ' Anda'}.</>
              ) : (
                <>Orchestrator belum membaca WhatsApp — analisa chat muncul setelah siklus penuh pertama.</>
              )}
            </p>
          </div>
          <div className="status-strip">
            <button onClick={() => scrollTo('queue-card')}><span className="n" style={{ background: 'var(--accent)' }}>{pending}</span>Keputusan</button>
            <button onClick={() => scrollTo('reply-card')}><span className="n" style={{ background: 'var(--bad)' }}>{t?.needs_reply ?? 0}</span>Chat menunggu<span className="m">· &gt; {due}</span></button>
            <button onClick={() => scrollTo('reply-card')}><span className="n" style={{ background: 'var(--warn)' }}>{t?.waiting ?? 0}</span>Belum dibalas</button>
            <button onClick={() => nav('/chat')}><span className="n" style={{ background: 'var(--indigo)' }}>{t?.new_numbers ?? 0}</span>Nomor baru</button>
            <button onClick={() => nav('/chat')}><span className="n" style={{ background: 'var(--indigo)' }}><Icon name="chat" /></span>Buka Chat</button>
          </div>
        </div>
        <div className="orch-card">
          <div className="oc-h">
            <span className="ai">Orchestrator</span>
            {orch.running ? <Pill tone="accent" icon="refresh">Menganalisis</Pill> : <Pill tone="good" icon="check">Siap</Pill>}
            <span className="meta">{orch.run ? `siklus #${orch.run.toLocaleString('id-ID')} · ${orch.last} · ${orch.dur}` : 'belum ada siklus · tiap jam 06.00–20.00'}</span>
          </div>
          <div className="oc-pipe"><PipeChips /></div>
          <LastUpdate />
          <WALastRead sales={sales} compact />
          <div className="oc-foot">
            <div className="oc-nums"><span><b>{fmt(t?.threads)}</b> chat</span><span><b>{fmt(t?.inbound)}</b> pesan masuk</span><span><b>{t?.needs_reply ?? 0}</b> menunggu</span><span><b>{pending}</b> keputusan</span></div>
            <div className="oc-btns">
              <button className="btn primary" disabled={orch.running} onClick={() => reanalyze('all', 'mcp')} title="Claude menganalisis lewat MCP"><Icon name="plug" />Analisis ulang</button>
              <button className="btn ghost" onClick={() => nav(page.link('/orchestrator'))}>Buka Orchestrator</button>
            </div>
          </div>
        </div>
      </div>

      {canWA && <div className="today">
        <div className="stack">
          <div className="card" id="queue-card">
            <div className="card-h"><h2>Keputusan</h2><span className="ai" style={{ marginLeft: 6 }}>Claude · MCP</span><span className="meta">{pending ? `${pending} usulan Claude dari chat WhatsApp` : 'Tidak ada usulan Claude dari chat yang menunggu'}</span></div>
            <Queue items={waQueue} />
          </div>

          <div className="card brief">
            <div className="card-h"> <span className="ai">Ringkasan Claude (MCP) · {wa?.generated_at ? `${shortDate(wa.generated_at)} · ${hhmm(wa.generated_at)}` : '—'}</span><span className="meta">{wa?.cycle_number ? `${wa.window_days} hari terakhir · siklus #${wa.cycle_number.toLocaleString('id-ID')}` : ''}</span></div>
            <WABrief data={wa} scoped={scoped} />
          </div>

          <div className="card" id="reply-card">
            <div className="card-h"><h2>Menunggu balasan</h2><span className="meta">merah = lewat {due} · baris bertanda AI ditulis Claude (MCP)</span></div>
            <WAChatList items={waiting} empty="Semua chat sudah dibalas." />
          </div>
        </div>

        <div className="stack">
          <div className="card">
            <CardH title="KPI WhatsApp" meta="Balas cepat, order tidak hilang" />
            <WAWheels data={wa} />
          </div>
          <div className="card">
            <CardH title="Agenda sales" meta="chat yang menunggu balasan · per sales" />
            <WAAgenda data={wa} />
          </div>
        </div>
      </div>}
    </>
  )
}

const fmt = (v: number | undefined) => (v ?? 0).toLocaleString('id-ID')

/** "Update terakhir analisis": when the last cycle finished, its number, and how long ago (refreshed every minute). */
function LastUpdate() {
  const orch = useOrchStatus()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 60_000)
    return () => clearInterval(t)
  }, [])
  const c = orch.lastDone
  if (orch.running) return <div className="oc-last running"><Icon name="refresh" />Analisis sedang berjalan · {orch.stage}…</div>
  if (!c) return <div className="oc-last"><Icon name="cal" />Update terakhir analisis: <b>belum ada</b> · berikutnya {orch.next}</div>
  const at = c.finished_at ?? c.started_at
  const mins = Math.round((now - Date.parse(at)) / 60_000)
  const ago = mins < 1 ? 'baru saja' : mins < 60 ? `${mins} menit lalu` : mins < 24 * 60 ? `${Math.floor(mins / 60)} jam lalu` : ''
  const failed = c.status === 'failed'
  return (
    <div className={`oc-last${failed ? ' bad' : ''}`}>
      <Icon name={failed ? 'alert' : 'check'} />
      Update terakhir analisis: <b>{shortDate(at)} · {hhmm(at)} WIB</b>
      {ago && <span> ({ago})</span>}
      <span> · siklus #{(c.number ?? 0).toLocaleString('id-ID')} · {failed ? 'gagal' : c.status === 'partial' ? 'sebagian' : 'berhasil'}</span>
      <span> · berikutnya {orch.next}</span>
    </div>
  )
}
