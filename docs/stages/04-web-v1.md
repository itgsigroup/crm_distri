# Stage 04 — Web v1: Chat, Relasi + peta stakeholder + Peta 3D, Pengaturan WA, Hari ini lite

## Tujuan
Antarmuka pertama yang bisa dipakai Sam untuk menguji: menautkan WA dari UI, melihat chat dengan anotasi agen, membuka akun dengan memori, ledger, timeline, peta stakeholder, dan peta koneksi 3D.

## Baca dulu
`05-ui-spec.md` dan `reference/arc-crm-mockup.html` (spesifikasi visual; porting, jangan mendesain ulang), `04` (hak akses).

## Kerjakan
1. Auth sederhana (pengguna internal dari seed, sesi cookie, role, cabang) + `GET /me`.
2. Shell: rail (Kerja hari ini / Bisnis / Pengaturan), topbar dengan judul, view tabs, kotak Tanya ARC (⌘K — v1: hanya cari akun; pertanyaan aktif di tahap 06), tabbar mobile, tema light/dark, toast, action sheet global (komponen; dipakai penuh di tahap 05).
3. **Chat**: 3 panel sesuai mockup — daftar dengan tab Semua/Pelanggan/Grup ekst./Grup int./Internal + judul bagian + badge; thread dengan anotasi ARC di bawah pesan (dari ekstraksi tahap 03) dan tag internal/eksternal pengirim grup; panel konteks (akun, ekstraksi, komitmen; grup: anggota, ringkasan, tugas terdeteksi → tombol "Buat tugas" membuat Action); composer "Balas sebagai <pemilik nomor>" → Action `send_wa` proposed; 3 saran balasan (`GET /chat/threads/{id}/suggestions`, Follow-up agent v0: LLM heavy dari 10 pesan terakhir).
4. **Relasi**: daftar akun (health dot, nilai, terakhir, cari); halaman akun dengan anchor bar dan kartu: header (health ring + tren), Langkah berikutnya (placeholder sampai tahap 05), Memori akun (+provenance), **Peta stakeholder** (SVG radial dari Person: tebal garis = intensitas, putus-putus = belum kontak, badge single-threaded), Commitment ledger, Timeline (+kesimpulan ARC). Menu **Peta 3D** (rail) + view tab di Relasi + **peta mini per akun** di halaman akun (`createNet()` reusable, fallback 2D canvas bila three.js gagal dimuat): port dari mockup dengan data nyata `GET /network?period=&sales=` (agregat Interaction WA per bulan; internal dikecualikan), filter, periode 30/60/90/180, tooltip, fokus → akun, panel pasangan terkuat & pola koneksi (insight deterministik dari tahap 03/04 API).
5. **Pengaturan**: tab WhatsApp (mode bridge/Cloud per nomor, daftar nomor & status sesi, **QR pairing dari bridge**, riwayat awal 30–180 hari, Batas & privasi dengan toggle nyata ke Policy, grup yang dibaca dengan opt-in ext/int), tab Nomor internal (tabel, form, impor CSV, dugaan ARC), tab Sumber sinyal (status), tab AI & model (routing dari config, biaya dari `/llm/usage`). Tab MCP & API placeholder.
6. **Hari ini (lite)**: sapaan, Sinyal baru, Komitmen hari ini, Keputusan (Action proposed dengan tombol approve/reject sederhana), baris agen. Brief & forecast placeholder (tahap 07/05).
7. Vitest komponen kritis + Playwright smoke: login → Pengaturan WA (status) → Chat (pilih thread, lihat anotasi) → Relasi (akun RSUD, peta stakeholder) → Peta 3D render.

## Acceptance criteria
- Playwright smoke lolos; 400 px tanpa scroll horizontal; a11y Lighthouse ≥ 90 di Chat & Relasi.
- Uji manual end-to-end didokumentasikan: scan QR di UI → kirim WA dari HP lain → muncul di Chat ≤ 5 detik → anotasi ARC muncul ≤ 90 detik → Person/akun & komitmen terlihat di Relasi → node muncul di Peta 3D.
- Semua data dari API; tidak ada konstanta mockup di web.
