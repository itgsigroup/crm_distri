-- +goose Up
-- Analisa Segmen (ADR 0053): the Segmen page's AI reading — the dealers moving between segments read with their chats.
alter table screen_analyses drop constraint if exists screen_analyses_screen_check;
alter table screen_analyses add constraint screen_analyses_screen_check check (screen in ('stock', 'credit', 'orbit', 'segmen'));

-- +goose Down
delete from screen_analyses where screen = 'segmen';
alter table screen_analyses drop constraint if exists screen_analyses_screen_check;
alter table screen_analyses add constraint screen_analyses_screen_check check (screen in ('stock', 'credit', 'orbit'));
