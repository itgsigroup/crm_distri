-- Real data import (BigQuery / CSV) and master data (Pengaturan → Data & master).

-- name: UpsertImportRow :exec
-- An unchanged row keeps its updated_at, so the next apply rebuilds only what changed at the source.
insert into import_rows (entity, key, data, source, updated_at) values ($1, $2, $3, $4, now())
on conflict (entity, key) do update set data = excluded.data, source = excluded.source, updated_at = now()
where import_rows.data is distinct from excluded.data or import_rows.source is distinct from excluded.source;

-- name: DeleteImportRowsNotIn :execrows
-- A snapshot (stock) replaces what the same source delivered before: rows missing from it go.
delete from import_rows where entity = sqlc.arg(entity) and source = sqlc.arg(source) and not (key = any(sqlc.arg(keys)::text[]));

-- name: DeleteInvoiceLinesNotIn :execrows
-- Lines of the delivered invoices that are no longer in the delivery (an invoice lost a line).
delete from import_rows where entity = 'invoice_lines' and split_part(key, '#', 1) = any(sqlc.arg(invoices)::text[])
  and not (key = any(sqlc.arg(keys)::text[]));

-- name: ListImportRows :many
select key, data from import_rows where entity = $1 order by key;

-- name: CountImportRows :many
select entity, count(*)::bigint as n, max(updated_at)::timestamptz as last_at from import_rows group by entity order by entity;

-- name: DeleteImportRows :execrows
delete from import_rows where entity = $1 and source = $2 and updated_at < $3;

-- name: ListMappings :many
select * from data_mappings where (sqlc.arg(kind)::text = '' or kind = sqlc.arg(kind)::text)
order by kind, (target is null) desc, seen desc, source_value;

-- name: SeenMapping :exec
-- Records a source value met during an import (unmapped ones then show up in Pengaturan).
insert into data_mappings (kind, source_value, seen) values ($1, $2, $3)
on conflict (kind, source_value) do update set seen = excluded.seen;

-- name: SetMapping :exec
insert into data_mappings (kind, source_value, target, updated_by, updated_at) values ($1, $2, $3, $4, now())
on conflict (kind, source_value) do update set target = excluded.target, updated_by = excluded.updated_by, updated_at = now();

-- name: MappingCounts :many
select kind, count(*)::bigint as total, count(*) filter (where target is null)::bigint as unmapped from data_mappings group by kind order by kind;

-- name: StartImportRun :one
insert into import_runs (source, entity, started_by) values ($1, $2, $3) returning id;

-- name: FinishImportRun :exec
update import_runs set status = $2, rows = $3, upserted = $4, skipped = $5, unmapped = $6, error = $7, notes = $8, finished_at = now() where id = $1;

-- name: ListImportRuns :many
select * from import_runs order by started_at desc limit $1;

-- name: LastImportRun :one
select * from import_runs where source = $1 and status = 'done' order by started_at desc limit 1;

-- name: GetSecret :one
select value from secrets where key = $1;

-- name: SetSecret :exec
insert into secrets (key, value, updated_by, updated_at) values ($1, $2, $3, now())
on conflict (key) do update set value = excluded.value, updated_by = excluded.updated_by, updated_at = now();

-- name: DeleteSecret :exec
delete from secrets where key = $1;

-- name: UpsertImportedSalesUser :one
insert into sales_users (name, branch, wa_number, role, email, external_name, source_system, source_id)
values ($1, $2, $3, 'sales', $4, $5, 'import', $6)
on conflict (source_system, source_id) do update
  set name = excluded.name, branch = excluded.branch, wa_number = coalesce(excluded.wa_number, sales_users.wa_number),
      email = coalesce(excluded.email, sales_users.email), external_name = excluded.external_name
returning id;

-- name: ImportDealer :one
-- Fields people set by hand in Pengaturan (master_locked) keep their value.
insert into dealers (slug, name, city, branch, tier, segment_desc, owner_id, credit_limit, payment_terms_days, customer_type, phone,
                     source_system, source_id, source_write_date)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'import', $12, now())
