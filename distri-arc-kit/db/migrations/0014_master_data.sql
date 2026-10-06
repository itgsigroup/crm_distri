-- +goose Up
-- Real data (BigQuery / CSV import) and master data: customer types, source → Distri ARC mappings, import runs,
-- sealed secrets. Dealers keep the fields people set by hand (master_locked) when an import runs again.
alter table dealers add column customer_type text not null default 'reseller' check (customer_type in ('reseller','si')),
  add column master_locked text[] not null default '{}',     -- fields edited in Pengaturan: tier, credit_limit, customer_type, owner_id, payment_terms_days
  add column phone text;
create index dealers_customer_type on dealers (customer_type);

-- source value → target, per kind:
--   branch    "Kantor Pusat"        → "Semarang"
--   warehouse "01. LAMPER"          → "Semarang"
--   category  "Hikvision NVR"       → "Kamera & NVR" (one of the 6 KAT)
--   sales     "ANDI WIBOWO"         → sales_users.id
--   ctype     "RESELLER" / "SI"     → reseller | si
-- target null = seen in the data, not mapped yet (Pengaturan → Data & master lists them).
create table data_mappings (
  kind text not null check (kind in ('branch','warehouse','category','sales','ctype')),
  source_value text not null,
  target text,
  seen int not null default 0,                -- rows of the last import that carried it
  updated_by text,
  updated_at timestamptz not null default now(),
  primary key (kind, source_value)
);

create table import_runs (
  id uuid primary key default gen_random_uuid(),
  source text not null,                        -- bigquery | csv
  entity text not null,                        -- sales | customers | invoices | invoice_lines | stock
  status text not null default 'running' check (status in ('running','done','failed')),
  rows int not null default 0,
  upserted int not null default 0,
  skipped int not null default 0,
  unmapped int not null default 0,
  error text,
  notes jsonb,
  started_by text,
  started_at timestamptz not null default now(),
  finished_at timestamptz
);
create index import_runs_recent on import_runs (started_at desc);

-- secrets sealed with SESSION_SECRET (AES-GCM): BigQuery service account JSON …
create table secrets (
  key text primary key,
  value text not null,
  updated_by text,
  updated_at timestamptz not null default now()
);

alter table sales_users add column email text, add column external_name text;

-- raw rows as the source delivered them (latest per key); the transform re-runs from here when a mapping changes
create table import_rows (
  entity text not null,
  key text not null,
  data jsonb not null,
  source text not null,
  updated_at timestamptz not null default now(),
  primary key (entity, key)
);

-- +goose Down
drop table if exists import_rows;
alter table sales_users drop column external_name, drop column email;
drop table if exists secrets;
drop table if exists import_runs;
drop table if exists data_mappings;
drop index if exists dealers_customer_type;
alter table dealers drop column phone, drop column master_locked, drop column customer_type;
