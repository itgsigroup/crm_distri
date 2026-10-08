// Orbit reading aids: summary tiles, the filter bar and the dealer list under the board. All three use the
// definitions in filters.ts so a tile, a filter and the list always count the same dealers.
import { useNavigate } from 'react-router'
import type { BoardItem, Segment } from '../../api/types'
import { ActBtn } from '../../components/actions'
import { Icon } from '../../components/Icon'
import { useMore } from '../../components/More'
import { Pill, type Tone } from '../../components/ui'
import { fmtRp } from '../../lib/format'
import { KUAD, SEGMENT_ORDER } from '../../lib/i18n/id'
import { EMPTY, activeCount, type OrbitDigest, type OrbitFilter, type Sort } from './filters'
import { RINGS } from './geometry'

type SetF = (patch: Partial<OrbitFilter>) => void

/** Four tiles that answer "what needs me today"; a click filters the board to exactly those dealers. */
export function OrbitSummary({ g, f, set }: { g: OrbitDigest; f: OrbitFilter; set: SetF }) {
  const tiles: { key: string; on: boolean; patch: Partial<OrbitFilter>; tone: string; icon: string; n: number; t: string; s: string }[] = [
    { key: 'due', on: f.jadwal === 'minggu', patch: { jadwal: f.jadwal === 'minggu' ? '' : 'minggu' }, tone: 'good', icon: 'cal', n: g.due.n, t: 'Jadwal order minggu ini', s: `hubungi H-1 dengan rekomendasi order · ${fmtRp(g.due.omzet)}/bln` },
    { key: 'late', on: f.jadwal === 'lewat', patch: { jadwal: f.jadwal === 'lewat' ? '' : 'lewat' }, tone: 'warn', icon: 'refresh', n: g.late.n, t: 'Lewat jadwal', s: `follow-up sebelum ordernya hilang · ${fmtRp(g.late.omzet)}/bln` },
    { key: 'collect', on: f.limit === 'tagih', patch: { limit: f.limit === 'tagih' ? '' : 'tagih' }, tone: 'bad', icon: 'cash', n: g.collect.n, t: 'Tagih dulu', s: `over limit / overdue · piutang ${fmtRp(g.collect.exposure)}` },
    { key: 'churn', on: f.status === 'Churn', patch: { status: f.status === 'Churn' ? '' : 'Churn' }, tone: 'neutral', icon: 'user-x', n: g.churn.n, t: 'Churn', s: 'sudah lama tidak order · cukup dipantau' },
  ]
  return (
    <div className="os">
      <p className="os-line">
        <b>{g.total.toLocaleString('id-ID')} dealer</b> · omzet <b>{fmtRp(g.omzet)}/bln</b>. Klik kotak untuk menampilkan dealernya saja.
      </p>
      <div className="os-grid">
        {tiles.map((x) => (
          <button key={x.key} type="button" className={`os-tile os-${x.tone}${x.on ? ' is-on' : ''}`} aria-pressed={x.on} onClick={() => set(x.patch)}>
            <span className="os-ic"><Icon name={x.icon} /></span>
            <span className="os-n">{x.n.toLocaleString('id-ID')}</span>
            <span className="os-t">{x.t}</span>
            <span className="os-s">{x.s}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

export function OrbitFilterBar({ f, set, branches, shown, total }: { f: OrbitFilter; set: SetF; branches: string[]; shown: number; total: number }) {
  const n = activeCount(f)
  return (
    <div className="of">
      <div className="search of-q"><Icon name="search" /><input value={f.q} onChange={(e) => set({ q: e.target.value })} type="text" placeholder="Cari dealer, kota, sales…" aria-label="Cari dealer" /></div>
      <select className={`of-s${f.status ? ' on' : ''}`} value={f.status} onChange={(e) => set({ status: e.target.value })} aria-label="Status">
        <option value="">Semua status</option>
        {RINGS.map((r) => <option key={r} value={r}>{r}</option>)}
      </select>
      <select className={`of-s${f.jadwal ? ' on' : ''}`} value={f.jadwal} onChange={(e) => set({ jadwal: e.target.value as OrbitFilter['jadwal'] })} aria-label="Jadwal order">
        <option value="">Semua jadwal</option>
        <option value="minggu">Jadwal order ≤ 7 hari</option>
        <option value="lewat">Lewat jadwal</option>
      </select>
      <select className={`of-s${f.limit ? ' on' : ''}`} value={f.limit} onChange={(e) => set({ limit: e.target.value as OrbitFilter['limit'] })} aria-label="Sisa limit">
        <option value="">Semua sisa limit</option>
        <option value="aman">Limit aman</option>
        <option value="tipis">Limit tipis</option>
        <option value="tagih">Over limit / overdue</option>
        <option value="cash">Cash</option>
      </select>
      <select className={`of-s${f.segmen ? ' on' : ''}`} value={f.segmen} onChange={(e) => set({ segmen: e.target.value as Segment | '' })} aria-label="Segmen">
        <option value="">Semua segmen</option>
        {SEGMENT_ORDER.map((k) => <option key={k} value={k}>{KUAD[k].n} · {KUAD[k].nick}</option>)}
      </select>
      {branches.length > 1 && (
        <select className={`of-s${f.cabang ? ' on' : ''}`} value={f.cabang} onChange={(e) => set({ cabang: e.target.value })} aria-label="Cabang">
          <option value="">Semua cabang</option>
          {branches.map((b) => <option key={b} value={b}>{b}</option>)}
        </select>
      )}
      <span className="of-n">{n ? <><b>{shown.toLocaleString('id-ID')}</b> dari {total.toLocaleString('id-ID')} dealer</> : `${total.toLocaleString('id-ID')} dealer`}</span>
      {n > 0 && <button type="button" className="btn quiet of-x" onClick={() => set(EMPTY)}><Icon name="x" />Hapus filter ({n})</button>}
    </div>
  )
}

const STATUS_TONE: Record<string, Tone> = { 'Key account': 'accent', Aktif: 'good', Baru: 'good', 'At risk': 'warn', Churn: 'neutral' }
const LIMIT: Record<string, [Tone, string]> = { aman: ['good', 'aman'], tipis: ['warn', 'tipis'], 'over limit': ['bad', 'over limit'], overdue: ['bad', 'overdue'], cash: ['neutral', 'cash'] }

/** Jadwal order in words: "3 hari lagi", "lewat 9 hari", "diam 75 hari". */
export function scheduleText(d: BoardItem): [string, Tone] {
  const m = d.metrics
  if (m.status === 'Churn') return [`diam ${m.last_order_days ?? '—'} hari`, 'neutral']
  if (!m.rhythm_days || m.due_in == null) return ['baru · belum ada siklus', 'neutral']
  if (m.due_in < 0) return [`lewat ${-m.due_in} hari`, m.status === 'At risk' ? 'warn' : 'neutral']
  if (m.due_in === 0) return ['hari ini', 'good']
  if (m.due_in === 1) return ['besok', 'good']
  return [`${m.due_in} hari lagi`, m.due_in <= 7 ? 'good' : 'neutral']
}

export function OrbitDealerList({ list, sort, onSort, filtered }: { list: BoardItem[]; sort: Sort; onSort: (s: Sort) => void; filtered: boolean }) {
  const nav = useNavigate()
  const [shown, more] = useMore(list, 15)
  return (
    <div className="card">
      <div className="card-h">
        <h2>Daftar dealer</h2>
        <span className="meta" style={{ marginLeft: 6, marginRight: 'auto' }}>{list.length.toLocaleString('id-ID')} dealer{filtered ? ' sesuai filter' : ''}</span>
        <select className="of-s" value={sort} onChange={(e) => onSort(e.target.value as Sort)} aria-label="Urutkan">
          <option value="omzet">Urutkan: omzet terbesar</option>
          <option value="jadwal">Urutkan: paling mendesak</option>
          <option value="diam">Urutkan: paling lama tidak order</option>
        </select>
      </div>
      {list.length === 0 ? (
        <p className="sg-hint">Tidak ada dealer yang cocok dengan filter ini.</p>
      ) : (
        <div className="odl-wrap">
          <table className="odl">
            <thead><tr><th>Dealer</th><th>Status</th><th>Jadwal order</th><th className="r">Omzet</th><th>Sisa limit</th><th /></tr></thead>
            <tbody>
              {shown.map((d) => {
                const m = d.metrics
                const [sch, schTone] = scheduleText(d)
                const [lt, ll] = LIMIT[m.credit.state] ?? ['neutral', m.credit.state]
                return (
                  <tr key={d.id}>
                    <td className="odl-name">
                      <button className="ev" onClick={() => nav('/dealer/' + d.id)}>{d.name}</button>
                      <small>{d.city} · {d.owner.name} · {KUAD[m.segment].n}</small>
                    </td>
                    <td><Pill tone={STATUS_TONE[m.status] ?? 'neutral'}>{m.status === 'Baru' ? 'Aktif · baru' : m.status}</Pill></td>
                    <td className={`odl-sch t-${schTone}`}>{sch}</td>
                    <td className="odl-rp">{fmtRp(m.omzet_bln)}<small>/bln</small></td>
                    <td><Pill tone={lt}>{ll}</Pill></td>
                    <td className="odl-act">{d.next ? <ActBtn small next={d.next} /> : null}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      {more}
    </div>
  )
}
