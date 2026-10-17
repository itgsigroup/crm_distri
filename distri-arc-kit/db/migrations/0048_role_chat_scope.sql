-- +goose Up
-- Chat WhatsApp per role (Sam 17 Oct): 'all' reads every number, 'own' only the numbers the user holds — so a role with
-- the management pages (like Penagihan) still reads only its own chats. Role Penagihan: the Admin pages, own chats;
-- an admin who holds a Penagihan (billing) number moves to it.
alter table roles add column chat_scope text not null default 'all' check (chat_scope in ('all', 'own'));
update roles set chat_scope = 'own' where base = 'sales' or scope = 'own';

insert into roles (key, name, description, base, screens, decide, wa_allowed, active, scope, policies, chat_scope, updated_by)
select 'penagihan', 'Penagihan', 'Halaman Admin; chat WhatsApp hanya nomor yang dipegang', base, screens, decide, wa_allowed, true, scope, false, 'own', 'migrasi 0048'
from roles where key = 'admin'
on conflict (key) do nothing;

update users u set role_key = 'penagihan'
where u.role_key = 'admin' and exists (select 1 from wa_numbers n where n.user_id = u.id and n.team = 'billing');

-- +goose Down
update users set role_key = 'admin' where role_key = 'penagihan';
delete from roles where key = 'penagihan';
alter table roles drop column chat_scope;
