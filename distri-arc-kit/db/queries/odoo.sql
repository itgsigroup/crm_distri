-- name: GetSyncState :one
select * from odoo_sync_state where model = $1;

-- name: ListSyncState :many
select * from odoo_sync_state order by model;

-- name: SetSyncState :exec
insert into odoo_sync_state (model, last_write_date, last_run_at, records, error)
values ($1, $2, $3, $4, $5)
on conflict (model) do update set last_write_date = coalesce(excluded.last_write_date, odoo_sync_state.last_write_date),
  last_run_at = excluded.last_run_at, records = excluded.records, error = excluded.error;

-- name: ResetSyncState :exec
delete from odoo_sync_state;

-- name: ListCategoryMap :many
select * from category_map order by odoo_category_id;

-- name: UpsertProduct :exec
insert into products (sku, name, category, odoo_category_id, list_price, cost, prices, source_system, source_id, source_write_date)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (source_system, source_id) do update set sku = excluded.sku, name = excluded.name, category = excluded.category,
  odoo_category_id = excluded.odoo_category_id, list_price = excluded.list_price, cost = excluded.cost, prices = excluded.prices,
  source_write_date = excluded.source_write_date;

-- name: ListProducts :many
select * from products where active order by name;

-- name: GetSalesByOdooUser :one
select * from sales_users where odoo_user_id = $1;

-- name: DealerIDBySource :one
select id, slug from dealers where source_system = $1 and source_id = $2;

-- name: OrderIDBySource :one
select id from orders where source_system = $1 and source_id = $2;

-- name: InvoiceIDBySource :one
select id from invoices where source_system = $1 and source_id = $2;

-- name: UpsertContactFromOdoo :one
-- Odoo owns name, role and number; interaction counters are computed by GSI Orbit and kept.
insert into contacts (dealer_id, name, role, wa_number, source_system, source_id)
values ($1, $2, $3, $4, $5, $6)
on conflict (source_system, source_id) do update set name = excluded.name, role = excluded.role, wa_number = excluded.wa_number, dealer_id = excluded.dealer_id
returning id;

-- name: SignalExists :one
select exists(select 1 from signals where dedupe_key = $1)::bool;
