-- +goose Up
-- Many WhatsApp numbers (Baileys bridge, ADR 0017): a conversation belongs to the number that holds it (account),
-- not only to a sales user — one sales can link several numbers and a team number (CS, kantor) has no sales owner.
alter table chat_threads add column account text;
update chat_threads t set account = s.wa_number from sales_users s where s.id = t.sales_id and t.account is null;
alter table chat_threads drop constraint chat_threads_sales_jid_key;
create unique index chat_threads_account_jid on chat_threads (account, wa_jid);
create index chat_threads_account on chat_threads (account);

alter table wa_numbers drop constraint wa_numbers_transport_check;
alter table wa_numbers add constraint wa_numbers_transport_check check (transport in ('fake','whatsmeow','cloudapi','baileys'));
alter table wa_numbers add column created_at timestamptz not null default now();

-- +goose Down
alter table wa_numbers drop column created_at;
alter table wa_numbers drop constraint wa_numbers_transport_check;
update wa_numbers set transport = 'fake' where transport = 'baileys';
alter table wa_numbers add constraint wa_numbers_transport_check check (transport in ('fake','whatsmeow','cloudapi'));
drop index if exists chat_threads_account;
drop index if exists chat_threads_account_jid;
alter table chat_threads add constraint chat_threads_sales_jid_key unique (sales_id, wa_jid);
alter table chat_threads drop column account;
