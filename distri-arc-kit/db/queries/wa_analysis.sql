-- Analisa WhatsApp (ADR 0026)

-- name: WAWindowMessages :many
-- The messages of the conversations with customers in [$1, $2]: one-to-one (dealers and new numbers) and customer
-- groups (ADR 0033). Internal groups are never read here and DMs between employees are never stored. A group is one
-- conversation for every GSI number in it (each message is stored under the number that got it first): its messages
-- come under one thread of the group (the first by id). internal: a GSI employee wrote it in the group — our side.
-- Failed sends do not count as a reply.
select m.id, coalesce(gt.id, m.thread_id)::uuid as thread_id, m.direction, m.body, m.sent_at, m.signal_id, m.internal, m.from_name,
  t.kind as thread_kind, t.title as thread_title, t.dealer_id, t.sales_id,
  d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name
from chat_messages m
join chat_threads t on t.id = m.thread_id
left join wa_groups g on g.id = t.group_id
left join lateral (select x.id from chat_threads x where t.kind = 'group' and x.kind = 'group' and x.wa_jid = t.wa_jid order by x.id limit 1) gt on true
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
where (t.kind in ('dealer','new') or (t.kind = 'group' and g.kind = 'external' and g.read_enabled))
  and m.sent_at >= sqlc.arg(since) and m.sent_at <= sqlc.arg(until) and m.status <> 'failed'
order by 2, m.sent_at, m.id;

-- name: ConversationOf :one
-- The thread a conversation is analysed under: a group's first thread by id (see WAWindowMessages), else itself.
select coalesce((select x.id from chat_threads x where t.kind = 'group' and x.kind = 'group' and x.wa_jid = t.wa_jid order by x.id limit 1), t.id)::uuid
from chat_threads t where t.id = $1;

-- name: SaveWAAnalysis :exec
insert into wa_analyses (cycle_id, window_days, threads, inbound, outbound, waiting, reply_median_min, overview, source, created_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (cycle_id) do update set window_days = excluded.window_days, threads = excluded.threads, inbound = excluded.inbound,
  outbound = excluded.outbound, waiting = excluded.waiting, reply_median_min = excluded.reply_median_min,
  overview = excluded.overview, source = excluded.source, created_at = excluded.created_at;

-- name: DeleteWAInsights :exec
delete from wa_insights where cycle_id = $1;

-- name: InsertWAInsight :exec
insert into wa_insights (cycle_id, thread_id, kind, dealer_id, sales_id, title, intent, items, last_text, inbound, outbound,
  last_in_at, last_out_at, waiting_since, reply_median_min, needs_reply, priority, signal_ids, proposal_ids, summary, confidence)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21);

-- name: LatestWAAnalysis :one
select a.*, c.number as cycle_number, c.status as cycle_status
from wa_analyses a join cycles c on c.id = a.cycle_id
order by c.number desc limit 1;

-- name: ListWAAnalyses :many
select a.*, c.number as cycle_number, c.status as cycle_status, c.via as cycle_via
from wa_analyses a join cycles c on c.id = a.cycle_id
order by c.number desc limit $1;

-- name: ListWAInsights :many
-- team: the team of the number holding the conversation (sales, admin, billing — ADR 0033).
select i.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, coalesce(n.team, 'sales')::text as team
from wa_insights i
left join dealers d on d.id = i.dealer_id
left join sales_users s on s.id = i.sales_id
left join chat_threads t on t.id = i.thread_id
left join wa_numbers n on n.wa_number = t.account
where i.cycle_id = $1
order by i.priority desc, i.last_in_at desc nulls last;

-- name: SetWAOverview :exec
update wa_analyses set overview = $2, source = $3 where cycle_id = $1;

-- name: SetWAInsightSummary :exec
update wa_insights set summary = $3, confidence = $4 where cycle_id = $1 and thread_id = $2;

-- name: ThreadsWaitingSince :many
-- The current state of conversations: the oldest customer message not answered yet (null when the last word was ours).
select t.id, t.dealer_id, s.name as sales_name,
  (select min(m.sent_at) from chat_messages m where m.thread_id = t.id and m.direction = 'in'
     and m.sent_at > coalesce((select max(o.sent_at) from chat_messages o where o.thread_id = t.id and o.direction = 'out' and o.status <> 'failed'), '-infinity'::timestamptz)) as waiting_since
from chat_threads t left join sales_users s on s.id = t.sales_id
where t.id = any(sqlc.arg(ids)::uuid[]);

-- name: DeleteChatPlanItems :exec
delete from plan_items where plan_date = $1 and link like 'chat:%';

-- name: MaxPlanSeq :one
select coalesce(max(seq), 0)::int as seq from plan_items where plan_date = $1;

-- name: UpsertWAMessageInsights :exec
-- One analysis of many bubbles at once (a JSON array of rows); the AI note is kept.
insert into wa_message_insights (message_id, thread_id, sent_at, direction, state, intent, items, wait_min, cycle_id, analyzed_at)
select x.message_id, x.thread_id, x.sent_at, x.direction, x.state, x.intent, coalesce(x.items, '[]'), x.wait_min, sqlc.narg(cycle_id)::uuid, sqlc.arg(analyzed_at)::timestamptz
from jsonb_to_recordset(sqlc.arg(rows)::jsonb)
  as x(message_id uuid, thread_id uuid, sent_at timestamptz, direction text, state text, intent text, items jsonb, wait_min int)
on conflict (message_id) do update set state = excluded.state, intent = excluded.intent, items = excluded.items, wait_min = excluded.wait_min,
  cycle_id = excluded.cycle_id, analyzed_at = excluded.analyzed_at;

-- name: SetWAMessageNote :exec
update wa_message_insights set note = sqlc.arg(note), confidence = sqlc.arg(confidence), note_cycle_id = sqlc.narg(cycle_id)::uuid
where message_id = sqlc.arg(message_id);

-- name: WAMessageInsights :many
-- The analysis of the bubbles of one conversation (the chat page), with the cycle that wrote it.
select i.message_id, i.state, i.intent, i.items, i.wait_min, i.analyzed_at, i.note, i.confidence, c.number as cycle_number
from wa_message_insights i left join cycles c on c.id = i.cycle_id
where i.thread_id = sqlc.arg(thread_id) and i.message_id = any(sqlc.arg(message_ids)::uuid[]);

-- name: LastWAMessageAnalysis :one
-- When the bubbles of a conversation were last analysed (null: never).
select max(analyzed_at)::timestamptz as analyzed_at from wa_message_insights where thread_id = $1;

-- name: WACycleThreads :many
-- The conversations whose bubbles a cycle read (Analisa WhatsApp of one conversation, one dealer, or the chat screen):
-- waiting for a reply first, then the latest; the request of the newest customer bubble.
select i.thread_id, t.kind, t.title, d.name as dealer_name, s.name as sales_name,
  coalesce((array_agg(i.intent order by i.sent_at desc) filter (where i.direction = 'in'))[1], '')::text as intent,
  min(i.sent_at) filter (where i.state in ('waiting', 'needs_reply')) as waiting_since,
  max(i.sent_at)::timestamptz as last_at
from wa_message_insights i
join chat_threads t on t.id = i.thread_id
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
where i.cycle_id = sqlc.narg(cycle_id)::uuid
group by i.thread_id, t.kind, t.title, d.name, s.name
having count(*) filter (where i.direction = 'in') > 0
order by (min(i.sent_at) filter (where i.state in ('waiting', 'needs_reply'))) is null, max(i.sent_at) desc
limit 50;
