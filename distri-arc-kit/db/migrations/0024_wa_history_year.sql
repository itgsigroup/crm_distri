-- +goose Up
-- A year of WhatsApp in Chat (ADR 0028): retention.chat_days 90 → 365 (kept when someone already set it longer), and
-- per conversation the progress of the on-demand history backfill (wa.backfill asks the linked phone for the messages
-- before the oldest one kept, 50 at a time, until a year back or until the phone has nothing older).
update policies set value = jsonb_set(value, '{chat_days}', '365'), version = version + 1, updated_at = now()
where key = 'retention' and coalesce((value->>'chat_days')::int, 0) < 365;
insert into policy_history (key, version, value, updated_by, updated_at)
select key, version, value, null, updated_at from policies where key = 'retention'
on conflict (key, version) do nothing;

alter table wa_numbers alter column backfill_days set default 365;
update wa_numbers set backfill_days = 365 where backfill_days < 365;

alter table chat_threads
  add column history_req_oldest timestamptz,  -- the oldest message when older history was last requested
  add column history_req_at timestamptz,      -- when it was requested
  add column history_tries int not null default 0, -- requests in a row that brought nothing older
  add column history_done_at timestamptz;     -- a year reached, or the phone has nothing older

-- +goose Down
alter table chat_threads drop column if exists history_req_oldest, drop column if exists history_req_at,
  drop column if exists history_tries, drop column if exists history_done_at;
alter table wa_numbers alter column backfill_days set default 30;
