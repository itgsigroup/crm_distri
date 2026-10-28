-- +goose Up
-- Reaksi per pesan (Sam 28 Oct, like WhatsApp): one emoji per person per stored message. reactor is the digits of who
-- reacted (a GSI number for the team's own). Like stars, keyed to the message without a foreign key; each linked
-- number's copy of a message carries its own reactions.
create table chat_message_reactions (
  message_id uuid not null,
  reactor text not null,
  emoji text not null,
  reacted_at timestamptz not null default now(),
  primary key (message_id, reactor)
);

-- +goose Down
drop table if exists chat_message_reactions;
