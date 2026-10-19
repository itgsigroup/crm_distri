-- +goose Up
-- Analisa Relasi (ADR 0054): the Peta relasi page's AI reading — how close the sales are to their dealers, from the chats.
alter table screen_analyses drop constraint if exists screen_analyses_screen_check;
alter table screen_analyses add constraint screen_analyses_screen_check check (screen in ('stock', 'credit', 'orbit', 'segmen', 'relasi'));

-- +goose Down
delete from screen_analyses where screen = 'relasi';
alter table screen_analyses drop constraint if exists screen_analyses_screen_check;
alter table screen_analyses add constraint screen_analyses_screen_check check (screen in ('stock', 'credit', 'orbit', 'segmen'));
