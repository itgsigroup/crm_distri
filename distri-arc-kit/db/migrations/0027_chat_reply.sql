-- +goose Up
-- Replies (WhatsApp "Balas"): the WhatsApp id of the message a bubble quotes — written by the customer, sent from the
-- phone, or from Chat. Resolved to the quoted bubble when the conversation is shown.
alter table chat_messages add column quoted_wa_id text;

-- +goose Down
alter table chat_messages drop column if exists quoted_wa_id;
