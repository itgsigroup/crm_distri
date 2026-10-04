# Runbook ARC

Untuk admin IT GSI yang merawat ARC di Mini PC/VPS (Ubuntu). Stack: PostgreSQL 17, API Go (`arc`), `wa-bridge` (Node + Baileys), Caddy. Lihat ADR 0003 dan 0004.

## Layanan & port
| Layanan | Port | Health | Catatan |
|---|---|---|---|
| api | 8000 (internal) | `GET /health` → `status, db, llm, mocks, clock` | menyajikan web build + MCP + webhook |
| wa-bridge | 3001 (internal, tidak dipublikasikan) | `GET /health` → `sessions, queue, last_event, limits` (pemakaian batas per nomor) | auth state Baileys di skema `wa_bridge` |
| postgres | 5432 (internal) | `pg_isready` | volume `pgdata` |
| caddy | 80/443 | — | HTTPS otomatis untuk `ARC_DOMAIN` |

Metrik operasional (CEO): `GET /api/metrics` — status job terakhir & gagal 24 jam, panggilan/biaya LLM 24 jam, antrean Action per status, status sesi WA, health bridge.

## Operasi rutin
```bash
make compose-up                          # build + start (restart: unless-stopped)
docker compose -f infra/docker-compose.yml logs -f api wa-bridge
docker compose -f infra/docker-compose.yml exec api /app/bin/arc job sync_odoo   # jalankan job manual
docker compose -f infra/docker-compose.yml exec api /app/bin/arc brief           # pratinjau brief
```
Job terjadwal (WIB): per jam `extract, identify_inbound, hygiene, capture_gmail, capture_calendar, sync_odoo, meeting_prep`; harian 06.30 `score_opportunities, forecast, collection, renewal, tender_radar, coaching, memory`; brief 06.45 & 16.00; `bridge_watch` tiap 5 menit. Riwayat di tabel `jobs_runs`.

## Alert otomatis (email/notifier ke CEO)
- Job gagal 2× berturut-turut.
- Biaya AI harian > `ARC_DAILY_COST_ALERT_IDR`.
- Sesi WhatsApp putus > 10 menit.

## Insiden umum
| Gejala | Tindakan |
|---|---|
| Pesan WA tidak masuk | `curl wa-bridge:3001/health`: sesi `disconnected` → tautkan ulang QR di Pengaturan. `queue` > 0 → API tidak terjangkau; bridge mengirim ulang otomatis tiap 15 dtk. |
| Bridge menolak kirim (403) | Action belum *approved* — benar secara desain. Cek status Action di web. |
| Bridge 429 | Batas pakai tercapai (`hourly_limit`, `daily_limit`, `warmup_limit`, `chat_limit`, `chat_gap`); lihat header `Retry-After`. **Jangan** menaikkan batas untuk mengejar target — itu yang memicu blokir. |
| Bridge 403 `first_contact` / `broadcast` / `opted_out` | Penjaga anti-blokir: kontak belum pernah menghubungi nomor ini, teks sama ke banyak chat, atau kontak minta berhenti. Hubungi via email/telepon/Cloud API, atau personalisasi pesan. |
| Bridge 409 `quiet_hours` | Jam tenang 21.00–07.00 WIB; approve ulang besok pagi. |
| Sesi `disconnected` alasan `forbidden` | WhatsApp menolak perangkat (kemungkinan nomor dibatasi). Bridge sengaja **tidak** reconnect. Hentikan pengiriman dari nomor itu, buka WhatsApp di HP, ikuti instruksi banding bila ada; pertimbangkan Cloud API. |
| Sesi `disconnected` alasan `logged_out` / `replaced` | Perangkat dilepas dari HP atau ditautkan di tempat lain. Tautkan ulang lewat QR. |
| Sinkron Odoo gagal | Cek kredensial & hak user API. Konflik tulis → Signal `sync_conflict` (tidak menimpa data Odoo). |
| Biaya AI melonjak | `GET /api/llm/usage` per tujuan; turunkan tier di Pengaturan → AI & model (routing). |
| Login 429 | Rate limit login 10/menit per IP. |

