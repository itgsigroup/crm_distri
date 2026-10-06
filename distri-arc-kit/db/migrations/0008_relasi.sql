-- +goose Up
-- Stage 08: interactions per (sales number, dealer, month) for Peta relasi. `seed` rows are history imported
-- before Distri ARC (WhatsApp backfill up to as_of); everything after as_of is counted live from chat_messages
-- and orders. Contacts keep the imported 90-day baseline so the daily recount does not erase it.
create table interactions_monthly (
  sales_id uuid not null references sales_users on delete cascade,
  dealer_id uuid not null references dealers on delete cascade,
  month date not null,                 -- first day of the month (WIB)
  n int not null,
  source text not null default 'seed', -- seed | import
  as_of timestamptz not null,          -- live counting starts after this moment
  primary key (sales_id, dealer_id, month, source)
);
alter table contacts add column interactions_base int, add column base_as_of timestamptz;

-- +goose Down
alter table contacts drop column base_as_of, drop column interactions_base;
drop table if exists interactions_monthly;
