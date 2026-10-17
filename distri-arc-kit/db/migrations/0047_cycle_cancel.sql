-- +goose Up
-- "Batalkan analisis" (Sam 17 Oct): a person stops a cycle that waits (e.g. Claude was never opened); the worker sees
-- cancelled_at at the next stage or while it waits for Claude and ends the cycle as failed with who cancelled it.
alter table cycles add column cancelled_at timestamptz, add column cancelled_by text;

-- +goose Down
alter table cycles drop column cancelled_at, drop column cancelled_by;
