-- +goose Up
-- Stage 03: WhatsApp numbers (one per paired sales phone), thread uniqueness per sales number, presentation
-- hints, message delivery state. The whatsmeow device store lives in its own schema; its tables are created by
-- `arc ctl migrate` (whatsmeow's own versioned upgrades), never at runtime.
create schema if not exists whatsmeow;

create table wa_numbers (
  wa_number text primary key,
  sales_id uuid references sales_users,
  label text,
  transport text not null default 'fake' check (transport in ('fake','whatsmeow','cloudapi')),
  jid text,
  state text not null default 'unpaired' check (state in ('unpaired','pairing','connected','disconnected','logged_out')),
  qr text, qr_expires_at timestamptz,
  last_seen_at timestamptz, paired_at timestamptz,
  backfill_days int not null default 30,
  updated_at timestamptz not null default now()
);

alter table chat_threads drop constraint chat_threads_wa_jid_key;
alter table chat_threads add constraint chat_threads_sales_jid_key unique (sales_id, wa_jid);
alter table chat_threads add column tag jsonb, add column suggestions jsonb, add column seed_key text unique;
create index chat_threads_recent on chat_threads (last_message_at desc);

alter table chat_messages add column status text not null default 'received'
  check (status in ('received','pending','sent','failed')),
  add column proposal_id uuid references proposals on delete set null,
  add column internal bool not null default false;

-- +goose Down
alter table chat_messages drop column internal, drop column proposal_id, drop column status;
drop index if exists chat_threads_recent;
alter table chat_threads drop column seed_key, drop column suggestions, drop column tag;
alter table chat_threads drop constraint chat_threads_sales_jid_key;
alter table chat_threads add constraint chat_threads_wa_jid_key unique (wa_jid);
drop table if exists wa_numbers;
drop schema if exists whatsmeow cascade;
