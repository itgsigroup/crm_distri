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
-- A person chose the kind: the automatic customer-group rule no longer changes it.
update wa_groups set kind = $2, read_enabled = $3, kind_manual = true where id = $1 returning *;

-- name: GetWAGroup :one
select * from wa_groups where id = $1;

-- name: MarkCustomerGroup :exec
-- The automatic rule: a customer of the master is in the group — it becomes a customer group, unless a person chose.
update wa_groups set kind = 'external' where id = $1 and kind = 'internal' and not kind_manual;

-- name: AnyCustomerNumber :one
-- Whether one of the numbers is a customer contact of the master (a group with them is a customer group).
select exists(select 1 from contacts where wa_number = any(sqlc.arg(numbers)::text[]))::bool;

-- name: ListWANumbers :many
-- Every number with who holds it (user, role, branch) and its chats (count, unread, last activity).
select n.*, s.name as sales_name, s.branch as sales_branch, u.name as user_name, u.email as user_email,
  r.name as user_role, us.branch as user_branch, c.thread_count, c.unread_count, c.last_message_at
from wa_numbers n left join sales_users s on s.id = n.sales_id left join users u on u.id = n.user_id
  left join roles r on r.key = coalesce(u.role_key, u.role) left join sales_users us on us.id = u.sales_user_id
  left join lateral (select count(*)::int as thread_count, coalesce(sum(t.unread), 0)::int as unread_count, max(t.last_message_at) as last_message_at
    from chat_threads t where t.account = n.wa_number) c on true
order by coalesce(u.name, s.name) nulls last, n.label, n.created_at;

-- name: GetWANumberByUser :one
select * from wa_numbers where user_id = $1;

-- name: GetWANumberBySession :one
select * from wa_numbers where session_id = $1;

-- name: LinkWANumber :exec
-- A linked device reported its number: the number is registered (or re-linked) under the link's session, in the
-- team chosen for the link (a re-linked number keeps its team).
insert into wa_numbers (wa_number, session_id, label, transport, state, jid, paired_at, last_seen_at, team)
values ($1, $2, $3, $4, 'connected', $5, now(), now(), coalesce((select l.team from wa_links l where l.session_id = $2), 'sales'))
on conflict (wa_number) do update set session_id = excluded.session_id, transport = excluded.transport, state = 'connected',
  jid = coalesce(excluded.jid, wa_numbers.jid), qr = null, paired_at = coalesce(wa_numbers.paired_at, now()), last_seen_at = now(), updated_at = now();

-- name: SetWANumberUser :exec
-- Gives a linked number to a user (one user, one number; null releases it).
update wa_numbers set user_id = sqlc.narg(user_id), sales_id = sqlc.narg(sales_id), label = sqlc.narg(label), updated_at = now()
where wa_number = sqlc.arg(wa_number);

-- name: AssignThreadsSales :exec
-- Conversations of a number follow the sales who holds it.
update chat_threads set sales_id = sqlc.narg(sales_id) where account = sqlc.arg(account);

-- name: InsertWALink :exec
insert into wa_links (session_id, method, phone, created_by, team) values ($1, $2, $3, $4, $5);

-- name: SetWANumberTeam :exec
update wa_numbers set team = $2, updated_at = now() where wa_number = $1;

-- name: GetWALink :one
select * from wa_links where session_id = $1;

-- name: SetWALink :exec
update wa_links set state = $2, qr = $3, wa_number = coalesce(sqlc.narg(wa_number), wa_number), error = sqlc.narg(error), updated_at = now()
where session_id = $1;

-- name: SetUserWANumber :exec
update users set wa_number = sqlc.narg(wa_number) where id = sqlc.arg(id);

-- name: ClearUserWANumber :exec
update users set wa_number = null where wa_number = $1;

-- name: GetWANumber :one
select * from wa_numbers where wa_number = $1;

-- name: UpsertWANumber :exec
insert into wa_numbers (wa_number, sales_id, label, transport, state, user_id, session_id)
values ($1, $2, $3, $4, $5, $6, $1)
on conflict (wa_number) do update set sales_id = excluded.sales_id, label = excluded.label, transport = excluded.transport,
  user_id = coalesce(excluded.user_id, wa_numbers.user_id);

-- name: SetWANumberState :exec
update wa_numbers set state = $2, jid = coalesce($3, jid), last_seen_at = now(),
  paired_at = case when $2 = 'connected' and paired_at is null then now() else paired_at end, updated_at = now(),
  state_since = case when state is distinct from $2 then now() else state_since end
where wa_number = $1;

