# ADR 0044 — Kirim kartu kontak WhatsApp; nama panggilan disimpan ke kontak WhatsApp HP

Status: diterima · 2026-10-17 · Sam

## Konteks
Sam ingin bisa mengirim kontak dari Chat, dan nomor yang diberi nama di GSI Orbit ikut tersimpan di kontak WhatsApp HP yang memegang chat itu.

## Keputusan
- **Kirim kontak**: konten baru `kind: contact` (`{name, phone}`) di jalur kirim yang sama dengan file dan polling. Jadi tetap lewat outbox (keputusan manusia yang tercatat) dan penjaga anti-blokir. Bridge membangun vCard 3.0 dengan `waid`. Bubble menampilkan "[kontak] Nama · +62…" sebagai kartu dengan tombol Chat (wa.me). Kartu kontak yang masuk kini ikut menampilkan nomornya.
- **Sinkron kontak**: saat "Nama panggilan" disimpan (`PUT /chat/threads/{id}/alias`), API memanggil bridge `POST /sessions/{id}/contacts`. Bridge memanggil `addOrEditContact` Baileys (aksi app-state "contact", sama seperti WhatsApp menyimpan kontak di aplikasinya) dengan `saveOnPrimaryAddressbook: true`, sehingga kontak tersinkron ke HP dan buku kontaknya.
- Sinkron ini best effort: kalau HP offline atau transport tidak mendukung, nama tetap tersimpan di GSI Orbit dan pengguna diberi tahu. Menghapus nama panggilan **tidak** menghapus kontak di HP, karena menghapus kontak orang tidak bisa dibatalkan.

## Konsekuensi
- Sinkron kontak memakai fitur WhatsApp yang tidak resmi untuk linked device (Baileys). Perilakunya bisa berubah bila WhatsApp mengubah app-state, dan sebagian HP/versi mungkin hanya menyimpan di WhatsApp, bukan di buku kontak telepon.
- Cloud API (nomor resmi) tidak mendukung keduanya lewat jalur ini; transport lain menolak dengan pesan yang jelas.
