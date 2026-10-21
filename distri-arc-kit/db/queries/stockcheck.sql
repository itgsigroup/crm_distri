-- Cek stok: what GSI Orbit reads for one product next to the real stock a user counted.

-- name: StockItemsBySKU :many
select * from stock_items where lower(sku) = lower(sqlc.arg(sku)::text) order by qty desc, branch;

-- name: StockSourceRows :many
-- The source rows (BigQuery / CSV) of one SKU as delivered, one per warehouse.
select key, data, source, updated_at from import_rows
where entity = 'stock' and lower(split_part(key, '|', 1)) = lower(sqlc.arg(sku)::text) order by key;

-- name: InsertStockCheck :one
insert into stock_checks (sku, product, branch, arc_qty, real_qty, note, synced_at, checked_by, checked_by_name)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9) returning id;

-- name: ListStockChecks :many
select * from stock_checks
where sqlc.arg(sku)::text = '' or lower(sku) = lower(sqlc.arg(sku)::text)
order by checked_at desc, branch
limit sqlc.arg(lim);

-- name: ListStockProducts :many
-- Every product in the stock (Cek stok's table): total and quantity per branch.
select sku, max(name)::text as name, max(category)::text as category, sum(qty)::bigint as qty,
  jsonb_object_agg(branch, qty) as by_branch
from stock_items group by sku order by max(name);

-- name: LastStockChecks :many
-- The last saved check per product (one save is one transaction, so its rows share checked_at) and how many of its
-- branches differed.
select lower(c.sku)::text as sku, c.checked_at, count(*)::bigint as branches, count(*) filter (where c.real_qty <> c.arc_qty)::bigint as off
from stock_checks c
where c.checked_at = (select max(x.checked_at) from stock_checks x where lower(x.sku) = lower(c.sku))
group by lower(c.sku), c.checked_at;
