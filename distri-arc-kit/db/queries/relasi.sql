-- name: UpsertInteractionMonth :exec
insert into interactions_monthly (sales_id, dealer_id, month, n, source, as_of) values ($1, $2, $3, $4, $5, $6)
on conflict (sales_id, dealer_id, month, source) do update set n = excluded.n, as_of = excluded.as_of;

-- name: InteractionsAsOf :one
-- Live counting starts after the imported history (or from the beginning without one).
select coalesce(max(as_of), '1970-01-01'::timestamptz)::timestamptz as as_of from interactions_monthly;

-- name: RelasiMonthly :many
-- Interactions per (sales, dealer, month) since a month: imported history + live WhatsApp messages and orders
-- after as_of (dealer threads only; internal numbers are never stored).
with live as (
  select t.sales_id, t.dealer_id, date_trunc('month', m.sent_at at time zone 'Asia/Jakarta')::date as month, count(*)::int as n
  from chat_messages m join chat_threads t on t.id = m.thread_id
  where t.kind = 'dealer' and t.dealer_id is not null and t.sales_id is not null and m.sent_at > sqlc.arg(as_of)::timestamptz
  group by 1, 2, 3
  union all
  select d.owner_id, o.dealer_id, date_trunc('month', o.ordered_at at time zone 'Asia/Jakarta')::date, count(*)::int
  from orders o join dealers d on d.id = o.dealer_id
  where d.owner_id is not null and o.ordered_at > sqlc.arg(as_of)::timestamptz
  group by 1, 2, 3
), all_rows as (
  select sales_id, dealer_id, month, n from interactions_monthly
  union all
  select sales_id, dealer_id, month, n from live
)
select a.sales_id, a.dealer_id, a.month, sum(a.n)::bigint as n
from all_rows a
where a.month >= sqlc.arg(since)::date
group by 1, 2, 3
order by 1, 2, 3;

-- name: SetContactBaseline :exec
update contacts set interactions_base = interactions_90d, base_as_of = $2 where id = $1;

-- name: RecountContacts :exec
-- PIC aktif: messages from each contact's number in the last 90 days (plus the imported baseline while it is
-- inside the window), last interaction = newest of baseline and messages.
update contacts c set
  interactions_90d = (case when c.base_as_of is not null and c.base_as_of >= sqlc.arg(window_start)::timestamptz then coalesce(c.interactions_base, 0) else 0 end)
    + (select count(*) from chat_messages m where m.from_number = c.wa_number and m.direction = 'in' and m.sent_at >= sqlc.arg(window_start)::timestamptz
         and (c.base_as_of is null or m.sent_at > c.base_as_of))::int,
  last_interaction_at = greatest(c.last_interaction_at, (select max(m.sent_at) from chat_messages m where m.from_number = c.wa_number and m.direction = 'in'))
where c.wa_number is not null;
