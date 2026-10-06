-- +goose Up
-- Stage 13: monthly partitions for signals and chat_messages (data moved), global dedupe through key tables
-- (a unique constraint on a partitioned table must contain the partition key), 2FA TOTP, and a system outbox for
-- internal alerts.

-- arc_attach_month creates the partition of one month for a table partitioned by range, moving rows that already
-- sit in the default partition (an attach fails while the default holds rows of the new range).
-- +goose StatementBegin
create or replace function arc_attach_month(tbl text, col text, m date) returns bool language plpgsql as $$
declare
  part text := format('%s_y%sm%s', tbl, to_char(m, 'YYYY'), to_char(m, 'MM'));
  lo timestamptz := m::timestamptz;
  hi timestamptz := (m + interval '1 month')::timestamptz;
begin
  if to_regclass(format('%I.%I', current_schema(), part)) is not null then
    return false;
  end if;
  execute format('create table %I (like %I including defaults including constraints)', part, tbl);
  if to_regclass(format('%I.%I', current_schema(), tbl || '_default')) is not null then
    execute format('with moved as (delete from %I where %I >= $1 and %I < $2 returning *) insert into %I select * from moved',
                   tbl || '_default', col, col, part) using lo, hi;
  end if;
  execute format('alter table %I attach partition %I for values from (%L) to (%L)', tbl, part, lo, hi);
  return true;
end $$;
-- +goose StatementEnd

-- arc_ensure_partitions makes sure every month from `from_month` through `months_ahead` months after `now_at` has
-- its partition (job partitions.ensure, daily).
-- +goose StatementBegin
create or replace function arc_ensure_partitions(tbl text, col text, from_month date, now_at timestamptz, months_ahead int) returns int language plpgsql as $$
declare
  m date := date_trunc('month', from_month)::date;
  stop date := (date_trunc('month', now_at) + make_interval(months => months_ahead))::date;
  n int := 0;
begin
  while m <= stop loop
    if arc_attach_month(tbl, col, m) then n := n + 1; end if;
    m := (m + interval '1 month')::date;
  end loop;
  return n;
end $$;
-- +goose StatementEnd

-- arc_drop_partitions_before drops whole monthly partitions that end on or before the cutoff (retention.purge).
-- +goose StatementBegin
create or replace function arc_drop_partitions_before(tbl text, cutoff timestamptz) returns int language plpgsql as $$
declare
  r record;
  hi timestamptz;
  n int := 0;
begin
  for r in select c.relname from pg_inherits i join pg_class c on c.oid = i.inhrelid join pg_class p on p.oid = i.inhparent
           join pg_namespace ns on ns.oid = p.relnamespace
           where p.relname = tbl and ns.nspname = current_schema() and c.relname ~ '_y[0-9]{4}m[0-9]{2}$' loop
    hi := (to_date(right(r.relname, 8), '"y"YYYY"m"MM') + interval '1 month')::timestamptz;
    if hi <= cutoff then
      execute format('drop table %I', r.relname);
      n := n + 1;
    end if;
  end loop;
  return n;
end $$;
-- +goose StatementEnd

-- ---------- signals ----------
alter table chat_messages drop constraint chat_messages_signal_id_fkey;
alter table signals rename to signals_old;
alter index signals_pkey rename to signals_old_pkey;
alter index signals_dealer_time rename to signals_old_dealer_time;
alter table signals_old rename constraint signals_dedupe_key_key to signals_old_dedupe_key_key;
alter table signals_old rename constraint signals_kind_check to signals_old_kind_check;

create table signals (
  id uuid not null default gen_random_uuid(),
  kind text not null check (kind in ('wa','wa_group','so','invoice','payment','stock','manual')),
  dealer_id uuid references dealers on delete set null,
  contact_id uuid references contacts on delete set null,
  sales_id uuid references sales_users,
  occurred_at timestamptz not null,
  dedupe_key text not null,                            -- unique through signal_keys
  summary text,
  payload jsonb not null,
  processed_at timestamptz,
  created_at timestamptz not null default now(),
  primary key (id, occurred_at)
) partition by range (occurred_at);
create table signals_default partition of signals default;
create index signals_dealer_time on signals (dealer_id, occurred_at desc);
create index signals_id on signals (id);
create index signals_dedupe on signals (dedupe_key);
create index signals_kind_time on signals (kind, occurred_at desc);
-- Timeline dealer reads only signals with an agent conclusion or a reply link (docs/PERF.md)
create index signals_timeline on signals (dealer_id, occurred_at desc) where payload ? 'conclusion' or payload ? 'reply_to';

-- one row per dedupe key: wa message id, odoo model:id:write_date … (idempotent ingest across partitions)
create table signal_keys (
  dedupe_key text primary key,
  signal_id uuid not null,
  occurred_at timestamptz not null
);
create index signal_keys_time on signal_keys (occurred_at);

select arc_ensure_partitions('signals', 'occurred_at',
  greatest(coalesce((select min(occurred_at) from signals_old), now()), now() - interval '36 months')::date, now(), 3);
insert into signals (id, kind, dealer_id, contact_id, sales_id, occurred_at, dedupe_key, summary, payload, processed_at, created_at)
  select id, kind, dealer_id, contact_id, sales_id, occurred_at, dedupe_key, summary, payload, processed_at, created_at from signals_old;
