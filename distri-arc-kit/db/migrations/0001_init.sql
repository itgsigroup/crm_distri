-- +goose Up
-- Distri ARC Orbit core schema (docs/design/03-data-model.md).
-- Conventions: uuid PKs, money bigint rupiah, percentages smallint, JSONB for raw payloads and policies.
-- Every table sourced from Odoo carries source_system/source_id/source_write_date, unique (source_system, source_id).
create extension if not exists pgcrypto with schema public;
create extension if not exists pg_trgm with schema public;

-- ---------- Inti dealer ----------
create table sales_users (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  branch text not null,
  wa_number text unique,
  odoo_user_id int,
  role text not null check (role in ('ceo','sales','admin','finance','warehouse')),
  active bool not null default true,
  source_system text, source_id text,
  created_at timestamptz not null default now(),
  unique (source_system, source_id)
);

create table dealers (
  id uuid primary key default gen_random_uuid(),
  slug text unique,                                   -- stable short key used in URLs and seed (e.g. "mitra")
  name text not null,
  city text,
  branch text not null,
  tier char(1) check (tier in ('A','B','C')),
  segment_desc text,                                  -- "Toko CCTV & jaringan"
  owner_id uuid references sales_users,
  credit_limit bigint not null default 0,
  payment_terms_days int not null default 30,
  source_system text, source_id text, source_write_date timestamptz,
  memo text, memo_signal_ids uuid[], memo_updated_at timestamptz,
  metrics_current jsonb,                              -- cache of metrics.Compute
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (source_system, source_id)
);
create index dealers_name_trgm on dealers using gin (name gin_trgm_ops);

create table contacts (
  id uuid primary key default gen_random_uuid(),
  dealer_id uuid references dealers on delete cascade,
  name text,
  role text,
  wa_number text,
  is_primary bool not null default false,
  last_interaction_at timestamptz,
  interactions_90d int not null default 0,
  source_system text, source_id text,
  unique (dealer_id, wa_number),
  unique (source_system, source_id)
);

create table dealer_sow_estimates (                   -- sales confirmation, once per quarter
  id uuid primary key default gen_random_uuid(),
  dealer_id uuid references dealers on delete cascade,
  quarter text not null,
  sow smallint not null check (sow between 0 and 100),
  note text,
  confirmed_by uuid references sales_users,
  confirmed_at timestamptz not null default now(),
  unique (dealer_id, quarter)
);

create table dealer_metrics_daily (                   -- snapshots for trends and "3 months ago"
  dealer_id uuid references dealers on delete cascade,
  as_of date not null,
  rhythm_days int, last_order_days int, cyc numeric(6,2),
  status text, activity text, freq numeric(6,2), avg_order bigint, segment text, sow smallint, mix smallint,
  credit_room numeric(5,2), credit_state text, pay_days int, on_time smallint, pic_active smallint, score smallint,
  score_parts jsonb,
  primary key (dealer_id, as_of)
);

-- ---------- Sinyal & transaksi ----------
create table signals (                                 -- everything that comes in, whatever the source
  id uuid primary key default gen_random_uuid(),       -- partition at stage 13 (monthly by occurred_at)
  kind text not null check (kind in ('wa','wa_group','so','invoice','payment','stock','manual')),
  dealer_id uuid references dealers on delete set null,
  contact_id uuid references contacts on delete set null,
  sales_id uuid references sales_users,
  occurred_at timestamptz not null,
  dedupe_key text not null unique,                     -- wa msg id / odoo model:id:write_date
  summary text,
  payload jsonb not null,
  processed_at timestamptz,
  created_at timestamptz not null default now()
);
create index signals_dealer_time on signals (dealer_id, occurred_at desc);

create table orders (                                  -- sale.order
  id uuid primary key default gen_random_uuid(),
  dealer_id uuid references dealers on delete cascade,
  number text,
  state text not null check (state in ('order','siap','kirim','invoice','bayar','cancel')),
  ordered_at timestamptz, confirmed_at timestamptz, shipped_at timestamptz, invoiced_at timestamptz, paid_at timestamptz,
  total bigint not null default 0,
  margin_pct numeric(5,2),
  lines jsonb not null default '[]',                   -- [{product, category, qty, price}]
  created_by text not null default 'odoo',             -- 'odoo' | 'ai_order_draft'
  source_system text, source_id text, source_write_date timestamptz,
  unique (source_system, source_id)
);
create index orders_dealer_time on orders (dealer_id, confirmed_at desc);

create table invoices (
  id uuid primary key default gen_random_uuid(),
  dealer_id uuid references dealers on delete cascade,
  order_id uuid references orders on delete set null,
  number text,
  issued_at date, due_at date,
  total bigint not null default 0,
  paid bigint not null default 0,
  paid_at date,
  state text not null default 'posted',
  source_system text, source_id text, source_write_date timestamptz,
  unique (source_system, source_id)
);
create index invoices_dealer on invoices (dealer_id, issued_at desc);

