-- +goose Up
-- Sesi login tanpa batas waktu (Sam 28 Oct): a login lasts until the person logs out. Each login is a row here; its id
-- rides in the session token, and logging out revokes the row, so a copied cookie stops working as well.
create table user_sessions (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users on delete cascade,
  created_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  ip text,
  user_agent text,
  revoked_at timestamptz
);
create index user_sessions_user on user_sessions (user_id);

-- +goose Down
drop table if exists user_sessions;
