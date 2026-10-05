-- +goose Up
-- Stage 05: what the ActionSheet and the Keputusan queue show for a proposal, and which button was chosen.
alter table proposals
  add column summary text,                        -- one-line description (queue card)
  add column button text,                         -- primary action label ("Setujui DP 50%")
  add column icon text,                           -- sprite icon name
  add column pills jsonb,                         -- [["bad","AI Kredit"],["neutral","Diajukan Fajar · Jakarta"]]
  add column options jsonb,                       -- [{key,label,style,result}] alternatives in the queue
  add column queue bool not null default false,   -- shown in "Keputusan" (high-stakes kinds)
  add column payload jsonb,                       -- machine data for execution (SO lines, limit, target contact…)
  add column chosen_option text,
  add column dedupe_key text;
create unique index proposals_open_dedupe on proposals (dedupe_key) where status in ('proposed','approved','edited');
create index proposals_dealer on proposals (dealer_id, created_at desc);

-- +goose Down
drop index if exists proposals_dealer;
drop index if exists proposals_open_dedupe;
alter table proposals drop column dedupe_key, drop column chosen_option, drop column payload, drop column queue,
  drop column options, drop column pills, drop column icon, drop column button, drop column summary;
