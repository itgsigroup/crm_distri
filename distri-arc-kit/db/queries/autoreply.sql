-- name: ClaimAutoReply :execrows
-- The message is answered once: a second run of its job finds the row and stops.
insert into auto_replies (message_id, thread_id, kind, break_start) values ($1, $2, $3, $4) on conflict (message_id) do nothing;

-- name: SetAutoReplySent :exec
update auto_replies set proposal_id = $2, reply_id = $3 where message_id = $1;

-- name: CountAutoReplies :one
-- Automatic answers of a kind in a conversation during one break.
select count(*) from auto_replies where thread_id = $1 and break_start = $2 and kind = $3 and reply_id is not null;

-- name: HumanRepliedSince :one
-- A person answered the conversation since: from the phone (no proposal) or from Chat — not an automatic answer.
select exists (
  select 1 from chat_messages m left join proposals p on p.id = m.proposal_id
  where m.thread_id = $1 and m.direction = 'out' and m.sent_at >= $2 and m.status <> 'failed'
    and coalesce(p.agent, '') <> 'Balas otomatis'
)::bool;

-- name: IsAutoReplyText :one
-- The text counts as an automatic message (chat stays "Belum dibalas"), as chat_messages.auto_reply computes it.
select is_auto_reply($1::text)::bool;

-- name: ContactTypeOf :one
select coalesce((select type from contact_types where wa_number = $1), '')::text;

-- name: RecentAutoReplies :many
select a.message_id, a.kind, a.created_at, t.id as thread_id, coalesce(t.title, '')::text as title, coalesce(t.account, '')::text as account,
  coalesce(m.body, '')::text as question, coalesce(r.body, '')::text as answer, coalesce(r.status, 'skipped')::text as status
from auto_replies a
  join chat_threads t on t.id = a.thread_id
  join chat_messages m on m.id = a.message_id
  left join chat_messages r on r.id = a.reply_id
order by a.created_at desc
limit $1;
