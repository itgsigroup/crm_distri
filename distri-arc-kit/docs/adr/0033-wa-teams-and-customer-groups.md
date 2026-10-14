# ADR 0033 — Tim nomor WhatsApp (Sales · Admin · Penagihan) dan grup pelanggan

Status: diterima · 2026-10-14 (Sam) · melengkapi ADR 0017 (banyak nomor), ADR 0026 (Analisa WhatsApp), ADR 0032

## Konteks
Permintaan: tambahkan WhatsApp admin dan WhatsApp penagihan ke GSI Orbit, dengan pengelompokan terpisah dari sales;
Chat harus bisa membaca grup yang berisi pelanggan. Sebelumnya semua nomor tampil dalam satu baris rata di Chat, dan
setiap grup baru dicatat `internal` — Analisa WhatsApp hanya membaca chat dealer dan nomor baru, jadi grup pelanggan
tidak pernah dibaca Claude.

## Keputusan
- `wa_numbers.team` ∈ `sales | admin | billing` (Penagihan). Dipilih saat menautkan (Chat → + Nomor, oleh CEO/admin;
  orang yang menautkan nomornya sendiri mendapat tim perannya: finance → Penagihan, sales → Sales, lainnya → Admin),
  disimpan di `wa_links.team` sampai WhatsApp melaporkan nomornya. Bisa dipindah di detail nomor
  (`PUT /api/wa/numbers/{wa}/team`, CEO/admin). Migrasi 0031 mengisi nomor yang ada dari peran pemegangnya.
- Chat: rail nomor dikelompokkan per tim; judul tim menampilkan semua chat nomor tim itu (`GET /api/chat/threads?team=`).
  Hak baca tidak berubah: sales tetap hanya melihat chat nomornya sendiri.
- Grup pelanggan = `wa_groups.kind = 'external'`. Aturan otomatis: grup baru yang anggotanya (dari bridge) atau
  pengirimnya ada di kontak pelanggan menjadi grup pelanggan; grup internal berubah jadi grup pelanggan saat kontak
  pelanggan menulis di dalamnya. Orang bisa menandai sendiri di panel Chat atau Pengaturan; pilihan manusia
  (`kind_manual`) tidak diubah aturan. Konservatif demi privasi: grup tanpa kontak pelanggan yang dikenal tetap internal.
- Analisa WhatsApp membaca grup pelanggan seperti chat pelanggan: satu percakapan per grup walau beberapa nomor GSI
  ada di dalamnya (thread pertama per `wa_jid`), pesan karyawan GSI (`internal`) dihitung sebagai balasan, nama
  pengirim pelanggan dimasking sebelum ke Claude. Grup internal tetap tidak pernah dibaca.
- MCP: `chat.thread` menerima `thread_id` (termasuk grup pelanggan, menolak grup internal); `wa.analisis` menyertakan
  `jenis` dan `tim`.

## Konsekuensi
- Grup pelanggan dengan pelanggan yang belum ada di master kontak perlu ditandai manual sekali.
- Usulan penagihan (`collect`) belum otomatis dikirim dari nomor tim Penagihan — tetap lewat nomor yang memegang
  percakapan (catatan di OPEN-QUESTIONS).
