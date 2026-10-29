# Changelog
Semua perubahan per tahap dicatat di sini oleh Claude Code (`feat(stage-NN): ...`).

## UI · Push stok: kotak pencarian di kanan · 2026-10-29
- Di tabel stok menua, kotak **Cari produk, SKU, cabang, dealer** pindah ke kanan; jumlah dan nilai stok menua di kiri. Halaman lain tidak berubah.

## Feat · Chat otomatis: Lyra membalas semua chat seperti sales · 2026-10-29
- Pilihan baru di **Chat otomatis → langkah 2**: **"Lyra membalas SEMUA chat seperti sales"**. Pada jam aktif, Lyra menyapa pelanggan dengan namanya dan menjawab sesuai isi pesan: stok, harga, status order, limit/tagihan, salam, atau janji temu.
- Angka selalu dari sistem. Balasan yang memuat angka di luar data sistem (misalnya diskon karangan) **tidak dikirim**, dan sebagai gantinya terkirim kartu produk atau template.
- Lyra tidak menjanjikan diskon, kredit, tanggal kirim, atau menerima pesanan. Untuk itu Lyra bilang sales akan menindaklanjuti.
- Balasan ditandai "— Lyra, asisten GSI (pesan otomatis)" dan chat tetap "Belum dibalas" untuk sales.
- ADR 0078 · test: `TestAutoReplyLikeSales`.

## UI · Chat otomatis: pilihan yang aktif hijau dengan centang · 2026-10-29
- Tombol hari, fakta cadangan AI Trigger, dan pilihan nomor yang dipilih sekarang **hijau dengan tanda ✓** (sebelumnya biru, tanpa arti yang jelas). Yang tidak dipilih tetap putih polos.

## Fix · Chat: mengetik tidak perlu refresh lagi · 2026-10-29
- Membuka chat langsung menaruh kursor di kotak ketik, seperti WhatsApp Web (di HP tidak, supaya keyboard tidak tiba-tiba muncul). Sebelumnya fokus tertinggal di daftar chat, jadi ketikan tidak masuk.
- Setelah menekan tombol **Kirim** dengan mouse, kursor tetap di kotak ketik. Sebelumnya fokus pindah ke tombol yang berubah menjadi tombol rekam suara, jadi ketikan berikutnya hilang.
- Mengetik huruf setelah mengklik di area pesan atau tombol lain langsung masuk ke kotak ketik (kecuali saat jendela/menu lain terbuka).
- Diuji di browser: buka chat → ketik → Enter → ketik lagi → klik Kirim → ketik lagi.

## UI · Chat otomatis: tampilan dibuat sangat sederhana · 2026-10-29
- Status besar berwarna di paling atas: **NYALA — sedang menjawab** (hijau), **NYALA — menunggu jamnya** (biru), atau **MATI** (abu-abu), dengan satu tombol besar **Nyalakan/Matikan** (minta konfirmasi dulu).
- Gambar alur 3 langkah: pelanggan chat → Lyra cari barangnya → balasan terkirim.
- Pengaturan jadi 4 langkah bernomor (Kapan? Siapa yang menjawab? Pesan untuk chat lainnya, Nomor mana?), masing-masing dengan tanda ✓ hijau atau ✗ merah.
- Jam dipilih dari daftar 08.00–18.00 per setengah jam (sebelumnya kotak jam browser bisa menampilkan 13.00 sebagai "01:00").
- Bila ada perubahan, muncul bar kuning "BELUM DISIMPAN" (merah bila ada yang salah) dengan tombol Simpan.
- Test: `autoreply.test.ts`.

## Feat · Chat otomatis dijawab Lyra, dengan halaman pengaturan · 2026-10-29
- Halaman baru **Pengaturan → Chat otomatis**: nyalakan/matikan, atur **jam aktif** per hari (dari jam berapa sampai jam berapa, 08.00–18.00 WIB), pilih nomor yang ikut, ubah pesan template, dan lihat riwayat pesan otomatis. Hanya CEO yang bisa menyimpan, dan menyimpan berarti menyetujui aturannya.
- Pada jam aktif, pertanyaan produk dijawab **Lyra** (Hermes Agent): Lyra menyebut barangnya, stok per kota dan harga DPP diambil dari sistem, sama seperti kartu Lyra di Chat. Bila Lyra tidak menemukan produk atau belum tersambung, AI Trigger menjawab; pesan lain mendapat template.
- Masih mati sampai CEO menyalakannya. Sales yang membalas dalam 10 menit terakhir tetap memegang chat.
- ADR 0077 · migrasi 0073 · test: `TestAutoReplyByLyra`, `autoreply.test.ts`.

## Access · Menu Sales, Admin, dan Super Admin · 2026-10-28
- **Sales** dan **Admin** hanya mendapat halaman operasional: Pusat kendali, Orchestrator, Chat, Orbit, Dealer, Push stok, Cek stok, dan Kredit · kas. Sales melihat dealer, piutang, dan chat miliknya sendiri. Admin melihat data semua sales dan semua chat. Pembatasan menu juga berlaku di API halaman yang dilindungi.
- **CEO/Super Admin** mendapat semua halaman dan semua data.
- Dashboard, Cek stok, AI Trigger, dan Buku manual sekarang menjadi halaman tersendiri di Peran & akses. **Peran lain** (mis. Penagihan, Gudang, dan peran buatan sendiri) tetap mendapat halaman yang mereka lihat sebelumnya.
- Pengaturan peran sebelum perubahan disimpan di `roles_backup_0065`, dan rollback migrasi mengembalikannya persis.
- Migrasi 0065 · test: `TestNormalizeAndDerive`, `TestRoleMaster`, e2e menu per peran.

## Chore · Balas otomatis: sisi server terpasang, masih mati · 2026-10-28
- Mesin "Balas otomatis saat sales istirahat" (ADR 0066) sudah terpasang di server: tabel `auto_replies` (migrasi 0066), job worker, dan API pengaturan. **Bawaannya mati.** Belum ada halaman untuk menyalakannya, jadi tidak ada pesan otomatis ke pelanggan dan tidak ada yang berubah bagi tim sales.
- Perubahan menu Sales/Admin (migrasi 0065) ditahan dulu, karena mengubah halaman yang dipakai tim sales dan menimpa pengaturan peran.
- Test: `autoreply_test.go`.

## Feat · Kartu Lyra: harga per unit bisa diedit · 2026-10-28
- Harga DPP per unit di kartu Lyra sekarang berupa kolom isian. Sales bisa mengubah harga untuk penawaran itu, lalu diskon, subtotal, total, dan pesan untuk pelanggan langsung mengikuti harga baru.
- Harga yang diedit diberi warna, dan di sebelahnya tampil "harga sistem Rp … · kembalikan" untuk kembali ke harga dari GSI Orbit. Barang yang belum punya harga di data juga bisa diisi harganya.
- Harga editan hanya berlaku untuk penawaran itu dan tidak mengubah harga di data. Pesan tetap terkirim hanya setelah dikonfirmasi.
- Test: `lyra.test.ts`.

## Feat · Stasiun Orbit: kantor kru AI dengan estafet hasil · 2026-10-28
- Dashboard astronot sekarang berupa **kantor di dalam stasiun**, seperti kantor virtual 3D untuk AI agent: dek bundar dengan jendela panorama ke luar angkasa, inti AI (Orchestrator) di tengah, dan 10 meja astronot di sekelilingnya.
- **Estafet**: astronot yang selesai menganalisa mengambil kapsul hasil, berjalan menyusuri jalur ke meja astronot berikutnya, menyerahkan hasilnya, lalu kembali ke mejanya. Urutannya dari 1 Analisa sales sampai 10 Data siap. Ini terjadi saat siklus sungguhan berjalan (🚀 AI Agent bekerja).
- Tombol baru **▶ Putar alur kerja** memutar estafet 1 → 10 sebagai tayangan, tanpa menjalankan analisis dan tanpa biaya AI. Setiap meja menampilkan hasil dari data terakhir.
- ADR 0076.

## Fix · Sistem lebih ringan: pesan baru hanya memuat ulang chat-nya sendiri · 2026-10-28
- Penyebab lambat (dari log API produksi, 1 jam): setiap pesan baru dari 9 nomor membuat setiap tab memuat ulang chat yang sedang dibuka beserta konteksnya, apa pun chat-nya. Hasilnya sekitar 6.100 kali buka chat per jam (rata-rata 509 ms, p95 1,3 detik) dan 6.100 kali muat konteks.
- Sekarang pesan baru hanya memuat ulang chat yang menerima pesan itu dan konteksnya. Daftar chat, pencarian, dan badge belum-dibaca tetap diperbarui realtime.
- Popup swal tidak memperlambat apa pun: popup itu hanya muncul setelah aksi (sukses/gagal) atau saat nomor WA logout, bukan saat data dimuat. Jadi popup itu tetap ada.
- Test: `SseProvider.test.tsx`.

## Change · Sesi login tidak pernah berakhir, kecuali Logout · 2026-10-28
- Login sekarang berlaku tanpa batas waktu dan tanpa batas idle. Pengguna tidak perlu login lagi kecuali menekan **Keluar**. Cookie diperbarui otomatis selama aplikasi dipakai.
- **Keluar** benar-benar mengakhiri sesi di server. Token sesi itu, termasuk salinannya, langsung tidak berlaku. Perangkat lain tetap login.
- Gangguan server sesaat tidak lagi melempar pengguna ke halaman login. Halaman login juga langsung meneruskan ke aplikasi bila sesi masih aktif.
- Pengguna yang sedang login tidak terputus oleh perubahan ini. Sesi lamanya otomatis dipindah ke sesi baru.
- ADR 0075 · migrasi 0072 · test: `TestSessionUnlimited`, `TestHashAndToken`.

## Feat · Reaksi per pesan seperti WhatsApp · 2026-10-28
- Arahkan kursor ke bubble, klik 🙂 di samping tombol Balas, lalu pilih 👍 ❤️ 😂 😮 😢 🙏 atau **+** untuk emoji lain. Reaksi terkirim ke WhatsApp atas nama nomor GSI chat itu dan tercatat sebagai keputusan manusia (proposal, outbox, audit). Memilih emoji yang sama membatalkannya.
- Reaksi pelanggan dan rekan dari WhatsApp sekarang tampil sebagai chip di bawah bubble (emoji dan jumlah). Reaksi Anda ditandai, dan bisa dibatalkan atau diikuti dengan klik.
- Reaksi tidak dihitung sebagai pesan: tidak memicu AI, tidak membuat chat belum dibaca, dan tidak membuka opt-out STOP.
- ADR 0074 · migrasi 0071 · test: `TestChatReactions`, bridge `events.test.ts`/`sender.test.ts`, web `reactions.test.ts`.

## Change · Lyra juga membaca chat internal · 2026-10-28
- Pertanyaan stok, harga, atau produk di chat antar karyawan (DM antar nomor GSI) dan di grup internal sekarang juga mendapat kartu Lyra, sama seperti chat pelanggan. Contoh: "cek stok 76d0t-exlpf" dari Catarina ke Granike.
- Menu bubble **Tanya Lyra** juga muncul di pesan internal. Teks tetap dimasking sebelum dikirim ke Lyra. Harga memakai harga list (tier A) karena chat internal tidak tertaut ke dealer.
- ADR 0070 (perubahan 28 Okt (3)) · test: `TestLyraProductCard`.

## Change · Di bawah bubble Chat hanya kartu Lyra · 2026-10-28
- Kartu **AI Trigger** (stok/harga dengan draf ChatGPT/Claude) tidak lagi muncul di bawah pesan pelanggan. Yang tersisa hanya kartu **Lyra**: stok per kota, harga DPP, diskon, dan kirim ke pelanggan.
- Halaman AI Trigger tetap ada. Kata kuncinya masih menjadi pemicu kartu Lyra, dan simulator "Coba kalimat" tetap jalan.
- ADR 0070 (perubahan 28 Okt (2)).

## Change · Chat Hermes Agent = jendela ke Lyra · 2026-10-28
- Pertanyaan di halaman Hermes Agent dikirim apa adanya ke Lyra, lalu jawabannya tampil di GSI Orbit. Lyra sendiri yang menyimpan percakapan, memori, tool, dan model, sama seperti di Telegram. GSI Orbit tidak lagi menambahkan prompt atau riwayatnya sendiri.
- Tidak ada lagi mode demo di chat. Selama Lyra belum tersambung, kolom ketik dimatikan. **Hapus riwayat** memulai percakapan baru di Lyra.
- Tab Sambungan: Lyra berjalan di server yang sama, jadi URL-nya `http://127.0.0.1:8642`. Langkah Tailscale dihapus.
- ADR 0067 (perubahan 28 Okt (2)) · test: `TestHermesChat`.

## Fix · Halaman Hermes Agent hanya untuk chat dengan Lyra · 2026-10-28
- Halaman Hermes Agent sekarang hanya untuk berinteraksi dengan Lyra. Isinya tab **Chat** dan tab **Sambungan** (URL dan API key API server Hermes di device Lyra). Teks chat menyebut Lyra.
- Panduan token MCP, YAML, daftar tool, aturan main, dan FAQ dihapus dari halaman ini. Materi itu sudah ada di **MCP Claude → Hermes Agent**. Kartu Sambungan menjelaskan bedanya: token MCP dipakai Lyra untuk membaca GSI Orbit, sedangkan API server dipakai GSI Orbit untuk bertanya ke Lyra.
- ADR 0067 (perubahan 28 Okt).

## Fix · Tempel sel Excel di Chat terkirim sebagai gambar tabel · 2026-10-28
- Menyalin sel dari Excel lalu menempelkannya di kolom chat sekarang menghasilkan **gambar tabel**, seperti di WhatsApp Web. Gambar tampil sebagai pratinjau lampiran, dan kolom ketik menjadi keterangannya. Sebelumnya yang masuk hanya teks selnya.
- Teks biasa, halaman web, dan dokumen Word tetap ditempel sebagai teks. Bila browser tidak menyertakan gambar dari Excel, teks sel tetap dipakai seperti sebelumnya.
- Test: `paste.test.ts`.

## Fix · Kolom ketik chat tidak bisa diklik di layar laptop kecil · 2026-10-28
- Di lebar layar 901–1100 px (laptop kecil, atau jendela browser yang tidak penuh) dengan panel Lead ringkas, panel percakapan menyempit menjadi 0 px, sehingga kolom ketik tidak bisa diklik. Penyebabnya, aturan grid untuk lebar 1280 px lebih spesifik dan menimpa aturan untuk lebar 1100 px. Masalah ini sudah ada sebelum 28 Okt.
- Panel Lead yang dibuka penuh dan melayang di atas percakapan (≤1100 px) sekarang berhenti di atas kolom ketik, jadi kolom ketik tidak lagi tertutup.
- Dicek di browser pada lebar 390, 901, 960, 1024, 1100, 1101, 1215, dan 1440 px, dengan panel Lead ringkas maupun penuh.

## Fix · Grup "hanya admin yang bisa kirim pesan" · 2026-10-28
- Di grup WhatsApp yang hanya mengizinkan admin mengirim pesan, kolom ketik di Orbit sekarang tertutup dengan tulisan "Hanya admin yang dapat mengirim pesan ke grup ini" bila nomor GSI itu bukan admin, seperti di WhatsApp.
- Sebelumnya pesan tampak terkirim di Orbit tetapi ditolak WhatsApp. Sekarang pengiriman ditolak lebih dulu, dan bila tetap ada yang masuk antrean, pesannya ditandai **gagal** dengan alasan dari WhatsApp.
- ADR 0073 · test: `TestAdminOnlyGroup`, wa-bridge `sender.test.ts`.

## Fix · Chat ganda LID: juga untuk LID yang belum dikenal perangkat · 2026-10-28
- Chat "Mas Rizal IT - GSI" masih tampil dua kali karena perangkat belum mengetahui nomor di balik ID tersembunyinya. Sekarang Orbit menanyakan ID kandidat ke WhatsApp. Kandidatnya adalah chat bernomor dengan nama yang sama di nomor GSI yang sama. Chat hanya digabung bila WhatsApp menyatakan ID itu milik nomor tersebut.

## Fix · Chat ganda satu kontak (LID) digabung · 2026-10-28
- Satu kontak bisa muncul dua kali di nomor GSI yang sama: satu chat dengan nomor, satu "Nomor baru". Penyebabnya, WhatsApp kadang mengirim pesan memakai ID tersembunyi (LID) sebelum nomornya diketahui.
- Sekarang, setiap kali daftar chat dibuka, chat LID diterjemahkan ke nomor (dari data perangkat, tanpa bertanya ke WhatsApp). Isinya digabung ke chat bernomor: pesan, bintang, label, pin, analisis, dan jumlah belum dibaca. Setelah itu chat duplikatnya dihapus.
- ADR 0072 · test: `TestMergeLIDThread`, `TestChatListMergesLIDChats`, wa-bridge `http.test.ts`.

## Fix · Nama chat memakai nama yang disimpan di kontak WhatsApp · 2026-10-28
- Daftar chat, header, panel Lead, dan profil sekarang hanya menampilkan **nama yang disimpan di kontak WhatsApp** (mis. "Mas Rizal IT - GSI"). Sebelumnya tampil gabungan dengan nama profil WhatsApp orangnya ("Mas Rizal IT - GSI - rizal fachrezi").
- Nama profil atau nama dari master data hanya dipakai bila nomor itu belum disimpan. Pencarian chat tetap menemukan semua nama.

## Fix · Grup bersama di profil tidak lagi hilang-muncul, klik grup membuka chat grupnya · 2026-10-28
- Grup bersama sebelumnya kadang kosong. Penyebabnya, anggota grup WhatsApp sering tercatat dengan ID tersembunyi (LID), dan daftar grup kadang gagal diambil ulang. Sekarang LID diterjemahkan ke nomor, daftar grup terakhir tetap dipakai bila WhatsApp tidak menjawab, semua nomor GSI ditanya bersamaan, dan grup tempat kontak pernah menulis di GSI Orbit ikut ditampilkan.
- Klik grup di profil langsung membuka chat grupnya, juga bila grup itu ada di nomor GSI lain: pilihan nomor ikut pindah. Grup dicatat lewat nomor yang memegang chat grupnya.
- Test: `TestChatPhotoAndProfile`.

## Feat · Foto profil WhatsApp dan profil kontak dengan grup bersama · 2026-10-28
- Avatar di daftar chat, header chat, dan panel Lead memakai **foto profil WhatsApp**, termasuk foto grup. Bila tidak ada foto atau foto disembunyikan, inisial tetap tampil.
- **Klik foto**: profil seperti di WhatsApp tampil. Isinya foto besar, nama, nomor, nama WhatsApp, Info, status akun bisnis, dan **grup bersama** (grup nomor GSI yang Anda pegang yang juga diikuti kontak itu). Klik grup untuk membuka chat grupnya.
- Foto disimpan sebagai cache 3 hari, dan permintaan ke WhatsApp dibatasi 120 per nomor per jam (anti-blokir).
- ADR 0071 · migrasi 0070 · test: `TestChatPhotoAndProfile`, wa-bridge `http.test.ts`.

## Fix · Lyra hanya muncul untuk pertanyaan stok, harga, produk · 2026-10-28
- Kartu Lyra tidak lagi muncul di bawah setiap foto, misalnya screenshot yang dikirim ke rekan. Pesan atau caption foto baru dibaca otomatis bila memuat kata kunci trigger AI Trigger **stok** atau **harga**, atau kata produk (`produk`, `barang`, `stok`, `harga`, `ready`, `pricelist`).
- Kartu lama yang muncul karena aturan sebelumnya disembunyikan. Pesan lain tetap bisa dikirim ke Lyra lewat menu bubble **Tanya Lyra (stok & harga)**.
- Migrasi 0069 · test: `TestAsks`, `TestLyraProductCard`.

## Fix · Label waktu balas di bawah bubble dihapus · 2026-10-28
- Label "dibalas N menit kemudian", "belum dianalisa", dan "jawaban sementara · waktu respon tetap berjalan" tidak lagi tampil di bawah bubble chat. Waktu balas cukup dilihat di kartu **Statistik** chat (Median balasan, Dibalas < 1 jam, Belum dibalas).
- Catatan Claude dari analisis WhatsApp tetap tampil di bawah bubble bila ada.

## Feat · Lyra di bawah bubble: stok per kota dan harga DPP, diskon, kirim ke pelanggan · 2026-10-28
- Pesan pelanggan yang menanyakan barang, baik tulisan maupun **foto**, dibaca oleh Hermes Agent **Lyra**. Di bawah bubble muncul kartu berisi barang yang dimaksud, stok per kota (dengan rincian gudang bila ada), dan **harga DPP** per barang sesuai tier dealer (atau harga list).
- Isi **Diskon %** di kartu: harga per barang dan total langsung dihitung ulang. Qty per barang bisa diubah (awalnya jumlah yang diminta pelanggan).
- **Kirim ke pelanggan** membuka pesan siap kirim yang bisa diedit. Pesan baru terkirim setelah Anda menekan **Kirim**, sebagai balasan bubble itu, lewat antrean anti-blokir.
- Lyra hanya memilih barang dari katalog. Stok dan harga dihitung GSI Orbit. Pesan lain bisa dikirim ke Lyra lewat menu bubble **Tanya Lyra (stok & harga)**, dan kartu bisa diminta **Baca ulang**. Tanpa Hermes tersambung, kartu berjalan dalam mode demo.
- ADR 0070 · migrasi 0068 (`hermes_product_reads`) · test: `TestLyraProductCard`, `internal/hermes` (`TestAsks`, `TestResolve`, `TestBuildCard`), `lyra.test.ts`.

## Fix · Esc benar-benar menutup chat dan panel Lead · 2026-10-27
- Sebelumnya, Esc (dan tombol kembali) mengubah alamat ke `/chat`, tetapi chat dan panel Lead tetap tampil. Penyebabnya, halaman menyimpan chat terakhir supaya tetap terbuka saat filter diganti, dan chat itu tersimpan lagi sebelum alamat selesai berubah.
- Sekarang chat yang ditutup tidak disimpan ulang, dan halaman langsung menampilkan "Pilih chat". Dicek di browser: Esc dari kotak balasan dan dari area pesan, Esc pertama membatalkan balasan, chat yang sama bisa dibuka lagi, Back browser, dan chat tetap terbuka saat filter diganti.

## Fix · Navbar tidak lagi menutupi panel Lead dan header chat · 2026-10-27
- Saat chat dibuka, navbar atas ("Chat · Pengguna") membentang di atas seluruh halaman dan menutupi header panel Lead dan header percakapan. Keduanya tampak pudar dan tombolnya (telepon, label, zoom, cari, Accurate, muat ulang Lead) tidak bisa diklik.
- Sekarang navbar hanya menumpang di atas kolom daftar chat, selebar kolom itu (termasuk lebar yang diubah dengan menarik pembatas). Dicek di 1440, 1215, dan 1024 px.

## Feat · Esc menutup chat yang terbuka (seperti WhatsApp Web) · 2026-10-27
- Tekan **Esc** untuk menutup percakapan yang sedang dibuka dan kembali ke tampilan "Pilih chat".
- Urutannya seperti WhatsApp Web. Esc pertama menutup menu, sheet, gambar besar, daftar `/` atau `@`, atau membatalkan balas/edit/file. Esc berikutnya baru menutup chat. Esc di kotak teks lain (Cari chat, catatan) tidak menutup chat.

## Fix · Customer langsung melihat centang biru saat chat dibuka · 2026-10-27
- Sebelumnya tanda terbaca baru dikirim ke WhatsApp saat sales membalas, dan balasan itu pun antre lewat penjaga anti-blokir. Akibatnya customer lama melihat pesannya "belum dibaca" walau sudah dibaca di Orbit.
- Sekarang membuka chat di Orbit langsung mengirim centang biru, seperti WhatsApp Web. Pesan yang masuk saat chat sedang terbuka juga ikut terbaca. Tetap tidak ada auto-read saat pesan masuk, dan DM internal tidak dikirimi tanda terbaca (ADR 0069).
- Bridge: `POST /sessions/{id}/read`. Test: `TestChatReadSendsBlueTicks`, bridge `http.test.ts`.

## Perf · Chat yang dibuka memuat 1 bulan terakhir · 2026-10-27
- Membuka chat kini hanya memuat pesan **30 hari terakhir**, bukan seluruh riwayat yang tersimpan. Pesan lebih lama muncul saat digulir ke atas. Chat yang sebulan terakhir sepi tetap terbuka dengan 30 pesan terakhirnya.
- Daftar chat, Cari chat, Statistik, dan Report tidak berubah. Bisa diatur lewat `CHAT_OPEN_DAYS` (bawaan 30; ADR 0068).
- Test: `TestChatOpensLastMonth`.

## Fix · Mengetik di Chat tidak lagi lambat di laptop yang lebih pelan · 2026-10-27
- Pengguna laptop ThinkPad melaporkan ketikan terasa lambat di Chat; di MacBook tidak terasa. Penyebabnya, setiap huruf di kotak balasan menggambar ulang semua bubble chat yang terbuka (bisa setahun pesan) dan seluruh daftar chat di kiri (penanda "Draf: …").
- Sekarang bubble tidak digambar ulang saat mengetik, draf disimpan setelah jeda 400 ms (tetap tersimpan saat pindah chat atau mulai edit), dan kotak "Cari chat" menampilkan huruf seketika sementara daftar menyusul.

## Fix · Pencarian chat tidak naik-turun saat mengetik · 2026-10-27
- Setiap huruf yang diketik memulai pencarian baru. Tulisan "Mencari di semua chat dan pesan…" muncul di atas daftar, mendorong chat ke bawah, lalu hilang lagi, sehingga daftar naik-turun.
- Sekarang hasil pencarian sebelumnya tetap tampil sampai hasil baru datang, dan tanda "Mencari…" pindah ke dalam kotak pencarian. Daftar chat tidak bergeser lagi.

## Fix · Pencarian chat tidak naik-turun lagi · 2026-10-27
- Saat mencari di Chat, daftar sebelumnya melompat naik-turun setiap ada pesan baru masuk. Penyebabnya, hasil pencarian dimuat ulang di latar belakang dan tulisan "Mencari di semua chat dan pesan…" muncul lalu hilang lagi, sehingga daftar terdorong.
- Sekarang tulisan itu hanya tampil saat pencarian pertama. Pembaruan di latar belakang tidak lagi menggeser daftar.

## Fix · Laporan tetap menghitung chat dari nomor yang logout · 2026-10-27
- Kunci riwayat chat untuk nomor WhatsApp yang logout kini hanya berlaku di **Chat** (daftar, isi percakapan, pencarian, label). **Report WhatsApp** dan **Analisa WhatsApp** tetap menghitung chat lama dari nomor itu, dengan batas akses per pengguna yang sama seperti biasa.
- Test: `TestWAReport` (laporan CEO dan sales tetap lengkap saat nomor logout).

## Fix · WhatsApp logout: riwayat chat dikunci untuk semua dan pop-up tautkan ulang · 2026-10-27
- Nomor WhatsApp yang logout dari perangkat tertaut **tidak menampilkan riwayat chat untuk siapa pun**, termasuk CEO/admin. Daftar chat, isi percakapan, pencarian, dan label disembunyikan sampai nomor ditautkan ulang. Nomor yang hanya sedang menyambung ulang tidak dikunci.
- Begitu sebuah nomor logout, muncul **pop-up** "WhatsApp logout — nomor harus ditautkan ulang" dengan tombol **Tautkan ulang** (scan QR) atau **Nanti**. Pop-up muncul sekali per kejadian logout per sesi browser, sedangkan banner merah tetap tampil sampai nomor tersambung lagi.
- Status nomor kini membawa `state_since` (waktu logout yang sebenarnya).
- Test: `TestChatLockedWhileNumberLoggedOut` (CEO ikut terkunci).

## Feat · Chat dengan Hermes Agent seperti di Telegram · 2026-10-27
- Halaman **Hermes Agent** sekarang dibuka di tab **Chat**: gelembung pesan seperti Telegram, "sedang mengetik…" selama Hermes bekerja, jawaban berformat (tabel/daftar), pembatas tanggal, tombol salin, contoh pertanyaan, dan Hapus riwayat. Riwayat chat milik masing-masing pengguna.
- Tab **Panduan & koneksi** berisi panduan lama, ditambah kartu **Chat Hermes di halaman ini**. Di kartu itu CEO mengisi URL API server Hermes dan `API_SERVER_KEY`. Isian diperiksa ke Hermes lalu disimpan terenkripsi. Selama Hermes belum tersambung, chat berjalan dalam mode demo.
- API: `GET/POST/DELETE /api/mcp/hermes/chat`, `PUT/DELETE /api/mcp/hermes/config`. Migrasi 0067 (`hermes_messages`). Migrasi sekarang boleh diterapkan tidak berurutan. Lihat ADR 0067.
- Test: `TestHermesChat`.

## Fix · Cek stok sama dengan data sumber · 2026-10-27
- **Masalah**: stok di Cek stok kadang berbeda dengan database. Penyebab yang ditemukan di kode: (1) dua baris sumber untuk barang × gudang yang sama saling menimpa — hanya baris terakhir yang terhitung; (2) baris tanpa nama barang dibuang diam-diam sehingga stoknya hilang; (3) baris stok dari unggahan CSV lama tidak pernah terhapus saat stok ditarik dari BigQuery, jadi ikut terjumlah; (4) bila langkah Apply lain gagal (faktur, pelanggan), stok tidak diperbarui padahal baris sumber sudah baru — Cek stok memperlihatkan "Orbit" ≠ "sumber"; (5) tabel Cek stok menjumlah per kode barang dengan huruf besar/kecil berbeda dari detail produk.
- **Perbaikan**: baris yang sama dijumlah (qty, nilai, terjual 90 hari); barang tanpa nama memakai kodenya; satu tarikan stok mengganti stok dari semua sumber; stok tetap dibangun ulang walau langkah Apply lain gagal (kecepatan jual yang lama dipertahankan); tabel dan detail menjumlah dengan aturan yang sama.
- Test: `TestStockMatchesSource`.

