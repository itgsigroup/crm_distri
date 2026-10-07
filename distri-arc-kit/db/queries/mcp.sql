-- name: InsertMCPClient :one
insert into mcp_clients (id, name, kind, token_hash, scopes, owner_id, token_prefix) values ($1, $2, $3, $4, $5, $6, $7) returning *;

-- name: GetMCPClient :one
select * from mcp_clients where id = $1;

-- name: ListMCPClients :many
select c.*, (select count(*) from mcp_calls m where m.client_id = c.id and m.created_at >= sqlc.arg(since)::timestamptz)::bigint as calls_today,
  u.name as user_name, u.email as user_email
from mcp_clients c left join users u on u.id = c.user_id order by c.created_at;

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

-- name: InsertOAuthClient :one
insert into oauth_clients (client_id, client_name, redirect_uris) values ($1, $2, $3) returning *;

-- name: GetOAuthClient :one
select * from oauth_clients where client_id = $1;

-- name: CountOAuthClientsSince :one
select count(*)::bigint from oauth_clients where created_at >= $1;

-- name: InsertOAuthRequest :exec
insert into oauth_requests (id, client_id, redirect_uri, state, code_challenge, scopes, resource, expires_at)
values ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetOAuthRequest :one
select r.*, c.client_name from oauth_requests r join oauth_clients c using (client_id) where r.id = $1 and r.expires_at > now();

-- name: DeleteOAuthRequest :exec
delete from oauth_requests where id = $1;

-- name: PurgeOAuth :exec
-- Expired requests and codes go (called on every authorize).
with a as (delete from oauth_requests where expires_at < now() returning 1)
delete from oauth_codes where expires_at < now() - interval '1 day';

-- name: InsertOAuthCode :exec
insert into oauth_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, scopes, expires_at)
values ($1, $2, $3, $4, $5, $6, $7);

-- name: UseOAuthCode :one
-- A code works once, before it expires.
update oauth_codes set used_at = now() where code_hash = $1 and used_at is null and expires_at > now() returning *;

-- name: InsertOAuthConnection :one
insert into mcp_clients (id, name, kind, token_hash, scopes, owner_id, token_prefix, user_id, oauth_client_id, expires_at, refresh_hash, refresh_expires_at)
values ($1, $2, 'oauth', $3, $4, $5, $6, $7, $8, $9, $10, $11) returning *;

-- name: GetMCPClientByRefresh :one
select * from mcp_clients where refresh_hash = $1 and active and kind = 'oauth' and refresh_expires_at > now();

-- name: RotateMCPToken :exec
update mcp_clients set token_hash = $2, token_prefix = $3, expires_at = $4, refresh_hash = $5, refresh_expires_at = $6, last_seen_at = now()
where id = $1;

-- name: TouchOAuthClient :exec
update oauth_clients set last_used_at = now() where client_id = $1;
