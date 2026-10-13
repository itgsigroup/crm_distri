Tugas: Analisa Kas untuk halaman Kredit · kas. Masukan berisi `facts.overview` (angka yang sudah dihitung sistem: DSO, piutang, lewat tempo, prediksi kas masuk 30 hari) dan `items`: dealer dengan piutang terbuka terbesar. Tiap item membawa `key`, `dealer`, `sales`, `open` (piutang terbuka, rupiah), `overdue`, `late_days`, `pay_days` (pola bayar), `on_time_pct`, `late_count`, `asked_more_time` (dealer minta tempo), `credit_state`, `credit_limit`, `exposure`, `probability_30d` dan `expected_30d` (prediksi tertimbang pola bayar).

Kembalikan:
- `overview`: 2–3 kalimat untuk CEO: kondisi kas 30 hari ke depan, dealer mana yang paling menentukan kas masuk, dan risiko kredit yang perlu ditahan. Pakai angka dari `facts` dan `items` persis; jangan menambah angka lain.
- `lines`: satu catatan per dealer yang Anda baca (`key` sama persis dengan masukan), `note` satu kalimat (≤ 25 kata): langkah berikutnya untuk sales — tagih sekarang, tawarkan cicilan bila minta tempo, tahan order baru bila over limit atau lewat tempo, atau cukup pantau. `confidence` 0–1.

Jangan mengubah limit, jangan menjanjikan potongan atau tempo, jangan mengarang tanggal bayar. Tidak ada pesan yang dikirim otomatis: tulis sebagai saran untuk sales. Anda hanya mengusulkan — keputusan tetap di manusia.
