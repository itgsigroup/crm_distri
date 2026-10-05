// Types mirror the JSON of the Go API (internal/views, internal/domain). Money in rupiah, percentages 0–100.

export type CreditState = 'aman' | 'tipis' | 'over limit' | 'overdue' | 'cash'
export type Status = 'Key account' | 'Aktif' | 'At risk' | 'Churn' | 'Baru'
export type Segment = 'A' | 'B' | 'C' | 'D' | 'Baru'

export interface Credit {
  limit: number
  exposure: number
  room: number | null
  state: CreditState
  late: boolean
  late_days: number
  pay_days: number
  on_time: number
}

export interface ScoreParts { r: number; p: number; k: number; n: number; i: number }

export interface DealerMetrics {
  as_of: string
  rhythm_days: number | null
  last_order_days: number | null
  cyc: number
  due_in: number | null
  status: Status
  activity: 'normal' | 'menurun' | 'berhenti' | 'baru'
  freq: number | null
  avg_order: number
  omzet_bln: number
  segment: Segment
  sow: number
  sow_source: 'confirmed' | 'estimated' | 'default'
  mix: number
  mix_cats: boolean[]
  credit: Credit
  pic_active: number
  score: number
  score_parts: ScoreParts
  orders_6m: number
  cycle_days: number
}

export interface Owner { key: string; name: string; initials: string; branch: string }
export interface ProductShare { product: string; category: string; pct: number; value: number }
export interface Prev { rhythm_days: number | null; avg_order: number; freq: number | null; segment: Segment }

export interface BoardItem {
  id: string
  uuid: string
  name: string
  short_name: string
  city: string
  branch: string
  tier: string
  segment_desc: string
  owner: Owner
  credit_limit: number
  metrics: DealerMetrics
  composition: ProductShare[]
  prev: Prev | null
  root_cause?: RootCause
}

export type RootCause = 'project_unpaid' | 'marketplace_module' | 'marketplace' | 'wholesaler' | 'small_share'

export interface ContactView {
  name: string
  role: string
  level: 'utama' | 'aktif' | 'jarang' | 'belum'
  active: boolean
  is_primary: boolean
  interactions_90d: number
}

export interface OrderLine { product: string; category: string; qty: number; price: number; subtotal: number }
export interface OrderView {
  number: string
  confirmed_at: string | null
  total: number
  phase: number
  phase_label: string
  paid_at: string | null
  lines: OrderLine[]
}
export interface MonthTotal { month: string; label: string; total: number }
export interface Orders { orders: OrderView[] | null; months: MonthTotal[]; last: OrderView | null; cycle_days: number }

export interface OpenInvoice { number: string; issued_at: string; due_at: string; total: number; residual: number; late_days: number }
export interface Commitment { title: string; detail: string; status: 'open' | 'done' | 'late'; due_at: string | null; late_days: number; invoice?: string }
export interface TimelineEntry { at: string; kind: string; via: string; who: string; text: string; conclusion: string; signal_id: string }

export interface DealerDetail extends BoardItem {
  memo: string
  memo_updated_at: string | null
  memo_signals: TimelineEntry[] | null
  contacts: ContactView[]
  commitments: { kami: Commitment[]; mereka: Commitment[] }
  orders: Orders
  open_invoices: OpenInvoice[] | null
  timeline: TimelineEntry[]
  flags: string[]
}

export interface Sales { key: string; name: string; branch: string; initials: string; wa_number: string; dealers: number }

export interface StatusSummary { status: Status; count: number; omzet_bln: number }
export interface SegmentSummary { segment: Segment; count: number; omzet_bln: number; pct: number }

export interface Mover {
  kind: 'moving_out' | 'approaching' | 'thin_mix' | 'moved' | 'slowing' | 'stronger'
  dealer_id: string
  name: string
  last_order_days?: number
  rhythm_days?: number
  due_in?: number
  credit_state?: CreditState
  mix?: number
  from?: Segment
  to?: Segment
  up?: boolean
  prev_rhythm_days?: number
  prev_avg_order?: number
  avg_order?: number
  sow?: number
  root_cause?: RootCause
}

export interface KPI {
  on_schedule_pct: number
  dso_days: number
  stock_turn_days: number
  targets: { on_schedule_pct: number; dso_days: number; stock_turn_days: number }
  drift_count: number
  overdue_amount: number
  tight_count: number
  stock_value: number
}

export interface AgendaItem { kind: 'due' | 'drift' | 'collect'; dealer_id: string; short_name: string; due_in?: number; late_days?: number }
export interface AgendaRow { sales: Owner; dealers: number; count: number; items: AgendaItem[] | null }

export interface StockItem {
  id: string
  branch: string
  sku: string
  name: string
  category: string
  qty: number
  unit_cost: number
  value: number
  age_days: number
  weekly_velocity: number
}
export interface PushCandidate { dealer_id: string; name: string; reason: string; due_in: number | null; drifting: boolean; omzet_bln: number }
export interface AgingItem extends StockItem { candidates: PushCandidate[] | null; due_this_week: number }

export interface BriefDealer {
  id: string
  name: string
  short_name: string
  due_in?: number | null
  last_order_days?: number | null
  rhythm_days?: number | null
  status?: Status
  credit_state?: CreditState
  exposure_pct?: number
  late_days?: number
  late_invoice?: string
  room_pct?: number
  next_invoice?: string
  root_cause?: RootCause
}

export interface BriefPoint {
  kind: 'on_schedule' | 'drift' | 'credit' | 'push'
  tone: 'good' | 'warn' | 'bad' | 'accent'
  dealers: BriefDealer[]
  count: number
  amount?: number
  orders?: number
  order_dealers?: number
  item?: AgingItem
  signal_ids: string[]
}
export interface Brief {
  generated_at: string
  source: 'template' | 'llm'
  points: BriefPoint[]
  counts: { wa: number; so: number; payments: number; branches: number }
  confidence: number
}

export interface Me { id: string; email: string; name: string; role: string; branch: string }
export interface Health { db: string; queue: string; now: string; sample_data?: boolean }
