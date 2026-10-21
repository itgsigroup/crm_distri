-- Cek stok: what GSI Orbit reads for one product next to the real stock a user counted.

-- name: SearchStockProducts :many
-- Products in the stock by name or SKU, with their total over every branch.
select sku, max(name)::text as name, sum(qty)::bigint as qty, count(*)::bigint as branches
from stock_items
where sqlc.arg(q)::text = '' or name ilike '%' || sqlc.arg(q)::text || '%' or sku ilike '%' || sqlc.arg(q)::text || '%'
group by sku
order by (lower(sku) = lower(sqlc.arg(q)::text)) desc, max(name)
limit sqlc.arg(lim);

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
