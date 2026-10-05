-- +goose Up
-- Stage 06: one active cycle at a time (POST /cycles → 409), grouped plan steps, and conflict details.
create unique index cycles_one_active on cycles ((true)) where status in ('queued', 'running');
create index cycles_started on cycles (started_at desc);
alter table cycles add column stage text;                 -- current stage while running
alter table plan_items
  add column proposal_ids uuid[],                          -- grouped steps ("Kirim rekomendasi order ke 3 dealer")
  add column link text,                                    -- "chat" for steps without a proposal button
  add column wait_for text;                                -- waiting step: what releases it ("payment:INV/0901")
alter table conflicts
  add column tone text,                                    -- warn | accent | bad | indigo (mockup chip colour)
  add column visible bool not null default true;           -- false for housekeeping rules (dedupe, suppression, gap)
alter table proposals add column dealer_ids uuid[];        -- multi-dealer proposals (push_stock)
create index proposals_dealer_ids on proposals using gin (dealer_ids);

-- +goose Down
drop index if exists proposals_dealer_ids;
alter table proposals drop column dealer_ids;
alter table conflicts drop column visible, drop column tone;
alter table plan_items drop column wait_for, drop column link, drop column proposal_ids;
alter table cycles drop column stage;
drop index if exists cycles_started;
drop index if exists cycles_one_active;
