// API contract between the Go backend (apps/api) and the React web app.
// Every screen in reference/arc-crm-mockup.html is rendered from these shapes;
// the web app holds no business data of its own.
//
// Conventions:
// - Money is a number in rupiah (float). Format on the client with fmtRp/fmtRp1.
// - `tone` / `k` values are one of Tone and map to CSS classes (pill good, k bad, …).
// - `icon` values are sprite ids from the mockup (e.g. "i-send", "i-mail").
// - `go` values are navigation targets "screen" or "screen:id" (e.g. "rel:rsud", "pros").

export type Tone = 'good' | 'warn' | 'bad' | 'accent' | 'indigo' | 'neutral';
export type ScreenKey = 'today' | 'ask' | 'chat' | 'rel' | 'net' | 'pipe' | 'pros' | 'cash' | 'conn';

export interface Pill { k: Tone; t: string; icon?: string }

// ---------- Auth & shell ----------
// GET /api/me
export interface Me {
  id: string; name: string; role: 'ceo' | 'manager' | 'sales' | 'finance' | 'ops';
  branch: string; initials: string; role_label: string; // "CEO · semua cabang"
}
// POST /api/auth/login {email, password} -> Me (sets cookie arc_session)
export interface LoginRequest { email: string; password: string }

// GET /api/shell
export interface Shell {
  screens: Record<ScreenKey, { title: string; sub: string }>;
  badges: { today: number; chat: number; cash: number; conn: number };
  agents: { active: number; list: { name: string; on: boolean }[] };
  sample_data: boolean; // shows the "Data contoh" chip
}

// ---------- Actions (used everywhere: queue, kanban, account, cash, chat) ----------
export interface ActionOption { key: string; label: string; primary?: boolean }
export interface ImpactItem { label: string; value: string; tone?: Tone }
export interface Action {
  id: string;
  agent: string;            // "Follow-up agent"
  type: string;             // send_wa | send_email | create_invoice | credit_release | ...
  kind: 'send' | 'policy' | 're' | 'task' | 'internal';
  title: string;
  button_label: string;     // primary button text, e.g. "Kirim besok 08.00"
  icon: string;
  account_id?: string;
  account_name: string;
  opportunity_id?: string;
  due_label: string;        // "Hari ini"
  summary: string;          // queue body (qd)
  why: string;              // "Kenapa sekarang"
  prep: string;             // "Yang sudah disiapkan ARC"
  preview: string;          // draft text (may be empty)
  preview_from: string;     // "Kepada: … · Dari: …" / "WhatsApp · dari nomor Andi"
  context_note: string;     // AI note line in the queue (qw)
  impact: ImpactItem[];
  steps: string[];          // "Setelah Anda setujui"
  options: ActionOption[];  // custom decision buttons (policy items); empty for send items
  tags: Pill[];             // pills in the queue header
  confidence: number;
  model: string;
  provenance_line: string;  // "dianalisis dari email, WhatsApp, meeting & Odoo akun ini"
  status: 'proposed' | 'approved' | 'edited' | 'rejected' | 'snoozed' | 'executed' | 'cancelled';
  decided_at_label?: string; // "17:07"
  result_text: string;      // shown after a decision (qres)
  reject_reason?: string;   // "Tidak tepat waktu — …"
  in_queue: boolean;
}
// POST /api/actions/{id}/decision
export interface DecisionRequest {
  decision: 'approve' | 'edit' | 'reject' | 'snooze' | 'option';
  option?: string;          // ActionOption.key when decision = option
  reason?: string;          // required for reject: one of reject_reasons
  note?: string;
  preview?: string;         // edited draft when decision = edit
}
export interface DecisionResponse { action: Action; toast: string }
// GET /api/actions?status=proposed&account=rsud -> Action[]
// GET /api/actions/{id} -> Action
// GET /api/meta/reject-reasons -> string[]