create table payments (
  id uuid primary key default gen_random_uuid(),
  dealer_id uuid references dealers on delete cascade,
  invoice_id uuid references invoices on delete set null,
  paid_at date,
  amount bigint not null,
  source_system text, source_id text,
  unique (source_system, source_id)
);

create table stock_items (
  id uuid primary key default gen_random_uuid(),
  branch text not null,
  sku text not null,
  name text not null,
  category text not null,                              -- one of the 6 KAT
  qty int not null default 0,
  unit_cost bigint not null default 0,
  value bigint not null default 0,
  age_days int not null default 0,
  weekly_velocity numeric(8,2) not null default 0,
  source_system text, source_id text, source_write_date timestamptz,
  unique (source_system, source_id, branch)
);

create table category_map (odoo_category_id int primary key, kat text not null);

-- Two-way commitments shown on the dealer page ("Kami" / "Mereka"); filled from proposals and WA extraction (stage 12).
create table commitments (
  id uuid primary key default gen_random_uuid(),
  dealer_id uuid references dealers on delete cascade,
  side text not null check (side in ('kami','mereka')),
  title text not null,
  detail text,
  status text not null default 'open' check (status in ('open','done','late')),
  due_at date,
  late_label text,
  invoice_id uuid references invoices on delete set null,
  proposal_id uuid,
  signal_ids uuid[],
  source_key text unique,
  created_at timestamptz not null default now()
);

-- ---------- Orchestrator, agen, keputusan ----------
create table cycles (
  id uuid primary key default gen_random_uuid(),
  number bigserial,
  trigger text not null check (trigger in ('schedule','manual','mcp')),
  scope text not null default 'all',                   -- all | screen:orbit | dealer:<id> | agent:<name>
  via text check (via in ('api','mcp')),
  requested_by text,
  status text not null default 'running',              -- running | done | partial | failed
  started_at timestamptz not null default now(), finished_at timestamptz, duration_ms int,
  signals_count int, auto_count int, decision_count int, conflict_count int, note text
);

create table cycle_stages (
  cycle_id uuid references cycles on delete cascade,
  stage text not null check (stage in ('ingest','analyze','synthesize','decide','execute','learn')),
  status text not null,
  started_at timestamptz, finished_at timestamptz,
  detail jsonb,
  primary key (cycle_id, stage)
);

create table agent_runs (
  id uuid primary key default gen_random_uuid(),
  cycle_id uuid references cycles on delete cascade,
  agent text not null,
  status text,
  duration_ms int,
  input_hash text,
  proposals_count int,
  error text,
  llm_call_ids uuid[],
  unique (cycle_id, agent)
);

create table proposals (                               -- "Action" in the UI
  id uuid primary key default gen_random_uuid(),
  cycle_id uuid references cycles on delete set null,
  agent text not null,
  dealer_id uuid references dealers on delete cascade,
  kind text not null,  -- followup|collect|credit_release|credit_limit|credit_hold|price_counter|push_stock|so_draft|return|transfer|po_request|installment|new_dealer|price_list|reply|plan_change
  title text not null,
  why text not null,
  prep text, preview text, steps jsonb, impact jsonb,
  confidence numeric(3,2) not null check (confidence between 0 and 1),
  signal_ids uuid[] not null check (cardinality(signal_ids) > 0),
  autonomy text not null check (autonomy in ('auto','approve')),
  status text not null default 'proposed' check (status in ('proposed','approved','edited','rejected','executed','expired','suppressed')),
  due_label text,
  decided_by uuid references sales_users, decided_at timestamptz, decision_reason text, edited_payload jsonb,
  executed_at timestamptz,
  created_at timestamptz not null default now()
);
create index proposals_status_time on proposals (status, created_at desc);

create table conflicts (
  id uuid primary key default gen_random_uuid(),
  cycle_id uuid references cycles on delete cascade,
  dealer_id uuid references dealers on delete cascade,
  agent_a text, agent_b text, title text, resolution text,
  rule text,                                           -- credit_over_stock | collect_before_followup | margin_floor | one_owner | dedupe | suppression | followup_gap
  proposal_ids uuid[],
  created_at timestamptz not null default now()
);

create table plan_items (                              -- "Rencana hari ini"
  id uuid primary key default gen_random_uuid(),
  plan_date date not null,
  cycle_id uuid references cycles on delete set null,
  seq int not null,
  time_label text, agent text, autonomy text, text_html text,
  proposal_id uuid references proposals on delete set null,
  status text not null default 'scheduled' check (status in ('scheduled','running','done','waiting','skipped')),
  unique (plan_date, seq)
);

