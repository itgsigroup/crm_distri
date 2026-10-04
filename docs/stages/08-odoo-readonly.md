# Stage 08 — Odoo read-only sync & penautan (Odoo jadi source of truth)

## Tujuan
Menyambungkan ARC ke Odoo tanpa satu pun tulis: sinkron incremental akun/kontak/opportunity/stage/SO/project/invoice/pembayaran, **menautkan** data ARC-native yang sudah ada ke record Odoo, lalu Odoo menjadi source of truth untuk stage.

## Baca dulu
`03` (Odoo), `00` (multi-company, definisi segmen L2C), CLAUDE.md §2 (dua fase).

## Kerjakan
1. `OdooClient` (XML-RPC, API key, batch 200, retry, `allowed_company_ids`) + `FakeOdooClient` dari fixture (3 company `[NEW]`, `crm.stage` nyata).
2. Mapper Odoo → domain; `StageDefinition` diganti/dipetakan ke `crm.stage` (nama dari data). Job `sync_odoo` per jam + manual; watermark per model/company; `SyncRun`.
3. **Penautan**: cocokkan Account (nama/domain/telepon), Person (telepon/email), Opportunity (akun + nama + nilai ± 20% + tanggal) → usulan tautan sebagai Action `link_to_odoo` (auto bila conf ≥ 0.95, sisanya approve); setelah tertaut, field Odoo read-only di ARC dan stage dikunci ke Odoo; opportunity ARC tanpa padanan → Action `create_in_odoo` (dieksekusi di tahap 10) atau `archive`.
4. L2C & CashItem dari SO/project/invoice/payment (definisi segmen Sam; field tanggal custom via `fields_get`, fallback chatter).
5. UI: bar sinkron Odoo (status, jumlah tertaut, konflik), kanban memakai stage Odoo, panel Odoo vs bukti ARC.

## Acceptance criteria
- FakeOdoo: sync penuh lalu ulang → 0 perubahan; ubah `write_date` satu lead → 1 update.
- Penautan fixture: 8 opportunity ARC → 6 tertaut otomatis, 2 usulan; tidak ada duplikat akun.
- L2C 6 kasus benar. Tidak ada `create/write/unlink` di connector (test grep).
- Tanpa kredensial → `done-with-mocks`.
