# Stage 12 — Eksekusi nyata: SO draft ke Odoo, catatan Odoo, outbox WA produksi, komitmen dua arah
**Baca**: `04-orchestrator.md` (Eksekusi), `02` (aliran 4), `09` (WA limits), mockup Komitmen & Timeline.

## Tujuan
Proposal yang disetujui benar-benar dieksekusi: SO draft di Odoo, catatan di partner Odoo, pesan WA ke dealer lewat nomor sales; semua tercatat sebagai sinyal dan komitmen dua arah.

## Deliverables
1. `ODOO_WRITE=true`: `so_draft` approved → `odoo.CreateSODraft` → `orders` dengan `created_by='ai_order_draft'` + sinyal; gagal → outbox `error` + proposal tetap `approved` dengan toast/notifikasi.
2. `odoo_note`: setiap proposal approved/rejected menulis catatan ringkas di `res.partner` (opsional, policy `odoo.write_notes`).
3. Outbox WA produksi: batas harian per sales, jeda acak, jam kirim 08–18 WIB, retry 3×, status di Timeline dealer ("terkirim 09.12 oleh Andi · dari proposal #…"); pesan masuk balasan dihubungkan ke proposal (reply tracking) → komitmen "Mereka".
4. Komitmen dua arah (`commitments`): dari proposal (Kami: "Kirim 10 kamera Senin") dan dari ekstraksi WA (Mereka: "Bayar INV/0889 minggu depan") dengan status terbuka/selesai/terlambat; AI Penagihan & AI Follow-up membaca komitmen.
5. Timeline dealer lengkap: sinyal + proposal + keputusan + kirim + balasan, dengan ikon sumber.
6. Uji: outbox state machine; reply tracking; komitmen terlambat memicu proposal.

## Acceptance
- Setujui `so_draft` Toko Sinar → SO draft muncul di Odoo uji dengan catatan sumber; Timeline menampilkan rantai lengkap.
- Dengan WA nyata (nomor uji Sam → nomor uji kedua): pesan terkirim sesuai jam & jeda; balasan muncul di Timeline terhubung ke proposal.
- `make check` hijau.

Commit: `feat(stage-12): eksekusi nyata odoo so draft, outbox wa produksi, komitmen`
