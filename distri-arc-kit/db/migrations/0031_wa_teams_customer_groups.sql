-- +goose Up
-- WhatsApp teams and customer groups (ADR 0033). Every number belongs to one team — sales, admin or billing
-- (penagihan) — so Chat groups the numbers by team instead of one flat row. A new link carries its team until
-- WhatsApp reports the number. Existing numbers take the team of their holder's role (finance → billing,
-- other non-sales roles → admin); a number with a sales or no holder stays sales.
alter table wa_numbers add column team text not null default 'sales' check (team in ('sales','admin','billing'));
alter table wa_links add column team text not null default 'sales' check (team in ('sales','admin','billing'));
update wa_numbers n set team = case u.role when 'finance' then 'billing' else 'admin' end
from users u where u.id = n.user_id and n.sales_id is null and u.role in ('finance','admin','ceo','warehouse');

-- A group with customers (kind external, "Grup pelanggan") is read by Analisa WhatsApp; an internal group never is.
-- kind_manual: a person chose the kind (Pengaturan or Chat) — the automatic rule no longer changes it.
alter table wa_groups add column kind_manual bool not null default false;
update wa_groups g set kind_manual = true
where exists (select 1 from audit_log a where a.action = 'wa_group.update' and a.entity_id = g.id);
-- every group started internal: one where a customer of the master wrote becomes a customer group
update wa_groups g set kind = 'external'
where g.kind = 'internal' and not g.kind_manual and exists (
  select 1 from chat_threads t join chat_messages m on m.thread_id = t.id join contacts c on c.wa_number = m.from_number
  where t.group_id = g.id and m.direction = 'in' and not m.internal);
update chat_threads t set subtitle = 'Grup pelanggan' from wa_groups g where g.id = t.group_id and g.kind = 'external';

alter table wa_insights drop constraint wa_insights_kind_check;
alter table wa_insights add constraint wa_insights_kind_check check (kind in ('dealer','new','group'));

-- +goose Down
delete from wa_insights where kind = 'group';
alter table wa_insights drop constraint wa_insights_kind_check;
alter table wa_insights add constraint wa_insights_kind_check check (kind in ('dealer','new'));
alter table wa_groups drop column kind_manual;
alter table wa_links drop column team;
alter table wa_numbers drop column team;
