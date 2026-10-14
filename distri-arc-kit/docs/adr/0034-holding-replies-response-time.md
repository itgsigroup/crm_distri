# ADR 0034 — Waktu respon dihitung sampai pelanggan mendapat jawaban

Status: diterima · 2026-10-14 (Sam) · mengubah cara ADR 0026/0027 menghitung "dibalas"; pengecualian sempit dari ADR 0032

## Konteks
Permintaan: di semua chat, waktu respon jangan berhenti pada "tunggu sebentar" / "saya konfirmasi dulu" — hitung
sampai pelanggan benar-benar mendapat jawaban. Sebelumnya pesan apa pun dari pihak kita menghentikan jam: chat
hilang dari "Belum dibalas", timer menunggu berhenti, dan waktu balas sales terlihat cepat padahal belum menjawab.

## Keputusan
- Satu definisi di database: fungsi `is_holding_reply(body)` dan kolom terhitung `chat_messages.holding` (migrasi
  0033). Jawaban sementara = pesan pendek (≤ 160 karakter), **tanpa angka** (harga, jumlah, tanggal = isi jawaban),
  bukan media, yang hanya menjanjikan jawaban: tunggu sebentar / mohon ditunggu, sebentar ya, cek / konfirmasi /
  tanyakan / hitung dulu, tanyakan ke gudang / admin / atasan, nanti / segera saya kabari / kirim, sedang / lagi /
  masih dicek / diproses, izin cek, on process.
- Semua hitungan memakai definisi itu: "Belum dibalas" dan timer menunggu di Chat, Analisa WhatsApp (menunggu, perlu
  dibalas, waktu balas per sales, Pusat kendali), status per bubble (state baru `holding`: "jawaban sementara — belum
  menjawab"), dan bacaan Claude lewat MCP (`holding: true` / `jawaban_sementara`).
- Ini angka yang diukur dengan aturan tetap, bukan analisa AI; ADR 0032 tetap berlaku untuk analisa.

## Konsekuensi
- Kalimat penunda yang tidak ada di daftar tetap dihitung sebagai jawaban; daftar diperluas lewat migrasi baru
  (kolom terhitung dibuat ulang).
- Jawaban sementara yang memuat angka ("tunggu 5 menit ya") dihitung sebagai jawaban — sengaja, agar jawaban berisi
  harga/jumlah tidak pernah salah dianggap penunda.
