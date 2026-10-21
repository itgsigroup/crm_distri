-- +goose Up
-- AI Trigger (Sam 21 Oct): Claude finds the products a customer message means among candidates from the stock
-- catalogue — every product of the message and the quantity asked. One reading per message (provenance: model,
-- time; the SKUs are checked against the catalogue before they are stored). Kept as long as the message.
create table reply_ai_matches (
  message_id uuid primary key,
  items jsonb not null default '[]'::jsonb, -- [{"sku": "X5S_8W", "qty": 1}]
  note text not null default '',
  provider text not null,
  model text not null,
  created_at timestamptz not null default now()
);
create index reply_ai_matches_time on reply_ai_matches (created_at);

-- +goose Down
drop table if exists reply_ai_matches;
