-- Analisa WhatsApp (ADR 0026)

-- name: WAWindowMessages :many
-- The messages of one-to-one conversations (dealers and new numbers) in [$1, $2]; groups are never read here and
-- DMs between employees are never stored. Failed sends do not count as a reply.
select m.id, m.thread_id, m.direction, m.body, m.sent_at, m.signal_id,
  t.kind as thread_kind, t.title as thread_title, t.dealer_id, t.sales_id,
  d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name
from chat_messages m
join chat_threads t on t.id = m.thread_id
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
where t.kind in ('dealer','new') and m.sent_at >= sqlc.arg(since) and m.sent_at <= sqlc.arg(until) and m.status <> 'failed'
order by m.thread_id, m.sent_at, m.id;

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
select i.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name
from wa_insights i
left join dealers d on d.id = i.dealer_id
left join sales_users s on s.id = i.sales_id
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
