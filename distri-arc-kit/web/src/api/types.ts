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
  next: NextAction | null
}

export interface NextAction {
  id: string
  kind: string
  title: string
  button: string
  icon: string
  agent: string
  due_label: string
  status: ProposalStatus
  why: string
  decided_at: string | null
  executed_at: string | null
  autonomy: 'auto' | 'approve'
  wait_for?: string
}

export type ProposalStatus = 'proposed' | 'approved' | 'edited' | 'rejected' | 'executed' | 'expired' | 'suppressed'
export interface ProposalOption { key: string; label: string; style: 'primary' | 'ghost' | 'quiet'; result: string; sends: boolean; preview?: string }
export interface ImpactItem { label: string; value: string; tone?: string }
export interface Proposal {
  payload?: Record<string, unknown> | null
  id: string
  agent: string
  dealer_id: string | null
  dealer_slug: string | null
  dealer_name: string | null
  kind: string
  title: string
  summary: string | null
  why: string
  prep: string | null
  preview: string | null
  steps: string[]
  impact: ImpactItem[]
  options: ProposalOption[]
  pills: [string, string][]
  button: string | null
  icon: string | null
  due_label: string | null
  confidence: number
  autonomy: 'auto' | 'approve'
  status: ProposalStatus
  queue: boolean
  decided_at: string | null
  executed_at: string | null
  decision_reason: string | null
  chosen_option: string | null
  decided_by_name: string | null
  created_at: string
  signals?: TimelineEntry[]
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

export interface Tag { k: string; t: string }
export interface ThreadView {
  id: string
  kind: 'dealer' | 'group' | 'new'
  title: string
  subtitle: string
  dealer_id?: string
  dealer_name?: string
  sales: string
  last_message_at: string | null
  unread: number
  last_body: string
  last_from: string
  tag: Tag | null
  group_kind?: string
}
export interface Annotation { k: string; t: string; act?: string }
export interface MessageView {
  id: string
  direction: 'in' | 'out'
  from_name: string
  body: string
  sent_at: string
  status: 'received' | 'pending' | 'sent' | 'failed'
  internal: boolean
  annotation: Annotation | null
  signal_id: string | null
  proposal?: { id: string; button: string; status: ProposalStatus; executed_at: string | null; decided_at: string | null }
}
export interface ThreadDetail {
  thread: { id: string; kind: ThreadView['kind']; title: string; subtitle: string; dealer_id: string; sales: string; sales_wa: string; suggestions: string[] | null; tag: Tag | null; unread: number }
  messages: MessageView[]
}
export interface Identification {
  best_name: string
  best_org: string
  score: number
  sources: { source: string; ok: string; value: string }[]
  potential?: string
}
export interface ChatContext {
  kind: ThreadView['kind']
  dealer?: BoardItem
  extracted: { annotation: Annotation; sent_at: string; from_name: string }[]
  identification: Identification | null
}
export interface WANumber {
  wa_number: string
  masked: string
  sales: string
  branch: string
  transport: string
  state: 'unpaired' | 'pairing' | 'connected' | 'disconnected' | 'logged_out'
  last_seen_at: string | null
  paired_at: string | null
  qr_png?: string
}
export interface InternalNumber { wa_number: string; label: string | null; department: string | null; is_sales: boolean }
export interface WAGroup { id: string; jid: string; name: string | null; kind: 'internal' | 'external'; branch: string | null; members: number | null; read_enabled: boolean }

// ---------- Orchestrator (stage 06) ----------
export type StageName = 'ingest' | 'analyze' | 'synthesize' | 'decide' | 'execute' | 'learn'
export interface StageDetail { text?: string; signals?: number; wa?: number; so?: number; payments?: number; branches?: number; auto?: number; decisions?: number; conflicts?: number; [k: string]: unknown }
export interface CycleStage { cycle_id: string; stage: StageName; status: string; started_at: string | null; finished_at: string | null; detail: StageDetail | null }
export interface Cycle {
  id: string
  number: number | null
  trigger: 'schedule' | 'manual' | 'mcp'
  scope: string
  via: 'api' | 'mcp' | null
  requested_by: string | null
  status: 'queued' | 'running' | 'done' | 'partial' | 'failed'
  started_at: string
  finished_at: string | null
  duration_ms: number | null
  signals_count: number | null
  auto_count: number | null
  decision_count: number | null
  conflict_count: number | null
  note: string | null
  stage: StageName | null
  stages: CycleStage[]
  label: string
}
export interface CycleLatest { cycle: Cycle | null; last_done: Cycle | null; last_full: Cycle | null; running: boolean; next_at: string }
export interface Conflict {
  id: string
  cycle_id: string
  dealer_id: string | null
  agent_a: string
  agent_b: string
  title: string
  resolution: string
  rule: string
  tone: string | null
  visible: boolean
  dealer_name: string | null
  dealer_slug: string | null
}
export interface AgentInfo {
  name: string
  role: string
  icon: string
  auto: string
  approve: string
  never: string
  output: string
  confidence: number | null
  last_run_at: string | null
  today: number
}
export interface PlanProposal { id: string; kind: string; status: ProposalStatus; button: string; icon: string; autonomy: 'auto' | 'approve' }
export interface PlanStep {
  id: string
  plan_date: string
  seq: number
  time_label: string | null
  agent: string | null
  autonomy: 'auto' | 'approve' | null
  text_html: string | null
  status: 'scheduled' | 'running' | 'done' | 'waiting' | 'skipped'
  link: string | null
  wait_for: string | null
  proposals: PlanProposal[]
}
export interface Plan { items: PlanStep[]; total: number; auto: number; approve: number; cycle_number: number | null }
export interface AutonomyRow { auto: string[]; approve: string[]; never: string[]; labels: Record<string, string> }
export interface AutonomyPolicy { matrix: Record<string, AutonomyRow>; guard: { min_confidence: number; dealer_messages: 'confirm' | 'auto' }; order: string[] }

// ---------- MCP (stage 07) ----------
export interface MCPTool { name: string; scope: string; description: string }
export interface MCPInfo { endpoint: string; enabled: boolean; tools: MCPTool[]; llm: { mode: 'api' | 'mcp' | 'both'; provider: string; model: string; api_key: boolean; fallback: string } }
export interface MCPClient { id: string; name: string; scopes: string[]; token_prefix: string; active: boolean; last_seen_at: string | null; created_at: string; calls_today: number }
export interface MCPCall { id: string; client_id: string | null; client_name: string | null; tool: string; args: Record<string, unknown> | null; result_summary: string | null; status: string; duration_ms: number | null; created_at: string }
export interface MCPPolicy { allow_reanalyze: boolean; allow_plan_update_proposal: boolean; allow_send: false; mask_pii_in_read: boolean; max_cycles_per_hour: number }
