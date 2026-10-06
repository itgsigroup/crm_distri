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
- 2026-10-05 · 06 · Kapan langkah otonom yang mengirim WA ke dealer (follow-up H-1, pengingat H-3) boleh terkirim tanpa klik *Jalankan sekarang*? · `autonomy.guard.dealer_messages = "confirm"` (ADR 0008) · confirm
- 2026-10-05 · 06 · Bundle stok: mockup menulis LED P5 −8% ke 6 dealer, rumus (floor margin 9% dari modal, kandidat glossary) memberi −1,5% ke 2 dealer. Floor dihitung dari modal stok (`unit_cost`) — benar, atau dari HPP lain di Odoo? · rumus dipakai · −1,5%
- 2026-10-05 · 07 · Uji nyata dari Claude Desktop Sam dan connector ChatGPT tim sales ke `https://distri.gsi.co.id/mcp` (butuh domain + TLS, Stage 13) · diuji dengan klien SDK (`tools/mcpcheck`) ke localhost · —
- 2026-10-05 · 07 · `jadwal.due {sales:"Dewi"}` memberi 2 dealer (Prima besok, Bina 3 hari); acceptance menulis 3 (mockup menghitung Mitra yang lewat jadwal di agenda Dewi). Tetap pisahkan *jadwal* dan *lewat jadwal*? · dipisah seperti glossary · 2
- 2026-10-06 · 09 · Akses API Truecaller (resmi untuk bisnis) — ada? · identifikasi memakai profil WA Business, Getcontact (CSV) dan Odoo; Truecaller fake · —
- 2026-10-06 · 09 · Stok kritis: bila cabang lain punya stok cukup, AI Stok mengusulkan transfer (HDD 4TB Surabaya → Yogyakarta), mockup menulis PO. Aturan transfer-dulu disetujui? Target 2,5 minggu & sumber ≥ 4 minggu sesuai praktik gudang? · ADR 0011 · transfer dulu
- 2026-10-06 · 09 · Prediksi kas: konstanta probabilitas dikalibrasi ke mockup (ADR 0011). Bandingkan dengan realisasi kas 2–3 bulan pilot lalu kalibrasi ulang · — · seperti ADR
- 2026-10-06 · 11 · Peran gudang/finance: menu dan hak keputusan di ADR 0013 (finance: penagihan & limit; gudang: transfer & PO) — sesuai praktik GSI? · ADR 0013 · —
- 2026-10-06 · 11 · Akun produksi: siapa saja (email) dan perannya, untuk dibuat lewat `arc ctl user add` saat pilot · akun contoh di seed · —

