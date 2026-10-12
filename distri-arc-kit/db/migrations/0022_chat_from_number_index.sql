-- +goose Up
-- PIC aktif (RecountContacts) counts each contact's inbound messages by number. Without an index every recount scanned
-- chat_messages once per contact (2.5k contacts × 14k messages), and one ran per inbound dealer message: a burst of
-- messages (a freshly linked number's history) starved the database and the bridge's forwards timed out (12 Oct).
create index if not exists chat_messages_from_in on chat_messages (from_number, sent_at) where direction = 'in';

-- +goose Down
drop index if exists chat_messages_from_in;