// ---------- Today ----------
export interface StripItem { n: string; tone: Tone | 'muted'; label: string; muted?: string; scroll?: string; go?: string }
export interface BriefPoint { k: Tone; icon: string; html: string } // html may contain <b class="num"> and <button class="ev" data-go="rel:id">
export interface Brief {
  written_label: string;    // "Ditulis ARC · 17:02"
  meta: string;             // "Dari 14 email, 6 WhatsApp, 2 meeting, 1 pembayaran hari ini"
  points: BriefPoint[];
  sources: { icon: string; label: string }[];
  confidence: number;
}
export interface CommitmentRow { who: 'Kami' | 'Mereka'; t: string; s: string; pill: Pill }
export interface SignalRow { id: number; k: Tone; icon: string; title: string; detail: string; go: string }
export interface AgendaItem { time: string; duration: string; title: string; detail: string; pills: Pill[] }
export interface ForecastRow { label: string; value: number; width: number; variant: '' | 'soft' | 'faint' }
export interface Forecast {
  title: string;            // "Forecast Q3"
  meta: string;             // "Berbasis bukti · 3 hari lagi"
  rows: ForecastRow[];      // Commit / Best case / Pipeline
  target: number; target_pos: number; // percent position of the target marker
  note: string;
  commit: number; best: number; pipeline: number;
}
export interface PulseRow { label: string; value: string; delta: string; tone: 'good' | 'bad' | 'n' }
export interface RenewalRow { icon: string; tone: Tone; html: string; sub: string }
export interface Today {
  greeting: { title: string; sub: string };
  strip: StripItem[];
  brief: Brief | null;
  queue: { meta: string; filters: { key: string; label: string }[]; items: Action[] };
  commitments: CommitmentRow[];
  signals: SignalRow[];
  tomorrow: { label: string; items: AgendaItem[] };
  forecast: Forecast;
  pulse: { meta: string; rows: PulseRow[] };
  renewal: { rows: RenewalRow[] };
  agents_line: { summary_html: string; feed: { agent: string; html: string }[] };
}

// ---------- Ask ----------
export interface EvidenceCard {
  account_id: string; name: string; value: number; band: Tone;
  items: { icon: string; text: string }[];
  provs: string[];          // ["health 41 ↓12", "conf 0.94"]
}
export interface ScenarioRow { label: string; width: number; was: boolean; target_pos?: number; value: string; delta?: string }
export interface AskRec { label: string; icon?: string; primary?: boolean; action?: string; toast?: string }
export interface AskAnswer {
  id: string;
  question: string;
  who_label: string;        // "menilai 8 deal terbuka · 17:03"
  paragraphs: string[];     // html allowed (<b class="num">)
  evidence: EvidenceCard[];
  scenario: ScenarioRow[];
  followup: string;         // optional closing paragraph
  recs: AskRec[];
  sources: string[];        // provenance chips, e.g. "sumber: 3 dokumen"
  confidence: number;
  mode: 'graph' | 'llm' | 'fallback';
}
// POST /api/ask {question, screen?} -> AskAnswer
// GET /api/ask/history -> AskAnswer[]   (conversation on the Ask screen)
// GET /api/ask/suggestions?screen=today -> string[]

