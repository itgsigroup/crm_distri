-- ARC domain schema (Stage 01). Every table has id/created_at/updated_at;
-- records that can come from an external system carry source_system/source_id/source_write_date.

CREATE TABLE users (
  id            text PRIMARY KEY,
  name          text NOT NULL,
  email         text NOT NULL UNIQUE,
  role          text NOT NULL CHECK (role IN ('ceo','manager','sales','finance','ops')),
  branch        text NOT NULL,
  initials      text NOT NULL DEFAULT '',
  odoo_user_id  bigint,
  wa_numbers    text[] NOT NULL DEFAULT '{}',
  password_hash text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_sessions (
  token      text PRIMARY KEY,
  user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE stage_definitions (
  id            serial PRIMARY KEY,
  name          text NOT NULL UNIQUE,
  seq           int  NOT NULL,
  is_won        boolean NOT NULL DEFAULT false,
  is_lost       boolean NOT NULL DEFAULT false,
  source_system text,
  source_id     text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
  id                 text PRIMARY KEY,
  name               text NOT NULL,
  sector             text NOT NULL DEFAULT '',
  branch             text NOT NULL DEFAULT '',
  owner_user_id      text REFERENCES users(id),
  odoo_partner_id    bigint,
  odoo_company_id    bigint,
  health             int,
  health_trend_30d   int NOT NULL DEFAULT 0,
  normal_rhythm_days int NOT NULL DEFAULT 7,
  is_government      boolean NOT NULL DEFAULT false,
  memory             text NOT NULL DEFAULT '',
  memory_version     int  NOT NULL DEFAULT 0,
  memory_provenance  jsonb NOT NULL DEFAULT '[]',
  memory_updated_at  timestamptz,
  tags               text[] NOT NULL DEFAULT '{}',
  last_interaction_at timestamptz,
  last_via           text NOT NULL DEFAULT '',
  source_system      text,
  source_id          text,
  source_write_date  timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_system, source_id)
);
CREATE INDEX accounts_branch_idx ON accounts(branch);

CREATE TABLE account_memory_history (
  id             bigserial PRIMARY KEY,
  account_id     text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  version        int  NOT NULL,
  memory         text NOT NULL,
  evidence       jsonb NOT NULL DEFAULT '[]',
  model          text NOT NULL DEFAULT '',
  prompt_version text NOT NULL DEFAULT '',
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, version)
);

CREATE TABLE people (
  id               text PRIMARY KEY,
  name             text NOT NULL,
  role             text NOT NULL DEFAULT '',
  account_id       text REFERENCES accounts(id) ON DELETE SET NULL,
  phones           text[] NOT NULL DEFAULT '{}',
  emails           text[] NOT NULL DEFAULT '{}',
  wa_ids           text[] NOT NULL DEFAULT '{}',
  stakeholder_tag  text NOT NULL DEFAULT 'user' CHECK (stakeholder_tag IN ('decision','champion','influencer','user','procurement','ghost','former')),
  strength         int  NOT NULL DEFAULT 0 CHECK (strength BETWEEN 0 AND 3),
  stakeholder_note text NOT NULL DEFAULT '',
  is_decision      boolean NOT NULL DEFAULT false,
  is_internal      boolean NOT NULL DEFAULT false,
  internal_unit    text NOT NULL DEFAULT '',
  is_private       boolean NOT NULL DEFAULT false,
  last_contact_at  timestamptz,
  source_system    text,
  source_id        text,
  source_write_date timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_system, source_id)
);
CREATE INDEX people_account_idx ON people(account_id);
CREATE INDEX people_phones_idx ON people USING gin(phones);
CREATE INDEX people_emails_idx ON people USING gin(emails);

