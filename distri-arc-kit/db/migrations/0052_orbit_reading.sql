-- +goose Up
-- Analisa Orbit (ADR 0052): the Orbit page's AI reading — the dealers in orbit read with their WhatsApp chats.
alter table screen_analyses drop constraint if exists screen_analyses_screen_check;
alter table screen_analyses add constraint screen_analyses_screen_check check (screen in ('stock', 'credit', 'orbit'));

-- +goose Down
delete from screen_analyses where screen = 'orbit';
alter table screen_analyses drop constraint if exists screen_analyses_screen_check;
alter table screen_analyses add constraint screen_analyses_screen_check check (screen in ('stock', 'credit'));
