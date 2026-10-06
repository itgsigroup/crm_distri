-- name: UpsertGetcontact :exec
insert into getcontact_imports (wa_number, name, tags, imported_by, imported_at) values ($1, $2, $3, $4, $5)
on conflict (wa_number, name) do update set tags = excluded.tags, imported_by = excluded.imported_by, imported_at = excluded.imported_at;

-- name: GetcontactFor :many
select * from getcontact_imports where wa_number = $1 order by tags desc, imported_at desc;

-- name: InboundNumber :one
-- Identification is only for numbers that wrote to a sales number first (09-policies-security).
select t.id, t.wa_jid, t.sales_id, su.wa_number as sales_wa from chat_threads t left join sales_users su on su.id = t.sales_id
where t.wa_jid = sqlc.arg(jid) and t.kind = 'new'
  and exists (select 1 from chat_messages m where m.thread_id = t.id and m.direction = 'in')
limit 1;

-- name: ContactByNumber :one
select c.name, d.name as dealer_name from contacts c join dealers d on d.id = c.dealer_id where c.wa_number = $1 limit 1;

-- name: SaveIdentification :exec
insert into identifications (wa_number, sources, best_name, best_org, score, potential, identified_at) values ($1, $2, $3, $4, $5, $6, $7)
on conflict (wa_number) do update set sources = excluded.sources, best_name = excluded.best_name, best_org = excluded.best_org,
  score = excluded.score, potential = coalesce(excluded.potential, identifications.potential), identified_at = excluded.identified_at;

-- name: GetIdentification :one
select * from identifications where wa_number = $1;

-- name: SetThreadIdentification :exec
update chat_threads set identification = $2 where wa_jid = $1 and kind = 'new';

-- name: CityPotential :one
-- Monthly omzet of dealers in a city (median of their average order × frequency) — potential of a new dealer.
select count(*)::int as dealers, coalesce(percentile_cont(0.5) within group (order by (d.metrics_current->>'omzet_bln')::numeric), 0)::bigint as median_omzet
from dealers d where lower(d.city) = lower($1) and d.metrics_current is not null;

-- name: ThreadProposals :many
-- Proposals about a thread without a dealer (new number): new_dealer and price_list.
select id, kind, title, button, icon, agent, due_label, status, why, decided_at, executed_at, autonomy
from proposals where payload->>'thread_id' = sqlc.arg(thread_id)::text and status <> 'expired'
order by created_at desc;
