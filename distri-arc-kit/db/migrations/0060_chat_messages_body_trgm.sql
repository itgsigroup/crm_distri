-- +goose Up
-- Cari chat across every conversation (Sam 22 Oct, like WhatsApp's "Pesan" results): a trigram index keeps the
-- case-insensitive regular expression over every stored message body fast as the chat history grows.
create index if not exists chat_messages_body_trgm on chat_messages using gin (body gin_trgm_ops);

-- +goose Down
drop index if exists chat_messages_body_trgm;
