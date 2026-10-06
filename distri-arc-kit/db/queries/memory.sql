-- name: SetDealerMemo :exec
update dealers set memo = $2, memo_signal_ids = $3, memo_sentences = $4, memo_updated_at = $5 where id = $1;

-- name: DealerMemoState :many
-- Memo state per dealer and whether signals arrived after the memo was written.
select d.id, d.slug, d.memo, d.memo_sentences, d.memo_updated_at, d.memo_signal_ids,
  exists (select 1 from signals s where s.dealer_id = d.id and s.occurred_at > coalesce(d.memo_updated_at, '1970-01-01'::timestamptz)) as has_new
from dealers d order by d.slug;

-- name: RejectionsForLessons :many
-- Human rejections of the last 30 days with what they were about (lesson aggregation).
select c.agent, c.kind, c.reason, c.created_at, p.payload, p.title, d.name as dealer_name, d.tier
from calibration_events c join proposals p on p.id = c.proposal_id left join dealers d on d.id = c.dealer_id
where c.decision = 'rejected' and c.created_at >= sqlc.arg(since)::timestamptz
order by c.created_at;

-- name: UpsertLesson :exec
insert into calibration_lessons (agent, kind, reason, scope, product, rejections, text, suppress_until, created_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (agent, kind, reason, scope, product) do update set rejections = excluded.rejections, text = excluded.text,
  suppress_until = excluded.suppress_until;

-- name: ActiveLessons :many
select * from calibration_lessons where suppress_until >= $1 order by created_at desc;

-- name: ListLessons :many
select * from calibration_lessons order by created_at desc limit $1;

-- name: SaveBrief :exec
insert into briefs (brief_date, cycle_id, brief, source, created_at) values ($1, $2, $3, $4, $5)
on conflict (brief_date) do update set cycle_id = excluded.cycle_id, brief = excluded.brief, source = excluded.source, created_at = excluded.created_at;

-- name: GetBrief :one
select * from briefs where brief_date = $1;
