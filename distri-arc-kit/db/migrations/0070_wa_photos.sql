-- +goose Up
-- Foto profil WhatsApp (ADR 0071): the photo of a chat's contact or group, cached so the chat list does not ask
-- WhatsApp again on every load (the bridge caps photo queries per number per hour). has = false: no photo, or hidden
-- by the contact's privacy settings (asked again after a day).
create table wa_photos (
  jid text not null,
  full_size bool not null,
  account text not null default '',
  has bool not null,
  mime text not null default '',
  data bytea,
  fetched_at timestamptz not null default now(),
  primary key (jid, full_size)
);

-- +goose Down
drop table wa_photos;
