-- +goose Up
-- Tags in group chats (WhatsApp "@Budi", Sam 16 Oct): a message keeps the members it tags, and a GSI number tagged
-- in a group sees it on its chat row ("@") until it opens the chat, as WhatsApp shows it.
alter table chat_messages add column mentions jsonb; -- [{token, number, name}]: "@<token>" in the body is that member
alter table chat_threads add column mentioned int not null default 0; -- unread messages that tag this thread's number

-- +goose Down
alter table chat_threads drop column mentioned;
alter table chat_messages drop column mentions;
