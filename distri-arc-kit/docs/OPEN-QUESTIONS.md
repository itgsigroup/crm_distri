# Pertanyaan terbuka
Diisi Claude Code saat ada hal yang butuh keputusan/kredensial dari Sam. Format: tanggal · tahap · pertanyaan · dampak bila belum dijawab · nilai sementara yang dipakai.

- 2026-10-05 · 03 · Nomor WA mana yang dipasangkan dulu untuk pilot (1 sales Semarang?) · WA ingest memakai `wa.Fake` sampai ada · —
- 2026-10-05 · 04 · URL + API key Odoo (read-only user) dan mapping `product_category_id` → 6 KAT · Odoo sync memakai `odoo.Fake` dari seed · —
- 2026-10-05 · 05 · `ANTHROPIC_API_KEY` (dan `OPENAI_API_KEY` cadangan) · agen memakai `llm.Fake` · —
- 2026-10-05 · 00 · Kode ARC v1 (apps/, packages/, docs/ lama) masih di root repo `crm_distri`; penghapusan otomatis ditolak pengaman. Hapus, pindahkan ke `legacy/`, atau biarkan? · Distri ARC dibangun di `distri-arc-kit/` (branch `distri-arc-orbit`) · —
- 2026-10-05 · 00 · Data contoh mockup punya angka yang tidak konsisten (KPI 85%/DSO 36/limit tipis "3") · angka dihitung dari rumus glossary; teks seed disesuaikan · —
- 2026-10-05 · 01 · DSO dihitung untuk penjualan kredit saja (penjualan tunai tidak membentuk piutang): seed → 30 hari, mockup menulis 36 · Setuju definisi ini? · dipakai kredit-saja · —
- 2026-10-05 · 03 · Enkripsi kunci sesi whatsmeow dengan `WA_SESSION_KEY` tidak didukung store resmi; tetap simpan di skema `whatsmeow` + backup terenkripsi, atau tulis store terenkripsi sendiri? · sesi tidak terenkripsi di DB · skema terpisah
- 2026-10-05 · 04 · Field Odoo GSI untuk: limit kredit dealer (`credit_limit` bawaan atau field kustom), tier (nama pricelist "Tier A/B/C"?), jenis usaha dealer (`industry_id`?), velocity stok, dan cabang = company? · mapper memakai asumsi tsb · fake
