import type { ScreenKey } from '../../lib/i18n/id'

// Buku manual GSI Orbit (Sam 22 Oct): how the system works, a tutorial per menu, the daily routine per role, terms,
// FAQ. Plain Markdown rendered by components/Markdown (headings, lists, tables, quotes, **bold**, `code`).
// Keep it in step with the app: a changed screen means a changed chapter.

export interface Chapter {
  id: string
  group: string
  title: string
  icon: string
  /** the menu this chapter explains: "Buka halaman ini" */
  screen?: ScreenKey
  md: string
}

export const CHAPTERS: Chapter[] = [
  {
    id: 'mulai',
    group: 'Mengenal sistem',
    title: 'Selamat datang di GSI Orbit',
    icon: 'sun',
    md: `
# Selamat datang di GSI Orbit

**GSI Orbit** adalah CRM distribusi B2B milik PT Gosyen Solusi Indonesia. Sistem ini membantu tim menjaga setiap dealer, toko, dan installer tetap **"di orbit"**: order rutin sesuai siklusnya, limit kredit sehat, dan pilihan produknya makin lebar.

Berbeda dengan CRM biasa yang mengejar "closing", di distribusi dealer yang sama order berulang kali. Karena itu yang dikelola di sini adalah **ritme**: kapan dealer seharusnya order lagi, siapa yang mulai menjauh, siapa yang tagihannya menumpuk, dan barang apa yang perlu didorong.

## Apa yang bisa dilakukan GSI Orbit

- **Chat WhatsApp banyak nomor** dalam satu layar: nomor sales, admin, dan penagihan, termasuk grup pelanggan.
- **Membaca data penjualan, piutang, dan stok** dari Accurate (lewat BigQuery) lalu menghitung angka-angka penting secara otomatis.
- **Orchestrator AI** yang mengatur enam agen AI untuk menganalisa data dan **mengusulkan** langkah: follow-up dealer, penagihan, push stok, rilis kredit, dan lain-lain.
- **Analisa** per dealer, per chat, per stok, dan per sales, lengkap dengan insight.
- **Kontrol akses** per peran: setiap orang hanya melihat menu dan data yang menjadi tugasnya.

## Prinsip yang selalu berlaku

> **AI mengusulkan, manusia memutuskan.** Tidak ada pesan, harga, rilis kredit, atau perubahan limit yang sampai ke pelanggan tanpa keputusan orang yang tercatat namanya.

1. **Angka dihitung, bukan dikarang.** Semua angka (siklus order, sisa limit, skor dealer, perputaran stok, dan lainnya) dihitung sistem dengan rumus tetap. AI hanya menulis alasan, ringkasan, dan draf pesan.
2. **Setiap klaim AI punya sumber.** Hasil AI selalu disertai siklus, waktu, dan tingkat yakin, sehingga bisa ditelusuri.
3. **Tidak ada pengiriman otomatis.** Draf dari AI masuk ke kolom ketik atau antrean Keputusan; Anda yang menekan **Kirim** atau **Setujui**.
4. **Privasi dijaga.** Chat pribadi antar karyawan tidak dianalisa, dan data pribadi disamarkan sebelum dibaca AI.

## Cara membaca buku manual ini

- Bab **Mengenal sistem** menjelaskan cara kerja dan istilah. Baca sekali di awal.
- Bab **Tutorial per menu** menjelaskan setiap halaman, langkah demi langkah. Tombol **Buka halaman ini** di atas bab membawa Anda langsung ke menunya.
- Bab **Alur kerja harian** berisi rutinitas yang disarankan untuk tiap peran.
- Bab **Bantuan** berisi tanya-jawab dan cara mengatasi masalah umum.

Gunakan kotak **Cari di manual** di kiri atas untuk menemukan topik dengan cepat, misalnya "label", "limit", atau "reorder".
`,
  },
  {
    id: 'cara-kerja',
    group: 'Mengenal sistem',
    title: 'Cara kerja sistem',
    icon: 'net',
    md: `
# Cara kerja sistem

Secara sederhana, GSI Orbit bekerja dalam empat lapis: **data masuk → angka dihitung → AI menganalisa → manusia memutuskan**.

## 1. Data masuk

| Sumber | Isi | Seberapa sering |
|---|---|---|
| **WhatsApp** | Pesan dari dan ke pelanggan di semua nomor GSI yang ditautkan, termasuk grup pelanggan | Seketika (real-time) |
| **Accurate lewat BigQuery** | Pelanggan, faktur, pembayaran, penjualan per barang, stok per gudang | Stok tiap 5 menit; data lain sesuai jadwal sinkron |
| **Input tim** | Keputusan, label, catatan, hitung stok real, nama panggilan, dan lainnya | Saat Anda menyimpannya |

GSI Orbit **tidak mengubah data Accurate**. Data pelanggan baru, misalnya, disalin dari tombol **Kirim ke Accurate** lalu disimpan manual di Accurate oleh admin.

## 2. Angka dihitung sistem

Dari data itu, sistem menghitung untuk setiap dealer: **siklus order**, **jadwal order berikutnya**, **status** (Key account, Aktif, Baru, At risk, Churn), **segmen** (A, B, C, D), **sisa limit**, **share of wallet**, **product mix**, **PIC aktif**, dan **skor dealer 0–100**. Untuk stok: **umur stok**, **kecepatan jual**, **perputaran stok**, dan kelas **fast / slow moving**. Rumusnya dijelaskan di bab **Istilah & rumus penting**.

## 3. Orchestrator dan enam agen AI

**Orchestrator** adalah "manajer" yang mengatur enam agen AI. Setiap siklus melewati enam tahap:

1. **Ingest (Baca data)** — mengumpulkan chat, order, faktur, dan stok yang baru.
2. **Analisis** — enam agen bekerja bersamaan.
3. **Sintesis (Gabungkan)** — menggabungkan temuan dan menyelesaikan konflik antar agen (misalnya jangan menawarkan barang ke dealer yang tagihannya lewat tempo; tagih dulu).
4. **Keputusan (Usulan)** — menyusun usulan untuk Anda.
5. **Eksekusi (Simpan)** — menjalankan hanya yang **sudah disetujui manusia**.
6. **Belajar** — mencatat keputusan Anda (disetujui, diedit, ditolak) agar usulan berikutnya lebih tepat.

| Agen | Tugas |
|---|---|
| **AI Order** | Mengubah permintaan di WhatsApp menjadi draf order, menangkap nego harga |
| **AI Follow-up** | Mengingatkan dealer yang jadwal order-nya dekat atau sudah lewat, lengkap dengan rekomendasi order |
| **AI Kredit** | Menjaga sisa limit: tahan, rilis sebagian, atau usul kenaikan limit |
| **AI Stok** | Mendorong stok menua ke dealer yang cocok, usul mutasi dan pembelian |
| **AI Penagihan** | Pengingat faktur dengan nada sesuai pola bayar dealer |
| **AI Prospek** | Mengenali nomor baru, mengusulkan dealer baru dan kirim daftar harga |

Orchestrator berjalan **otomatis tiap jam** pada jam kerja, dan bisa dijalankan kapan saja lewat tombol **Analisis ulang**. Analisa AI dikerjakan oleh **Claude** lewat koneksi MCP; pilihan **Sistem** menjalankan aturan dan rumus GSI Orbit saja, tanpa AI.

## 4. Manusia memutuskan

Usulan AI masuk ke antrean **Keputusan** di Pusat kendali, atau menjadi draf di kolom ketik Chat. Anda bisa **Setujui**, **Edit** lalu setujui, atau **Tolak** dengan alasan. Hanya yang disetujui yang dikirim, dan nama Anda tercatat sebagai pengirim.

## Siapa melihat apa

Menu dan data mengikuti **peran** Anda (diatur di **Peran & akses**):

- **Sales** melihat dealer dan chat miliknya sendiri, serta chat di nomor yang ia pegang.
- **Admin** dan **Penagihan** melihat chat di nomor timnya.
- **Manajer** dan **CEO** melihat semua data.
- Keputusan tertentu (misalnya rilis kredit di atas limit) hanya bisa diambil peran yang berwenang.
`,
  },
  {
    id: 'istilah',
    group: 'Mengenal sistem',
    title: 'Istilah & rumus penting',
    icon: 'doc',
    screen: 'konsep',
    md: `
# Istilah & rumus penting

Istilah di GSI Orbit dipakai persis sama di semua layar. Penjelasan bergambar ada di menu **Panduan** (konsep Orbit).

## Ritme order

| Istilah | Arti |
|---|---|
| **Siklus order** | Jarak rata-rata (median) antar hari order dealer dalam 6 bulan terakhir. Beberapa faktur di hari yang sama dihitung 1 order. |
| **Jadwal order** | Hari dealer seharusnya order lagi: order terakhir + siklus order. |
| **Lewat jadwal** | Dealer melewati 1,2× siklusnya tanpa order. |
| **cyc** | Posisi dalam siklus: hari sejak order terakhir ÷ siklus order. 1,0 = tepat jadwal. |

## Status dealer

| Status | Arti |
|---|---|
| **Key account** | Dealer aktif dengan share of wallet ≥ 50%, bayar tepat waktu ≥ 85%, dan omzet besar |
| **Aktif** | Order sesuai ritme |
| **Baru** | Baru sekali order, ritmenya belum terbentuk |
| **At risk** | Sudah melewati 1,2× siklusnya |
| **Churn** | Berhenti order (lebih dari 2× siklus, atau lebih dari setahun) |
| **Prospek** | Belum pernah order |

## Segmen (seringnya × besarnya order)

| Segmen | Ciri | Cara melayani |
|---|---|---|
| **A** | Sering dan besar | Prioritas: jaga & layani terbaik |
| **B** | Sering tapi kecil | Upsell: naikkan nilai order |
| **C** | Jarang tapi besar | Project-based: ikuti proyeknya |
| **D** | Jarang dan kecil | Low-touch: layani otomatis |

## Kredit dan kas

| Istilah | Arti |
|---|---|
| **Sisa limit** | 1 − piutang berjalan ÷ limit. **Aman**, **Tipis** (sisa < 40% atau rata-rata bayar > 35 hari), **Overdue** (ada faktur lewat tempo), **Over limit** (piutang melebihi limit), **Cash** (tanpa limit) |
| **Pola bayar** | Rata-rata hari dari faktur sampai dibayar, 6 bulan |
| **On time** | Persen faktur dibayar sebelum jatuh tempo, 12 bulan |
| **DSO** | Rata-rata hari dari order sampai dibayar, 90 hari (target ≤ 30 hari) |

## Dealer

| Istilah | Arti |
|---|---|
| **Share of wallet** | Porsi belanja dealer yang jatuh ke GSI dibanding total belanjanya untuk kategori yang GSI jual |
| **Product mix** | Berapa dari 6 kategori (Kamera & NVR, HDD & storage, Kabel & PoE, Modul LED, Fire alarm, Aksesoris) yang dibeli dealer dalam 6 bulan |
| **PIC aktif** | Kontak dealer yang membalas WhatsApp atau order dalam 90 hari. Hanya 1 PIC = risiko |
| **Skor dealer** | 0–100 dari ritme order (30%), share of wallet (25%), product mix (15%), kesehatan kredit (15%), dan PIC aktif (15%). ≥ 70 kuat, 50–69 waspada, < 50 lemah |

## Stok

| Istilah | Arti |
|---|---|
| **Stok menua** | Stok yang umurnya lebih dari 90 hari |
| **Stok kritis** | Akan habis dalam kurang dari 10 hari pada kecepatan jual sekarang |
| **Kecepatan jual** | Rata-rata unit terjual per minggu dalam 90 hari terakhir |
| **Perputaran stok** | Nilai stok ÷ HPP harian = berapa hari stok bertahan (target ≤ 40 hari) |
| **Fast moving** | 20% barang dengan kecepatan jual tertinggi |
| **Slow moving** | Masih terjual, tetapi stoknya cukup untuk lebih dari 90 hari |
| **Dead stock** | Tidak terjual sama sekali dalam 90 hari |
| **ABC** | Kelas A = barang yang menghasilkan ±80% penjualan, B = 15% berikutnya, C = sisanya |

Angka ambang (1,2× siklus, 40% sisa limit, 90 hari stok menua, dan lainnya) adalah **kebijakan** yang bisa diubah CEO di **Pengaturan**.
`,
  },
  {
    id: 'navigasi',
    group: 'Mengenal sistem',
    title: 'Masuk & navigasi',
    icon: 'lock',
    md: `
# Masuk & navigasi

## Masuk ke aplikasi

1. Buka alamat GSI Orbit di browser (Chrome atau Edge disarankan).
2. Masukkan **email** dan **kata sandi** yang diberikan admin.
3. Sesi tetap tersimpan, sehingga Anda tidak perlu login ulang setiap hari di komputer yang sama.

> Gunakan **akun Anda sendiri**. Label pengirim di setiap pesan, keputusan, dan audit mengikuti akun yang login. Jika beberapa admin memakai satu akun bersama, semua pesan akan tercatat atas nama akun itu.

## Bagian layar

- **Sidebar kiri** — daftar menu sesuai peran Anda, dikelompokkan: Kendali, Dealer, Operasi, Master data, Pengaturan. Angka merah/hijau di samping menu adalah jumlah yang perlu perhatian (misalnya chat belum dibaca, keputusan menunggu).
- **Judul halaman** di atas, dengan **pill Orchestrator** di kanan atas yang menunjukkan status analisis dan jadwal berikutnya.
- **Tombol Orchestrator** — kotak di kanan bawah (atau tombol di header Chat) untuk menjalankan **Analisis ulang** dan bertanya ke Orchestrator.

## Kebiasaan yang memudahkan

| Aksi | Cara |
|---|---|
| Ringkas sidebar jadi ikon | Tombol panah di bawah sidebar |
| Ganti tema terang/gelap dan warna sidebar | Klik nama Anda di kiri bawah |
| Buka kotak "Tanya Orchestrator" | **Ctrl+K** (Windows) atau **⌘K** (Mac) |
| Keluar | Ikon keluar di samping nama Anda |

Di HP, menu utama ada di bilah bawah layar.
`,
  },
  {
    id: 'pusat-kendali',
    group: 'Tutorial per menu',
    title: 'Pusat kendali',
    icon: 'sun',
    screen: 'today',
    md: `
# Pusat kendali

Halaman pertama setelah login. Isinya **apa yang harus dikerjakan hari ini**.

## Isi halaman

- **Rencana hari ini** — daftar langkah berurutan jam yang disusun Orchestrator (maksimal 10), misalnya "Balas chat Toko Sinar — menunggu 5 jam" atau "Follow-up Mitra Jaya H-1 jadwal order".
- **Keputusan** — antrean usulan AI yang menunggu persetujuan Anda.
- **Ringkasan Orchestrator** — hasil siklus terakhir: berapa sinyal dibaca, berapa usulan, konflik yang diselesaikan.
- **Jadwal order 7 hari** dan **Lewat jadwal** — dealer yang perlu disapa.
- **KPI utama** — Order tepat jadwal, DSO, Perputaran stok.
- **Agenda per sales**, **Push stok**, dan **Limit tipis**.

## Tutorial: memproses keputusan

1. Buka kartu **Keputusan**. Setiap usulan menampilkan agen pengusul, judul, alasan, dan sumber datanya.
2. Klik usulan untuk melihat detail: draf pesan, langkah, dampak, dan opsi (misalnya DP 50% / tahan / rilis sebagian).
3. Pilih salah satu:
   - **Setujui** — usulan dijalankan; draf WhatsApp dikirim dari nomor sales pemilik dealer atas nama Anda.
   - **Edit** — ubah draf atau angka, lalu setujui. Perubahan Anda dipelajari AI.
   - **Tolak** — pilih alasan. Usulan serupa untuk dealer itu ditahan 14 hari.
4. Status usulan berubah dan tercatat di audit dengan nama Anda.

> Usulan yang menyangkut kredit di atas limit hanya bisa disetujui peran yang berwenang (CEO).

## Tips

- Kerjakan Rencana hari ini dari atas ke bawah; urutannya sudah memperhitungkan konflik (misalnya tagih dulu sebelum menawarkan barang).
- Label **Analisa WhatsApp** hijau berarti analisa chat terbaru sudah masuk; merah "terlambat" berarti lebih dari 2½ jam tidak ada analisa baru.
`,
  },
  {
    id: 'dashboard',
    group: 'Tutorial per menu',
    title: 'Dashboard',
    icon: 'chart',
    screen: 'dashboard',
    md: `
# Dashboard

Ringkasan kondisi bisnis dalam satu layar: dealer, order, kas, stok, dan aktivitas.

## Stasiun Orbit

Gambar 3D stasiun luar angkasa dengan **10 ruangan** di sekitar Inti AI. Setiap ruangan adalah satu tugas: Analisa sales, Analisa WhatsApp, Analisa dealer, Eksekusi order, Follow-up, Cek stok, Cek kredit, Cek penagihan, Prospek, dan Data siap. Saat siklus Orchestrator berjalan, ruangan menyala berurutan sesuai tahap yang sedang dikerjakan. Klik ruangan untuk membuka layar terkait.

Tombol **Perbesar** dan **Layar penuh** cocok untuk ditampilkan di TV kantor.

## Hasil overview AI

Di bawah stasiun ada 10 kotak hasil: omzet bulan ini, chat belum dibaca, dealer At risk, jadwal order, stok menua dan kritis, limit tipis, lewat tempo, nomor baru, dan usulan menunggu. Klik kotak untuk membuka detailnya.
`,
  },
  {
    id: 'orchestrator',
    group: 'Tutorial per menu',
    title: 'Orchestrator & Analisis ulang',
    icon: 'spark',
    screen: 'orch',
    md: `
# Orchestrator & Analisis ulang

Halaman **Orchestrator** menampilkan status siklus, enam tahap, riwayat siklus, kartu enam agen, konflik yang diselesaikan, dan matriks otonomi (usulan mana yang boleh jalan otomatis dan mana yang wajib disetujui).

## Tutorial: menjalankan Analisis ulang

1. Klik kotak **Orchestrator** (kanan bawah), atau tombol **Orchestrator** di samping nama chat di halaman Chat.
2. Lihat **Layar ini**: analisis akan dibatasi pada halaman yang sedang dibuka (misalnya hanya stok, hanya satu dealer, atau hanya chat yang terbuka). Tekan **Semua** untuk menganalisa seluruh data.
3. Pilih mesin:
   - **Sistem** — aturan dan rumus GSI Orbit, langsung jalan, tanpa AI dan tanpa biaya.
   - **MCP Claude** — Claude membaca data dan menulis analisa. Pilih **Model** dan **Effort** (semakin tinggi semakin teliti, semakin lama).
4. Tekan **Analisis ulang**.
5. Pop-up analisis menampilkan tahap yang sedang berjalan, agen yang sudah selesai, dan durasinya.
   - Tekan **Perkecil · jalan di belakang layar** untuk lanjut bekerja. Pop-up menjadi bar kecil di pojok kiri bawah; klik bar itu untuk membukanya lagi.
   - Tekan **Batalkan analisis** jika analisis tertahan. Data dan saran sebelumnya tetap dipakai.
6. Setelah selesai, muncul hasil **berhasil**, **selesai sebagian**, atau **gagal**, dan halaman otomatis diperbarui.

Di halaman Chat, progres analisis tampil langsung di dalam panel Orchestrator (tanpa pop-up kedua).

## Routine Claude

Jika **Routine Claude** sudah diisi di halaman **MCP Claude**, Claude bekerja otomatis di claude.ai; tidak perlu membuka aplikasi apa pun. Jika belum, pop-up menampilkan tombol **Buka Claude**: buka Claude, pilih model yang disebut, lalu tekan Enter.

## Tanya Orchestrator

Ketik pertanyaan atau perintah di kotak **Tanya atau perintahkan Orchestrator**, misalnya "dealer mana yang lewat jadwal?" atau "analisis ulang stok".

Jawaban mengikuti pertanyaan. Contoh untuk stok:

| Pertanyaan | Jawaban |
|---|---|
| "stok kamera ip 4mp dome berapa?" | Stok barang itu per cabang, kecepatan jual, cukup berapa hari |
| "barang apa yang fast moving?" | Daftar fast moving |
| "dead stock apa saja" / "slow moving" | Daftar barang yang tidak atau lambat terjual |
| "barang yang perlu reorder" | Barang yang akan habis dengan saran jumlah pesan |
| "mutasi stok" | Peluang mutasi antar cabang |
| "umur stok" / "stok di jakarta" | Umur stok, atau ringkasan stok satu cabang |
| "stok apa yang harus didorong" | Stok menua beserta dealer yang cocok untuk push |

Pertanyaan lain: risiko dealer, prediksi kas atau tagihan, jadwal order, dealer yang lewat jadwal. Menulis nama dealer saja (tanpa pertanyaan) membuka halaman dealernya.
`,
  },
  {
    id: 'chat',
    group: 'Tutorial per menu',
    title: 'Chat WhatsApp',
    icon: 'chat',
    screen: 'chat',
    md: `
# Chat WhatsApp

Semua nomor WhatsApp GSI dalam satu layar: chat pribadi pelanggan, grup pelanggan, dan grup internal.

## Bagian layar

1. **Bar nomor** di atas — pilih nomor yang ingin dilihat. Nomor dikelompokkan **Sales · Admin · Penagihan**; klik nama tim untuk melihat semua chat tim itu.
2. **Daftar chat** di kiri — dengan kotak cari, filter, dan label.
3. **Percakapan** di tengah.
4. **Panel lead** di kanan — data pelanggan dan analisa AI.

## Tutorial: mencari chat dan pesan

1. Ketik di **Cari chat** (minimal 2 huruf).
2. Hasil terbagi dua seperti WhatsApp: **Chat** (cocok dari nama atau nomor) dan **Pesan** (isi pesan di semua chat, termasuk yang lebih lama dari 14 hari).
3. Klik hasil di bagian Pesan untuk membuka chat dan langsung melompat ke pesannya.
4. Untuk mencari di dalam satu percakapan, klik ikon 🔍 di header chat. Gunakan **Enter** / **Shift+Enter** untuk berpindah hasil.

## Filter dan label

- Chip filter: **Semua, Belum dibaca, Belum dibalas, Dealer, Grup internal, Grup pelanggan, Nomor baru, Internal, Pemasok**.
- **Label** seperti WhatsApp Business (Pelanggan baru, Pesanan baru, Menunggu pembayaran, Sudah dibayar, Pesanan selesai, atau buat sendiri). Pasang di chat lewat ikon label di header, atau di pesan lewat menu bubble.
- Menu **⌄** di kartu chat (atau klik kanan): **Sematkan chat**, **Label**, **Tandai belum dibaca**, **Tandai belum dibalas / sudah dibalas**.
- **Ringkas tampilan daftar**: tombol di atas daftar atau **Alt+R**.

## Tutorial: membalas pelanggan

1. Buka chat, ketik di kolom bawah. **Enter** mengirim, **Shift+Enter** baris baru.
2. Format teks seperti WhatsApp: pilih teks untuk tebal, miring, coret, atau monospace.
3. Lampiran lewat 📎: **Dokumen**, **Foto & video**, **Polling**, **Kontak**. Tempel gambar langsung dengan Ctrl+V. Maksimal 25 MB.
4. **Pesan suara**: tekan ikon mikrofon saat kolom kosong.
5. Di grup, ketik **@** untuk menandai anggota.
6. Menu **⌄** di bubble: **Balas**, **Salin**, **Teruskan**, **Sematkan**, **Label**, **Edit** (≤ 15 menit), **Hapus untuk semua orang** (≤ 48 jam).

Setiap pesan keluar menampilkan **label nama pengirim** (akun yang login saat mengirim), misalnya **Tantri** atau **Sekar**. Pesan yang diketik langsung di HP berlabel **dari HP**. Pesan dikirim lewat penjaga anti-blokir (ada jeda dan "mengetik…" seperti manusia).

## Kartu AI Trigger di bawah pesan pelanggan

Jika pesan pelanggan menanyakan **stok**, **harga**, **limit & tagihan**, atau **status order**, muncul kartu di bawah bubble berisi data yang sesuai (stok per cabang, harga tier dealer, sisa limit, status order) dan **draf jawaban dari AI**.

1. Periksa angka di kartu.
2. Tekan **Pakai jawaban** — draf masuk ke kolom ketik.
3. Edit seperlunya, lalu kirim sendiri.

## Panel lead (kanan)

| Tab | Isi |
|---|---|
| **Main** | Identitas pelanggan, tahap, peringatan kredit, usulan Claude, dan **Analisa Claude dari awal sampai akhir**: overview perusahaan, perjalanan percakapan atau grup, dan kerjasama (usulan Claude + keputusan tim) |
| **AI** | Catatan Claude per pesan dan usulan Claude |
| **Sumber** | Nomor masuk lewat mana, pesan pertama, hasil identifikasi nomor |
| **Accurate** | Kode pelanggan, limit, piutang, rata-rata bayar |
| **Akun** | Nomor WhatsApp GSI yang menangani chat |
| **Statistik** | Insight dan grafik seluruh riwayat chat: kecepatan balasan, tren, jam aktif pelanggan, siapa yang memulai, barang yang sering ditanya |

Tombol **Kirim ke Accurate** menyalin data pelanggan untuk dibuat atau diperbarui di Accurate.

## Tampilan

- Ikon panah di header chat: **Ringkas header** menjadi satu baris tipis.
- Tombol **− / +**: zoom percakapan.
- **Per pesan**: tampilkan analisa AI per bubble.
- Panel lead bisa diringkas jadi ikon.

## Grup dan tipe kontak

- **Grup pelanggan** dibaca analisa; pesan karyawan GSI di grup dihitung sebagai balasan. **Grup internal** tidak dianalisa (hanya stok, jadwal, tugas). Ubah lewat panel lead → Main.
- Untuk chat pribadi, tandai **Pemasok** (tidak dihitung sebagai lead) atau **Obrolan pribadi** (dihapus dari CRM dan tidak disimpan lagi).
- Klik ikon pensil di samping nama untuk memberi **nama panggilan**; nama ini juga tersimpan ke kontak WhatsApp di HP.
`,
  },
  {
    id: 'report-wa',
    group: 'Tutorial per menu',
    title: 'Report WhatsApp',
    icon: 'trend',
    screen: 'reportwa',
    md: `
# Report WhatsApp

Laporan kinerja chat per sales: kecepatan respons, chat gantung, nomor baru, dan order dari chat. Semua dihitung dari chat, tanpa AI.

## Cara memakai

1. Pilih periode: **Hari ini**, **7 hari**, atau **30 hari**.
2. Pilih sales (sales hanya melihat datanya sendiri).
3. Baca angka tim: chat masuk, % dibalas sesuai target (30 menit di jam kerja), % permintaan dijawab, chat gantung, nomor baru, order dari chat.
4. Lihat **Peringkat sales** (skor 0–100, winning sales), **feedback baik & buruk** tiap sales, dan **ringkasan untuk direktur & manager**.
5. Gunakan **Tren harian** dan **Jam ramai pelanggan** untuk mengatur jadwal jaga.
6. Tabel **Chat gantung** (belum dibalas atau janji "cek dulu") dan **Nomor baru** bisa dicari, diurutkan, dan diklik untuk membuka chat.

> Waktu respons dihitung sampai pelanggan **mendapat jawaban**. Balasan sementara seperti "tunggu sebentar" tidak menghentikan hitungan.
`,
  },
  {
    id: 'orbit',
    group: 'Tutorial per menu',
    title: 'Orbit, Segmen & Peta relasi',
    icon: 'target',
    screen: 'orbit',
    md: `
# Orbit, Segmen & Peta relasi

Tiga cara melihat semua dealer.

## Orbit — kapan dealer harus disapa

Papan lingkaran: setiap titik adalah dealer, posisinya menunjukkan letak dealer dalam siklus order-nya. **Atas** = jadwal order; titik yang **keluar dari lingkar** = lewat jadwal. Lingkar dari dalam ke luar: Key account, Aktif, At risk, Churn.

1. Arahkan kursor ke titik untuk melihat nama dan status; klik untuk membuka halaman dealer.
2. Daftar di samping memuat isi tiap status dan dealer yang baru bergerak.
3. Kartu **Analisa Orbit** berisi bacaan Claude dari chat WhatsApp.

## Segmen — dealer jenis apa

Grafik titik: sumbu datar = **seberapa sering** order, sumbu tegak = **seberapa besar** order. Garis membagi menjadi Segmen A, B, C, D. Titik samar = posisi 3 bulan lalu, sehingga terlihat siapa yang naik atau turun.

- Geser dan zoom seperti peta.
- Kotak tiap segmen berisi apa yang harus dilakukan (Prioritas, Upsell, Project-based, Low-touch).
- **Analisa Segmen** oleh Claude.

## Peta relasi — seberapa dekat sales dengan dealer

Grafik 3D sales ↔ dealer berdasarkan interaksi WhatsApp dan order. Garis tebal = hubungan kuat. Gunakan filter sales dan dealer, lihat **pasangan terkuat**, dan baca **Analisa Relasi** oleh Claude untuk menemukan dealer yang hanya bergantung pada satu orang.
`,
  },
  {
    id: 'dealer',
    group: 'Tutorial per menu',
    title: 'Dealer',
    icon: 'building',
    screen: 'dealer',
    md: `
# Dealer

## Daftar dealer

- Kotak **Dealer aktif** menunjukkan persentase dealer aktif dan komposisi status. Klik chip **Key account · Aktif · Baru · At risk · Churn** untuk memfilter; klik lagi untuk melepas.
- Daftar diurutkan menurut posisi siklus order. Klik dealer untuk membuka halamannya.

## Halaman satu dealer

| Bagian | Isi |
|---|---|
| **Header** | Status, segmen, skor dealer dengan 5 komponennya, omzet/bulan |
| **Langkah berikutnya** | Usulan terbaik untuk dealer ini, siap disetujui |
| **Analisa AI** | Penilaian Claude: **Bermasalah / Perlu perhatian / Aman**, alasan, ringkasan, dan draf WhatsApp |
| **Order-to-cash** | Order → Siap → Kirim → Invoice → Bayar, dengan lama putaran |
| **Share of wallet & product mix** | Porsi belanja ke GSI dan kategori yang dibeli / belum dibeli (peluang) |
| **Sisa limit** | Limit, piutang berjalan, faktur lewat tempo, pola bayar |
| **Memori** | Catatan penting tentang dealer |
| **PIC aktif** | Kontak yang aktif membalas atau order |
| **Komitmen** | Janji GSI dan janji dealer, dengan status |
| **Timeline** | Riwayat order, faktur, pembayaran, dan chat |

## Tutorial: menindaklanjuti Analisa AI

1. Buka halaman dealer, lihat kartu **Analisa AI**. Jika belum ada atau sudah lama, jalankan **Analisis ulang** (layar ini = dealer ini).
2. Baca status dan alasannya.
3. Tekan tombol chat (**selesaikan masalah**, **sapa & tawarkan**, atau **sapa dealer**). Chat dealer terbuka dengan draf Claude sudah di kolom ketik.
4. Edit dan kirim sendiri.
`,
  },
  {
    id: 'push-stok',
    group: 'Tutorial per menu',
    title: 'Push stok',
    icon: 'box',
    screen: 'stock',
    md: `
# Push stok

Mengubah **stok yang menua** menjadi alasan follow-up ke dealer yang jadwal order-nya dekat.

## Isi halaman

- **KPI stok** dan status sinkron stok dari BigQuery (tiap 5 menit; tombol **Sinkron sekarang** untuk yang berwenang).
- **Kandidat push**: stok lebih dari 90 hari dengan dealer yang cocok — kategori barang pernah dibeli dealer (atau kategori yang masih kosong), jadwal order ≤ 7 hari atau sudah lewat, dan sisa limit tidak over/overdue.
- **Stok kritis**: barang yang akan habis sebelum siklus berikutnya.
- **Penjualan per produk**.

## Tutorial

1. Pilih barang di daftar kandidat.
2. Periksa dealer yang disarankan (urut: jadwal order terdekat, lalu omzet terbesar).
3. Jalankan **Analisis ulang** untuk membuat usulan **push stok**. Harga bundle tidak pernah di bawah batas margin minimum (9%).
4. Setujui usulannya di Pusat kendali. Pesan dikirim dari nomor sales pemilik dealer.
`,
  },
  {
    id: 'cek-stok',
    group: 'Tutorial per menu',
    title: 'Cek stok & Analisa stok',
    icon: 'check',
    screen: 'stockcheck',
    md: `
# Cek stok & Analisa stok

## Analisa stok (kartu paling atas)

Analisa untuk owner dan manajer dalam **satu kartu**: di mana uang tertahan dan apa yang harus dilakukan. Pilih **Semua cabang** atau satu cabang, lalu pilih **filter**:

| Filter | Menjawab |
|---|---|
| **Ringkasan** | Komposisi stok FSN (Fast moving, Normal, Slow moving, Dead stock), analisa ABC, dan insight |
| **Fast moving** | Barang paling cepat terjual, berapa hari stoknya cukup |
| **Umur stok** | Nilai stok per periode umur (0–30, 31–60, 61–90, 91–180, > 180 hari) |
| **Per cabang** | Nilai stok, dead stock, dan hari perputaran tiap cabang |
| **Per kategori** | Nilai stok dan hari perputaran tiap kategori |
| **Reorder segera** | Barang yang habis kurang dari 10 hari, dengan saran unit untuk 30 hari |
| **Dead stock** | Barang tidak terjual 90 hari dan di cabang mana ia diam |
| **Slow moving** | Barang yang stoknya cukup lebih dari 90 hari |
| **Mutasi antar cabang** | Barang yang diam di satu cabang dan dibutuhkan cabang lain |

Cara memakai:
1. Klik chip filter. Angka di chip = jumlah barang (atau cabang/kategori).
2. Pada **Umur stok**, **Per cabang**, dan **Per kategori**, klik batang grafik untuk melihat barang di kelompok itu. **Lepas pilihan** untuk kembali.
3. Cari barang di tabel, tekan **Tampilkan … lagi** untuk baris berikutnya.
4. Klik baris untuk membuka cek barangnya.
5. Kotak KPI **Dead stock**, **Stok menua**, dan **Reorder segera** langsung membuka filternya.

## Data stok dan cek barang

1. Cari barang di tabel **Data stok** (nama, kode, kategori). Filter per cabang, urutkan per kolom.
2. Klik barang. Jendela cek terbuka berisi jawaban AI Trigger untuk barang itu, stok per cabang, data mentah BigQuery per gudang, harga per tier, dan riwayat cek.
3. Ketik **stok real** dari Accurate atau hasil hitung gudang per cabang, beri catatan, lalu **Simpan**.
4. Hasilnya menunjukkan sisi mana yang salah: data GSI Orbit vs BigQuery (masalah sinkron atau mapping gudang), atau BigQuery vs stok real.
`,
  },
  {
    id: 'kredit',
    group: 'Tutorial per menu',
    title: 'Kredit · kas',
    icon: 'cash',
    screen: 'ar',
    md: `
# Kredit · kas

Ruang kredit setiap dealer, pola bayar, dan prediksi kas masuk.

## Isi halaman

- **KPI kas**: DSO, piutang berjalan, lewat tempo.
- **Sisa limit tiap dealer**: aman, tipis, overdue, over limit, atau cash, dengan pola bayar dan on time.
- **Exposure vs limit**: dealer yang paling dekat atau sudah melewati limit.
- **Prediksi kas masuk** dari faktur terbuka dan pola bayar dealer.

## Tutorial: menangani permintaan rilis di atas limit

1. AI Kredit membuat usulan dengan opsi: **DP 50%**, **tahan sampai lunas**, atau **rilis sebagian**.
2. Pastikan checklist keamanan terpenuhi (PO diverifikasi lewat telepon ke nomor terdaftar, alamat kirim konsisten).
3. Pemegang wewenang (CEO) memilih opsi dan menyetujui di Pusat kendali.

> GSI Orbit tidak pernah melepas barang atau mengubah limit di Accurate. Keputusan dicatat di sini; pelaksanaannya tetap di sistem resmi.
`,
  },
  {
    id: 'master',
    group: 'Tutorial per menu',
    title: 'Master data: pengguna, peran, cabang',
    icon: 'people',
    screen: 'users',
    md: `
# Master data: pengguna, peran, cabang

Menu untuk admin dan CEO.

## Pengguna

1. Buka **Pengguna** → **Tambah pengguna**.
2. Isi nama, email, kata sandi awal, **peran**, dan **cabang**.
3. Simpan. Berikan email dan kata sandi ke pengguna; minta ia login dengan akunnya sendiri.
4. Nomor WhatsApp ditautkan di halaman **Chat** (tombol **+ Nomor**), lalu dipilih pemegangnya.

> Satu orang satu akun. Ini penting agar label pengirim di chat dan jejak keputusan menyebut orang yang benar.

## Peran & akses

1. Buka **Peran & akses** → pilih atau buat peran.
2. Centang **halaman** yang boleh dibuka.
3. Pilih **cakupan data**: semua data, atau hanya milik sendiri (sales).
4. Centang **keputusan** yang boleh diambil peran ini (misalnya pengingat penagihan, SO draft, rilis kredit).
5. Simpan. Perubahan berlaku saat pengguna membuka halaman berikutnya.

## Cabang

Master cabang dipakai oleh pengguna, dealer, dan mapping gudang. Tambah atau ubah dari tabel **Cabang**.

## Mapping sales

Menghubungkan **pengguna** GSI Orbit dengan **nama sales di Accurate/BigQuery**. Satu pengguna bisa punya beberapa nama. Mapping yang benar membuat dealer, penjualan, dan laporan jatuh ke sales yang tepat.
`,
  },
  {
    id: 'pengaturan',
    group: 'Tutorial per menu',
    title: 'Pengaturan, MCP Claude & AI Trigger',
    icon: 'gear',
    screen: 'conn',
    md: `
# Pengaturan, MCP Claude & AI Trigger

## MCP Claude

Menghubungkan Claude dengan GSI Orbit. Claude membaca data dan mengusulkan; keputusan tetap di aplikasi.

- **Routine Claude**: CEO menyimpan ID routine dan token trigger dari claude.ai, lalu **Tes sekarang**. Setelah itu setiap Analisis ulang dan Analisa WhatsApp otomatis dikerjakan Claude di claude.ai tanpa membuka aplikasi apa pun.
- **Riwayat**: setiap analisa Claude (berhasil/gagal, keterangan, laporan lengkap).
- **Izin akses**: klien MCP yang terhubung dan haknya.

## API & model AI

Opsi kedua untuk analisa lewat API (Claude API), lengkap dengan model dan total biaya token. Kartu **Analisa WhatsApp otomatis**: aktif/mati, model, dan jam berjalan (WIB).

## AI Trigger

Kata kunci di pesan pelanggan yang memunculkan kartu data di Chat.

1. Lihat daftar trigger bawaan: **Tanya stok**, **Tanya harga**, **Tanya limit & tagihan**, **Status order**.
2. Ubah kata kunci, draf, dan instruksi gaya jawaban AI; aktifkan atau matikan per trigger.
3. Uji dengan kotak **Coba kalimat pelanggan** (bisa memilih dealer dan cabang).

Draf jawaban selalu ditulis AI dari angka sistem. Draf yang mengubah angka ditolak. Tidak ada yang terkirim otomatis.

## Pengaturan

- **Kebijakan orbit**: ambang lewat jadwal, sisa limit minimum, umur stok menua, margin minimum, jarak follow-up, matriks otonomi. Hanya CEO yang mengubah; setiap versi tersimpan.
- **Data**: sumber BigQuery/Accurate, jadwal sinkron, dan status impor.
- **Pilot**: mode bayangan (usulan dicatat tetapi tidak dikirim) untuk masa uji coba.
- **Kalibrasi agen**: tingkat persetujuan tiap agen dan contoh yang dipelajari.
`,
  },
  {
    id: 'rutinitas',
    group: 'Alur kerja harian',
    title: 'Rutinitas per peran',
    icon: 'cal',
    md: `
# Rutinitas harian per peran

## Sales

**Pagi (08.00)**
1. Buka **Pusat kendali** → kerjakan **Rencana hari ini** dan **Keputusan** milik Anda.
2. Cek **Jadwal order 7 hari** dan **Lewat jadwal**; sapa dealer yang H-1.

**Sepanjang hari**
3. Di **Chat**, filter **Belum dibalas**; balas yang paling lama menunggu dulu (lihat jam di samping nama).
4. Gunakan kartu **AI Trigger** untuk menjawab stok dan harga: **Pakai jawaban** → cek → kirim.
5. Sebelum menyapa dealer, buka panel lead → **Main** dan **Statistik** untuk konteks.

**Sore**
6. Pastikan tidak ada chat gantung (Report WhatsApp → Chat gantung).

## Admin

1. Pilih nomor tim Admin di bar nomor Chat.
2. Tangani filter **Belum dibalas** dan **Nomor baru**.
3. Untuk pelanggan baru: tab **Accurate** → **Kirim ke Accurate** → buat pelanggan di Accurate.
4. Pasang **label** (Pesanan baru, Menunggu pembayaran, dan seterusnya) agar tim tahu status pesanan.
5. Login dengan akun sendiri; nama Anda tampil di setiap pesan yang Anda kirim.

## Penagihan

1. Buka **Kredit · kas**: lewat tempo, overdue, prediksi kas masuk.
2. Setujui pengingat penagihan dari **AI Penagihan** di Pusat kendali (nada sudah disesuaikan pola bayar dealer).
3. Balas dari nomor Penagihan di Chat; pasang label **Sudah dibayar** setelah bukti transfer masuk.

## Gudang

1. **Cek stok**: cocokkan stok AI dengan stok real, simpan hasil hitung.
2. **Analisa stok**: kerjakan **Reorder segera** dan **Mutasi antar cabang**; laporkan dead stock.
3. Pantau grup internal di Chat untuk permintaan stok.

## Manajer & CEO

1. **Dashboard** dan **KPI utama**: order tepat jadwal, DSO, perputaran stok.
2. **Report WhatsApp**: peringkat sales dan ringkasan untuk direktur.
3. **Keputusan** yang butuh wewenang (rilis kredit, perubahan limit, harga khusus).
4. **Analisa stok**: dead stock, slow moving, dan perputaran per kategori dan cabang.
5. Mingguan: tinjau **Kebijakan** dan **Kalibrasi agen** di Pengaturan.
`,
  },
  {
    id: 'faq',
    group: 'Bantuan',
    title: 'Tanya-jawab & pemecahan masalah',
    icon: 'alert',
    md: `
# Tanya-jawab & pemecahan masalah

**Apakah AI bisa mengirim pesan ke pelanggan sendiri?**
Tidak. Semua pengiriman butuh orang yang menekan Kirim atau Setujui, dan namanya tercatat.

**Kenapa angka di GSI Orbit berbeda dengan Accurate?**
Data stok disinkron tiap 5 menit, data lain sesuai jadwal sinkron. Lihat jam sinkron di atas halaman. Untuk stok, gunakan **Cek stok** untuk melihat apakah selisihnya dari sinkron, mapping gudang, atau stok real.

**Chat pelanggan tidak muncul.**
- Pastikan nomor yang benar dipilih di bar nomor.
- Daftar hanya memuat chat 14 hari terakhir; gunakan **Cari chat** untuk chat yang lebih lama.
- Sales hanya melihat chat di nomor yang ia pegang.

**Analisis ulang tidak selesai.**
Lihat pop-up analisis. Jika menunggu Claude, buka Claude dengan tombol **Buka Claude** atau minta CEO mengisi **Routine Claude**. Tekan **Batalkan analisis** untuk menghentikannya; data dan saran sebelumnya tetap dipakai.

**Tombol Analisis ulang tidak bisa ditekan.**
Hanya satu siklus yang bisa berjalan pada satu waktu. Tunggu siklus yang sedang berjalan selesai (lihat pill Orchestrator di kanan atas).

**Label pengirim di pesan salah nama.**
Label mengikuti akun yang login. Pastikan setiap orang login dengan akunnya sendiri.

**Kartu AI Trigger tidak muncul.**
Kata kunci pesan belum ada di daftar trigger, atau trigger dimatikan. Tambahkan kata kuncinya di **AI Trigger**.

**Analisa Claude di tab Main kosong.**
Chat itu belum pernah dianalisa. Jalankan **Analisis ulang** dari tombol Orchestrator di samping nama chat.

**Saya tidak melihat sebuah menu.**
Menu mengikuti peran. Minta admin menambahkan halaman di **Peran & akses**.

## Privasi dan keamanan

- Chat pribadi antar karyawan tampil di Chat tetapi **tidak** dianalisa AI.
- Grup internal hanya dipakai untuk stok, jadwal, dan tugas.
- Nama dan nomor pelanggan disamarkan sebelum dibaca AI.
- Chat disimpan 365 hari.
- Chat yang ditandai **Obrolan pribadi** dihapus dari CRM dan pesan berikutnya tidak disimpan.
- Setiap keputusan, pengiriman, label, dan sematan tercatat di audit.
`,
  },
]

/** The chapters whose title or text holds every word of the query (case-insensitive). */
export function searchChapters(q: string, list: Chapter[] = CHAPTERS): Chapter[] {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return list
  return list.filter((c) => {
    const t = (c.title + ' ' + c.md).toLowerCase()
    return words.every((w) => t.includes(w))
  })
}
