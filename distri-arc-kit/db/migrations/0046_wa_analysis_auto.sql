-- +goose Up
-- Analisa WhatsApp otomatis through the API (ADR 0043, Sam 17 Oct): Pengaturan → API & model AI picks the hours and the
-- model (Claude API or ChatGPT API); the server runs the same task as the claude.ai routine at those hours. Off until
-- the CEO turns it on. A failed run is not retried with the other API.
insert into mcp_schedules (key, name, prompt, cron, enabled, runner, scopes, max_steps)
select 'wa_analysis_auto', 'Analisa WhatsApp otomatis · API', prompt, '0 8,10,12,14,16 * * 1-6', false, 'server', scopes, max_steps
from mcp_schedules where key = 'wa_analysis'
on conflict (key) do nothing;

-- +goose Down
delete from mcp_schedules where key = 'wa_analysis_auto';
