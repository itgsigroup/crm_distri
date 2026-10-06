-- Stage 14: pilot measurement (Pengaturan → Pilot, CSV, docs/PILOT-REPORT.md). Proposals without a dealer (stock
-- transfers, purchase requests) count for every branch.

-- name: PilotAgentStats :many
select p.agent,
  count(*)::bigint as proposed,
  count(*) filter (where p.status in ('approved','executed') and p.decided_by is not null)::bigint as approved,
  count(*) filter (where p.status = 'edited')::bigint as edited,
  count(*) filter (where p.status = 'rejected')::bigint as rejected,
  count(*) filter (where p.status = 'expired')::bigint as expired,
  count(*) filter (where p.status = 'proposed')::bigint as open,
  count(*) filter (where p.decided_by is null and p.decided_at is not null and p.status <> 'expired')::bigint as autonomous,
  coalesce(percentile_cont(0.5) within group (order by extract(epoch from p.decided_at - p.created_at) / 60)
    filter (where p.decided_by is not null), -1)::float8 as median_decision_min
from proposals p left join dealers d on d.id = p.dealer_id
where p.kind <> 'reply' and p.created_at >= sqlc.arg(since)::timestamptz and p.created_at < sqlc.arg(until)::timestamptz
  and (sqlc.arg(branch)::text = '' or d.branch = sqlc.arg(branch)::text or p.dealer_id is null)
group by p.agent order by p.agent;

-- name: PilotWeeklyConfidence :many
-- Human decisions per agent and ISO week (WIB): approved/edited vs rejected — the unlock rule reads it.
select p.agent, date_trunc('week', p.decided_at at time zone 'Asia/Jakarta')::date as week,
  count(*) filter (where p.status in ('approved','edited','executed'))::bigint as accepted,
  count(*) filter (where p.status = 'rejected')::bigint as rejected
from proposals p left join dealers d on d.id = p.dealer_id
where p.kind <> 'reply' and p.decided_by is not null and p.decided_at >= sqlc.arg(since)::timestamptz
  and (sqlc.arg(branch)::text = '' or d.branch = sqlc.arg(branch)::text or p.dealer_id is null)
group by 1, 2 order by 1, 2;

-- name: PilotDriftCaught :one
-- Dealers that went "At risk" (lewat jadwal) in the window: how many ordered again before reaching Churn after a
-- decided proposal, and how many reached Churn.
with risk as (
  select m.dealer_id, min(m.as_of) as first_risk
  from dealer_metrics_daily m join dealers d on d.id = m.dealer_id
  where m.status = 'At risk' and m.as_of >= sqlc.arg(since)::date and m.as_of < sqlc.arg(until)::date
    and (sqlc.arg(branch)::text = '' or d.branch = sqlc.arg(branch)::text)
  group by m.dealer_id
), churned as (
  select distinct r.dealer_id from risk r join dealer_metrics_daily m on m.dealer_id = r.dealer_id
  where m.status = 'Churn' and m.as_of > r.first_risk and m.as_of < sqlc.arg(until)::date
), caught as (
  select r.dealer_id from risk r
  where r.dealer_id not in (select dealer_id from churned)
    and exists (select 1 from proposals p where p.dealer_id = r.dealer_id and p.decided_at >= r.first_risk
                and p.status in ('approved','edited','executed'))
    and exists (select 1 from orders o where o.dealer_id = r.dealer_id and o.ordered_at >= r.first_risk and o.state <> 'cancel')
)
select (select count(*) from risk)::bigint as at_risk, (select count(*) from caught)::bigint as caught,
       (select count(*) from churned)::bigint as churned;

-- name: AuditUnapprovedSends :one
-- Rows delivered without a recorded decision (human, or the system within policy for automatic steps).
select count(*)::bigint from outbox o left join proposals p on p.id = o.proposal_id
where o.status = 'sent' and o.channel <> 'wa_system' and o.sent_at >= sqlc.arg(since)::timestamptz and o.sent_at < sqlc.arg(until)::timestamptz
  and (p.id is null or p.decided_at is null or p.status not in ('approved','edited','executed'));

