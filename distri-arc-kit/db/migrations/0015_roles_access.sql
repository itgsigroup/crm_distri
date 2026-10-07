-- +goose Up
-- Master peran & akses (ADR 0019): a role narrows what its base system role may do (menu screens, proposal kinds it
-- decides, whether its users hold a WhatsApp number). users.role stays the base (all server checks), users.role_key
-- names the role. One user holds at most one WhatsApp number (users.wa_number, wa_numbers.user_id).
create table roles (
  key text primary key,
  name text not null,
  description text not null default '',
  base text not null check (base in ('ceo','admin','finance','sales','warehouse')),
  screens text[],              -- null = every screen of the base
  decide text[],               -- null = every proposal kind the base decides
  wa_allowed bool not null default true,
  system bool not null default false,
  active bool not null default true,
  updated_by text,
  updated_at timestamptz not null default now()
);
insert into roles (key, name, description, base, wa_allowed, system) values
  ('ceo', 'CEO', 'Semua layar · kebijakan · rilis kredit', 'ceo', true, true),
  ('admin', 'Admin', 'Semua layar · pengguna · keputusan kecuali kredit', 'admin', true, true),
  ('finance', 'Finance', 'Kredit · kas · penagihan · limit', 'finance', true, true),
  ('sales', 'Sales', 'Dealer miliknya · chat nomornya', 'sales', true, true),
  ('warehouse', 'Gudang', 'Push stok · transfer · PO', 'warehouse', true, true);

alter table users add column role_key text references roles (key),
  add column wa_number text unique;
update users set role_key = role where role in ('ceo','admin','finance','sales','warehouse');
with s as (
  select u.id, s.wa_number, row_number() over (partition by s.wa_number order by u.email) as rn
  from users u join sales_users s on s.id = u.sales_user_id where s.wa_number is not null
)
update users u set wa_number = s.wa_number from s where s.id = u.id and s.rn = 1;

alter table wa_numbers add column user_id uuid unique references users (id) on delete set null;
with m as (
  select n.wa_number, u.id, row_number() over (partition by u.id order by n.wa_number) as rn
  from wa_numbers n join users u on u.wa_number = n.wa_number
)
update wa_numbers n set user_id = m.id from m where m.wa_number = n.wa_number and m.rn = 1;

-- +goose Down
alter table wa_numbers drop column user_id;
alter table users drop column wa_number, drop column role_key;
drop table roles;
