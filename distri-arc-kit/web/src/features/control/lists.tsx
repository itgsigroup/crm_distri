import { useNavigate } from 'react-router'
import type { AgingItem, BoardItem, Brief, NextAction } from '../../api/types'
import { useStockProposals } from '../../app/queries'
import { ActBtn } from '../../components/actions'
import { Icon } from '../../components/Icon'
import { Pill } from '../../components/ui'
import { fmtRp, shortName } from '../../lib/format'
import { ROOT_CAUSE, ROOT_SHORT } from '../../lib/i18n/id'

export const creditTone = (s: string) => (s === 'aman' ? 'good' : s === 'tipis' ? 'warn' : s === 'cash' ? 'neutral' : 'bad')
const isBad = (d: BoardItem) => creditTone(d.metrics.credit.state) === 'bad'

export function useGoDealer() {
  const nav = useNavigate()
  return (id: string) => nav('/dealer/' + id)
}

function Ev({ id, children }: { id: string; children: React.ReactNode }) {
  const go = useGoDealer()
  return <button className="ev" onClick={() => go(id)}>{children}</button>
}

export const dueLabel = (n: number) => (n === 0 ? 'hari ini' : n === 1 ? 'besok' : `${n} hr`)

/** "Jadwal order · 7 hari" (mockup renderDue). */
export function DueList({ items }: { items: BoardItem[] }) {
  return (
    <ul className="row-list">
      {items.map((d) => {
        const due = d.metrics.due_in ?? 0
        const basket = d.composition.slice(0, 3).map((c) => c.product).join(' · ')
        return (
          <li key={d.id}>
            <span className="who" style={{ width: 'auto' }}><Pill tone={due <= 1 ? 'good' : 'neutral'}>{dueLabel(due)}</Pill></span>
            <div>
              <div className="t"><Ev id={d.id}>{d.name}</Ev> · siklus order {d.metrics.rhythm_days} hr · {fmtRp(d.metrics.avg_order)}/order</div>
              <div className="s">Rekomendasi order: {basket}{isBad(d) && <> · <b style={{ color: 'var(--bad)' }}>over limit / overdue — tagih dulu</b></>}</div>
            </div>
            {d.next ? <ActBtn small next={d.next} /> : <span className="pill neutral st">rutin</span>}
          </li>
        )
      })}
    </ul>
  )
}

/** "Lewat jadwal" (mockup renderDrift). */
export function DriftList({ items }: { items: BoardItem[] }) {
  return (
    <ul className="row-list">
      {items.map((d) => {
        const churn = d.metrics.status === 'Churn'
        return (
          <li key={d.id}>
            <span className="who" style={{ width: 'auto' }}><Pill tone={churn ? 'bad' : 'warn'}>{d.metrics.last_order_days} hr</Pill></span>
            <div>
              <div className="t"><Ev id={d.id}>{d.name}</Ev> · siklus order {d.metrics.rhythm_days} · {fmtRp(d.metrics.avg_order)}/order · <b>{d.metrics.status}</b></div>
              <div className="s">{d.root_cause ? ROOT_CAUSE[d.root_cause] : ''}</div>
            </div>
            {d.next && !churn ? <ActBtn small next={d.next} /> : <span className="pill neutral st">biarkan</span>}
          </li>
        )
      })}
    </ul>
  )
}

/** "Push stok" (mockup renderPush). */
export function PushList({ items }: { items: AgingItem[] }) {
  const { data: props = [] } = useStockProposals()
  const bundleOf = (name: string): NextAction | null => {
    const p = props.find((x) => x.kind === 'push_stock' && x.payload?.name === name && !x.payload?.parent)
    return p ? { id: p.id, kind: p.kind, title: p.title, button: p.button ?? 'Buat bundle', icon: p.icon ?? 'box', agent: p.agent, due_label: p.due_label ?? '', status: p.status, why: p.why, decided_at: p.decided_at, executed_at: p.executed_at, autonomy: p.autonomy } : null
  }
  return (
    <ul className="row-list">
      {items.slice(0, 3).map((x) => {
        const c = x.candidates ?? []
        const unit = x.category === 'Kamera & NVR' && !/kamera/i.test(x.name) ? 'unit' : /modul|detektor/i.test(x.name) ? 'pcs' : 'unit'
        const names = c.slice(0, 3).map((k) => shortName(k.name)).join(', ')
        return (
          <li key={x.id}>
            <div>
              <div className="t"><b>{x.name}</b> · {x.qty} {unit} · {x.age_days} hari · {fmtRp(x.value)}</div>
              <div className="s">→ {c.length} dealer {c.length ? `(${names}${c.length > 3 ? ', +' + (c.length - 3) : ''})` : 'belum ada yang cocok'}{x.due_this_week ? ` · ${x.due_this_week} jadwal order minggu ini` : ''}</div>
            </div>
            {c.length > 0 && <ActBtn small next={bundleOf(x.name)} label="Buat bundle" icon="box" />}
          </li>
        )
      })}
    </ul>
  )
}

