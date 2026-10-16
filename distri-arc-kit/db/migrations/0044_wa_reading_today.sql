-- +goose Up
-- Analisa WhatsApp reads only today's new customer messages (Sam 16 Oct: save tokens): what the AI already read stays
-- on Pusat kendali — a conversation's line and the overview are carried to the next cycle until new messages come.
alter table wa_insights add column summary_at timestamptz; -- when the AI wrote this conversation's line
update wa_insights i set summary_at = a.created_at from wa_analyses a where a.cycle_id = i.cycle_id and i.summary is not null;
create index wa_insights_summary on wa_insights (thread_id, summary_at desc) where summary is not null;

-- +goose Down
drop index wa_insights_summary;
alter table wa_insights drop column summary_at;
