-- +goose Up
-- WhatsApp kept 90 days (Sam 22 Oct: "ya pendekan 90 hari", once older chats can be loaded again from the phone like
-- WhatsApp Web — POST /chat/threads/{id}/history). retention.chat_days 365 → 90 as a new policy version with history;
-- the nightly purge then removes messages older than 90 days, and the automatic backfill stops at 90 days.
update policies set value = jsonb_set(value, '{chat_days}', '90'), version = version + 1, updated_at = now()
where key = 'retention' and coalesce((value->>'chat_days')::int, 0) > 90;
insert into policy_history (key, version, value, updated_by, updated_at)
select key, version, value, null, updated_at from policies where key = 'retention'
on conflict (key, version) do nothing;

-- +goose Down
update policies set value = jsonb_set(value, '{chat_days}', '365'), version = version + 1, updated_at = now()
where key = 'retention';
insert into policy_history (key, version, value, updated_by, updated_at)
select key, version, value, null, updated_at from policies where key = 'retention'
on conflict (key, version) do nothing;
