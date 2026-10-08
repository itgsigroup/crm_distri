-- name: ListMCPSchedules :many
select * from mcp_schedules order by created_at;

-- name: GetMCPSchedule :one
select * from mcp_schedules where id = $1;

-- name: InsertMCPSchedule :one
insert into mcp_schedules (name, prompt, cron, enabled, scopes, max_steps, created_by, next_run_at)
values ($1, $2, $3, $4, $5, $6, $7, $8) returning *;

-- name: UpdateMCPSchedule :one
update mcp_schedules set name = $2, prompt = $3, cron = $4, enabled = $5, scopes = $6, max_steps = $7, next_run_at = $8,
  updated_at = now()
where id = $1 returning *;

-- name: DeleteMCPSchedule :one
delete from mcp_schedules where id = $1 returning client_id;

-- name: SetMCPScheduleClient :exec
update mcp_schedules set client_id = $2 where id = $1;

-- name: DueMCPSchedules :many
-- Enabled schedules whose next run has come (or was never computed).
select * from mcp_schedules where enabled and (next_run_at is null or next_run_at <= $1) order by next_run_at nulls first;

-- name: AdvanceMCPSchedule :execrows
-- Moves next_run_at only if nobody else did (two ticks never both fire one slot).
update mcp_schedules set next_run_at = sqlc.arg(next) where id = sqlc.arg(id) and next_run_at is not distinct from sqlc.narg(prev);

-- name: InsertMCPScheduleRun :one
insert into mcp_schedule_runs (schedule_id, slot, trigger, triggered_by) values ($1, $2, $3, $4)
on conflict (schedule_id, slot) do nothing returning *;

-- name: FinishMCPScheduleRun :exec
update mcp_schedule_runs set status = $2, engine = $3, model = $4, report = $5, steps = $6, tokens_in = $7, tokens_out = $8,
  cost_idr = $9, error = $10, finished_at = $11
where id = $1;

-- name: TouchMCPScheduleRun :exec
update mcp_schedules set last_run_at = $2 where id = $1;

-- name: ListMCPScheduleRuns :many
select id, schedule_id, slot, trigger, triggered_by, status, engine, model, tokens_in, tokens_out, cost_idr, error, started_at, finished_at,
  jsonb_array_length(steps)::int as step_count, left(coalesce(report, ''), 280)::text as preview
from mcp_schedule_runs where schedule_id = $1 order by started_at desc limit $2;

-- name: LastMCPScheduleRuns :many
-- The latest run of every schedule (list view).
select distinct on (schedule_id) id, schedule_id, status, started_at, finished_at, cost_idr
from mcp_schedule_runs order by schedule_id, started_at desc;

-- name: GetMCPScheduleRun :one
select r.*, s.name as schedule_name from mcp_schedule_runs r join mcp_schedules s on s.id = r.schedule_id where r.id = $1;

-- name: FailStaleMCPScheduleRuns :exec
-- A run still "running" after the job timeout died with its worker.
update mcp_schedule_runs set status = 'error', error = 'Terputus (worker berhenti)', finished_at = now()
where status = 'running' and started_at < $1;

-- name: AnalystCostSince :one
select coalesce(sum(cost_idr), 0)::bigint as cost, count(*)::int as runs from mcp_schedule_runs where started_at >= $1;

-- name: InsertScheduleMCPClient :one
insert into mcp_clients (name, kind, scopes, owner_id) values ($1, 'schedule', $2, $3) returning *;

-- name: UpdateScheduleMCPClient :exec
update mcp_clients set name = $2, scopes = $3, active = true where id = $1;