insert into signal_keys (dedupe_key, signal_id, occurred_at) select dedupe_key, id, occurred_at from signals_old;
drop table signals_old;

-- ---------- chat_messages ----------
alter table chat_messages rename to chat_messages_old;
alter index chat_messages_pkey rename to chat_messages_old_pkey;
alter index chat_messages_thread_time rename to chat_messages_old_thread_time;
alter table chat_messages_old rename constraint chat_messages_wa_msg_id_key to chat_messages_old_wa_msg_id_key;
alter table chat_messages_old rename constraint chat_messages_direction_check to chat_messages_old_direction_check;
alter table chat_messages_old rename constraint chat_messages_status_check to chat_messages_old_status_check;
alter table chat_messages_old rename constraint chat_messages_proposal_id_fkey to chat_messages_old_proposal_id_fkey;
alter table chat_messages_old rename constraint chat_messages_thread_id_fkey to chat_messages_old_thread_id_fkey;

create table chat_messages (
  id uuid not null default gen_random_uuid(),
  thread_id uuid references chat_threads on delete cascade,
  wa_msg_id text,                                      -- unique through chat_message_keys
  direction text check (direction in ('in','out')),
  from_number text, from_name text, body text, media jsonb,
  sent_at timestamptz not null,
  annotation jsonb,
  signal_id uuid,                                      -- signals are partitioned: no FK, purged later than chat
  status text not null default 'received' check (status in ('received','pending','sent','failed')),
  proposal_id uuid references proposals on delete set null,
  internal bool not null default false,
  primary key (id, sent_at)
) partition by range (sent_at);
create table chat_messages_default partition of chat_messages default;
create index chat_messages_thread_time on chat_messages (thread_id, sent_at);
create index chat_messages_id on chat_messages (id);
create index chat_messages_wa_msg on chat_messages (wa_msg_id);
create index chat_messages_proposal on chat_messages (proposal_id) where proposal_id is not null;

create table chat_message_keys (
  wa_msg_id text primary key,
  message_id uuid not null,
  sent_at timestamptz not null
);
create index chat_message_keys_time on chat_message_keys (sent_at);

select arc_ensure_partitions('chat_messages', 'sent_at',
  greatest(coalesce((select min(sent_at) from chat_messages_old), now()), now() - interval '36 months')::date, now(), 3);
insert into chat_messages (id, thread_id, wa_msg_id, direction, from_number, from_name, body, media, sent_at, annotation, signal_id, status, proposal_id, internal)
  select id, thread_id, wa_msg_id, direction, from_number, from_name, body, media, sent_at, annotation, signal_id, status, proposal_id, internal from chat_messages_old;
insert into chat_message_keys (wa_msg_id, message_id, sent_at)
  select coalesce(wa_msg_id, 'local:' || id), id, sent_at from chat_messages_old;
drop table chat_messages_old;

-- ---------- 2FA TOTP (ceo/admin) ----------
alter table users add column totp_secret text,              -- AES-GCM sealed with SESSION_SECRET
  add column totp_enabled_at timestamptz,
  add column totp_last_step bigint;                         -- last accepted 30-s step (no replay)

-- ---------- system outbox + alerts (internal WhatsApp group only, never a dealer) ----------
alter table outbox alter column proposal_id drop not null;
alter table outbox drop constraint outbox_channel_check;
alter table outbox add constraint outbox_channel_check check (channel in ('wa','odoo_so_draft','odoo_note','wa_system'));
alter table outbox add constraint outbox_system_only check ((proposal_id is null) = (channel = 'wa_system'));

create table system_alerts (
  key text primary key,                                -- wa_disconnected:<number> | cycle_failed | queue_depth
  message text not null,
  opened_at timestamptz not null,
  notified_at timestamptz,
  resolved_at timestamptz
);

-- +goose Down
drop table if exists system_alerts;
delete from outbox where channel = 'wa_system';
alter table outbox drop constraint outbox_system_only;
alter table outbox drop constraint outbox_channel_check;
alter table outbox add constraint outbox_channel_check check (channel in ('wa','odoo_so_draft','odoo_note'));
alter table outbox alter column proposal_id set not null;
alter table users drop column totp_last_step, drop column totp_enabled_at, drop column totp_secret;

create table chat_messages_plain (like chat_messages including defaults including constraints);
insert into chat_messages_plain select * from chat_messages;
drop table chat_messages;
drop table chat_message_keys;
alter table chat_messages_plain rename to chat_messages;
alter table chat_messages add primary key (id), add unique (wa_msg_id),
  add foreign key (thread_id) references chat_threads on delete cascade,
  add foreign key (proposal_id) references proposals on delete set null;
create index chat_messages_thread_time on chat_messages (thread_id, sent_at);

create table signals_plain (like signals including defaults including constraints);
insert into signals_plain select * from signals;
drop table signals;
drop table signal_keys;
alter table signals_plain rename to signals;
alter table signals add primary key (id), add unique (dedupe_key),
  add foreign key (dealer_id) references dealers on delete set null,
  add foreign key (contact_id) references contacts on delete set null,
  add foreign key (sales_id) references sales_users;
create index signals_dealer_time on signals (dealer_id, occurred_at desc);
alter table chat_messages add foreign key (signal_id) references signals on delete set null;
drop function if exists arc_drop_partitions_before(text, timestamptz);
drop function if exists arc_ensure_partitions(text, text, date, timestamptz, int);
drop function if exists arc_attach_month(text, text, date);
