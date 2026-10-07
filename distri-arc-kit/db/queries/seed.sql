-- Idempotent upserts used by `arc ctl seed` and, later, by the Odoo sync (same source keys).

-- name: UpsertSalesUser :one
insert into sales_users (name, branch, wa_number, odoo_user_id, role, source_system, source_id)
values ($1, $2, $3, $4, $5, $6, $7)
on conflict (source_system, source_id) do update
  set name = excluded.name, branch = excluded.branch, wa_number = excluded.wa_number,
      odoo_user_id = excluded.odoo_user_id, role = excluded.role
returning id;

-- name: UpsertUser :one
insert into users (email, name, role, sales_user_id, role_key, wa_number)
values ($1, $2, $3, $4, $3, (select s.wa_number from sales_users s where s.id = $4 and not exists (select 1 from users x where x.wa_number = s.wa_number and lower(x.email) <> lower($1))))
on conflict (email) do update set name = excluded.name, role = excluded.role, sales_user_id = excluded.sales_user_id,
  role_key = coalesce(users.role_key, excluded.role_key), wa_number = coalesce(users.wa_number, excluded.wa_number)
returning id;

-- name: UpsertDealer :one
insert into dealers (slug, name, city, branch, tier, segment_desc, owner_id, credit_limit, payment_terms_days,
                     source_system, source_id, source_write_date, memo, memo_updated_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
on conflict (source_system, source_id) do update
  set slug = coalesce(dealers.slug, excluded.slug), name = excluded.name, city = excluded.city, branch = excluded.branch, tier = excluded.tier,
      segment_desc = excluded.segment_desc, owner_id = excluded.owner_id, credit_limit = excluded.credit_limit,
      payment_terms_days = excluded.payment_terms_days, source_write_date = excluded.source_write_date,
      memo = coalesce(dealers.memo, excluded.memo), memo_updated_at = coalesce(dealers.memo_updated_at, excluded.memo_updated_at),
      updated_at = now()
returning id;

-- name: SetDealerMemoSignals :exec
update dealers set memo_signal_ids = $2 where id = $1;

-- name: UpsertContact :one
insert into contacts (dealer_id, name, role, wa_number, is_primary, last_interaction_at, interactions_90d, source_system, source_id)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (source_system, source_id) do update
  set name = excluded.name, role = excluded.role, wa_number = excluded.wa_number, is_primary = excluded.is_primary,
      last_interaction_at = excluded.last_interaction_at, interactions_90d = excluded.interactions_90d
returning id;

-- name: UpsertSowEstimate :exec
insert into dealer_sow_estimates (dealer_id, quarter, sow, note, confirmed_by, confirmed_at)
values ($1, $2, $3, $4, $5, $6)
on conflict (dealer_id, quarter) do update set sow = excluded.sow, note = excluded.note,
  confirmed_by = excluded.confirmed_by, confirmed_at = excluded.confirmed_at;

-- name: UpsertOrder :one
insert into orders (dealer_id, number, state, ordered_at, confirmed_at, shipped_at, invoiced_at, paid_at,
                    total, margin_pct, lines, created_by, source_system, source_id, source_write_date)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
on conflict (source_system, source_id) do update
  set number = excluded.number, state = excluded.state, ordered_at = excluded.ordered_at,
      confirmed_at = excluded.confirmed_at, shipped_at = excluded.shipped_at, invoiced_at = excluded.invoiced_at,
      paid_at = excluded.paid_at, total = excluded.total, margin_pct = excluded.margin_pct, lines = excluded.lines,
      source_write_date = excluded.source_write_date
returning id;

-- name: UpsertInvoice :one
insert into invoices (dealer_id, order_id, number, issued_at, due_at, total, paid, paid_at, state,
                      source_system, source_id, source_write_date)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
on conflict (source_system, source_id) do update
  set order_id = excluded.order_id, number = excluded.number, issued_at = excluded.issued_at, due_at = excluded.due_at,
      total = excluded.total, paid = excluded.paid, paid_at = excluded.paid_at, state = excluded.state,
      source_write_date = excluded.source_write_date
returning id;

-- name: UpsertPayment :exec
insert into payments (dealer_id, invoice_id, paid_at, amount, source_system, source_id)
values ($1, $2, $3, $4, $5, $6)
on conflict (source_system, source_id) do update
  set invoice_id = excluded.invoice_id, paid_at = excluded.paid_at, amount = excluded.amount;

-- name: UpsertStockItem :exec
insert into stock_items (branch, sku, name, category, qty, unit_cost, value, age_days, weekly_velocity,
                         source_system, source_id, source_write_date)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
on conflict (source_system, source_id, branch) do update
  set sku = excluded.sku, name = excluded.name, category = excluded.category, qty = excluded.qty,
      unit_cost = excluded.unit_cost, value = excluded.value, age_days = excluded.age_days,
      weekly_velocity = excluded.weekly_velocity, source_write_date = excluded.source_write_date;

-- name: UpsertCommitment :exec
insert into commitments (dealer_id, side, title, detail, status, due_at, invoice_id, source_key)
values ($1, $2, $3, $4, $5, $6, $7, $8)
on conflict (source_key) do update
  set title = excluded.title, detail = excluded.detail, status = excluded.status, due_at = excluded.due_at,
      invoice_id = excluded.invoice_id;

-- name: UpsertSignal :one
-- signal_keys holds the dedupe key across monthly partitions: a new key inserts the signal, a known key updates
-- summary and payload of the stored one (its occurred_at stays).
with k as (
  insert into signal_keys (dedupe_key, signal_id, occurred_at)
  values (sqlc.arg(dedupe_key)::text, gen_random_uuid(), sqlc.arg(occurred_at)::timestamptz)
  on conflict (dedupe_key) do update set dedupe_key = excluded.dedupe_key
  returning signal_id, occurred_at, (xmax = 0) as fresh
), ins as (
  insert into signals (id, kind, dealer_id, contact_id, sales_id, occurred_at, dedupe_key, summary, payload)
  select k.signal_id, sqlc.arg(kind)::text, sqlc.narg(dealer_id)::uuid, sqlc.narg(contact_id)::uuid, sqlc.narg(sales_id)::uuid,
         k.occurred_at, sqlc.arg(dedupe_key)::text, sqlc.narg(summary)::text, sqlc.arg(payload)::jsonb
  from k where k.fresh
  returning id
), upd as (
  update signals s set summary = sqlc.narg(summary)::text, payload = sqlc.arg(payload)::jsonb
  from k where not k.fresh and s.id = k.signal_id and s.occurred_at = k.occurred_at
  returning s.id
)
select k.signal_id as id from k;

-- name: InsertPolicyIfMissing :exec
insert into policies (key, value) values ($1, $2) on conflict (key) do nothing;

-- name: UpsertCategoryMap :exec
insert into category_map (odoo_category_id, kat) values ($1, $2)
on conflict (odoo_category_id) do update set kat = excluded.kat;

-- name: CountSeeded :one
select (select count(*) from dealers)::bigint      as dealers,
       (select count(*) from orders)::bigint       as orders,
       (select count(*) from invoices)::bigint     as invoices,
       (select count(*) from payments)::bigint     as payments,
       (select count(*) from contacts)::bigint     as contacts,
       (select count(*) from signals)::bigint      as signals,
       (select count(*) from stock_items)::bigint  as stock_items,
       (select count(*) from sales_users)::bigint  as sales_users,
       (select count(*) from commitments)::bigint  as commitments,
       (select count(*) from policies)::bigint     as policies;
