-- +goose Up
-- Balasan cepat (Sam 24 Oct, like WhatsApp Business quick replies): saved texts the team types with "/shortcut" in
-- Chat. The text only fills the box; sending stays the person's Kirim (ADR 0065).
create table quick_replies (
  id uuid primary key default gen_random_uuid(),
  shortcut text not null,
  body text not null,
  created_by uuid references users on delete set null,
  created_by_name text not null default '',
  uses int not null default 0,
  last_used_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create unique index quick_replies_shortcut on quick_replies (lower(shortcut));

-- +goose Down
drop table if exists quick_replies;
