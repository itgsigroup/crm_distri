# Stage 03 — WhatsApp ingest (whatsmeow + Cloud API), nomor internal, grup, layar Chat
**Baca**: ADR 0002, `02-architecture.md` (Ingest), `03-data-model.md` (WhatsApp & chat), `09-policies-security.md` (privasi), `07-api.md` (Chat), mockup layar Chat.

## Tujuan
Pesan WA dari nomor sales masuk sebagai `signals` + `chat_messages` secara idempoten, dengan filter privasi, dan tampil di layar Chat 3 panel. Balasan manusia dari UI terkirim lewat outbox.

## Deliverables
1. `internal/wa`: `Transport` interface (`Start`, `Pair` (QR), `Status`, `Send`, `Events() <-chan Event`), implementasi `whatsmeow` (sesi terenkripsi di tabel `wa_sessions` via store whatsmeow + `WA_SESSION_KEY`), `cloudapi` (webhook `/api/wa/cloud/webhook` + Graph send), `fake` (in-memory, bisa `Inject`).
2. Parser: event → `chat_threads` (dealer: cocokkan nomor ke `contacts.wa_number`; grup: `wa_groups`; tak dikenal: `kind='new'`) → `chat_messages` (dedupe `wa_msg_id`) → `signals(kind='wa'|'wa_group')`. **Filter privasi**: DM antara dua `internal_numbers` dibuang sebelum DB; grup `internal` hanya disimpan bila `read_enabled`.
3. Job `wa.ingest` (worker, streaming) + `wa.backfill` (riwayat 30–180 hari saat pairing, whatsmeow) dengan `policy.retention.chat_days`.
4. API Chat (semua endpoint di 07), `POST /chat/threads/{id}/messages` → proposal `kind='reply'` auto-approved oleh pengirim (tercatat) → `outbox` → job `outbox.send` → `wa.Transport.Send` dengan batas harian & jeda acak.
5. `internal_numbers` + `wa_groups` CRUD; UI di Pengaturan (bagian minimal) dan di Chat (tandai nomor internal dari thread).
6. SSE `chat_message`, `wa_status`.
7. Frontend `features/chat/*` persis mockup: tab Semua/Dealer/Grup internal/Nomor baru, thread, konteks dealer (skor ring, sisa limit, langkah berikutnya placeholder, produk favorit), kotak balas; `Pengaturan → Sumber sinyal → WhatsApp` dengan status per nomor dan tombol pairing (QR dari `/api/wa/pair`).
8. Uji: parser dedupe; filter DM internal; grup internal tidak memengaruhi `signals` dealer; `outbox.send` menghormati batas harian; integrasi dengan `wa.Fake`.

## Acceptance
- Dengan `WA_TRANSPORT=fake`, `arc ctl wa inject --from <no dealer> --text "order 10 kamera"` → muncul di Chat ≤ 2 dtk (SSE), `signals` +1, dealer `last_interaction` berubah.
- Pesan dari nomor internal ke nomor internal **tidak** tersimpan (uji).
- Balas dari UI → `outbox` `sent` (fake) dan tercatat sebagai keputusan manusia di `proposals` + `audit_log`.
- Dengan `WA_TRANSPORT=whatsmeow` dan nomor uji milik Sam: pairing QR berhasil, pesan masuk nyata tampil (dicatat di OPEN-QUESTIONS bila belum ada nomor).
- `make check` hijau.

Commit: `feat(stage-03): whatsapp ingest (whatsmeow/cloud/fake), privasi, layar chat`