-- name: SetWANumberQR :exec
update wa_numbers set qr = $2, qr_expires_at = $3, state = 'pairing', updated_at = now(),
  state_since = case when state is distinct from 'pairing' then now() else state_since end where wa_number = $1;

-- name: FindContactByNumber :one
select c.*, d.slug as dealer_slug, d.name as dealer_name, d.owner_id as dealer_owner
from contacts c join dealers d on d.id = c.dealer_id where c.wa_number = $1 limit 1;

-- name: GetSalesByNumber :one
select * from sales_users where wa_number = $1;

-- name: GetThreadByAccountJID :one
-- A conversation belongs to the WhatsApp number that holds it (many numbers per sales, team numbers).
select * from chat_threads where wa_jid = sqlc.arg(wa_jid) and account is not distinct from sqlc.narg(account);

-- name: InsertThread :one
insert into chat_threads (kind, dealer_id, group_id, contact_id, wa_jid, title, subtitle, sales_id, last_message_at, unread, identification, tag, suggestions, seed_key, account)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
insert into chat_messages (id, thread_id, wa_msg_id, direction, from_number, from_name, body, media, sent_at, annotation, signal_id, status, proposal_id, internal, quoted_wa_id)
select k.message_id, sqlc.narg(thread_id)::uuid, sqlc.narg(wa_msg_id)::text, sqlc.narg(direction)::text, sqlc.narg(from_number)::text,
       sqlc.narg(from_name)::text, sqlc.narg(body)::text, sqlc.narg(media)::jsonb, k.sent_at, sqlc.narg(annotation)::jsonb,
       sqlc.narg(signal_id)::uuid, sqlc.arg(status)::text, sqlc.narg(proposal_id)::uuid, sqlc.arg(internal)::bool, sqlc.narg(quoted_wa_id)::text
from k
returning id;

-- name: UpdateChatMessageSent :exec
with k as (
  update chat_message_keys set wa_msg_id = sqlc.narg(wa_msg_id)::text, sent_at = sqlc.arg(sent_at)::timestamptz
  where message_id = sqlc.arg(id)::uuid
)
update chat_messages set wa_msg_id = sqlc.narg(wa_msg_id)::text, status = sqlc.arg(status)::text, sent_at = sqlc.arg(sent_at)::timestamptz,
  -- the text that actually went out (an edit made while it was queued may have come too late)
  body = coalesce(sqlc.narg(body)::text, body)
where id = sqlc.arg(id)::uuid;

-- name: ListThreads :many
select t.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, g.kind as group_kind,
  coalesce(n.label, s.name, t.account) as account_label, coalesce(n.team, 'sales')::text as account_team,
  -- a group's last message may have been stored under another GSI number's thread of the same group
  lm.body as last_body, lm.from_name as last_from, lm.direction as last_direction,
  -- the oldest customer message not yet answered: how long the conversation has been waiting for a reply
  (select min(m.sent_at) from chat_messages m where m.thread_id = t.id and m.direction = 'in'
     and m.sent_at > coalesce((select max(o.sent_at) from chat_messages o where o.thread_id = t.id and o.direction = 'out'), '-infinity'::timestamptz)) as waiting_since
