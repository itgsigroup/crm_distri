-- +goose Up
-- Analisa WhatsApp every 2 hours runs in a Claude routine on claude.ai (ADR 0037 rev., Sam 14 Oct): Claude is driven by
-- the CEO's Claude subscription and reaches GSI Orbit through the CRM DISTRI MCP connector — no Claude API key on the
-- server. runner 'external': the server never starts the schedule; every Analisa WhatsApp Claude submits for a
-- screen:wa (or full) cycle is recorded as a run of it, so Pusat kendali and MCP Claude show it.
alter table mcp_schedules add column runner text not null default 'server' check (runner in ('server', 'external'));
update mcp_schedules set runner = 'external', next_run_at = null where key = 'wa_analysis';

-- +goose Down
alter table mcp_schedules drop column runner;
