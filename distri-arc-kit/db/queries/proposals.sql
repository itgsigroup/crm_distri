-- name: InsertAgentProposal :one
insert into proposals (cycle_id, agent, dealer_id, kind, title, why, prep, preview, steps, impact, confidence, signal_ids,
  autonomy, status, due_label, summary, button, icon, pills, options, queue, payload, dedupe_key)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
on conflict (dedupe_key) where status in ('proposed','approved','edited') do nothing
returning id;

-- name: ListProposals :many
select p.*, d.slug as dealer_slug, d.name as dealer_name, s.name as decided_by_name
from proposals p left join dealers d on d.id = p.dealer_id left join sales_users s on s.id = p.decided_by
where (sqlc.narg(status)::text is null or p.status = sqlc.narg(status))
  and (sqlc.narg(agent)::text is null or p.agent = sqlc.narg(agent))
  and (sqlc.narg(dealer)::uuid is null or p.dealer_id = sqlc.narg(dealer))
  and p.kind <> 'reply'
order by p.created_at desc limit sqlc.arg(lim);

-- name: GetProposal :one
select p.*, d.slug as dealer_slug, d.name as dealer_name, s.name as decided_by_name
from proposals p left join dealers d on d.id = p.dealer_id left join sales_users s on s.id = p.decided_by
where p.id = $1;

-- name: DecideProposal :one
update proposals set status = $2, decided_by = $3, decided_at = $4, decision_reason = $5, edited_payload = $6, chosen_option = $7
where id = $1 and status = 'proposed'
returning *;

-- name: ActiveSuppressions :many
select agent, dealer_id, kind, suppress_until from calibration_events where suppress_until >= $1;

-- name: InsertCalibration :exec
insert into calibration_events (proposal_id, agent, dealer_id, kind, decision, reason, suppress_until)
values ($1, $2, $3, $4, $5, $6, $7);

-- name: ListCalibration :many
select c.*, p.title from calibration_events c left join proposals p on p.id = c.proposal_id order by c.created_at desc limit $1;

-- name: InsertLLMCall :one
insert into llm_calls (cycle_id, agent, provider, model, purpose, input_hash, tokens_in, tokens_out, cost_idr, duration_ms)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) returning id;

-- name: ExpireOpenProposals :exec
-- Proposals of earlier days that nobody decided expire so the queue only shows what is current.
update proposals set status = 'expired' where status = 'proposed' and created_at < $1;

-- name: DealerSignals :many
select id, kind, occurred_at, summary, payload from signals where dealer_id = $1 order by occurred_at desc limit $2;

-- name: RecentInboundWA :many
select s.id, s.dealer_id, s.contact_id, s.occurred_at, s.summary, s.payload, c.name as contact_name
from signals s left join contacts c on c.id = s.contact_id
where s.kind = 'wa' and s.dealer_id is not null and s.occurred_at >= $1 and coalesce(s.payload->>'direction','in') = 'in'
order by s.occurred_at;

-- name: LastFollowup :one
select coalesce(max(executed_at), 'epoch'::timestamptz)::timestamptz as last from proposals
where dealer_id = $1 and kind = 'followup' and status = 'executed';

-- name: ProposalKeyUsed :one
select exists(select 1 from proposals where dedupe_key = $1 and status <> 'expired')::bool;

-- name: SetProposalStatus :exec
update proposals set status = $2, executed_at = case when $2 = 'executed' then now() else executed_at end where id = $1;

-- name: ApproveAuto :exec
update proposals set status = 'approved', decided_at = $2, decision_reason = $3 where id = $1;

-- name: ListProposalsSince :many
select p.id, p.dealer_id, p.kind, p.title, p.button, p.icon, p.agent, p.due_label, p.status, p.decided_at, p.executed_at, p.why, p.queue, p.created_at, p.autonomy, p.confidence
from proposals p where p.created_at >= $1 and p.kind <> 'reply' and p.status <> 'expired' order by p.created_at;

-- name: FindContactByName :one
select * from contacts where dealer_id = $1 and name = $2 limit 1;

-- name: FindThreadBySalesJID :one
select id from chat_threads where sales_id = $1 and wa_jid = $2;

-- name: ProposalsForSignals :many
select id, signal_ids, button, icon, status, title, kind, executed_at, decided_at from proposals
where signal_ids && sqlc.arg(ids)::uuid[] and status <> 'expired' and kind <> 'reply' order by created_at desc;

-- name: AgentDecisionStats :many
-- Human decisions per agent (calibration bar): approved/edited vs rejected.
select agent,
  count(*) filter (where status in ('approved','edited','executed') and decided_by is not null)::bigint as accepted,
  count(*) filter (where status = 'rejected')::bigint as rejected
from proposals
where kind <> 'reply' and decided_at >= sqlc.arg(since)::timestamptz
group by agent order by agent;
