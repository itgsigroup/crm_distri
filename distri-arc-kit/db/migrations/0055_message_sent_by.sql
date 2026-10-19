-- +goose Up
-- Who sent a message from GSI Orbit (Sam 19 Oct): the person who pressed Kirim / Teruskan / Setujui — shown as "dikirim
-- oleh <nama>" on the bubble for those who see every chat. Empty for messages received, and for ones sent from the phone.
alter table chat_messages add column sent_by uuid, add column sent_by_name text;

-- the replies, forwards and approvals already sent from GSI Orbit: the person is in the audit of the decision behind
-- them (Kirim / Teruskan in Chat, or Setujui on a proposal)
update chat_messages m set sent_by = u.id, sent_by_name = u.name
from (
  select distinct on (a.entity_id) a.entity_id, a.actor
  from audit_log a
  where a.entity = 'proposal' and a.actor_kind = 'user'
  order by a.entity_id, a.created_at
) a
join users u on u.email = a.actor
where m.proposal_id = a.entity_id and m.direction = 'out' and m.sent_by is null;

-- +goose Down
alter table chat_messages drop column sent_by_name, drop column sent_by;
