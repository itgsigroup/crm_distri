-- Analisa WhatsApp (ADR 0026)

-- name: WAWindowMessages :many
-- The messages of the conversations with customers in [$1, $2]: one-to-one (dealers and new numbers) and customer
-- groups (ADR 0033). Internal groups are never read here and DMs between employees are never stored. A group is one
-- conversation for every GSI number in it (each message is stored under the number that got it first): its messages
-- come under one thread of the group (the first by id). internal: a GSI employee wrote it in the group — our side.
-- Failed sends do not count as a reply.
select m.id, coalesce(gt.id, m.thread_id)::uuid as thread_id, m.direction, m.body, m.sent_at, m.signal_id, m.internal, m.from_name, m.holding,
  m.closing, m.auto_reply,
  t.kind as thread_kind, t.title as thread_title, t.dealer_id, t.sales_id,
  d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, t.account
from chat_messages m
join chat_threads t on t.id = m.thread_id
left join wa_groups g on g.id = t.group_id
left join lateral (select x.id from chat_threads x where t.kind = 'group' and x.kind = 'group' and x.wa_jid = t.wa_jid order by x.id limit 1) gt on true
left join dealers d on d.id = t.dealer_id
left join sales_users s on s.id = t.sales_id
where (t.kind in ('dealer','new') or (t.kind = 'group' and g.kind = 'external' and g.read_enabled))
  -- a supplier or a personal number is not a customer (ADR 0035)
  and not exists (select 1 from contact_types ct where ct.wa_number = split_part(t.wa_jid, '@', 1))
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
  last_in_at, last_out_at, waiting_since, reply_median_min, needs_reply, priority, signal_ids, proposal_ids, summary, confidence,
  reply_work_min, reply_off_min)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23);

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
select i.*, d.slug as dealer_slug, d.name as dealer_name, s.name as sales_name, coalesce(n.team, 'sales')::text as team, t.account
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
update wa_insights set summary = $3, confidence = $4, summary_at = now() where cycle_id = $1 and thread_id = $2;

-- name: CarryWASummaries :exec
-- What the AI already read stays: a conversation of this cycle without a line takes its latest line from an earlier
-- cycle (with when it was written, so only newer customer messages are read again).
update wa_insights i set summary = p.summary, confidence = p.confidence, summary_at = p.summary_at
from (select distinct on (x.thread_id) x.thread_id, x.summary, x.confidence, x.summary_at from wa_insights x
      where x.summary is not null and x.cycle_id <> sqlc.arg(cycle_id)::uuid order by x.thread_id, x.summary_at desc nulls last) p
where i.cycle_id = sqlc.arg(cycle_id)::uuid and i.thread_id = p.thread_id and i.summary is null;

-- name: CarryWAOverview :exec
-- The overview stays until the AI writes a new one: a cycle without one takes the latest.
update wa_analyses a set overview = p.overview, source = p.source
from (select overview, source from wa_analyses where cycle_id <> sqlc.arg(cycle_id)::uuid and overview is not null order by created_at desc limit 1) p
where a.cycle_id = sqlc.arg(cycle_id)::uuid and a.overview is null;

-- name: ThreadsWaitingSince :many
-- The current state of conversations: the oldest customer message not answered yet (null when the last word was ours
-- and was an answer, not a holding reply — ADR 0034).
select t.id, t.dealer_id, s.name as sales_name,
  -- the marks made by hand in Chat (migration 0040) count here too, as in the chat list
  coalesce(
    (select min(m.sent_at) from chat_messages m where m.thread_id = t.id and m.direction = 'in' and not m.internal and not m.closing and not m.auto_reply
       and coalesce(m.media->>'kind', '') <> 'system' -- a group's own line ("Anda menambahkan Budi", ADR 0063) waits for no answer
       and m.sent_at > greatest(coalesce((select max(o.sent_at) from chat_messages o where o.thread_id = t.id and (o.direction = 'out' or o.internal)
                                 and not o.holding and not o.auto_reply and o.status <> 'failed'), '-infinity'::timestamptz), coalesce(t.replied_mark, '-infinity'::timestamptz))),
    case when t.unreplied_mark is not null and t.unreplied_mark > coalesce((select max(o.sent_at) from chat_messages o where o.thread_id = t.id and o.direction = 'out'
                                 and not o.holding and not o.auto_reply and o.status <> 'failed'), '-infinity'::timestamptz) then t.unreplied_mark end
  ) as waiting_since
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

-- name: LatestWAOverview :one
-- The overview the AI wrote last, before this cycle (the reading brings it up to date).
select overview from wa_analyses where cycle_id <> sqlc.arg(cycle_id)::uuid and overview is not null and source = 'ai'
order by created_at desc limit 1;

-- name: WAReportOrders :many
-- Report WhatsApp (ADR 0050): the orders of these dealers since a time — an order request in chat counts as won when
-- the dealer orders within a few days after it.
select o.dealer_id, coalesce(o.confirmed_at, o.ordered_at)::timestamptz as at, coalesce(o.total, 0)::bigint as total
from orders o
where o.dealer_id = any(sqlc.arg(dealer_ids)::uuid[]) and o.state <> 'cancel'
  and coalesce(o.confirmed_at, o.ordered_at) >= sqlc.arg(since)
order by 2;

