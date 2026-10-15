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

-- name: TrimPins :exec
-- WhatsApp keeps three pins: a fourth replaces the oldest.
delete from chat_pins where conv = sqlc.arg(conv)::text and message_id not in
  (select x.message_id from chat_pins x where x.conv = sqlc.arg(conv)::text order by x.pinned_at desc limit sqlc.arg(keep)::int);

-- name: DeletePin :exec
delete from chat_pins where conv = $1 and message_id = $2;
