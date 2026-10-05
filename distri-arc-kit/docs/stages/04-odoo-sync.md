# Stage 04 — Odoo sync: dealer master, SO, invoice, pembayaran, stok; order-to-cash
**Baca**: `02-architecture.md`, `03-data-model.md` (sinyal & transaksi), `01-glossary.md` (Order-to-cash, KPI), `09` (Odoo SoT).

## Tujuan
Odoo menjadi sumber kebenaran: `arc` membaca partner (dealer + kontak), `sale.order`, `stock.picking`, `account.move`, pembayaran, `stock.quant` per cabang (multi-company GSI), memetakan ke tabel internal secara idempoten, menghitung fase order-to-cash, dan memicu `metrics.recompute`.

## Deliverables
1. `internal/odoo`: client JSON-RPC (`/web/session/authenticate`, `call_kw` `search_read` dengan `write_date > since`), konfigurasi per company/cabang, `Fake` dari `db/seed/odoo/*.json`.
2. Mapper: partner → `dealers`/`contacts` (tier dari `property_product_pricelist` atau tag; `credit_limit` dari field kustom/limit internal), SO → `orders` (+ `lines` dengan kategori via `category_map`), picking → `shipped_at`, invoice → `invoices`, payment → `payments`, quant → `stock_items` (age dari tanggal masuk, velocity dari 8 minggu SO).
3. Job `odoo.sync` tiap 10 menit (periodic) + `arc ctl odoo sync --full`; setiap record baru/berubah → `signals(kind='so'|'invoice'|'payment'|'stock')` dengan `dedupe_key = model:id:write_date`.
4. `category_map` admin UI minimal (Pengaturan → Sumber sinyal → Odoo) + `POST /connections/odoo/test`.
5. Order-to-cash: `orders.state` dihitung dari status Odoo; `DealerPage → Order-to-cash` menampilkan fase SO terakhir + lama putaran; `KPI DSO` dari data nyata.
6. SO draft write (hanya dipakai Stage 05+): `odoo.Client.CreateSODraft(dealer, lines, note)` dengan `note = "Dibuat Distri ARC · proposal <id> · disetujui <user>"`; **dinonaktifkan** bila `ODOO_WRITE=false` (default).
7. Uji: mapper idempoten (jalankan dua kali), fase order-to-cash untuk 5 kombinasi status, `Fake` menghasilkan metrik seed yang sama.

## Acceptance
- Dengan `ODOO_MODE=fake`: `arc ctl odoo sync --full` memuat 18 dealer + order 6 bulan; `GET /api/dealers/{sinar}/orders` menampilkan fase.
- Dengan kredensial Odoo uji (read-only): sync 1 cabang berjalan tanpa error, `signals` bertambah, tidak ada write (uji log).
- Sync ulang tidak menggandakan baris (uji integrasi).
- `make check` hijau.

Commit: `feat(stage-04): odoo sync read-only, order-to-cash, so draft (off)`
