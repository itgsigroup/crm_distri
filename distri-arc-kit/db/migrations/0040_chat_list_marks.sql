-- +goose Up
-- Chat list, like WhatsApp (15 Oct 2026): a person marks a chat "belum dibalas" (unreplied_mark: waiting from then
-- until a reply goes out) or "sudah dibalas" (replied_mark: the customer's messages up to then need no reply), and
-- pins chats to the top of their own list (three, as WhatsApp).
alter table chat_threads add column unreplied_mark timestamptz, add column replied_mark timestamptz;

create table chat_thread_pins (
  user_id uuid not null references users on delete cascade,
  conv text not null,
  pinned_at timestamptz not null default now(),
  primary key (user_id, conv)
);

-- +goose Down
drop table chat_thread_pins;
alter table chat_threads drop column replied_mark, drop column unreplied_mark;
