-- name: GetLoginUser :one
select id, email, name, role, password_hash, active from users where lower(email) = lower($1);

-- name: ListUsers :many
select u.id, u.email, u.name, u.role, u.active, u.password_hash is not null as has_password, s.name as sales_name, s.branch
from users u left join sales_users s on s.id = u.sales_user_id order by array_position(array['ceo','admin','finance','sales','warehouse'], u.role), u.name;

-- name: CreateUser :one
insert into users (email, name, role, password_hash, sales_user_id, active) values ($1, $2, $3, $4, $5, true) returning id;

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
