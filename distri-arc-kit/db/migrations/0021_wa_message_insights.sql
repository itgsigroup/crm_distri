-- +goose Up
-- Analisa WhatsApp per bubble (ADR 0027): every analysis also reads each message of the conversations it covers —
-- the request of a customer message and whether it was answered, how long a sales reply made the customer wait —
-- and the AI note on it when a model or Claude through MCP read it. One row per message, the latest analysis wins
-- (re-running replaces it; the AI note stays until a new one is written).
create table wa_message_insights (
  message_id uuid primary key,                       -- chat_messages is partitioned: no foreign key, purged with it
  thread_id uuid not null references chat_threads on delete cascade,
  sent_at timestamptz not null,
  direction text not null check (direction in ('in','out')),
  state text not null check (state in ('replied','waiting','needs_reply','closing','reply','follow_up')),
  intent text not null,
  items jsonb not null default '[]',
  wait_min int,
  cycle_id uuid references cycles on delete set null,
  analyzed_at timestamptz not null,
  note text,                                          -- written by AI only (ADR 0025)
  confidence smallint,
  note_cycle_id uuid references cycles on delete set null
);
create index wa_message_insights_thread on wa_message_insights (thread_id, sent_at);

-- +goose Down
drop table if exists wa_message_insights;
