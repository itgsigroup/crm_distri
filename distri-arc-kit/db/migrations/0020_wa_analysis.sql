-- +goose Up
-- Analisa WhatsApp (ADR 0026): every full Orchestrator cycle reads the WhatsApp conversations of the last days next to
-- the BigQuery data and keeps what it found — one row per cycle (counts, the AI overview) and one per conversation
-- (intent, requested items, waiting time, the AI line). Re-running a cycle replaces its rows (primary keys).
create table wa_analyses (
  cycle_id uuid primary key references cycles on delete cascade,
  window_days smallint not null,
  threads int not null default 0,
  inbound int not null default 0,
  outbound int not null default 0,
  waiting int not null default 0,
  reply_median_min int,
  overview text,                                        -- written by AI only (ADR 0025); null without a model
  source text not null default 'rules' check (source in ('rules','ai')),
  created_at timestamptz not null default now()
);
create table wa_insights (
  cycle_id uuid not null references wa_analyses on delete cascade,
  thread_id uuid not null references chat_threads on delete cascade,
  kind text not null check (kind in ('dealer','new')),
  dealer_id uuid references dealers on delete set null,
  sales_id uuid references sales_users on delete set null,
  title text not null,
  intent text not null,
  items jsonb not null default '[]',
  last_text text,
  inbound int not null default 0,
  outbound int not null default 0,
  last_in_at timestamptz,
  last_out_at timestamptz,
  waiting_since timestamptz,
  reply_median_min int,
  needs_reply bool not null default false,
  priority int not null default 0,
  signal_ids uuid[] not null default '{}',              -- provenance: the inbound messages' signals
  proposal_ids uuid[] not null default '{}',            -- proposals the agents made from those signals
  summary text,                                         -- written by AI only
  confidence smallint,                                  -- 0–100, AI summary
  primary key (cycle_id, thread_id)
);
create index wa_insights_thread on wa_insights (thread_id);

-- +goose Down
drop table if exists wa_insights;
drop table if exists wa_analyses;
