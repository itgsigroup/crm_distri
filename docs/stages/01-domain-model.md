# Stage 01 — Domain model & storage (ARC-native)

## Tujuan
Skema domain lengkap (02-domain-model.md) dengan migrasi Alembic, repository, audit, provenance, health function, dan seed — **tanpa Odoo**: Account/Person/Opportunity adalah milik ARC dulu, siap ditautkan ke Odoo di tahap 08.

## Baca dulu
`02-domain-model.md`, `04-policies-privacy.md`, ADR 0001.

## Kerjakan
1. `packages/core/domain/`: Pydantic models semua entitas + enum + `Provenance`.
2. `packages/core/storage/`: SQLAlchemy 2 + Alembic (SQLite WAL, FK on). Index pada account_id, occurred_at, status, `raw_ref` unik, `wamid` unik.
3. `StageDefinition` tabel: seed `Baru, Berkualifikasi, Penawaran, Won, Lost` (urutan, is_won, is_lost) — dapat diubah di Pengaturan; Opportunity.stage_id merujuk ke sini. Field `source_system/source_id` nullable di Account/Person/Opportunity untuk penautan Odoo nanti.
4. Repository per agregat (upsert-by-source, get, list-with-filters, paging). `AuditLog` append-only via context manager `audit(actor, verb, obj)`.
5. `compute_health(inputs) -> HealthResult` (rumus di 02) + unit test 4 kasus.
6. Idempotensi: `upsert_interaction(raw_ref)`, `upsert_commitment(hash)`.
7. `make seed`: muat fixture (akun, orang, opportunity dengan stage seed, komitmen, interaksi timeline, edge WA per bulan sebagai Interaction agregat, inbound, internal numbers, chats).
8. Endpoint baca: `GET /accounts`, `GET /accounts/{id}` (brief), `GET /people`, `GET /opportunities`, `GET /stages`, `GET /actions?status=`.

## Acceptance criteria
- `make db-upgrade && make seed` → `GET /accounts/rsud` health 41 ± 3 dari fixture dengan breakdown.
- Seed dua kali → tidak ada baris bertambah. Health test lolos. AuditLog terisi.
- mypy strict lolos di `packages/core`.