// ---------- Chat ----------
export type ChatType = 'cust' | 'gext' | 'gint' | 'internal';
export interface ChatListItem {
  id: string; type: ChatType; name: string; sub: string; via: string; time: string;
  unread: number; tag: Pill; last: string; private: boolean; initials: string;
}
export interface ChatMessage {
  id?: number;
  day?: string;             // day separator when set (other fields absent)
  f?: 'in' | 'out';
  who?: string; int?: boolean;
  t?: string; tm?: string; sent?: string;
  ann?: { k: Tone; t: string; act?: string; act_id?: string };
}
export interface ChatThread {
  id: string; type: ChatType; name: string; sub: string; via: string; initials: string;
  via_note: string;         // "pelanggan · dibaca ARC"
  private: boolean;
  private_note?: { title: string; body: string };
  messages: ChatMessage[];
  suggestions: string[];
  policy_line: string;      // "Dikirim dari nomor Andi · Andi diberi tahu · dicatat ke chatter opportunity Odoo"
  context: ChatContext;
}
export interface ChatContext {
  kind: 'customer' | 'group' | 'internal';
  note?: string;            // internal/gint explanation
  internal_members?: { n: string; unit: string }[];
  account?: { id: string; name: string; health: number; line: string; action?: Action };
  project?: { id: string; stage: string; next: string };
  summary?: string[];
  tasks?: { id: string; t: string; who: string; src: string; status: string }[];
  members?: { n: string; r: string; int: boolean }[];
  extracted?: { k: Tone; t: string; tm: string }[];
  open_commitments?: { t: string; s: string; pill: Pill }[];
}
// GET /api/chat/threads?type=all|cust|gext|gint|internal -> {items: ChatListItem[], unread: number}
// GET /api/chat/threads/{id} -> ChatThread  (marks read)
// POST /api/chat/threads/{id}/reply {text} -> {action: Action, toast: string, message: ChatMessage}
// POST /api/chat/tasks/{id}/send -> {action: Action, toast: string}       (Ke Basecamp / tugas)
// POST /api/chat/annotations/{id}/act -> {toast: string}                  (act button under a message)

// ---------- Relasi ----------
export interface AccountListItem {
  id: string; name: string; opp: string; last: string; owner: string; value: number; health: number; band: Tone;
}
export interface Stakeholder { n: string; role: string; tag: string; s: number; note: string; initials: string }
export interface LedgerRow { t: string; s: string; st: 'done' | 'open' | 'late'; d?: string }
export interface TimelineRow { d: string; via: string; icon: string; who: string; t: string; x: string; hot: boolean }
export interface Flag { id: number; t: string; s: string; k: Tone; act: string }
export interface Whitespace { p: string; st: 'ada' | 'proses' | 'peluang' | '-'; v?: number; why?: string }
export interface AccountPage {
  id: string; name: string; sector: string; branch: string; owner: string;
  health: number; trend: number; value: number; last: string; last_icon: string;
  next_action: Action | null; next_meta: string; alternatives: string[];
  expansion: null | {
    installed: { s: string; y: string; w: string; c: string; warn: boolean }[];
    whitespace: Whitespace[]; potential: number; lines: number;
  };
  memory: { text: string; updated: string; provenance: { icon: string; label: string }[] };
  stakeholders: Stakeholder[]; single_threaded: boolean;
  deal: {
    opportunity_id: string; opp: string; odoo_stage: string; signal: string; stage_why: string;
    breakdown: { label: string; value: number }[]; flags: Flag[]; locked: boolean;
  } | null;
  commitments: { kami: LedgerRow[]; mereka: LedgerRow[] };
  timeline: TimelineRow[];
  has_network: boolean;
}
// GET /api/accounts?q= -> {items: AccountListItem[], total_active: number}
// GET /api/accounts/{id} -> AccountPage
// POST /api/signals/{id}/act -> {action: Action, toast}          (flag / alternative buttons)
// POST /api/accounts/{id}/alternative {label} -> {action, toast}
// POST /api/accounts/{id}/whitespace {line} -> {action, toast}   (+ Opportunity …)
// POST /api/opportunities/{id}/override {stage?: string, probability?: number, reason: string} -> {toast}

// ---------- Network (Peta 3D) ----------
export interface NetSales { id: string; n: string; branch: string; no: string }
export interface NetContact { id: string; n: string; role: string; acc: string | null; account_name?: string; health: number | null; decision: boolean }
export interface Network {
  months: string[];         // ["Apr", …, "Sep"]
  period: number;           // 1 | 2 | 3 | 6 (months)
  period_label: string;     // "30 hari"
  sales: NetSales[];
  contacts: NetContact[];
  edges: [string, string, number][];        // [salesId, contactId, messages in period]
  monthly: Record<string, number[]>;         // contactId -> 6 monthly totals (all sales)
  pairs: { a: string; b: string; account: string; w: number }[];
  insights: { k: Tone; icon: string; t: string; s: string }[];
  count: { connections: number; messages: number };
}
// GET /api/network?period=1&sales=all&account=rsud -> Network

