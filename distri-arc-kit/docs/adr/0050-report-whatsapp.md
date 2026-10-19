# ADR 0050 — Report WhatsApp per sales

Status: diterima · 2026-10-19 · Sam

## Konteks
Sam meminta halaman report WhatsApp: analisa tiap sales, kecepatan respons, feedback baik/buruk, nomor baru, sales
terbaik, chat gantung, dan hal lain yang penting untuk sales, direktur, dan manager.

## Keputusan
- Halaman **Report WhatsApp** (`/report-whatsapp`, layar `reportwa` di menu Kendali; migrasi 0050 menambahkannya ke
  semua peran yang punya Chat). `GET /api/wa/report?days=1|7|30&sales=` (dijaga layar `reportwa`).
- Semua angka **dihitung di Go** (`internal/wareport`) dari `chat_messages` lewat `WAWindowMessages` dengan logika
  balas yang sama dengan Analisa WhatsApp (`waanalysis.Analyze`: putaran pelanggan, balasan "mohon ditunggu" tidak
  menghentikan jam, ack/penutup diabaikan, jam kerja dari `chat.rules`). Tanpa LLM.
- Per sales (atribusi = pemegang nomor, `chat_threads.sales_id`): permintaan, dijawab, waktu respons pertama median/p90
  di jam kerja, % dalam target (`chat.rules.reply_sla_min`, bawaan 30 menit; hanya permintaan di jam kerja yang
  diukur), di luar jam kerja, menunggu sekarang, **chat gantung** (menunggu ≥ 2 jam kerja), **janji "cek dulu"** yang
  belum ditindaklanjuti ≥ 2 jam kerja, nomor baru dan waktu balas pertamanya, **permintaan order di chat yang jadi SO**
  (order dealer itu ≤ 3 hari setelah permintaan, tabel `orders`).
- **Skor 0–100**: dijawab 35 · tepat waktu 30 · kecepatan terhadap target 20 · tanpa gantung 15. Peringkat hanya untuk
  sales dengan chat; "Tanpa sales" (nomor admin/penagihan/tanpa pemegang) tidak diperingkat. **Winning sales** tampil
  bila skor ≥ 60 dengan minimal 3 chat.
- **Feedback baik/buruk** per sales dan **ringkasan untuk direktur/manager** disusun dari angka (aturan tetap di Go).
- Juga: tren harian (satu seri, median di tooltip — tanpa sumbu ganda), peta jam ramai pelanggan (hari × jam, satu
  warna), tabel chat gantung dan nomor baru (cari, urut, halaman).
- Visibilitas sama dengan Chat: sales hanya percakapannya sendiri; pemegang nomor (`chat_scope own`) hanya nomornya;
  lainnya memilih sales lewat `?sales=`.

## Konsekuensi
- Report 30 hari membaca semua pesan sebulan setiap dibuka (disegarkan tiap 5 menit, tidak per pesan masuk). Bila berat,
  pindah ke ringkasan harian tersimpan.
- `chat.rules.reply_sla_min` kini dipakai (sebelumnya tidak dibaca kode mana pun).
- Feedback berbentuk aturan, bukan kalimat Claude; kalimat coaching dari Claude bisa ditambahkan lewat routine kemudian.
