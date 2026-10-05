-- name: ListInternalNumbers :many
select * from internal_numbers order by label, wa_number;

-- name: IsInternalNumber :one
select exists(select 1 from internal_numbers where wa_number = $1)::bool as internal;

-- name: UpsertInternalNumber :exec
insert into internal_numbers (wa_number, label, department, is_sales, added_by)
values ($1, $2, $3, $4, $5)
on conflict (wa_number) do update set label = excluded.label, department = excluded.department, is_sales = excluded.is_sales;

-- name: DeleteInternalNumber :exec
delete from internal_numbers where wa_number = $1;

-- name: ListWAGroups :many
select * from wa_groups order by name;

-- name: GetWAGroupByJID :one
select * from wa_groups where jid = $1;

-- name: UpsertWAGroup :one
insert into wa_groups (jid, name, kind, branch, members, read_enabled)
values ($1, $2, $3, $4, $5, $6)
on conflict (jid) do update set name = coalesce(excluded.name, wa_groups.name), members = coalesce(excluded.members, wa_groups.members)
returning *;

-- name: UpdateWAGroup :one
update wa_groups set kind = $2, read_enabled = $3 where id = $1 returning *;

-- name: ListWANumbers :many
select n.*, s.name as sales_name, s.branch as sales_branch
from wa_numbers n left join sales_users s on s.id = n.sales_id order by s.name;

-- name: GetWANumber :one
select * from wa_numbers where wa_number = $1;

-- name: UpsertWANumber :exec
insert into wa_numbers (wa_number, sales_id, label, transport, state)
values ($1, $2, $3, $4, $5)
on conflict (wa_number) do update set sales_id = excluded.sales_id, label = excluded.label, transport = excluded.transport;

-- name: SetWANumberState :exec
update wa_numbers set state = $2, jid = coalesce($3, jid), last_seen_at = now(),
  paired_at = case when $2 = 'connected' and paired_at is null then now() else paired_at end, updated_at = now()
where wa_number = $1;

-- name: SetWANumberQR :exec
update wa_numbers set qr = $2, qr_expires_at = $3, state = 'pairing', updated_at = now() where wa_number = $1;

-- name: FindContactByNumber :one
select c.*, d.slug as dealer_slug, d.name as dealer_name, d.owner_id as dealer_owner
from contacts c join dealers d on d.id = c.dealer_id where c.wa_number = $1 limit 1;

-- name: GetSalesByNumber :one
select * from sales_users where wa_number = $1;

-- name: GetThreadBySalesJID :one
select * from chat_threads where wa_jid = $1 and sales_id is not distinct from $2;

-- name: InsertThread :one
insert into chat_threads (kind, dealer_id, group_id, contact_id, wa_jid, title, subtitle, sales_id, last_message_at, unread, identification, tag, suggestions, seed_key)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
on conflict (seed_key) do update set title = excluded.title, subtitle = excluded.subtitle, tag = excluded.tag,
  suggestions = excluded.suggestions, identification = excluded.identification, unread = excluded.unread,
  dealer_id = excluded.dealer_id, contact_id = excluded.contact_id, group_id = excluded.group_id
returning *;

-- name: TouchThread :exec
update chat_threads set last_message_at = greatest(coalesce(last_message_at, $2), $2),
  unread = unread + $3 where id = $1;

-- name: MarkThreadRead :exec
update chat_threads set unread = 0 where id = $1;

-- name: InsertChatMessage :one
insert into chat_messages (thread_id, wa_msg_id, direction, from_number, from_name, body, media, sent_at, annotation, signal_id, status, proposal_id, internal)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
on conflict (wa_msg_id) do nothing
returning id;

-- name: UpdateChatMessageSent :exec
update chat_messages set wa_msg_id = $2, status = $3, sent_at = $4 where id = $1;

-- name: ListThreads :many
select t.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, g.kind as group_kind,
  (select m.body from chat_messages m where m.thread_id = t.id order by m.sent_at desc limit 1) as last_body,
  (select m.from_name from chat_messages m where m.thread_id = t.id order by m.sent_at desc limit 1) as last_from
from chat_threads t
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
left join wa_groups g on g.id = t.group_id
order by case t.kind when 'dealer' then 0 when 'group' then 1 else 2 end, t.last_message_at desc nulls last;

-- name: GetThread :one
select t.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, s.wa_number as sales_wa
from chat_threads t
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
where t.id = $1;

-- name: ListThreadMessages :many
select * from (
  select * from chat_messages where thread_id = $1 and sent_at < $2 order by sent_at desc limit $3
) x order by sent_at;

-- name: LatestThreadSignal :one
select signal_id from chat_messages where thread_id = $1 and signal_id is not null order by sent_at desc limit 1;

-- name: TouchContact :exec
update contacts set last_interaction_at = greatest(coalesce(last_interaction_at, $2), $2),
  interactions_90d = interactions_90d + 1 where id = $1;

-- name: InsertProposal :one
insert into proposals (cycle_id, agent, dealer_id, kind, title, why, prep, preview, steps, impact, confidence, signal_ids,
  autonomy, status, due_label, decided_by, decided_at, decision_reason, edited_payload)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
returning *;

-- name: InsertOutbox :one
insert into outbox (proposal_id, channel, to_ref, payload) values ($1, $2, $3, $4)
on conflict (proposal_id, channel) do update set payload = outbox.payload
returning *;

-- name: GetOutbox :one
select * from outbox where id = $1;

-- name: SetOutboxResult :exec
update outbox set status = $2, attempts = attempts + 1, sent_at = $3, error = $4 where id = $1;

-- name: CountSentToday :one
select count(*)::bigint from outbox
where channel = 'wa' and status = 'sent' and payload->>'from' = sqlc.arg(from_number)::text and sent_at >= sqlc.arg(since);

-- name: LastSentAt :one
select coalesce(max(sent_at), 'epoch'::timestamptz)::timestamptz as last from outbox
where channel = 'wa' and status = 'sent' and payload->>'from' = sqlc.arg(from_number)::text;

-- name: SetProposalExecuted :exec
update proposals set status = 'executed', executed_at = now() where id = $1;

-- name: InsertManualSignal :one
insert into signals (kind, dealer_id, contact_id, sales_id, occurred_at, dedupe_key, summary, payload)
values ('manual', $1, $2, $3, $4, $5, $6, $7)
on conflict (dedupe_key) do update set summary = excluded.summary
returning id;

-- name: SetMessageSignal :exec
update chat_messages set signal_id = $2 where id = $1;

-- name: UpsertIdentification :exec
insert into identifications (wa_number, sources, best_name, best_org, score) values ($1, $2, $3, $4, $5)
on conflict (wa_number) do update set sources = excluded.sources, best_name = excluded.best_name, best_org = excluded.best_org, score = excluded.score;
