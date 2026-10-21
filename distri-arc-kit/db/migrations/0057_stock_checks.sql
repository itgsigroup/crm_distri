-- +goose Up
-- Cek stok (21 Oct): a user counts a product's real stock (Accurate, the warehouse) next to what GSI Orbit and the AI
-- Trigger read from BigQuery, one row per branch. The difference shows whether the AI or the data is wrong.
create table stock_checks (
  id uuid primary key default gen_random_uuid(),
  sku text not null,
  product text not null,
  branch text not null,
  arc_qty int not null,  -- stock_items.qty at the time of the check (what the AI reads)
  real_qty int not null, -- counted by the user
  note text not null default '',
  synced_at timestamptz, -- when GSI Orbit last read the stock from its source
  checked_by uuid references users on delete set null,
  checked_by_name text not null default '',
  checked_at timestamptz not null default now()
);
create index stock_checks_sku on stock_checks (sku, checked_at desc);
create index stock_checks_latest on stock_checks (checked_at desc);

-- +goose Down
drop table if exists stock_checks;
