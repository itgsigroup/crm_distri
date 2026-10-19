import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Icon } from '../../components/Icon'
import { CardH, Pill } from '../../components/ui'
import { greeting, hhmm, shortDate } from '../../lib/format'
import { PipeChips } from '../../app/Dock'
import { useMe, useNow, usePlan, useQueue, useStockPush, useWAAnalysis } from '../../app/queries'
import { useOrch, useOrchStatus } from '../../app/orch'
import { PlanList } from './Plan'
import { PushList } from './lists'
import { Queue } from './Queue'
import { AnalysisPanel } from './AnalysisPanel'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { WALastRead, useCanWA } from '../wa/WAAnalysis'
import { WAAgenda, WABrief, WAChatList, WAClaudeStatus, WAWheels, waIntent, waWaiting } from '../wa/WACards'
import { waitLabel } from '../wa/wa'

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const NO_WA = 'Analisa WhatsApp mengutip pesan pelanggan, jadi hanya tampil untuk pengguna dengan akses halaman Chat.'

// Pusat kendali, the cards of 08-frontend.md in their places (ADR 0045): every analysis read from WhatsApp, Push stok
// from the BigQuery stock.
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
  const { data: plan } = usePlan(sales)
  const { data: queue = [] } = useQueue(sales)
  const { data: push = [] } = useStockPush(sales)
  const scoped = !!sales || page.locked

  const t = wa?.totals
  const due = waitLabel(wa?.reply_due_min ?? 120)
  // only what came from WhatsApp: the proposals tied to a conversation, and the plan steps of those proposals or a chat
  const waIds = new Set((wa?.items ?? []).flatMap((r) => r.proposal_ids))
  const waQueue = queue.filter((q) => waIds.has(q.id))
  const pending = waQueue.filter((q) => q.status === 'proposed').length
  const waPlanItems = (plan?.items ?? []).filter((it) => it.link?.startsWith('chat') || it.proposals.some((p) => waIds.has(p.id)))
  const waPlan = plan && { ...plan, items: waPlanItems, total: waPlanItems.length, auto: waPlanItems.filter((it) => it.autonomy === 'auto').length, approve: waPlanItems.filter((it) => it.autonomy !== 'auto').length }
  const orders = waIntent(wa, 'order')
  const late = waWaiting(wa).filter((r) => r.needs_reply)
  const tight = waIntent(wa, 'release', 'return')
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
                <>{NO_WA}</>
              ) : wa?.cycle_number ? (
                <>Orchestrator sudah membaca {fmt(t?.threads)} percakapan WhatsApp ({t?.dealers ?? 0} dealer, {t?.new_numbers ?? 0} nomor baru) dengan {fmt(t?.inbound)} pesan masuk, dan stok cabang. {t?.needs_reply ?? 0} chat menunggu lebih dari {due}, {pending} menunggu keputusan{sales ? '' : ' Anda'}.</>
              ) : (
                <>Orchestrator belum membaca WhatsApp — analisa chat muncul setelah siklus penuh pertama.</>
              )}
            </p>
          </div>
          <div className="status-strip">
            <button onClick={() => scrollTo('queue-card')}><span className="n" style={{ background: 'var(--accent)' }}>{pending}</span>Keputusan</button>
            <button onClick={() => scrollTo('due-card')}><span className="n" style={{ background: 'var(--good)' }}>{orders.length}</span>Jadwal order<span className="m">· 7 hari</span></button>
            <button onClick={() => scrollTo('drift-card')}><span className="n" style={{ background: 'var(--warn)' }}>{late.length}</span>Lewat jadwal</button>
            <button onClick={() => scrollTo('tight-card')}><span className="n" style={{ background: 'var(--bad)' }}>{tight.length}</span>Limit tipis</button>
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
              <button className="btn primary" disabled={orch.running} onClick={() => reanalyze('all', 'mcp', 'Claude lewat MCP')}><Icon name="refresh" />Analisis ulang</button>
              <button className="btn ghost" onClick={() => nav(page.link('/orchestrator'))}>Buka Orchestrator</button>
            </div>
          </div>
        </div>
      </div>

      {/* while an analysis runs: what the AI reads, a game to wait with, then berhasil / gagal */}
      <AnalysisPanel />

      <div className="today">
        <div className="stack">
          <div className="card" id="plan-card">
            <div className="card-h"><h2>Rencana hari ini</h2><span className="ai" style={{ marginLeft: 6 }}>disusun Orchestrator · dari WhatsApp</span><span className="meta">{waPlan?.total ? `${waPlan.total} langkah · ${waPlan.auto} otonom · ${waPlan.approve} butuh approve` : '0 langkah'}</span></div>
            {canWA ? <PlanList plan={waPlan} /> : <p className="sg-hint">{NO_WA}</p>}
            <div className="plan-foot"><span className="pol"><Icon name="lock" />Langkah berlabel <b>otonom</b> berjalan sendiri dalam batas kebijakan; yang lain menunggu Anda. Tidak ada pesan yang terkirim sendiri.</span><button className="btn ghost" onClick={() => scrollTo('queue-card')}>Ke keputusan</button></div>
          </div>

          <div className="card" id="queue-card">
            <div className="card-h"><h2>Keputusan</h2><span className="ai" style={{ marginLeft: 6 }}>Claude · MCP</span><span className="meta">{pending ? `${pending} item · usulan dari chat WhatsApp` : 'Tidak ada usulan dari chat yang menunggu'}</span></div>
            {canWA ? <Queue items={waQueue} /> : <p className="sg-hint">{NO_WA}</p>}
          </div>

          <div className="card brief">
            <div className="card-h"><span className="ai">Ringkasan Orchestrator · {wa?.generated_at ? `${shortDate(wa.generated_at)} · ${hhmm(wa.generated_at)}` : '—'}</span><span className="meta" title="AI hanya membaca pesan pelanggan baru hari ini; yang sudah dibaca tetap tampil. Angka dihitung dari 7 hari terakhir tanpa AI.">{wa?.cycle_number ? `dari WhatsApp · ${wa.window_days} hari terakhir · siklus #${wa.cycle_number.toLocaleString('id-ID')}` : ''}</span></div>
            {canWA ? <><WAClaudeStatus data={wa} /><WABrief data={wa} scoped={scoped} /></> : <p className="sg-hint">{NO_WA}</p>}
          </div>

          <div className="grid-2">
            <div className="card" id="due-card">
              <div className="card-h"><h2>Jadwal order · 7 hari</h2><span className="ai" style={{ marginLeft: 6 }}>AI Order</span><span className="meta">Dealer yang minta order di chat WhatsApp minggu ini</span></div>
              {canWA ? <WAChatList items={orders} empty="Belum ada permintaan order di chat 7 hari terakhir." /> : <p className="sg-hint">{NO_WA}</p>}
            </div>
            <div className="card" id="drift-card">
              <div className="card-h"><h2>Lewat jadwal</h2><span className="ai" style={{ marginLeft: 6 }}>AI Follow-up</span><span className="meta">Chat lewat {due} belum dibalas — sebelum ordernya hilang</span></div>
              {canWA ? <WAChatList items={late} empty={`Tidak ada chat yang menunggu lebih dari ${due}.`} /> : <p className="sg-hint">{NO_WA}</p>}
            </div>
          </div>
        </div>

        <div className="stack">
          <div className="card">
            <CardH title="KPI utama" meta="dari WhatsApp · balas cepat, order tidak hilang" />
            {canWA ? <WAWheels data={wa} /> : <p className="sg-hint">{NO_WA}</p>}
          </div>
          <div className="card">
            <CardH title="Agenda sales" meta="dibagi Orchestrator · chat menunggu balasan per sales" />
            {canWA ? <WAAgenda data={wa} /> : <p className="sg-hint">{NO_WA}</p>}
          </div>
          <div className="card">
            <CardH title="Push stok" ai="AI Stok" meta="Stok BigQuery → dealer yang product mix-nya cocok" />
            <PushList items={push} />
          </div>
          <div className="card" id="tight-card">
            <CardH title="Limit tipis" ai="AI Kredit" meta="Chat minta rilis kredit atau retur" />
            {canWA ? <WAChatList items={tight} empty="Belum ada permintaan rilis kredit atau retur di chat." limit={5} /> : <p className="sg-hint">{NO_WA}</p>}
          </div>
        </div>
      </div>
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