from chat_threads t
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
left join wa_groups g on g.id = t.group_id
left join wa_numbers n on n.wa_number = t.account
left join lateral (
  select u.body, u.from_name, u.direction from (
    (select m.body, m.from_name, m.direction, m.sent_at from chat_messages m where m.thread_id = t.id order by m.sent_at desc limit 1)
    union all
    (select o.body, o.from_name, o.direction, o.sent_at from chat_threads x
       cross join lateral (select m.body, m.from_name, m.direction, m.sent_at from chat_messages m
                           where m.thread_id = x.id order by m.sent_at desc limit 1) o
     where t.kind = 'group' and x.wa_jid = t.wa_jid and x.id <> t.id)
  ) u order by u.sent_at desc limit 1
) lm on true
-- newest conversation first, like WhatsApp (the kind tabs filter; ordering by kind buried today's chats)
order by t.last_message_at desc nulls last, t.id;

-- name: GetThread :one
-- sales_wa is the number replies go out from: the thread's own number, else its sales' main number.
select t.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, coalesce(t.account, s.wa_number) as sales_wa,
  coalesce(n.label, s.name, t.account) as account_label, d.source_id as dealer_code, d.city as dealer_city,
  coalesce(n.team, 'sales')::text as account_team, g.kind as group_kind,
  (select min(m.sent_at) from chat_messages m where m.thread_id = t.id and m.direction = 'in'
     and m.sent_at > coalesce((select max(o.sent_at) from chat_messages o where o.thread_id = t.id and o.direction = 'out'), '-infinity'::timestamptz)) as waiting_since
from chat_threads t
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
left join wa_numbers n on n.wa_number = t.account
left join wa_groups g on g.id = t.group_id
where t.id = $1;

-- name: ListThreadMessages :many
-- A group is one conversation for every GSI number in it: each message is stored once (under the number that
-- received it first), so a group thread shows the messages of all threads of that group.
select * from (
  select m.* from chat_messages m
  where m.thread_id in (select x.id from chat_threads x join chat_threads t on t.id = sqlc.narg(thread_id)::uuid
                        where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
    and m.sent_at < sqlc.arg(sent_at) order by m.sent_at desc limit sqlc.arg(lim)
) x order by sent_at;

-- name: GroupThreadAccounts :many
-- The GSI numbers holding a group's messages (label shown on a message another number received or sent).
select x.id, x.account, coalesce(n.label, s.name, x.account) as account_label
from chat_threads x join chat_threads t on t.id = $1 and t.kind = 'group' and x.wa_jid = t.wa_jid
left join wa_numbers n on n.wa_number = x.account left join sales_users s on s.id = x.sales_id;

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
-- A sale order draft created in Odoo by GSI Orbit (outbox odoo_so_draft); the Odoo sync updates it later.
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
-- The newest message GSI Orbit sent from a proposal in a thread before a moment (reply tracking, 72 hours).
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

-- name: ShadowOutbox :execrows
-- Pilot shadow mode: an approved proposal's rows are kept for the record but never delivered.
update outbox set status = 'shadow', error = 'mode bayangan pilot: tidak dikirim' where proposal_id = $1 and status = 'pending';

-- name: DropPendingBubble :exec
-- The pending chat bubble of a row that will not be sent (shadow mode) is removed with its dedupe key.
with gone as (delete from chat_messages where proposal_id = $1 and status = 'pending' and direction = 'out' returning id)
delete from chat_message_keys k using gone where k.message_id = gone.id;

-- name: PendingOutboxIDs :many
-- Rows waiting for delivery (pilot rehearsal; the worker uses one job per row).
select id from outbox where status in ('pending', 'failed') order by created_at;

-- name: DeleteWANumber :exec
-- Removes a team number (no sales owner) after it was unlinked; a sales' main number stays as "unpaired".
delete from wa_numbers where wa_number = $1 and sales_id is null;

-- name: SetWANumberUnpaired :exec
update wa_numbers set state = 'unpaired', qr = null, qr_expires_at = null, jid = null, paired_at = null, updated_at = now() where wa_number = $1;

-- name: OutboxApprovedForSend :one
-- The bridge asks before every send: the row must be a WhatsApp row of a proposal a person (or the system within
-- policy) approved, and not delivered yet (CLAUDE.md §2 — nothing reaches a dealer without a recorded decision).
-- No exception: system messages (alerts) are never sent (Sam, 12 Oct: no automatic message to WhatsApp).
select exists(select 1 from outbox o join proposals p on p.id = o.proposal_id
  where o.id = $1 and o.channel = 'wa' and o.status in ('pending', 'failed')
    and p.decided_at is not null and p.status in ('approved', 'edited', 'executed'))::bool as approved;

-- name: SetChatMessageStatus :exec
update chat_messages set status = sqlc.arg(status)::text where id = sqlc.arg(id)::uuid;

-- name: SetWANumbersTransport :exec
update wa_numbers set transport = $1, updated_at = now() where transport <> $1;

-- name: NameNewThread :exec
-- A conversation with an unknown number shows the contact's name once the phone knows it (until then the masked
-- number). Dealer and group threads keep their own titles.
update chat_threads set title = $2 where id = $1 and kind = 'new' and title is distinct from $2;

-- name: HistoryBackfillCandidates :many
-- Conversations of connected numbers whose kept history may not reach a year back yet (wa.backfill): each with its
-- oldest WhatsApp message, the anchor the phone is asked to sync history before. Busiest first.
select t.id, t.account, t.wa_jid, t.history_req_oldest, t.history_tries,
  o.wa_msg_id as oldest_wa_id, o.direction as oldest_direction, o.sent_at as oldest_at
from chat_threads t
join wa_numbers n on n.wa_number = t.account and n.state = 'connected'
cross join lateral (
  select m.wa_msg_id, m.direction, m.sent_at from chat_messages m
  where m.thread_id = t.id and m.wa_msg_id is not null and m.wa_msg_id not like 'local:%'
  order by m.sent_at, m.id limit 1
) o
where t.history_done_at is null and t.wa_jid is not null and t.account is not null
  and (t.history_req_at is null or t.history_req_at < sqlc.arg(retry_before)::timestamptz)
order by t.last_message_at desc nulls last, t.id
limit sqlc.arg(lim);

-- name: SetHistoryRequested :exec
update chat_threads set history_req_oldest = sqlc.arg(oldest)::timestamptz, history_req_at = sqlc.arg(at)::timestamptz,
  history_tries = sqlc.arg(tries)::int where id = sqlc.arg(id)::uuid;

-- name: SetHistoryDone :exec
update chat_threads set history_done_at = sqlc.arg(at)::timestamptz where id = sqlc.arg(id)::uuid;

-- name: SetChatMessageMediaRef :exec
-- A message stored before media references were kept gets one when WhatsApp delivers it again (history sync).
update chat_messages m set media = sqlc.arg(media)::jsonb
from chat_message_keys k
where k.wa_msg_id = sqlc.arg(wa_msg_id)::text and m.id = k.message_id and m.sent_at = k.sent_at
  and (m.media is null or m.media->>'ref' is null);

-- name: EditChatMessageByWAID :one
-- A WhatsApp edit of a stored message (customer, phone or Chat): the new text replaces the old one.
update chat_messages m set body = sqlc.arg(body)::text, edited_at = sqlc.arg(edited_at)::timestamptz
from chat_message_keys k
where k.wa_msg_id = sqlc.arg(wa_msg_id)::text and m.id = k.message_id and m.sent_at = k.sent_at
returning m.id, m.thread_id;

-- name: GetThreadMessage :one
-- One message of a conversation; a group thread also owns the messages stored under another GSI number's thread of
-- the group (as ListThreadMessages). account is the number that received it (media is fetched through it).
select m.*, x.account as holder_account from chat_messages m
join chat_threads x on x.id = m.thread_id
join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
where m.id = sqlc.arg(id)::uuid and (x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid));

