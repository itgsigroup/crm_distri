-- name: QueueCycle :one
-- Fails on cycles_one_active when a cycle is queued or running (→ 409 cycle_running).
insert into cycles (trigger, scope, via, requested_by, status, started_at)
values ($1, $2, $3, $4, 'queued', $5) returning *;

-- name: StartCycle :exec
update cycles set status = 'running', stage = 'ingest', started_at = $2 where id = $1;

-- name: SetCycleStage :exec
update cycles set stage = $2 where id = $1;

-- name: FinishCycle :exec
update cycles set status = $2, stage = null, finished_at = $3, duration_ms = $4, signals_count = $5, auto_count = $6,
  decision_count = $7, conflict_count = $8, note = $9
where id = $1;

-- name: FailStaleCycles :exec
-- A crashed worker leaves its cycle running; the next worker start marks it failed so a new one can run. Queued
-- cycles keep their job unless they are older than `before` (the job was lost).
update cycles set status = 'failed', stage = null, finished_at = now(), note = coalesce(note, 'dihentikan: worker berhenti')
where status = 'running' or (status = 'queued' and started_at < sqlc.arg(before)::timestamptz);

-- name: GetCycle :one
select * from cycles where id = $1;

-- name: LatestCycle :one
select * from cycles order by started_at desc, number desc limit 1;

-- name: LatestDoneCycle :one
select * from cycles where status in ('done', 'partial') order by started_at desc, number desc limit 1;

-- name: LatestFullCycle :one
-- The last finished cycle with scope all (Rencana hari ini, Ringkasan, conflicts come from it).
select * from cycles where status in ('done', 'partial') and scope = 'all' order by started_at desc, number desc limit 1;

-- name: ListCycles :many
select * from cycles order by started_at desc, number desc limit $1;

-- name: UpsertCycleStage :exec
insert into cycle_stages (cycle_id, stage, status, started_at, finished_at, detail)
values ($1, $2, $3, $4, $5, $6)
on conflict (cycle_id, stage) do update set status = excluded.status, started_at = coalesce(cycle_stages.started_at, excluded.started_at),
  finished_at = excluded.finished_at, detail = excluded.detail;

-- name: ListCycleStages :many
select * from cycle_stages where cycle_id = $1
order by array_position(array['ingest','analyze','synthesize','decide','execute','learn'], stage);

-- name: InsertAgentRun :exec
insert into agent_runs (cycle_id, agent, status, duration_ms, input_hash, proposals_count, error)
values ($1, $2, $3, $4, $5, $6, $7)
on conflict (cycle_id, agent) do update set status = excluded.status, duration_ms = excluded.duration_ms,
  proposals_count = excluded.proposals_count, error = excluded.error;

-- name: ListAgentRuns :many
select * from agent_runs where cycle_id = $1 order by agent;

-- name: InsertConflict :exec
insert into conflicts (cycle_id, dealer_id, agent_a, agent_b, title, resolution, rule, proposal_ids, tone, visible)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: ListConflicts :many
select c.*, d.name as dealer_name, d.slug as dealer_slug from conflicts c left join dealers d on d.id = c.dealer_id
where c.cycle_id = $1 and (c.visible or sqlc.arg(all_rules)::bool)
order by c.created_at, c.id;

-- name: UnprocessedSignals :many
-- Signals not yet seen by a cycle (optionally one dealer).
select id, dealer_id, kind, occurred_at from signals
where processed_at is null and (sqlc.narg(dealer_id)::uuid is null or dealer_id = sqlc.narg(dealer_id))
order by occurred_at;

-- name: MarkSignalsProcessed :exec
update signals set processed_at = sqlc.arg(at)::timestamptz where id = any(sqlc.arg(ids)::uuid[]);

-- name: CycleProposals :many
-- Proposals of today that the plan is built from (any decision state except expired), newest first.
select p.*, d.name as dealer_name, d.slug as dealer_slug from proposals p left join dealers d on d.id = p.dealer_id
where p.kind <> 'reply' and p.created_at >= sqlc.arg(since)::timestamptz and p.status not in ('expired')
order by p.created_at desc;

-- name: ProposalsOfCycle :many
select p.*, d.name as dealer_name, d.slug as dealer_slug from proposals p left join dealers d on d.id = p.dealer_id
where p.cycle_id = $1 order by p.created_at;

-- name: OpenProposalByKey :one
select * from proposals where dedupe_key = $1 and status not in ('expired', 'rejected', 'suppressed') order by created_at desc limit 1;

-- name: DeletePlan :exec
delete from plan_items where plan_date = $1;

-- name: InsertPlanItem :exec
insert into plan_items (plan_date, cycle_id, seq, time_label, agent, autonomy, text_html, proposal_id, proposal_ids, status, link, wait_for)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: ListPlan :many
select * from plan_items where plan_date = $1 order by seq;

-- name: GetPlanItem :one
select * from plan_items where id = $1;

