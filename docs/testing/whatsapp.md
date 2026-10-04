# Uji manual WhatsApp (nomor cadangan)

Tujuan: membuktikan alur nyata WA masuk → Chat ≤ 5 detik → anotasi ARC ≤ 90 detik → relasi & peta, dan bahwa kirim hanya terjadi setelah approve.

> Pakai **nomor cadangan**, bukan nomor utama sales. wa-bridge memakai protokol linked device yang tidak resmi; penggunaan wajar (≤ 20 kirim/jam, jeda 2–6 detik) membatasi risiko blokir.

## Persiapan
1. `cp .env.example .env`, isi `DATABASE_URL`, `BRIDGE_SECRET` (sama untuk API & bridge), kosongkan `ARC_CLOCK_ANCHOR` bila ingin jam asli.
2. `make db-upgrade && make seed` (atau tanpa seed untuk database bersih).
3. `make dev` → API :8000, bridge :3001, web :5173. Cek `curl localhost:3001/health` → `{"ok":true,"sessions":{},...}`.

## Langkah
1. Login web sebagai CEO (`sam@gsi.co.id`) → **Pengaturan → Nomor WhatsApp** → pilih sesi (mis. "Rizky · Surabaya" berstatus *pairing*) → **Tautkan**. QR tampil (berlaku 60 detik, diperbarui otomatis).
2. Di HP nomor cadangan: WhatsApp → Perangkat tertaut → Tautkan perangkat → scan. Status berubah **Terhubung**; `curl localhost:3001/health` menunjukkan sesi `connected`.
3. Riwayat: bridge meneruskan riwayat sesuai `history_days` sesi sebagai `is_history=true` (tidak memicu notifikasi).
4. Dari HP lain, kirim pesan ke nomor cadangan, mis. *"Pak, minta penawaran 16 kamera untuk gudang Sidoarjo, PO bisa Kamis"*.
   - **≤ 5 detik**: pesan muncul di **Chat** (thread pelanggan) dan di `GET /api/chat/threads`.
   - **≤ 90 detik** (debounce 60 dtk + ekstraksi): anotasi ARC (komitmen "PO Kamis", kebutuhan 16 kamera) muncul; nomor baru masuk **Prospek** bila belum dikenal.
   - **Relasi**: Person/akun dan komitmen terlihat; **Peta 3D**: node baru.
5. Balas dari Chat → tombol balasan membuat Action `send_wa` (status *proposed*). Belum ada yang terkirim.
6. Buka Action → **Approve**. API memanggil bridge `POST /sessions/{id}/send`; bridge memverifikasi ulang ke `GET /bridge/actions/{id}` (HMAC) bahwa action *approved*, menunggu 2–6 detik, lalu mengirim. Pesan tiba di HP lain; Interaction outbound tercatat dengan `wamid`.
7. Uji penolakan: `curl -X POST localhost:3001/sessions/<id>/send` dengan action yang belum di-approve → **403** "kirim ditolak".
8. Privasi: kirim pesan dari nomor internal ke nomor internal lain (1:1) → isi **tidak** disimpan; pesan di grup yang belum opt-in → hanya counter.

## Catat hasil
| Langkah | Target | Hasil | Waktu |
|---|---|---|---|
| Pesan muncul di Chat | ≤ 5 dtk | | |
| Anotasi ARC | ≤ 90 dtk | | |
| Kirim setelah approve | terkirim, 1× | | |
| Kirim tanpa approve | 403 | | |

## Pemulihan
- Sesi putus > 10 menit → email ke CEO (job `bridge_watch`). Tautkan ulang dari Pengaturan.
- API mati → bridge menyimpan event di `BRIDGE_DATA_DIR/queue.jsonl` dan mengirim ulang tiap 15 detik (idempoten per `wamid`).
- Lepas tautan: Pengaturan atau `DELETE /sessions/{id}` (HMAC) → logout perangkat.
- Migrasi ke Cloud API (nomor bisnis resmi): isi `WA_CLOUD_*`, daftarkan webhook `https://<domain>/webhooks/wa-cloud` (verify token `WA_CLOUD_VERIFY_TOKEN`), ubah transport sesi ke `cloud`. Grup tidak didukung Cloud API.
