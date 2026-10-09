-- +goose Up
-- Mapping sales (ADR 0024): the source data (BigQuery / Accurate) spells one person several ways ("Granike Monica",
-- "Granike Monica M."). A source profile merged into a GSI Orbit sales keeps its row for the import key but is
-- inactive; its dealers, conversations and numbers belong to the target, and the import mapping points there too.
alter table sales_users add column merged_into uuid references sales_users on delete set null;
create index sales_users_merged_into on sales_users (merged_into) where merged_into is not null;

-- +goose Down
drop index if exists sales_users_merged_into;
alter table sales_users drop column merged_into;
