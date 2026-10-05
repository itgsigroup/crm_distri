-- +goose Up
-- Stage 04: Odoo read-only sync cursors and the product catalog (AI Order matches WA requests against it).
create table odoo_sync_state (
  model text primary key,
  last_write_date timestamptz,
  last_run_at timestamptz,
  records int not null default 0,
  error text
);

create table products (
  id uuid primary key default gen_random_uuid(),
  sku text,
  name text not null,
  category text,                                   -- one of the 6 KAT (via category_map)
  odoo_category_id int,
  list_price bigint not null default 0,            -- tier A
  cost bigint not null default 0,
  prices jsonb not null default '{}',              -- {"A":…, "B":…, "C":…}
  active bool not null default true,
  source_system text, source_id text, source_write_date timestamptz,
  unique (source_system, source_id)
);
create index products_name_trgm on products using gin (name gin_trgm_ops);

-- +goose Down
drop table if exists products;
drop table if exists odoo_sync_state;
