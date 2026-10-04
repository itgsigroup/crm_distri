# 00 — Bisnis GSI (baca selalu)

**PT Gosyen Solusi Indonesia (GSI)** — integrator AV/LED display & sistem keamanan untuk pemerintah dan enterprise. CEO/founder: Sam (Samuel Ridwan Setiadi). Nilai inti: *Simple – Innovative – Professional*.

## Lini bisnis
CCTV & VMS · fire alarm · videotron/LED (indoor/outdoor) · command center/videowall · digital signage · distribusi B2B (penjualan kredit/termin) · project instalasi · service & maintenance.

## Struktur
4 cabang: Jakarta, Semarang, Yogyakarta, Surabaya. ±30 staf, 14 unit organisasi. Sales per cabang (contoh di mockup: Andi/Semarang, Dewi/Yogyakarta, Rizky/Surabaya, Fajar/Jakarta). Teknisi ada di project Odoo tapi **tidak punya akun Odoo**. Odoo dipasang multi-company: tiap cabang = company terpisah meski satu badan hukum; transfer stok antar cabang diperlakukan internal (tanpa SO/PO, tidak masuk omset/pajak). Company lama tanpa prefix `[NEW]` diarsipkan; yang dipakai adalah company `[NEW]`.

## Sistem yang ada
- **Odoo** (gosyen-solusi-indonesia-2.odoo.com): CRM, sales, purchase, inventory, accounting, project. **Source of truth.**
- **Basecamp**: kolaborasi internal, delegasi, project internal. Tujuan pengiriman brief & tugas.
- **Talenta**: HR (data karyawan, absensi, cuti). Sumber nomor karyawan untuk registry nomor internal.
- Gmail & Google Calendar & Google Drive (proposal, penawaran, BAST).
- WhatsApp: nomor pribadi sales + grup project (eksternal dengan pelanggan, internal teknisi/gudang).
- Accurate Online (historis, CV Global Solusi) — di luar scope ARC.

## Masalah bisnis yang ARC harus pecahkan (urutan prioritas)
1. **Lead-to-cash terlalu lama** (median ~96 hari) karena langkah tidak diamati: quotation → Won → persiapan → barang siap → pemasangan → serah terima/BAST → invoice → lunas. Definisi segmen Sam: *quotation* = kartu CRM ke "Penawaran"; *order* = Won; *persiapan* = project dibuat → "barang siap"; *menunggu project* = barang siap → project berjalan; *pemasangan* = berjalan → selesai/serah terima; *administrasi* = selesai → BAST clear; *invoicing* = BAST clear → invoice dibuat; *lunas* = SO terbayar penuh.
2. **Piutang & penagihan**: pelanggan pemerintah mengikuti termin anggaran; enterprise punya pola bayar per akun. KPI divisi penagihan sudah ada di Odoo.
3. **Deal bertumpu satu orang, sinyal tidak terbaca** (kompetitor disebut, champion mutasi, sunyi > ritme normal).
4. **Pendapatan berulang** (maintenance saat garansi habis) dan **ekspansi lini** di akun yang sudah percaya tidak digarap sistematis.
5. **Distribusi kredit**: risiko konsentrasi piutang; SOP-SEC-001 (pencegahan social engineering) untuk pelepasan barang kredit — verifikasi PO via telepon ke nomor terdaftar, alamat kirim konsisten.
6. **Tender pemerintah** (LPSE Jateng/DIY/Jatim, e-katalog) masih dicari manual.

## Ritme & angka acuan (untuk seed data dan heuristik)
Siklus pemerintah ±96 hari, enterprise ±54 hari. Termin umum 30 hari. Deal dengan ≥ 2 stakeholder aktif menang ~3× lebih sering. Sunyi > 14 hari setelah revisi harga → ~71% berakhir kalah/mati. Sumber deal 24 bulan: tender 18, inbound 13, referral 12, ekspansi 11, cold 7 — win rate ekspansi 61%, referral 54%, inbound 31%, tender 22%, cold 9%.

## Arah strategis pemilik
Sam ingin mundur dari operasional harian dan membangun sistem yang berjalan sendiri (target independen akhir 2026). Inisiatif payungnya: **GSI Intelligent Business Operating System (IBOS)** — lapisan intelligence + orchestration + control di atas Odoo/Basecamp/Talenta, prinsip *manage by exception*, *execution without verification = not completed*, 5 level otonomi, segregation of duties, AI tidak pernah punya wewenang transfer bank. ARC adalah modul relasi/penjualan dari IBOS; desainnya harus cocok dengan itu (Chief AI, agen spesialis, approval center, exception center, CEO brief).
