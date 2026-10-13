-- +goose Up
-- Chat media and edits (ADR 0029). chat_messages.media (jsonb, since 0001) now holds the media's kind, mime, size,
-- duration and a download reference (the WhatsApp key and CDN fields, never the content); the file is fetched from
-- WhatsApp only when a person opens it. edited_at marks a message edited on WhatsApp (by the customer, on the phone,
-- or from Chat within WhatsApp's 15 minutes).
alter table chat_messages add column edited_at timestamptz;

-- +goose Down
alter table chat_messages drop column if exists edited_at;
