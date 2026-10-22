-- name: ListChatLabels :many
-- The team's labels with how many conversations carry each.
select l.id, l.name, l.color, l.position,
  (select count(*) from chat_conv_labels c where c.label_id = l.id)::int as chats
from chat_labels l order by l.position, l.name;

-- name: InsertChatLabel :one
insert into chat_labels (name, color, position, created_by)
values (sqlc.arg(name)::text, sqlc.arg(color)::text, (select coalesce(max(position), 0) + 1 from chat_labels), sqlc.narg(created_by)::text)
returning id, name, color, position;

-- name: UpdateChatLabel :execrows
update chat_labels set name = sqlc.arg(name)::text, color = sqlc.arg(color)::text where id = sqlc.arg(id)::uuid;

-- name: DeleteChatLabel :execrows
delete from chat_labels where id = $1;

-- name: AllConvLabels :many
-- Every conversation's labels (the chat list shows and filters by them).
select conv, label_id from chat_conv_labels;

-- name: ConvLabels :many
select label_id from chat_conv_labels where conv = $1 order by added_at;

-- name: DeleteConvLabelsExcept :exec
delete from chat_conv_labels where conv = sqlc.arg(conv)::text and not (label_id = any(sqlc.arg(keep)::uuid[]));

-- name: AddConvLabel :exec
insert into chat_conv_labels (conv, label_id, added_by) values (sqlc.arg(conv)::text, sqlc.arg(label_id)::uuid, sqlc.narg(added_by)::text)
on conflict do nothing;

-- name: MessageLabels :many
select message_id, label_id from chat_message_labels where message_id = any(sqlc.arg(ids)::uuid[]) order by added_at;

-- name: DeleteMessageLabelsExcept :exec
delete from chat_message_labels where message_id = sqlc.arg(message_id)::uuid and not (label_id = any(sqlc.arg(keep)::uuid[]));

-- name: AddMessageLabel :exec
insert into chat_message_labels (message_id, sent_at, conv, label_id, added_by)
values (sqlc.arg(message_id)::uuid, sqlc.arg(sent_at)::timestamptz, sqlc.arg(conv)::text, sqlc.arg(label_id)::uuid, sqlc.narg(added_by)::text)
on conflict do nothing;

-- name: ListPins :many
-- A conversation's pinned messages, last pinned first (the bar above the chat).
select p.message_id, p.sent_at, p.pinned_by, p.pinned_at, coalesce(m.body, '')::text as body, coalesce(m.from_name, '')::text as from_name,
  coalesce(m.direction, '')::text as direction, m.media
from chat_pins p join chat_messages m on m.id = p.message_id and m.sent_at = p.sent_at
where p.conv = $1 and m.deleted_at is null
order by p.pinned_at desc;

-- name: InsertPin :exec
insert into chat_pins (conv, message_id, sent_at, pinned_by) values (sqlc.arg(conv)::text, sqlc.arg(message_id)::uuid, sqlc.arg(sent_at)::timestamptz, sqlc.narg(pinned_by)::text)
on conflict (conv, message_id) do update set pinned_at = now(), pinned_by = excluded.pinned_by;

-- name: DeletePin :exec
delete from chat_pins where conv = $1 and message_id = $2;

-- name: StarMessage :exec
-- Pesan berbintang: one person's star on a message (again = nothing).
insert into chat_message_stars (user_id, message_id, thread_id) values (sqlc.arg(user_id)::uuid, sqlc.arg(message_id)::uuid, sqlc.arg(thread_id)::uuid)
on conflict (user_id, message_id) do nothing;

-- name: UnstarMessage :exec
delete from chat_message_stars where user_id = sqlc.arg(user_id)::uuid and message_id = sqlc.arg(message_id)::uuid;

-- name: UserStarsIn :many
-- Which of these messages the person starred (the bubbles of an open chat).
select message_id from chat_message_stars where user_id = sqlc.arg(user_id)::uuid and message_id = any(sqlc.arg(ids)::uuid[]);

-- name: ListStarred :many
-- The person's starred messages, newest message first, with their conversation; the API checks who may read which.
select s.message_id, s.created_at as starred_at, m.thread_id, m.sent_at, coalesce(m.direction, '')::text as direction,
  coalesce(m.from_name, '')::text as from_name, coalesce(m.body, '')::text as body, (m.media is not null and m.media <> 'null'::jsonb)::bool as has_media,
  coalesce(t.kind, '')::text as kind, coalesce(t.title, '')::text as title, coalesce(t.phone_name, '')::text as phone_name,
  t.sales_id, t.account, coalesce(n.team, 'sales')::text as account_team, coalesce(g.kind, '')::text as group_kind
from chat_message_stars s
join chat_messages m on m.id = s.message_id
join chat_threads t on t.id = m.thread_id
left join wa_numbers n on n.wa_number = t.account
left join wa_groups g on g.id = t.group_id
where s.user_id = sqlc.arg(user_id)::uuid and m.deleted_at is null
order by m.sent_at desc
limit 500;
