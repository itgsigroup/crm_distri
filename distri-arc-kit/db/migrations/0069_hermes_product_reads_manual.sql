-- +goose Up
-- Lyra di bawah bubble (ADR 0070, 28 Oct): a reading asked from the bubble (Tanya Lyra / Baca ulang) stays shown;
-- one started by the trigger is shown only while its message has a stock, price or product word.
alter table hermes_product_reads add column manual bool not null default false;

-- +goose Down
alter table hermes_product_reads drop column manual;
