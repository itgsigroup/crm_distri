-- +goose Up
-- Analysis comes only from Claude through MCP (ADR 0032, Sam 13 Oct): llm.routing mode "mcp" and require_ai on. A
-- person can still change it in Pengaturan → Jalur analisis; the history keeps the previous value.
update policies set value = value || '{"mode": "mcp", "require_ai": true}'::jsonb, version = version + 1, updated_at = now()
where key = 'llm.routing' and (value->>'mode' is distinct from 'mcp' or coalesce((value->>'require_ai')::bool, false) = false);
insert into policy_history (key, version, value, updated_by, updated_at)
select key, version, value, null, updated_at from policies where key = 'llm.routing'
on conflict (key, version) do nothing;

-- +goose Down
select 1;
