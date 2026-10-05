# 03 · Data Model — PostgreSQL 16

Konvensi: `id uuid default gen_random_uuid()`, `created_at/updated_at timestamptz`, uang `bigint` rupiah, persentase `smallint`, JSONB untuk payload mentah dan kebijakan. Semua tabel yang bersumber dari Odoo punya `source_system text`, `source_id text`, `source_write_date timestamptz` dengan unique `(source_system, source_id)`.

## Inti dealer
```sql
create table sales_users (
  id uuid primary key, name text not null, branch text not null,
  wa_number text unique, odoo_user_id int, role text not null check (role in ('ceo','sales','admin','finance','warehouse')),
  active bool default true, created_at timestamptz default now()
);
create table dealers (
  id uuid primary key, name text not null, city text, branch text not null,
  tier char(1) check (tier in ('A','B','C')), segment_desc text,          -- "Toko CCTV & jaringan"
  owner_id uuid references sales_users, credit_limit bigint default 0, payment_terms_days int default 30,
  source_system text, source_id text, source_write_date timestamptz,
  memo text, memo_signal_ids uuid[], memo_updated_at timestamptz,           -- memori dealer (brief hidup)
  metrics_current jsonb,                                                     -- cache hasil metrics.Compute
  created_at timestamptz default now(), updated_at timestamptz default now(),
  unique (source_system, source_id)
);
create index on dealers using gin (name gin_trgm_ops);
create table contacts (
  id uuid primary key, dealer_id uuid references dealers on delete cascade, name text, role text,
  wa_number text, is_primary bool default false, last_interaction_at timestamptz, interactions_90d int default 0,
  source_system text, source_id text, unique (dealer_id, wa_number)
);
create table dealer_sow_estimates (                       -- konfirmasi sales 1×/kuartal
  id uuid primary key, dealer_id uuid references dealers, quarter text, sow smallint, note text,
  confirmed_by uuid references sales_users, confirmed_at timestamptz
);
create table dealer_metrics_daily (                       -- snapshot untuk tren & "3 bulan lalu"
  dealer_id uuid references dealers, as_of date, rhythm_days int, last_order_days int, cyc numeric(6,2),
  status text, activity text, freq numeric(6,2), avg_order bigint, segment char(1), sow smallint, mix smallint,
  credit_room numeric(5,2), credit_state text, pay_days int, on_time smallint, pic_active smallint, score smallint,
  score_parts jsonb, primary key (dealer_id, as_of)
);
```

## Sinyal & transaksi
```sql
create table signals (                                    -- semua yang masuk, apa pun sumbernya
  id uuid primary key, kind text not null check (kind in ('wa','wa_group','so','invoice','payment','stock','manual')),
  dealer_id uuid references dealers, contact_id uuid references contacts, sales_id uuid references sales_users,
  occurred_at timestamptz not null, dedupe_key text not null unique,       -- wa msg id / odoo model:id:write_date
  summary text, payload jsonb not null, processed_at timestamptz,
  created_at timestamptz default now()
) partition by range (occurred_at);                       -- partisi bulanan (Stage 13; sebelum itu tabel biasa)
create index on signals (dealer_id, occurred_at desc);
create table orders (  -- SO
  id uuid primary key, dealer_id uuid references dealers, number text, state text,   -- order|siap|kirim|invoice|bayar
  ordered_at timestamptz, confirmed_at timestamptz, shipped_at timestamptz, invoiced_at timestamptz, paid_at timestamptz,
  total bigint, margin_pct numeric(5,2), lines jsonb,                                 -- [{product, category, qty, price}]
  created_by text,                                                                    -- 'odoo' | 'ai_order_draft'
  source_system text, source_id text, source_write_date timestamptz, unique (source_system, source_id)
);
create table invoices (
  id uuid primary key, dealer_id uuid references dealers, order_id uuid references orders, number text,
  issued_at date, due_at date, total bigint, paid bigint default 0, paid_at date, state text,
  source_system text, source_id text, source_write_date timestamptz, unique (source_system, source_id)
);
create table payments (
  id uuid primary key, dealer_id uuid references dealers, invoice_id uuid references invoices,
  paid_at date, amount bigint, source_system text, source_id text, unique (source_system, source_id)
);
create table stock_items (
  id uuid primary key, branch text, sku text, name text, category text,                -- 6 KAT
  qty int, unit_cost bigint, value bigint, age_days int, weekly_velocity numeric(8,2),
  source_system text, source_id text, source_write_date timestamptz, unique (source_system, source_id, branch)
);
create table category_map (odoo_category_id int primary key, kat text not null);        -- → 6 KAT
```

