# ADR 0055 — Analisa Dealer oleh Claude (routine) dengan status dan draf WhatsApp

Status: diterima · 2026-10-19 · Sam

## Konteks
Sam: "pada dealer apakah sudah sesuai MD? dealer diperbarui sesuai routine Claude dan analisa AI ini juga dari Claude;
tambahkan button untuk chat ke WhatsApp sesuai analisa AI-nya apakah bermasalah atau tidak". Halaman Dealer sudah
memuat komponen MD (DealerList, Header, Langkah berikutnya, Order-to-cash, Share of wallet & mix, Sisa limit, Memori,
PIC aktif, Komitmen, Timeline). Tetapi tombol Analisa AI memakai mesin "auto" (tidak selalu Claude/routine), Memori
dealer berlabel "brief otomatis" padahal dirangkum template, dan tombol WhatsApp memuat semua chat.

## Keputusan
- Bacaan **Analisa Dealer** (agent `Analisa Dealer`) hanya untuk scope `dealer:<id>` (`ScreenReadingsIn`). Input:
  angka dealer (status orbit, cyc, siklus, jadwal, segmen, omzet, porsi belanja, PIC, skor, kredit, invoice terbuka
  dengan telat, lewat tempo, order terakhir) + 15 pesan WhatsApp terakhir (30 hari). Claude mengisi `lines` berkunci
  tetap: `status` (bermasalah | perlu_perhatian | aman), `alasan_1..3`, `wa` (draf ≤ 300 karakter) + `overview`.
  Divalidasi (`checkDealer`), disimpan di tabel baru `dealer_analyses` (migrasi 0056), terbaru per dealer.
- Tombol **Analisa AI** lewat MCP Claude (routine dipicu server, ADR 0049); progres sebagai pop-up (bukan kartu).
- Kartu **Analisa AI** di bawah header: status berwarna, ringkasan, alasan, draf WhatsApp, dan tombol **Chat: …** sesuai
  status (merah: selesaikan masalah · kuning: sapa & tawarkan · hijau: sapa dealer). Tombol membuka chat dealer dengan
  `?draft=`; Chat mengisi kolom ketik sekali — sales yang membaca, mengubah, dan menekan Kirim (tidak ada kirim otomatis).
- `GET /api/dealers/{id}/analysis` dan `GET /api/dealers/{id}/threads` (chat dealer yang boleh dibuka orang itu; tanpa
  akses Chat: kosong) — tombol WhatsApp tidak lagi memuat semua chat.
- Memori dealer diberi label jujur "dirangkum sistem dari sinyal".
