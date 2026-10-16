-- +goose Up
-- DMs between GSI employees are kept and shown in Chat (Sam 16 Oct: "DM antar karyawan tetap ada; internal itu pada
-- analisa tidak termasuk"). A 1:1 chat with a GSI number (internal, linked, a user's or a sales') is its own thread
-- kind, read by no analysis (Analisa WhatsApp reads dealer/new threads and customer groups); its messages are internal.
alter table chat_threads drop constraint chat_threads_kind_check;
alter table chat_threads add constraint chat_threads_kind_check check (kind in ('dealer', 'group', 'new', 'internal'));

with gsi as (
  select wa_number from internal_numbers union select wa_number from wa_numbers
  union select wa_number from users where wa_number is not null union select wa_number from sales_users where wa_number is not null
), th as (
  update chat_threads t set kind = 'internal', subtitle = 'Karyawan GSI', identification = null
  where t.kind = 'new' and split_part(t.wa_jid, '@', 1) in (select wa_number from gsi)
  returning t.id
)
update chat_messages m set internal = true from th where m.thread_id = th.id and not m.internal;

-- +goose Down
update chat_threads set kind = 'new' where kind = 'internal';
alter table chat_threads drop constraint chat_threads_kind_check;
alter table chat_threads add constraint chat_threads_kind_check check (kind in ('dealer', 'group', 'new'));
