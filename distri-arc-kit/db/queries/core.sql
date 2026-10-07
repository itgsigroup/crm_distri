-- name: Ping :one
select 1::int as ok;

-- name: QueueDepth :one
select count(*)::bigint as depth from river_job where state in ('available', 'scheduled', 'retryable');

-- name: InsertAudit :exec
insert into audit_log (actor, actor_kind, action, entity, entity_id, before, after)
values ($1, $2, $3, $4, $5, $6, $7);

-- name: LastAudit :one
select * from audit_log where action = $1 order by created_at desc limit 1;

-- name: GetUserByEmail :one
select u.id, u.email, u.name, u.role, u.sales_user_id, s.branch, s.name as sales_name, u.totp_enabled_at,
  u.wa_number, r.key as role_key, r.name as role_name, r.screens as role_screens, r.decide as role_decide, coalesce(r.wa_allowed, true) as role_wa
from users u left join sales_users s on s.id = u.sales_user_id left join roles r on r.key = u.role_key
where lower(u.email) = lower($1) and u.active;

-- name: ListPolicies :many
select * from policies order by key;

-- name: GetPolicy :one
select * from policies where key = $1;

-- name: ListDealersBasic :many
select d.*, s.name as owner_name
from dealers d left join sales_users s on s.id = d.owner_id
order by d.name;

-- name: GetDealerBySlug :one
select d.*, s.name as owner_name
from dealers d left join sales_users s on s.id = d.owner_id
where d.slug = $1;

-- name: ListSalesUsers :many
select * from sales_users where active order by odoo_user_id nulls last, name;

-- name: ListRecentCycles :many
select * from cycles order by started_at desc limit $1;

-- name: ListProposalsByStatus :many
select * from proposals where status = $1 order by created_at desc limit $2;