-- name: AuditSentBetween :one
-- Rows delivered in a period (shadow weeks must have none).
select count(*)::bigint from outbox where status = 'sent' and channel <> 'wa_system'
  and sent_at >= sqlc.arg(since)::timestamptz and sent_at < sqlc.arg(until)::timestamptz;

-- name: AuditSystemToNonInternal :one
select count(*)::bigint from outbox o where o.channel = 'wa_system' and o.status = 'sent'
  and o.sent_at >= sqlc.arg(since)::timestamptz and o.sent_at < sqlc.arg(until)::timestamptz
  and not exists (select 1 from wa_groups g where g.jid = o.to_ref and g.kind = 'internal');

-- name: AuditInternalDMs :one
-- Direct messages between internal numbers must never be stored (09-policies-security).
select count(*)::bigint from chat_messages m join chat_threads t on t.id = m.thread_id
where t.kind <> 'group' and m.sent_at >= sqlc.arg(since)::timestamptz and m.sent_at < sqlc.arg(until)::timestamptz
  and split_part(t.wa_jid, '@', 1) in (select wa_number from internal_numbers);

-- name: AuditInternalGroupToDealer :one
-- Internal group messages are for stock, schedules and tasks only: never attributed to a dealer.
select count(*)::bigint from signals s join chat_threads t on t.id = nullif(s.payload->>'thread', '')::uuid
join wa_groups g on g.id = t.group_id
where s.kind = 'wa_group' and g.kind = 'internal' and s.dealer_id is not null
  and s.occurred_at >= sqlc.arg(since)::timestamptz and s.occurred_at < sqlc.arg(until)::timestamptz;

-- name: AuditOutboundIdentified :one
-- Number identification only for numbers that wrote first.
select count(*)::bigint from identifications i
where i.created_at >= sqlc.arg(since)::timestamptz and i.created_at < sqlc.arg(until)::timestamptz
  and not exists (select 1 from chat_messages m where m.from_number = i.wa_number and m.direction = 'in');

-- name: UpsertPilotWeek :exec
insert into pilot_weeks (week, branch, data) values ($1, $2, $3)
on conflict (week, branch) do update set data = excluded.data, created_at = now();

-- name: ListPilotWeeks :many
select * from pilot_weeks where branch = $1 order by week desc limit 12;

-- name: TopDealersForSOW :many
-- Konfirmasi share of wallet: the biggest dealers by monthly turnover, with this quarter's confirmation if any.
select d.id, d.slug, d.name, d.branch, s.name as sales_name,
  coalesce((d.metrics_current->>'sow')::int, 0) as sow, coalesce(d.metrics_current->>'sow_source', '') as sow_source,
  coalesce((d.metrics_current->>'omzet_bln')::bigint, 0) as omzet_bln,
  e.sow as confirmed_sow, e.note as confirmed_note, e.confirmed_at
from dealers d left join sales_users s on s.id = d.owner_id
left join dealer_sow_estimates e on e.dealer_id = d.id and e.quarter = sqlc.arg(quarter)::text
where (sqlc.arg(branch)::text = '' or d.branch = sqlc.arg(branch)::text)
  and (sqlc.narg(owner)::uuid is null or d.owner_id = sqlc.narg(owner)::uuid)
order by coalesce((d.metrics_current->>'omzet_bln')::bigint, 0) desc limit sqlc.arg(lim);

-- name: ConfirmSOW :exec
insert into dealer_sow_estimates (dealer_id, quarter, sow, note, confirmed_by, confirmed_at)
values ($1, $2, $3, $4, $5, $6)
on conflict (dealer_id, quarter) do update set sow = excluded.sow, note = excluded.note, confirmed_by = excluded.confirmed_by,
  confirmed_at = excluded.confirmed_at;
