-- +goose Up
-- Stage 14: weekly pilot snapshots (Pengaturan → Pilot, CSV export, docs/PILOT-REPORT.md). One row per ISO week and
-- branch; the job pilot.snapshot (Monday 00.45 WIB) and `arc ctl pilot snapshot` write it idempotently.
create table pilot_weeks (
  week date not null,                                  -- Monday of the ISO week (WIB)
  branch text not null,
  data jsonb not null,                                 -- pilot.Week: agents, KPI, caught-before-churn, audit
  created_at timestamptz not null default now(),
  primary key (week, branch)
);

-- +goose Down
drop table if exists pilot_weeks;
