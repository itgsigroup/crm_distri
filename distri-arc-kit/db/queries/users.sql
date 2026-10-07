-- name: GetLoginUser :one
select id, email, name, role, password_hash, active, totp_secret, totp_enabled_at from users where lower(email) = lower($1);

-- name: ListUsers :many
select u.id, u.email, u.name, u.role, u.active, u.password_hash is not null as has_password, s.name as sales_name, s.branch,
  coalesce(u.role_key, u.role) as role_key, coalesce(r.name, u.role) as role_name, coalesce(r.wa_allowed, true) as wa_allowed,
  u.wa_number, n.state as wa_state, n.wa_number as wa_linked
from users u left join sales_users s on s.id = u.sales_user_id left join roles r on r.key = u.role_key
left join wa_numbers n on n.user_id = u.id
order by array_position(array['ceo','admin','finance','sales','warehouse'], u.role), u.name;

-- name: GetUserByID :one
select u.id, u.email, u.name, u.role, u.role_key, u.wa_number, u.sales_user_id, u.active, coalesce(r.wa_allowed, true) as wa_allowed
from users u left join roles r on r.key = u.role_key where u.id = $1;

-- name: SetUserProfile :exec
-- Role, WhatsApp number and branch of an account (null = unchanged; empty wa_number clears it).
update users set role_key = coalesce(sqlc.narg(role_key), role_key), role = coalesce(sqlc.narg(role), role),
  wa_number = case when sqlc.narg(wa_number)::text is null then wa_number when sqlc.narg(wa_number)::text = '' then null else sqlc.narg(wa_number)::text end
where id = sqlc.arg(id);

-- name: SetSalesProfileWA :exec
update sales_users set wa_number = nullif(sqlc.arg(wa_number)::text, ''), branch = coalesce(sqlc.narg(branch), branch), role = coalesce(sqlc.narg(role), role)
where id = sqlc.arg(id);

-- name: ListRoles :many
select r.*, (select count(*) from users u where u.role_key = r.key and u.active) as users
from roles r order by r.system desc, array_position(array['ceo','admin','finance','sales','warehouse'], r.base), r.name;

-- name: GetRole :one
select * from roles where key = $1;

-- name: UpsertRole :exec
insert into roles (key, name, description, base, screens, decide, wa_allowed, active, updated_by, updated_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
on conflict (key) do update set name = excluded.name, description = excluded.description, base = excluded.base,
  screens = excluded.screens, decide = excluded.decide, wa_allowed = excluded.wa_allowed, active = excluded.active,
  updated_by = excluded.updated_by, updated_at = now();

-- name: DeleteRole :execrows
delete from roles r where r.key = $1 and not r.system and not exists (select 1 from users u where u.role_key = r.key);

-- name: SyncRoleBase :exec
-- A role's base changed: its users follow (users.role is what every server check reads).
update users set role = $2 where role_key = $1;

-- name: CreateUser :one
insert into users (email, name, role, password_hash, sales_user_id, active, role_key, wa_number) values ($1, $2, $3, $4, $5, true, $6, $7) returning id;

-- name: UpdateUser :exec
update users set role = coalesce(sqlc.narg(role), role), active = coalesce(sqlc.narg(active), active),
  password_hash = coalesce(sqlc.narg(password_hash), password_hash), name = coalesce(sqlc.narg(name), name)
where id = sqlc.arg(id);

-- name: SetUserPassword :exec
update users set password_hash = $2 where lower(email) = lower($1);

-- name: UsersWithoutPassword :many
select email from users where password_hash is null;

-- name: SalesUserByName :one
select id from sales_users where lower(name) = lower($1) limit 1;

-- name: GetUserTOTP :one
select id, email, role, totp_secret, totp_enabled_at from users where id = $1;

-- name: SetTOTPSecret :exec
-- A new secret waits for its first code (totp_enabled_at stays null until then).
update users set totp_secret = $2, totp_enabled_at = null, totp_last_step = null where id = $1;

-- name: EnableTOTP :exec
update users set totp_enabled_at = $2, totp_last_step = $3 where id = $1;

-- name: UseTOTPStep :execrows
-- Accepts a step only once (a code cannot be replayed within its 90-second window).
update users set totp_last_step = $2 where id = $1 and coalesce(totp_last_step, 0) < $2;

-- name: DisableTOTP :exec
update users set totp_secret = null, totp_enabled_at = null, totp_last_step = null where id = $1;

-- name: ResetTOTPByEmail :execrows
update users set totp_secret = null, totp_enabled_at = null, totp_last_step = null where lower(email) = lower($1);

-- name: CreatePersonProfile :one
-- Every account gets a person profile (sales_users): decisions are recorded against it (proposals.decided_by).
insert into sales_users (name, branch, role, email) values ($1, $2, $3, $4) returning id;
