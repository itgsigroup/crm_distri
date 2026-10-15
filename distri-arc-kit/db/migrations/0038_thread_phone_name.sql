-- +goose Up
-- The name the linked phone saved for the contact ("Pak Yohanes") — often not the dealer's name in Odoo
-- ("JOHANNES YUDHAYANTO"). Kept per conversation so Chat search finds it like WhatsApp does (15 Oct).
alter table chat_threads add column phone_name text;

-- +goose Down
alter table chat_threads drop column phone_name;
