# ADR 0014 — Eksekusi nyata: tulis Odoo lewat outbox, jendela kirim WA, komitmen dua arah
**Status**: diterima · 2026-10-06 · melengkapi ADR 0007 dan 0008

## Keputusan
1. **Satu outbox untuk semua eksekusi.** Keputusan manusia (atau langkah otonom yang lolos penjaga) menulis baris `outbox`
   di transaksi yang sama dengan keputusan, lalu job `outbox.send` (river, maks 3 percobaan) menjalankannya. Kanal:
   `wa` (WhatsApp), `odoo_so_draft`, `odoo_note`. Status: `pending → sent | failed | manual`. Gagal → proposal tetap
   `approved`, NOTIFY `outbox_failed` (toast), worker mencoba ulang; berhasil → proposal `executed` + audit.
2. **Yang ditulis ke Odoo hanya dua hal**, keduanya dengan catatan sumber (proposal id + penyetuju):
   SO **draft** (`sale.order.create`, tidak pernah dikonfirmasi) dan **catatan internal** pada partner
   (`message_post`, subtype `mail.mt_note`) untuk keputusan dealer. Nomor baru yang disetujui menjadi partner baru +
   catatan. Transfer internal dan PO berstatus `manual` (picking type per cabang belum diketahui — OPEN-QUESTIONS).
   `ODOO_WRITE=false` mematikan semuanya; kebijakan `odoo.write.notes` mematikan catatan saja.
3. **SO draft tercatat di Distri ARC** sebagai order `created_by = 'ai_order_draft'` dengan `source_id = sale.order:<id>`
   (sinkron Odoo berikutnya memperbaruinya, tanpa duplikat), sinyal `so` untuk Timeline, dan komitmen **Kami**
   "Kirim … <hari>". Bila sales sudah menjanjikan pengiriman yang sama di chat (komitmen Kami terbuka tanpa proposal,
   "Kirim …"), komitmen itu diadopsi — tidak dicatat dua kali.
4. **Jendela kirim WA**: pesan proaktif hanya 08.00–18.00 WIB (`WA_SEND_HOURS`, `off` untuk dev dengan jam tetap);
   di luar jendela job dijadwalkan ulang ke jam buka. Balasan manusia di thread tidak ditahan jendela. Batas harian dan
   jeda acak dari ADR 0007 tetap. Setiap kirim meninggalkan jejak di Timeline: "Terkirim HH.MM oleh {sales} · dari
   proposal …".
5. **Reply tracking**: pesan masuk dealer dalam 72 jam setelah pesan dari proposal di thread yang sama ditautkan ke
   proposal itu (`chat_messages.proposal_id`, `signals.payload.reply_to`). Kirim pertama ke kontak tanpa thread membuka
   thread baru agar balasannya bisa ditautkan.
6. **Komitmen Mereka** dibaca dengan aturan (bukan LLM) dari pesan masuk: janji bayar ("transfer hari Kamis", "tgl 20",
   "minggu depan") → "Bayar INV/… Rp X jt" dengan tanggal (atau "tanpa tanggal"); jawaban ya atas follow-up/penawaran →
   "Order sesuai rekomendasi" (+2 hari). Idempoten per pesan (`source_key = wa:<id>`). Tahap Ingest Orchestrator
   menandai komitmen lewat tanggal sebagai `late`; AI Penagihan mengubah janji bayar terlewat menjadi pengingat tegas
   yang selalu menunggu keputusan manusia.

## Konsekuensi
- Uji di Odoo produksi tidak dilakukan oleh Claude Code: butuh instance uji + user dengan hak `sale.order.create` dan
  `mail.message` (OPEN-QUESTIONS). Fake Odoo mencatat create & catatan untuk uji integrasi.
- Pola janji bayar berbahasa Indonesia sederhana; frasa di luar pola tidak menjadi komitmen (aman: tidak ada aksi
  otomatis darinya).
