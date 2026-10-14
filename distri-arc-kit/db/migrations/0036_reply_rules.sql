-- +goose Up
-- Rekomendasi balasan (ADR 0038, Sam 14 Oct): a customer message with one of a rule's words gets a suggested answer
-- under its bubble from the data — for "stok", the stock of the product named in the message. The sales presses
-- "Pakai jawaban", edits and sends it; nothing is sent automatically.
create table reply_rules (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  action text not null check (action in ('stock')),
  keywords text[] not null,
  -- {produk} {jumlah} {cabang} {cabang_lain} — the draft when the product is in stock / out of stock
  template_ready text not null,
  template_empty text not null,
  show_qty bool not null default true,   -- the draft tells the customer the quantity, else just "ready"
  enabled bool not null default true,
  updated_by text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

insert into reply_rules (name, action, keywords, template_ready, template_empty) values (
  'Tanya stok', 'stock',
  '{stok,stock,ready,redy,ada barang,barang ada,tersedia,masih ada,ada gak,ada ga,ada tidak,kosong,inden}',
  'Ready {jumlah} pcs untuk {produk} di {cabang}.',
  'Mohon maaf, {produk} di {cabang} sedang kosong.');

-- +goose Down
drop table reply_rules;
