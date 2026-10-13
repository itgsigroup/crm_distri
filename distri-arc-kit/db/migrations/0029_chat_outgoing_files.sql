-- +goose Up
-- Files sent from Chat (photo, video, document, voice note): kept only until the outbox delivers them, then deleted —
-- the sent message keeps WhatsApp's download reference like any other media (ADR 0031). Leftovers of replies that
-- failed or were cancelled are purged after 7 days by the retention job.
create table chat_outgoing_files (
  id uuid primary key default gen_random_uuid(),
  message_id uuid not null,              -- chat_messages is partitioned: no foreign key
  mime text not null,
  file_name text not null default '',
  size int not null,
  data bytea not null,
  created_at timestamptz not null default now()
);
create index chat_outgoing_files_message on chat_outgoing_files (message_id);

-- +goose Down
drop table if exists chat_outgoing_files;
