-- +goose Up
-- Chat dengan Hermes dari halaman Hermes Agent (ADR 0067): each user's own conversation. The question row and its
-- reply share created_at (the reply is ordered after it by role); the reply stays 'pending' while Hermes works.
create table hermes_messages (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users on delete cascade,
  role text not null check (role in ('user', 'assistant')),
  status text not null default 'done' check (status in ('pending', 'done', 'error')),
  content text not null default '',
  demo bool not null default false,
  created_at timestamptz not null default now(),
  finished_at timestamptz
);
create index hermes_messages_user on hermes_messages (user_id, created_at desc);

-- +goose Down
drop table hermes_messages;
