-- +goose Up
-- When the number entered its current state. A bridge restart shows every number "unpaired"/"pairing" for a few
-- seconds before it reconnects; only a linked number stuck in pairing for minutes was removed from the phone.
alter table wa_numbers add column state_since timestamptz not null default now();

-- +goose Down
alter table wa_numbers drop column if exists state_since;
