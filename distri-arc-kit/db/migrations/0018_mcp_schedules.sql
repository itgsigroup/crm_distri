-- +goose Up
-- Scheduled analysis (ADR 0022): Claude analyses the data through the same MCP tools on a cron schedule (WIB) and
-- writes a report. Each schedule has its own mcp_clients row (kind 'schedule', no token) so its tool calls show in
-- the MCP call log. A run is unique per (schedule, slot): re-running the tick never doubles a report.
create table mcp_schedules (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  prompt text not null,
  cron text not null,                       -- 5 fields, WIB (minute hour day month weekday)
  enabled bool not null default true,
  scopes text[] not null default '{read,analyze}',
  max_steps int not null default 12,        -- tool calls per run
  client_id uuid references mcp_clients on delete set null,
  created_by uuid references users on delete set null,
  next_run_at timestamptz,                  -- null: the tick computes it from cron
  last_run_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table mcp_schedule_runs (
  id uuid primary key default gen_random_uuid(),
  schedule_id uuid not null references mcp_schedules on delete cascade,
  slot timestamptz,                         -- the scheduled time; null for "Jalankan sekarang"
  trigger text not null check (trigger in ('schedule', 'manual')),
  triggered_by text,
  status text not null default 'running' check (status in ('running', 'ok', 'template', 'error')),
  engine text,                              -- claude | template
  model text,
  report text,                              -- Markdown
  steps jsonb not null default '[]',        -- [{tool, args, status, ms, summary}]
  tokens_in int not null default 0,
  tokens_out int not null default 0,
  cost_idr bigint not null default 0,
  error text,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  unique (schedule_id, slot)
);
create index mcp_schedule_runs_recent on mcp_schedule_runs (schedule_id, started_at desc);

insert into policies (key, value) values ('mcp.analyst', '{"model": "claude-opus-5-5", "daily_budget_idr": 50000}')
on conflict (key) do nothing;

insert into mcp_schedules (name, prompt, cron) values
  ('Ringkasan pagi',
   'Analisis kondisi bisnis distribusi pagi ini. Mulai dari data_ringkasan dan penjualan_bulanan (3 bulan), lalu dalami yang paling penting: dealer lewat jadwal (jadwal_lewat), piutang lewat tempo (piutang_ringkas), dan stok menua (stok_aging). Tulis: 1) kondisi singkat dengan angka, 2) cabang dan sales yang perlu perhatian, 3) 5 tindakan prioritas hari ini — siapa, apa, kenapa.',
   '0 7 * * 1-6');

-- +goose Down
delete from policies where key = 'mcp.analyst';
drop table mcp_schedule_runs;
drop table mcp_schedules;