## Mengurangi risiko blokir WhatsApp (ADR 0004)
- Pakai nomor kerja yang **sudah lama aktif** dan dipakai normal dari HP; jangan menautkan nomor baru lalu langsung dipakai banyak (bridge membatasi 15 pesan di hari pertama, naik bertahap 7 hari).
- Satu nomor = satu sesi bridge. Jangan menautkan nomor yang sama di beberapa server.
- Biarkan HP tetap dipakai seperti biasa (membalas, membuka chat); bridge tidak menandai pesan terbaca dan tidak tampil online terus-menerus.
- Kirim hanya ke kontak yang sudah menghubungi lebih dulu; kontak baru/promo massal lewat **Cloud API** dengan template resmi.
- Pantau `GET /health` → `limits` dan laporan penolakan; banyak penolakan `broadcast`/`first_contact` = kebiasaan pengiriman perlu dibenahi, bukan batasnya.
- Bila muncul `forbidden`/peringatan di HP: hentikan pengiriman dari nomor itu selama beberapa hari.

## Backup & restore
- Versi `pg_dump`/`pg_restore` harus ≥ versi server (17); set `PG_BIN=/usr/lib/postgresql/17/bin` bila perlu. Dump ditulis ke `.partial` lalu dipindah, jadi dump gagal tidak meninggalkan file kosong.
- Harian: cron `30 1 * * * cd /opt/arc && scripts/backup.sh` → `backups/arc-YYYYmmdd-HHMMSS.dump` (custom format, termasuk skema `wa_bridge`), simpan 14 terakhir. Salin offsite (rclone crypt ke Google Drive).
- Restore di mesin bersih:
  ```bash
  createdb arc
  RESTORE_DATABASE_URL=postgres://arc:...@localhost:5432/arc scripts/restore.sh backups/arc-....dump
  make compose-up
  curl https://<domain>/health
  ```
  Diuji 2026-10-04 di mesin pengembangan: dump → restore ke database kosong → jumlah baris identik (termasuk 17 tabel `wa_bridge`) → API start dan login berhasil. Uji di mesin bersih tetap wajib sebelum go-live.
  Setelah restore, sesi WA tetap tertaut (kunci ikut di dump). Bila nomor sudah ditautkan di mesin lain, lepas tautan lama dulu.

## Upgrade
1. `scripts/backup.sh`.
2. `git pull` → `make test` → `make compose-up` (migrasi SQL berjalan otomatis saat `serve`).
3. **Baileys**: versi dipin persis di `apps/wa-bridge/package.json` (saat ini `7.0.0-rc14`). Upgrade: `cd apps/wa-bridge && npm install --save-exact baileys@<versi> && npm test && npm run lint`, lalu uji QR + kirim di **nomor cadangan** (`docs/testing/whatsapp.md`) sebelum produksi. Bila sesi sering putus dengan kode 405/408 setelah update WhatsApp, cek rilis Baileys terbaru.

## Keamanan
- Rahasia hanya di `.env` (izin 600). Production menolak start bila `ARC_ENCRYPTION_KEY`/`BRIDGE_SECRET` masih default, `ARC_CLOCK_ANCHOR` terisi, atau URL bukan https.
- Disk terenkripsi (LUKS) — database berisi kunci WhatsApp dan token Google (token Google juga dienkripsi AES-GCM).
- Webhook: bridge ↔ API HMAC-SHA256 (`X-ARC-Signature`); Cloud API `X-Hub-Signature-256`.
- Audit log append-only (trigger menolak UPDATE/DELETE).
- Pemeriksaan dependensi: `govulncheck ./...` (API & bridge), `npm audit` di `apps/web`.
