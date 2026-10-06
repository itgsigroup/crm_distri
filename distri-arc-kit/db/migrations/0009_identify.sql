-- +goose Up
-- Stage 09: manual Getcontact imports (CSV) kept per number, and when a number was last identified.
create table getcontact_imports (
  wa_number text not null,
  name text not null,
  tags int not null default 0,
  imported_by uuid references sales_users on delete set null,
  imported_at timestamptz not null default now(),
  primary key (wa_number, name)
);
alter table identifications add column identified_at timestamptz, add column potential text;

-- +goose Down
alter table identifications drop column potential, drop column identified_at;
drop table if exists getcontact_imports;
