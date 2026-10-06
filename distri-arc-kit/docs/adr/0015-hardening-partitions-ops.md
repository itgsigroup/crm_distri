# ADR 0015 — Hardening: partisi bulanan, retensi, 2FA, observabilitas, backup, deploy
**Status**: diterima · 2026-10-06 · melengkapi ADR 0005, 0007, 0013

## Keputusan
1. **Partisi bulanan** `signals` (occurred_at) dan `chat_messages` (sent_at), migrasi 0011 memindahkan data lama.
   Unique constraint pada tabel berpartisi wajib memuat kunci partisi, jadi idempotensi global dipindah ke tabel kunci:
   `signal_keys(dedupe_key → signal_id, occurred_at)` dan `chat_message_keys(wa_msg_id → message_id, sent_at)`. Upsert
   memakai CTE: sisip kunci dulu (`on conflict`), sisip baris bila kunci baru, perbarui bila lama. PK menjadi
   `(id, occurred_at|sent_at)` + indeks `id`. FK `chat_messages.signal_id → signals` dilepas (sinyal disimpan lebih
   lama dari chat). Fungsi SQL `arc_attach_month` (memindahkan baris dari partisi default), `arc_ensure_partitions`,
   `arc_drop_partitions_before`; job `partitions.ensure` harian (3 bulan ke depan) dan `retention.purge` 02.30 WIB.
2. **Retensi** dari kebijakan `retention`: chat 90 hari, sinyal 24 bulan, `llm_calls` 180 hari, identifikasi nomor
   yang tidak menjadi kontak 90 hari. Bulan utuh di-`drop`, sisanya di-`delete`. Dealer, transaksi, metrik dan snapshot
   tidak pernah disentuh. **PDP**: `arc ctl pdp export --dealer` (JSON lengkap) dan `pdp delete --contact --yes`
   (kontak, thread, pesan termasuk di grup, sinyal percakapan, identifikasi; memo ditulis ulang di siklus berikutnya;
   audit menyimpan SHA-256 nomor saja).
3. **2FA TOTP** (RFC 6238, SHA-1/6 digit/30 dtk, stdlib) untuk CEO/admin: kunci disegel AES-256-GCM dengan kunci
   turunan `SESSION_SECRET`; aktif setelah kode pertama; satu langkah waktu hanya sekali (`totp_last_step`); toleransi
   ±1 langkah; kode salah dihitung rate limit login. Reset darurat: `arc ctl user totp-reset`. Tanpa QR (tidak ada
   dependensi): kunci setup + tautan `otpauth://`.
4. **Observabilitas** tanpa dependensi baru: `/api/health` publik (status tiap bagian) dan rinci untuk CEO/admin
   (nomor WA, model Odoo, LLM hari ini, siklus, outbox, alert); `/metrics` teks Prometheus (gauge + histogram durasi per
   pola rute) dengan `METRICS_TOKEN` atau hanya localhost, tidak diproksikan Caddy. **Alert** (`alerts.check` 5 menit):
   WA terputus, siklus gagal 2×, antrean > 500 → tabel `system_alerts` (sekali per insiden, "Pulih" saat selesai) →
   baris outbox `wa_system` tanpa proposal. Sender menolak target selain grup WhatsApp `internal` (constraint
   `outbox_system_only` + cek di kode): alert tidak pernah ke dealer.
5. **Backup**: `infra/backup.sh` = `pg_dump -Fc` → age (`BACKUP_AGE_RECIPIENT`) atau OpenSSL AES-256-CBC PBKDF2
   200k (`BACKUP_PASSPHRASE`) + sha256, rotasi 30 hari, salinan rclone opsional; tanpa kunci → menolak. `restore.sh`
   memverifikasi checksum lalu `pg_restore --clean`. `infra/restore-test.sh` membuktikan metrik identik (fingerprint
   `arc ctl fingerprint`, tanpa `as_of`) dan jumlah baris sama — lokal (`make restore-test`) dan CI mingguan.
6. **Deploy**: satu image `arc` (api/worker/ctl, Debian slim + pg_dump 16) dan image `web` (Caddy + `web/dist`);
   `docker-compose.prod.yml` (postgres, migrate sekali jalan, api, worker, backup, caddy; hanya 80/443 publik);
   alternatif systemd. Caddy: HSTS, CSP `script-src 'self'` (font `data:` karena Vite meng-inline font kecil), nosniff,
   frame DENY, SSE tanpa buffer. `arc ctl check-env` gagal untuk kesalahan produksi (secret lemah, ARC_NOW, akun demo,
   jendela kirim mati, adapter tanpa kredensial, backup tanpa enkripsi). `seed --policies-only` untuk produksi.
7. **Performa**: `tools/loadtest` (50k sinyal, 20k chat) + `docs/PERF.md`. Perbaikan dari EXPLAIN: join timeline via
   uuid (bukan `id::text`) dan indeks parsial `signals_timeline`. Semua p95 ≤ 122 ms (16 klien), target 300 ms.

## Konsekuensi
- Query baru ke `signals`/`chat_messages` sebaiknya memuat rentang waktu agar partisi dipangkas.
- Insert langsung ke `signals`/`chat_messages` tanpa tabel kunci melewati dedupe — selalu lewat query sqlc.
- Uji deploy 24 jam di VPS belum bisa dilakukan dari sini (OPEN-QUESTIONS).
