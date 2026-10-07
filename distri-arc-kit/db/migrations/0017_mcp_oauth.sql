-- +goose Up
-- OAuth 2.1 for the MCP server (ADR 0021): Claude (claude.ai, Claude Desktop, Claude Code) registers itself
-- (dynamic client registration), a Distri ARC user approves it on the consent page, and the connection gets a
-- short-lived access token plus a rotating refresh token. A connection is an mcp_clients row (kind 'oauth') so
-- it shows, records calls and is revoked like a manual token.
create table oauth_clients (
  client_id text primary key,
  client_name text not null,
  redirect_uris text[] not null,
  created_at timestamptz not null default now(),
  last_used_at timestamptz
);

-- an authorization request waiting for a person to approve it (10 minutes)
create table oauth_requests (
  id text primary key,
  client_id text not null references oauth_clients on delete cascade,
  redirect_uri text not null,
  state text not null default '',
  code_challenge text not null,
  scopes text[] not null,
  resource text not null default '',
  expires_at timestamptz not null,
  created_at timestamptz not null default now()
);

-- an approved request's one-time code (5 minutes); stored hashed
create table oauth_codes (
  code_hash text primary key,
  client_id text not null references oauth_clients on delete cascade,
  user_id uuid not null references users on delete cascade,
  redirect_uri text not null,
  code_challenge text not null,
  scopes text[] not null,
  expires_at timestamptz not null,
  used_at timestamptz
);

alter table mcp_clients add column user_id uuid references users on delete set null,
  add column oauth_client_id text references oauth_clients on delete set null,
  add column expires_at timestamptz,          -- access token expiry (oauth); null = manual token without expiry
  add column refresh_hash text unique,        -- sha256 of the current refresh token (rotated on every use)
  add column refresh_expires_at timestamptz;

update roles set screens = array_append(screens, 'mcp') where key in ('ceo', 'admin') and not ('mcp' = any(screens));

-- +goose Down
update roles set screens = array_remove(screens, 'mcp');
alter table mcp_clients drop column refresh_expires_at, drop column refresh_hash, drop column expires_at,
  drop column oauth_client_id, drop column user_id;
drop table oauth_codes;
drop table oauth_requests;
drop table oauth_clients;
