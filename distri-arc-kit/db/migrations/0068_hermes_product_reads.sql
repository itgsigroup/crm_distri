-- +goose Up
-- Lyra di bawah bubble (ADR 0070): the company's Hermes Agent reads a customer message (text or photo) that asks about
-- products and names them (SKU + quantity asked). One row per message, so re-opening a chat never asks again; the
-- stock and DPP price under the bubble are computed by GSI Orbit when the chat is shown, never by the agent.
create table hermes_product_reads (
  message_id uuid primary key,
  thread_id uuid not null,
  status text not null default 'pending' check (status in ('pending', 'done', 'none', 'error')),
  items jsonb not null default '[]', -- [{"sku": "...", "qty": 2}]
  note text not null default '',
  demo bool not null default false,
  created_at timestamptz not null default now(),
  finished_at timestamptz
);
create index hermes_product_reads_thread on hermes_product_reads (thread_id);

-- +goose Down
drop table hermes_product_reads;
