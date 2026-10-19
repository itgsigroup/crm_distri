-- +goose Up
-- AI Trigger (ADR 0051, Sam 19 Oct): every trigger has its recommendation written by AI the moment a customer message
-- matches it; a new trigger too. The numbers stay computed by the system; the template draft remains the fallback.
update reply_rules set ai_phrase = true where not ai_phrase;
alter table reply_rules alter column ai_phrase set default true;

-- +goose Down
alter table reply_rules alter column ai_phrase set default false;
