-- +goose Up
-- Stage 07: MCP clients and calls (status per call), and the Input an MCP client analyses (routing = mcp).
alter table mcp_calls add column status text not null default 'ok';  -- ok | forbidden | rate_limited | human_only | error
create index mcp_calls_time on mcp_calls (created_at desc);
alter table mcp_clients add column token_prefix text;               -- shown in the UI ("arc_3f9a…")

create table cycle_inputs (
  cycle_id uuid not null references cycles on delete cascade,
  agent text not null,
  input jsonb not null,                -- masked agents.Input as the MCP client sees it
  signal_ids uuid[] not null,          -- provenance a submission may cite
  mapping jsonb,                       -- placeholder → value, to unmask submissions (never sent to the client)
  submitted jsonb,                     -- proposals from orchestrator.submit
  submitted_by uuid references mcp_clients on delete set null,
  submitted_at timestamptz,
  created_at timestamptz not null default now(),
  primary key (cycle_id, agent)
);

-- +goose Down
drop table if exists cycle_inputs;
alter table mcp_clients drop column token_prefix;
drop index if exists mcp_calls_time;
alter table mcp_calls drop column status;