## Orchestrator, agen, keputusan
```sql
create table cycles (
  id uuid primary key, number bigserial, trigger text not null check (trigger in ('schedule','manual','mcp')),
  scope text not null default 'all',                     -- all | screen:orbit | dealer:<id> | agent:<name>
  via text check (via in ('api','mcp')),                 -- jalur LLM yang dipakai
  requested_by text,                                     -- user id / mcp client id / 'scheduler'
  status text not null default 'running',                -- running | done | partial | failed
  started_at timestamptz default now(), finished_at timestamptz, duration_ms int,
  signals_count int, auto_count int, decision_count int, conflict_count int, note text
);
create table cycle_stages (
  cycle_id uuid references cycles on delete cascade, stage text not null,   -- ingest|analyze|synthesize|decide|execute|learn
  status text not null, started_at timestamptz, finished_at timestamptz, detail jsonb, primary key (cycle_id, stage)
);
create table agent_runs (
  id uuid primary key, cycle_id uuid references cycles, agent text not null, status text, duration_ms int,
  input_hash text, proposals_count int, error text, llm_call_ids uuid[]
);
create table proposals (                                 -- "Action" di UI
  id uuid primary key, cycle_id uuid references cycles, agent text not null, dealer_id uuid references dealers,
  kind text not null,          -- followup|collect|credit_release|credit_limit|price_counter|push_stock|so_draft|return|transfer|new_dealer
  title text not null, why text not null, prep text, preview text, steps jsonb, impact jsonb,
  confidence numeric(3,2) not null, signal_ids uuid[] not null check (cardinality(signal_ids) > 0),
  autonomy text not null check (autonomy in ('auto','approve')),
  status text not null default 'proposed',  -- proposed|approved|edited|rejected|executed|expired|suppressed
  due_label text, decided_by uuid references sales_users, decided_at timestamptz, decision_reason text, edited_payload jsonb,
  executed_at timestamptz, created_at timestamptz default now()
);
create index on proposals (status, created_at desc);
create table conflicts (
  id uuid primary key, cycle_id uuid references cycles, dealer_id uuid references dealers,
  agent_a text, agent_b text, title text, resolution text, rule text,      -- rule: credit_over_stock | collect_before_followup | margin_floor | one_owner
  proposal_ids uuid[], created_at timestamptz default now()
);
create table plan_items (                                -- Rencana hari ini
  id uuid primary key, plan_date date not null, cycle_id uuid references cycles, seq int,
  time_label text, agent text, autonomy text, text_html text, proposal_id uuid references proposals,
  status text default 'scheduled',  -- scheduled|running|done|waiting|skipped
  unique (plan_date, seq)
);
create table outbox (                                    -- satu-satunya jalur keluar
  id uuid primary key, proposal_id uuid references proposals not null, channel text not null,  -- wa|odoo_so_draft|odoo_note
  to_ref text, payload jsonb, status text default 'pending', attempts int default 0, sent_at timestamptz, error text
);
create table calibration_events (
  id uuid primary key, proposal_id uuid references proposals, agent text, dealer_id uuid, kind text,
  decision text, reason text, suppress_until date, created_at timestamptz default now()
);
create table agent_state (agent text primary key, confidence smallint, last_run_at timestamptz, last_output text, params jsonb);
```

## WhatsApp & chat
```sql
create table internal_numbers (wa_number text primary key, label text, department text, is_sales bool default false, added_by uuid, added_at timestamptz default now());
create table wa_groups (id uuid primary key, jid text unique, name text, kind text check (kind in ('internal','external')), branch text, members int, read_enabled bool default true);
create table chat_threads (
  id uuid primary key, kind text check (kind in ('dealer','group','new')), dealer_id uuid references dealers, group_id uuid references wa_groups,
  wa_jid text unique, title text, sales_id uuid references sales_users, last_message_at timestamptz, unread int default 0, identification jsonb
);
create table chat_messages (
  id uuid primary key, thread_id uuid references chat_threads, wa_msg_id text unique, direction text check (direction in ('in','out')),
  from_number text, body text, media jsonb, sent_at timestamptz not null, annotation jsonb, signal_id uuid references signals
) partition by range (sent_at);
create table identifications (wa_number text primary key, sources jsonb, best_name text, best_org text, score smallint, created_at timestamptz default now());
```

## Kebijakan, MCP, audit, auth
```sql
create table policies (key text primary key, value jsonb not null, version int default 1, updated_by uuid, updated_at timestamptz default now());
-- kunci: orbit.thresholds, segment.thresholds, credit.rules, followup.rules, margin.floor, autonomy.matrix, mcp.permissions, llm.routing
create table policy_history (key text, version int, value jsonb, updated_by uuid, updated_at timestamptz, primary key (key, version));
create table mcp_clients (id uuid primary key, name text, kind text, token_hash text unique, scopes text[] not null, owner_id uuid, last_seen_at timestamptz, active bool default true);
create table mcp_calls (id uuid primary key, client_id uuid references mcp_clients, tool text, args jsonb, result_summary text, cycle_id uuid, duration_ms int, created_at timestamptz default now());
create table llm_calls (id uuid primary key, cycle_id uuid, agent text, provider text, model text, purpose text, input_hash text, tokens_in int, tokens_out int, cost_idr bigint, duration_ms int, created_at timestamptz default now());
create table audit_log (id uuid primary key, actor text, actor_kind text check (actor_kind in ('user','agent','mcp','system')), action text, entity text, entity_id uuid, before jsonb, after jsonb, created_at timestamptz default now());
create table users (id uuid primary key, email text unique, name text, role text, password_hash text, sales_user_id uuid references sales_users, active bool default true);
```

## View penting (sqlc)
- `v_dealer_board`: dealer + `metrics_current` dibongkar menjadi kolom (status, segment, cyc, due_in, sow, credit_state, score) → dipakai Orbit, Segmen, daftar dealer.
- `v_due_7d`, `v_drift`, `v_credit_tight`: daftar Pusat kendali.
- `v_cycle_summary`: cycles + agregat stages.

## Seed
`db/seed/dealers.json` berisi 18 dealer dari mockup (sinar, graha, mitra, indo, cahaya, lampu, bina, nusantara, citra, anugerah, megah, sarana, prima, jaya, nusa, borneo, rejeki, global) lengkap dengan 6 bulan order sintetis yang **menghasilkan metrik persis seperti mockup** (mis. Mitra Jaya: rhythm 21, last 30, At risk, Segmen C, over limit). `arc ctl seed` idempoten.
