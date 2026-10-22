-- +goose Up
-- Pesan berbintang (Sam 22 Oct, like WhatsApp's starred messages): each person's own stars, not the team's (pins are
-- the team's). A star goes with its message (purged with it: the list joins chat_messages) and its person.
create table chat_message_stars (
  user_id uuid not null references users on delete cascade,
  message_id uuid not null,
  thread_id uuid not null references chat_threads on delete cascade,
  created_at timestamptz not null default now(),
  primary key (user_id, message_id)
);
create index chat_message_stars_user_time on chat_message_stars (user_id, created_at desc);

-- +goose Down
drop table if exists chat_message_stars;
