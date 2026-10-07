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
- 2026-10-06 · 12 · Instance Odoo **uji** (URL, DB, user dengan hak `sale.order.create` + catatan `mail.message`) untuk membuktikan SO draft & catatan sebelum produksi · tulis Odoo diuji dengan fake · `ODOO_WRITE=false` di produksi
- 2026-10-06 · 12 · Uji WA nyata: nomor uji Sam → nomor uji kedua (pairing QR whatsmeow) untuk jam kirim, jeda, dan balasan · diuji dengan `wa.Fake` · —
- 2026-10-06 · 12 · Transfer internal & PO: picking type / operation type per cabang di Odoo GSI, dan apakah PO dibuat sebagai RFQ draft · baris outbox berstatus `manual` (dikerjakan gudang di Odoo) · manual
- 2026-10-06 · 12 · Catatan keputusan di partner Odoo (chatter) untuk setiap keputusan dealer — diinginkan, atau terlalu ramai? · kebijakan `odoo.write.notes` · aktif
- 2026-10-06 · 13 · VPS uji (Ubuntu 24.04, 2 vCPU/4 GB) + subdomain (mis. `distri.gsi.co.id`) untuk membuktikan deploy: HTTPS, login 2FA, siklus per jam 24 jam tanpa error · semua dibuktikan lokal; `docs/DEPLOY.md` siap · —
- 2026-10-06 · 13 · Lokasi salinan backup di luar server (Backblaze B2 / Google Drive GSI / NAS kantor via rclone) dan siapa yang memegang `BACKUP_PASSPHRASE` · backup hanya di disk server · —
- 2026-10-06 · 13 · Grup WhatsApp internal mana yang menerima alert sistem (IT / Sam / admin)? · grup internal pertama (Gudang Semarang) · `ALERT_WA_GROUP`
- 2026-10-06 · 13 · 2FA wajib (bukan opsional) untuk CEO/admin saat pilot? · opsional, disarankan di layar · opsional
- 2026-10-06 · 13 · Validasi `infra/Caddyfile` dengan biner caddy belum jalan (unduhan modul timeout di jaringan ini); CSP sudah diuji pada build produksi · dijalankan saat deploy (`caddy validate`) · —
- 2026-10-06 · 14 · Tanggal mulai pilot Semarang dan dua nomor sales yang dipasangkan (Andi + ?) · perangkat pilot siap, menunggu deploy produksi (pertanyaan Stage 13) · —
- 2026-10-06 · 14 · Daftar nomor internal karyawan (CSV `wa_number,label,department,is_sales`) untuk `arc ctl wa import-internal` · hanya 4 nomor sales di seed · —
- 2026-10-06 · 14 · Target pilot di `docs/PILOT-REPORT.md` (saran diterima ≥ 70%, median keputusan < 4 jam kerja, lewat jadwal tertangkap ≥ 50%) — setuju, atau Sam punya angka lain? · dipakai sebagai usulan · —
- 2026-10-06 · 14 · Saran tanpa dealer (transfer stok, PO) dihitung untuk semua cabang di dashboard pilot; perlu dipisah per gudang cabang? · dihitung semua · —
- 2026-10-06 · 14 · Gladi: dengan 2 sales, AI Order / Kredit / Prospek bisa tidak mencapai 5 keputusan per minggu sehingga tidak pernah memenuhi syarat buka otonomi dalam 2 minggu. Turunkan `pilot.min_decisions_per_week` (mis. 3), hitung per 2 minggu, atau biarkan agen itu tetap approve? · tetap 5 · 5
- 2026-10-06 · 13 · ~~Deploy key server belum diterima GitHub~~ — terjawab 2026-10-06: deploy key read-only aktif, server menarik `distri-arc-orbit` lewat `infra/deploy.sh` · — · —
- 2026-10-06 · 13 · `https://crm-distri.gsiindo.id` berjalan sebagai staging (data contoh, adapter fake). Kapan diganti data nyata: hapus database `distri_arc`, `arc ctl migrate`, `seed --policies-only`, isi ODOO_*/WA_*/ANTHROPIC_* di /etc/distri-arc/env · staging · —
- 2026-10-06 · WA · Nomor WhatsApp mana yang dipasangkan dulu di server (nomor kerja yang sudah lama aktif, bukan nomor baru) dan labelnya · staging berisi 4 nomor sales contoh (unpaired) · —
- 2026-10-06 · WA · Penjaga "hanya membalas kontak yang pernah menghubungi" membuat follow-up ke dealer yang belum pernah chat ke nomor itu ditolak. Tetap aktif (disarankan) atau dilonggarkan per nomor? · aktif · `BRIDGE_REQUIRE_PRIOR_INBOUND=on`
- 2026-10-06 · data · Dataset & tabel BigQuery (project, lokasi, nama tabel/kolom) untuk 5 query impor, dan service account baca-saja · impor CSV tersedia; contoh query di docs/DATA-IMPORT.md · —
- 2026-10-06 · data · Di sumber, kolom apa yang membedakan Dealer (reseller) dan Freelance/SI? · default reseller, ubah per pelanggan di Master pelanggan atau lewat mapping Jenis pelanggan · reseller
- 2026-10-06 · data · Tier dan limit kredit per pelanggan belum ada di Accurate — diisi di Master pelanggan, atau ada sumber lain? · tanpa limit = cash · —

## Peran & akses (2026-10-07)
- Pengguna yang sudah ada di server memakai peran bawaan base-nya. Bila perlu peran khusus (mis. "Sales Telemarketing", "CS Kantor"), CEO membuatnya di Pengaturan → Peran & akses lalu menetapkannya ke pengguna.
- Nomor WhatsApp tiap pengguna diisi di Pengaturan → Pengguna sebelum ditautkan di Chat.