CREATE TABLE opportunities (
  id                 text PRIMARY KEY,
  account_id         text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name               text NOT NULL,
  expected_revenue   numeric(18,2) NOT NULL DEFAULT 0,
  probability        int  NOT NULL DEFAULT 0,
  stage_id           int  NOT NULL REFERENCES stage_definitions(id),
  date_deadline      date,
  closing_label      text NOT NULL DEFAULT '',
  tags               text[] NOT NULL DEFAULT '{}',
  priority           int  NOT NULL DEFAULT 0,
  activity_state     text NOT NULL DEFAULT '',
  owner_user_id      text REFERENCES users(id),
  source             text NOT NULL DEFAULT '' CHECK (source IN ('','tender','inbound','referral','ekspansi','cold')),
  note               text NOT NULL DEFAULT '',
  status             text NOT NULL DEFAULT 'open' CHECK (status IN ('open','won','lost')),
  lead_at            timestamptz,
  quotation_at       timestamptz,
  won_at             timestamptz,
  lost_at            timestamptz,
  first_response_minutes int,
  historical         boolean NOT NULL DEFAULT false,
  active_stakeholders int NOT NULL DEFAULT 0,
  silent_after_revision boolean NOT NULL DEFAULT false,
  referrer_account_id text,
  product_line       text NOT NULL DEFAULT '',
  bast_at            timestamptz,
  invoice_at         timestamptz,
  paid_at            timestamptz,
  health             int,
  health_breakdown   jsonb NOT NULL DEFAULT '{}',
  health_trend_30d   int NOT NULL DEFAULT 0,
  signal             text NOT NULL DEFAULT '',
  stage_evidence     text NOT NULL DEFAULT '',
  arc_probability    int,
  next_action_id     text,
  locked_to_source   boolean NOT NULL DEFAULT false,
  source_system      text,
  source_id          text,
  source_write_date  timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_system, source_id)
);
CREATE INDEX opportunities_account_idx ON opportunities(account_id);
CREATE INDEX opportunities_status_idx ON opportunities(status);
CREATE INDEX opportunities_hist_idx ON opportunities(historical);

-- Latest value per health component with provenance (computed, extracted or fixture).
CREATE TABLE health_components (
  id             bigserial PRIMARY KEY,
  opportunity_id text NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
  component      text NOT NULL CHECK (component IN ('engagement','multithreading','momentum','fit','sentiment')),
  value          int  NOT NULL CHECK (value BETWEEN 0 AND 100),
  origin         text NOT NULL CHECK (origin IN ('computed','extracted','fixture','manual')),
  evidence       jsonb NOT NULL DEFAULT '[]',
  confidence     numeric(4,3) NOT NULL DEFAULT 0.8,
  model          text NOT NULL DEFAULT '',
  prompt_version text NOT NULL DEFAULT '',
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (opportunity_id, component)
);

CREATE TABLE health_snapshots (
  id             bigserial PRIMARY KEY,
  opportunity_id text NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
  health         int NOT NULL,
  breakdown      jsonb NOT NULL DEFAULT '{}',
  taken_on       date NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (opportunity_id, taken_on)
);

