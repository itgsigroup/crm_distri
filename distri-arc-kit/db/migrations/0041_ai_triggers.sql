-- +goose Up
-- AI Trigger (ADR 0041, Sam 16 Oct): Rekomendasi balasan gets its own page and more kinds of answer — stock, price,
-- credit (sisa limit and open invoices), the status of the last order — and an AI may write the draft from the same
-- facts. The numbers stay computed in Go; the AI only phrases them. Nothing is sent automatically.
alter table reply_rules drop constraint reply_rules_action_check;
alter table reply_rules add constraint reply_rules_action_check check (action in ('stock', 'price', 'credit', 'order'));
alter table reply_rules add column ai_phrase bool not null default false, add column ai_instruction text not null default '';

-- the AI's draft for one customer message and rule (chat_messages is partitioned: no foreign key; retention purges)
create table reply_ai_drafts (
  message_id uuid not null,
  rule_id uuid not null references reply_rules on delete cascade,
  draft text not null,
  provider text not null,
  model text not null,
  facts_hash text not null,       -- the facts the draft was written from; new facts (stock synced) write it again
  created_at timestamptz not null default now(),
  primary key (message_id, rule_id)
);
create index reply_ai_drafts_time on reply_ai_drafts (created_at);

insert into reply_rules (name, action, keywords, template_ready, template_empty, show_qty) values
  ('Tanya harga', 'price',
   '{harga,harganya,price,pricelist,price list,brp,berapaan,berapa harga,berapa an}',
   'Harga {produk} Rp {harga} per unit.',
   'Mohon maaf, harga {produk} kami cek dulu ya, segera kami kabari.', false),
  ('Tanya limit & tagihan', 'credit',
   '{limit,sisa limit,plafon,tagihan,piutang,jatuh tempo,tempo,invoice,faktur,belum lunas,outstanding}',
   'Sisa limit saat ini Rp {sisa_limit} dari limit Rp {limit}. Tagihan terbuka {jumlah_tagihan} invoice senilai Rp {tagihan}.',
   'Saat ini ada {jumlah_tagihan} tagihan terbuka senilai Rp {tagihan}, tertua jatuh tempo {jatuh_tempo}. Mohon dibantu pelunasannya ya.', false),
  ('Status order', 'order',
   '{status order,status pesanan,pesanan saya,orderan saya,order saya,sudah dikirim,udah dikirim,kapan dikirim,kapan kirim,kapan sampai,resi,no resi,sudah jalan,barang saya}',
   'Order {nomor} tanggal {tanggal} saat ini {status}.',
   'Mohon maaf, pesanan Bapak/Ibu kami cek dulu ya.', false);

-- +goose Down
drop table reply_ai_drafts;
delete from reply_rules where action <> 'stock';
alter table reply_rules drop column ai_instruction, drop column ai_phrase;
alter table reply_rules drop constraint reply_rules_action_check;
alter table reply_rules add constraint reply_rules_action_check check (action in ('stock'));
