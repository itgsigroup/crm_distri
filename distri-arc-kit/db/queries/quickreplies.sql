-- name: ListQuickReplies :many
-- The team's quick replies, most used first (then by shortcut).
select id, shortcut, body, created_by, created_by_name, uses, updated_at from quick_replies
order by uses desc, shortcut;

-- name: CountQuickReplies :one
select count(*)::int from quick_replies;

-- name: InsertQuickReply :one
insert into quick_replies (shortcut, body, created_by, created_by_name)
values (sqlc.arg(shortcut)::text, sqlc.arg(body)::text, sqlc.narg(created_by)::uuid, sqlc.arg(created_by_name)::text)
returning id, shortcut, body, created_by, created_by_name, uses, updated_at;

-- name: GetQuickReplyOwner :one
select created_by from quick_replies where id = $1;

-- name: UpdateQuickReply :execrows
update quick_replies set shortcut = sqlc.arg(shortcut)::text, body = sqlc.arg(body)::text, updated_at = now()
where id = sqlc.arg(id)::uuid;

-- name: DeleteQuickReply :execrows
delete from quick_replies where id = $1;

-- name: UseQuickReply :execrows
-- A quick reply put in the box: counted so the most used come first.
update quick_replies set uses = uses + 1, last_used_at = now() where id = $1;
