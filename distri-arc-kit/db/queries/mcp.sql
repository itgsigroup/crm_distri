-- name: InsertMCPClient :one
insert into mcp_clients (id, name, kind, token_hash, scopes, owner_id, token_prefix) values ($1, $2, $3, $4, $5, $6, $7) returning *;

-- name: GetMCPClient :one
select * from mcp_clients where id = $1;

-- name: ListMCPClients :many
select c.*, (select count(*) from mcp_calls m where m.client_id = c.id and m.created_at >= sqlc.arg(since)::timestamptz)::bigint as calls_today
from mcp_clients c order by c.created_at;

-- name: TouchMCPClient :exec
update mcp_clients set last_seen_at = $2 where id = $1;

-- name: RevokeMCPClient :exec
update mcp_clients set active = false where id = $1;

-- name: InsertMCPCall :exec
insert into mcp_calls (client_id, tool, args, result_summary, cycle_id, duration_ms, status, created_at) values ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListMCPCalls :many
select m.*, c.name as client_name from mcp_calls m left join mcp_clients c on c.id = m.client_id order by m.created_at desc limit $1;

-- name: MCPCyclesSince :one
-- Cycles an MCP client started in the window (orchestrator.* rate limit = mcp.permissions.max_cycles_per_hour).
select count(*)::bigint from cycles where trigger = 'mcp' and requested_by = $1 and started_at >= sqlc.arg(since)::timestamptz;

-- name: UpsertCycleInput :exec
insert into cycle_inputs (cycle_id, agent, input, signal_ids, mapping) values ($1, $2, $3, $4, $5)
on conflict (cycle_id, agent) do update set input = excluded.input, signal_ids = excluded.signal_ids, mapping = excluded.mapping;

-- name: GetCycleInput :one
select * from cycle_inputs where cycle_id = $1 and agent = $2;

-- name: SubmitCycleInput :exec
update cycle_inputs set submitted = $3, submitted_by = $4, submitted_at = $5 where cycle_id = $1 and agent = $2;

-- name: ListCycleInputs :many
select * from cycle_inputs where cycle_id = $1 order by agent;
