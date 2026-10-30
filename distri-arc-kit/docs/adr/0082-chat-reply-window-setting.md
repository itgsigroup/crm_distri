# ADR 0082 — Pengaturan: aturan 24 jam Chat bisa dibuka

Status: diterima · 2026-10-30 · Sam ("pada halaman pengaturan chat itu di setting, jadi saya bisa atur chat nya di buka atau tetep 24 jam").

## Konteks
ADR 0081 mengunci Chat pada aturan 24 jam WhatsApp. CEO ingin bisa memilih sendiri kapan aturan itu dipakai.

## Keputusan
- Kebijakan `chat.rules` mendapat field opsional `reply_window_open` (boolean, bawaan `false`). Tanpa field ini aturan
  24 jam tetap berlaku, jadi database lama tidak berubah perilaku.
- `true` = **Dibuka**: `replyClosed` selalu terbuka, pelanggan bisa dikirimi dari Chat kapan saja.
- Pengaturan → kartu **Chat WhatsApp** → "Batas balas 24 jam": pilihan **24 jam** / **Dibuka**. Hanya CEO
  (`edit_policies`), lewat `PUT /policies/chat.rules` yang tervalidasi skema dan berversi seperti kebijakan lain.

## Akibat
- Saat dibuka, risiko nomor diblokir WhatsApp kembali ke pengguna; kartu menampilkan "Chat dibuka" sebagai peringatan.
- Grup dan chat internal tetap tidak terikat, dibuka atau tidak.