-- name: SetPlanStatus :exec
update plan_items set status = $2 where id = $1;

-- name: SyncPlanStatus :exec
-- Plan steps follow their proposals: decided → done/skipped, otherwise unchanged.
update plan_items pi set status = case
    when (select bool_and(p.status in ('executed','approved','edited')) from proposals p where p.id = any(coalesce(pi.proposal_ids, array[pi.proposal_id]))) then 'done'
    when (select bool_and(p.status in ('rejected','expired','suppressed')) from proposals p where p.id = any(coalesce(pi.proposal_ids, array[pi.proposal_id]))) then 'skipped'
    else pi.status end
where pi.plan_date = $1 and (pi.proposal_id is not null or pi.proposal_ids is not null) and pi.status <> 'waiting';

-- name: ReleaseWaiting :exec
-- A waiting step becomes scheduled once what it waits for happened (payment of the invoice).
update plan_items set status = 'scheduled', wait_for = null where id = $1 and status = 'waiting';

-- name: PaymentForInvoice :one
-- Whether an invoice was paid (fully) — releases "setelah pembayaran masuk" steps.
select exists (select 1 from invoices where number = $1 and paid >= total) as paid;

-- name: UpsertAgentState :exec
insert into agent_state (agent, confidence, last_run_at, last_output, params) values ($1, $2, $3, $4, $5)
on conflict (agent) do update set confidence = excluded.confidence, last_run_at = excluded.last_run_at,
  last_output = excluded.last_output, params = coalesce(excluded.params, agent_state.params);

-- name: ListAgentState :many
select * from agent_state order by agent;

-- name: EditedSince :many
-- Proposals a human edited since the last cycle: few-shot examples for their agent.
select id, agent, kind, preview, edited_payload, decided_at from proposals
where status in ('edited', 'executed') and edited_payload is not null and decided_at >= sqlc.arg(since)::timestamptz
order by decided_at desc limit 50;

-- name: RejectedSince :many
select c.agent, c.kind, c.reason, c.dealer_id from calibration_events c where c.created_at >= sqlc.arg(since)::timestamptz;

-- name: ApprovedWithoutOutbox :many
-- Approved proposals the Eksekusi stage still has to carry out (auto approvals; human approvals already queued).
select p.* from proposals p
where p.status in ('approved', 'edited') and not exists (select 1 from outbox o where o.proposal_id = p.id)
order by p.decided_at;

-- name: LastSentFollowup :one
-- When the last follow-up to a dealer went out (followup_gap rule).
select max(coalesce(p.executed_at, p.decided_at))::timestamptz as at from proposals p
where p.dealer_id = $1 and p.kind = 'followup' and p.status in ('approved', 'edited', 'executed');

-- name: SalesInteractions :many
-- Interactions per sales number with a dealer in the last 30 days (one_owner rule).
select su.name as sales, count(*)::bigint as n
from chat_messages m join chat_threads t on t.id = m.thread_id join sales_users su on su.id = t.sales_id
where t.dealer_id = $1 and m.sent_at >= sqlc.arg(since)::timestamptz
group by su.name order by n desc;

-- name: NewNumberThreads :many
-- Inbound unknown numbers with an identification (AI Prospek).
select t.id as thread_id, t.wa_jid, su.name as sales, i.wa_number, i.best_name, i.best_org, i.score, i.sources,
  m.signal_id, m.body as last_text, m.sent_at as last_at
from chat_threads t
join identifications i on t.wa_jid like i.wa_number || '@%'
join lateral (select signal_id, body, sent_at from chat_messages where thread_id = t.id and direction = 'in' and signal_id is not null
  order by sent_at desc limit 1) m on true
left join sales_users su on su.id = t.sales_id
where t.kind = 'new' and t.dealer_id is null;

-- name: LatestStockSignals :many
-- Newest stock signal per summary prefix ("Stok <name> <branch>:") — provenance for AI Stok.
select distinct on (split_part(summary, ':', 1)) id, summary from signals
where kind = 'stock' and summary is not null
order by split_part(summary, ':', 1), occurred_at desc;

-- name: SetPolicy :one
-- Writes a policy as a new version (Pengaturan, PUT /policies/autonomy); takes effect on the next cycle.
insert into policies (key, value, version, updated_by, updated_at) values ($1, $2, 1, $3, $4)
on conflict (key) do update set value = excluded.value, version = policies.version + 1, updated_by = excluded.updated_by, updated_at = excluded.updated_at
returning *;

-- name: TodayAgentCounts :many
-- Proposals per agent today (agent cards).
select agent, count(*)::bigint as n from proposals where created_at >= sqlc.arg(since)::timestamptz and kind <> 'reply' and payload->>'parent' is null
group by agent;

-- name: MovePlanItem :exec
update plan_items set time_label = $2 where id = $1;

-- name: NextPlanSeq :one
select coalesce(max(seq), 0)::int + 1 from plan_items where plan_date = $1;
