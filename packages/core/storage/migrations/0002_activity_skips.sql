-- Stage 10: track whether the Odoo activity of a "kami" commitment was marked done.
ALTER TABLE commitments ADD COLUMN odoo_activity_done boolean NOT NULL DEFAULT false;

-- Stage 02: remember skipped messages (hash of session + wamid only, never content)
-- so replays do not inflate the "not read" counters.
CREATE TABLE wa_skipped (
  key        text PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);
