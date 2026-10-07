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
insert into roles (key, name, description, base, screens, decide, wa_allowed, active, scope, policies, updated_by, updated_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
on conflict (key) do update set name = excluded.name, description = excluded.description, base = excluded.base,
  screens = excluded.screens, decide = excluded.decide, wa_allowed = excluded.wa_allowed, active = excluded.active,
  scope = excluded.scope, policies = excluded.policies, updated_by = excluded.updated_by, updated_at = now();

-- name: DeleteRole :execrows
delete from roles r where r.key = $1 and not exists (select 1 from users u where u.role_key = r.key);

-- name: SyncRoleBase :exec
-- A role's derived base changed: its users and their person profiles follow (users.role is what the server's
-- data-scope checks read).
with u as (update users set role = $2 where role_key = $1 returning sales_user_id)
update sales_users s set role = $2 from u where s.id = u.sales_user_id;

-- name: PolicyHoldersExcept :one
-- Active users holding policy rights, not counting one role (being changed) or one user (being moved).
select count(*) from users u join roles r on r.key = u.role_key
where u.active and r.active and r.policies and r.key <> sqlc.arg(role_key)::text and u.id <> sqlc.arg(user_id)::uuid;

-- name: ListBranches :many
select b.*, (select count(*) from sales_users s join users u on u.sales_user_id = s.id where s.branch = b.name and u.active) as users,
  (select count(*) from dealers d where d.branch = b.name) as dealers
from branches b order by b.active desc, b.name;

-- name: GetBranch :one
select * from branches where id = $1;

-- name: BranchByName :one
select * from branches where lower(name) = lower($1);

-- name: InsertBranch :one
insert into branches (name, city, address, updated_by) values ($1, $2, $3, $4) returning id;

-- name: UpdateBranch :exec
update branches set name = $2, city = $3, address = $4, active = $5, updated_by = $6 where id = $1;

-- name: RenameBranchRefs :exec
-- A branch renamed in the master: people, dealers and source mappings follow.
with a as (update sales_users set branch = sqlc.arg(new_name)::text where branch = sqlc.arg(old_name)::text returning 1),
     b as (update dealers set branch = sqlc.arg(new_name)::text where branch = sqlc.arg(old_name)::text returning 1)
update data_mappings set target = sqlc.arg(new_name)::text where kind in ('branch','warehouse') and target = sqlc.arg(old_name)::text;

-- name: SetPersonBranch :exec
update sales_users set branch = $2 where id = $1;

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

-- name: EnsureBranches :exec
-- Branches in use by people and dealers join the branch master (seed, first install).
insert into branches (name)
select distinct b from (select branch as b from sales_users union select branch from dealers) x
where b is not null and b <> '' and b <> 'Semua cabang'
on conflict do nothing;

-- name: LinkNumbersToUsers :exec
-- Numbers whose holder is known by the user's own number (seed, older installs) are given to that user.
update wa_numbers n set user_id = u.id from users u where u.wa_number = n.wa_number and n.user_id is null;
