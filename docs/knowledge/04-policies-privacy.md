# 04 — Kebijakan, privasi, keamanan (baca di tahap 04, 06, 09, 11, 12, 13)

## Batas otonomi (5 level)
L0 baca & catat · L1 ekstrak & usulkan · L2 siapkan draft · L3 jalankan tindakan internal (buat activity Odoo, tugas Basecamp) setelah approve · L4 tindakan ke pelanggan setelah approve · L5 (tidak ada): tidak pernah otonom ke pelanggan, tidak pernah transaksi keuangan.
Default agen: L1–L2. Tindakan internal berisiko rendah bisa L3 otomatis bila policy mengizinkan (mis. buat activity pengingat). Kirim ke pelanggan selalu L4.

## Keputusan human-only
`approve/reject/edit` Action, override stage/probabilitas, pengecualian diskon > policy, pelepasan barang kredit di atas limit, mengirim pesan/email, perubahan policy (CEO), penghapusan data.

## Privasi (UU PDP & kepatutan)
- Baca hanya kanal yang disetujui pengguna pemilik kanal (consent tercatat).
- **Nomor internal** dikenali (registry). Chat pribadi antar karyawan **tidak dibaca**. Grup internal dibaca untuk jadwal/tugas saja dan tidak memengaruhi health akun. Grup eksternal dibaca penuh.
- Sales bisa menandai kontak/chat sebagai pribadi → dilewati.
- Identifikasi nomor hanya untuk nomor **inbound**; hasil disimpan 90 hari kecuali jadi lead; tidak untuk prospek dingin.
- PII dimasking sebelum ke provider LLM eksternal; dokumen kontrak penuh, data HR, dan chat internal tidak pernah dikirim ke provider.
- Retensi: interaksi 24 bulan; log LLM 90 hari (hash input, bukan isi).
- Hak akses: pengguna hanya melihat akun cabangnya + yang di-share; CEO/manager melihat semua. Klien MCP/API mewarisi hak pengguna pemiliknya — tidak pernah lebih.

## Keamanan
API key dengan scope (`read`, `propose`, `events`, `human` — `human` hanya untuk sesi pengguna, tidak untuk mesin). OAuth 2.1 untuk MCP. Rate limit per key. Secrets di `.env`/secret manager. Audit log append-only. Backup terenkripsi harian. Webhook WA diverifikasi signature.

## Kebijakan bisnis default (dapat diubah di UI Pengaturan → Policy)
`discount_max_without_ceo = 5%` · `quiet_threshold_days = 14` (atau 2× ritme normal akun) · `gov_reminder_min_days = 30` · `credit_limit` per akun dari Odoo · `single_thread_share = 80%` · `bast_benchmark_days = 3` · `prep_benchmark_days = 14` · `install_benchmark_days = 14`.
