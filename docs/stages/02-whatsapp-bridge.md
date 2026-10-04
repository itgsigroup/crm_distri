# Stage 02 — WhatsApp bridge & capture (QR + Cloud API adapter, grup, nomor internal)

## Tujuan
Pesan WhatsApp nyata masuk ke ARC: tautkan nomor lewat QR, terima 1:1 dan grup, klasifikasi grup eksternal/internal, registry nomor internal, riwayat awal, dan adapter Cloud API — semua dalam satu `WaEvent` standar. Ini fondasi untuk uji end-to-end Sam.

## Baca dulu
ADR 0002, `03-integrations.md` (WhatsApp, Talenta), `04-policies-privacy.md`, `02-domain-model.md` (Interaction, ChatThread/Group, InternalNumber).

## Kerjakan
### wa-bridge (Node 20 + TS + `@whiskeysockets/baileys`, versi dipin)
1. Multi-sesi: `POST /sessions {label}` → QR (PNG base64 + string) di `GET /sessions/{id}/qr`; status (`pairing|connected|disconnected`); auth state di `data/wa-sessions/<id>/` terenkripsi (`BRIDGE_SECRET`). Reconnect otomatis dengan backoff; event `session.status` ke API.
2. Inbound: pesan 1:1 & grup → `WaEvent` (wamid, from, to, chat_id, is_group, sender_name, text, media_meta, timestamp, quoted, transport=`bridge`) → `POST {API_URL}/webhooks/wa` dengan HMAC-SHA256 (`BRIDGE_SECRET`), retry dengan antrean lokal (file) bila API mati.
3. Riwayat awal: setelah pairing, tarik riwayat N hari (`HISTORY_DAYS` dari API per sesi) dan kirim sebagai batch `WaEvent` (`is_history=true`).
4. Grup: `GET /sessions/{id}/groups` (id, nama, anggota dengan nomor & nama). Profil kontak: `GET /sessions/{id}/contacts/{jid}` (nama, about, foto ada/tidak).
5. Outbound: `POST /sessions/{id}/send {chat_id, text, action_id}` — hanya diterima bila API mengonfirmasi `action_id` berstatus `approved` (bridge memanggil balik `GET /actions/{id}` dengan token). Rate-limit ≤ 20/jam/sesi, jeda acak 2–6 detik. Log semua kirim.
6. Health & metrik: `/health` (sesi, antrean, last event), log JSON.
### API (Python)
7. `WhatsAppTransport` interface + `BridgeTransport` (klien HTTP ke bridge) + `CloudAPITransport` (webhook `/webhooks/wa-cloud` dengan verifikasi `X-Hub-Signature-256`, parse `messages/statuses`, kirim via Graph API, ambil profil bisnis) + `FakeTransport` (replay fixture).
8. `POST /webhooks/wa`: verifikasi HMAC, idempoten `wamid`, simpan `Interaction` (channel `wa_message|wa_group_message`, direction, participants), buat/perbarui `ChatThread` (per nomor sesi × chat) dan `ChatGroup` (type dari anggota: ada non-internal → `external`, else `internal`; `read_policy` opt-in default: external=off sampai diaktifkan, internal=off). Pesan grup yang belum opt-in **tidak disimpan** (hanya counter). Chat 1:1 antara dua nomor internal: tidak disimpan isinya.
9. Registry nomor internal: CRUD, impor CSV Talenta, `arc_suggested` (muncul di ≥ 3 grup dan tidak ada di Person eksternal), konfirmasi/tolak.
10. Identity ringan (deterministik): nomor → Person yang sudah ada (ARC) → tautkan ke akun; belum ada → `InboundContact(status=unknown)` (analisis di tahap 03).
11. Parser ekspor chat (.txt/.zip WhatsApp, format ID/EN) → `WaEvent(is_history=true)`; endpoint unggah.
12. Endpoint: `GET /chat/threads?type=`, `GET /chat/threads/{id}/messages`, `POST /chat/threads/{id}/reply` → membuat Action `send_wa` (status `proposed`; eksekusi setelah approve di tahap 05/07 — sementara sediakan `POST /actions/{id}/approve` sederhana untuk pengujian, role human, audit).
13. Dokumen uji manual `docs/testing/whatsapp.md`: pasang nomor cadangan, scan QR, kirim pesan dari HP lain, lihat di `GET /chat/threads`.

## Acceptance criteria
- Fixture replay (FakeTransport, 24 event: 10 pelanggan 1:1, 8 grup eksternal, 4 grup internal, 2 internal 1:1) → Interaction & thread benar; grup terklasifikasi benar; 0 isi chat internal 1:1 tersimpan; pesan grup non-opt-in hanya menambah counter; replay ulang → 0 duplikat.
- Ekspor chat fixture 90 hari → historis tanpa duplikat terhadap event live.
- HMAC salah → 401. Kirim dari bridge tanpa `action_id` approved → ditolak (test bridge dengan API mock).
- Uji manual (jika ada nomor cadangan): pesan dari HP lain muncul di `GET /chat/threads` ≤ 5 detik; dicatat di laporan tahap. Tanpa nomor → `done-with-mocks`.
