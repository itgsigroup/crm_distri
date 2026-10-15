-- +goose Up
-- Pinned messages and labels in Chat, like WhatsApp Business (15 Oct 2026). conv is the conversation: 'g:<group jid>'
-- for a group (the same for every GSI number in it), the thread id otherwise. chat_messages is partitioned (no FK to
-- it): message rows keep sent_at and are purged with the messages. Pins stay in GSI Orbit; nothing goes to WhatsApp.
create table chat_labels (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  color text not null,
  position int not null default 0,
  created_by text,
  created_at timestamptz not null default now()
);
create unique index chat_labels_name on chat_labels (lower(name));
insert into chat_labels (name, color, position) values
  ('Pelanggan baru', '#34B7F1', 1), ('Pesanan baru', '#F5B400', 2), ('Menunggu pembayaran', '#FF7043', 3),
  ('Sudah dibayar', '#25D366', 4), ('Pesanan selesai', '#8E7CC3', 5);

create table chat_conv_labels (
  conv text not null,
  label_id uuid not null references chat_labels on delete cascade,
  added_by text,
  added_at timestamptz not null default now(),
  primary key (conv, label_id)
);

create table chat_message_labels (
  message_id uuid not null,
  sent_at timestamptz not null,
  conv text not null,
  label_id uuid not null references chat_labels on delete cascade,
  added_by text,
  added_at timestamptz not null default now(),
  primary key (message_id, label_id)
);
create index chat_message_labels_conv on chat_message_labels (conv);

create table chat_pins (
  conv text not null,
  message_id uuid not null,
  sent_at timestamptz not null,
  pinned_by text,
  pinned_at timestamptz not null default now(),
  primary key (conv, message_id)
);

-- +goose Down
drop table chat_pins;
drop table chat_message_labels;
drop table chat_conv_labels;
drop table chat_labels;
