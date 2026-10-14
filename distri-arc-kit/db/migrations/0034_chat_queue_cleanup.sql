-- +goose Up
-- Chat queue and response time per the director's chat review of 14 Oct (ADR 0035): the "perlu dibalas" queue holds
-- only commerce, and response time is substantive and split by working hours.

-- Contact type of a number: a supplier (pemasok) is kept but is not a customer lead; a personal chat (pribadi) is not
-- kept in the CRM at all. A number with no row is a customer (or internal, through internal_numbers).
create table contact_types (
  wa_number text primary key,
  type text not null check (type in ('supplier','personal')),
  set_by text,
  set_at timestamptz not null default now()
);

-- A customer's message that asks nothing: a polite closing ("ok bu", "siap", "terima kasih 🙏"), emoji or a sticker.
-- The same words as waanalysis.IsAck.
-- +goose StatementBegin
create function is_closing_message(body text) returns bool language sql immutable parallel safe as $fn$
  select case
    when coalesce(btrim(body), '') = '' or length(btrim(body)) > 40 then false
    when lower(body) !~ '[a-z]' then lower(body) !~ '[0-9?]'
    else not exists (
      select 1 from regexp_split_to_table(lower(body), '[^a-z]+') w
      where w <> '' and w <> all (array['ok','oke','okee','okey','okay','okeh','okwy','okk','okok','siap','baik','sip','sipp',
        'mantap','mantab','terima','kasih','makasih','trims','tq','thx','thanks','thank','you','sama','samaa','noted','iya','ya',
        'yaa','y','bu','pak','mba','mbak','mas','kak','ko','ci','gan','bro','sis','banyak','stiker','gif']))
  end
$fn$;
-- +goose StatementEnd

-- An automatic message of a WhatsApp Business account (greeting, away message), ours or the customer's: not an answer
-- and not a request.
-- +goose StatementBegin
create function is_auto_reply(body text) returns bool language sql immutable parallel safe as $fn$
  select coalesce(length(body), 0) between 1 and 700 and lower(body) ~ ('('
    || 'terima\s*kasih\s+(telah|sudah|atas)\s+(menghubungi|menghubungin|chat|pesan)'
    || '|kami\s+akan\s+segera\s+(membalas|merespon|merespons|menghubungi|menjawab)'
    || '|(sedang|saat\s+ini|lagi)\s+(tidak|tdk|gak|ga)\s+(ada\s+)?di\s+tempat|tidak\s+ada\s+di\s+tempat'
    || '|di\s+luar\s+jam\s+(kerja|operasional|kantor|layanan)'
    || '|jam\s+(operasional|kerja|layanan)\s+(kami|kantor|toko)'
    || '|(pesan|balasan)\s+otomatis|auto\s*-?\s*reply'
    || '|selamat\s+datang\s+di\s+\w+'
    || ')')
$fn$;
-- +goose StatementEnd

alter table chat_messages add column closing bool generated always as (is_closing_message(body)) stored,
  add column auto_reply bool generated always as (is_auto_reply(body)) stored;

-- per conversation: each substantive response, in minutes — working-hours ones counted in working minutes, the
-- others (the customer wrote outside working hours) as they were
alter table wa_insights add column reply_work_min int[] not null default '{}', add column reply_off_min int[] not null default '{}';

-- working hours (policy chat.rules) default to Monday–Saturday 08.00–17.00 WIB in code (domain.DefaultPolicies)

-- +goose Down
alter table wa_insights drop column reply_off_min, drop column reply_work_min;
alter table chat_messages drop column auto_reply, drop column closing;
drop function is_auto_reply(text);
drop function is_closing_message(text);
drop table contact_types;
