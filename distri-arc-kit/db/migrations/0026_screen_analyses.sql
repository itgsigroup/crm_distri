-- +goose Up
-- Analisa Stok and Analisa Kas (ADR 0030): like Analisa WhatsApp, the AI reading of the Push stok and Kredit · kas
-- pages is written by Claude (through MCP, or the server's model) — an overview and one note per item (aging SKU per
-- branch, dealer). The numbers stay computed by GSI Orbit; only what the AI wrote is stored here, per cycle.
create table screen_analyses (
  cycle_id uuid not null references cycles on delete cascade,
  screen text not null check (screen in ('stock', 'credit')),
  overview text not null,
  lines jsonb not null default '{}'::jsonb,  -- {"<stock item id | dealer uuid>": {"note": "...", "confidence": 80}}
  source text not null default 'ai',
  created_at timestamptz not null default now(),
  primary key (cycle_id, screen)
);
create index screen_analyses_latest on screen_analyses (screen, created_at desc);

-- +goose Down
drop table if exists screen_analyses;