create table outbox (                                  -- the only way out
  id uuid primary key default gen_random_uuid(),
  proposal_id uuid references proposals not null,
  channel text not null check (channel in ('wa','odoo_so_draft','odoo_note')),
  to_ref text,
  payload jsonb,
  status text not null default 'pending',
  attempts int not null default 0,
  sent_at timestamptz,
  error text,
  created_at timestamptz not null default now(),
  unique (proposal_id, channel)
);

create table calibration_events (
  id uuid primary key default gen_random_uuid(),
  proposal_id uuid references proposals on delete set null,
  agent text, dealer_id uuid, kind text,
  decision text, reason text,
  suppress_until date,
  created_at timestamptz not null default now()
);

create table agent_state (agent text primary key, confidence smallint, last_run_at timestamptz, last_output text, params jsonb);

-- ---------- WhatsApp & chat ----------
create table internal_numbers (
  wa_number text primary key, label text, department text, is_sales bool not null default false,
  added_by uuid, added_at timestamptz not null default now()
);
create table wa_groups (
  id uuid primary key default gen_random_uuid(), jid text unique, name text,
  kind text check (kind in ('internal','external')), branch text, members int, read_enabled bool not null default true
);
create table chat_threads (
  id uuid primary key default gen_random_uuid(),
  kind text check (kind in ('dealer','group','new')),
  dealer_id uuid references dealers on delete set null,
  group_id uuid references wa_groups on delete set null,
  contact_id uuid references contacts on delete set null,
  wa_jid text unique, title text, subtitle text,
  sales_id uuid references sales_users,
  last_message_at timestamptz, unread int not null default 0, identification jsonb
);
create table chat_messages (                           -- partition at stage 13 (monthly by sent_at)
  id uuid primary key default gen_random_uuid(),
  thread_id uuid references chat_threads on delete cascade,
  wa_msg_id text unique,
  direction text check (direction in ('in','out')),
  from_number text, from_name text, body text, media jsonb,
  sent_at timestamptz not null,
  annotation jsonb,
  signal_id uuid references signals on delete set null
);
create index chat_messages_thread_time on chat_messages (thread_id, sent_at);
create table identifications (wa_number text primary key, sources jsonb, best_name text, best_org text, score smallint, created_at timestamptz not null default now());

-- ---------- Kebijakan, MCP, audit, auth ----------
create table policies (key text primary key, value jsonb not null, version int not null default 1, updated_by uuid, updated_at timestamptz not null default now());
create table policy_history (key text, version int, value jsonb, updated_by uuid, updated_at timestamptz, primary key (key, version));
create table mcp_clients (id uuid primary key default gen_random_uuid(), name text, kind text, token_hash text unique, scopes text[] not null, owner_id uuid, last_seen_at timestamptz, active bool not null default true, created_at timestamptz not null default now());
create table mcp_calls (id uuid primary key default gen_random_uuid(), client_id uuid references mcp_clients on delete set null, tool text, args jsonb, result_summary text, cycle_id uuid, duration_ms int, created_at timestamptz not null default now());
create table llm_calls (id uuid primary key default gen_random_uuid(), cycle_id uuid, agent text, provider text, model text, purpose text, input_hash text, tokens_in int, tokens_out int, cost_idr bigint, duration_ms int, created_at timestamptz not null default now());
create table audit_log (
  id uuid primary key default gen_random_uuid(), actor text,
  actor_kind text check (actor_kind in ('user','agent','mcp','system')),
  action text, entity text, entity_id uuid, before jsonb, after jsonb,
  created_at timestamptz not null default now()
);
create index audit_log_time on audit_log (created_at desc);
create table users (
  id uuid primary key default gen_random_uuid(), email text unique, name text, role text, password_hash text,
  sales_user_id uuid references sales_users, active bool not null default true
);

-- ---------- Views ----------
-- v_dealer_board: dealer + metrics_current unpacked (Orbit, Segmen, dealer list).
create view v_dealer_board as
select d.id, d.slug, d.name, d.city, d.branch, d.tier, d.segment_desc, d.credit_limit,
       s.name as owner_name,
       d.metrics_current->>'status'        as status,
       d.metrics_current->>'segment'       as segment,
       (d.metrics_current->>'cyc')::numeric as cyc,
       (d.metrics_current->>'due_in')::int  as due_in,
       (d.metrics_current->>'sow')::int     as sow,
       d.metrics_current->'credit'->>'state' as credit_state,
       (d.metrics_current->>'score')::int   as score
from dealers d left join sales_users s on s.id = d.owner_id;

-- +goose Down
drop view if exists v_dealer_board;
drop table if exists users, audit_log, llm_calls, mcp_calls, mcp_clients, policy_history, policies,
  identifications, chat_messages, chat_threads, wa_groups, internal_numbers,
  agent_state, calibration_events, outbox, plan_items, conflicts, proposals, agent_runs, cycle_stages, cycles,
  commitments, category_map, stock_items, payments, invoices, orders, signals,
  dealer_metrics_daily, dealer_sow_estimates, contacts, dealers, sales_users cascade;
