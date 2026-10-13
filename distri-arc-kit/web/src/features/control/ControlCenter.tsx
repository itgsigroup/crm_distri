import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { Icon } from '../../components/Icon'
import { Pill } from '../../components/ui'
import { greeting, hhmm, shortDate } from '../../lib/format'
import { useMe, useNow, useQueue, useWAAnalysis } from '../../app/queries'
import { useOrch, useOrchStatus } from '../../app/orch'
import { Queue } from './Queue'
import { SalesBanner, SalesPicker, useSalesPage } from '../../components/SalesPicker'
import { useCanWA } from '../wa/WAAnalysis'
import { WAAgenda, WAChatList, waWaiting } from '../wa/WACards'
import { waitLabel } from '../wa/wa'

type Tab = 'decide' | 'reply' | 'sales'

// Pusat kendali, ringkas: one header line, one row of numbers, Claude's summary, and ONE card with tabs — every list is
// one click away instead of a column of cards. Analysis only from Claude through MCP (ADR 0032); the rest are counts
// measured from the chats.
export function ControlCenter() {
  const nav = useNavigate()
  const [params, setParams] = useSearchParams()
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
  const busySales = (wa?.sales ?? []).filter((s) => s.waiting > 0).length
  const first = me?.name.split(' ')[0] ?? ''
  const threads = t?.threads ?? 0
  const answered = threads ? Math.round(((threads - (t?.waiting ?? 0)) / threads) * 100) : null
  const med = t?.reply_median_min ?? null
  const due2 = wa?.reply_due_min ?? 120

  const tab: Tab = (['decide', 'reply', 'sales'] as const).find((k) => k === params.get('tab')) ?? (pending ? 'decide' : 'reply')
  const open = (k: Tab) => {
    const next = new URLSearchParams(params)
    next.set('tab', k)
    setParams(next, { replace: true })
    document.getElementById('cc-main')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }

  const tiles: { k: string; label: string; v: string; sub: string; tone: string; go?: () => void }[] = [
    { k: 'decide', label: 'Keputusan', v: fmt(pending), sub: 'usulan Claude menunggu', tone: pending ? 'accent' : 'n', go: () => open('decide') },
    { k: 'late', label: 'Menunggu', v: fmt(t?.needs_reply), sub: `lebih dari ${due}`, tone: t?.needs_reply ? 'bad' : 'good', go: () => open('reply') },
    { k: 'wait', label: 'Belum dibalas', v: fmt(t?.waiting), sub: `${busySales} sales`, tone: t?.waiting ? 'warn' : 'good', go: () => open('reply') },
    { k: 'ans', label: 'Chat terbalas', v: answered == null ? '—' : answered + '%', sub: 'target ≥ 90%', tone: answered == null ? 'n' : answered >= 90 ? 'good' : 'warn', go: () => open('sales') },
    { k: 'med', label: 'Waktu balas', v: med == null ? '—' : waitLabel(med), sub: `median · target ≤ ${due}`, tone: med == null ? 'n' : med <= due2 ? 'good' : 'warn', go: () => open('sales') },
    { k: 'new', label: 'Nomor baru', v: fmt(t?.new_numbers), sub: `${fmt(threads)} chat · ${fmt(t?.inbound)} pesan masuk`, tone: 'n', go: () => nav('/chat') },
  ]

  return (
    <>
      <SalesPicker page={page} />
      <SalesBanner page={page} />
      <div className="card cc-top">
        <div className="cc-greet">
          <h2>{greeting(now)}, {first}.</h2>
          <LastUpdate />
        </div>
        <div className="cc-acts">
          {orch.running ? <Pill tone="accent" icon="refresh">Menganalisis · {orch.stage}</Pill> : <Pill tone="good" icon="check">Siap</Pill>}
          <button className="btn primary" disabled={orch.running} onClick={() => reanalyze('all', 'mcp')} title="Claude menganalisis lewat MCP"><Icon name="plug" />Analisis ulang</button>
          <button className="btn ghost" onClick={() => nav('/chat')}><Icon name="chat" />Chat</button>
          <button className="btn ghost" onClick={() => nav(page.link('/orchestrator'))}>Orchestrator</button>
        </div>
      </div>

      {!canWA ? (
        <div className="card"><p className="sg-hint">Analisa WhatsApp mengutip pesan pelanggan, jadi hanya tampil untuk pengguna dengan akses halaman Chat.</p></div>
      ) : (
        <>
          <div className="cc-tiles">
            {tiles.map((x) => (
              <button key={x.k} type="button" className={`cc-tile ${x.tone}`} onClick={x.go}>
                <small>{x.label}</small><b className="num">{x.v}</b><span>{x.sub}</span>
              </button>
            ))}
          </div>

          <div className="card cc-brief">
            <span className="ai cc-brief-h">Ringkasan Claude (MCP){wa?.generated_at ? ` · ${shortDate(wa.generated_at)} ${hhmm(wa.generated_at)}` : ''}</span>
            {wa?.overview && !scoped
              ? <p>{wa.overview}</p>
              : <p className="muted">{scoped ? 'Ringkasan Claude mencakup semua sales — di halaman sales ini hanya angkanya.' : !wa?.cycle_number ? 'WhatsApp belum dibaca — ringkasan muncul setelah siklus pertama.' : 'Belum ada ringkasan dari Claude pada siklus ini. Tekan Analisis ulang, atau minta di Claude "jalankan analisis GSI Orbit".'}</p>}
          </div>

          <div className="card cc-main" id="cc-main">
            <div className="cc-tabs" role="tablist">
              {([['decide', 'Keputusan', pending], ['reply', 'Menunggu balasan', waiting.length], ['sales', 'Per sales', busySales]] as const).map(([k, l, n]) => (
                <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'is-active' : ''} onClick={() => open(k)}>{l}<span>{n}</span></button>
              ))}
              <span className="cc-tab-meta">{tab === 'decide' ? 'usulan Claude (MCP) dari chat — setujui di sini' : tab === 'reply' ? `merah = lewat ${due} · baris bertanda AI ditulis Claude` : 'chat yang menunggu balasan, per sales'}</span>
            </div>
            <div className="cc-pane">
              {tab === 'decide' && <Queue items={waQueue} />}
              {tab === 'reply' && <WAChatList items={waiting} empty="Semua chat sudah dibalas." limit={10} />}
              {tab === 'sales' && <WAAgenda data={wa} />}
            </div>
          </div>
        </>
      )}
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
  if (!c) return <div className="oc-last"><Icon name="cal" />Analisis terakhir: <b>belum ada</b> · berikutnya {orch.next}</div>
  const at = c.finished_at ?? c.started_at
  const mins = Math.round((now - Date.parse(at)) / 60_000)
  const ago = mins < 1 ? 'baru saja' : mins < 60 ? `${mins} menit lalu` : mins < 24 * 60 ? `${Math.floor(mins / 60)} jam lalu` : ''
  const failed = c.status === 'failed'
  return (
    <div className={`oc-last${failed ? ' bad' : ''}`}>
      <Icon name={failed ? 'alert' : 'check'} />
      Analisis terakhir: <b>{shortDate(at)} · {hhmm(at)} WIB</b>
      {ago && <span> ({ago})</span>}
      <span> · siklus #{(c.number ?? 0).toLocaleString('id-ID')} · {failed ? 'gagal' : c.status === 'partial' ? 'sebagian' : 'berhasil'}</span>
      <span> · berikutnya {orch.next}</span>
    </div>
  )
}
