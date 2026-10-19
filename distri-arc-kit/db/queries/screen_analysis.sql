-- name: SaveScreenAnalysis :exec
insert into screen_analyses (cycle_id, screen, overview, lines, source, created_at)
values (sqlc.arg(cycle_id)::uuid, sqlc.arg(screen)::text, sqlc.arg(overview)::text, sqlc.arg(lines)::jsonb, sqlc.arg(source)::text, sqlc.arg(created_at)::timestamptz)
on conflict (cycle_id, screen) do update set overview = excluded.overview, lines = excluded.lines, source = excluded.source, created_at = excluded.created_at;

-- name: LatestScreenAnalysis :one
select a.cycle_id, a.screen, a.overview, a.lines, a.source, a.created_at, c.number as cycle_number
from screen_analyses a join cycles c on c.id = a.cycle_id
where a.screen = $1
order by a.created_at desc
limit 1;

-- name: SaveDealerAnalysis :exec
insert into dealer_analyses (cycle_id, dealer_id, status, overview, reasons, wa_draft, confidence, created_at)
values (sqlc.arg(cycle_id)::uuid, sqlc.arg(dealer_id)::uuid, sqlc.arg(status)::text, sqlc.arg(overview)::text, sqlc.arg(reasons)::jsonb,
  sqlc.arg(wa_draft)::text, sqlc.narg(confidence)::int, sqlc.arg(created_at)::timestamptz)
on conflict (cycle_id, dealer_id) do update set status = excluded.status, overview = excluded.overview, reasons = excluded.reasons,
  wa_draft = excluded.wa_draft, confidence = excluded.confidence, created_at = excluded.created_at;

-- name: LatestDealerAnalysis :one
select a.cycle_id, a.dealer_id, a.status, a.overview, a.reasons, a.wa_draft, a.confidence, a.created_at, c.number as cycle_number
from dealer_analyses a join cycles c on c.id = a.cycle_id
where a.dealer_id = $1
order by a.created_at desc
limit 1;
