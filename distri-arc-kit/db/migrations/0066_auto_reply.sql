-- +goose Up
-- Balas otomatis saat sales istirahat (ADR 0066): one row per customer message answered automatically, so a job run
-- twice never sends twice and the guards count per conversation and break. chat_messages is partitioned (no foreign
-- key to its id), like reply_ai_drafts.
create table auto_replies (
  message_id uuid primary key,
  thread_id uuid not null references chat_threads on delete cascade,
  kind text not null check (kind in ('fact', 'template')),
  break_start timestamptz not null,
  proposal_id uuid references proposals,
  reply_id uuid,
  created_at timestamptz not null default now()
);
create index auto_replies_thread on auto_replies (thread_id, break_start);
create index auto_replies_created on auto_replies (created_at desc);

-- +goose Down
drop table auto_replies;
