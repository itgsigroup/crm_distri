-- Stage 13: retention, partitions, PDP (subject rights), health and alerts.

-- name: EnsurePartitions :one
select arc_ensure_partitions(sqlc.arg(tbl)::text, sqlc.arg(col)::text, sqlc.arg(from_month)::date, sqlc.arg(now_at)::timestamptz, sqlc.arg(months_ahead)::int)::int as created;

-- name: DropPartitionsBefore :one
select arc_drop_partitions_before(sqlc.arg(tbl)::text, sqlc.arg(cutoff)::timestamptz)::int as dropped;

-- name: PurgeChatMessages :execrows
delete from chat_messages where sent_at < sqlc.arg(cutoff)::timestamptz;

-- name: PurgeChatMessageKeys :execrows
delete from chat_message_keys where sent_at < sqlc.arg(cutoff)::timestamptz;

-- name: PurgeSignals :execrows
delete from signals where occurred_at < sqlc.arg(cutoff)::timestamptz;

-- name: PurgeSignalKeys :execrows
delete from signal_keys where occurred_at < sqlc.arg(cutoff)::timestamptz;

-- name: PurgeLLMCalls :execrows
delete from llm_calls where created_at < sqlc.arg(cutoff)::timestamptz;

-- name: PurgeIdentifications :execrows
-- identifications of numbers that never became a contact are kept 90 days (09-policies-security).
delete from identifications i where i.created_at < sqlc.arg(cutoff)::timestamptz
  and not exists (select 1 from contacts c where c.wa_number = i.wa_number);

-- name: PDPDealerExport :one
-- Everything Distri ARC holds about one dealer and its people (UU PDP access right).
select jsonb_build_object(
  'dealer', (select to_jsonb(d) - 'metrics_current' from dealers d where d.id = sqlc.arg(dealer_id)::uuid),
  'contacts', (select coalesce(jsonb_agg(to_jsonb(c) order by c.name), '[]') from contacts c where c.dealer_id = sqlc.arg(dealer_id)::uuid),
  'chat_threads', (select coalesce(jsonb_agg(jsonb_build_object('id', t.id, 'title', t.title, 'wa_jid', t.wa_jid, 'kind', t.kind)), '[]')
                   from chat_threads t where t.dealer_id = sqlc.arg(dealer_id)::uuid),
  'chat_messages', (select coalesce(jsonb_agg(jsonb_build_object('thread_id', m.thread_id, 'direction', m.direction, 'from_number', m.from_number,
                      'from_name', m.from_name, 'body', m.body, 'sent_at', m.sent_at) order by m.sent_at), '[]')
                    from chat_messages m join chat_threads t on t.id = m.thread_id where t.dealer_id = sqlc.arg(dealer_id)::uuid),
  'signals', (select coalesce(jsonb_agg(jsonb_build_object('id', s.id, 'kind', s.kind, 'occurred_at', s.occurred_at, 'summary', s.summary,
                'payload', s.payload) order by s.occurred_at), '[]') from signals s where s.dealer_id = sqlc.arg(dealer_id)::uuid),
  'commitments', (select coalesce(jsonb_agg(jsonb_build_object('side', c.side, 'title', c.title, 'detail', c.detail, 'status', c.status,
                    'due_at', c.due_at)), '[]') from commitments c where c.dealer_id = sqlc.arg(dealer_id)::uuid),
  'proposals', (select coalesce(jsonb_agg(jsonb_build_object('id', p.id, 'kind', p.kind, 'title', p.title, 'status', p.status,
                  'created_at', p.created_at, 'decided_at', p.decided_at) order by p.created_at), '[]')
                from proposals p where p.dealer_id = sqlc.arg(dealer_id)::uuid),
  'orders', (select coalesce(jsonb_agg(jsonb_build_object('number', o.number, 'state', o.state, 'ordered_at', o.ordered_at, 'total', o.total)
               order by o.ordered_at), '[]') from orders o where o.dealer_id = sqlc.arg(dealer_id)::uuid),
  'invoices', (select coalesce(jsonb_agg(jsonb_build_object('number', i.number, 'issued_at', i.issued_at, 'due_at', i.due_at, 'total', i.total,
                 'paid', i.paid) order by i.issued_at), '[]') from invoices i where i.dealer_id = sqlc.arg(dealer_id)::uuid)
)::jsonb as data;