// ---------- Penjualan / Pipeline ----------
export interface KanbanCard {
  id: string;               // opportunity id
  account_id: string | null;// null => "Lead baru · belum ada halaman akun"
  opp: string; account: string; value: number; prob: string; // "at 70%"
  closing: string;          // "15 Okt 2026" or "Won 15 Sep"
  tags: string[]; prio: number | null; activity: Tone | null; owner_initials: string;
  health: number | null; signal: string; note: string;
  action: Action | null; won: boolean;
}
export interface Pipeline {
  sync: { connected: boolean; title: string; meta: string; pill: Pill; source_label: string };
  stages: { id: number; name: string; total: number; won: boolean; cards: KanbanCard[] }[];
  field: { id: string; account: string; opp: string; value: number; health: number; trend: number; stage: string; signal: string; manual_prob: number; next: string }[];
  field_title: string;      // "8 deal terbuka · Rp 11,9 M"
  inference: { id: string; account: string; why: string; signal: string; stage: string; manual: number; arc: number; can_write: boolean }[];
  weighted: { sales: number; arc: number; note: string };
  winloss: { meta: string; rows: { tag: string; html: string }[] };
  tenders: { meta: string; items: Tender[] };
  team: TeamRow[];
}
export interface Tender { id: string; score: number; tone: Tone; title: string; detail_html: string; status: string; primary: boolean }
export interface TeamRow { name: string; branch: string; pipeline: number; response: string; followup: Pill; multithread: Pill; coach: string }
// GET /api/pipeline -> Pipeline
// POST /api/opportunities/{id}/stage {stage_id} -> {toast}
// POST /api/opportunities/{id}/write-probability -> {action, toast}
// POST /api/sync/odoo -> {toast}
// POST /api/tenders/{id}/qualify | /skip -> {toast}
// GET /api/forecast?exclude=bsd -> Forecast

// ---------- Prospek & funnel ----------
export interface FunnelStage { label: string; sub: string; n: number; width: number; conv: string; conv_tone: 'good' | 'warn' | ''; soft?: boolean; won?: boolean }
export interface InboundListItem {
  id: string; score: number | null; status: 'unknown' | 'identified' | 'qualified' | 'lead' | 'not_prospect';
  tone: Tone; name: string; company: string; no: string; via: string; first: string; when: string; lead: boolean;
}
export interface InboundDetail extends InboundListItem {
  role: string;
  sources: { s: string; c: string; v: string }[];
  overview: string;
  solutions: { t: string; v: number; k: 'proses' | 'peluang' | 'paket'; why: string }[];
  questions: { q: string; u: string }[];
  requested: number; extra: number;
}
export interface Prospects {
  funnel: { meta: string; stages: FunnelStage[]; meta_cards: { label: string; value: string; sub: string }[] };
  sources: { meta: string; rows: { label: string; n: number; win: number; tone: Tone | '' }[]; note: string };
  inbound: InboundListItem[];
  flywheel: {
    meta: string; center: string;
    metrics: { label: string; value: string; sub: string }[];
    verdict_html: string;
  };
}
// GET /api/prospects -> Prospects
// GET /api/prospects/{id} -> InboundDetail
// POST /api/prospects/{id}/lead | /reply-first | /not-prospect | /undo -> {toast, item: InboundDetail}
// POST /api/prospects/{id}/question {index} -> {toast}
// POST /api/prospects/{id}/talking-points -> {toast}

// ---------- Kas ----------
export interface CashKpi { label: string; value: string; unit?: string; delta: string; tone: 'good' | 'bad' | 'n' }
export interface L2CRow {
  id: string; account: string; sub: string; stage: number; stage_label: string;
  days: number; bench: number; tone: Tone; note: string; action: Action | null;
}
export interface Cash {
  kpis: CashKpi[];
  l2c: L2CRow[];
  aging: { meta: string; rows: { label: string; width: number; color: string; value: string; sub: string }[] };
  collection: { badge: Pill; t: string; s: string; action: Action }[];
  forecast: { total: number; items: { t: string; s: string; p: number; v: number }[]; note_html: string };
}
// GET /api/cash -> Cash
// GET /api/cash/forecast?what_if=create_invoice:<cash_item_id> -> {total, delta}
// GET /api/cash/forecast.csv