-- name: ThreadAISummaries :many
-- The tab Main's Analisa Claude, start to end (Sam 22 Oct): every line Claude wrote on this conversation through the
-- Orchestrator (Analisa WhatsApp), oldest first; a group reads the threads of every GSI number in it. A line carried
-- to later cycles keeps its summary_at, so it is listed once (the cycle that wrote it).
select distinct on (coalesce(i.summary_at, c.started_at)) i.cycle_id, c.number as cycle_number,
  coalesce(i.summary_at, c.started_at)::timestamptz as at, i.intent, i.items, coalesce(i.summary, '')::text as summary, i.confidence
from wa_insights i join cycles c on c.id = i.cycle_id
where i.thread_id in (select x.id from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
                      where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
  and coalesce(i.summary, '') <> ''
order by coalesce(i.summary_at, c.started_at), c.number;

-- name: ThreadAIFacts :one
-- The conversation from its first stored message to its last, and how many messages carry a Claude note.
with th as (select x.id from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
            where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
select count(*)::int as messages, count(*) filter (where m.direction = 'in')::int as inbound,
  coalesce(min(m.sent_at), 'epoch')::timestamptz as first_at, coalesce(max(m.sent_at), 'epoch')::timestamptz as last_at,
  (select count(*) from wa_message_insights w where w.thread_id in (select id from th) and w.note is not null)::int as notes
from chat_messages m
where m.deleted_at is null and m.thread_id in (select id from th);

-- name: DealerClaudeProposals :many
-- Kerjasama: what Claude proposed for this dealer through the Orchestrator (cycles via MCP) and what a person decided.
select p.id, p.agent, p.kind, p.title, p.why, p.status, p.created_at, p.decided_at, coalesce(u.name, '')::text as decided_by_name,
  c.number as cycle_number
from proposals p join cycles c on c.id = p.cycle_id left join sales_users u on u.id = p.decided_by
where p.dealer_id = sqlc.arg(dealer_id)::uuid and c.via = 'mcp' and p.status <> 'suppressed'
order by p.created_at desc limit 12;

-- name: ThreadAISubmitted :many
-- Claude's Analisa WhatsApp answers through MCP (orchestrator_submit) that speak of this conversation: its line (an
-- "Analisis ulang chat ini" cycle has no wa_insights rows, so its line lives only in the answer). The intent and
-- items are what Claude was given for the conversation.
with th as (select x.id from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
            where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
select ci.cycle_id, c.number as cycle_number, coalesce(ci.submitted_at, ci.created_at)::timestamptz as at,
  coalesce(l.v->>'summary', '')::text as summary, coalesce((l.v->>'confidence')::float8, 0)::float8 as confidence,
  coalesce(i.v->>'intent', '')::text as intent_label, coalesce(i.v->'items', '[]'::jsonb)::jsonb as items
from cycle_inputs ci
join cycles c on c.id = ci.cycle_id
cross join lateral jsonb_array_elements(case when jsonb_typeof(ci.submitted->'threads') = 'array' then ci.submitted->'threads' else '[]'::jsonb end) l(v)
left join lateral (select x.v from jsonb_array_elements(case when jsonb_typeof(ci.input->'threads') = 'array' then ci.input->'threads' else '[]'::jsonb end) x(v)
                   where x.v->>'id' = l.v->>'id' limit 1) i on true
where ci.agent = 'Analisa WhatsApp' and ci.submitted is not null and (l.v->>'id') in (select id::text from th)
order by at;

-- name: ThreadAINotes :many
-- Claude's note on each customer message of the conversation (Analisa WhatsApp per bubble), oldest first.
select w.message_id, w.sent_at, coalesce(w.note, '')::text as note, w.confidence, w.intent, w.items, c.number as cycle_number
from wa_message_insights w left join cycles c on c.id = coalesce(w.note_cycle_id, w.cycle_id)
where w.thread_id in (select x.id from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
                      where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
  and coalesce(w.note, '') <> ''
order by w.sent_at;

-- name: ThreadAIOverviews :many
-- The overview Claude wrote for a cycle scoped to this conversation ("Analisis ulang chat ini"): it is about this
-- chat or group alone.
select ci.cycle_id, c.number as cycle_number, coalesce(ci.submitted_at, ci.created_at)::timestamptz as at,
  coalesce(ci.submitted->>'overview', '')::text as overview
from cycle_inputs ci join cycles c on c.id = ci.cycle_id
where ci.agent = 'Analisa WhatsApp' and ci.submitted is not null and coalesce(ci.submitted->>'overview', '') <> ''
  and c.scope in (select 'thread:' || x.id::text from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
                  where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
order by at;

-- name: ThreadStatsMessages :many
-- Statistik: every stored message of the conversation (a group: of every GSI number's thread of it), oldest first.
select m.sent_at, coalesce(m.direction, '')::text as direction, m.internal, coalesce(m.holding, false)::bool as holding,
  coalesce(m.closing, false)::bool as closing, coalesce(m.auto_reply, false)::bool as auto_reply, m.status,
  (m.media is not null and m.media <> 'null'::jsonb)::bool as has_media, coalesce(m.from_name, '')::text as from_name
from chat_messages m
where m.deleted_at is null and m.thread_id in (select x.id from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
                      where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid))
order by m.sent_at;

-- name: ThreadStatsAsks :many
-- What Analisa WhatsApp read on each customer message of the conversation: intent and items.
select w.intent, w.items
from wa_message_insights w
where w.direction = 'in' and w.thread_id in (select x.id from chat_threads x join chat_threads t on t.id = sqlc.arg(thread_id)::uuid
                      where x.id = t.id or (t.kind = 'group' and x.wa_jid = t.wa_jid));