-- name: EditQueuedReply :one
-- An edit of a reply still waiting in the outbox: the queued text changes before it goes out.
update outbox set payload = jsonb_set(payload, '{text}', to_jsonb(sqlc.arg(text)::text))
where channel = 'wa' and status = 'pending' and payload->>'message_id' = sqlc.arg(message_id)::text
returning id, proposal_id;

-- name: SetChatMessageBody :exec
update chat_messages set body = sqlc.arg(body)::text, edited_at = sqlc.narg(edited_at)::timestamptz where id = sqlc.arg(id)::uuid;

-- name: MessagesByWAIDs :many
-- The bubbles quoted by replies (WhatsApp "Balas") that are not on the page being shown.
select m.id, k.wa_msg_id, m.thread_id, m.direction, m.from_name, m.body, m.media, m.deleted_at
from chat_message_keys k join chat_messages m on m.id = k.message_id and m.sent_at = k.sent_at
where k.wa_msg_id = any(sqlc.arg(ids)::text[]);

-- name: DeleteChatMessageByWAID :one
-- A WhatsApp delete-for-everyone of a stored message (customer, phone or Chat).
update chat_messages m set deleted_at = sqlc.arg(deleted_at)::timestamptz
from chat_message_keys k
where k.wa_msg_id = sqlc.arg(wa_msg_id)::text and m.id = k.message_id and m.sent_at = k.sent_at and m.deleted_at is null
returning m.id, m.thread_id;

-- name: SetChatMessageDeleted :exec
update chat_messages set deleted_at = sqlc.arg(deleted_at)::timestamptz where id = sqlc.arg(id)::uuid;

-- name: CancelQueuedReply :one
-- A reply deleted while still waiting in the outbox: it never goes out.
update outbox set status = 'cancelled', error = 'Dihapus sebelum terkirim'
where channel = 'wa' and status = 'pending' and payload->>'message_id' = sqlc.arg(message_id)::text
returning id, proposal_id;

-- name: InsertOutgoingFile :one
insert into chat_outgoing_files (message_id, mime, file_name, size, data) values ($1, $2, $3, $4, $5) returning id;

-- name: GetOutgoingFile :one
select * from chat_outgoing_files where id = $1;

-- name: DeleteOutgoingFile :exec
delete from chat_outgoing_files where id = $1;

-- name: PurgeOutgoingFiles :execrows
delete from chat_outgoing_files where created_at < $1;

-- name: SetChatMessageMedia :exec
update chat_messages set media = sqlc.arg(media)::jsonb where id = sqlc.arg(id)::uuid;