## Feat · Balasan cepat di Chat (seperti WhatsApp Business) · 2026-10-24
- Simpan teks yang sering dikirim dengan pintasan, mis. `/rekening`, `/alamat`, `/salam`. Ketik **/** di kolom chat: daftar yang cocok muncul (paling sering dipakai di atas). Pilih dengan ↑ ↓ + Enter/Tab atau klik, maka teks mengisi kolom dan Anda tetap menekan **Kirim**.
- Kelola lewat 📎 → **Balasan cepat**, atau *Kelola balasan cepat* di bawah daftar: tambah, ubah, hapus. Milik tim: siapa pun bisa menambah; pembuat atau admin yang bisa mengubah/menghapus. Pintasan unik, maksimal 200 per tim.
- Migrasi 0063 `quick_replies`, API `/chat/quick-replies`, test Go + vitest + e2e Playwright (`e2e/quick-replies.spec.ts`). ADR 0065.

## Fix · Chat grup (dan semua chat) tidak lagi tertahan "mengirim…" · 2026-10-24
- Kasus: Sam mengirim di grup dari Orbit, tetapi pesan hanya menampilkan "mengirim…" dan tidak sampai ke HP. Penyebabnya bukan grup: nomor 6289506401111 mengirim ±22 balasan dalam 20 menit, lalu penjaga anti-blokir menahan **semua** kiriman berikutnya dari nomor itu (grup dan pribadi) dengan "Batas 20 pesan/jam". Batas itu dibuat untuk follow-up, bukan untuk orang yang sedang chat.
- Kini balasan yang diketik di Chat dikirim sebagai **live**: tidak ditahan batas 20/jam, 120/hari, 6/chat/jam, masa pemanasan, maupun jam tenang. Yang tetap berlaku: opt-out, kontak dingin, broadcast teks panjang, jeda 20 detik per chat, dan "mengetik…". Ada plafon pengaman 100 pesan/jam per nomor (`BRIDGE_MAX_LIVE_PER_HOUR`). Batas follow-up hanya menghitung kiriman non-live.
- Bubble yang masih tertahan menampilkan alasannya ("tertahan · Batas … · terkirim otomatis begitu diizinkan"), bukan "mengirim…" tanpa keterangan. ADR 0064.
- Test panggilan WhatsApp (`calls.test.ts`) tidak lagi gagal setelah tanggal pembuatannya: tracker berjalan pada waktu event.

## Fix · Grup WhatsApp baru langsung muncul di Chat (baris sistem grup) · 2026-10-23
- Kasus: Sam membuat grup WhatsApp baru dan "info terakhir grup" tidak masuk ke sistem. Grup baru tidak punya thread di Orbit sampai seseorang mengetik pesan teks pertama: bridge membuang stub grup (dibuat, ganti nama, anggota ditambah/keluar) karena tidak berisi pesan.
- Kini stub grup live menjadi **baris sistem** di chat grup, seperti WhatsApp: "Anda membuat grup "X"", "Rina mengubah nama grup menjadi "Y"", "Anda menambahkan Budi, +6285…", "Budi keluar", "… sekarang admin", foto/deskripsi grup diubah. Chat grup ada di Orbit sejak grup dibuat, lewat jalur realtime yang sama (SSE), dengan nama dan anggotanya.
- Baris sistem bukan pesan siapa pun: tidak unread, tidak masuk *Belum dibalas* dan tidak menjawabnya, tidak dibaca analisa, tidak memicu AI Trigger. Ditampilkan di tengah tanpa bubble. Stub dari history sync tidak diteruskan. ADR 0063.

## Fix · WhatsApp yang logout di HP tampil logout di Orbit · 2026-10-23
- Kasus: perangkat tertaut nomor Granike Monica dihapus WhatsApp (10.33 WIB, `device_removed`); setelah restart worker nomor itu tampil **Belum dipasangkan** dan alert sempat berbunyi "✅ Pulih" padahal masih logout, sehingga chat bos ke nomor itu tidak masuk tanpa ada yang tahu.
- Kini nomor yang pernah tertaut dan tidak lagi dipegang bridge tetap **Keluar dari perangkat** (`logged_out`) sampai ditautkan ulang — juga setelah restart; nomor yang dilepas sendiri di Orbit tetap *Belum dipasangkan*. Worker menunggu bridge siap sebelum mengumumkan status (tidak ada lagi "unpaired" sesaat saat deploy), jadi tidak ada "Pulih" palsu.
- Di Chat, nomor yang dibuka dan tidak tersambung menampilkan banner merah "WhatsApp <nama> keluar dari perangkat tertaut sejak … — chat baru tidak masuk ke Orbit" dengan tombol **Tautkan ulang**.

## Feature · Panggilan WhatsApp tercatat di chat; tombol Telepon · 2026-10-23
- Panggilan WhatsApp ke nomor GSI kini muncul sebagai bubble di chat: **Panggilan suara tak terjawab** (merah, masuk *Belum dibalas* dan unread), **Panggilan suara masuk · 3:12** (hijau, dihitung sudah dilayani), **ditolak**, termasuk panggilan video. Satu bubble per panggilan, dicatat saat panggilan selesai; GSI Orbit tidak pernah mengangkat atau menolak panggilan — HP tetap yang berdering.
- Tombol **Telepon** (ikon telepon) di header chat 1-on-1: menelepon nomor itu dari perangkat Anda (`tel:`).
- Belum bisa: menelepon/bicara langsung dari browser — WhatsApp lewat perangkat tertaut tidak membawa suara; butuh layanan berbayar (WhatsApp Cloud API Calling atau VoIP), menunggu keputusan. Panggilan grup dan panggilan keluar dari HP belum tercatat. ADR 0062.

## Feature · Cadangan chat dan Ekspor chat dalam format WhatsApp · 2026-10-23
- **Ekspor chat**: menu ⌄ chat di daftar Chat → *Ekspor chat* menyimpan `Chat WhatsApp dengan <nama>.txt` — format persis Ekspor chat WhatsApp di HP (`23/10/26 14.05 - Nama: pesan`, `<Media tidak disertakan>`, `Pesan ini telah dihapus`, `<Pesan ini diedit>`). API `GET /api/chat/threads/{id}/export`, diaudit.
- **Cadangan chat** (Pengaturan, CEO/admin): setiap malam 02.15 WIB — sebelum chat di atas 90 hari dibersihkan — satu zip per bulan berisi satu .txt per chat; bulan yang sudah lewat tidak ditulis ulang, jadi chat lama tetap terbaca setelah dihapus dari database. Disimpan 12 bulan (`CHAT_BACKUP_KEEP_MONTHS`) di `CHAT_BACKUP_DIR`. Tombol **Cadangkan** dan **Unduh** per bulan.
- Enteng: satu query per bulan, pesan dialirkan baris demi baris langsung ke zip, tanpa media, tanpa tabel baru — diukur 150 ribu pesan / 3.000 chat → zip 2,3 MB dalam 1,6 detik, memori ±10 MB. Penyamaran kredensial (`password: ••••••`) kini melewati regex untuk pesan tanpa kata kunci (52 → 0,8 µs per pesan), jadi Chat juga lebih ringan. Chat yang ditandai Obrolan pribadi dan penghapusan UU PDP ikut membuang chat itu dari semua cadangan. ADR 0061.

## Change · Chat disimpan 90 hari; "Muat pesan lama dari HP" seperti WhatsApp Web · 2026-10-22
- Masa simpan chat **365 → 90 hari** (migrasi 0062, versi kebijakan baru). Pembersihan malam (02.30 WIB, setelah backup 02.00) menghapus pesan lebih tua dari 90 hari — pada malam pertama ±636 ribu pesan (68%); tetap ada di backup terenkripsi 30 hari dan di HP masing-masing.
- Baru: di puncak percakapan tombol **Muat pesan lama dari HP** — GSI Orbit meminta 50 pesan sebelumnya ke HP pemegang nomor (seperti WhatsApp Web menggulir ke atas), lalu menampilkannya di atas; bisa diulang sampai 1 tahun ke belakang. API `POST /api/chat/threads/{id}/history`. ADR 0060.

## Fix · Chat realtime: koneksi yang mati diam-diam tersambung ulang dan mengejar pesan yang terlewat · 2026-10-22
- Diukur di produksi: server menyimpan pesan WhatsApp dalam **median 0,7 detik** (95% ≤ 1,1 detik, 0 pesan > 60 detik dari 3.299 pesan/24 jam) — yang tertinggal adalah tab browser: koneksi realtime (SSE) bisa mati tanpa error (laptop tidur, Wi-Fi berganti, koneksi diam diputus), pesan selama putus tidak pernah diambil, dan tidak ada pengganti.
- Kini: **watchdog** atas heartbeat server (25 detik) — 70 detik tanpa sinyal = sambung ulang; setiap tersambung ulang, kembali ke tab (> 10 detik), atau jaringan kembali → chat, daftar chat, status nomor, dan usulan langsung dimuat ulang; selama terputus, chat diperbarui **tiap 10 detik** dan daftar chat menampilkan "Menyambung ulang ke server…". Antrean event per tab di server dinaikkan (32 → 256) agar lonjakan pesan grup tidak membuang event.
- Catatan: pesan grup yang gagal didekripsi WhatsApp (Bad MAC, beberapa per jam di log bridge) diminta ulang otomatis ke pengirim oleh Baileys; yang tidak pernah dikirim ulang HP pengirim memang tidak bisa diambil.

## Fix · Chat: draf balasan tidak hilang saat pindah chat · 2026-10-22
- Teks yang diketik di kolom balasan kini tersimpan sebagai **draf per chat** (seperti WhatsApp): tetap ada saat membuka chat lain dan setelah halaman dimuat ulang, sampai dikirim atau dikosongkan. Daftar chat menampilkan **Draf: …** (hijau) untuk chat yang punya draf. Mengedit pesan tidak menimpa draf — setelah edit selesai atau dibatalkan, draf kembali. Disimpan per browser.

## Fix · Chat: membaca chat lama tidak lagi terlempar ke bawah saat pesan baru masuk · 2026-10-22
- Sebelumnya setiap pesan baru (sering di grup) menggulir percakapan ke paling bawah walau sedang membaca pesan di atas. Kini seperti WhatsApp: bila sedang di bawah, pesan baru tetap diikuti; bila sedang menggulir ke atas, posisi baca **tetap** dan muncul tombol **↓ N pesan baru** untuk turun ke pesan terbaru. Mengirim pesan sendiri membawa ke bawah; membuka chat tetap mulai dari pesan terakhir.

## Feat · Chat: lebar daftar chat bisa ditarik seperti VS Code · 2026-10-22
- Di garis antara daftar chat dan percakapan kursor berubah menjadi ↔; tarik untuk memperlebar atau mempersempit daftar (240–640 px, percakapan tetap mendapat minimal 420 px). Lebar diingat per browser; klik dua kali garis untuk kembali ke lebar awal; bisa juga dengan ← → saat garis difokus. Tidak tampil di layar HP.

## Feat · Chat: Pesan berbintang dan filter Berbintang · 2026-10-22
- Menu **⌄** di bubble punya **Beri bintang / Hapus bintang** (seperti WhatsApp). Bubble berbintang bertanda ★. Bintang bersifat **pribadi per pengguna** (berbeda dengan Sematkan yang untuk tim) dan tidak dikirim ke WhatsApp.
- Filter baru **Berbintang** (ikon ★ dengan jumlahnya) di daftar chat: semua pesan berbintang Anda di nomor/tim yang dipilih, terbaru dulu, bisa dicari; klik membuka chat tepat di pesannya. Hanya chat yang boleh Anda baca yang tampil; pesan yang dihapus keluar dari daftar.
- API: `PUT /api/chat/threads/{id}/messages/{mid}/star {starred}`, `GET /api/chat/starred?account=&team=`; `starred` pada pesan. Migrasi 0061 (`chat_message_stars`). Penanda bintang tidak pernah menghalangi chat terbuka.

## Feat · Tanya Orchestrator: pertanyaan stok membuka tabel Cek stok yang terfilter + kartu Hasil AI · 2026-10-22
- Pertanyaan stok di kotak Orchestrator (mis. **"stok jakarta"**) kini langsung membuka **Cek stok**: tabel **Data stok** terfilter cabang/barang yang disebut, kartu **Analisa stok** membuka filter yang sesuai (Per cabang dengan Jakarta terpilih dan daftar barangnya; Fast moving, Dead stock, Reorder, Mutasi, Umur stok, Per kategori), dan di atasnya kartu **Hasil AI · Orchestrator** berisi pertanyaan dan jawabannya (× menutup dan melepas filter). Alamatnya bisa dibagikan (`/stok/cek?tanya=…&cabang=…&cari=…&filter=…`).
- Pertanyaan push/bundle tetap menjawab dengan dealer kandidat di pop-up. Dock Orchestrator menutup setelah pertanyaan dikirim agar tidak menutupi tabel; di halaman Cek stok, "Layar ini" kini **push stok** (sebelumnya "semua").
- API `POST /api/ask`: `show` {page, branch, search, view} untuk jawaban stok.

## Change · Orchestrator: pilihan mesin MCP Claude atau ChatGPT · 2026-10-22
- Pilihan **Dianalisis oleh** di dock Orchestrator, halaman Orchestrator, dan Stasiun Orbit (Dashboard) kini **MCP Claude** atau **ChatGPT**, masing-masing dengan modelnya (ChatGPT: GPT-5 mini, GPT-5, GPT-4.1, GPT-4.1 mini, o4-mini) dan effort. Pilihan **Sistem (tanpa AI)** dihapus; pilihan lama "Sistem" terbaca sebagai MCP Claude.
- **ChatGPT** berjalan lewat **ChatGPT API di server** — langsung, tanpa membuka apa pun — memakai kunci OpenAI yang sama dengan AI Trigger; biaya tercatat dan dibatasi anggaran harian di API & model AI. Tanpa kunci, tombol memberi tahu dan tidak ada analisa yang diantre.
- ChatGPT tetap tidak pernah mengambil alih sendiri (opsi kedua API, Analisa WhatsApp otomatis tetap tanpa ChatGPT). ADR 0059 (mencabut sebagian ADR 0048). Buku manual diperbarui.

## Fix · Tanya Orchestrator: pertanyaan stok dijawab sesuai pertanyaan, bukan daftar dealer · 2026-10-22
- Sebelumnya semua pertanyaan berisi "stok" dijawab dengan template stok menua + dealer kandidat push. Kini jawabannya mengikuti maksud: **nama/kode barang** → stok per cabang, kecepatan jual, cukup berapa hari, kelas; **fast moving**, **dead stock**, **slow moving**, **reorder/habis/kritis**, **mutasi**, **umur stok** → daftar itu; **nama cabang** atau **kategori** → ringkasannya; barang tidak dikenal → "tidak ditemukan"; pertanyaan stok umum → ringkasan stok. Hanya pertanyaan **push / didorong / bundle** yang menyebut dealer kandidat. Angka sama dengan Analisa stok di Cek stok.
- Kotak perintah tidak lagi membuka halaman dealer bila kalimatnya menanyakan topik (stok, barang, tagihan, jadwal, …) walau kata itu ada di nama dealer.
- Buku manual (bab Orchestrator) memuat contoh pertanyaan stok.

## Change · Cek stok: Analisa stok jadi satu kartu dengan filter · 2026-10-22
- Kartu Analisa stok tidak lagi berupa banyak panel. Satu kartu dengan **filter**: **Ringkasan** (komposisi FSN, ABC, insight) · **Fast moving** · **Umur stok** · **Per cabang** · **Per kategori** · **Reorder segera** · **Dead stock** · **Slow moving** · **Mutasi antar cabang** — tiap chip dengan jumlahnya.
- Umur stok, per cabang, dan per kategori: klik batang grafik (mis. 91–180 hari, Jakarta, Modul LED) untuk menampilkan barangnya di tabel; tabel bisa dicari dan ditambah halaman; klik baris membuka cek barang. Kotak KPI Dead stock, Stok menua, dan Reorder segera membuka filternya.
- API `GET /api/stock/analysis` kini menyertakan semua barang (`products`) dan semua peluang mutasi, bukan hanya 10 teratas.

## Feat · Buku manual: menu baru dengan tutorial dan penjelasan sistem · 2026-10-22
- Menu **Buku manual** (grup Pengaturan, juga di Jelajahi fitur Dashboard) terbuka untuk **semua pengguna** tanpa perlu diatur di Peran & akses.
- 18 bab dalam 4 bagian: **Mengenal sistem** (selamat datang, cara kerja sistem & enam agen, istilah & rumus, masuk & navigasi), **Tutorial per menu** (Pusat kendali, Dashboard, Orchestrator & Analisis ulang, Chat WhatsApp, Report WhatsApp, Orbit/Segmen/Peta relasi, Dealer, Push stok, Cek stok & Analisa stok, Kredit · kas, Master data, Pengaturan/MCP Claude/AI Trigger), **Alur kerja harian** per peran (Sales, Admin, Penagihan, Gudang, Manajer & CEO), dan **Bantuan** (tanya-jawab, pemecahan masalah, privasi).
- Daftar isi dengan **Cari di manual**, tombol **Buka halaman …** ke menu yang dijelaskan, navigasi bab sebelumnya/berikutnya, tautan per bab (`/manual?bab=chat`), dan **Cetak / simpan PDF** seluruh buku. Di HP, bab dipilih dari dropdown.

## Feat · Cek stok: Analisa stok (fast / slow moving, dead stock, perputaran, reorder) dengan grafik · 2026-10-22
- Kartu **Analisa stok** di atas Data stok, dari sudut pandang owner, per semua cabang atau satu cabang:
  - KPI: nilai stok, perkiraan penjualan per bulan, **perputaran stok** (nilai stok ÷ HPP harian, target ≤ 40 hari — glosarium), dead stock, stok menua (> 90 hari), reorder segera (habis < 10 hari).
  - **Insight** otomatis: modal tertahan di dead/slow stock, fast moving yang harus dijaga, barang yang akan habis, ABC, kategori paling lambat berputar, peluang mutasi antar cabang.
  - Grafik: komposisi stok **FSN** (Fast moving · Normal · Slow moving · Dead stock) dengan nilai & porsi penjualan, umur stok, **analisa ABC** (nilai penjualan), nilai stok per kategori dan per cabang dengan hari perputaran.
  - Daftar: Fast moving, **Reorder segera** (saran unit untuk 30 hari), Dead stock terbesar, Slow moving, **Mutasi antar cabang** (barang diam di satu cabang yang dibutuhkan cabang lain). Klik barang membuka cek barangnya.
- Semua angka dihitung dari stok dan penjualan 90 hari (`internal/stockanalysis`, diuji); warna grafik divalidasi untuk buta warna, terang & gelap. API: `GET /api/stock/analysis?branch=`.

## Feat · Chat: label admin pengirim di setiap bubble · 2026-10-22
- Satu nomor sales biasanya dipegang beberapa admin (mis. nomor Granike: Tantri, Sekar, …). Setiap bubble keluar kini punya **label kecil nama pengirim** di atasnya (inisial + nama), diambil dari **user yang login** saat pesan dikirim dari GSI Orbit — balas, file/foto, suara, polling, kontak, teruskan, maupun persetujuan usulan. Pesan yang diketik langsung di HP berlabel **dari HP**.
- Label kini tampil untuk **semua yang bisa membuka chat itu** (sebelumnya hanya admin/manajer yang melihat semua chat), dan pindah dari baris jam ke atas bubble agar mudah dibaca.

## Feat · Chat: Statistik lebih jelas, detail, dan penuh insight · 2026-10-22
- Tab **Statistik** di panel lead kini dihitung dari **seluruh riwayat** chat/grup (bukan hanya pesan yang dimuat):
  - **Insight** otomatis: pesan belum dibalas, kecepatan balasan tim, tren 30 hari, pelanggan lama tidak menghubungi, hari & jam paling aktif (waktu terbaik menghubungi), porsi pesan di luar jam kerja, siapa yang memulai percakapan, barang dan permintaan terbanyak, anggota grup paling aktif.
  - **Ringkasan**: total pesan (masuk/keluar), 30 hari terakhir dengan tren, median balasan & 10% terlambat, % dibalas < 1 jam, yang belum dibalas, hari aktif sejak pesan pertama, jumlah media.
  - Grafik: aktivitas 6 bulan (masuk vs keluar), jam pelanggan menulis (WIB), hari dalam seminggu, siapa memulai percakapan; daftar barang paling sering disebut (frekuensi & unit), permintaan pelanggan, anggota pelanggan paling aktif di grup. Data dealer tetap di bawahnya.
- Semua angka dihitung sistem (`internal/chatstats`, rumus diuji); permintaan & barang dari bacaan Analisa WhatsApp. API: `GET /api/chat/threads/{id}/stats`.

## Fix · Chat: Analisa Claude di tab Main menampilkan hasil Orchestrator chat/grup · 2026-10-22
- Kartu **Analisa Claude** di tab Main kosong ("Belum ada analisa") padahal Claude sudah menganalisa grup/chat: hasil **Analisis ulang chat ini** hanya tersimpan di jawaban MCP Claude, dan catatan per pesan cuma dihitung. Kini garis waktunya mengumpulkan semua yang ditulis Claude lewat Orchestrator, terlama dulu dan tanpa duplikat: ringkasan chat tiap siklus, analisa siklus khusus chat ini, dan catatan Claude per pesan pelanggan.
- Paling atas tampil **Bacaan Claude terakhir**; tiap baris dengan permintaan pelanggan (mis. Tanya stok), barang, siklus, tanggal, dan tingkat yakin.
- Kartu langsung diperbarui saat Claude mengirim Analisa WhatsApp, tanpa menunggu siklus selesai.

## Fix · Orchestrator: tampilan analisis besar di dock dihapus, progres ringkas di panel pertama · 2026-10-22
- Setelah **Analisis ulang**, panel Orchestrator tidak lagi berganti ke tampilan analisis besar (enam kotak tahap). Panel tetap seperti biasa; di bawah garis tahap muncul blok kecil: tahap yang berjalan + durasi, agen yang sudah selesai, info Claude (routine / **Buka Claude**), dan **Batalkan analisis** — lalu satu baris hasil (berhasil / sebagian / gagal, siklus, durasi, saran diperbarui).
- Tutup panel = analisis tetap jalan di belakang layar; hasilnya muncul sendiri saat selesai.

## Fix · Orchestrator: satu pop-up saat Analisis ulang · 2026-10-22
- Panel Orchestrator (mesin, model, effort, **Analisis ulang**) dan pop-up progres analisis digabung jadi satu: setelah Analisis ulang ditekan, panel yang sama berubah menampilkan progres (tahap, agen, Claude/routine, Batalkan analisis) lalu hasilnya (berhasil / sebagian / gagal) — tidak ada pop-up kedua. **Perkecil · jalan di belakang layar** menutup panel, analisis tetap berjalan dan hasilnya muncul sebagai notifikasi; **Analisis lagi** kembali ke pilihan mesin. Berlaku di header Chat dan dock Orchestrator di halaman lain.

## Feat · Chat: Analisa Claude dari awal sampai akhir di tab Main · 2026-10-22
- Panel lead tab **Main** punya kartu **Analisa Claude · dari awal sampai akhir**, hanya berisi hasil Claude lewat Orchestrator:
  - **Overview perusahaan**: Analisa Dealer terbaru (Aman / Perlu perhatian / Bermasalah, ringkasan, alasan).
  - **Perjalanan percakapan / grup**: sejak pesan pertama sampai terakhir (jumlah pesan, berapa dicatat Claude) dan garis waktu setiap ringkasan Analisa WhatsApp, terlama dulu, dengan permintaan dan barangnya. Grup pelanggan membaca thread semua nomor GSI di grup itu.
  - **Kerjasama**: usulan Claude untuk dealer (siklus via MCP) dan keputusan tim (menunggu, disetujui, ditolak, dijalankan, oleh siapa).
- Setiap baris membawa provenance: nomor siklus, tanggal, tingkat yakin. Grup internal tidak ditampilkan (tidak dianalisa). Kartu diperbarui saat siklus Orchestrator selesai.
- API: `GET /api/chat/threads/{id}/ai-overview`.

## Feat · Chat: tombol Ringkas header · 2026-10-22
- Header percakapan punya tombol **Ringkas header** (ikon panah, di samping ikon cari). Saat aktif header menjadi satu baris tipis: nama chat, tombol Orchestrator (ikon saja), dan ikon aksi; avatar, baris nomor, pill Analisa WhatsApp, dan label tombol disembunyikan agar ruang pesan lebih luas. Klik lagi untuk header lengkap; pilihan diingat di browser.

## Fix · Chat: Cari chat mencari isi pesan di semua chat, seperti WhatsApp · 2026-10-22
- Sebelumnya **Cari chat** hanya mencocokkan nama, nomor, dan pesan terakhir tiap chat — kata di pesan lain tidak ketemu. Kini hasilnya dua bagian seperti WhatsApp: **Chat** (nama/nomor) dan **Pesan** (isi semua pesan tersimpan, terbaru dulu, kata yang dicari ditandai). Klik hasil pesan membuka chat itu dan langsung lompat ke pesannya, termasuk pesan lama di luar 14 hari.
- Pencarian ke server mulai dari 2 huruf (sebelumnya 3). Sales hanya menemukan pesan di chat nomor yang boleh dibacanya; filter nomor/tim tetap berlaku.
- API: `GET /api/chat/messages/search?q=&account=&team=`; migrasi 0060 menambah index trigram pada isi pesan agar pencarian tetap cepat.

## Feat · Analisis ulang: game dihapus, pop-up bisa diperkecil · 2026-10-22
- Game **Lompat!** saat Orchestrator menganalisa dihapus (komponen, logika, test, CSS) — di CRM waktu adalah segalanya.
- **Analisis ulang** tetap membuka pop-up seperti sebelumnya, kini dengan tombol **Perkecil · jalan di belakang layar**: pop-up menjadi bar kecil di pojok kiri bawah (tahap yang berjalan + durasi), analisis tetap berjalan dan Anda bisa lanjut bekerja. Klik bar untuk membuka pop-up lagi; saat selesai, hasilnya (berhasil / sebagian / gagal) muncul sendiri.

## Feat · Dashboard: Stasiun Orbit jadi 10 ruangan kru AI + kartu Hasil overview AI · 2026-10-21
- **Stasiun Orbit** tidak lagi menampilkan data pelanggan: planet dealer, nama dealer, dan komet "Nomor baru · nama" dihapus. Gantinya stasiun luar angkasa dengan **10 ruangan berkubah kaca** di sekeliling Inti AI, satu astronot per ruangan di konsol dengan hologram kerjanya: 1 Analisa sales, 2 Analisa WhatsApp, 3 Analisa dealer, 4 Eksekusi order, 5 Follow-up, 6 Cek stok, 7 Cek kredit, 8 Cek penagihan, 9 Prospek, 10 **Data siap**.
- Saat siklus Orchestrator berjalan, ruangan bekerja berurutan: 1–3 di tahap Ingest/Analisis, 4–9 mengikuti agennya (antri, menganalisa dengan sinar ke Inti AI, selesai, gagal), hasilnya terbang ke ruangan Data siap. Papan nama ruangan hanya berisi tugas dan angka; klik ruangan membuka layar terkait.
- Kartu baru **Hasil overview AI** di bawah stasiun: 10 kotak hasil per astronot (omzet bulan ini, chat belum dibaca, dealer At risk, jadwal order, stok menua/kritis, limit tipis, lewat tempo, nomor baru, usulan menunggu) plus status langsung saat siklus berjalan dan jumlah saran dari siklus terakhir.

## Feat · AI Trigger: Claude mencari barang di pesan; beberapa barang dan jumlah diminta · 2026-10-21
- Barang di pesan pelanggan kini dicari **Claude** dari kandidat katalog stok, bukan pencocokan kata (ADR 0058). Semua barang di satu pesan terbaca (mis. "H8c 2mp : 8 … sekalian X5S-8W 1 unit" → dua barang), dengan jumlah yang diminta.
- Kartu stok: satu baris per barang (stok, harga, cabang lain) dan **cukup / kurang** terhadap jumlah diminta ("Kurang · 5 dari 8 diminta" + saran mutasi). Angka tetap dihitung sistem; ChatGPT/Claude menulis draf yang menjawab semua barang.
- Pesan baru: kartu menampilkan "Claude mencari barang…" sampai Claude selesai (tanpa tebakan). Bacaan disimpan per pesan (`reply_ai_matches`, migrasi 0059); pesan lama dibaca ulang sekali saat chat dibuka. Tanpa kunci Claude kartu memakai kata kunci dan memberi tahu.
- *Coba kalimat* di halaman AI Trigger juga memakai Claude.

## Feat · Cek stok: harga tampil di tabel dan modal · 2026-10-21
- Tabel **Data stok** punya kolom **Harga** (harga list tier A dari daftar harga yang sama dengan kartu Cek harga; bisa diurutkan). Modal detail punya kotak **Harga (tier A)** dengan harga tier lain di bawahnya. Barang tanpa harga menampilkan "—" / "belum ada di daftar harga".
- API: `price` di `GET /api/stock/products` dan `GET /api/stock/check`.

## Feat · Cek stok: detail barang dalam modal · 2026-10-21
- Klik barang di tabel **Data stok** membuka detailnya dalam **modal** lebar di atas tabel (jawaban AI Trigger, cocokkan per cabang, data mentah BigQuery, riwayat cek), bukan lagi di bawah tabel. Tutup dengan ×, Esc, atau klik di luar; bila stok real sudah diisi tetapi belum disimpan, ada konfirmasi dulu. Link `?sku=` tetap membuka modal barang itu.
- "Cek stok terakhir" selalu tampil di bawah tabel.

## Feat · AI Trigger: kartu stok menampilkan harga · 2026-10-21
- Kartu **Cek stok** di Chat kini juga menampilkan **Harga** dan **Dasar**-nya (harga tier dealer yang tertaut, atau harga list tier A) — angka yang sama dengan kartu Cek harga. Barang tanpa harga di daftar harga tidak menampilkan harga.
- Placeholder `{harga}` tersedia untuk template stok, dan harga ikut menjadi fakta yang boleh dipakai ChatGPT/Claude di draf.
- Jawaban AI Trigger di halaman Cek stok ikut menampilkan harga.

## Fix · AI Trigger: jawaban di Chat wajib dari ChatGPT atau Claude, tidak pernah template · 2026-10-21
- Kartu AI Trigger hanya menampilkan draf tulisan ChatGPT/Claude untuk fakta saat ini; draf template tidak lagi tampil atau bisa dipakai (ADR 0057).
- Bila AI pilihan gagal (galat, atau draf mengubah angka), AI lainnya mencoba. Bila stok/fakta berubah setelah draf ditulis, draf ditulis ulang otomatis (job `reply.ai` per pesan + fakta).
- Tanpa draf AI kartu menampilkan "ChatGPT / Claude sedang menulis…" atau alasannya, dengan tombol ChatGPT dan Claude.
- Semua trigger wajib ditulis AI (migrasi 0058, `check (ai_phrase)`); halaman AI Trigger tidak lagi punya pilihan mematikan AI, dan *Coba kalimat* selalu memakai AI.

## Feat · Cek stok: datatable semua stok dengan pencarian & paginasi · 2026-10-21
- Kartu **Data stok** di halaman Cek stok: semua barang yang dibaca AI, kolom Total dan satu kolom per cabang, **Cek terakhir** (Cocok / n selisih · waktu), pencarian (nama, kode, kategori), urut naik/turun tiap kolom, filter cabang, 10/25/50 baris per halaman. Klik baris atau **Cek** membuka pencocokan barang itu di bawah tabel (tombol **Tutup** kembali ke riwayat).
- `GET /api/stock/products` kini mengembalikan semua barang dengan `by_branch` dan `last_check` (menggantikan pencarian `?q=`).

## Feat · Halaman Cek stok: cocokkan stok yang dibaca AI dengan stok real · 2026-10-21
- Menu **Operasi → Cek stok** (`/stok/cek`, terlihat bagi pemegang Push stok atau Chat). Cari barang (nama/kode) lalu lihat per cabang: **Dibaca AI** (`stock_items`, yang dipakai AI Trigger, Push stok, Orchestrator), **Baris BigQuery** (baris sumber per gudang yang dipetakan ke cabang itu), isi **Stok real** dari Accurate/gudang → **Selisih** dan **Hasil**: *Cocok*, *AI ≠ BigQuery* (sinkron / mapping gudang) atau *BigQuery ≠ real* (query / data sumber).
- **Jawaban AI Trigger** untuk barang itu per cabang percakapan (termasuk "Semua cabang"), dihitung dengan fungsi yang sama dengan kartu di Chat.
- **Data mentah BigQuery per gudang** dengan cabang hasil mapping; gudang yang belum dipetakan ditandai.
- **Simpan hasil cek** menyimpan hitungan real per cabang bersama angka yang dibaca AI saat itu (tabel `stock_checks`, migrasi 0057); riwayat per barang dan "Cek stok terakhir" untuk semua barang.
- API: `GET /api/stock/products?q=`, `GET /api/stock/check?sku=&branch=`, `GET/POST /api/stock/checks`.

## Fix · AI Trigger Cek stok tidak lagi menjawab "Kosong di Semua cabang" · 2026-10-21
- Percakapan sales yang memegang semua cabang (mis. Granike, cabang "Semua cabang") membaca stok cabang bernama "Semua cabang" — yang tidak ada — sehingga stok selalu 0 dan kartu menulis "Kosong di Semua cabang" padahal Ezviz NVR X5S 8W ada di Surabaya, Semarang, Soetta, Jakarta. Cabang percakapan yang tidak dimiliki baris stok mana pun kini dianggap "tanpa cabang": stok semua cabang dijumlah ("Ready · 151 di semua cabang").

## Perf · Semua halaman lebih cepat: JIT PostgreSQL dimatikan, halaman dimuat per kebutuhan, refetch berlebihan dihapus · 2026-10-20
- **JIT PostgreSQL dimatikan** untuk setiap koneksi aplikasi (API dan worker). Daftar chat 2,1 dtk → ~0,4 dtk: 1,9 dtk di antaranya kompilasi JIT untuk ~240 ms kerja (EXPLAIN ANALYZE di produksi). Berlaku untuk semua query dengan estimasi biaya tinggi (Orbit, brief, kredit, health).
- **Halaman dimuat per kebutuhan**: setiap halaman kecuali Pusat kendali jadi chunk sendiri, diunduh di latar belakang saat browser idle. Muatan awal 1,6 MB → 327 KB (gzip 452 → 102 KB); three.js hanya untuk Dashboard dan Relasi. Tab lama yang meminta chunk yang sudah diganti deploy dimuat ulang sekali otomatis.
- **Refetch berlebihan dihapus**: pesan WA baru tidak lagi memuat ulang Analisa WhatsApp di pill kanan atas setiap halaman (~11.700×/hari), label, dan anggota grup — hanya daftar chat, isi chat, konteks, pencarian, dan status nomor. Kartu "Chat belum dibaca" di Dashboard memakai hitungan nomor WA, bukan seluruh daftar chat; komet Stasiun Orbit disegarkan per menit.
- Dibersihkan: komponen `control/Queue.tsx` yang tidak dipakai lagi, file `.DS_Store`.

## Feat · Stasiun Orbit: tombol "AI Agent bekerja", Perbesar & Layar penuh, hasil kerja agen terlihat · 2026-10-20
- Tombol **🚀 AI Agent bekerja** di kepala adegan menjalankan Orchestrator untuk semua data (lewat `POST /cycles`, mesin **Sistem** atau **MCP Claude** — pilihan yang sama dengan dock Orchestrator). Tidak ada yang dikirim ke dealer: hasilnya usulan yang menunggu keputusan manusia.
- **Astronot memerankan agennya sungguhan** dari `GET /cycles/{id}/progress` (diperbarui tiap 2 detik): *antri* melayang menunggu di dekat Inti AI, *menganalisa* mengitari inti dengan sinar ke inti (label berisi data yang dibaca), *selesai* terbang ke depan Dek Keputusan sambil mengangkat kapsul usulan dengan label hasilnya (mis. "Order · ✓ 3 saran"), *gagal* berlabel merah. Siklus terjadwal juga diperankan saat adegan terbuka.
- **Panel hasil kerja AI agent** di pojok adegan: status tiap agen langsung, lalu setelah selesai jumlah usulan baru dan judulnya, dengan tombol *Putuskan di Pusat kendali*. Hasil siklus yang dimulai dari tombol tetap tampil sampai ditutup; hasil siklus terjadwal tertutup sendiri setelah 45 detik. Pop-up "Analisis berhasil" tidak muncul selama adegan terbuka (hasilnya sudah di adegan).
- Tombol **Perbesar** (adegan lebih tinggi) dan **Layar penuh** (Esc untuk keluar; di peramban tanpa Fullscreen API adegan memenuhi jendela).

## Fix · Stasiun Orbit: layar hitam di sebagian GPU/browser · 2026-10-20
- Di sebagian perangkat, jalur HD (bloom di render target MSAA float) tidak menggambar apa pun: hanya label yang tampil di atas latar hitam. Render target float kini hanya dipakai bila GPU mendukungnya; setelah frame pertama adegan memeriksa piksel di Inti AI, dan bila hitam (atau jalur HD error) otomatis beralih ke render langsung ke kanvas (tetap PBR, ACES, MSAA; tanpa bloom).

## Feat · Dashboard: Tambang Distribusi diganti Stasiun Orbit 3D HD · 2026-10-20
- Adegan Dashboard kini **miniatur luar angkasa 3D grafik HD** (three.js): MSAA, tone mapping ACES, bloom, material PBR dengan environment map, nebula dan ribuan bintang, sabuk asteroid. Kualitas turun otomatis (pixel ratio, lalu bloom) bila perangkat tidak kuat; tanpa WebGL tampil pesan; *reduce motion* menghentikan gerak.
- **Inti AI = Orchestrator** di tengah, dikelilingi cincin stasiun. Enam busur di sekitar inti (dan chip di pojok) menyala per tahap siklus: Ingest → Analisis → Sintesis → Keputusan → Eksekusi → Belajar. Saat siklus berjalan, inti berdenyut, semua astronot berkumpul menganalisa (sinar ke inti), kilat kecil saat Sintesis & konflik, sinyal mengalir planet → inti (Ingest), inti → Dek Keputusan (Keputusan), dek → planet (Eksekusi), gelombang kejut saat Belajar.
- **Dealer = planet** dengan tekstur prosedural: ukuran = omzet, makin dekat inti = jadwal order makin dekat (lewat jadwal paling dalam), atmosfer = status kredit, cincin emas = Key account, cincin merah berdenyut = At risk, planet es = Churn.
- **Enam agen = astronot** berwarna sendiri. AI Order (biru) mengantar peti ke dealer jadwal order dekat, AI Penagihan (merah) menagih dealer over limit / overdue (koin), AI Kredit (ungu) memasang perisai heksagon di dealer limit tipis, AI Follow-up (oranye) spacewalk bertali ke dealer At risk. Keempatnya membawa **kapsul usulan ke Dek Keputusan**. AI Stok merapikan peti stok menua di cargo bay (lampu merah bila ada stok kritis), AI Prospek mengarahkan radar.
- **Komandan berhelm emas = manusia** di Dek Keputusan: satu kapsul per usulan yang menunggu (warna agennya), hanya manusia yang memutuskan.
- **Komet = nomor baru** yang menghubungi lebih dulu (Chat → Nomor baru, tanpa pemasok). Sistem tidak mencari leads: tidak ada yang muncul tanpa pesan masuk. Radar AI Prospek memindai komet bergantian.
- Arahkan kursor untuk detail (termasuk apa yang sedang dikerjakan tiap astronot). Klik planet → dealer, inti → Orchestrator, dek/komandan → Pusat kendali, cargo bay → Push stok, antena → Chat, komet → chat nomor itu. Seret untuk memutar, pinch / Ctrl+scroll atau tombol +/− untuk zoom, klik dua kali untuk kembali.
- Chat belum dibaca kini diambil dari hitungan nomor WA, bukan dengan memuat semua chat.

## Feat · Dashboard: Kota Distribusi diganti Tambang Distribusi 3D · 2026-10-20
- Adegan di atas Dashboard kini **tambang terbuka 3D** (three.js): lubang tambang berteras dengan jalan angkut spiral, Gudang GSI dan Kantor sales di bibir tambang, pohon dan menara lampu sorot (menyala malam hari, WIB).
- **Setiap dealer = satu titik tambang** di sepanjang jalan angkut (omzet terbesar paling dekat bibir tambang). Penambang mengayun beliung (serpihan batu beterbangan); warna bijih = sisa limit, emas = Key account, tanda ! = At risk, palang = Churn, label "order N hr lagi" untuk jadwal ≤ 14 hari.
- Semua gerakan dari data nyata: **excavator** menggali dan menuang di dealer yang jadwal ordernya ≤ 14 hari, **dump truck** turun dari gudang mengantar lalu menuang muatan, **pickup merah** menagih dealer over limit / overdue (koin di gudang saat kembali), **surveyor** berjalan follow-up ke dealer At risk. Peti stok menua di halaman gudang, lampu merah berkedip bila ada stok kritis, label chat belum dibaca di kantor.
- Arahkan kursor untuk detail, klik titik tambang untuk membuka dealer (gudang → Push stok, kantor → Chat). Seret untuk memutar kamera, klik dua kali untuk kembali. Tombol Sembunyikan tetap ada; tanpa WebGL tampil pesan. Gerak dihentikan bila pengguna memilih *reduce motion*.

## Feat · MCP Claude: semua kartu daftar jadi datatable (cari, urut, paginasi) · 2026-10-20
- **Koneksi**, **Tool MCP**, **Contoh prompt analisis**, tabel model di **Pemakaian AI**, daftar **Analisis terjadwal**, dan **Riwayat Analisa WhatsApp** kini datatable seperti Riwayat MCP: pencarian, urut per kolom, paginasi 10/25/50. Koneksi yang dicabut dan jadwal yang dijeda tetap tampil redup; tombol Cabut, Jalankan, Laporan, Ubah, dan switch jadwal tetap di barisnya. Riwayat Analisa WhatsApp kini memuat semua run (sebelumnya 12 terakhir).
- Kartu formulir (Hubungkan Claude, Token manual, Izin & batas, Routine Claude) tidak berisi daftar, jadi tetap.

## Feat · Cabang: datatable (cari, urut per kolom, paginasi) · 2026-10-20
- Master data → **Cabang** kini datatable seperti Pengguna dan Peran & akses: pencarian (cabang, kota, alamat, aktif/nonaktif), urut per kolom (Cabang, Kota, Pengguna, Dealer, Status), paginasi 10/25/50. Kolom **Status** baru; cabang nonaktif tetap tampil redup.

## Feat · Peran & akses: datatable (cari, urut per kolom, paginasi) · 2026-10-20
- Master data → **Peran & akses** kini datatable seperti Pengguna: pencarian (nama peran, keterangan, nama halaman, cakupan data, kebijakan, aktif/nonaktif), urut per kolom (Peran, Halaman, Data, Keputusan, WhatsApp, Pengguna, Status), paginasi 10/25/50. Kolom **Status** baru; peran nonaktif tetap tampil redup.

## Feat · Pengguna: datatable (cari, urut per kolom, paginasi); sinkron data dicek saat worker menyala · 2026-10-20
- Master data → **Pengguna** kini datatable seperti kartu Pusat kendali: pencarian (nama, email, peran, cabang, nomor WhatsApp, aktif/nonaktif), urut per kolom (Nama, Peran, Cabang, WhatsApp, Status), paginasi 10/25/50. Kolom **Status** baru; akun nonaktif tetap tampil redup.
- Sinkron BigQuery kini juga dicek saat worker menyala — jadwalnya tidak lagi mundur setiap deploy (tetap mengikuti interval sinkron di Pengaturan → Data).
- Kredit · kas: label KPI "DSO · order → bayar, 90 hari" (sebelumnya tertulis dua kali).

## Fix · Data BigQuery: satu query gagal tidak lagi membekukan semua data (beku sejak 7 Okt) · 2026-10-20
- **Masalah**: query stok membaca tabel BigQuery `accurate_data.master_aging` yang sudah tidak ada (404). Karena satu query gagal, setiap sinkron berhenti sebelum langkah *Apply* — baris invoice, pembayaran, dan pelanggan tetap ditarik tiap 15 menit tetapi tidak pernah masuk ke aplikasi. Akibatnya Kredit · kas, status dealer, Orbit, Segmen, dan Push stok memakai data **7 Okt**.
- **Perbaikan**: query yang gagal dicatat sebagai run gagal (terlihat di Pengaturan → Data) dan dilewati; entitas lain tetap disimpan dan di-*Apply*. Sinkron berikutnya mengejar data 8–20 Okt.
- **Belum**: query stok perlu diarahkan ke tabel stok yang baru di BigQuery (Pengaturan → Data → query stok). Sampai itu, stok tetap data 7 Okt.

## Feat · Dealer: klasifikasi dealer — % dealer aktif dan filter per status · 2026-10-20
- Daftar dealer punya kotak **Dealer aktif**: persentase dealer aktif (Key account + Aktif + Baru) dari dealer yang pernah order, bar komposisi status, dan chip **Key account · Aktif · Baru · At risk · Churn** dengan persen dan jumlah — klik untuk memfilter (klik lagi untuk melepas); klik angka "Dealer aktif" untuk menampilkan semua dealer aktif. **Prospek** (belum pernah order) dihitung terpisah, tidak masuk persentase. Persentase mengikuti pencarian dan jenis pelanggan yang dipilih. Menggantikan dropdown status.
- Status di daftar ditulis apa adanya (Baru tidak lagi ditampilkan sebagai Aktif) agar sama dengan filternya.
- Dicek terhadap MD (08-frontend): DealerList (urut posisi siklus), DealerHeader, Langkah berikutnya, Order-to-cash, Share of wallet & product mix, Sisa limit, Memori, PIC aktif, Komitmen, Timeline — sesuai.

## Fix · Peta relasi: catatan Analisa Relasi ikut filter sales dan dealer · 2026-10-20
- Dicek terhadap MD (08-frontend, ADR 0054): node sales biru, dealer berwarna skor, ukuran = interaksi/bulan, periode 30–180 hari, klik dua kali → dealer, cadangan 2D, Lihat dari, Pasangan terkuat, Pola relasi — sesuai. **Analisa Relasi** memang ditulis Claude dari isi chat WhatsApp lewat routine: setiap siklus Orchestrator (tiap jam) Claude mengambil input "Analisa Relasi" dan mengirim ringkasan + catatan per dealer lewat MCP.
- Catatan Claude per dealer kini **ikut filter "Lihat dari" (sales) dan filter dealer**, seperti peta — sebelumnya selalu menampilkan semua catatan. Kartu menyebut cakupannya (chat 30 hari terakhir) dan bahwa analisa ditulis lewat routine.

## Fix · Segmen: titik dealer Baru terlihat (sebelumnya putih di kanvas putih) · 2026-10-20
- Dealer **Baru** (baru 1 order, belum punya frekuensi) kini digambar **indigo** dengan garis putus indigo — sebelumnya putih dengan garis abu tipis sehingga hilang di kanvas putih, terutama di Segmen D. Legenda menambah "Baru · 1 order".

## Fix · Segmen: titik di posisi asli (sesuai MD), Segmen D tampil, kontrol Titik dihapus · 2026-10-20
- **Prioritas** kini 40 dealer omzet terbesar **per segmen** — sebelumnya 150 omzet terbesar dari semua dealer, sehingga Segmen D (dealer kecil) dan sebagian besar Segmen B tidak punya titik sama sekali. "Semua titik" tetap menampilkan seluruh dealer.
- **Setiap titik di posisi aslinya** (08-frontend: *posisi titik tidak digeser*): titik tidak lagi didorong menjauh agar tidak bertumpuk. Titik yang bertumpuk dipisahkan dengan zoom in (ukuran titik di layar tetap saat zoom in).
- **Besar titik = porsi belanja dealer di GSI** (share of wallet); kini tertulis di legenda. Kontrol **Titik** (kecil / besar) dihapus dari Segmen karena ukurannya sudah membawa arti. Peta Orbit tidak berubah.

## Fix · Segmen: kanvas lebih besar, zoom in / zoom out diperbaiki · 2026-10-20
- Peta titik Segmen kini selebar kartunya (sebelumnya maksimal 720 px).
- **Zoom scroll diperbaiki**: setelah kanvas dibuat tanpa batas, posisi peta di layar terbaca salah sehingga scroll di sebagian besar area tidak meng-zoom dan zoom tidak berpusat di kursor. Kini zoom scroll / cubit / klik ganda berpusat tepat di kursor di mana pun.
- Tombol **− / +** melangkah ke angka bulat **50 · 75 · 100 · 125 · 150 · 200 · 300 · 400 · 600 · 800%**; klik angka % untuk kembali ke 100%. Segmen bisa di-zoom out sampai **50%** (seperti draw.io) — di bawah 100% titik dealer ikut mengecil. Peta Orbit tetap mulai dari 100%.

## Fix · Segmen: kanvas tidak terpotong saat digeser / zoom (seperti draw.io) · 2026-10-20
- Warna kotak segmen, garis grid, dan garis batas "sering" / "besar" kini menyambung sampai tepi layar ke mana pun peta digeser atau di-zoom — sebelumnya berhenti di batas data sehingga sisanya putih dan peta tampak terpotong.
- Latar titik-titik halus (seperti draw.io) ikut bergeser dan di-zoom bersama titik dealer; ukurannya tetap kecil di zoom berapa pun.

## Feat · Orchestrator: susunan kartu baru, Jalankan ulang agen tidak lagi tertahan · 2026-10-20
- Kolom kiri: Pipeline analisis → Resolusi konflik → **MCP sebagai orchestrator**. Kolom kanan: Agen → **Riwayat analisis** → **Matriks otonomi**.
- **Perbaikan "Jalankan ulang" di kartu agen**: bila browser melewatkan sinyal "siklus selesai" dan siklus lain sudah dimulai, aplikasi mengira Orchestrator masih berjalan sehingga semua tombol analisis tetap mati sampai halaman dimuat ulang. Kini status siklus itu dibaca langsung dari server. Tombol juga tidak lagi mati diam-diam: saat siklus lain berjalan, tombol bertuliskan "Menunggu…" dan bila diklik menjelaskan alasannya.

## Feat · Pusat kendali: kartu Limit tipis di bawah Ringkasan Orchestrator · 2026-10-20
- Kartu **Limit tipis** pindah ke kolom kanan, tepat di bawah **Ringkasan Orchestrator**. Kolom kiri: Jadwal order + Push stok | Lewat jadwal. Tombol "Limit tipis" di status strip tetap menggulir ke kartunya.

## Feat · Chat: nomor sales di samping judul, Analisa WhatsApp di samping Orchestrator, zoom mulai 50% · 2026-10-20
- Deretan nomor (Sales / Admin / Penagihan, + Nomor) pindah ke baris judul halaman, di samping tulisan **Chat** — area percakapan jadi lebih tinggi.
- Pill **Analisa WhatsApp · MCP <jam> · berikutnya <jam>** pindah dari kanan atas ke header percakapan, di samping tombol Orchestrator. Halaman lain tidak berubah.
- Zoom chat kini **50%–160%**.

## Feat · Chat: zoom foto dan Orchestrator di samping nama chat · 2026-10-20
- **Zoom percakapan**: tombol **− / % / +** di header chat (di samping tombol cari) memperbesar/memperkecil isi chat 70%–160%; klik angka % untuk kembali ke 100%. Ukuran diingat di browser ini untuk semua chat.
- Foto yang dibuka di chat kini bisa **diperbesar / diperkecil**: tombol **− / % / +** di kanan atas (100%–500%), scroll mouse, tombol keyboard `+` `-` `0`, atau klik dua kali. Saat diperbesar, foto bisa digeser dengan menyeret; klik angka % untuk kembali ke ukuran asli.
- Di halaman Chat, **Orchestrator** tidak lagi melayang di atas kolom ketik: kini tombol **Orchestrator** di header percakapan, di samping nama. Klik untuk membuka panelnya (tanya/perintah, Analisis ulang chat ini, Sistem/MCP Claude) tepat di bawahnya; Esc atau klik di luar menutupnya. ⌘K tetap membukanya. Halaman lain tidak berubah.

## Feat · Push stok: stok BigQuery tiap 5 menit, status sinkron, halaman ikut segar · 2026-10-19
- Stok kini ditarik dari BigQuery **tiap 5 menit** (hanya query stok, ringan) di antara sinkron penuh 15 menit. Halaman Push stok menampilkan sumber dan waktu sinkron terakhir ("sinkron 16.55 · 2 menit lalu · otomatis tiap 5 menit") dengan tombol **Sinkron sekarang**.
- Setelah sinkron BigQuery apa pun, halaman stok, dealer, orbit, segmen, KPI, dan kredit menyegarkan diri (sebelumnya event sinkron tidak didengar).
- Analisa Stok tetap ditulis Claude lewat MCP/routine; komponen halaman sesuai MD. ADR 0056.

## Feat · Dealer: Analisa AI oleh Claude (routine) dengan status dan tombol chat WhatsApp sesuai analisa · 2026-10-19
- **Analisa AI** di halaman dealer kini dikerjakan Claude lewat routine (tanpa membuka Desktop): membaca siklus order, limit, invoice terbuka, dan 15 chat WhatsApp terakhir dealer, lalu menilai **Bermasalah / Perlu perhatian / Aman** dengan alasan dan ringkasan. Progres tampil sebagai pop-up.
- Kartu Analisa AI punya **draf WhatsApp dari Claude** dan tombol **Chat: selesaikan masalah / sapa & tawarkan / sapa dealer** (warna sesuai status) yang membuka chat dealer dengan draf sudah terisi di kolom ketik — sales tetap yang mengirim.
- Tombol WhatsApp di halaman dealer tidak lagi memuat semua chat (lebih ringan). Label Memori dealer diperbaiki. Halaman dealer sesuai komponen MD. ADR 0055.

## Feat · Chat: label "oleh <nama>" di setiap pesan keluar (admin); sales melihat chat di nomor yang dipegangnya · 2026-10-19
- Pesan yang dikirim dari GSI Orbit kini mencatat **siapa yang menekan Kirim / Teruskan / Setujui** (kolom baru, migrasi 0055; riwayat lama diisi dari audit). Admin dan manager (yang melihat semua chat) melihat **"oleh Tantri"** di bubble keluar, atau **"dari HP"** bila dikirim langsung dari HP pemegang nomor. Sales tidak melihat label ini. Label hanya di aplikasi — tidak ikut terkirim ke pelanggan.
- Sales melihat chat milik profil sales-nya **dan** semua chat di nomor yang ia pegang (sebelumnya chat di nomornya sendiri bisa hilang bila nomor itu terdaftar atas sales lain). Penagihan tetap hanya nomor yang dipegangnya.

## Fix · Chat: pesan terkirim/diteruskan memakai nama pemegang nomor · 2026-10-19
- Balasan, pesan yang diteruskan, dan kirim dari Chat kini dicatat atas nama **pemegang nomor yang mengirim** (pengguna pemegang nomor, lalu sales nomor itu, lalu label nomor) — sebelumnya memakai nama sales yang ditautkan ke chat (mis. dealer milik Sofy di nomor Elisa tampil "Sofy" padahal terkirim dari nomor Elisa). Keterangan "Dikirim dari …" di bawah kolom ketik juga diperbaiki.
- Pesan lama yang sudah tercatat dengan nama salah tidak diubah.

## Feat · Peta relasi: Analisa Relasi dari isi chat WhatsApp oleh Claude (routine) · 2026-10-19
- Kartu baru **Analisa Relasi**: Claude membaca isi chat WhatsApp dealer — yang dichat dua sales, yang chatnya menipis, dealer besar yang jarang chat — lalu menulis ringkasan dan catatan per dealer (klik untuk membuka dealer). Lewat routine Claude, tanpa membuka Desktop.
- Peta tetap sesuai MD (garis = interaksi WA + order). Kartu "Pola relasi" diberi label jujur: dihitung dari interaksi WA + order.
- Analisis ulang di Orbit, Segmen, atau Peta relasi kini hanya meminta bacaan halaman itu (lebih cepat); siklus penuh meminta ketiganya. ADR 0054.

## Change · Segmen · Peta titik lebih enak dilihat dan interaktif · 2026-10-19
- Garis bantu halus putus-putus, garis ambang (sering / besar) lebih tegas; area > 3,5×/bln diberi arsir tipis, bukan blok abu-abu.
- Label segmen menjadi kartu di pojok (jumlah dealer + omzet/bln) yang selalu di atas titik — tidak tertutup titik lagi — dan memudar saat titik disorot.
- Arahkan ke zona: zona menyala dan titik segmen lain meredup; klik zona untuk menyaring. Arahkan ke titik: titik lain meredup, titik itu ditonjolkan dengan garis bidik ke kedua sumbu.

## Feat · Segmen: Analisa Segmen dari chat WhatsApp oleh Claude (routine); titik 3 bulan lalu kembali · 2026-10-19
- Kartu baru **Analisa Segmen**: Claude membaca dealer yang pindah segmen, yang melambat, dan yang sedang chat beserta 8 chat WhatsApp terakhirnya — kenapa pindah dan cara melayaninya — dengan catatan per dealer di daftar kotak. Lewat routine Claude, tanpa membuka Desktop.
- Segmen tetap dihitung dari order sesuai MD. Tombol "Analisis ulang" diganti kotak Orchestrator; label "Pindah kotak" diperbaiki.
- Peta titik: **titik putus-putus** posisi 3 bulan lalu (sesuai MD) kembali untuk dealer yang pindah segmen, dengan garis ke posisinya sekarang. ADR 0053.

## Feat · Orbit: Analisa Orbit dari chat WhatsApp oleh Claude (routine) · 2026-10-19
- Kartu baru **Analisa Orbit**: Claude membaca dealer yang perlu disapa (At risk, Churn, jadwal order ≤ 7 hari, atau yang sedang chat) beserta 8 chat WhatsApp terakhirnya, lalu menulis ringkasan dan catatan per dealer di daftar dealer — kenapa dealer bergeser dan langkah follow-up. Berjalan lewat routine Claude, tanpa membuka Desktop.
- Posisi dealer tetap dari siklus order sesuai MD. Tombol "Analisis ulang" di kepala Orbit diganti kotak Orchestrator kanan bawah; label "Yang bergerak" diperbaiki (dihitung dari siklus order).
- Perbaikan tampilan: kalimat "Belum dianalisis AI" di Orbit, Push stok, dan Kredit tidak lagi pecah menjadi kolom. ADR 0052.

## Change · Chat: pilih ChatGPT atau Claude di kartu trigger; "Analisa lanjut" diganti Orchestrator · 2026-10-19
- Kartu AI Trigger (stok, harga, limit, status order): tombol **Tulis dengan ChatGPT / Claude** dan *Tulis ulang*; draf AI diberi label AI penulisnya.
- Tombol **Analisa lanjut** dihapus. Kotak **Orchestrator** (seperti di Pusat kendali) kini ada di Chat sebagai ikon di atas kolom balasan dan menganalisa chat yang sedang dibuka.

## Change · Chat lebih ringan: tanpa tab Semua, tanpa total tim, tidak membuka chat otomatis · 2026-10-19
- Barisan nomor: tab **Semua** dihapus; label Sales / Admin / Penagihan hanya pengelompok (tanpa jumlah, tidak bisa diklik). Chat memuat satu nomor saja — nomor terakhir yang dibuka di browser itu, lalu nomor milik Anda, lalu nomor pertama.
- Membuka halaman Chat tidak lagi langsung membuka percakapan; panel kanan "Pilih chat" sampai Anda klik.
- Jumlah belum dibaca (barisan nomor dan badge Chat di menu) diambil dari status nomor, bukan dengan memuat semua chat — sebelumnya semua chat dimuat di setiap halaman hanya untuk badge itu.

## Fix · Chat: percakapan yang dibuka tidak berpindah sendiri · 2026-10-19
- Chat yang sedang terbuka tetap terbuka sampai Anda memilih chat lain: pesan baru yang menaikkan chat lain ke atas, pindah tab sales/nomor, atau pindah filter tidak lagi mengganti percakapan di layar. Sebelumnya tampilan ikut ke chat teratas bila belum ada chat yang diklik.

## Change · AI Trigger: rekomendasi jawaban stok/harga ditulis AI saat itu juga · 2026-10-19
- Semua trigger (stok, harga, limit & tagihan, status order) kini meminta AI menulis rekomendasi jawaban begitu pesan pelanggan cocok — hanya pesan yang cocok, jadi hemat. Angka tetap dari sistem; draf yang angkanya tidak cocok ditolak.
- Halaman AI Trigger: kartu **AI penulis rekomendasi** — ChatGPT (bawaan, gpt-4.1-mini) atau Claude, pilih model. ChatGPT hanya dipakai di sini; analisa tetap Claude. ADR 0051.

## Feat · Halaman Report WhatsApp per sales · 2026-10-19
- Menu baru **Report WhatsApp** (Kendali): periode Hari ini / 7 hari / 30 hari, filter sales; sales hanya melihat datanya sendiri.
- Angka tim: chat masuk, % dibalas ≤ target (30 menit jam kerja), % permintaan dijawab, chat gantung, nomor baru, order dari chat → SO.
- **Peringkat sales** dengan skor 0–100 dan **winning sales**; **feedback baik & buruk** tiap sales; **ringkasan untuk direktur & manager**.
- **Tren harian**, **jam ramai pelanggan** (hari × jam), tabel **chat gantung** (belum dibalas / janji "cek dulu") dan **nomor baru** — dengan cari, urut, halaman. Semua dihitung dari chat, tanpa AI. ADR 0050.

## Feat · Analisis Claude lewat routine claude.ai — tanpa membuka Claude Desktop · 2026-10-19
- Setiap siklus yang menunggu Claude (tombol Analisis ulang dan siklus per jam) memicu routine **"GSI Orbit · Analisis Orchestrator"** di claude.ai. Claude bekerja di cloud lewat konektor CRM-DISTRI dengan langganan Claude — tanpa membuka Desktop, tanpa kunci API, tanpa ChatGPT. Siklus menunggu sampai 30 menit.
- Halaman **MCP Claude → Routine Claude**: CEO menyimpan ID routine dan token trigger API (dibuat sekali di claude.ai), lalu **Tes sekarang**.
- Pop-up analisis: "Claude sedang bekerja di claude.ai (routine)" dengan tautan **Lihat Claude bekerja**; tombol Buka Claude hanya bila routine belum diisi atau gagal. ADR 0049.

## Change · ChatGPT tidak pernah dipakai · 2026-10-19
- ChatGPT dimatikan di kode: tidak lagi mengambil alih siklus Orchestrator atau Analisa WhatsApp bila Claude tidak menjawab, tidak dipakai analisa otomatis maupun draft balasan. (Siklus #305 dari Pusat kendali ternyata dijawab ChatGPT API cadangan, bukan Claude.)
- Pilihan AI Claude/ChatGPT dihapus: Analisis ulang hanya Claude lewat MCP. Halaman API & model AI tidak lagi menawarkan ChatGPT. ADR 0048.

## Change · Analisis ulang lewat MCP Claude atau ChatGPT, tanpa kunci API; pop-up analisis diperbaiki · 2026-10-19
- Kotak Orchestrator dan halaman Orchestrator: analisa lewat **MCP** dengan pilihan **AI: Claude | ChatGPT** (bawaan Claude), model dan effort. Pilihan "API & model AI" dihapus — tidak ada kunci API Claude yang dipakai.
- Pilih Claude = ChatGPT tidak dipakai: siklus dari tombol Analisis ulang tidak diambil alih ChatGPT API walau opsi kedua aktif.
- Pop-up "AI sedang membaca data…" kini mengikuti siklus yang berjalan (sebelumnya bisa menulis "Selesai" dan "API" dengan semua tahap tercentang padahal siklus belum jalan). ADR 0047.

## Change · Orchestrator: Analisis ulang dijalankan sistem, tanpa membuka Claude Desktop; progres jadi pop-up · 2026-10-19
- Tombol **Analisis ulang** (dan *Jalankan ulang* per agen) kini dijalankan server dengan Claude lewat kunci API (Pengaturan → API & model AI), memakai tool MCP yang sama — tidak perlu membuka Claude Desktop. Header menampilkan mesin yang dipakai; tanpa kunci API tombol nonaktif dengan alasannya.
- "AI sedang membaca data…" tampil sebagai pop-up, bukan kartu di halaman Orchestrator dan Pusat kendali.

## Change · Halaman Orchestrator: Analisis ulang lewat MCP Claude, isi dari WhatsApp, kartu dirapikan · 2026-10-19
- **Analisis ulang · MCP Claude** di header, dengan pilihan model dan effort (sama dengan kotak Orchestrator). *Jalankan ulang* per agen juga lewat MCP Claude.
- Isi dari WhatsApp: Pipeline (chat WhatsApp + stok), Resolusi konflik pada dealer yang chat, **Riwayat analisis** = riwayat Analisa WhatsApp (tabel data: cari, urut, halaman), kartu **Agen** menampilkan yang dibaca tiap agen dari chat dan usulannya.
- Tampilan: pipeline 3 kolom, kartu agen ringkas, tabel konflik dan riwayat muat di kartu, Matriks otonomi pindah ke kolom kiri.

## Change · Pusat kendali: susunan baru dan semua daftar jadi tabel data · 2026-10-19
- Susunan: **Agenda sales** di bawah Rencana hari ini; **Ringkasan Orchestrator** pindah ke tempat Agenda sales (kolom kanan, di bawah KPI utama); **Push stok** di bawah Jadwal order; **Limit tipis** di bawah Lewat jadwal.
- Rencana hari ini, Agenda sales, Keputusan, Jadwal order, Lewat jadwal, Push stok, dan Limit tipis kini tabel data: kolom pencarian, urut naik/turun per kolom, dan halaman 10/25/50 baris.

## Change · Pill kanan atas = Analisa WhatsApp otomatis lewat MCP · 2026-10-19
- Pill di kanan atas (sebelumnya "Analisis #… · berikutnya" dari siklus Orchestrator per jam) kini menampilkan **Analisa WhatsApp otomatis lewat MCP**: kapan terakhir berhasil, sedang berjalan atau gagal, dan jam berikutnya. Klik membuka Pusat kendali.
- Jadwal yang ditampilkan: routine claude.ai (ADR 0037) atau jadwal API server (ADR 0043) — yang aktif dan paling dekat jalannya. Pengguna tanpa akses Chat tetap melihat siklus Orchestrator.

## Change · "Tanya atau perintahkan Orchestrator" pindah ke kotak Orchestrator; kotaknya lebih jelas · 2026-10-19
- Kolom tanya di bilah atas dihapus. Kolom yang sama kini ada di kotak Orchestrator kanan bawah (paling atas saat kotak dibuka); ⌘K / Ctrl+K membuka kotak dan langsung ke kolom itu.
- Kotak Orchestrator diberi garis tepi berwarna, garis gradasi AI di atas, dan bayangan lebih tegas; ikon ringkasnya diberi cincin.

## Feat · Analisis ulang oleh sistem GSI Orbit, tanpa membuka Claude · 2026-10-19
- Mesin baru **Sistem**: aturan agen dan rumus GSI Orbit dijalankan langsung di server — tanpa Claude, tanpa API, tanpa biaya AI, tanpa menunggu. Usulan tetap menunggu keputusan manusia.
- Halaman **Orchestrator**: tombol utama **Analisis ulang (Sistem)**, tombol kedua **Lewat Claude · MCP**. Kotak Orchestrator kanan bawah: pilihan **Sistem · MCP Claude · API & model AI**.
- Pop-up analisis tidak lagi menulis "Selesai" saat siklusnya masih menunggu siklus lain. ADR 0046, migrasi 0049.

## Change · Pusat kendali dan Orchestrator persis sesuai MD; analisa dari WhatsApp, stok dari BigQuery · 2026-10-19
- **Pusat kendali** kembali ke susunan `08-frontend.md`: **Rencana hari ini**, Keputusan, Ringkasan, Jadwal order · 7 hari, Lewat jadwal, KPI utama, Agenda sales, Push stok, Limit tipis. Orchestrator tetap kotak di kanan bawah seperti sebelumnya (bisa diringkas jadi ikon).
- Isi dari WhatsApp: Rencana dan Keputusan dari usulan Claude atas chat; Jadwal order = chat minta order; Lewat jadwal = chat lewat batas balas; Limit tipis = chat minta rilis kredit/retur; KPI dan Agenda dari WhatsApp. **Push stok** dari stok BigQuery.
- **Orchestrator** kembali ke susunan MD: Pipeline, Resolusi konflik, Riwayat analisis, Agen (Jalankan ulang), MCP, Matriks otonomi. ADR 0045.

## Change · Dashboard tanpa Orchestrator; Orchestrator bisa diringkas jadi ikon · 2026-10-19
- Dock **Orchestrator** melayang tidak lagi tampil di halaman **Dashboard**.
- Dock punya tombol **Ringkas jadi ikon** (panah di kanan atas): dock mengecil jadi ikon bulat di pojok kanan bawah. Klik ikon untuk membuka lagi. Saat analisis berjalan, ikon berdenyut. Pilihan ringkas diingat di browser.

## Feat · Halaman dealer: Analisa AI (Orchestrator) dan tombol WhatsApp · 2026-10-17
- Header dealer punya tombol **Analisa AI**: Orchestrator menganalisis dealer itu, dan progres AI (data dibaca, per agen, langkah terakhir), tombol Buka Claude, game, lalu hasil berhasil/gagal tampil langsung di halaman dealer (tanpa pop-up). Panel yang sama dipakai halaman Orchestrator.
- Tombol **WhatsApp** (hijau) membuka chat dealer itu di Chat GSI Orbit. Bila beberapa PIC punya chat, muncul pilihan per chat dengan waktu terakhir. Balasan tetap lewat antrean kirim dan penjaga anti-blokir. Tombol tidak aktif bila belum ada chat, dan tidak tampil untuk peran tanpa menu Chat.

## Change · Bar nomor WhatsApp: chip nama saja · 2026-10-17
- Bar nomor di atas Chat kini berisi chip kecil berisi nama saja (titik status + nama + jumlah belum dibaca), tanpa kartu avatar inisial. Tinggi bar 40 px.

## Feat · Kirim kontak WhatsApp; nama panggilan tersimpan ke kontak WhatsApp HP · 2026-10-17
- **Kirim kontak**: menu lampiran (📎) punya **Kontak**. Isi nama + nomor (08… atau +62…), lalu terkirim sebagai kartu kontak WhatsApp (vCard), lewat antrean kirim dan penjaga anti-blokir seperti pesan lain. Kartu tampil di chat dengan nama, nomor, dan tombol **Chat**. Kartu kontak dari pelanggan kini ikut menampilkan nomornya.
- **Simpan ke kontak HP**: saat **Nama panggilan** (ikon pensil di header chat) disimpan, nama itu juga disimpan ke kontak WhatsApp nomor yang memegang chat, dan tersinkron ke HP-nya. Kalau HP offline, nama tetap tersimpan di GSI Orbit dan notifikasi memberi tahu. Menghapus nama panggilan tidak menghapus kontak di HP. ADR 0044.

## Change · Chat: Ringkas tampilan daftar (Alt+R), bubble dan kotak tulis lebih kecil · 2026-10-17
- Tombol **Ringkas tampilan** di header daftar chat (atau **Alt+R**): satu baris nama + jam, satu baris pesan terakhir; tag grup, sumber, label, dan "via" disembunyikan, sementara jumlah belum dibaca, waktu tunggu, dan pin tetap tampil. Baris ±49 px (sebelumnya ±88 px). Pilihan diingat di browser.
- Bubble chat lebih kecil (teks 12,5 px, padding lebih rapat) dan kotak tulis pesan lebih pendek (tinggi 36 px, tetap membesar otomatis untuk pesan panjang).

## Change · Chat lebih lega di layar laptop · 2026-10-17
- Halaman Chat kini pas satu layar (1366×650 tanpa menggulir). Judul halaman diringkas (tanpa subjudul), bar nomor lebih tipis, header daftar chat, filter, header percakapan, dan kotak tulis dirapatkan. Baris info "Dikirim dari …" dipendekkan (keterangan lengkap tetap muncul saat kursor diarahkan). Batas tinggi minimal 560 px yang membuat halaman ikut menggulir dihapus, jadi area percakapan dapat ±60–80 px lebih tinggi. Tidak ada fungsi yang dihapus.

## Fix · Sesi tidak perlu login ulang; Rendy (Penagihan) hanya melihat chat nomornya · 2026-10-17
- **Sesi bergeser**: login berlaku 30 hari dan diperpanjang otomatis setiap kali aplikasi dipakai (token diperbarui tiap jam). Sebelumnya sesi habis tetap 12 jam setelah login. Login ulang hanya setelah menekan Keluar, atau setelah 30 hari tidak membuka aplikasi.
- **Chat WhatsApp per peran**: Peran & akses punya sakelar baru **Chat WhatsApp hanya nomor sendiri** (`roles.chat_scope`, migrasi 0048). Peran dengan data "miliknya" selalu hanya nomornya sendiri. Peran ini juga tidak bisa mengatur tim atau menautkan nomor orang lain, dan hanya melihat nomor lengkap miliknya.
- Peran baru **Penagihan**: halaman sama dengan Admin, tetapi chat hanya nomor sendiri. Pemegang nomor tim Penagihan yang berperan Admin (Rendy) dipindah ke peran ini. Catharina tetap Admin.

## Feat · AI hanya membaca yang baru sejak analisis terakhir; tombol Batalkan analisis · 2026-10-17
- **Penanda analisis terakhir per agen**: Input untuk Claude/API kini hanya berisi pesan WA, nomor baru, dan dealer yang punya sinyal baru atau kandidat **sejak analisis AI terakhir agen itu** (siklus penuh yang jawabannya diterima). Sebelumnya setiap agen menerima semua 2.841 dealer + pesan 7 hari (~6 MB per agen). Angka dan kandidat tetap dihitung Go dari data lengkap. Analisis satu dealer dan agen yang belum pernah dianalisa tetap melihat 7 hari. Penanda ikut dikirim ke AI (`since`) dan tampil di progres ("baru sejak 17/10 11.19").
- **Batalkan analisis**: tombol di panel "AI sedang membaca data" (Pusat kendali, Orchestrator, pop-up). Siklus antre langsung ditutup, siklus berjalan berhenti dalam ±1 detik (juga saat menunggu Claude), dicatat "Dibatalkan oleh …", dan Analisis ulang bisa dipakai lagi. Endpoint `POST /api/cycles/{id}/cancel`, migrasi 0047.

## Feat · Orchestrator: Analisis ulang dengan progres AI dan game langsung di halaman · 2026-10-17
- Tombol **Analisis ulang (Claude · MCP)** di halaman Orchestrator tidak lagi membuka pop-up. Panel tampil di bawah header halaman: progres AI (data dibaca, per agen, langkah terakhir), tombol **Buka Claude**, dan game **Lompat!**. Setelah selesai, hasil berhasil/sebagian/gagal muncul di panel yang sama sampai ditutup.
- Siklus yang tidak dimulai dari halaman ini (siklus per jam) juga tampil di panel dengan progres dan game.

## Feat · Pusat kendali: progres "AI sedang menganalisa apa" · 2026-10-17
- Panel "AI sedang membaca data" kini menampilkan **data yang dibaca** (mis. "WhatsApp 963 chat · 288 menunggu"), **progres per agen** (antri · sedang dianalisa · selesai/gagal) dengan cakupannya (jumlah dealer, pesan, stok, kandidat), bar *x/y selesai*, dan **langkah AI terakhir** (membaca data AI Stok, mengirim analisis AI Kredit · 2 saran) dengan jam dan nama koneksinya. Diperbarui tiap 2,5 detik selama siklus berjalan; juga tampil di pop-up analisis.
- Endpoint baru `GET /api/cycles/{id}/progress` (tahap, agen, langkah MCP dari `mcp_calls`).

## Feat · Analisa WhatsApp otomatis: pilih jam dan model AI di Pengaturan · 2026-10-17
- Pengaturan → API & model AI punya kartu baru **Analisa WhatsApp otomatis**: Aktif/Mati, **Pakai AI** (Claude API atau ChatGPT API) dan modelnya, **jam (WIB)** yang dipilih per jam, dan opsi juga hari Minggu. Riwayat run (berhasil/gagal, model, biaya) tampil di kartu yang sama.
- Server menjalankan tugas Analisa WhatsApp di jam itu dengan model itu (migrasi 0046, jadwal `wa_analysis_auto`, ADR 0043). Bila gagal, **tidak diulang dengan AI lain** — menunggu jam berikutnya; selama aktif, API cadangan untuk routine Analisa WhatsApp juga diam.
- Bar nomor WhatsApp di Chat pindah ke atas panel chat (satu baris), panel chat lebih lega.

## Feat · Pilih model AI & effort di Orchestrator; tahap AI ringkas berikon; game lebih besar · 2026-10-16
- Dock Orchestrator (kanan bawah) punya pilihan **Model** dan **Effort (Low · Medium · High)**. MCP Claude: pilih Claude Opus 5.5 / Sonnet 5.5 / Haiku 5.5 / Fable 5.1 — perintah untuk Claude menyebut model dan effort itu, dan pop-up meminta memilih model yang sama di claude.ai. API di server: modelnya dari Pengaturan → API & model AI, effort-nya benar-benar dipakai (Claude: `output_config.effort`; ChatGPT: `reasoning_effort` pada GPT-5/o3/o4). Pilihan diingat di browser.
- Model dan effort tersimpan di siklus (migrasi 0045) dan tampil di pop-up analisis ("MCP Claude · Opus 5.5 · high") serta di halaman Orchestrator (Model AI terakhir).
- Tahap "AI sedang membaca data" diringkas jadi 6 chip berikon: Baca data · Analisa · Gabungkan · Usulan · Simpan · Belajar.
- Game Lompat! kini selebar pop-up (pop-up dilebarkan), di bawah tahap AI.

## Change · Analisa WhatsApp hanya membaca pesan baru hari ini (hemat token); game Lompat! · 2026-10-16
- AI (Claude lewat MCP atau model di server) kini hanya membaca percakapan yang punya **pesan pelanggan baru hari ini yang belum dibaca**, dan hanya pesan hari ini. Isi sebelumnya ikut sebagai ringkasan lama untuk diperbarui, bukan dibaca ulang. Siklus tanpa pesan baru tidak memanggil AI sama sekali (0 token), dan jadwal Analisa WhatsApp dianggap berhasil.
- Yang sudah dibaca tetap tampil di Pusat kendali: ringkasan per percakapan dan ringkasan umum dibawa ke siklus berikutnya sampai ada pesan baru; catatan per bubble memang sudah tersimpan. Angka (chat, pesan masuk, menunggu balasan, KPI) tetap dihitung sistem dari 7 hari terakhir tanpa AI. Migrasi 0044 (`wa_insights.summary_at`).
- Game diganti **Lompat!**: kurir berlari, tekan Spasi (atau Enter/↑/ketuk) untuk melompati pipa, dua kali untuk lompat lebih tinggi, ambil koin untuk skor tambahan; makin lama makin cepat, skor terbaik disimpan di browser.

## Change · Pop-up analisis: "AI sedang membaca data" + game, pengganti langkah MCP · 2026-10-16
- Pop-up "Claude menganalisis lewat MCP" (langkah 1–3, daftar agen, perintah) dihapus. Setiap **Analisis ulang** kini membuka pop-up **AI sedang membaca data…** dengan tahap yang sedang dibaca dan game **Tap tap Enter**, lalu di pop-up yang sama **Analisis berhasil / sebagian / gagal**.
- Lewat Claude (claude.ai) tetap ada satu tombol **Buka Claude**, karena Claude hanya bekerja bila perintahnya dikirim di sana; setelah Claude mulai mengirim, tampil jumlah agen yang sudah dikirim. Lewat API di server tidak perlu membuka apa pun.
- Panel di Pusat kendali kini hanya untuk analisis yang tidak Anda mulai (siklus per jam), agar tidak tampil dobel.

## Feat · Pusat kendali: AI sedang membaca data, hasil berhasil/gagal, dan game sambil menunggu · 2026-10-16
- Saat analisis berjalan, Pusat kendali menampilkan panel **AI sedang membaca data…**: tahap yang sedang dikerjakan dengan penjelasan (membaca chat, order, invoice, pembayaran dan stok → agen AI menganalisa → menggabungkan temuan → menyusun usulan → menyimpan hasil → mencatat pelajaran), lingkup, mesin AI, dan lama berjalan.
- Setelah selesai, panel yang sama menampilkan **Analisis berhasil**, **selesai sebagian**, atau **gagal** beserta siklus, jam selesai, durasi, jumlah saran diperbarui, dan penyebab bila gagal. Di halaman ini pop-up hasil tidak muncul lagi (tidak dobel).
- Sambil menunggu: game **Tap tap Enter**. Titik berputar di lingkaran; tekan Enter/Spasi atau ketuk saat titik di busur hijau. Tiap kena makin cepat; skor terbaik disimpan di browser. Langsung siap dimainkan saat analisis dimulai (kecuali lewat MCP Claude, yang membuka petunjuknya sendiri).

## Feat · Format teks WhatsApp: tebal, miring, coret, monospace · 2026-10-16
- Bubble kini menampilkan format WhatsApp: `*tebal*`, `_miring_`, `~coret~`, ```` ```monospace``` ````, `` `kode` `` dan baris kutipan `> …`. Aturannya sama dengan WhatsApp (penanda harus menempel pada kata; `2*3*4` atau `file_nama_baru.pdf` tidak berubah). Tag @ dan link tetap jalan di dalam teks yang diformat.
- Kotak pesan: pilih teks lalu klik **B / I / S / </>** yang muncul, atau pintasan Ctrl/⌘+B (tebal), Ctrl/⌘+I (miring), Ctrl/⌘+Shift+X (coret), Ctrl/⌘+Shift+M (monospace). Menekan lagi melepas formatnya. Pesan terkirim dengan penanda, jadi HP penerima menampilkan formatnya.

## Fix · Nama di grup: kontak tersimpan dulu, lalu nama profil WhatsApp · 2026-10-16
- Nama pengirim di bubble grup dan di daftar Anggota kini memakai nama kontak yang disimpan di HP nomor itu; bila tidak disimpan, nama profil WhatsApp orangnya. Nama master data hanya dipakai bila keduanya tidak ada, lalu nomor tersamar.
- Daftar nama anggota diambil dari HP per nomor GSI (disimpan 5 menit). Chat 1:1 tetap: nama kontak tersimpan, lalu nama profil; nama kontak kosong tidak lagi menghalangi nama profil.

## Feat · Anggota grup, klik untuk chat · 2026-10-16
- Grup di Chat punya tombol **Anggota** (ikon orang + jumlah) di atas: daftar anggota dari WhatsApp (admin di atas, tanda Admin/GSI), bisa dicari nama atau nomor. Anggota yang menyembunyikan nomornya dihitung tetapi tidak bisa dibuka.
- Klik anggota, atau klik nama pengirim di bubble grup, langsung membuka chat 1:1 dari nomor GSI yang sama. Bila belum pernah chat, percakapannya dibuat kosong (tidak ada yang terkirim; pesan pertama tetap lewat antrean dan penjaga anti-blokir). Nomor pribadi tidak dibuka.
- Daftar "@" saat menandai kini memakai anggota lengkap yang sama. wa-bridge mengirim tanda admin dan nama kontak anggota.

## Fix · Link di pesan bisa diklik · 2026-10-16
- Alamat web di pesan (`https://…`, `http://…`, `www.…`) kini tampil sebagai link dan terbuka di tab baru, seperti di WhatsApp. Tanda baca penutup kalimat tidak ikut masuk link; hanya alamat web yang dijadikan link.

## Change · Sematkan pesan tanpa batas · 2026-10-16
- Pesan yang disematkan tidak lagi dibatasi 3 per chat; sematan baru tidak menggantikan yang lama.
- Bar sematan tetap menampilkan satu per satu (klik untuk ke pesan dan lanjut ke berikutnya, nomor urut 1/n). Bila lebih dari 3, tombol **Semua** membuka daftar semua sematan untuk langsung ke pesannya atau melepasnya.

## Feat · Tag (@) di grup seperti WhatsApp · 2026-10-16
- Pesan grup yang menandai anggota kini tampil dengan nama yang ditandai (@Budi, berwarna), bukan deretan angka; pratinjau di daftar chat juga.
- Nomor GSI yang ditandai melihat tanda **@** di baris chat, sampai chat itu dibuka di nomor tersebut. Membaca grup lewat nomor lain tidak menghapus tanda nomor ini; riwayat lama tidak memunculkan tanda.
- Menandai dari Chat: ketik **@** di grup, pilih anggota (↑ ↓, Enter/Tab, atau klik). Daftar berisi orang yang pernah menulis di grup dan nomor GSI lain di grup itu. Terkirim ke WhatsApp sebagai tag asli, jadi HP anggota mendapat notifikasi tag.
- Migrasi 0043 (`chat_messages.mentions`, `chat_threads.mentioned`); wa-bridge meneruskan `mentionedJid` (LID diterjemahkan ke nomor) dan mengirim tag.

## Fix · DM antar karyawan tampil di Chat, tidak masuk analisa · 2026-10-16
- Pesan Tantri ke Granike ada di HP tapi tidak muncul di Chat: nomor tertaut yang punya pemegang tercatat sebagai nomor internal, dan DM antar nomor internal dibuang.
- DM antar karyawan (nomor internal, nomor tertaut, nomor user/sales) kini disimpan dan tampil di Chat kedua nomor sebagai percakapan **Internal** (chip filter *Internal*, tag INTERNAL). "Internal" berarti tidak masuk analisa: tanpa sinyal, tidak dibaca Analisa WhatsApp/Orchestrator, tidak dihitung *Belum dibalas*, tanpa saran balasan, tanpa identifikasi nomor.
- Percakapan lama dengan nomor yang ternyata nomor GSI diubah jadi Internal (migrasi 0042); audit privasi pilot kini memeriksa DM internal yang masuk analisa. Pesan yang dulu sudah terbuang tidak bisa diambil ulang.

## Change · Pusat kendali: Orchestrator jadi pop-up kanan bawah · 2026-10-16
- Kartu Orchestrator di samping sapaan Pusat kendali diganti pop-up di kanan bawah, sama seperti di Dashboard dan halaman lain. Isinya tetap: status siklus, enam tahap, update terakhir analisis, Analisa WhatsApp terakhir, jumlah chat / pesan masuk / menunggu / keputusan, pilihan MCP Claude atau model AI, Analisis ulang, dan Buka Orchestrator.
- Kartu sapaan kini selebar halaman.
- Pop-up Orchestrator diam di kanan bawah layar di semua halaman (tidak ikut ter-scroll bersama konten): dirender langsung di `body`, karena animasi masuk halaman membuat `position: fixed` menempel pada konten.

## Feat · Analisis ulang di pop-up Orchestrator: sesuai halaman, pilih MCP atau model AI, halaman ikut berubah · 2026-10-16
- Pop-up Orchestrator kini menulis halaman yang dianalisis dan apa yang dijalankan (mis. Push stok → AI Stok · Analisa Stok; Kredit · kas → AI Kredit · AI Penagihan · Analisa Kas).
- Pilih mesin analisa: **MCP Claude** (claude.ai / Desktop) atau **model dari halaman API & model AI** (mis. Claude Opus 5.5 lewat Claude API, atau ChatGPT API) yang langsung jalan di server lewat tool MCP yang sama. Pilihan diingat; model nonaktif bila kuncinya belum diisi atau anggaran harian habis. Tombol "Lewat MCP" yang lama diganti pilihan ini.
- Setelah analisa selesai, halaman yang sedang dibuka ikut diperbarui dengan hasilnya (keputusan, Analisa Stok, Analisa Kas, KPI, orbit, segmen, peta relasi, Analisa WhatsApp), juga saat koneksi real-time terputus.
- Perbaikan: kiriman Analisa Stok/Kas/WhatsApp lewat MCP tidak lagi membuat siklus tampil "selesai" sebelum waktunya. ADR 0042.

## Feat · Pop-up saat memuat data · 2026-10-16
- Saat halaman mengambil data yang belum ada (buka halaman, ganti dealer, buka chat), muncul pop-up di tengah layar dengan animasi orbit 3D — planet GSI Orbit, dua cincin miring dengan satelit — plus apa yang sedang dimuat ("Memuat dealer, chat…") dan bar progres.
- Hanya muncul bila memuat lebih dari ±0,35 detik (tidak berkedip untuk data cepat), tampil minimal 0,5 detik, tidak muncul untuk refresh di latar (pesan masuk, polling), dan tidak menghalangi klik. Mengikuti tema terang/gelap; animasi diam bila perangkat meminta gerak dikurangi.

## Change · Sidebar satu daftar · 2026-10-16
- Menu MCP Claude, API & model AI, AI Trigger, Panduan, dan Pengaturan tidak lagi menempel di bawah sidebar: kini bagian "Pengaturan" di daftar menu yang sama dan ikut scroll di layar pendek. Yang menempel di bawah hanya kartu akun.
- Kartu "Orchestrator · siap" dengan daftar enam agen di sidebar dihapus (status Orchestrator tetap di menu Orchestrator dan bar atas).

## Feat · AI Trigger membaca riwayat chat · warna kartu AI dibedakan · 2026-10-16
- Pesan yang memicu trigger tetapi tidak menyebut produk ("stoknya ada?", "harganya berapa?") kini mengambil produk dari 10 pesan sebelumnya di chat itu (3 hari terakhir, pesan terbaru dulu; pesan pelanggan maupun pesan kita). Kartu menulis "Produk dari riwayat chat: …". Bila beberapa produk cocok, kartu menampilkan pilihannya.
- AI yang menulis draf kini juga membaca riwayat chat (urut lama → baru) untuk konteks dan nada; angka tetap hanya dari data sistem, bukan dari riwayat.
- Kata kunci juga cocok dengan akhiran -nya/-kah/-lah ("stoknya", "harganya").
- Kartu AI Trigger dan catatan Analisa WhatsApp di bawah bubble kini berwarna ungu AI (latar ungu muda, garis ungu di kiri, label "AI Trigger · hanya terlihat tim, bukan pesan"), jelas berbeda dari bubble putih (pelanggan) dan hijau (kita). Draf jawaban tampil dalam kotak bergaris putus-putus.
- Coba kalimat pelanggan di halaman AI Trigger punya kolom "Pesan sebelumnya" untuk mencoba riwayat.

## Feat · Halaman AI Trigger · 2026-10-16
- Menu baru **Pengaturan → AI Trigger** (halaman sendiri, menggantikan kartu Rekomendasi balasan di Pengaturan): daftar trigger dengan kata kunci, aktif/mati langsung dari daftar, dan kotak *Coba kalimat pelanggan* (bisa memilih dealer dan cabang).
- Empat jenis data: **stok** per cabang, **harga** sesuai tier dealer, **limit & tagihan** (sisa limit, tagihan terbuka, tagihan tertua), **status order** terakhir. Trigger bawaan: Tanya stok, Tanya harga, Tanya limit & tagihan, Status order — kata kunci dan draf bisa diubah.
- Satu pesan bisa mendapat beberapa kartu (mis. "stok dan harga kamera 4MP?" → kartu stok + kartu harga).
- **Jawaban ditulis AI** (opsional per trigger, plus instruksi gaya): AI merangkai draf dari angka yang sama; draf yang mengubah atau menambah angka ditolak dan draf template tetap dipakai. AI otomatis menulis untuk pesan baru, atau lewat tombol **Tulis dengan AI** di kartu. Model dan kunci dari halaman API & model AI.
- Tetap tidak ada yang terkirim otomatis: sales menekan *Pakai jawaban*, mengedit, lalu mengirim sendiri. ADR 0041.

## Fix · Nama grup WhatsApp kosong · Feat · Tempel gambar di kolom chat · 2026-10-15
- Gejala: beberapa grup tampil tanpa nama. Penyebab: saat pesan pertama grup masuk, bridge kadang belum bisa membaca info grup (mis. dibatasi WhatsApp saat sinkron riwayat), nama kosong tersimpan dan tidak pernah diperbarui.
- Kini nama kosong tidak menimpa nama yang ada; pesan berikutnya yang membawa nama (atau nama baru setelah grup diganti nama) memperbarui grup dan judul chat-nya di semua nomor GSI. Grup lama tanpa nama diberi nama dari daftar grup tiap nomor (job worker tiap jam, juga saat start). Yang belum terbaca tampil "Grup (nama belum terbaca)", bukan kosong.
- **Tempel (Ctrl/⌘+V) di kolom chat** seperti WhatsApp Web: screenshot atau gambar yang di-copy langsung jadi lampiran (pratinjau + kolom keterangan), lalu kirim. Teks yang di-copy tetap tertempel sebagai teks. Pratinjau lampiran tidak lagi rusak di mode pengembangan.

## Fix · Unduhan media yang gagal & peringatan memo · 2026-10-15
- Media yang tidak bisa dibuka ("Gagal mengambil media dari WhatsApp", `fetch failed`): bila host media yang tersimpan tidak bisa dijangkau, bridge kini mencoba host media bawaan WhatsApp (`mmg.whatsapp.net`), lalu meminta salinan baru ke WhatsApp; penyebab jaringannya tercatat di log bridge.
- "memo not stored" (±17.000 per hari): dealer tanpa sinyal tidak lagi dibaca ulang setiap siklus — ditandai sudah diperiksa dan baru dibaca lagi bila ada sinyal baru. Memo yang sudah ada tidak berubah; peringatan hanya untuk memo yang benar-benar gagal validasi.

## Fix · Hasil review perubahan 15 Okt · 2026-10-15
- Opsi kedua (API): job API pada siklus tidak lagi dihentikan river setelah 1 menit, dan siklus kini menunggu `MCP_WAIT` + 10 menit + jeda (sebelumnya dipotong di menit ke-15) — API benar-benar sempat menjawab.
- Form lama MCP Claude → Analisis terjadwal (model + anggaran) tidak lagi diam-diam mematikan opsi kedua dan model ChatGPT.
- "Tandai sudah/belum dibalas" kini juga berlaku di Pusat kendali (Analisa WhatsApp) dan langkah "Balas chat" di Rencana.
- Balasan dari Chat ke grup yang lebih dulu diterima nomor GSI lain tampil sekali (bukan dua); echo HP yang datang di tengah penandaan terkirim tidak lagi membuat bubble ganda.
- Pesan yang belum terkirim tidak bisa disematkan (sematannya akan hilang saat terkirim).
- Cari di chat menemukan kata yang dipisah baris baru atau spasi ganda.
- Halaman API & model AI tidak lambat saat Anthropic/OpenAI tidak bisa dihubungi (kegagalan disimpan 5 menit).

## Fix · Gambar yang dikirim dari Chat bisa dilihat · 2026-10-15
- Gejala: gambar (dan file lain) yang dikirim dari Chat tidak bisa dilihat di bubble — selama antre kirim hanya tertulis "mengirim…", dan sesudah terkirim bergantung pada unduhan ulang dari WhatsApp.
- Kini file yang dikirim ditampilkan apa adanya dari salinan di server sejak masuk antrean sampai sesudah terkirim (disimpan 30 hari, lalu kembali diambil dari WhatsApp). Media yang diterima tetap tidak disimpan (ADR 0029, revisi).

## Feat · Menu di daftar chat: sematkan chat, label, tandai belum dibaca / belum dibalas · 2026-10-15
- Setiap kartu chat di daftar punya menu ⌄ (muncul saat kursor di atas kartu, atau klik kanan), seperti WhatsApp:
  - **Sematkan chat** / Lepas sematan: chat naik ke paling atas daftar Anda dengan ikon 📌 (per pengguna, maksimal 3 seperti WhatsApp).
  - **Label chat**: pilih atau buat label langsung dari daftar.
  - **Tandai belum dibaca** (badge muncul lagi sampai chat dibuka; chat yang sedang terbuka ditutup) / Tandai sudah dibaca.
  - **Tandai belum dibalas** (masuk filter "Belum dibalas" dan menunggu sampai ada balasan terkirim) / **Tandai sudah dibalas** (pesan pelanggan sampai saat itu dianggap terjawab). Hanya untuk chat pribadi.
- Migrasi 0040; setiap tanda tercatat di audit.

## Feat · Sematkan pesan & Label chat · 2026-10-15
- **Sematkan pesan** (menu bubble → Sematkan): maksimal 3 per chat, sematan ke-4 menggantikan yang paling lama, seperti WhatsApp. Bilah di atas percakapan menampilkan pesan yang disematkan; klik untuk melompat ke pesannya (juga yang lama), × untuk melepas. Bubble yang disematkan bertanda 📌. Sematan hanya untuk tim di GSI Orbit — tidak dikirim ke WhatsApp/HP pelanggan.
- **Label** seperti WhatsApp Business: label bawaan Pelanggan baru, Pesanan baru, Menunggu pembayaran, Sudah dibayar, Pesanan selesai (migrasi 0039). Pasang di **chat** (tombol label di kepala chat → tampil di daftar chat dan bisa difilter lewat "Label") dan di **pesan** (menu bubble → Label → tampil di bawah bubble). Label baru bisa dibuat siapa pun di Chat; menghapus label hanya admin.
- Di grup, sematan dan label sama untuk setiap nomor GSI di grup itu. Semua perubahan tercatat di audit; sematan dan label pesan ikut terhapus saat pesan lewat masa simpan.

## Feat · Cari di dalam chat · 2026-10-15
- Tombol 🔍 di kepala percakapan membuka **Cari di chat ini** seperti WhatsApp: ketik kata atau kalimat, hasil tampil terbaru dulu dengan tanggal, pengirim, dan potongan teks yang ditandai kuning.
- Klik hasil, atau **Enter** (lebih lama) / **Shift+Enter** (lebih baru) / tombol ▲▼, untuk melompat ke bubble-nya; kata yang dicari ikut ditandai di bubble. **Esc** menutup.
- Mencari di semua pesan yang tersimpan (365 hari), bukan hanya 14 hari yang sedang dimuat; grup ikut mencari pesan yang disimpan nomor GSI lain. Melompat ke pesan lama memuat semua pesan dari situ sampai yang sudah tampil, jadi percakapan tetap bersambung. Pesan yang dihapus tidak ikut dicari.

## Change · API & model AI ringkas, semua model, dalam dolar · 2026-10-15
- Halaman diringkas: satu kartu **Model API** (opsi kedua, satu baris per API: kunci + model, anggaran harian) dan satu kartu **Biaya token**; riwayat hanya tampil bila API pernah mengambil alih.
- **Semua model** terdaftar dengan harga per 1 juta token: 12 model Claude (Fable 5.1 … Haiku 4.5) dan 10 model ChatGPT (GPT-5, GPT-5 mini/nano, GPT-4.1 + mini/nano, GPT-4o + mini, o3, o4-mini). Begitu kunci tersimpan, daftar ditambah semua model yang tersedia untuk kunci itu dari Anthropic / OpenAI (model chat saja; harga "?" bila belum diketahui).
- Biaya, harga, dan anggaran harian ditampilkan dalam **USD** (anggaran bawaan $3/hari).
- Model Claude sebelum 4.6 (mis. Haiku 4.5) dipanggil tanpa adaptive thinking, yang tidak didukungnya.

## Feat · API & model AI — Claude API / ChatGPT API sebagai opsi kedua · 2026-10-15
- Halaman baru **Pengaturan → API & model AI** (menu di bawah MCP Claude): kunci Claude API dan ChatGPT API (diperiksa, disimpan terenkripsi), model masing-masing, pilihan **opsi kedua** (Mati / Claude API / ChatGPT API), anggaran harian, dan **total pembayaran token** — hari ini, 7 hari, bulan ini, 30 hari, per provider, grafik harian, per model — serta riwayat saat API mengambil alih.
- Urutan analisis: **Claude lewat MCP tetap utama**. Bila Claude belum menjawab siklus Orchestrator dalam `MCP_WAIT` (10 menit), API pilihan di server mengerjakan agen yang tersisa lewat tool MCP yang sama (validasi, masking, provenance sama). Analisa WhatsApp juga dikerjakan API bila routine claude.ai terlambat > 2½ jam.
- ChatGPT (gpt-5, gpt-5-mini, gpt-4.1, gpt-4.1-mini) lewat Chat Completions + function calling; setiap panggilan tercatat di `llm_calls` dengan token dan biaya.
- Tombol Analisis ulang tidak lagi langsung memakai Claude API; API hanya bekerja sebagai opsi kedua (ADR 0040).

## Fix · Chat antar dua nomor GSI · 2026-10-15
- Gejala: Catharina chat ke Granike, pesan ada di HP Granike tapi tidak muncul di Chat web Granike. Kedua HP membawa id WhatsApp yang sama; pesan tersimpan sekali di thread Catharina dan salinan di nomor Granike dibuang sebagai duplikat.
- Chat 1:1 antara dua nomor GSI yang tersambung kini tersimpan **satu salinan per nomor** (kunci `<id>@<nomor>`), jadi muncul di Chat kedua nomor dan menambah unread di penerima. Pengiriman ulang/riwayat tetap tidak menggandakan; edit dan hapus untuk semua orang mengenai setiap salinan. Grup tidak berubah (tetap satu salinan, dibaca dari semua nomor).
- Balasan dari Chat ke nomor GSI lain tetap menjadi "terkirim" walau HP penerima lebih dulu menyerahkan pesan itu.
- **Realtime**: begitu pesan antar dua nomor GSI tersimpan dari satu sisi (dikirim dari HP atau dari Chat), salinan untuk nomor lawan langsung dibuat di server — tidak menunggu HP penerima meneruskannya. Aturan privasi tetap berlaku (nomor internal, kontak pribadi).
- Pesan lama yang hanya tersimpan di satu sisi dipulihkan otomatis dari salinan yang ada saat worker menyala (365 hari terakhir, dianggap sudah dibaca); tidak perlu ambil ulang riwayat dari HP.

## Feat · Enter kirim, Shift+Enter baris baru · 2026-10-15
- Perbaikan: membuka Chat sempat error "React error #310" — hook ukuran kotak ketik terpanggil setelah return awal saat percakapan masih dimuat.
- Kolom ketik di Chat kini bisa beberapa baris: **Enter** mengirim, **Shift+Enter** baris baru (seperti WhatsApp Web); kotak membesar sampai ±6 baris. Esc tetap membatalkan balas/edit/lampiran.

## Fix · Cari chat · 2026-10-15
- Pencarian memaafkan ejaan nama: J/Y, huruf dobel, oe/u — "yohanes" menemukan "JOHANNES YUDHAYANTO", "yusuf" menemukan "Joesoef".
- Mulai 3 huruf, pencarian menjangkau semua chat yang tersimpan (bukan hanya 14 hari terakhir), dan bisa mencari nomor ("0819…", "62819…", 4 digit terakhir).
- Sapaan di depan diabaikan: "pak yo" menemukan "JOHANNES YUDHAYANTO" (Pak Yohanes di HP Granike).
- Nama kontak yang disimpan di HP ("Pak Yohanes") kini disimpan per chat (migrasi 0038) dan ikut dicari; terisi saat ada pesan baru di chat itu.
- Chat menampilkan nama dari HP bersama nama master, ditautkan lewat nomor telepon chat: **Pak Yohanes - JOHANNES YUDHAYANTO** (daftar chat, kepala percakapan, panel lead, Teruskan).
- **Nama panggilan**: tombol pensil di kepala chat memberi nama seperti di HP ("Pak Yohanes"); disimpan per nomor telepon sehingga berlaku di semua nomor GSI yang memegang chat dengan nomor itu, tidak ditimpa nama dari HP, tercatat di audit (`chat.alias`).
- Bila tetap tidak ketemu, Chat menjelaskan bahwa chat lama di HP yang tidak ikut tersinkron saat nomor ditautkan belum ada di Orbit.

## Fix · Status dibaca di Chat · 2026-10-15
- Membuka chat langsung menghapus jumlah pesan belum dibaca di daftar, chip "Belum dibaca", badge menu Chat, dan badge per nomor.
- Sebelumnya hitungan bisa kembali/tertahan: daftar chat memakai ulang query yang dimulai sebelum status baca tersimpan (perbaikan performa pagi ini). Kini setiap perubahan lewat API memaksa query baru.
- Grup yang dipegang beberapa nomor GSI: dibaca di satu nomor = terbaca di semua nomor.
- Membalas dari HP (chat pribadi) menandai chat sudah dibaca, seperti WhatsApp.
- Belum: membaca di HP *tanpa* membalas belum menghapus hitungan di Orbit (butuh event baca dari wa-bridge — `docs/OPEN-QUESTIONS.md`).

## Feat · Teruskan pesan · 2026-10-15
- Menu bubble di Chat → **Teruskan**: pilih hingga 5 chat tujuan (bisa dicari). Pesan dikirim dari nomor masing-masing chat tujuan.
- Teks diteruskan apa adanya. Foto, video, audio, dan dokumen diambil dari WhatsApp lalu dikirim ulang beserta keterangannya. Polling, lokasi, kontak, dan stiker diteruskan dari HP.
- Setiap penerusan adalah keputusan manusia (proposal disetujui pengirim → outbox), tercatat di audit sebagai `chat.forward` dengan asal pesan. Chat yang tidak boleh dilihat pengguna tidak bisa diteruskan.
- Pesan di HP penerima belum berlabel "Diteruskan" (butuh perubahan wa-bridge; ditunda agar bridge tidak di-restart lagi hari ini).

## Change · Chat memuat 14 hari terakhir · 2026-10-15
- Daftar chat hanya memuat percakapan yang aktif 14 hari terakhir.
- Percakapan dibuka dengan pesan 14 hari terakhir; pesan yang lebih lama dimuat saat menggulir ke atas.
- Riwayat yang diunduh dari HP dibatasi 14 hari. Backfill setahun penuh (±1.200 permintaan/jam) yang membebani server dihentikan.
- Data lama tetap tersimpan (retensi 365 hari). Jendela bisa diubah lewat `CHAT_WINDOW_DAYS`.
- ADR 0039.

## Fix · wa-bridge berhenti crash-loop karena antrean riwayat 513 MB · 2026-10-15
- Sejak 14 Okt 04:48 wa-bridge crash tiap ±27 detik (2.923 kali/24 jam): antrean batch riwayat yang timeout di API membengkak melewati batas string Node dan dibaca sekaligus saat start. Setiap crash = semua HP tertaut reconnect (risiko anti-ban, ADR 0004).
- Antrean kini dibaca baris per baris; batch riwayat besar mendapat timeout lebih panjang (30 dtk + 20 dtk/MB, maks 3 menit) sehingga tidak masuk antrean berulang-ulang.
- Isi antrean lama sudah tersimpan di DB (sampel 38/40; 2 sisanya sengaja dibuang: DM antar nomor internal dan pesan > 365 hari); file backlog dihapus atas persetujuan.
- Akar masalah: API menyimpan pesan semua nomor dalam satu jalur, sehingga batch riwayat menunggu > 30 dtk saat beberapa nomor sinkron bersamaan; bridge mengirim ulang batch yang sebenarnya tetap diproses (tiap batch tercatat 3×). Kini pesan disimpan paralel dalam 4 jalur per chat (urutan per chat tetap), API berhenti memproses batch yang sudah ditinggal bridge, dan batch riwayat diperkecil 200 → 50 pesan.
- Tautkan nomor baru: 5 percobaan scan pada 14 Okt sore macet karena crash-loop di atas (QR mati sebelum discan). QR yang tidak diperbarui bridge lebih dari 3 menit kini tampil "Waktu menautkan habis — mulai lagi" dengan tombol **Mulai lagi**, bukan QR mati.
- Daftar chat lambat (10–18 dtk sejak sinkron riwayat 14 Okt): satu query daftar ±1,7 dtk dijalankan ulang oleh setiap tab Chat pada setiap pesan baru. Kini permintaan yang datang bersamaan berbagi satu query; permintaan setelah pesan baru selalu memakai query baru, jadi daftar tidak pernah basi.
- `deploy.sh` kini menyebut alasan restart wa-bridge (berubah, atau sedang tidak berjalan).

## Change · Filter chat lebih ringkas · 2026-10-15
- Chip filter di daftar chat kini berupa ikon + jumlah; chip yang aktif juga menampilkan namanya, nama lengkap muncul saat kursor diarahkan.
- Panel lead kembali di kolom tengah (antara daftar chat dan percakapan) pada layar laptop 1100–1280px; hanya di bawah 1100px ia melayang di atas percakapan.
- Label "WHATSAPP BAILEYS" di kepala daftar chat dihapus, begitu juga sebutan Baileys di subjudul menu Chat.

## Change · Analisa WhatsApp tiap 2 jam lewat routine Claude di claude.ai (tanpa kunci API) · 2026-10-14
- Jadwal "Analisa WhatsApp · tiap 2 jam" kini dijalankan routine Claude di claude.ai memakai konektor CRM DISTRI dan langganan Claude — server tidak lagi butuh kunci Claude API untuk ini.
- Setiap analisa yang dikirim Claude tercatat sebagai riwayat di MCP Claude; label Pusat kendali hijau saat masuk, merah "terlambat" bila lebih dari 2½ jam tidak ada analisa, merah "gagal" bila kiriman ditolak.
- ADR 0037 (revisi), migrasi 0037.

## Feat · Rekomendasi balasan dari data stok · 2026-10-14
- **Pengaturan → Rekomendasi balasan**: aturan kata kunci (bawaan "Tanya stok": stok, ready, ada barang, tersedia, …) dengan draf jawaban untuk stok ready dan kosong; bisa ditambah, diubah, dimatikan, dan dicoba dengan kalimat pelanggan.
- **Di Chat**: pesan pelanggan yang memuat kata kunci mendapat kotak rekomendasi di bawah bubble — produk yang dikenali, stok di cabang chat, cabang lain yang ada stoknya, jam sinkron stok, dan draf jawaban. **Pakai jawaban** mengisi kolom ketik; sales mengedit dan mengirim sendiri. Tidak ada yang terkirim otomatis.
- ADR 0038, migrasi 0036.

## Feat · Analisa WhatsApp otomatis oleh Claude (MCP) setiap 2 jam · 2026-10-14
- Jadwal baru **"Analisa WhatsApp · tiap 2 jam"**: Claude membaca semua chat lewat MCP dan menulis ringkasan, baris per percakapan, dan catatan per pesan untuk Pusat kendali (siklus khusus `screen:wa`, tanpa agen lain). Tidak ada pesan yang dikirim ke pelanggan.
- **Label di Pusat kendali**: "Analisa Claude berhasil · jam" / "gagal · alasan" / "sedang menganalisa", jadwal berikutnya, dan tautan Riwayat.
- **Riwayat di halaman MCP Claude**: setiap run dengan hasil (berhasil/gagal), keterangan, biaya, dan laporan lengkap. Run dihitung berhasil hanya bila analisa Claude benar-benar masuk.
- ADR 0037, migrasi 0035.

## Fix · Share of wallet, omzet, dan margin (laporan direktur, tugas 11–13) · 2026-10-14
- **Share of wallet** tidak lagi 50 untuk semua: tanpa konfirmasi sales, diestimasi dari riwayat belanja dealer (3 bulan terakhir ÷ 3 bulan terbaik setahun). Dealer dengan riwayat < 6 bulan tampil "belum diketahui" dan tidak lagi otomatis Key account.
- **Omzet dashboard** kini omzet aktual bulan lalu + bulan berjalan dari faktur (bukan rata-rata 6 bulan); sumber data dan jam sinkron tampil di dashboard dan di KPI/MCP. Impor menerima DPP (tanpa PPN), faktur batal, dan retur penjualan.
- **Margin**: HPP dari stok kini per cabang (rata-rata tertimbang), HPP total-baris dibaca per satuan, rebate per merek bisa diatur; tool MCP `margin.cek` untuk rekonsiliasi per SKU dengan keuangan.
- ADR 0036.

## Feat · Laporan direktur 14 Okt — gelombang 1: antrean chat bersih, waktu respon jam kerja, privasi · 2026-10-14
- **Antrean "Belum dibalas" hanya urusan niaga**: penutup sopan ("ok bu", "siap pak terima kasih"), emoji, dan stiker tidak lagi membuat chat menunggu; balasan otomatis WhatsApp Business (sapaan, "sedang tidak di tempat") bukan jawaban dan bukan permintaan.
- **Tipe kontak** di panel Chat: *Pemasok* (chip sendiri, keluar dari antrean dan analisa lead) dan *Obrolan pribadi* (dihapus dari CRM, pesan berikutnya tidak disimpan).
- **Waktu respon per jam kerja** (Senin–Sabtu 08.00–17.00 WIB): median dan **p90** jawaban substantif per sales; pesan di luar jam kerja dilaporkan terpisah; "lewat 2 jam" dihitung dalam jam kerja.
- **Kredensial** (password, sandi, PIN, OTP) disamarkan di Chat, MCP, dan sebelum ke AI.
- ADR 0035, migrasi 0034.

## Fix · Waktu respon dihitung sampai pelanggan dijawab · 2026-10-14
- Di semua chat, jawaban sementara dari pihak GSI — "tunggu sebentar ya", "saya konfirmasi dulu", "saya tanyakan ke gudang", "nanti saya kabari", "sedang kami cek" — **tidak lagi dihitung sebagai balasan** (ADR 0034, migrasi 0033).
- Chat tetap di "Belum dibalas" dan timer menunggu terus berjalan sampai ada jawaban sungguhan; waktu balas per sales di Analisa WhatsApp dan Pusat kendali dihitung sampai jawaban itu.
- Bubble jawaban sementara diberi tanda "jawaban sementara · waktu respon tetap berjalan"; Claude (MCP) juga diberi tahu bahwa pelanggan belum dijawab.

## Fix · Grup pelanggan masuk otomatis; Penagihan hanya melihat WhatsApp-nya · 2026-10-14
- Grup kini dianggap **grup pelanggan kecuali semua anggotanya nomor GSI**. Sebelumnya hanya grup yang anggotanya cocok dengan kontak pelanggan master — padahal WhatsApp menyembunyikan nomor anggota grup (LID), jadi hampir tidak ada yang masuk. Grup bernama gudang/internal/karyawan/staff/HRD/absensi tetap internal; pilihan manual tetap. Grup yang sudah ada diperbarui (migrasi 0032).
- Pesan karyawan GSI di grup (nomor internal, tertaut, pengguna, sales) dihitung sebagai balasan.
- **Penagihan** (peran finance) hanya melihat chat, nomor, dan Analisa WhatsApp dari nomor yang dipegangnya, seperti sales; admin melihat semua. Peran finance mendapat menu Chat untuk membalas manual.
- Tagihan tidak dikirim otomatis — Penagihan membalas manual.

## Feat · WhatsApp Admin & Penagihan terpisah dari Sales, grup pelanggan dibaca · 2026-10-14
- Setiap nomor WhatsApp punya **tim**: Sales, Admin, atau Penagihan. Dipilih saat menautkan (Chat → + Nomor), bisa dipindah di detail nomor; nomor lama mengikuti peran pemegangnya (migrasi 0031, ADR 0033).
- Rail nomor di Chat dikelompokkan **SALES · ADMIN · PENAGIHAN**; klik judul tim untuk melihat semua chat tim itu.
- **Grup pelanggan**: grup berisi kontak pelanggan dikenali otomatis; grup lain bisa ditandai dari panel Chat ("Tandai grup pelanggan"). Analisa WhatsApp dan *Analisa lanjut* (Claude lewat MCP) kini membaca grup pelanggan — satu percakapan per grup, pesan karyawan GSI dihitung sebagai balasan, nama pelanggan dimasking. Grup internal tetap tidak dibaca.
- MCP `chat.thread` menerima `thread_id` (grup pelanggan bisa dibaca); `wa.analisis` menyertakan jenis dan tim.
- Perbaikan: masker PII tidak lagi mengira id percakapan (UUID berakhiran 12 angka) sebagai nomor rekening.

## Change · Pusat kendali kembali ke kartu semula, tanpa ruang kosong · 2026-10-13
- Tata letak bertab dibatalkan (permintaan Sam): Pusat kendali kembali ke kartu semula (sapaan + Orchestrator, Keputusan, Ringkasan Claude, Menunggu balasan, KPI WhatsApp, Agenda sales).
- Ruang kosong dirapikan: kartu sapaan setinggi kartu Orchestrator (isi di tengah), kartu Orchestrator dan KPI WhatsApp dibuat lebih padat (chip pipeline terlipat, roda lebih kecil), kedua kolom berakhir sejajar, kotak "Tidak ada keputusan" lebih kecil.

## Change · Pusat kendali ringkas (dibatalkan) · 2026-10-13
- Pusat kendali dirapikan dari tujuh kartu menjadi empat bagian: satu baris kepala (sapaan, analisis terakhir, Analisis ulang, Chat, Orchestrator), satu baris angka (Keputusan, Menunggu, Belum dibalas, Chat terbalas, Waktu balas, Nomor baru), ringkasan Claude (MCP) satu strip, dan **satu kartu bertab**: Keputusan · Menunggu balasan · Per sales.
- Klik kotak angka membuka tab yang sesuai; tab tersimpan di URL (`?tab=`). Angka yang dulu muncul berulang (sapaan, strip status, kartu Orchestrator, roda KPI, kaki ringkasan) kini sekali saja.

## Change · Analisa hanya dari Claude lewat MCP · 2026-10-13
- Orchestrator, Pusat kendali, Chat, Push stok, Kredit · kas kini hanya menampilkan analisa yang ditulis **Claude lewat MCP** (usulan, ringkasan WhatsApp, ringkasan per percakapan, catatan per bubble, Analisa Stok/Kas) plus angka yang diukur dari data (ADR 0032).
- Dihapus: deteksi "permintaan" berbasis kata kunci (bar, matriks, filter, kartu Minta order / Tanya stok & harga / Rilis kredit & retur), feed "6 agen AI", Rencana hari ini dari chat, baris AI di grid sales, anotasi dan "Rekomendasi balasan (AI)" contoh, tag chat, "pesan penutup", "minta tempo", saran template di panel lead.
- Backend: jalur analisis `mcp` + wajib AI (migrasi 0030); tanpa fallback template, tanpa tagihan otomatis dari aturan, tanpa ringkasan WhatsApp/halaman dari model server dalam mode MCP. Tombol Analisis ulang di halaman-halaman ini lewat MCP.

## Feat · Chat: kirim file, pesan suara, dan polling · 2026-10-13
- Tombol **📎** di kolom ketik: **Dokumen**, **Foto & video**, **Polling**. File tampil sebagai pratinjau dan kolom ketik menjadi keterangannya (maks. 25 MB).
- Tombol **mikrofon** saat kolom ketik kosong (seperti WhatsApp): rekam, batal, atau kirim **pesan suara**; bridge mengubah rekaman browser ke ogg/opus (`ffmpeg`) agar diputar di HP pelanggan.
- **Polling**: pertanyaan + 2–12 pilihan, satu atau beberapa jawaban; polling masuk dari pelanggan/grup ikut tampil.
- Semua lewat outbox, persetujuan pengirim, dan penjaga anti-blokir; file disimpan sementara (`chat_outgoing_files`, migrasi 0029) sampai terkirim, lalu bubble memakai referensi unduh WhatsApp (ADR 0031).
- nginx: batas upload 30 MB, mikrofon diizinkan untuk situs ini, CSP `media-src blob:` (sebelumnya video/VN di bubble tidak bisa diputar di produksi).

## Feat · Chat: menu per bubble — Balas, Salin, Edit, Hapus untuk semua orang · 2026-10-13
- Tiap bubble punya menu **⌄** di pojoknya (seperti WhatsApp Web): **Balas**, **Salin**, **Edit** (pesan teks dari nomor ini, ≤ 15 menit) dan **Hapus untuk semua orang** (pesan dari nomor ini, ≤ 48 jam). Tombol balas cepat tetap di samping bubble.
- Hapus: pesan yang masih antre dibatalkan sebelum terkirim; yang sudah terkirim menjadi keputusan manusia (proposal + audit `chat.delete`) yang dikirim lewat outbox dan bridge (`POST /sessions/{id}/delete`). Bubble menjadi "Anda menghapus pesan ini"; teks asli tetap tersimpan untuk audit.
- Penghapusan oleh pelanggan atau dari HP ikut tercatat: bubble menjadi "Pesan ini dihapus" (migrasi 0028 `chat_messages.deleted_at`). Kutipan yang menunjuk pesan terhapus ikut tertulis "Pesan ini dihapus".
- Perbaikan tampilan banner "Membalas …" / "Edit pesan" di kolom ketik.

## Feat · Chat: panel lead bisa diringkas · 2026-10-13
- Tombol × di panel lead diganti **Ringkas**: panel menjadi strip sempit yang tetap terlihat — avatar dengan warna tahap, tahap (teks tegak), peringatan, ikon tab (Analisa AI, Sumber, Data Accurate, Statistik) dan Kirim ke Accurate. Klik panah atau ikon mana pun untuk memperbesar lagi (langsung ke tab itu). Pilihan diingat di browser. Di HP panel tetap berupa lembar bawah.

## Feat · Chat: Balas (reply) per bubble seperti WhatsApp · 2026-10-13
- Tombol **Balas** di tiap bubble (saat kursor di atas pesan): kolom ketik menampilkan "Membalas …", dan balasan terkirim sebagai reply WhatsApp sungguhan — di HP pelanggan pesan aslinya tampil terkutip. Tetap keputusan manusia lewat outbox dan pemeriksaan persetujuan bridge.
- Balasan dari pelanggan atau dari HP ikut tercatat (bridge membaca kutipan dari semua jenis pesan: teks, foto, VN, dokumen), dan bubble menampilkan kotak kutipan; klik kutipan untuk loncat ke pesan aslinya.
- Migrasi 0027 `chat_messages.quoted_wa_id`; `POST /chat/threads/{id}/messages {body, reply_to}`.

## Fix · Analisa lanjut di Chat: Claude (MCP) menganalisa percakapan itu, termasuk per bubble · 2026-10-13
- Tombol **Analisa lanjut** kini menganalisa percakapan yang dibuka (scope baru `thread:<id>`; chat dealer tetap scope dealer) dan selalu lewat MCP.
- Pembacaan Analisa WhatsApp untuk Claude kini juga dibuat untuk siklus satu percakapan/dealer/chat (sebelumnya hanya siklus penuh, sehingga Claude tidak bisa menulis catatan per bubble). Prompt MCP meminta catatan per pesan pelanggan (`messages`). Dengan model di server, catatan per bubble juga ditulis di siklus non-penuh.
- Lembar MCP menjelaskan: Claude Desktop/claude.ai hanya bekerja bila perintahnya dikirim di sana; agar langsung jalan tanpa membuka Claude, simpan kunci Claude API (Analisis terjadwal).

## Feat · Push stok dan Kredit · kas dianalisa Claude lewat MCP · 2026-10-13
- **Analisa Stok** dan **Analisa Kas** (ADR 0030), seperti Analisa WhatsApp: Claude membaca stok menua dan piutang/prediksi kas lewat MCP (`orchestrator_input_get` → `orchestrator_submit` dengan `overview` + `lines`), lalu ringkasan dan catatan per SKU/dealer tampil di halaman bertanda "ditulis AI · siklus #n". Angka tetap dihitung GSI Orbit.
- Tombol **Analisis ulang** di kedua halaman kini lewat MCP (membuka lembar Claude, atau Claude di server bila kunci API tersimpan).
- Saran aturan tetap dihapus: "Usul: diskon atau retur / bundle tier C" di Push stok dan "Pengingat H-3 otomatis" di Kredit · kas (GSI Orbit tidak mengirim pesan otomatis). Tanpa analisa AI halaman menulis "Belum dianalisis AI".
- Migrasi 0026 `screen_analyses`; endpoint `GET /screen-analysis/{stock|credit}`.

## Feat · Chat: Belum dibalas, foto/video/VN di bubble, edit pesan · 2026-10-13
- Chip **Belum dibalas** di daftar chat: percakapan dealer dan nomor baru yang pesan pelanggannya belum dijawab (grup tidak dihitung); chat yang dibuka tetap tampil sesudah dibalas.
- **Foto, video, pesan suara, stiker, dan dokumen** dari pelanggan tampil di bubble (ADR 0029). Isinya tidak disimpan: bridge menyimpan referensi unduh, file diambil dari WhatsApp saat dibuka (foto saat terlihat, video/VN/dokumen saat diklik). Foto bisa dibuka besar dan diunduh.
- **Edit pesan** seperti WhatsApp: tombol pensil di pesan keluar — yang masih di antrean diubah sebelum terkirim, yang sudah terkirim bisa diedit sampai 15 menit (dicatat sebagai keputusan manusia, lewat outbox dan bridge). Edit dari pelanggan atau dari HP ikut memperbarui bubble, ditandai "diedit" (migrasi 0025).
- Media dari pesan lama baru bisa dibuka setelah WhatsApp mengirimnya lagi lewat sinkron riwayat; sampai itu tampil "buka di ponsel".

## Fix · jalur analisis bisa diganti lagi; Claude boleh menjawab "tidak ada usulan" · 2026-10-13
- **Pengaturan → jalur analisis (api/mcp/both) selalu ditolak 400** sejak `require_ai` ditambahkan (ADR 0025): skema `llm.routing` belum mengenal kolom itu. Skema diperbaiki; test baru memeriksa setiap kebijakan default lolos skemanya setelah disimpan ulang.
- **Claude lewat MCP kini bisa mengirim `proposals: []`** (tidak ada usulan untuk agen itu). Sebelumnya ditolak tanpa alasan, sehingga dengan wajib-AI agen itu menunggu 10 menit lalu gagal dan siklus menjadi "sebagian".
- Test: `TestToolsAndScopes` (macet 10 menit) kini memerankan Claude yang menjawab tiap agen seperti di produksi; e2e mengikuti dialog hasil Analisis ulang dan tampilan Orchestrator yang baru. `make check` dan e2e (26/26, data contoh segar) hijau.

## Feat · Chat: filter Belum dibaca dan riwayat WhatsApp 1 tahun · 2026-10-13
- Chip **Belum dibaca** di daftar chat (seperti WhatsApp): semua jenis percakapan yang punya pesan belum dibaca, dengan jumlahnya; chat yang sedang dibuka tetap tampil sesudah dibaca.
- Membuka percakapan langsung menampilkan **seluruh riwayat yang tersimpan (1 tahun)** — tombol "Muat pesan sebelumnya" dihapus; chat yang sangat ramai (> 3000 pesan) memuat sisanya otomatis saat digulir ke atas.
- **Retensi chat 90 → 365 hari** (migrasi 0024; tetap bisa diubah di Pengaturan). Tanpa ini riwayat lama terhapus lagi tiap malam.
- Backfill menjaga jeda 4,5 detik per nomor sendiri (40 permintaan per putaran, ±1200/jam untuk 3 nomor). Di produksi, putaran pertama hanya 3 dari 20 permintaan yang lolos: sisanya ditolak penjaga laju bridge (4 detik per nomor).
- **Sinkron riwayat 1 tahun** (ADR 0028): bridge meneruskan riwayat 365 hari (`BRIDGE_HISTORY_DAYS`) dan meminta riwayat penuh saat pairing; job `wa.backfill` tiap 2 menit meminta HP 50 pesan lebih lama per percakapan (`fetchMessageHistory`) sampai 1 tahun atau sampai HP tidak punya lagi — nomor yang sudah tertaut tidak perlu scan ulang. `WA_BACKFILL_DAYS` default 365.

## Fix · tidak ada pesan otomatis; grup lengkap di setiap nomor · 2026-10-12
- Daftar chat diurutkan **chat terbaru di atas** (seperti WhatsApp). Sebelumnya diurutkan per jenis dulu (dealer → grup → nomor baru), sehingga chat dealer lama selalu di atas dan chat hari ini tertimbun.
- **GSI Orbit tidak pernah mengirim pesan WhatsApp sendiri**, termasuk alert ke grup internal (permintaan Sam). Alert hanya tercatat di aplikasi (Pengaturan → Status sistem, `system_alerts`). Tiga lapis: Notifier tidak membuat pesan, outbox menolak baris `wa_system`, bridge menolak kiriman tanpa proposal yang disetujui manusia (pengecualian grup internal dihapus).
- Grup yang beranggotakan beberapa nomor GSI kini tampil **lengkap di setiap nomor**: pesan disimpan sekali (di nomor yang menerimanya pertama), thread grup membaca pesan dari semua thread grup itu; waktu dan pesan terakhir di daftar chat ikut bergerak. Sebelumnya grup di nomor Catharina tampak bolong karena sebagian pesan tersimpan di thread Elisa/Granike.

## Fix · chat WhatsApp tidak sinkron (server kewalahan) · 2026-10-12
- Gejala: chat Catharina terus berjalan di HP tapi tertinggal di sistem. Bridge mencatat "forward failed, queued" (API/worker tidak menjawab dalam 10 detik) dan pesan menumpuk di antrean bridge.
- Penyebab: tiap pesan masuk dari dealer membuat satu job `metrics.recompute` tanpa penggabungan; tiap job menghitung ulang PIC aktif **semua 2,5 ribu kontak** tanpa index (11,8 dtk di data produksi) di bawah satu kunci. Riwayat nomor yang baru tersambung (ribuan pesan) menumpuk puluhan job, koneksi database habis, pemilihan leader River gagal, forward bridge timeout.
- Bridge: antrean yang sedang di-flush saat bridge restart kini dikirim lebih dulu — sebelumnya ditimpa antrean berikutnya (pesan hilang); angka antrean di /health kini ikut menghitung batch yang sedang di-flush (sebelumnya 0 padahal 22 ribu pesan menunggu).
- Perbaikan: (1) index `chat_messages(from_number, sent_at)` (migrasi 0022, 11,8 → 2,7 dtk); (2) hitung ulang PIC hanya untuk kontak dealer terkait, penuh tiap 10 menit; (3) pesan dalam 20 detik digabung menjadi satu job (`DealerBatcher`); (4) browser menggabungkan refetch dari event SSE (sebelumnya ±5 refetch daftar chat per detik per tab).

## Fix · nomor WhatsApp yang terlepas dari HP tetap ber-alert · 2026-10-12
- Masa tenggang 3 menit: setelah restart bridge nomor tampil "unpaired" beberapa detik sebelum tersambung — bukan lagi alert "perangkat tertaut dihapus" (kolom `wa_numbers.state_since`, migrasi 0023).
- **Alert WhatsApp ke grup internal kini benar-benar terkirim.** Sejak 9 Okt semua 65 alert ditolak penjaga kirim ("kirim tanpa outbox yang disetujui ditolak"): alert sistem dikirim tanpa id outbox dan bridge hanya menerima kiriman proposal yang disetujui. Kini baris `wa_system` ke grup **internal** disetujui bridge (tetap tidak pernah ke dealer); bila penjaga anti-blokir meminta tunggu, alert dijadwalkan ulang, bukan gagal.
- Alert "Siklus gagal" tidak lagi bolak-balik "Pulih" tiap jam: siklus yang masih berjalan tidak memutus hitungan gagal beruntun.
- Deploy hanya me-restart wa-bridge bila build, unit, atau pengaturan WA/bridge berubah (12 Okt: 9 restart sehari; tiap restart = sambung ulang semua HP tertaut).
- Nomor yang perangkat tertautnya dihapus dari HP (`device_removed` / logout) setelah restart bridge tampil "pairing" (QR baru) dan alert "WhatsApp terputus" hilang — padahal chat tidak sinkron. Kini nomor yang pernah terpasang lalu kembali "pairing"/"unpaired" dihitung terputus sampai QR dipindai ulang; pesan alert menyebut penyebabnya.

## Chat · analisa per bubble · 2026-10-12 (ADR 0027)
- Setiap analisa (siklus per jam, *Analisis ulang*, *Analisa lanjut* di Chat, analisa per dealer) kini juga menganalisa **setiap bubble** chat dealer & nomor baru: permintaan + barang, dibalas berapa menit kemudian / menunggu berapa lama / pesan penutup, dan untuk pesan sales berapa lama pelanggan menunggu.
- Catatan AI per pesan pelanggan ditulis Claude lewat MCP (`orchestrator_submit` → `messages`) atau model di server.
- Di halaman Chat: baris analisa di bawah tiap bubble, tombol ✦ *Per pesan* untuk tampil/sembunyi; status balasan selalu terkini.
- *Analisa lanjut* untuk nomor baru tidak lagi gagal (scope `screen:chat`). Migrasi 0021 `wa_message_insights`.

## Dashboard · riwayat di setiap kartu · 2026-10-12
- Kartu Dashboard (Dealer di Orbit sampai Kas masuk · prediksi 30 hari) kini punya grafik batang + garis: **12 bulan** terakhir atau **Per hari** (tanggal 1 sampai akhir bulan, pilih bulannya). Arahkan kursor/ketuk batang untuk nilainya dan perubahan dari titik sebelumnya (hijau = membaik, merah = memburuk).
- Setiap titik dihitung ulang di Go dengan rumus `metrics` pada riwayat order/invoice/pembayaran yang dipotong di tanggal itu (bayar sesudahnya dianggap belum terjadi); omzet dan kas masuk = nilai aktual di bulan/hari tersebut. API `GET /api/dashboard/trend`, dataset 2 tahun di-cache 5 menit.
- Stok menua dan perputaran stok belum punya riwayat (data stok hanya posisi saat ini) — kartunya menyatakan itu.

## Analisa WhatsApp + BigQuery · 2026-10-12 (ADR 0026)
- Pusat kendali dan Orchestrator kembali ke **tata letak awal**, tetapi seluruh analisanya dari **WhatsApp** (tab BigQuery dihapus). Pusat kendali: Rencana ("Balas chat"), Keputusan dari chat, Ringkasan WhatsApp, Menunggu balasan, Minta order, KPI WhatsApp (chat terbalas, waktu balas, menunggu), Agenda sales per chat menunggu, Tanya stok & harga, Rilis kredit & retur. Orchestrator: Pipeline, Analisa WhatsApp, Percakapan, Riwayat baca WhatsApp, kartu per sales, MCP, Matriks permintaan.
- Label **Analisa WhatsApp terakhir** (jam, berapa lama lalu, siklus, jumlah chat) di Pusat kendali dan Orchestrator; oranye bila > 2 jam.
- "Balas chat" masuk Rencana sejak tahap Ingest — tetap ada walau siklus gagal di Analisis.
- Ringkasan AI WhatsApp lewat Claude MCP (tanpa API key): agent `Analisa WhatsApp` di `orchestrator_input_get` / `orchestrator_submit`.
- Orchestrator membaca chat WhatsApp 7 hari terakhir di setiap siklus penuh (`internal/waanalysis`): permintaan per percakapan, barang, chat yang menunggu balasan, waktu balas per sales; disimpan di `wa_analyses` / `wa_insights` (migrasi 0020) dengan provenance sinyal dan usulan.
- Chat yang menunggu > 2 jam tanpa usulan agen masuk **Rencana hari ini** sebagai "Balas chat" (tombol Buka chat, selesai otomatis setelah dibalas, terbatas per sales).
- Ringkasan AI per siklus dan per percakapan hanya bila ditulis model (PII dimasking, tercatat di `llm_calls`).
- API `GET /api/wa/analysis`, tool MCP `wa.analisis`; teks tahap Ingest kini "WhatsApp … · BigQuery …".

## Stage 00 — Fondasi · 2026-10-05
- `cmd/arc` satu binary: `api` (chi, `/api/health`, `/api/me`), `worker` (river, heartbeat tiap menit → `audit_log`), `ctl` (`migrate`, `seed [--if-empty]`, `reset`, `counts`).
- Migrasi goose `0001_init.sql` (semua tabel `03-data-model.md` + `commitments`, view `v_dealer_board`) dan `0002_river.sql`.
- sqlc (`db/queries` → `internal/store/gen`), store + transaksi, `internal/testdb` (skema terisolasi per test).
- Seed 18 dealer, 4 sales + CEO/admin/finance, 379 SO, 379 invoice, 85 sinyal (68 WA), stok 4 cabang — digenerasi dari data contoh mockup (`tools/seedgen`) dan idempoten.
- `web/`: Vite + React 19 + TS, `tokens.css` + `mockup.css` disalin verbatim dari mockup, font self-host, proxy `/api`.
- `Makefile` (`dev`, `check`, `test`, `lint`, `migrate`, `seed`, `sqlc`, `seedgen`, `e2e`), `.golangci.yml`, CI `check.yml`, `infra/docker-compose.yml`, `.env.example`. ADR 0006.

## Stage 01 — Metrics engine & API baca · 2026-10-05
- `internal/domain` (tipe murni + PolicySet + akar terduga) dan `internal/metrics` (semua rumus `01-glossary.md`, fungsi murni).
- Uji: 16 kasus tabel glossary, uji properti (skor 0–100, status monoton, ambang segmen), regresi seed — 18 dealer persis mockup (status, segmen, sisa limit, skor + 5 komponen, jadwal order).
- `internal/dealersvc` (muat riwayat → `metrics_current` + `dealer_metrics_daily`), job river `metrics.recompute` (10 mnt) & `metrics.snapshot` (00.30 WIB); `arc ctl recompute`, `arc ctl metrics --dealer`.
- API: `/sales`, `/dealers` (+ pencarian trigram), `/dealers/{id}` (+ orders, mix, credit, contacts, commitments, timeline, memo), `/dealers/due|drift|credit-tight`, `/orbit` (+ summary, movers), `/segmen` (+ summary, movers dari snapshot 90 hari / `metrics_prev.json`), `/kpi`, `/agenda`, `/stock/aging|critical|push|sales-by-product`, `/credit/overview|dealers|exposure|forecast`.

## Stage 02 — Frontend shell, Pusat kendali (baca), Orbit, Segmen, Dealer · 2026-10-05
- `web/src/app`: router semua rute 08-frontend, Shell (Rail, Topbar + view tabs Orbit/Segmen/Peta relasi, ⌘K, Tabbar, Dock), React Query, SseProvider (`/api/events`, heartbeat 25 dtk).
- Komponen mengikuti kelas CSS mockup (`tokens.css` + `mockup.css` verbatim): Pill, ScoreRing, Toast, Sheet, ActBtn.
- Layar: Pusat kendali (baca), Orbit (port `renderOrbit`, geometri teruji), Segmen (port `renderKuad`, skala log teruji), Dealer (header KPI, anchors, order-to-cash, share of wallet & product mix, sisa limit, memori, PIC aktif, komitmen, timeline). Layar tahap berikutnya berupa placeholder berlabel tahap.
- API: `/api/brief/today` (ringkasan 4 poin, template dari metrik), `/api/events` (Postgres LISTEN/NOTIFY → SSE), `sample_data` di `/api/health`; urutan dealer & sales mengikuti id Odoo.
- Uji: vitest (fmtRp, parser ⌘K, geometri orbit 5 kasus, skala segmen), Playwright smoke 6 rute × 1440/390 + screenshot.

## Stage 03 — WhatsApp ingest & Chat · 2026-10-05 (done-with-fakes)
- `internal/wa`: `Transport` (fake, whatsmeow linked device + backfill, Cloud API), ingest → `chat_threads`/`chat_messages`/`signals` idempoten, filter privasi, NOTIFY `chat_message`/`wa_status`.
- `internal/outbox`: kirim hanya baris outbox untuk proposal disetujui; batas harian per nomor, jeda acak, balasan manusia dengan jeda mengetik (ADR 0007).
- Migrasi `0003_whatsapp.sql` (wa_numbers, skema whatsmeow, thread unik per nomor sales, status pesan); seed nomor internal, grup gudang, 4 nomor sales, 6 thread mockup, identifikasi nomor baru.
- API: `/chat/threads` (+ detail, read, context, messages), `/internal-numbers`, `/wa/groups`, `/wa/status`, `/wa/pair`, `/wa/cloud/webhook`, `/policies`. `arc ctl wa inject|numbers`.
- Web: Chat 3 panel (tab, anotasi agen, konteks dealer/grup/nomor baru, balas sebagai sales), Pengaturan (kebijakan dari DB, sumber sinyal, panel WhatsApp: QR, nomor internal, grup).

## Stage 04 — Odoo sync read-only & order-to-cash · 2026-10-05 (done-with-fakes)
- `internal/odoo`: klien JSON-RPC (API key, `search_read` berhalaman) dan `Fake` (domain =, !=, >, <, in) dari `db/seed/odoo/*.json` yang diekspor `tools/seedgen` dengan id sama dengan seed.
- Syncer: partner → dealer & kontak (tier dari pricelist, termin, cabang dari company, sales dari user), produk + harga tier (`products`), SO + baris (kategori via `category_map`), picking/invoice/pembayaran → fase Order → Siap → Kirim → Invoice → Bayar, quant → stok; sinyal `so|invoice|payment|stock` dedupe `model:id:write_date`; kursor per model (`odoo_sync_state`).
- Job river `odoo.sync` (10 mnt) + recompute dealer tersentuh; `arc ctl odoo sync [--full]`, `arc ctl odoo test`; `CreateSODraft` dengan catatan sumber (ditolak bila `ODOO_WRITE=false`).
- API `/connections`, `/connections/odoo/test|sync|categories`; Pengaturan → Odoo (status sinkron, uji, sinkron penuh, pemetaan kategori ke 6 KAT).

## Stage 05 — LLM provider, agen v1, proposal & keputusan · 2026-10-05 (done-with-fakes)
- `internal/llm`: `Provider` (Anthropic beta Messages: structured output, effort, server-side fallback; OpenAI cadangan; `Fake`), `Router` (masking PII → unmask, validator, fallback template, log `llm_calls` provider/model/token/biaya Rp/hash/cycle), prompt di `internal/llm/prompts/`.
- `internal/agents`: AI Order (ekstraksi intent & item WA → `so_draft`/`credit_hold`, `price_counter` dengan floor margin, `return`), AI Follow-up (H-1, 2–7 hari, lewat jadwal dengan akar terduga), AI Kredit (`credit_release` SOP-SEC-001 + DP 50%, `credit_limit`). Semua angka dari `internal/metrics`; provenance wajib di domain.
- Migrasi `0005_proposals.sql` (ringkasan, tombol, opsi, pills, antrean, payload, `dedupe_key` unik untuk proposal terbuka).
- `internal/proposals`: runner (kedaluwarsa harian, dedupe, supresi kalibrasi, otonom hanya SO draft lengkap + credit hold) dan `Decide` (RBAC rilis CEO / limit CEO-finance, tolak wajib alasan → kalibrasi 14 hari, approve/edit → outbox WA dari nomor sales pemilik atau catatan/SO draft Odoo, audit + NOTIFY).
- API `/proposals` (+ `?queue=1&today=1`), `/proposals/{id}` (+ sinyal sumber), `POST /proposals/{id}/decide`, `/calibration` (confidence per agen 30 hari + kejadian terbaru), `/dealers/{id}/next`; `next` di daftar dealer, anotasi proposal di chat. `arc ctl agents run [--all|--agent|--dealer|--if-empty]`; `make dev` mengisi proposal awal.
- Web: antrian Keputusan (port `renderQueue`), ActionSheet (port `openSheet`: sumber, kenapa sekarang, disiapkan, setelah disetujui, edit draft, tolak dengan alasan), tombol aksi di Jadwal order/Lewat jadwal/Limit tipis/Dealer/Chat, Kalibrasi agen di Pengaturan. Perbaikan: `FeedbackProvider` di dalam router (sheet dapat bernavigasi).

## Stage 06 — Orchestrator 6 tahap, konflik, otonomi, rencana, layar Orchestrator · 2026-10-05 (done-with-fakes)
- `internal/orchestrator`: `Run(Scope, Trigger)` → Ingest, Analisis (errgroup ≤ 4), Sintesis & konflik, Keputusan, Eksekusi, Belajar; `cycle_stages` + NOTIFY per transisi; satu siklus aktif (indeks `cycles_one_active` → `409`, advisory lock per skema); siklus ber-scope memakai proposal hari ini sebagai jangkar aturan.
- Aturan murni teruji: `suppression`, `followup_gap`, `dedupe` (covers, satu tawaran per dealer, sama jenis), `credit_over_stock`, `collect_before_followup`, `margin_floor`, `one_owner`; `Evaluate` otonomi (matriks + `autonomy.guard`); `plan.Build` (≤ 10 langkah, jam, status).
- Agen v1 baru: AI Stok (bundle di atas floor margin), AI Penagihan (H-3 ramah, tegas, cicilan), AI Prospek (nomor baru → dealer tier C). Keputusan bundle → satu draft WA per dealer; dealer baru → catatan Odoo (Stage 12).
- Migrasi `0006_orchestrator.sql`; job `cycle.run` (tiap jam 06–20 WIB + dari `POST /cycles`); `arc ctl reanalyze --scope …`, `arc ctl cycle status`; `make dev` menjalankan siklus pertama.
- API: `/cycles/latest`, `/cycles`, `POST /cycles` (202/409), `/cycles/{id}`, `/conflicts`, `/agents`, `/agents/{name}/run`, `/plan/today`, `POST /plan/{id}/run` (*Jalankan sekarang*, keputusan manusia), `/policies/autonomy` (GET/PUT CEO), `/brief/today` dari siklus terakhir.
- Web: kartu Orchestrator & pill topbar hidup, Rencana hari ini, layar Orchestrator (pipeline, resolusi konflik, riwayat, agen, matriks; panel MCP menunggu Stage 07), Dock dengan scope layar, Analisis ulang di Orbit/Segmen/Dealer/⌘K, reducer `useCycle` (vitest), e2e Analisis ulang. ADR 0008.

## Stage 07 — MCP server, MCP sebagai orchestrator, Koneksi AI · 2026-10-05 (done-with-fakes)
- `internal/mcp`: Streamable HTTP di `/mcp` (SDK Go resmi) + `arc ctl mcp-stdio`; token `arc_…` (argon2id, tampil sekali), scope per tool, rate limit 60/mnt dan `max_cycles_per_hour`, `mcp_calls` + `audit_log` + SSE `mcp_call`.
- Tool baca (`dealer.*`, `segmen.list`, `jadwal.*`, `kredit.check`, `stok.aging`, `chat.thread` bermasking, `kpi.utama`, `cycles.recent`, `proposals.list`), analisis (`analisis.dealer|segmen|kas|stok`), orkestrasi (`orchestrator.run|reanalyze|agent.run|plan|plan.update|input.get|submit|status`); `actions.decide` → `human_only`. Resources & prompts.
- Orchestrator jalur `mcp`: migrasi `0007_mcp.sql` (`cycle_inputs`), Input per agen dimasking, submit tervalidasi (kind, provenance, dealer), Analisis menunggu ≤ 10 mnt lalu `partial`; `plan_change` dari MCP masuk Keputusan dan diterapkan setelah approve.
- API `/mcp/info`, `/mcp/clients` (buat/cabut token, CEO), `/mcp/calls`, `/policies/mcp` (`allow_send` terkunci), `/policies/llm`; CLI `arc ctl mcp-token`; `tools/mcpcheck`.
- Web: panel *MCP sebagai orchestrator* (saklar izin, log panggilan, tool), Pengaturan → Koneksi AI (jalur analisis, status, endpoint + salin, klien & token). `docs/mcp-clients.md`, ADR 0009.

## Stage 08 — Peta relasi 3D, pola relasi, PIC aktif · 2026-10-06
- Migrasi `0008_relasi.sql` (`interactions_monthly`, baseline kontak); seed riwayat interaksi 6 bulan dari mockup (`db/seed/interactions.json`); interaksi setelah `as_of` dihitung live dari WhatsApp + order.
- `views/relasi.go`: graf per periode & filter sales, pasangan terkuat, insight relasi (fungsi murni, diuji); API `/relasi`, `/relasi/insights`.
- PIC aktif dihitung ulang dari pesan kontak sebelum metrik; AI Follow-up meminta nomor admin/kasir bila hanya 1 PIC.
- Web: layar Peta relasi 3D (`three` 0.160, `NetView.ts` port `createNet`, layout di Web Worker ber-seed, fallback 2D, reduced-motion), vitest layout, e2e. ADR 0010.

## Stage 09 — Agen stok/penagihan/prospek, layar Push stok & Kredit · kas · 2026-10-06 (done-with-fakes)
- AI Stok: transfer antar cabang dan permintaan PO untuk stok kritis (keputusan → catatan Odoo, Stage 12); seed menulis sinyal `stock` seperti sinkron Odoo.
- AI Penagihan & Follow-up: `installment` untuk dealer yang minta tempo; usulan gabungan Mitra Jaya berjenis `installment`.
- AI Prospek & `internal/identify`: profil WA Business (worker, `wa.ProfileReader`), Truecaller (adapter, fake), impor CSV Getcontact, Odoo; skor; job `identify.number`; `price_list` dikirim ke thread nomor baru setelah approve. Migrasi `0009_identify.sql`.
- Prediksi kas masuk 30 hari dikalibrasi (tepat waktu, pola bayar, lewat tempo, minta tempo) → Rp 0,99 M pada seed.
- API `/chat/identify`, `/identifications/import`; `asked_tempo` di `/credit/forecast`; usulan per thread di konteks chat.
- Web: layar **Push stok** dan **Kredit · kas** dari API (port `renderAging`/`renderAR`), Chat → Nomor baru (Buat dealer tier C / Kirim harga), Pengaturan → Identifikasi nomor (impor CSV). ADR 0011.

## Stage 10 — Memori dealer, Ringkasan, kalibrasi, Tanya · 2026-10-06 (done-with-fakes)
- `internal/memory`: memo dealer sebagai kalimat bersumber (≤ 120 kata, validator menolak kalimat tanpa sumber), atribusi memo lama, penyegaran fakta saat ada sinyal baru, dirapikan LLM; ditulis di tahap Belajar. Seed menulis sinyal invoice terbuka seperti sinkron Odoo.
- Ringkasan Orchestrator ditulis siklus penuh: teks per poin dengan tautan dealer + `signal_ids`, LLM dengan validator tautan/angka; `/brief/today` membaca ringkasan siklus.
- Pelajaran kalibrasi: ≥ 3 penolakan alasan sama → pelajaran 14 hari yang menahan usulan serupa; `/calibration` memuat pelajaran.
- `internal/ask` + `POST /ask`: router pertanyaan, jawaban dari data dengan sumber.
- Web: Memori dealer (hover/klik → sumber per kalimat), Ringkasan dari siklus, Kalibrasi agen dengan pelajaran, Sheet jawaban ⌘K. Migrasi `0010_memory.sql`, ADR 0012.

## Stage 11 — Pengaturan kebijakan, auth & peran, Panduan · 2026-10-06
- `internal/policy`: JSON Schema per kebijakan + aturan terkunci (SOP-SEC-001, rilis CEO, MCP tanpa kirim, kredit tidak otonom); `Save` berversi dengan history, audit, NOTIFY; `PUT /policies/{key}`, `GET /policies/{key}/history`. Ambang yang berubah membuat siklus berikutnya menghitung ulang semua dealer.
- `internal/auth`: kata sandi argon2id, sesi JWT HttpOnly (stdlib), rate limit login; `/auth/login`, `/auth/logout`, `/users`; `arc ctl user add|passwd`; akun gudang di seed; `ARC_DEMO_PASSWORD` untuk dev.
- RBAC: menu per peran dari `/me`, keputusan per jenis + kepemilikan dealer, daftar dealer sales difilter server.
- Web: halaman login & keluar, menu/aksi per peran, Pengaturan dapat diedit (editor kebijakan + riwayat, Pengguna & peran, Matriks otonomi per sel), layar **Panduan** (isi mockup, tombol ke layar terkait). ADR 0013.

## Stage 12 — Eksekusi nyata: Odoo SO draft, outbox WA produksi, komitmen · 2026-10-06 (done-with-fakes)
- Outbox sebagai jalur eksekusi tunggal: `wa`, `odoo_so_draft`, `odoo_note`; status `pending/sent/failed/manual`, job `outbox.send` 3 percobaan, NOTIFY `outbox_failed` → toast; berhasil → proposal `executed` + audit.
- Odoo: `Source.PostNote` (`message_post` catatan internal); SO draft dengan catatan sumber; catatan keputusan di partner (kebijakan `odoo.write.notes`); nomor baru → partner baru; transfer/PO `manual`. SO draft dicatat sebagai order `ai_order_draft`, sinyal `so`, komitmen Kami "Kirim … Senin" (mengadopsi janji kirim dari chat).
- WhatsApp: jendela kirim proaktif 08–18 WIB (`WA_SEND_HOURS`), kirim pertama membuka thread, jejak "Terkirim HH.MM oleh … · dari proposal …" di Timeline.
- `internal/commitment`: reply tracking 72 jam (`chat_messages.proposal_id`, `reply_to`), janji bayar → komitmen Mereka bertanggal, jawaban ya → "Order sesuai rekomendasi"; komitmen lewat tanggal → `late` → AI Penagihan "Janji bayar terlewat" (selalu approve).
- Timeline dealer: keputusan (Disetujui/Ditolak/Otonom) dan "Balasan untuk: …". `arc ctl reanalyze` ikut mengantrekan eksekusi otonom. ADR 0014.

## Stage 13 — Hardening: keamanan, partisi & retensi, backup, observabilitas, deploy · 2026-10-06 (done-with-fakes)
- Keamanan: 2FA TOTP untuk CEO/admin (Pengaturan → Keamanan akun, kode di login, anti-replay, `arc ctl user totp-reset`), `arc ctl check-env`, CI menjalankan govulncheck + `npm audit` + uji check-env, header Caddy (HSTS, CSP, nosniff, frame DENY).
- Data: migrasi `0011_hardening` — partisi bulanan `signals` & `chat_messages` (data dipindah), tabel kunci dedupe `signal_keys`/`chat_message_keys`, job `partitions.ensure` & `retention.purge`, `arc ctl retention purge|partitions`, `arc ctl pdp export|delete`.
- Backup: `infra/backup.sh` (pg_dump terenkripsi age/OpenSSL, rotasi 30 hari), `infra/restore.sh`, `infra/restore-test.sh` + `make restore-test` + workflow mingguan `restore-test`; `arc ctl fingerprint`.
- Observabilitas: `/api/health` rinci untuk CEO/admin, `/metrics` Prometheus (histogram per rute), kartu **Status sistem**, alert WA terputus / siklus gagal 2× / antrean > 500 ke grup WhatsApp internal (outbox `wa_system`, ditolak bila bukan grup internal).
- Deploy: `infra/Dockerfile`, `infra/docker-compose.prod.yml`, `infra/Caddyfile`, `infra/systemd/`, `docs/DEPLOY.md`, `docs/RUNBOOK.md`; `arc ctl seed --policies-only` untuk produksi.
- Performa: `tools/loadtest` (+ `k6.js`, `make loadtest`), `docs/PERF.md`; timeline dealer 1,9 → 0,2 ms (join uuid + indeks parsial). ADR 0015.

## Stage 14 — Pilot cabang Semarang: perangkat pilot · 2026-10-06 (in-progress — menunggu pilot nyata)
- Kebijakan `pilot` (off / bayangan / live) + migrasi `0012_pilot`. Mode bayangan: tidak ada langkah otomatis, keputusan tetap mengkalibrasi, outbox `shadow` tidak pernah dikirim, balasan dari layar Chat ditolak, `ApproveBySystem` menolak.
- `internal/pilot`: % saran diterima & median waktu keputusan per agen, confidence mingguan, order tepat jadwal, DSO, lewat jadwal tertangkap sebelum churn, audit privasi & kirim, aturan buka otonomi ≥ 80% dua minggu (dicek server), snapshot mingguan (`pilot.snapshot`) & CSV.
- API `/pilot`, `/pilot/export.csv`, `/pilot/mode`, `/pilot/unlock`, `/sow/top`, `/sow/confirm`; `arc ctl pilot start|live|off|status|audit|snapshot|export`, `arc ctl wa import-internal`.
- Web: Pengaturan → **Pilot** (mode, KPI, per agen, audit, laporan mingguan), kartu Pilot di Pengaturan, chip **Mode bayangan** di topbar, Dealer → **Konfirmasi share of wallet** (20 dealer teratas, per kuartal).
- `docs/PILOT-REPORT.md` (checklist hari 0, aturan pilot, tabel hasil, temuan, go/no-go untuk ditandatangani Sam). ADR 0016.
- Perbaikan: `pdp delete` kini juga menghapus sinyal percakapan berdasarkan `payload.from_number`.
- Gladi pilot (`arc ctl pilot rehearse`, dev): 21 hari simulasi (14 bayangan + 7 live) — `docs/PILOT-REHEARSAL.md`. Perbaikan dari gladi: audit tidak lagi menghitung catatan Odoo atas saran yang ditolak sebagai kirim tanpa persetujuan; transfer/PO/bundle yang sudah disetujui tidak diusulkan ulang selama 7 hari (AI Stok 214 → 82 saran dalam 21 hari).

## Deploy staging · 2026-10-06
- `https://crm-distri.gsiindo.id` di VPS GSI (nginx + certbot bersama aplikasi lain): user `arc`, database `distri_arc`, API `127.0.0.1:8110`, systemd api/worker/backup, `infra/deploy.sh` (git pull → build → migrasi → restart → cek health), `infra/nginx/distri-arc.conf`. Data contoh, adapter fake, akun dengan kata sandi acak.

## Tampilan · 2026-10-06
- Tema **Terang** sebagai default (tidak lagi mengikuti mode gelap OS), dengan pilihan Terang / Gelap / Otomatis.
- Warna sidebar mejikuhibiniu: Netral, Merah, Jingga, Kuning, Hijau, Biru, Nila, Ungu — dari menu akun (semua peran) dan Pengaturan → Tampilan; disimpan per perangkat. Badge tetap terbaca di semua warna.
- Sidebar Biru memakai gradasi biru tua → nila (seperti aplikasi GSI lain) dengan penanda putih di menu aktif; sidebar bisa **diringkas** (ikon saja, tooltip nama menu, diingat per perangkat); kartu profil berisi avatar, nama, email, dan tombol keluar langsung.

## WhatsApp Baileys, banyak nomor · 2026-10-06
- Transport `baileys` (ADR 0017): bridge Node `apps/wa-bridge` (Baileys 7, sesi di Postgres, satu sesi per nomor) + transport di worker (event masuk HMAC, pemeriksaan persetujuan outbox sebelum setiap kirim). Layanan systemd `distri-arc-wa-bridge`, dibangun oleh `infra/deploy.sh`; target Docker `wa-bridge`.
- Penjaga anti-blokir: hanya membalas kontak yang pernah menghubungi, opt-out, anti-broadcast, jam tenang, 20/jam & 120/hari per nomor, pemanasan nomor baru, jeda acak, "mengetik…". Penolakan jeda menunda, penolakan final menggagalkan dengan alasan.
- Banyak nomor: migrasi `0013_wa_accounts` (percakapan milik nomor), tambah / pasangkan / lepas nomor di Pengaturan → WhatsApp dengan penghitung anti-blokir, bar nomor di Chat dengan QR langsung, "via nomor" di tiap percakapan.
- Privasi: sales hanya melihat & membuka percakapan nomornya sendiri; `/wa/pair` dibatasi CEO/admin/pemilik nomor.

## Data asli & master data · 2026-10-06
- Impor data asli (ADR 0018): kontrak kolom baku untuk tim sales, pelanggan, faktur, item faktur, stok; sumber **BigQuery** (SQL per entitas, kunci service account terenkripsi, sinkron berkala, tes query) atau **CSV** (unggah + template). Staging → transformasi idempoten → metrik dihitung ulang.
- Master data: mapping cabang, gudang → cabang, kategori → 6 kategori product mix, sales, jenis pelanggan (nilai baru tercatat sebagai "belum dipetakan"); master pelanggan (jenis, tier, limit, termin, sales) yang tidak tertimpa impor; tim sales.
- Dua jenis pelanggan: **Dealer (reseller)** dan **Freelance / System Integrator** — filter di Dealer, badge di halaman dealer, `?type=` di API board.
- Pengaturan → **Data & master**; `arc ctl wipe --confirm <db>`; setiap akun baru mendapat profil keputusan otomatis. Panduan: `docs/DATA-IMPORT.md`.

## Pemetaan BigQuery (Accurate) · 2026-10-07
- `GET /api/data/schema`: daftar dataset, tabel, kolom, dan jumlah baris project BigQuery (REST baca saja), plus saran per jenis data: tabel paling cocok (dan kandidat lain), pemetaan kolom → kontrak dari nama kolom Accurate / Indonesia (`nomor_faktur`, `sisa_tagihan`, `gudang`, …), dan SQL siap pakai.
- Pengaturan → Data & master → **Pemetaan BigQuery**: pilih tabel, cocokkan kolom (boleh ekspresi SQL), filter WHERE, pratinjau SQL, simpan semua query (CEO), tes query, jelajahi semua tabel.
- Mapping master: saran otomatis untuk kategori → 6 kategori product mix dan jenis pelanggan → reseller / SI ("saran: …", tombol **Isi saran**); tetap dikonfirmasi manusia sebelum disimpan.

## Chat WhatsApp (Baileys) · 2026-10-07
- Halaman Chat tanpa percakapan kini menampilkan **Hubungkan WhatsApp**: tambah nomor (CEO/admin) lalu tautkan, daftar nomor dan statusnya, ringkasan aturan anti-blokir; sales tanpa nomor diberi petunjuk. Chip **+ Nomor** di bar nomor.
- Menautkan dengan **kode di HP** (8 karakter, "Tautkan dengan nomor telepon saja") selain QR — `POST /wa/pair {method:"code"}`, bridge `POST /sessions {phone_code:true}`.
- Nama kontak dari HP (nama tersimpan / bisnis / push name) jadi judul percakapan nomor baru; riwayat setelah menautkan masuk sebagai sudah dibaca dan tidak memicu identifikasi profil.
- Anti-blokir: chat dibaca (centang biru) hanya tepat sebelum membalas, dengan jeda membaca 1–3 dtk, lalu "mengetik…".
- Chat **banyak nomor**: rel nomor WhatsApp di kiri (seperti akun di WhatsApp Business) — "Semua" + tiap nomor dengan avatar, status tertaut, dan jumlah belum dibaca; klik untuk melihat chat nomor itu saja, nomor yang belum tertaut langsung dibuka untuk ditautkan, **+ Nomor** untuk menambah. Kepala daftar menunjukkan nomor terpilih / jumlah nomor terhubung. Di HP rel menjadi baris yang bisa digeser.

## Master peran & akses · satu pengguna satu nomor WhatsApp · 2026-10-07
- **Peran & akses** (Pengaturan, ADR 0019): tambah/ubah peran — nama, jenis akses dasar (Admin / Finance / Sales / Gudang), menu yang dibuka, jenis saran AI yang boleh diputuskan, boleh memegang nomor WhatsApp. Peran hanya mempersempit akses dasarnya; menu ditegakkan juga di API (403), keputusan di luar peran ditolak. Hanya CEO yang mengubah; admin melihat. Migrasi `0015_roles_access`.
- **Pengguna**: peran dipilih dari master peran, cabang, dan **satu nomor WhatsApp** per pengguna (unik); daftar menunjukkan nomor dan status WA-nya. Harus selalu ada satu CEO aktif.
- **Tambah nomor di Chat dari master pengguna**: pilih pengguna → nomornya terisi dari master → Tambah & tautkan; satu pengguna satu nomor; pengguna non-admin bisa menautkan nomornya sendiri. Form yang sama di Pengaturan → WhatsApp.
- Menu **Pengguna** di sidebar (CEO/admin, menu `users` di master peran): halaman sendiri berisi master pengguna dan Peran & akses; Pengaturan menautkan ke sana. Di layar laptop pendek kartu agen di sidebar disembunyikan agar kartu profil tetap terlihat.
- Pop-up (tambah pengguna, tautkan nomor, keputusan, dll.) muncul **di tengah layar** di laptop/desktop; di HP tetap dari bawah.
- Siklus "terakhir" diurutkan menurut nomor siklus, bukan jam mulai — jam yang mundur (jam simulasi dev, koreksi NTP) tidak lagi menyembunyikan siklus baru.

## Peran custom · master cabang · WhatsApp scan lalu pilih pengguna · 2026-10-07
- **Peran & akses tanpa hardcode** (ADR 0020, migrasi `0016`): tiap peran dicentang akses per halaman (14 halaman, dikunci juga di server), cakupan data (semua / hanya miliknya), jenis keputusan, hak kebijakan (setara CEO), dan izin memegang WhatsApp. Peran bawaan bisa diubah/dihapus; selalu ada satu pemegang hak kebijakan aktif; rilis kredit di atas limit tetap butuh hak kebijakan.
- **Master cabang** (menu Master data → Cabang): tambah/ubah/nonaktifkan; ganti nama ikut ke pengguna, dealer, dan mapping data; cabang pengguna dan mapping cabang/gudang dipilih dari master.
- **Tambah pengguna** tanpa nomor WhatsApp; cabang dari master; **lihat kata sandi** (ikon mata) di form pengguna dan halaman login.
- **Chat → + Nomor**: scan QR (atau kode di HP) → nomor terbaca otomatis → pilih nama pengguna pemegangnya; nomor yang belum dipegang bisa dipilih penggunanya kemudian. `POST /wa/links`, `GET /wa/links/{id}`, `PUT /wa/numbers/{wa}/user`; bridge menyertakan nomor HP di setiap event.
- Sidebar: grup **Master data** (Pengguna, Peran & akses, Cabang).

## Push stok dari BigQuery · 2026-10-07
- Halaman Push stok sepenuhnya dari data impor (BigQuery/CSV) sesuai rumus glossary: **margin** order dihitung dari HPP (`invoice_lines.cost`, else HPP stok) sehingga **Perputaran stok** (nilai stok ÷ HPP harian) dan **Penjualan per produk** (nilai · margin) benar; **umur stok** dari `received_date` bila `age_days` kosong; **kecepatan jual** (stok kritis) dari item faktur 90 hari per SKU × cabang bila `sold_90d` kosong.
- Kontrak: `invoice_lines.cost`, `stock.received_date`; saran kolom BigQuery mengenali `hpp`/`harga_pokok`, `tanggal_masuk`/`last_purchase_date`. Tabel "bagian halaman → kolom" dan contoh SQL Accurate di `docs/DATA-IMPORT.md`.
- Pengaturan: kartu **Data asli · BigQuery (Accurate)** di atas Sumber sinyal — status kunci, **Unggah kunci JSON** langsung (Project ID terisi dari kunci, sumber otomatis jadi BigQuery), lalu "Lanjut: petakan tabel Accurate" ke Data & master. Kartu yang sama di atas halaman Data & master (tombol unggah tidak lagi tersembunyi di mode "Belum").

## Data Accurate tersambung · 2026-10-07
- BigQuery `accurate_data` terpasang (sinkron 60 menit): 37 sales, 2.841 pelanggan B2B (reseller/SI), 24.084 faktur & 52.798 item 18 bulan, 1.455 stok barang × cabang; master Cabang (Semarang/Kantor Pusat, Yogyakarta, Jakarta, Surabaya, Mataram, Soetta); 131 kategori → 6 kategori. Detail di `docs/DATA-IMPORT.md`.
- Sinkron besar: batas waktu job 30 menit; staging item faktur tanpa hapus per faktur (sebelumnya memindai tabel per faktur); baris yang tidak berubah tidak ditulis ulang (sinkron berikutnya hanya memproses perubahan); angka ilmiah BigQuery (`4.6355E7`) terbaca benar.
- Push stok sesuai glossary: kandidat tidak lagi memuat dealer **Churn** (berhenti order — bukan "lewat jadwal"); margin **per produk** dari HPP baris; daftar stok menua ringkas 12 teratas + "Tampilkan semua" + filter cabang; daftar Push di Pusat kendali hanya stok menua yang punya kandidat.
- AI Stok dengan data Accurate: harga bundle memakai harga jual terakhir bila belum ada harga tier; barang tanpa harga dilewati; maksimal 20 usulan push per siklus (nilai stok menua terbesar dulu); usulan dengan payload yang tidak bisa disimpan kini menjadi error (sebelumnya tersimpan kosong).

## Efisiensi untuk data asli (ribuan dealer) · 2026-10-07
- Server: papan dealer di-cache 60 detik (langkah berikutnya tetap segar tiap request; cache dibuang setelah perubahan lewat API); JSON dikompresi gzip; stok menua membawa 3 dealer kandidat teratas + jumlahnya (bukan ratusan); daftar Push di Pusat kendali 10 teratas; bundle AI Stok maksimal 30 dealer teratas ("30 teratas dari N yang cocok").
- Orbit: mode **Prioritas** (150 dealer omzet terbesar, 30 terbesar bernama) dan **Semua titik** (semua dealer sebagai titik kecil tanpa label); penataan label hanya untuk dealer bernama. Segmen: semua titik tetap, nama hanya 50 omzet terbesar. Daftar Dealer, daftar segmen, dan daftar Pusat kendali (jadwal order, lewat jadwal, limit tipis) dimuat bertahap ("Tampilkan N lagi").
- Filter sales menjadi dropdown bila sales lebih dari 8 (Orbit, Segmen, Peta relasi).

## MCP Claude · 2026-10-07
- **Claude menganalisis semua data** lewat MCP (ADR 0021): OAuth 2.1 untuk custom connector claude.ai / Claude Desktop / Claude Code — metadata (RFC 9728/8414), pendaftaran klien dinamis, PKCE S256 wajib, halaman persetujuan **Izinkan Claude** (`/claude/izin`, hanya pengguna dengan menu MCP Claude dan cakupan semua data; orchestrate hanya hak kebijakan), token akses 1 jam + refresh berputar 90 hari, bisa dicabut. Migrasi `0017_mcp_oauth`.
- Server MCP: nama tool ramah Claude (`dealer_list`, …), daftar dihalaman (`limit`/`offset`/`total`/`next_offset`), tool baru `data_ringkasan`, `penjualan_bulanan`, `produk_terlaris`, `piutang_ringkas`; perbaikan 403 di belakang nginx (pengaman DNS-rebinding SDK).
- Halaman **MCP Claude** (sidebar, menu `mcp` di Peran & akses): status, cara menghubungkan (claude.ai/Desktop, Claude Code, token manual), koneksi + cabut, izin & batas, daftar tool, contoh prompt analisis, log panggilan. nginx/Caddy meneruskan `/.well-known/oauth-*` dan `/oauth/`.

## Analisis terjadwal (MCP Claude) · 2026-10-07
- **Claude menganalisis sesuai jadwal cron** (ADR 0022): kartu *Analisis terjadwal* di halaman MCP Claude — jadwal (preset atau cron bebas, pratinjau "Senin–Sabtu pukul 07.00 · berikutnya …", jarak minimal 15 menit), prompt, izin tool, batas langkah, aktif/jeda, *Jalankan* sekarang, riwayat laporan (Markdown + daftar panggilan tool).
- Mesin: Claude (Messages API) memanggil tool MCP yang sama lewat sesi in-process; tiap panggilan tercatat di log MCP atas nama "Terjadwal · <nama>". Kunci Claude API diisi CEO (diperiksa, disimpan terenkripsi), pilihan model Opus 5.5/Sonnet 5.5/Haiku 4.5, anggaran harian. Tanpa kunci / lewat anggaran → laporan template berisi angka dari tool MCP.
- Worker: `analyst.tick` tiap menit + `analyst.run` (sekali per slot, tanpa retry). Migrasi `0018_mcp_schedules.sql` (jadwal awal *Ringkasan pagi* Senin–Sabtu 07.00).

## Halaman login baru · 2026-10-08
- Login satu layar tanpa scroll di atas **galaksi orbit animasi** (canvas: bintang berkelip, nebula, 7 orbit elips dengan planet berekor, inti berdenyut; diam bila "kurangi gerakan"): judul, 9 fitur sistem ringkas, dan kartu Masuk. Keterangan fitur menyusut di layar pendek; di HP hanya judul + kartu.
- Tombol mata **lihat kata sandi** lebih jelas di dalam kolom (menyala saat kata sandi terlihat).


## Nama & logo: GSI Orbit · 2026-10-08
- Nama sistem menjadi **GSI Orbit** ("CRM Distribusi · AI") di aplikasi, login, tab browser, MCP Claude, perangkat WhatsApp tertaut, 2FA, catatan Odoo, dan prompt AI.
- Logo baru (inti dengan titik yang mengorbit, biru → ungu) di sidebar, login, dan favicon.

## Pusat kendali lebih mudah dibaca · 2026-10-08
- Daftar dealer panjang di Rencana hari ini dan Ringkasan Orchestrator diringkas (5 nama + "+N dealer lainnya", bisa dibuka); poin ringkasan yang panjang dibatasi 4 baris + "Selengkapnya".
- Agenda sales: sales tersibuk dulu, 6 sales lalu "Tampilkan lagi", 3 dealer per sales, sales tanpa agenda mendesak dirangkum satu baris; baris dealer rata kiri dengan keterangan di kanan.
- Judul kartu sempit (Jadwal order, Lewat jadwal, Push stok, Limit tipis) tidak lagi terpecah; keterangan rekomendasi maksimal 2 baris. Tinggi halaman dengan data asli turun dari ±8.800 px ke ±3.700 px. Tanpa perubahan fungsi.

## Klasifikasi ulang Orbit & Segmen · 2026-10-08
- ADR 0023: faktur di hari yang sama = 1 order; siklus order minimal 7 hari; seringnya & omzet/bln dari pembelian aktual 6 bulan (bukan proyeksi); status **Prospek** untuk pelanggan yang belum pernah order (tidak digambar di Orbit/Segmen, dihitung terpisah, filter status di halaman Dealer); order sekali > 90 hari atau diam > 1 tahun = Churn; Key account bisa diberi syarat omzet minimum.

## Peta relasi 3D untuk data asli · 2026-10-08
- Peta menggambar sales dan dealer yang punya interaksi (WhatsApp + order) dalam 6 bulan, maksimal 250 dealer teraktif (sebelumnya semua 2.841 pelanggan sehingga layout tidak pernah selesai dan peta kosong); node sama untuk semua periode; HUD "250 dealer teraktif dari N".
- Order dihitung per hari order (bukan per faktur); filter sales menjadi dropdown untuk tim besar.

## Orbit mudah dibaca untuk data asli · 2026-10-08
- Titik digeser hanya **sepanjang lingkarnya sendiri** (tidak lagi menumpuk jadi kolom yang memotong lingkar); ukuran seragam kecil selama share of wallet masih estimasi; 12 nama terpenting dengan garis penunjuk; klik status di "Isi orbit" untuk menyorot satu lingkar. Arti glossary tetap (lingkar = status, sudut = siklus, ukuran = SOW, warna = sisa limit); data contoh 18 dealer memakai tata letak mockup.

## Segmen mudah dibaca · 2026-10-08
- Tampilan bawaan **Kotak**: 4 kotak besar (A–D) dengan nama sehari-hari (Pelanggan andalan, Rutin tapi kecil, Pelanggan proyek, Kecil & jarang), label "Sering/Jarang order" & "Order besar/kecil", jumlah dealer, omzet/bulan, bar % omzet, satu kalimat "Yang dilakukan", dan 3 contoh dealer. Dealer baru di baris terpisah. Klik kotak → daftar dealer di panel kanan (di HP otomatis bergulir ke daftar).
- Grafik titik tetap ada di tab **Peta titik**: huruf lebih besar, nama zona + nama sehari-hari, 20 nama dealer (dari 50), judul sumbu dengan kata biasa. Teks "Pindah kotak" ditulis sebagai kalimat (naik/turun ke Segmen X, Saran: …). Nama glossary (Segmen A–D) tidak berubah.

## Orbit: ringkasan, filter, daftar dealer · 2026-10-08
- **Ringkasan** di atas orbit: 4 kotak — Jadwal order minggu ini, Lewat jadwal, Tagih dulu (over limit / overdue, dengan total piutang), Churn — masing-masing dengan jumlah dealer dan omzet; klik kotak = filter.
- **Filter**: cari (dealer/kota/sales), status, jadwal order (≤ 7 hari / lewat jadwal), sisa limit, segmen, cabang; "Hapus filter (n)"; filter tersimpan di URL (`/orbit?jadwal=lewat`) sehingga bisa dibagikan. Klik status di "Isi orbit" kini menyaring.
- **Daftar dealer** di bawah orbit (mengikuti filter): status, jadwal order dalam kata ("lewat 9 hari", "besok"), omzet/bln, sisa limit, tombol tindakan; urutkan omzet terbesar / paling mendesak / paling lama tidak order. Definisi di `web/src/features/orbit/filters.ts` (diuji), sama dengan Pusat kendali.

## Orbit & Segmen: zoom, geser, layar penuh · 2026-10-09
- **Zoom sungguhan** di peta Orbit dan Peta titik Segmen: scroll mouse / touchpad (geser dua jari atau cubit — termasuk Safari di Mac) / cubit dua jari di HP / tombol − + / klik ganda (Shift+klik ganda = zoom out), 100%–800%, berpusat di kursor. **Seret** untuk menggeser. **Layar penuh** memakai seluruh layar (bentuk layar diikuti); di iPhone yang tidak punya Fullscreen API dipakai mode layar penuh CSS (tutup dengan tombol atau Esc).
- **Titik bertumpuk**: ukuran titik tetap sama di layar saat zoom, jadi zoom in memisahkan buletin yang bertumpuk; penyebaran anti-tumpuk dihitung ulang sesuai ukuran titik di layar (makin di-zoom, makin dekat ke posisi asli). Kontrol **Titik** (kecil/besar) terpisah dari zoom; titik terkecil = setiap titik tepat di posisi aslinya.
- Teks peta (nama lingkar, sumbu, zona) dan lingkaran GSI tetap berukuran normal saat zoom; garis tidak menebal. Klik titik tetap membuka dealer; seret tidak dianggap klik.
- `web/src/components/MapViewport.tsx` (+ `MapViewport.test.ts`: matematika zoom, letterbox, posisi asli pada titik terkecil, penyebaran mengecil saat zoom).
- **Segmen · Peta titik**: hanya area titik yang di-zoom/digeser; garis sumbu dengan panahnya dan judul sumbu tetap di tempat (juga di layar penuh), angka sumbu menyesuaikan bagian yang terlihat (mis. 2,3×–3× · 40–80 jt saat di-zoom). Titik dasar lebih kecil (ukuran tetap = share of wallet) supaya ratusan dealer tidak langsung bertumpuk.
- **Segmen · tidak ada lagi tumpukan di tepi**: dealer > 3,5×/bln dulu semua ditaruh di satu garis di tepi kanan (dan > 200 jt / < 4 jt di tepi atas/bawah). Sekarang rentang sumbu mengikuti data: 0–3,5× tetap linear di 80% lebar, di atas 3,5× dipadatkan (log) di 20% sisanya dengan angka 4×, 5×, 8×, 12×…; sumbu Rp melebar sesuai order terbesar/terkecil. Setiap dealer mendapat posisi aslinya (`scale.ts` `Domain`, diuji).

## Peta relasi 3D: filter, zoom, layar penuh · 2026-10-09
- **Filter seperti Orbit/Segmen** di atas peta: cari dealer/kota/sales, status, jadwal order, sisa limit, segmen, cabang, "Hapus filter (n)" dan "n dari N dealer". Dealer yang tidak cocok memudar beserta garisnya; nomor sales tetap menyala selama terhubung ke dealer yang cocok. Bisa digabung dengan filter nomor sales.
- **Zoom**: scroll mouse, touchpad (geser dua jari / cubit, termasuk Safari di Mac), cubit dua jari di HP, tombol − + dengan persen (50%–1000%; 100% = seluruh jaringan terlihat), ↺ atur ulang. **Geser**: Shift+seret, klik kanan+seret, atau dua jari. Seret biasa tetap memutar.
- **Layar penuh** untuk panggung 3D (cadangan CSS untuk iPhone, Esc untuk keluar); peta dirender ulang sesuai ukuran layar (dulu render berhenti karena `offsetParent` bernilai null pada elemen layar penuh).
- `useFullscreen` + `FullscreenButton` di `MapViewport.tsx` dipakai bersama Orbit, Segmen, dan Peta relasi.

## Peta relasi: tampilan Galaksi · 2026-10-09
- Pilihan **Jaringan | Galaksi** di peta relasi (diingat per browser). Di **Galaksi**: setiap nomor sales = **planet** (ukuran = interaksi), setiap dealer = **satelit** yang mengorbit sales terdekatnya (interaksi terbanyak); **orbit makin dekat = makin sering berinteraksi**, satelit dekat bergerak lebih cepat; warna = skor dealer. Dealer yang juga dekat dengan sales lain punya garis putus ke planet itu. Planet tersebar di lengan spiral di sekitar inti "GSI", sistemnya tidak pernah bertumpuk; dealer tanpa interaksi beredar di sabuk luar. Latar luar angkasa dengan bintang.
- Klik planet → kamera terbang ke sistemnya, satelitnya diberi nama (tanpa label bertabrakan), sistem lain meredup. Klik dealer dua kali → buka dealer. Filter, zoom (scroll/touchpad/cubit), geser, putar/miringkan, dan layar penuh berlaku di kedua tampilan.
- `relasi/galaxy.ts` (tata letak murni, diuji: orbit sesuai sales terdekat, orbit dalam = lebih cepat, sistem tidak bertumpuk, inti tetap kosong, deterministik), `GalaxyView.ts` (canvas, berjalan tanpa WebGL), kontrol & tooltip dipakai bersama (`controls.ts`, `tip.ts`).
- **Galaksi v2** (masukan Sam): planet tidak lagi sebaris — tersebar acak (tetap sama untuk data yang sama) di ruang 3D sekitar inti, tiap sistem dengan kemiringan orbit sendiri. **Zoom menuju kursor** (scroll/touchpad/cubit) sehingga terasa terbang menembus galaksi, sampai ±4000%; debu angkasa melintas saat dekat. **Prospek (lead yang belum pernah order)** ikut tampil: **komet** = prospek milik sales (orbit lonjong mengelilingi planet sales-nya, ekor menjauhi planet), **meteor** = prospek tanpa sales (melayang lurus menembus galaksi). Maks. 400 prospek, klik dua kali untuk membuka. Label planet tidak lagi bertumpuk.
- **Galaksi v3** (masukan Sam): komet tidak lagi mengorbit beramai-ramai — **sesekali satu komet melesat dari atas ke bawah layar** (tiap 3–7 detik) membawa satu prospek secara bergiliran, namanya tampil sekejap, klik untuk membuka. Meteor (prospek tanpa sales) dibatasi 30. **Latar bintang**: ±14.000 bintang berbagai ukuran & warna (sebagian berkelip, yang terang berkilau) dan pita Bima Sakti samar.

## Dashboard · 2026-10-09
- Card **Sorotan Orchestrator** dihapus dari Dashboard (permintaan Sam). Keputusan dan ringkasan Orchestrator tetap ada di Pusat kendali; header Dashboard masih menampilkan waktu ringkasan AI terakhir.
- **Jadwal order 7 → 14 hari** (permintaan Sam) di Orbit: kotak ringkasan "Jadwal order 2 minggu ke depan", filter "Jadwal order ≤ 14 hari (2 minggu)", warna hijau di kolom jadwal daftar dealer; metrik Dashboard ikut ("Jadwal order · 14 hari", satu definisi `DUE_DAYS` di `orbit/filters.ts`, diuji batas 14/15 hari). Pusat kendali dan agen AI di server tetap 7 hari.

## Push stok: tabel data · 2026-10-09
- Card **Push stok** menjadi tabel data selebar halaman: kolom Produk (SKU · kategori), Cabang, Qty, Nilai, Umur stok, Dealer yang cocok, aksi. **Cari** (produk, SKU, kategori, cabang, nama dealer yang cocok), **urutkan** naik/turun di setiap kolom (klik judul kolom; bawaan umur stok terlama), **paginasi** 10/25/50 baris. Chip cabang tetap ada. Stok kritis dan Penjualan per produk pindah ke bawah tabel. Logika cari/urut di `stock/table.ts` (diuji).

## Kredit · kas: Exposure vs limit sebagai tabel · 2026-10-09
- Card **Exposure vs limit** menjadi tabel data selebar halaman: Dealer (sales · cabang), Exposure, Limit, Sisa limit (merah bila minus), Pemakaian limit (bar + %). **Cari** (dealer, sales, cabang), **urutkan** naik/turun di setiap kolom (bawaan pemakaian tertinggi), **paginasi** 10/25/50; jumlah dealer over limit di kanan atas.
- Komponen bersama `components/DataTable.tsx` (`useDataTable`, `SortTh`, `TableSearch`, `TablePager`) dipakai tabel Push stok dan Exposure; perbandingan kolom diuji (`stock/table.ts`, `credit/exposure.ts`).

## Orchestrator: Resolusi konflik & Riwayat analisis sebagai tabel · 2026-10-09
- **Resolusi konflik**: tabel Agen (A ↔ B), Dealer, Konflik, Resolusi Orchestrator — cari, urutkan naik/turun tiap kolom, paginasi 10/25/50 (tetap konflik siklus penuh terakhir).
- **Riwayat analisis**: tabel Waktu (tanggal · jam), No., Jalur, Sinyal, Otonom, Keputusan, Konflik, Catatan — cari, urutkan naik/turun tiap kolom (bawaan terbaru), paginasi; kini memuat 100 siklus terakhir (sebelumnya 7 dari 20). Logika di `orchestrator/tables.ts` (diuji), memakai `DataTable` bersama.

## Dashboard: Kota Distribusi (mode game) · 2026-10-09
- Adegan animasi di atas Dashboard, digerakkan data asli: **Gudang GSI** (peti = stok menua, "!" = ada stok kritis), **Kantor sales** (jumlah chat belum dibaca), dan **toko dealer** di dua jalan (★ Key account, "!" At risk, papan silang = Churn, warna atap = sisa limit, label "order N hr lagi"). **Truk** mengantar paket ke dealer dengan jadwal order ≤ 14 hari, **kurir motor** menagih dealer over limit/overdue dan pulang membawa koin, **sales berjalan** ke dealer At risk untuk follow-up. Siang/malam mengikuti jam WIB (lampu jalan & jendela menyala malam hari).
- HUD ala game: omzet/bln, jumlah antaran, tagihan, follow-up, dan **Level** (dari % order tepat jadwal vs target). Klik toko → dealer, gudang → Push stok, kantor → Chat. Bisa disembunyikan (diingat per browser); metrik dashboard lama tetap di bawah. Gerak dimatikan bila pengguna memilih "reduce motion".
- `dashboard/town.ts` (pemilihan toko & tugas, diuji), `TownView.ts` (canvas, tanpa library baru), `DashboardTown.tsx`.

## Analisis ulang: pop-up hasil & update terakhir · 2026-10-09
- Hasil **Analisis ulang** kini tampil sebagai pop-up gaya SweetAlert (ikon beranimasi): **Analisis berhasil** (siklus, saran diperbarui, jam selesai, catatan), **Analisis selesai sebagian**, **Analisis gagal** (dengan penyebab), juga saat gagal dimulai atau Orchestrator masih berjalan. Berlaku untuk semua tombol Analisis ulang; tutup dengan OK atau Esc. Komponen tanpa library baru (`feedback.tsx` → `alert()`).
- **Pusat kendali**: baris "Update terakhir analisis: 5 Okt · 06.45 WIB (12 menit lalu) · siklus #12 · berhasil · berikutnya 07.00" di kartu Orchestrator, diperbarui tiap menit; saat berjalan menampilkan tahap yang sedang dikerjakan.

## Chat: perbaikan tautan nomor kedua + loading saat scan QR · 2026-10-09
- **Bug: nomor kedua tidak terdeteksi / datanya tidak diambil.** Penyebab: sheet tidak pernah di-unmount, sehingga "+ Nomor" yang dibuka lagi masih memegang sesi tautan nomor pertama (sudah "Terhubung") dan tidak meminta QR baru; "Tautkan ulang" nomor lain juga tidak mengirim permintaan pairing. Kini setiap pembukaan sheet adalah komponen baru (`feedback.tsx`, diuji). Pengaman tambahan: nomor akun pesan tidak lagi diambil dari digit acak di id sesi `link-…` (`baileys.go`, diuji).
- **Loading saat scan QR**: wa-bridge memberi tanda saat QR dipindai/kode diketik (`isNewLogin` → awalan `linking:`), UI menampilkan spinner "QR berhasil dipindai — menautkan perangkat dan mengambil data chat…" sampai nomor terhubung; juga spinner saat menyiapkan QR/kode. Tanpa migrasi (pola yang sama dengan awalan `code:`).

## Chat: nama & detail per nomor WhatsApp · 2026-10-09
- Nomor di Chat kini tampil dengan **nama pemegangnya** (sebelumnya nomor yang ditautkan lewat "+ Nomor" lalu diberikan ke sales tetap tampil sebagai "+62 899-••••-…", karena nama pengguna tidak dipakai). Rail menampilkan nama depan (nomor tanpa pemegang: "…0002" dan avatar "?"), header daftar chat: nama · peran · cabang · nomor.
- **Detail nomor** (ikon di samping nama, atau tombol Detail di daftar nomor): pemegang & peran, cabang, email, nomor lengkap (hanya untuk admin dan pemegangnya), status, tertaut sejak, terakhir aktif, chat terakhir, jumlah percakapan & belum dibaca, kirim hari ini vs batas & masa pemanasan, jenis perangkat; aksi Ganti pemegang / Tautkan ulang.
- Perbaikan: hitungan kirim per nomor (hari ini x/y, pemanasan) tidak pernah muncul untuk nomor yang ditautkan dari Chat — bridge menghitung per id sesi (`link-…`), server mencari per nomor. `ListWANumbers` kini membawa peran, cabang, dan statistik chat (sqlc diregenerasi).
- **Pemegang nomor tidak tersimpan diam-diam**: memilih sales yang sudah memegang nomor lain ditolak server (satu pengguna satu nomor) dan hanya tampil sebagai toast sekejap, sehingga nomor baru tetap tanpa nama — sales itu bahkan tidak muncul di pilihan. Kini semua pengguna yang boleh memegang WhatsApp tampil (ditandai "memegang …1234"), memilih yang sudah memegang nomor memunculkan peringatan dan tombol **Pindahkan ke nomor ini** (nomor lama tetap tersambung tanpa pemegang; `move: true` di API), dan setiap kegagalan simpan tampil permanen di form + pop-up.
- **Nomor bisa dipegang sales tanpa akun.** Sales lapangan dari master Sales (mis. impor Odoo) yang belum punya login tidak pernah muncul di pilihan pemegang, sehingga nomor yang sudah discan tetap tanpa nama. Kini form pemegang punya grup **Sales (tanpa akun)**; memilihnya mengisi nama nomor, menautkan percakapan ke sales itu, dan menjadikannya nomor utama sales bila belum ada (`PUT /wa/numbers/{wa}/user` dengan `sales_id`, hanya pemegang menu Pengguna; sales yang punya akun tetap lewat pengguna). `/sales` kini menyertakan `id`.

## Chat: tampilan inbox gaya Komo · 2026-10-09
- **Daftar chat**: kepala akun WhatsApp (transport · pemegang · nomor) dengan tombol muat ulang, kotak **Cari chat**, chip filter dengan jumlah — **Semua · Dealer · Grup internal · Grup eksternal · Nomor baru** (grup eksternal kini terpisah dari grup internal; juga di API `tab=group_external`). Tiap baris: tag minat, **timer menunggu balasan** (sejak pesan pelanggan yang belum dijawab), sumber, pratinjau "Anda: …" bila pesan terakhir dari kita, badge belum dibaca.
- **Panel lead** (bisa ditutup/dibuka, diingat per browser): kepala gelap dengan nama, nomor, tahap + bar progres (skor dealer / identifikasi), tab **Main · AI · Sumber · Accurate · Akun · Statistik** — rincian kontak, peringatan (kredit/At risk/belum ada di master), **Rekomendasi AI**, aksi berikutnya, dan data pelanggan Accurate (kode pelanggan, limit, piutang, rata-rata bayar).
- **Percakapan**: gelembung gaya WhatsApp (latar krem, keluar hijau, jam di dalam gelembung), tautan cepat Google · Maps · Truecaller · wa.me, tombol **Analisa lanjut** (Orchestrator untuk dealer ini) dan **Accurate**, "Muat pesan sebelumnya" untuk riwayat lama, **Rekomendasi balasan (AI) — klik untuk pakai**.
- Odoo tidak dipakai di Chat; **Kirim ke Accurate** menyiapkan data pelanggan untuk disalin ke Accurate (API Accurate belum tersambung — `docs/OPEN-QUESTIONS.md`). Tetap: tidak ada pesan terkirim tanpa diketik/disetujui manusia.
- API: `/chat/threads` membawa `last_direction`, `waiting_since`; detail percakapan membawa `phone`, `dealer_code`, `dealer_city`, `waiting_since` (sqlc diregenerasi, tanpa migrasi).

## Pusat kendali & Orchestrator: halaman per sales · 2026-10-09
- Pilihan **sales** di kanan atas Pusat kendali dan Orchestrator. Memilih sales membuka **halaman sales itu** dengan alamat sendiri (`/?sales=andi`, `/orchestrator?sales=andi`) — bisa disimpan/dibagikan; tombol "Buka Orchestrator" membawa sales yang sama. Spanduk "Halaman sales: …" + tombol kembali ke Semua sales.
- Data di halaman sales hanya milik dealer sales itu, dihitung di server (tidak hanya disaring di layar): ringkasan, jadwal order, lewat jadwal, limit tipis, KPI, agenda (hanya sales itu), push stok, rencana hari ini (langkah & usulan yang menyentuh dealernya), keputusan, resolusi konflik, dan jumlah usulan agen hari ini; jumlah WA/SO/pembayaran di ringkasan juga hanya dari dealernya.
- Pengguna berperan **sales** selalu melihat datanya sendiri (chip "Data <nama>"), walau alamat diubah ke sales lain — kini juga untuk rencana, keputusan, dan konflik yang sebelumnya tampil untuk semua.
- Orchestrator di halaman sales: kartu **Keputusan sales ini**; Riwayat analisis tetap per siklus seluruh tim (siklus tidak dipecah per sales).
- API: `?sales=<key|nama>` di `/brief/today`, `/dealers/due|drift|credit-tight`, `/kpi`, `/agenda`, `/stock/push`, `/segmen/movers`, `/plan/today`, `/proposals`, `/conflicts`, `/agents`. Test `TestSalesPageNeverMixesSales`.

## Master data → Mapping sales · 2026-10-09
- Halaman baru **Mapping sales** (`/mapping-sales`, menu Master data; untuk pemegang menu Pengguna atau Pengaturan) untuk merapikan nama sales dari BigQuery (Accurate) yang dieja beberapa kali — mis. "Granike Monica · Semua cabang" dan "Granike Monica M. · Semarang" digabung ke sales GSI Orbit **"Granike Monika"**.
- Centang beberapa nama lalu pilih sales tujuan, atau **buat sales GSI Orbit baru** dan langsung gabungkan; tiap baris juga punya "Gabung ke…" cepat. Tab **Saran gabung** menandai nama yang mirip (ejaan c/k disamakan) dan tombol "pilih" mengisi kelompoknya beserta tujuan terbaik (profil GSI Orbit → yang punya login → dealer terbanyak). Tabel dengan pencarian, urut, dan paginasi.
- Menggabung memindahkan dealer, chat, nomor WhatsApp, sinyal, interaksi Peta relasi, login, dan nomor utama ke sales tujuan; ejaan sumber nonaktif dan hilang dari semua pilihan sales (Pusat kendali, Orchestrator, Orbit, master Tim sales). Mapping impor ikut diarahkan sehingga **impor berikutnya tetap tergabung**. **Pisahkan** membatalkan gabungan (dealernya kembali setelah data diproses ulang).
- Migrasi `0019_sales_merge` (`sales_users.merged_into`), ADR 0024, test `TestSalesMergeMovesDealersAndHidesSpellings`.
- Rail: menu utama bisa di-scroll di layar pendek dan menu halaman aktif selalu terlihat.

## Mapping sales: Pengguna ↔ banyak nama BigQuery · 2026-10-10
- **Mapping sales** kini berpusat pada **Pengguna** (akun login), relasi **one-to-many**: satu pengguna memegang banyak nama sales dari BigQuery — mis. Pengguna **Granike Monika** ↔ "Granike Monica · Semua cabang" dan "Granike Monica M. · Semarang".
- Tampilan: daftar Pengguna di kiri (jumlah nama BigQuery tiap pengguna; sales yang belum punya ditandai "belum"); di kanan **Data BigQuery milik <pengguna>** (profil utama + nama-nama terhubung, tombol **Lepas**) dan **Tambah dari BigQuery** (daftar centang nama yang belum terhubung, yang mirip nama pengguna di atas dengan tanda "mirip") → **Hubungkan N nama ke <pengguna>**. Tabel **Semua nama BigQuery** dengan kolom Pengguna untuk menghubungkan langsung.
- Menghubungkan membuat profil sales utama pengguna bila belum ada (nama pengguna, cabang dari nama BigQuery-nya), lalu dealer, chat, nomor WA, dan riwayat semua nama itu menjadi milik pengguna; saat pengguna sales login, ia melihat dealer dari semua namanya sekaligus. Profil utama pengguna lain tidak bisa diambil. Impor berikutnya tetap terhubung.
- API: `/sales-map` membawa `user_id`/`main` per nama dan daftar `users`; `POST /sales-map/merge {sources, user_id}`. `POST /sales-map/profiles` dihapus (profil dibuat otomatis). Test `TestSalesMapLinksManyNamesToOneUser`, ADR 0024 diperbarui.

## Filter sales di Pusat kendali & Orchestrator (admin) · 2026-10-10
- Baris **Filter sales** di atas Pusat kendali dan Orchestrator: chip **Semua sales** + satu chip per **Pengguna sales** (hasil Mapping sales; nama pengguna + jumlah dealer). Nama BigQuery yang belum dihubungkan ke pengguna ada di daftar "Belum terhubung (N)…". Sebelum ada mapping, chip berisi 10 sales dengan dealer terbanyak.
- Memilih chip membuka halaman sales itu (`?sales=`, ikut ke Orchestrator); spanduk menyebut nama pengguna, cabang, dan jumlah dealer.
- Angka di halaman sales kini milik sales itu juga: langkah otonom (rencana hari ini), sinyal (WA/SO/pembayaran dealernya), dan konflik diselesaikan — sebelumnya angka siklus seluruh tim.
- Keamanan: peran mana pun dengan cakupan data **"Milik sendiri"** (Peran & akses) kini dibatasi di server, tidak hanya peran sales.
- `/sales` membawa `user_id`/`user_name` pemegangnya. Test `SalesPicker.test.ts`.

## MCP Claude → Pemakaian AI · 2026-10-10
- Kartu baru **Pemakaian AI** di halaman MCP Claude, untuk kedua jenis analisis AI:
  - **Siklus Orchestrator** — model yang dipakai (mis. Claude Sonnet 5.5, cadangan OpenAI) atau *Template* bila API key belum diisi, jalur analisis (API AI / MCP), **setiap** "Tiap jam 06.00–20.00 WIB" + jadwal berikutnya, harga model per 1 jt token.
  - **Analisis terjadwal (MCP)** — model, jadwal aktif (mis. "Ringkasan pagi · Senin–Sabtu pukul 07.00" + berikutnya), jumlah analisis & biaya hari ini terhadap anggaran harian (bar).
- **Biaya**: hari ini, 7 hari, 30 hari (panggilan model, token masuk/keluar, rupiah perkiraan) dan tabel **per model** 30 hari.
- **Riwayat analisis**: gabungan siklus Orchestrator dan analisis terjadwal — waktu, pemicu (terjadwal / manual / MCP + siapa), model, token, biaya, durasi, status; filter jenis, pencarian, urut asc/desc, paginasi. Diperbarui tiap menit.
- API `GET /mcp/usage` (menu MCP Claude/Pengaturan; sales tidak melihat biaya). Test `TestAIUsageShowsModelCostAndHistory`, `usage.test.ts`.

## Pemakaian AI: "fake" ditampilkan apa adanya sebagai Template (tanpa AI) · 2026-10-10
- Server tanpa API key Claude berjalan dengan `LLM_PROVIDER=fake`: tidak ada model AI yang dipanggil; agen memakai kalimat template dari aturannya sendiri, sedangkan semua angka dan usulan dihitung dari data asli. Riwayat sebelumnya menampilkan model "fake" dan "361 panggilan model" — menyesatkan.
- Kini: model tertulis **Template (tanpa AI)**, langkah tanpa model dihitung terpisah ("26 langkah template, tanpa AI") dan tidak lagi disebut panggilan model; total biaya/panggilan hanya menghitung panggilan AI sungguhan. Kotak peringatan **"AI belum aktif — mode template"** menjelaskan artinya dan cara mengaktifkan Claude (`LLM_PROVIDER=anthropic` + `ANTHROPIC_API_KEY`).

## MCP Claude: muat otomatis + Riwayat MCP · 2026-10-10
- **Muat otomatis**: halaman MCP Claude memperbarui dirinya — koneksi tiap 15 dtk, panggilan tool tiap 10 dtk, Pemakaian AI tiap 30 dtk, info server tiap 30 dtk (dan saat tab dibuka lagi). Claude yang baru tersambung atau baru memanggil tool langsung tampil tanpa reload (diuji: koneksi & panggilan baru muncul ≤ 16 dtk).
- **Riwayat MCP** (menggantikan "Log panggilan · 20 terakhir"): data table lebar penuh berisi 500 panggilan terakhir — waktu, koneksi + jenis (Claude login / token manual / analisis terjadwal) + pengguna, tool + argumen, hasil, durasi, status (Berhasil / Hanya manusia / Dibatasi / Gagal); filter jenis, pencarian, urut asc/desc, paginasi, penanda **Live**.
- **Pemakaian AI**: kotak baru **Claude lewat MCP** (status Terhubung, koneksi aktif & pemiliknya, panggilan tool hari ini, sesi 30 hari, terakhir aktif; dibayar langganan Claude, tanpa biaya API di server) dan filter **Claude lewat MCP** di Riwayat analisis — panggilan tool dikelompokkan per sesi (jeda > 30 mnt = sesi baru) beserta tool yang dipakai. Peringatan mode template kini menyebut bahwa Claude sudah terhubung lewat MCP.
- API: `/mcp/calls` membawa `client_kind`/`user_name`, batas 500; `/mcp/usage` membawa `mcp` dan sesi Claude. Test `TestMCPSessions`.

## Ganti nama pengguna menjalar ke semua tempat · 2026-10-10
- Mengganti **Nama** di Master data → Pengguna kini juga mengganti nama profil sales pengguna itu (pemilik dealer, keputusan, agenda) dan nama koneksi Claude-nya ("Claude · Sam Setiadi" → "Claude · Admin"). Profil yang berasal dari BigQuery tetap memakai ejaan sumber. Rail, Mapping sales, dan MCP Claude langsung memperbarui nama tanpa reload. Test `TestRenameUserCarriesToProfileAndClaude`.

## Wajib AI — template dihapus dari analisis · 2026-10-10
- Kebijakan baru **wajib AI** (`llm.routing.require_ai`, aktif di produksi): semua analisis harus dari AI; yang tidak dikerjakan AI dicatat **Gagal** — tidak ada lagi usulan, ringkasan, atau laporan template.
- **Tanpa API key**, siklus Orchestrator otomatis memakai **jalur MCP**: data tiap agen diterbitkan ke Claude dan siklus menunggu `MCP_WAIT` (baru, bawaan 10 menit). Agen yang dikirimi analisis oleh Claude (`orchestrator_submit`) menghasilkan usulan bersumber MCP; yang tidak = **Gagal**; tak satu pun = siklus **Gagal** dengan petunjuk "minta Claude: jalankan analisis GSI Orbit". Diuji lokal: tanpa jawaban → siklus #6 Gagal, 6 agen gagal, 0 usulan baru; dengan jawaban Claude untuk AI Kredit → 2 usulan dari MCP, 5 agen Gagal, siklus Sebagian.
- **Dengan API key**, usulan yang teksnya tidak ditulis model dibuang dan agennya Gagal.
- **Ringkasan Orchestrator** hanya ditulis AI; Pusat kendali menampilkan "Belum ada ringkasan dari AI" (bukan kalimat template). **Analisis terjadwal** tanpa kunci Claude = Gagal.
- MCP Claude → Pemakaian AI: kotak "Wajib AI", model "Claude lewat MCP" / "Tanpa AI — gagal", riwayat siklus "x/6 agen dianalisis AI" dan jumlah usulan dari Claude (MCP); siklus lama ditandai "tanpa AI (template, sebelum wajib AI)". ADR 0025.

## Tombol "Analisis ulang" → dianalisis Claude lewat MCP, hasil tetap di sistem · 2026-10-10
- Klik **Analisis ulang** (Pusat kendali, Orchestrator, Dock, "Lewat MCP") saat wajib AI dan server tanpa model: siklus dikerjakan **Claude lewat MCP** dan usulannya tersimpan di GSI Orbit (`orchestrator_submit`).
  - **Kunci Claude API tersimpan** (Analisis terjadwal → Atur mesin): Claude di server langsung mengerjakan lewat tool MCP (job `cycle.mcp_analyst`, jadwal tersembunyi "Analisis ulang lewat MCP (tombol)") — tanpa membuka apa pun.
  - **Tanpa kunci**: panel **"Claude menganalisis lewat MCP"** terbuka — tombol **Buka Claude** membuka claude.ai dengan perintah siklus itu sudah terisi (cukup tekan Enter, memakai langganan Claude), salin perintah, dan progres per agen yang diperbarui tiap 3 detik ("dikirim Claude 06.45" / "menunggu Claude…" / "Gagal · tanpa jawaban").
- Siklus dengan via MCP (tombol atau `orchestrator_run` dari Claude) selalu memakai jalur MCP. API: `POST /cycles` membawa `mcp {engine, prompt, claude_url, wait_sec, agents}`; `GET /cycles/{id}/inputs`. Diuji lokal: klik → panel → Claude (simulasi MCP) mengirim AI Kredit → tercentang, 2 usulan bersumber MCP tersimpan, agen lain Gagal setelah batas tunggu, siklus #8 Sebagian.
## Deploy · 2026-10-10
- Folder produksi dipindah dari `/opt/distri-arc` ke `/var/www/crm_distri` (sejajar aplikasi lain di server). `deploy.sh`, unit systemd, nginx, dan `docs/DEPLOY.md` ikut diperbarui; data (`/var/lib/distri-arc`, `/etc/distri-arc/env`) tidak berubah.
