-- name: ListDealersFull :many
select d.id, d.slug, d.name, d.city, d.branch, d.tier, d.segment_desc, d.owner_id, d.credit_limit,
       d.payment_terms_days, d.memo, d.memo_signal_ids, d.memo_updated_at, d.memo_sentences, d.metrics_current, d.updated_at,
       s.name as owner_name, s.branch as owner_branch, s.wa_number as owner_wa
from dealers d left join sales_users s on s.id = d.owner_id
order by length(d.source_id), d.source_id, d.name;

-- name: GetDealer :one
select d.id, d.slug, d.name, d.city, d.branch, d.tier, d.segment_desc, d.owner_id, d.credit_limit,
       d.payment_terms_days, d.memo, d.memo_signal_ids, d.memo_updated_at, d.memo_sentences, d.metrics_current, d.updated_at,
       d.source_id, s.name as owner_name, s.branch as owner_branch, s.wa_number as owner_wa
from dealers d left join sales_users s on s.id = d.owner_id
where d.slug = $1 or d.id::text = $1;

-- name: SearchDealerIDs :many
-- Trigram search over dealer name, city, segment, owner and bought products (server-side, pg_trgm).
select d.id
from dealers d left join sales_users s on s.id = d.owner_id
where d.name % sqlc.arg(q)::text or d.name ilike '%' || sqlc.arg(q)::text || '%'
   or d.city ilike '%' || sqlc.arg(q)::text || '%' or d.segment_desc ilike '%' || sqlc.arg(q)::text || '%'
   or s.name ilike '%' || sqlc.arg(q)::text || '%'
   or exists (select 1 from orders o, jsonb_array_elements(o.lines) l
              where o.dealer_id = d.id and l->>'product' ilike '%' || sqlc.arg(q)::text || '%')
order by similarity(d.name, sqlc.arg(q)::text) desc;

-- name: ListOrders :many
select * from orders where confirmed_at >= $1 or confirmed_at is null order by confirmed_at;

-- name: ListDealerOrders :many
select * from orders where dealer_id = $1 and confirmed_at >= $2 order by confirmed_at desc;

-- name: ListInvoices :many
select * from invoices where issued_at >= $1 or paid < total order by issued_at;

-- name: ListContacts :many
select * from contacts order by dealer_id, interactions_90d desc, name;

-- name: ListDealerContacts :many
select * from contacts where dealer_id = $1 order by interactions_90d desc, name;

-- name: ListSowEstimates :many
select * from dealer_sow_estimates order by confirmed_at desc;

-- name: SetDealerMetrics :exec
update dealers set metrics_current = $2, updated_at = now() where id = $1;

-- name: UpsertMetricsSnapshot :exec
insert into dealer_metrics_daily (dealer_id, as_of, rhythm_days, last_order_days, cyc, status, activity, freq, avg_order,
  segment, sow, mix, credit_room, credit_state, pay_days, on_time, pic_active, score, score_parts)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
on conflict (dealer_id, as_of) do update set rhythm_days = excluded.rhythm_days, last_order_days = excluded.last_order_days,
  cyc = excluded.cyc, status = excluded.status, activity = excluded.activity, freq = excluded.freq, avg_order = excluded.avg_order,
  segment = excluded.segment, sow = excluded.sow, mix = excluded.mix, credit_room = excluded.credit_room,
  credit_state = excluded.credit_state, pay_days = excluded.pay_days, on_time = excluded.on_time,
  pic_active = excluded.pic_active, score = excluded.score, score_parts = excluded.score_parts;

-- name: ListSnapshotsOn :many
-- Latest snapshot at or before the given date for every dealer (movers "3 bulan lalu").
select distinct on (dealer_id) * from dealer_metrics_daily
where as_of <= $1 order by dealer_id, as_of desc;

-- name: ListDealerTimeline :many
-- Interactions with an agent conclusion ("Interaksi + kesimpulan agen").
select * from signals where dealer_id = $1 and payload ? 'conclusion' order by occurred_at desc limit $2;

-- name: ListSignalsByIDs :many
select * from signals where id = any(sqlc.arg(ids)::uuid[]) order by occurred_at desc;

-- name: ListDealerSignalTexts :many
select summary from signals where dealer_id = $1 and summary is not null order by occurred_at desc limit 40;

-- name: ListDealerCommitments :many
select c.*, i.number as invoice_number from commitments c left join invoices i on i.id = c.invoice_id
where c.dealer_id = $1 order by c.side, c.created_at;

-- name: ListDealerInvoices :many
select * from invoices where dealer_id = $1 order by issued_at desc limit $2;

-- name: ListStockItems :many
select * from stock_items order by branch, name;

-- name: CountSignalsSince :one
select count(*) filter (where kind in ('wa','wa_group'))::bigint as wa,
       count(*) filter (where kind = 'so')::bigint as so,
       count(*) filter (where kind = 'payment')::bigint as payments,
       count(*)::bigint as total
from signals where occurred_at >= $1;

-- name: DealerTimelineFull :many
-- Timeline dealer: interactions with an agent conclusion, sends (manual trail signals), replies linked to a
-- proposal, and decisions on proposals — newest first.
select x.at, x.kind, x.via, x.who, x.text, x.conclusion, x.ref from (
  select s.occurred_at as at, s.kind, coalesce(s.payload->>'via', '')::text as via, coalesce(s.payload->>'who', s.payload->>'from_name', '')::text as who,
    coalesce(s.payload->>'text', s.summary, '')::text as text,
    coalesce(s.payload->>'conclusion', 'Balasan untuk: ' || rp.title, '')::text as conclusion, s.id::text as ref
  from signals s left join proposals rp on rp.id::text = s.payload->>'reply_to'
  where s.dealer_id = sqlc.arg(dealer_id) and (s.payload ? 'conclusion' or s.payload ? 'reply_to')
  union all
  select p.decided_at, 'decision', 'form', coalesce(su.name, 'Orchestrator')::text,
    (case when p.status = 'rejected' then 'Ditolak' when p.status = 'expired' then 'Ditunda' when p.decided_by is null then 'Otonom' else 'Disetujui' end || ': ' || p.title)::text,
    (p.agent || ' · proposal ' || left(p.id::text, 8) || coalesce(' · ' || p.decision_reason, ''))::text, p.id::text
  from proposals p left join sales_users su on su.id = p.decided_by
  where p.dealer_id = sqlc.arg(dealer_id) and p.decided_at is not null and p.kind <> 'reply'
) x order by x.at desc limit sqlc.arg(lim);
