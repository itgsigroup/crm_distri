-- AI usage on the MCP Claude page: model, cost, schedule and history of every AI analysis.

-- name: LLMUsageSince :one
select count(*)::bigint as calls, coalesce(sum(tokens_in), 0)::bigint as tokens_in, coalesce(sum(tokens_out), 0)::bigint as tokens_out,
  coalesce(sum(cost_idr), 0)::bigint as cost_idr
from llm_calls where created_at >= sqlc.arg(since)::timestamptz;

-- name: LLMUsageByModel :many
select coalesce(provider, '') as provider, coalesce(model, '') as model, count(*)::bigint as calls,
  coalesce(sum(tokens_in), 0)::bigint as tokens_in, coalesce(sum(tokens_out), 0)::bigint as tokens_out, coalesce(sum(cost_idr), 0)::bigint as cost_idr,
  max(created_at)::timestamptz as last_at
from llm_calls where created_at >= sqlc.arg(since)::timestamptz
group by 1, 2 order by cost_idr desc, calls desc;

-- name: LLMUsageDaily :many
-- Cost per WIB day (the chart of the last days).
select (created_at at time zone 'Asia/Jakarta')::date as day, count(*)::bigint as calls, coalesce(sum(cost_idr), 0)::bigint as cost_idr
from llm_calls where created_at >= sqlc.arg(since)::timestamptz
group by 1 order by 1;

-- name: CycleAIUsage :many
-- Orchestrator cycles with the model calls they made.
select c.id, c.number, c.trigger, c.scope, coalesce(c.via, '') as via, coalesce(c.requested_by, '') as requested_by, c.status,
  c.started_at, c.duration_ms,
  coalesce(l.calls, 0)::bigint as calls, coalesce(l.tokens_in, 0)::bigint as tokens_in, coalesce(l.tokens_out, 0)::bigint as tokens_out,
  coalesce(l.cost_idr, 0)::bigint as cost_idr, coalesce(l.models, '')::text as models
from cycles c
left join (select cycle_id, count(*) as calls, sum(tokens_in) as tokens_in, sum(tokens_out) as tokens_out, sum(cost_idr) as cost_idr,
             string_agg(distinct model, ', ') as models
           from llm_calls where cycle_id is not null group by cycle_id) l on l.cycle_id = c.id
order by c.started_at desc limit sqlc.arg(lim);

-- name: ScheduleAIUsage :many
-- Scheduled MCP analyses (Analisis terjadwal) with their model and cost.
select r.id, r.schedule_id, s.name, r.trigger, coalesce(r.triggered_by, '') as triggered_by, r.status, coalesce(r.engine, '') as engine,
  coalesce(r.model, '') as model, r.tokens_in, r.tokens_out, r.cost_idr, r.started_at, r.finished_at,
  jsonb_array_length(r.steps)::int as steps
from mcp_schedule_runs r join mcp_schedules s on s.id = r.schedule_id
order by r.started_at desc limit sqlc.arg(lim);
