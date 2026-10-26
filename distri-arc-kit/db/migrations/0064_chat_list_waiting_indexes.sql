-- The chat list calculates waiting_since for every visible thread. These partial indexes keep the
-- per-thread searches focused on messages that can start or stop the waiting clock.
-- +goose Up
create index chat_messages_waiting_in on chat_messages (thread_id, sent_at)
where direction = 'in' and not internal and not closing and not auto_reply
  and coalesce(media->>'kind', '') <> 'system';

create index chat_messages_waiting_out on chat_messages (thread_id, sent_at desc)
where (direction = 'out' or internal) and not holding and not auto_reply and status <> 'failed';

-- +goose Down
drop index if exists chat_messages_waiting_out;
drop index if exists chat_messages_waiting_in;
