-- +goose Up
-- Analisa Dealer (ADR 0055): Claude's reading of one dealer — its numbers and its WhatsApp chats — written through MCP
-- (the claude.ai routine): a status (bermasalah / perlu perhatian / aman), the reasons, a summary and a WhatsApp draft
-- the sales may send from Chat. Only what Claude wrote is stored; the latest per dealer is shown on the dealer page.
create table dealer_analyses (
  cycle_id uuid not null references cycles on delete cascade,
  dealer_id uuid not null references dealers on delete cascade,
  status text not null check (status in ('bermasalah', 'perlu_perhatian', 'aman')),
  overview text not null,
  reasons jsonb not null default '[]'::jsonb,
  wa_draft text not null default '',
  confidence int,
  created_at timestamptz not null default now(),
  primary key (cycle_id, dealer_id)
);
create index dealer_analyses_latest on dealer_analyses (dealer_id, created_at desc);

-- +goose Down
drop table if exists dealer_analyses;
