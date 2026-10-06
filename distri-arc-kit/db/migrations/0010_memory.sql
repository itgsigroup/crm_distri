-- +goose Up
-- Stage 10: the dealer memo as sentences, each with its sources; calibration lessons.
alter table dealers add column memo_sentences jsonb;   -- [{"text": "...", "signal_ids": ["..."]}]
create table calibration_lessons (
  id uuid primary key default gen_random_uuid(),
  agent text not null,
  kind text not null,
  reason text not null,
  scope text not null,                -- "tier C" | dealer name | "semua dealer"
  product text,                       -- product the rejected proposals were about (when one)
  rejections int not null,
  text text not null,                 -- "AI Stok tidak lagi menawarkan HDD ke dealer tier C (ditolak 4×)"
  suppress_until date,
  created_at timestamptz not null default now(),
  unique nulls not distinct (agent, kind, reason, scope, product)
);

create table briefs (                 -- Ringkasan Orchestrator, written by the full cycle
  brief_date date primary key,
  cycle_id uuid references cycles on delete set null,
  brief jsonb not null,               -- views.Brief with text and signal_ids per point
  source text not null,               -- llm | template
  created_at timestamptz not null default now()
);

-- +goose Down
drop table if exists briefs;
drop table if exists calibration_lessons;
alter table dealers drop column memo_sentences;
