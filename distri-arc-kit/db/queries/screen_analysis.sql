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