/** "Limit tipis" (mockup renderBreath). */
export function TightList({ items }: { items: BoardItem[] }) {
  return (
    <ul className="row-list">
      {items.map((d) => {
        const due = d.metrics.due_in
        const soon = d.metrics.rhythm_days != null && due != null && due >= 0 && due <= 7
        return (
          <li key={d.id}>
            <span className="who" style={{ width: 'auto' }}><Pill tone={creditTone(d.metrics.credit.state)}>{d.metrics.credit.state}</Pill></span>
            <div>
              <div className="t"><Ev id={d.id}>{d.name}</Ev> · {fmtRp(d.metrics.credit.exposure)} / {fmtRp(d.credit_limit)}</div>
              <div className="s">{soon ? `jadwal order ${due} hr lagi — order akan tertahan` : `pola bayar ${d.metrics.credit.pay_days} hari`}</div>
            </div>
            <ActBtn small next={d.next} />
          </li>
        )
      })}
    </ul>
  )
}

/** "Ringkasan Orchestrator" points (mockup renderBrief), written from the structured brief. */
export function BriefPoints({ brief }: { brief: Brief }) {
  const [p1, p2, p3, p4] = brief.points
  const list = (xs: React.ReactNode[]) => xs.flatMap((x, i) => (i === 0 ? [x] : i === xs.length - 1 ? [' dan ', x] : [', ', x]))
  const drifting = p2.dealers.filter((d) => d.status !== 'Churn')
  const churned = p2.dealers.filter((d) => d.status === 'Churn')
  const recoverable = drifting.filter((d) => d.root_cause && d.root_cause !== 'small_share')
  const bad = p3.dealers.filter((d) => d.credit_state === 'over limit' || d.credit_state === 'overdue')
  const soon = p3.dealers.filter((d) => d.credit_state === 'tipis')
  return (
    <ol>
      <li>
        <span className="k good"><Icon name="check" /></span>
        <div>
          <b>Order tepat jadwal</b>: {p1.count} dealer jadwal order minggu ini
          {p1.dealers.length > 0 && <> — {list(p1.dealers.map((d) => <Ev key={d.id} id={d.id}>{d.short_name}</Ev>))} besok</>}; rekomendasi order disiapkan AI Follow-up. Order 7 hari terakhir {fmtRp(p1.amount ?? 0)} dari {p1.order_dealers ?? 0} dealer.
        </div>
      </li>
      <li>
        <span className="k warn"><Icon name="refresh" /></span>
        <div>
          <b>Lewat jadwal</b>: {list(drifting.map((d, i) => <span key={d.id}><Ev id={d.id}>{d.name}</Ev> ({d.last_order_days}{i === 0 ? ' hari / siklus order ' : ' / '}{d.rhythm_days})</span>))} mulai menjauh
          {churned.length > 0 && <>; {list(churned.map((d) => <Ev key={d.id} id={d.id}>{d.short_name}</Ev>))} sudah churn</>}. Potensi {fmtRp(p2.amount ?? 0)}/bulan.
          {recoverable.length > 0 && <> Yang bisa ditarik kembali: {list(recoverable.map((d) => <span key={d.id}>{d.short_name} (akar: {ROOT_SHORT[d.root_cause!]})</span>))}.</>}
        </div>
      </li>
      <li>
        <span className="k bad"><Icon name="shield" /></span>
        <div>
          <b>Over limit / overdue</b>:{' '}
          {list(bad.map((d) => <span key={d.id}><Ev id={d.id}>{d.short_name}</Ev> exposure {d.exposure_pct}% limit{d.late_days ? <> dan {d.late_invoice} lewat {d.late_days} hari</> : null}</span>))}
          {bad.length > 0 && ' — order berikutnya tertahan sampai pembayaran masuk.'}
          {soon.map((d) => (
            <span key={d.id}> <Ev id={d.id}>{d.short_name}</Ev> jadwal order {d.due_in} hari lagi tapi sisa limit {d.room_pct}% — tagih {d.next_invoice || 'invoice terbuka'} dulu agar ordernya tidak tertahan.</span>
          ))}
        </div>
      </li>
      <li>
        <span className="k accent"><Icon name="box" /></span>
        <div>
          <b>Push stok</b>:{' '}
          {p4.item ? (
            <>{p4.item.name} ({p4.item.qty} pcs, {p4.item.age_days} hari, {fmtRp(p4.item.value)}) cocok untuk {p4.count} dealer yang product mix-nya {p4.item.category} — {p4.item.due_this_week} di antaranya jadwal order minggu ini. Harga bundle tidak pernah di bawah floor margin 9%.</>
          ) : (
            'tidak ada stok di atas 90 hari.'
          )}
        </div>
      </li>
    </ol>
  )
}