CREATE TABLE wa_sessions (
  id            text PRIMARY KEY,
  label         text NOT NULL,
  user_id       text REFERENCES users(id),
  phone         text NOT NULL DEFAULT '',
  transport     text NOT NULL DEFAULT 'bridge' CHECK (transport IN ('bridge','cloud')),
  status        text NOT NULL DEFAULT 'unlinked' CHECK (status IN ('unlinked','pairing','connected','disconnected')),
  history_days  int  NOT NULL DEFAULT 30,
  messages_30d  int  NOT NULL DEFAULT 0,
  last_event_at timestamptz,
  qr_code       text NOT NULL DEFAULT '',
  qr_expires_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chat_groups (
  id            text PRIMARY KEY,
  session_id    text REFERENCES wa_sessions(id) ON DELETE SET NULL,
  jid           text NOT NULL UNIQUE,
  name          text NOT NULL,
  type          text NOT NULL CHECK (type IN ('external','internal')),
  read_policy   boolean NOT NULL DEFAULT false,
  members       jsonb NOT NULL DEFAULT '[]',
  account_id    text REFERENCES accounts(id) ON DELETE SET NULL,
  project       jsonb NOT NULL DEFAULT '{}',
  summary       jsonb NOT NULL DEFAULT '[]',
  skipped_count int  NOT NULL DEFAULT 0,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chat_threads (
  id          text PRIMARY KEY,
  session_id  text NOT NULL REFERENCES wa_sessions(id) ON DELETE CASCADE,
  chat_jid    text NOT NULL,
  type        text NOT NULL CHECK (type IN ('cust','gext','gint','internal','unknown')),
  name        text NOT NULL,
  subtitle    text NOT NULL DEFAULT '',
  account_id  text REFERENCES accounts(id) ON DELETE SET NULL,
  person_id   text REFERENCES people(id) ON DELETE SET NULL,
  group_id    text REFERENCES chat_groups(id) ON DELETE SET NULL,
  unread      int  NOT NULL DEFAULT 0,
  last_at     timestamptz,
  is_private  boolean NOT NULL DEFAULT false,
  skipped_count int NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (session_id, chat_jid)
);

CREATE TABLE interactions (
  id                 bigserial PRIMARY KEY,
  channel            text NOT NULL CHECK (channel IN ('email','wa_message','wa_group_message','meeting','call','document','form','erp_event','note','wa_aggregate')),
  direction          text NOT NULL DEFAULT 'in' CHECK (direction IN ('in','out')),
  occurred_at        timestamptz NOT NULL,
  person_ids         text[] NOT NULL DEFAULT '{}',
  user_ids           text[] NOT NULL DEFAULT '{}',
  participants_label text NOT NULL DEFAULT '',
  thread_id          text REFERENCES chat_threads(id) ON DELETE SET NULL,
  group_id           text REFERENCES chat_groups(id) ON DELETE SET NULL,
  subject            text NOT NULL DEFAULT '',
  body_text          text NOT NULL DEFAULT '',
  attachments        jsonb NOT NULL DEFAULT '[]',
  raw_ref            text NOT NULL UNIQUE,
  wamid              text UNIQUE,
  account_id         text REFERENCES accounts(id) ON DELETE SET NULL,
  opportunity_id     text REFERENCES opportunities(id) ON DELETE SET NULL,
  sender_name        text NOT NULL DEFAULT '',
  sender_phone       text NOT NULL DEFAULT '',
  sender_internal    boolean NOT NULL DEFAULT false,
  transport          text NOT NULL DEFAULT '',
  is_history         boolean NOT NULL DEFAULT false,
  message_count      int  NOT NULL DEFAULT 1,
  sentiment          numeric(4,3),
  summary            text NOT NULL DEFAULT '',
  inference          text NOT NULL DEFAULT '',
  hot                boolean NOT NULL DEFAULT false,
  extracted          boolean NOT NULL DEFAULT false,
  extraction_version int  NOT NULL DEFAULT 0,
  delivery_status    text NOT NULL DEFAULT '',
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX interactions_account_idx ON interactions(account_id);
CREATE INDEX interactions_occurred_idx ON interactions(occurred_at);
CREATE INDEX interactions_thread_idx ON interactions(thread_id, occurred_at);
CREATE INDEX interactions_unextracted_idx ON interactions(extracted) WHERE extracted = false;

-- Per-message ARC annotations produced by the Capture agent (shown under chat messages).
CREATE TABLE extractions (
  id             bigserial PRIMARY KEY,
  interaction_id bigint NOT NULL REFERENCES interactions(id) ON DELETE CASCADE,
  kind           text NOT NULL CHECK (kind IN ('commitment','signal','task','milestone','sentiment','note','risk','stage')),
  tone           text NOT NULL DEFAULT 'accent',
  text           text NOT NULL,
  action_label   text NOT NULL DEFAULT '',
  payload        jsonb NOT NULL DEFAULT '{}',
  evidence       jsonb NOT NULL DEFAULT '[]',
  confidence     numeric(4,3) NOT NULL DEFAULT 0.8,
  model          text NOT NULL DEFAULT '',
  prompt_version text NOT NULL DEFAULT '',
  version        int  NOT NULL DEFAULT 1,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (interaction_id, kind, text)
);

CREATE TABLE commitments (
  id                         text PRIMARY KEY,
  dedupe_hash                text NOT NULL UNIQUE,
  account_id                 text REFERENCES accounts(id) ON DELETE CASCADE,
  opportunity_id             text REFERENCES opportunities(id) ON DELETE SET NULL,
  who                        text NOT NULL CHECK (who IN ('kami','mereka')),
  text                       text NOT NULL,
  detail                     text NOT NULL DEFAULT '',
  due_at                     timestamptz,
  status                     text NOT NULL DEFAULT 'open' CHECK (status IN ('open','done','late','cancelled')),
  origin_interaction_id      bigint REFERENCES interactions(id) ON DELETE SET NULL,
  resolved_by_interaction_id bigint REFERENCES interactions(id) ON DELETE SET NULL,
  owner_user_id              text REFERENCES users(id),
  escalation_level           int NOT NULL DEFAULT 0,
  evidence                   jsonb NOT NULL DEFAULT '[]',
  confidence                 numeric(4,3) NOT NULL DEFAULT 0.8,
  model                      text NOT NULL DEFAULT '',
  draft_ready                boolean NOT NULL DEFAULT false,
  odoo_activity_id           bigint,
  created_at                 timestamptz NOT NULL DEFAULT now(),
  updated_at                 timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX commitments_account_idx ON commitments(account_id);
CREATE INDEX commitments_status_idx ON commitments(status, due_at);

CREATE TABLE signals (
  id               bigserial PRIMARY KEY,
  type             text NOT NULL,
  severity         text NOT NULL CHECK (severity IN ('bad','warn','good','accent','neutral')),
  account_id       text REFERENCES accounts(id) ON DELETE CASCADE,
  opportunity_id   text REFERENCES opportunities(id) ON DELETE CASCADE,
  title            text NOT NULL,
  detail           text NOT NULL DEFAULT '',
  suggested_action text NOT NULL DEFAULT '',
  headline         text NOT NULL DEFAULT '',
  evidence         jsonb NOT NULL DEFAULT '[]',
  confidence       numeric(4,3) NOT NULL DEFAULT 0.8,
  dedupe_key       text NOT NULL UNIQUE,
  detected_at      timestamptz NOT NULL DEFAULT now(),
  resolved_at      timestamptz,
  acknowledged_by  text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX signals_open_idx ON signals(detected_at) WHERE resolved_at IS NULL;

CREATE TABLE actions (
  id             text PRIMARY KEY,
  agent          text NOT NULL,
  type           text NOT NULL,
  kind           text NOT NULL DEFAULT 'send' CHECK (kind IN ('send','policy','re','task','internal')),
  title          text NOT NULL,
  button_label   text NOT NULL DEFAULT 'Setujui',
  icon           text NOT NULL DEFAULT 'i-send',
  account_id     text REFERENCES accounts(id) ON DELETE CASCADE,
  opportunity_id text REFERENCES opportunities(id) ON DELETE SET NULL,
  cash_item_id   text,
  subject_label  text NOT NULL DEFAULT '',
  due_label      text NOT NULL DEFAULT '',
  due_at         timestamptz,
  summary        text NOT NULL DEFAULT '',
  why            text NOT NULL DEFAULT '',
  prep           text NOT NULL DEFAULT '',
  preview        text NOT NULL DEFAULT '',
  preview_from   text NOT NULL DEFAULT '',
  context_note   text NOT NULL DEFAULT '',
  impact         jsonb NOT NULL DEFAULT '[]',
  steps          jsonb NOT NULL DEFAULT '[]',
  options        jsonb NOT NULL DEFAULT '[]',
  tags           jsonb NOT NULL DEFAULT '[]',
  payload        jsonb NOT NULL DEFAULT '{}',
  evidence       jsonb NOT NULL DEFAULT '[]',
  confidence     numeric(4,3) NOT NULL DEFAULT 0.8,
  model          text NOT NULL DEFAULT '',
  in_queue       boolean NOT NULL DEFAULT false,
  status         text NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed','approved','edited','rejected','snoozed','executed','cancelled')),
  decision       jsonb,
  result_text    text NOT NULL DEFAULT '',
  executed_at    timestamptz,
  execution_log  jsonb NOT NULL DEFAULT '[]',
  proposed_by    text NOT NULL DEFAULT 'agent',
  snoozed_until  timestamptz,
  suppress_key   text NOT NULL DEFAULT '',
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX actions_status_idx ON actions(status);
CREATE INDEX actions_account_idx ON actions(account_id);

CREATE TABLE action_decisions (
  id         bigserial PRIMARY KEY,
  action_id  text NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  agent      text NOT NULL,
  type       text NOT NULL,
  account_id text,
  user_id    text NOT NULL,
  decision   text NOT NULL CHECK (decision IN ('approve','edit','reject','snooze','option')),
  option     text NOT NULL DEFAULT '',
  reason     text NOT NULL DEFAULT '',
  note       text NOT NULL DEFAULT '',
  decided_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX action_decisions_agent_idx ON action_decisions(agent, decided_at);

CREATE TABLE suppressions (
  id         bigserial PRIMARY KEY,
  account_id text NOT NULL,
  agent      text NOT NULL,
  type       text NOT NULL,
  until      timestamptz NOT NULL,
  reason     text NOT NULL DEFAULT '',
  signal_fingerprint text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, agent, type)
);

CREATE TABLE learned_rules (
  id         bigserial PRIMARY KEY,
  agent      text NOT NULL,
  pattern    text NOT NULL UNIQUE,
  text       text NOT NULL,
  active     boolean NOT NULL DEFAULT true,
  notified   boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE policies (
  key         text PRIMARY KEY,
  value       jsonb NOT NULL,
  description text NOT NULL DEFAULT '',
  version     int  NOT NULL DEFAULT 1,
  updated_by  text NOT NULL DEFAULT 'seed',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE policy_history (
  id         bigserial PRIMARY KEY,
  key        text NOT NULL,
  value      jsonb NOT NULL,
  version    int  NOT NULL,
  updated_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Append-only audit trail.
CREATE TABLE audit_log (
  id           bigserial PRIMARY KEY,
  actor        text NOT NULL,
  actor_type   text NOT NULL CHECK (actor_type IN ('user','agent','machine','system')),
  verb         text NOT NULL,
  object_type  text NOT NULL,
  object_id    text NOT NULL,
  payload_hash text NOT NULL,
  payload      jsonb NOT NULL DEFAULT '{}',
  at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_object_idx ON audit_log(object_type, object_id);

CREATE FUNCTION audit_log_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER audit_log_no_update BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

CREATE TABLE internal_numbers (
  id           bigserial PRIMARY KEY,
  name         text NOT NULL,
  phone        text NOT NULL,
  phone_norm   text NOT NULL UNIQUE,
  unit         text NOT NULL DEFAULT 'Lainnya',
  branch       text NOT NULL DEFAULT 'Pusat',
  source       text NOT NULL CHECK (source IN ('talenta','manual','arc_suggested')),
  confirmed_by text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE internal_suspects (
  id          bigserial PRIMARY KEY,
  phone       text NOT NULL,
  phone_norm  text NOT NULL UNIQUE,
  name_hint   text NOT NULL DEFAULT '',
  reason      text NOT NULL DEFAULT '',
  group_count int  NOT NULL DEFAULT 0,
  status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','rejected')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
  id                    text PRIMARY KEY,
  title                 text NOT NULL,
  assignee              text NOT NULL DEFAULT '',
  source_interaction_id bigint REFERENCES interactions(id) ON DELETE SET NULL,
  source_label          text NOT NULL DEFAULT '',
  group_id              text REFERENCES chat_groups(id) ON DELETE SET NULL,
  account_id            text REFERENCES accounts(id) ON DELETE SET NULL,
  status                text NOT NULL DEFAULT 'detected' CHECK (status IN ('detected','proposed','open','done')),
  basecamp_id           text NOT NULL DEFAULT '',
  dedupe_hash           text NOT NULL UNIQUE,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE inbound_contacts (
  id             text PRIMARY KEY,
  phone          text NOT NULL,
  phone_norm     text NOT NULL UNIQUE,
  first_message  text NOT NULL DEFAULT '',
  via_user_id    text REFERENCES users(id),
  thread_id      text REFERENCES chat_threads(id) ON DELETE SET NULL,
  received_at    timestamptz NOT NULL DEFAULT now(),
  identification jsonb NOT NULL DEFAULT '{}',
  overview       text NOT NULL DEFAULT '',
  fit_score      int,
  status         text NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown','identified','qualified','lead','not_prospect')),
  solutions      jsonb NOT NULL DEFAULT '[]',
  pain_questions jsonb NOT NULL DEFAULT '[]',
  opportunity_id text REFERENCES opportunities(id) ON DELETE SET NULL,
  odoo_lead_id   bigint,
  expires_at     timestamptz NOT NULL DEFAULT (now() + interval '90 days'),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE funnel_events (
  id          bigserial PRIMARY KEY,
  inbound_id  text REFERENCES inbound_contacts(id) ON DELETE CASCADE,
  subject_key text NOT NULL,
  stage       text NOT NULL CHECK (stage IN ('masuk','teridentifikasi','relevan','pain_point','lead','penawaran','won')),
  channel     text NOT NULL DEFAULT 'wa',
  at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (subject_key, stage)
);

CREATE TABLE installed_systems (
  id               bigserial PRIMARY KEY,
  account_id       text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  system           text NOT NULL,
  installed_year   text NOT NULL DEFAULT '',
  warranty_end     date,
  warranty_label   text NOT NULL DEFAULT '',
  service_contract text NOT NULL DEFAULT '',
  units            int NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, system)
);

CREATE TABLE whitespace (
  id           bigserial PRIMARY KEY,
  account_id   text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  product_line text NOT NULL,
  status       text NOT NULL CHECK (status IN ('ada','proses','peluang','-')),
  value        numeric(18,2) NOT NULL DEFAULT 0,
  why          text NOT NULL DEFAULT '',
  seq          int  NOT NULL DEFAULT 0,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, product_line)
);

-- Lead-to-cash item per sales order / project (Won -> Lunas).
CREATE TABLE cash_items (
  id               text PRIMARY KEY,
  account_id       text REFERENCES accounts(id) ON DELETE SET NULL,
  account_name     text NOT NULL,
  project          text NOT NULL,
  so_id            text NOT NULL UNIQUE,
  project_id       text NOT NULL DEFAULT '',
  value            numeric(18,2) NOT NULL,
  stage            text NOT NULL CHECK (stage IN ('persiapan','pemasangan','bast','invoice','menunggu_bayar','lunas')),
  stage_entered_at timestamptz NOT NULL,
  benchmark_days   int  NOT NULL,
  owner_user_id    text REFERENCES users(id),
  note             text NOT NULL DEFAULT '',
  blocked_reason   text NOT NULL DEFAULT '',
  invoice_id       text,
  won_at           timestamptz,
  bast_at          timestamptz,
  invoiced_at      timestamptz,
  paid_at          timestamptz,
  source_system    text,
  source_id        text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE invoices (
  id             text PRIMARY KEY,
  number         text NOT NULL UNIQUE,
  so_id          text NOT NULL DEFAULT '',
  account_id     text REFERENCES accounts(id) ON DELETE SET NULL,
  account_name   text NOT NULL,
  category       text NOT NULL DEFAULT 'project' CHECK (category IN ('project','distribusi')),
  label          text NOT NULL DEFAULT '',
  amount         numeric(18,2) NOT NULL,
  residual       numeric(18,2) NOT NULL,
  invoice_date   date NOT NULL,
  due_date       date NOT NULL,
  paid_at        date,
  pay_pattern    text NOT NULL DEFAULT '',
  spm_submitted_at date,
  is_government  boolean NOT NULL DEFAULT false,
  source_system  text,
  source_id      text,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

-- Expected receipts not yet invoiced (termin after BAST, DP after PO).
CREATE TABLE expected_receipts (
  id          text PRIMARY KEY,
  account_id  text REFERENCES accounts(id) ON DELETE SET NULL,
  label       text NOT NULL,
  detail      text NOT NULL DEFAULT '',
  amount      numeric(18,2) NOT NULL,
  trigger     text NOT NULL CHECK (trigger IN ('after_bast','after_po','invoice_pending')),
  expected_on date,
  cash_item_id text REFERENCES cash_items(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payment_history (
  id          bigserial PRIMARY KEY,
  account_id  text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  invoice_ref text NOT NULL,
  amount      numeric(18,2) NOT NULL,
  term_days   int  NOT NULL DEFAULT 30,
  days_to_pay int  NOT NULL,
  paid_on     date NOT NULL,
  UNIQUE (account_id, invoice_ref)
);

CREATE TABLE credit_profiles (
  account_id      text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  credit_limit    numeric(18,2) NOT NULL,
  open_receivable numeric(18,2) NOT NULL DEFAULT 0,
  avg_days_to_pay int NOT NULL DEFAULT 30,
  overdue_note    text NOT NULL DEFAULT '',
  registered_phone text NOT NULL DEFAULT '',
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenders (
  id          text PRIMARY KEY,
  title       text NOT NULL,
  agency      text NOT NULL DEFAULT '',
  source      text NOT NULL DEFAULT '',
  hps         numeric(18,2) NOT NULL DEFAULT 0,
  deadline    date,
  description text NOT NULL DEFAULT '',
  match_score int,
  reasons     text NOT NULL DEFAULT '',
  status      text NOT NULL DEFAULT 'new' CHECK (status IN ('new','qualified','skipped','lead')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE calendar_events (
  id           text PRIMARY KEY,
  raw_ref      text NOT NULL UNIQUE,
  title        text NOT NULL,
  starts_at    timestamptz NOT NULL,
  duration_min int  NOT NULL DEFAULT 60,
  location     text NOT NULL DEFAULT '',
  attendees    text NOT NULL DEFAULT '',
  account_id   text REFERENCES accounts(id) ON DELETE SET NULL,
  internal     boolean NOT NULL DEFAULT false,
  prep_status  text NOT NULL DEFAULT 'none' CHECK (prep_status IN ('none','partial','ready')),
  prep_pills   jsonb NOT NULL DEFAULT '[]',
  owner_user_id text REFERENCES users(id),
  source       text NOT NULL DEFAULT 'manual',
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE briefs (
  id           bigserial PRIMARY KEY,
  slot         text NOT NULL CHECK (slot IN ('pagi','sore','manual')),
  brief_date   date NOT NULL,
  points       jsonb NOT NULL,
  source_counts jsonb NOT NULL DEFAULT '{}',
  confidence   numeric(4,3) NOT NULL DEFAULT 0.8,
  model        text NOT NULL DEFAULT '',
  html         text NOT NULL DEFAULT '',
  text_body    text NOT NULL DEFAULT '',
  sent         jsonb NOT NULL DEFAULT '[]',
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (brief_date, slot)
);

CREATE TABLE notifications (
  id         bigserial PRIMARY KEY,
  channel    text NOT NULL,
  recipient  text NOT NULL,
  subject    text NOT NULL DEFAULT '',
  body       text NOT NULL DEFAULT '',
  status     text NOT NULL DEFAULT 'sent',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE llm_calls (
  id          bigserial PRIMARY KEY,
  tier        text NOT NULL,
  provider    text NOT NULL,
  model       text NOT NULL,
  tokens_in   int  NOT NULL DEFAULT 0,
  tokens_out  int  NOT NULL DEFAULT 0,
  cost_est    numeric(12,6) NOT NULL DEFAULT 0,
  purpose     text NOT NULL,
  input_hash  text NOT NULL,
  duration_ms int  NOT NULL DEFAULT 0,
  ok          boolean NOT NULL DEFAULT true,
  error       text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX llm_calls_created_idx ON llm_calls(created_at);

CREATE TABLE jobs_runs (
  id          bigserial PRIMARY KEY,
  job         text NOT NULL,
  started_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  ok          boolean,
  detail      jsonb NOT NULL DEFAULT '{}'
);

CREATE TABLE api_keys (
  id             text PRIMARY KEY,
  name           text NOT NULL,
  prefix         text NOT NULL UNIQUE,
  key_hash       text NOT NULL,
  scopes         text[] NOT NULL,
  owner_user_id  text NOT NULL REFERENCES users(id),
  rate_limit_rpm int  NOT NULL DEFAULT 60,
  calls_count    bigint NOT NULL DEFAULT 0,
  last_used_at   timestamptz,
  revoked_at     timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ai_clients (
  id          text PRIMARY KEY,
  name        text NOT NULL,
  logo        text NOT NULL,
  color       text NOT NULL,
  description text NOT NULL,
  users_count int  NOT NULL DEFAULT 0,
  last_seen_at timestamptz,
  status      text NOT NULL DEFAULT 'connected'
);

CREATE TABLE mcp_tool_groups (
  id      text PRIMARY KEY,
  label   text NOT NULL,
  note    text NOT NULL DEFAULT '',
  seq     int  NOT NULL,
  enabled boolean NOT NULL DEFAULT true
);

CREATE TABLE oauth_clients (
  client_id     text PRIMARY KEY,
  client_name   text NOT NULL DEFAULT '',
  redirect_uris text[] NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_codes (
  code           text PRIMARY KEY,
  client_id      text NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
  user_id        text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  redirect_uri   text NOT NULL,
  code_challenge text NOT NULL,
  scope          text NOT NULL DEFAULT '',
  expires_at     timestamptz NOT NULL,
  used           boolean NOT NULL DEFAULT false
);

CREATE TABLE oauth_tokens (
  token_hash  text PRIMARY KEY,
  client_id   text NOT NULL,
  user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scope       text NOT NULL DEFAULT '',
  kind        text NOT NULL CHECK (kind IN ('access','refresh')),
  expires_at  timestamptz NOT NULL,
  revoked     boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_subscriptions (
  id         text PRIMARY KEY,
  url        text NOT NULL,
  secret     text NOT NULL,
  events     text[] NOT NULL,
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
  id              bigserial PRIMARY KEY,
  subscription_id text NOT NULL REFERENCES webhook_subscriptions(id) ON DELETE CASCADE,
  event           text NOT NULL,
  payload         jsonb NOT NULL,
  signature       text NOT NULL,
  status_code     int,
  error           text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE external_events (
  id          bigserial PRIMARY KEY,
  source      text NOT NULL,
  type        text NOT NULL,
  external_id text NOT NULL,
  payload     jsonb NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source, external_id)
);

CREATE TABLE connectors (
  id         text PRIMARY KEY,
  name       text NOT NULL,
  subtitle   text NOT NULL,
  logo       text NOT NULL,
  color      text NOT NULL,
  text_color text NOT NULL DEFAULT '#fff',
  grp        text NOT NULL CHECK (grp IN ('source','identity')),
  seq        int  NOT NULL,
  mode       text NOT NULL DEFAULT '',
  status     text NOT NULL DEFAULT 'off',
  detail     text NOT NULL DEFAULT '',
  last_sync_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sync_runs (
  id          bigserial PRIMARY KEY,
  system      text NOT NULL,
  started_at  timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  ok          boolean,
  counts      jsonb NOT NULL DEFAULT '{}',
  error       text NOT NULL DEFAULT ''
);

CREATE TABLE sync_watermarks (
  system     text NOT NULL,
  model      text NOT NULL,
  company    bigint NOT NULL DEFAULT 0,
  watermark  text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (system, model, company)
);

-- Raw mirror of Odoo records (read-only sync, Stage 08).
CREATE TABLE odoo_records (
  model      text NOT NULL,
  odoo_id    bigint NOT NULL,
  company_id bigint NOT NULL DEFAULT 0,
  write_date text NOT NULL,
  data       jsonb NOT NULL,
  synced_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (model, odoo_id)
);

CREATE TABLE odoo_links (
  arc_type   text NOT NULL,
  arc_id     text NOT NULL,
  odoo_model text NOT NULL,
  odoo_id    bigint NOT NULL,
  confidence numeric(4,3) NOT NULL,
  status     text NOT NULL DEFAULT 'linked' CHECK (status IN ('linked','proposed','rejected')),
  linked_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (arc_type, arc_id)
);

CREATE TABLE odoo_writes (
  id          bigserial PRIMARY KEY,
  action_id   text,
  model       text NOT NULL,
  method      text NOT NULL,
  odoo_id     bigint,
  payload     jsonb NOT NULL,
  dry_run     boolean NOT NULL DEFAULT false,
  ok          boolean NOT NULL,
  error       text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_google_tokens (
  user_id       text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  enc_token     text NOT NULL,
  scopes        text[] NOT NULL,
  consent_at    timestamptz NOT NULL DEFAULT now(),
  history_id    text NOT NULL DEFAULT ''
);

CREATE TABLE unresolved_identities (
  id          bigserial PRIMARY KEY,
  kind        text NOT NULL,
  value       text NOT NULL,
  interaction_id bigint REFERENCES interactions(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (kind, value)
);

CREATE TABLE privacy_rules (
  id       text PRIMARY KEY,
  title    text NOT NULL,
  detail   text NOT NULL,
  enabled  boolean NOT NULL DEFAULT true,
  locked   boolean NOT NULL DEFAULT false,
  seq      int NOT NULL
);

CREATE TABLE ask_history (
  id         text PRIMARY KEY,
  user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question   text NOT NULL,
  answer     jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ask_history_user_idx ON ask_history(user_id, created_at);

CREATE TABLE settings (
  key        text PRIMARY KEY,
  value      jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE metric_snapshots (
  key         text NOT NULL,
  subject     text NOT NULL,
  value       numeric NOT NULL,
  computed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (key, subject)
);
