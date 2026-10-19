-- +goose Up
-- Analisis ulang by GSI Orbit itself (Sam 19 Oct, ADR 0046): via 'system' runs the agents' rules and the glossary
-- formulas on the server — no Claude, no API model, no waiting.
alter table cycles drop constraint if exists cycles_via_check;
alter table cycles add constraint cycles_via_check check (via in ('api', 'mcp', 'system'));

-- +goose Down
update cycles set via = 'api' where via = 'system';
alter table cycles drop constraint if exists cycles_via_check;
alter table cycles add constraint cycles_via_check check (via in ('api', 'mcp'));
