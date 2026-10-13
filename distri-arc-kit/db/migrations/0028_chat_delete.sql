-- +goose Up
-- "Hapus untuk semua orang" (WhatsApp delete for everyone): when a message was deleted — by the customer on their phone,
-- on the sales' phone, or from Chat. The text stays for the audit trail; the conversation shows "Pesan ini dihapus".
alter table chat_messages add column deleted_at timestamptz;

-- +goose Down
alter table chat_messages drop column if exists deleted_at;
