-- +goose Up
-- Balas otomatis (ADR 0066/0077): a product question answered by Lyra, the company's Hermes Agent, is its own kind.
alter table auto_replies drop constraint auto_replies_kind_check;
alter table auto_replies add constraint auto_replies_kind_check check (kind in ('fact', 'template', 'lyra'));

-- +goose Down
delete from auto_replies where kind = 'lyra';
alter table auto_replies drop constraint auto_replies_kind_check;
alter table auto_replies add constraint auto_replies_kind_check check (kind in ('fact', 'template'));
