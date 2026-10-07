-- +goose Up
-- Roles become fully custom (ADR 0020): each role ticks pages, a data scope (all / own), the proposal kinds it decides
-- and whether it holds policy rights (CEO-level). roles.base is derived on save (ceo / sales / admin / finance) and
-- copied to users.role, which every existing server check reads. Branch master. WhatsApp numbers are linked first
-- (QR / code in Chat) and then given to a user: a link session gets its id before the phone number is known.
alter table roles add column scope text not null default 'all' check (scope in ('all','own')),
  add column policies bool not null default false;
update roles set policies = true where key = 'ceo';
update roles set scope = 'own' where base = 'sales';
update roles set screens = array['today','orch','chat','orbit','kuad','net','dealer','stock','ar','users','roles','branches','conn','konsep'] where key in ('ceo','admin');
update roles set screens = array['today','orch','chat','orbit','kuad','net','dealer','stock','konsep'] where key = 'sales' and screens is null;
update roles set screens = array['today','orch','orbit','kuad','dealer','ar','konsep'] where key = 'finance' and screens is null;
update roles set screens = array['today','chat','stock','dealer','konsep'] where key = 'warehouse' and screens is null;
update roles set decide = array['followup','collect','installment','credit_release','credit_limit','credit_hold','price_counter','push_stock','so_draft','return','transfer','po_request','new_dealer','price_list','reply','plan_change'] where key = 'ceo';
update roles set decide = array['followup','collect','installment','credit_hold','price_counter','push_stock','so_draft','return','transfer','po_request','new_dealer','price_list','reply','plan_change'] where key = 'admin' and decide is null;
update roles set decide = array['collect','installment','credit_limit'] where key = 'finance' and decide is null;
update roles set decide = array['followup','collect','installment','so_draft','return','new_dealer','price_list','reply'] where key = 'sales' and decide is null;
update roles set decide = array['transfer','po_request'] where key = 'warehouse' and decide is null;
update roles set system = false;

create table branches (
  id uuid primary key default gen_random_uuid(),
  name text not null unique,
  city text not null default '',
  address text not null default '',
  active bool not null default true,
  updated_by text,
  created_at timestamptz not null default now()
);
insert into branches (name)
select distinct b from (select branch as b from sales_users union select branch from dealers) x
where b is not null and b <> '' and b <> 'Semua cabang'
on conflict do nothing;

alter table wa_numbers add column session_id text unique;
update wa_numbers set session_id = wa_number;
alter table wa_numbers alter column session_id set not null;

create table wa_links (
  session_id text primary key,
  method text not null default 'qr' check (method in ('qr','code')),
  phone text,                  -- the number the code is requested for (method code)
  state text not null default 'pairing' check (state in ('pairing','connected','failed')),
  qr text,
  wa_number text,              -- the linked number once WhatsApp reports it
  error text,
  created_by text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- +goose Down
drop table wa_links;
alter table wa_numbers drop column session_id;
drop table branches;
alter table roles drop column policies, drop column scope;