// ---------- Pengaturan ----------
export interface ConnectorCard { id: string; name: string; sub: string; logo: string; color: string; text_color: string; dot: Tone | 'off'; status: string; active: boolean }
export interface SettingsSources { sources: ConnectorCard[]; identity: ConnectorCard[]; getcontact_note: string }
export interface WaNumber { id: string; initials: string; label: string; no: string; status: 'connected' | 'pairing' | 'unlinked' | 'disconnected'; note: string }
export interface PrivacyRule { id: string; title: string; detail: string; enabled: boolean; locked: boolean }
export interface WaGroup { id: string; name: string; type: 'external' | 'internal'; members_note: string; read: boolean }
export interface SettingsWhatsApp {
  mode: 'cloud' | 'bridge';
  history_days: number;
  history_note_html: string;
  numbers: WaNumber[];
  rules: PrivacyRule[];
  groups: WaGroup[];
  unlisted_groups: number;
  pairing: null | { session_id: string; name: string; branch: string; qr_png: string; expires_in: number; history_days: number };
  extracted: { period: string; messages: number; commitments: number; contacts: number; sent: number };
}
// GET /api/settings/whatsapp -> SettingsWhatsApp
// POST /api/settings/whatsapp {mode?, history_days?} -> SettingsWhatsApp
// POST /api/wa/sessions/{id}/link -> {toast, pairing}
// POST /api/wa/sessions/{id}/instructions -> {toast}
// POST /api/settings/privacy/{id} {enabled} -> {toast}
// POST /api/chat/groups/{id}/policy {read} -> {toast}
export interface InternalRow { id: number; n: string; no: string; unit: string; branch: string; src: string }
export interface SuspectRow { id: number; no: string; n: string; why: string }
export interface SettingsInternal { rows: InternalRow[]; meta: string; suspects: SuspectRow[]; units: string[]; branches: string[] }
// GET /api/internal-numbers -> SettingsInternal
// POST /api/internal-numbers {name, phone, unit, branch} -> {toast}
// DELETE /api/internal-numbers/{id} -> {toast}
// POST /api/internal-numbers/sync-talenta -> {toast}
// POST /api/internal-numbers/import (text/csv body) -> {toast}
// POST /api/internal-suspects/{id}/confirm | /reject -> {toast}
export interface McpTool { name: string; desc: string; kind: 'read' | 'write' | 'human-only' | 'ceo'; approval?: boolean }
export interface SettingsAI {
  mcp: { endpoint: string; version: string; active: boolean; groups: { id: string; label: string; note: string; enabled: boolean; tools: McpTool[] }[] };
  clients: { id: string; name: string; logo: string; color: string; desc: string; status: Pill; last: string }[];
  config_snippet: string;
  routing: { tier: string; title: string; sub: string; options: string[]; selected: string }[];
  cost_month: string;       // "Rp 1,9 jt"
  openapi_url: string;
}
// GET /api/settings/ai -> SettingsAI
// POST /api/settings/mcp-groups/{id} {enabled} -> {toast}
// POST /api/settings/routing {tier, option} -> {toast}
export interface ApiKeyRow { id: string; name: string; line: string; active: boolean }
export interface SettingsAPI {
  endpoint: string;
  endpoints: { method: 'GET' | 'POST' | 'WEBHOOK'; path: string; html: string }[];
  curl: string;
  keys: ApiKeyRow[];
  calibration: { agent: string; rate: number; warn: boolean }[];
  learned: { html: string }[];
}
// GET /api/settings/api -> SettingsAPI
// POST /api/api-keys {name?, scopes?} -> {key: string, row: ApiKeyRow, toast}
// DELETE /api/api-keys/{id} -> {toast}

export interface Toast { toast: string }
