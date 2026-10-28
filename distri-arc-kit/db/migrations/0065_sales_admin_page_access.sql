-- +goose Up
-- Menu Sales, Admin, dan Super Admin (Sam 26–28 Oct). Sales and Admin use the operating pages only. Sales are limited
-- to their own dealer data and WhatsApp chats; Admin sees all sales and all chats. CEO keeps every page and all data.
-- Dashboard, Cek stok, AI Trigger and Buku manual become pages of their own (no longer derived from another menu).
-- The roles as they are now are kept in roles_backup_0065: Down puts them back exactly.
create table roles_backup_0065 as select * from roles;

update roles
set screens = array['today','orch','chat','orbit','dealer','stock','stockcheck','ar'],
    scope = 'own', chat_scope = 'own',
    description = 'Pusat kendali, Orchestrator, chat dan halaman operasional miliknya'
where key = 'sales';

update roles
set screens = array['today','orch','chat','orbit','dealer','stock','stockcheck','ar'],
    scope = 'all', chat_scope = 'all', base = 'admin',
    description = 'Halaman operasional · semua sales dan semua cabang'
where key = 'admin';

update roles
set screens = array['dashboard','today','orch','chat','reportwa','orbit','kuad','net','dealer','stock','stockcheck','ar',
                    'users','roles','branches','mcp','aitrigger','conn','konsep','manual'],
    scope = 'all', chat_scope = 'all', policies = true,
    description = 'Semua halaman · semua data · hak kebijakan'
where key = 'ceo';

-- every other role keeps the menu it sees today: the pages it used to get through another menu are now written out
-- (Dashboard with Pusat kendali, Cek stok with Push stok or Chat, AI Trigger with Pengaturan or Chat, Buku manual for
-- everyone); a role without its own list gets its base's list written out first
update roles r
set screens = (
  select array_agg(distinct p) from unnest(
    s.cur
    || case when 'today' = any(s.cur) then array['dashboard'] else array[]::text[] end
    || case when 'stock' = any(s.cur) or 'chat' = any(s.cur) then array['stockcheck'] else array[]::text[] end
    || case when 'conn' = any(s.cur) or 'chat' = any(s.cur) then array['aitrigger'] else array[]::text[] end
    || array['manual']
  ) p)
from (
  select key, coalesce(screens, case base
    when 'sales' then array['today','orch','chat','reportwa','orbit','kuad','net','dealer','stock','konsep']
    when 'finance' then array['today','orch','chat','reportwa','orbit','kuad','dealer','ar','konsep']
    when 'warehouse' then array['today','chat','stock','dealer','konsep']
    else array['today','orch','chat','reportwa','orbit','kuad','net','dealer','stock','ar','users','roles','branches','mcp','conn','konsep']
  end) as cur
  from roles
) s
where r.key = s.key and r.key not in ('sales', 'admin', 'ceo');

-- +goose Down
update roles r
set screens = b.screens, scope = b.scope, chat_scope = b.chat_scope, base = b.base, policies = b.policies, description = b.description
from roles_backup_0065 b
where r.key = b.key;
drop table roles_backup_0065;