on conflict (source_system, source_id) do update
  set name = excluded.name, city = coalesce(excluded.city, dealers.city), branch = excluded.branch, phone = coalesce(excluded.phone, dealers.phone),
      segment_desc = coalesce(excluded.segment_desc, dealers.segment_desc),
      tier = case when 'tier' = any(dealers.master_locked) then dealers.tier else coalesce(excluded.tier, dealers.tier) end,
      owner_id = case when 'owner_id' = any(dealers.master_locked) then dealers.owner_id else coalesce(excluded.owner_id, dealers.owner_id) end,
      credit_limit = case when 'credit_limit' = any(dealers.master_locked) or excluded.credit_limit = 0 then dealers.credit_limit else excluded.credit_limit end,
      payment_terms_days = case when 'payment_terms_days' = any(dealers.master_locked) then dealers.payment_terms_days else excluded.payment_terms_days end,
      customer_type = case when 'customer_type' = any(dealers.master_locked) then dealers.customer_type else excluded.customer_type end,
      source_write_date = now(), updated_at = now()
returning id;

-- name: ImportContact :exec
insert into contacts (dealer_id, name, role, wa_number, is_primary, source_system, source_id)
values ($1, $2, 'PIC', $3, true, 'import', $4)
on conflict (source_system, source_id) do update set name = excluded.name, wa_number = excluded.wa_number;

-- name: ImportProduct :exec
insert into products (sku, name, category, list_price, cost, active, source_system, source_id)
values ($1, $2, $3, $4, $5, true, 'import', $1)
on conflict (source_system, source_id) do update
  set name = excluded.name, category = coalesce(excluded.category, products.category),
      list_price = case when excluded.list_price > 0 then excluded.list_price else products.list_price end,
      cost = case when excluded.cost > 0 then excluded.cost else products.cost end;

-- name: DeleteImportedStock :exec
delete from stock_items where source_system = 'import';

-- name: ImportedDealerIDs :many
select source_id, id from dealers where source_system = 'import';

-- name: MasterCustomers :many
select d.id, d.slug, d.name, d.city, d.branch, d.tier, d.customer_type, d.credit_limit, d.payment_terms_days, d.phone, d.master_locked,
  s.name as owner_name, d.owner_id, coalesce((d.metrics_current->>'omzet_bln')::bigint, 0) as omzet_bln,
  coalesce(d.metrics_current->>'status', '') as status
from dealers d left join sales_users s on s.id = d.owner_id
where (sqlc.arg(q)::text = '' or d.name ilike '%' || sqlc.arg(q)::text || '%' or d.city ilike '%' || sqlc.arg(q)::text || '%')
  and (sqlc.arg(ctype)::text = '' or d.customer_type = sqlc.arg(ctype)::text)
  and (not sqlc.arg(incomplete)::bool or d.credit_limit = 0 or d.owner_id is null or d.tier is null)
order by coalesce((d.metrics_current->>'omzet_bln')::bigint, 0) desc, d.name
limit sqlc.arg(lim) offset sqlc.arg(off);

-- name: UpdateCustomerMaster :exec
update dealers set
  customer_type = coalesce(sqlc.narg(customer_type)::text, customer_type),
  tier = coalesce(sqlc.narg(tier)::text, tier),
  credit_limit = coalesce(sqlc.narg(credit_limit)::bigint, credit_limit),
  payment_terms_days = coalesce(sqlc.narg(payment_terms_days)::int, payment_terms_days),
  owner_id = coalesce(sqlc.narg(owner_id)::uuid, owner_id),
  master_locked = (select array_agg(distinct x) from unnest(master_locked || sqlc.arg(locked)::text[]) x),
  updated_at = now()
where id = sqlc.arg(id)::uuid;

-- name: ListTeam :many
select s.id, s.name, s.branch, s.role, s.wa_number, s.email, s.external_name, s.active,
  (select count(*) from dealers d where d.owner_id = s.id)::bigint as dealers,
  (select u.email from users u where u.sales_user_id = s.id limit 1) as login_email
from sales_users s order by case s.role when 'sales' then 0 else 1 end, s.branch, s.name;

-- name: CreateSalesProfile :one
insert into sales_users (name, branch, wa_number, role, email, external_name) values ($1, $2, $3, $4, $5, $6) returning id;

-- name: UpdateSalesProfile :exec
update sales_users set name = $2, branch = $3, wa_number = $4, active = $5 where id = $1;

-- name: Branches :many
-- Active branches of the branch master, for pickers.
select name from branches where active order by name;

-- name: DeleteImportRowsPrefix :exec
delete from import_rows where entity = $1 and key like sqlc.arg(prefix)::text || '%';

-- name: ListImportRowsSince :many
select key, data from import_rows where entity = $1 and updated_at >= $2 order by key;

-- name: LastApplyStart :one
select coalesce(max(started_at), '1970-01-01'::timestamptz)::timestamptz from import_runs where entity = 'apply' and status = 'done';

-- name: SalesProfilesForMatch :many
select id, name, coalesce(external_name, '') as external_name from sales_users;
