-- +goose Up
-- The model and effort an analysis asked for (Sam 16 Oct): chosen on the Orchestrator dock — for Claude through MCP the
-- model to pick in claude.ai, for the API on the server the effort it runs with. Shown with the cycle.
alter table cycles add column engine_model text, add column effort text check (effort in ('low', 'medium', 'high'));

-- +goose Down
alter table cycles drop column engine_model, drop column effort;
