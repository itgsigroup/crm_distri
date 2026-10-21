# ADR 0057 — AI Trigger: jawaban wajib dari ChatGPT atau Claude, template tidak pernah tampil

Status: diterima · 2026-10-21 · Sam · menggantikan bagian "template sebagai cadangan" dari ADR 0051

## Konteks
Kartu "Tanya stok" untuk Ezviz NVR X5S 8W menampilkan draf template ("Mohon maaf, … sedang kosong") dengan tombol
*Pakai jawaban*, bukan tulisan AI. Template muncul diam-diam di empat keadaan: trigger yang mematikan "Jawaban ditulis
AI", AI yang gagal (galat atau draf yang mengubah angka), fakta yang berubah setelah draf ditulis (sinkron stok tiap 5
menit, ADR 0056 — draf AI terikat `facts_hash` dan dibuang), dan tanpa kunci AI. Sam: "jawaban trigger AI pada chat
harus dari Claude atau ChatGPT, itu wajib, jangan sampai manual".

## Keputusan
- `reply_rules.ai_phrase` selalu `true` (migrasi 0058 menambah `check (ai_phrase)`; simpan aturan memaksa `true`;
  checkbox diganti keterangan "wajib"). Template tetap ada sebagai **kerangka fakta** yang dibaca AI, tidak ditampilkan.
- Satu draf dicoba dengan AI pilihan (mesin AI Trigger, atau tombol ChatGPT/Claude di kartu), lalu AI lainnya yang
  kuncinya terisi bila yang pertama gagal (`replyai.modelsFor`).
- Kartu di Chat dan *Coba kalimat* hanya menampilkan draf AI untuk fakta saat ini. Tanpa itu: "ChatGPT / Claude sedang
  menulis…" atau alasan (tanpa kunci / AI tidak berhasil) dengan tombol ChatGPT dan Claude — tanpa *Pakai jawaban*.
- Fakta yang berubah (draf AI tidak cocok lagi, pesan > 2 menit) memasukkan job `reply.ai` baru berkunci
  `message_id + facts` (`replyai.CardsHash`); unik di semua status, jadi membuka chat berkali-kali tidak memanggil AI
  lagi untuk fakta yang sama, termasuk yang sudah gagal. Job dua percobaan.

## Akibat
- Tidak ada draf ke pelanggan yang bukan tulisan AI; angka tetap dihitung sistem dan diperiksa (`CheckDraft`).
- Biaya naik sedikit: draf ditulis ulang setiap fakta berubah saat chat dibuka (≈ Rp 15 per draf ChatGPT mini).
- Tanpa kunci ChatGPT/Claude, kartu hanya memberi data, tanpa draf.
