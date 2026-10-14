-- +goose Up
-- Customer groups, second rule (ADR 0033 rev. 14 Oct): WhatsApp hides most group members' numbers (LID), so few
-- groups matched a customer contact. A group is now a customer group unless everyone in it is GSI's (an internal
-- number, a linked number, a user's or a sales' number); a group named for internal work (gudang, internal, karyawan,
-- staff, HRD, absensi) stays internal; a kind a person chose is kept.
-- A GSI employee's message in a group is our side (internal), whichever GSI number list knows them.
update chat_messages m set internal = true
from chat_threads t
where t.id = m.thread_id and t.kind = 'group' and m.direction = 'in' and not m.internal and coalesce(m.from_number, '') <> ''
  and (exists (select 1 from internal_numbers i where i.wa_number = m.from_number)
    or exists (select 1 from wa_numbers w where w.wa_number = m.from_number)
    or exists (select 1 from users u where u.wa_number = m.from_number)
    or exists (select 1 from sales_users s where s.wa_number = m.from_number));
update wa_groups g set kind = 'external'
where g.kind = 'internal' and not g.kind_manual
  and lower(coalesce(g.name, '')) !~ '(internal|gudang|karyawan|staff|hrd|absensi)'
  and exists (select 1 from chat_threads t join chat_messages m on m.thread_id = t.id
              where t.group_id = g.id and m.direction = 'in' and not m.internal);
update chat_threads t set subtitle = 'Grup pelanggan' from wa_groups g where g.id = t.group_id and g.kind = 'external';

-- Penagihan sends by hand from its own number in Chat: the finance role opens Chat and may hold a number.
update roles set screens = array_append(screens, 'chat') where base = 'finance' and screens is not null and not ('chat' = any(screens));
update roles set wa_allowed = true where base = 'finance';

-- +goose Down
select 1;