-- name: ContactsByNumber :many
select id, dealer_id, name from contacts where wa_number = sqlc.arg(wa_number)::text;

-- name: PDPDeleteMessages :execrows
-- Messages written by or to the number: its 1:1 threads and its lines in groups.
with gone as (
  delete from chat_messages m
  where m.from_number = sqlc.arg(wa_number)::text
     or m.thread_id in (select t.id from chat_threads t where t.contact_id = any(sqlc.arg(contact_ids)::uuid[]) or t.wa_jid = sqlc.arg(jid)::text)
  returning m.id
)
delete from chat_message_keys k using gone where k.message_id = gone.id;

-- name: PDPDeleteThreads :execrows
delete from chat_threads where contact_id = any(sqlc.arg(contact_ids)::uuid[]) or wa_jid = sqlc.arg(jid)::text;

-- name: PDPDeleteSignals :execrows
-- Conversation signals of the person; transactional signals (SO, invoice, payment) stay without the contact.
with gone as (
  delete from signals s
  where s.kind in ('wa', 'wa_group', 'manual')
    and (s.contact_id = any(sqlc.arg(contact_ids)::uuid[]) or s.payload->>'from' = sqlc.arg(wa_number)::text)
  returning s.dedupe_key
)
delete from signal_keys k using gone where k.dedupe_key = gone.dedupe_key;

-- name: PDPDeleteContacts :execrows
delete from contacts where id = any(sqlc.arg(contact_ids)::uuid[]);

-- name: PDPDeleteIdentification :execrows
delete from identifications where wa_number = sqlc.arg(wa_number)::text;

-- name: MarkMemoStale :exec
-- The memo may quote the deleted person: the next cycle rewrites it from the remaining signals.
update dealers set memo_updated_at = null where id = any(sqlc.arg(dealer_ids)::uuid[]);

-- name: HealthWANumbers :many
select n.wa_number, n.state, n.transport, n.last_seen_at, s.name as sales_name
from wa_numbers n left join sales_users s on s.id = n.sales_id order by s.name;

-- name: HealthOdoo :many
select model, last_run_at, records, error from odoo_sync_state order by model;

-- name: HealthLLMToday :one
select count(*)::bigint as calls, coalesce(sum(cost_idr), 0)::bigint as cost_idr, coalesce(max(created_at), '1970-01-01'::timestamptz)::timestamptz as last_at
from llm_calls where created_at >= sqlc.arg(since)::timestamptz;

-- name: HealthCycles :many
select number, status, started_at, finished_at, duration_ms from cycles order by started_at desc limit 5;

-- name: HealthOutbox :many
select status, count(*)::bigint as n from outbox group by status order by status;

-- name: HealthCounts :one
select (select count(*) from signals)::bigint as signals, (select count(*) from chat_messages)::bigint as chat_messages,
       (select count(*) from dealers)::bigint as dealers, (select count(*) from proposals where status = 'proposed')::bigint as open_proposals;

-- name: OpenAlert :one
-- Opens an alert once; an open alert is not re-opened (notified once until resolved).
insert into system_alerts (key, message, opened_at) values ($1, $2, $3)
on conflict (key) do update set message = excluded.message,
  opened_at = case when system_alerts.resolved_at is not null then excluded.opened_at else system_alerts.opened_at end,
  notified_at = case when system_alerts.resolved_at is not null then null else system_alerts.notified_at end,
  resolved_at = null
returning *;

-- name: MarkAlertNotified :exec
update system_alerts set notified_at = $2 where key = $1;

-- name: ResolveAlerts :many
-- Closes open alerts whose condition is gone.
update system_alerts set resolved_at = sqlc.arg(at)::timestamptz
where resolved_at is null and not (key = any(sqlc.arg(active)::text[]))
returning key, message;

-- name: ListAlerts :many
select * from system_alerts order by opened_at desc limit 20;

-- name: InsertSystemOutbox :one
insert into outbox (channel, to_ref, payload) values ('wa_system', $1, $2) returning id;

-- name: InternalAlertGroup :one
-- The internal group that receives ops alerts: the one named in ALERT_WA_GROUP, else the first internal group.
select jid, name from wa_groups where kind = 'internal' and (sqlc.arg(jid)::text = '' or jid = sqlc.arg(jid)::text)
order by name limit 1;
