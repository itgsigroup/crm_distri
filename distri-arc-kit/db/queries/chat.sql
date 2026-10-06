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
-- chat_message_keys keeps wa_msg_id unique across monthly partitions; a known id inserts nothing (no rows).
with k as (
  insert into chat_message_keys (wa_msg_id, message_id, sent_at)
  values (coalesce(sqlc.narg(wa_msg_id)::text, 'local:' || gen_random_uuid()), gen_random_uuid(), sqlc.arg(sent_at)::timestamptz)
  on conflict (wa_msg_id) do nothing
  returning message_id, sent_at
)
insert into chat_messages (id, thread_id, wa_msg_id, direction, from_number, from_name, body, media, sent_at, annotation, signal_id, status, proposal_id, internal)
select k.message_id, sqlc.narg(thread_id)::uuid, sqlc.narg(wa_msg_id)::text, sqlc.narg(direction)::text, sqlc.narg(from_number)::text,
       sqlc.narg(from_name)::text, sqlc.narg(body)::text, sqlc.narg(media)::jsonb, k.sent_at, sqlc.narg(annotation)::jsonb,
       sqlc.narg(signal_id)::uuid, sqlc.arg(status)::text, sqlc.narg(proposal_id)::uuid, sqlc.arg(internal)::bool
from k
returning id;

-- name: UpdateChatMessageSent :exec
with k as (
  update chat_message_keys set wa_msg_id = sqlc.narg(wa_msg_id)::text, sent_at = sqlc.arg(sent_at)::timestamptz
  where message_id = sqlc.arg(id)::uuid
)
update chat_messages set wa_msg_id = sqlc.narg(wa_msg_id)::text, status = sqlc.arg(status)::text, sent_at = sqlc.arg(sent_at)::timestamptz
where id = sqlc.arg(id)::uuid;

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
with k as (
  insert into signal_keys (dedupe_key, signal_id, occurred_at)
  values (sqlc.arg(dedupe_key)::text, gen_random_uuid(), sqlc.arg(occurred_at)::timestamptz)
  on conflict (dedupe_key) do update set dedupe_key = excluded.dedupe_key
  returning signal_id, occurred_at, (xmax = 0) as fresh
), ins as (
  insert into signals (id, kind, dealer_id, contact_id, sales_id, occurred_at, dedupe_key, summary, payload)
  select k.signal_id, 'manual', sqlc.narg(dealer_id)::uuid, sqlc.narg(contact_id)::uuid, sqlc.narg(sales_id)::uuid,
         k.occurred_at, sqlc.arg(dedupe_key)::text, sqlc.narg(summary)::text, sqlc.arg(payload)::jsonb
  from k where k.fresh
  returning id
), upd as (
  update signals s set summary = sqlc.narg(summary)::text from k
  where not k.fresh and s.id = k.signal_id and s.occurred_at = k.occurred_at
  returning s.id
)
select k.signal_id as id from k;

-- name: SetMessageSignal :exec
update chat_messages set signal_id = $2 where id = $1;

-- name: UpsertIdentification :exec
insert into identifications (wa_number, sources, best_name, best_org, score) values ($1, $2, $3, $4, $5)
on conflict (wa_number) do update set sources = excluded.sources, best_name = excluded.best_name, best_org = excluded.best_org, score = excluded.score;

-- name: InsertAIDraftOrder :one
-- A sale order draft created in Odoo by Distri ARC (outbox odoo_so_draft); the Odoo sync updates it later.
insert into orders (dealer_id, number, state, ordered_at, total, margin_pct, lines, created_by, source_system, source_id, source_write_date)
values ($1, $2, 'order', $3, $4, $5, $6, 'ai_order_draft', 'odoo', $7, $3)
on conflict (source_system, source_id) do nothing
returning id;

-- name: InsertCommitment :one
insert into commitments (dealer_id, side, title, detail, status, due_at, invoice_id, proposal_id, signal_ids, source_key, created_at)
values ($1, $2, $3, $4, 'open', $5, $6, $7, $8, $9, $10)
on conflict (source_key) do update set detail = excluded.detail
returning id;

-- name: AdoptKamiCommitment :one
-- An executed SO draft takes over the open "Kirim …" promise sales already made in the chat (no proposal yet),
-- instead of listing the same delivery twice.
update commitments set proposal_id = sqlc.arg(proposal_id), detail = sqlc.arg(detail), due_at = coalesce(due_at, sqlc.narg(due_at)),
  signal_ids = array_append(signal_ids, sqlc.arg(signal_id)::uuid)
where id = (select c.id from commitments c where c.dealer_id = sqlc.arg(dealer_id) and c.side = 'kami' and c.status = 'open'
  and c.proposal_id is null and c.title ilike 'Kirim%' order by c.created_at limit 1)
returning id;

-- name: MarkLateCommitments :many
-- Open commitments past their date become late (the agents act on them).
update commitments set status = 'late' where status = 'open' and due_at < $1 returning id, dealer_id, side, title;

-- name: CloseCommitment :exec
update commitments set status = 'done' where id = $1;

-- name: OpenCommitments :many
select c.*, i.number as invoice_number from commitments c left join invoices i on i.id = c.invoice_id
where c.dealer_id = $1 and c.status <> 'done' order by c.due_at nulls last;

-- name: SetOutboxError :exec
update outbox set status = $2, attempts = attempts + 1, error = $3 where id = $1;

-- name: LastSentProposalInThread :one
-- The newest message Distri ARC sent from a proposal in a thread before a moment (reply tracking, 72 hours).
select m.proposal_id, m.sent_at, p.kind, p.title, p.payload from chat_messages m join proposals p on p.id = m.proposal_id
where m.thread_id = $1 and m.direction = 'out' and m.proposal_id is not null and m.sent_at <= sqlc.arg(before)::timestamptz
  and m.sent_at >= sqlc.arg(before)::timestamptz - interval '72 hours'
order by m.sent_at desc limit 1;

-- name: SetMessageReplyTo :exec
update chat_messages set proposal_id = $2 where id = $1;

-- name: SetSignalReply :exec
update signals set payload = payload || jsonb_build_object('reply_to', $2::text) where id = $1;

-- name: DealerOpenInvoiceList :many
select id, number, due_at, (total - coalesce(paid, 0))::bigint as residual from invoices
where dealer_id = $1 and total > coalesce(paid, 0) and state <> 'cancel' order by due_at;
