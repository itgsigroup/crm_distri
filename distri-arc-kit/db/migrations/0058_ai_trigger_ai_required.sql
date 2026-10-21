-- +goose Up
-- AI Trigger (Sam 21 Oct): every recommendation in Chat is written by ChatGPT or Claude — mandatory, never the
-- template. The template draft stays as the AI's frame of facts; a rule can no longer turn the AI off.
update reply_rules set ai_phrase = true where not ai_phrase;
alter table reply_rules add constraint reply_rules_ai_required check (ai_phrase);

-- +goose Down
alter table reply_rules drop constraint if exists reply_rules_ai_required;
