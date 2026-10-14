# ADR 0035 — Antrean chat bersih dan waktu respon per jam kerja (laporan direktur 14 Okt)

Status: diterima · 2026-10-14 (Sam) · tugas 1, 2, 4, 18 laporan "Analisis Chat WhatsApp dan Rencana Penyempurnaan CRM"

## Keputusan
- **Penutup sopan bukan permintaan.** `is_closing_message(body)` → kolom terhitung `chat_messages.closing` ("ok bu",
  "siap pak terima kasih", emoji, stiker/GIF; kata yang sama dengan `waanalysis.IsAck`). Pesan seperti itu tidak memulai
  waktu tunggu di "Belum dibalas", Analisa WhatsApp, maupun status per bubble.
- **Balasan otomatis bukan jawaban dan bukan permintaan.** `is_auto_reply(body)` → `chat_messages.auto_reply`
  (sapaan "Terima kasih telah menghubungi …", "sedang tidak di tempat", "di luar jam kerja", "jam operasional kami …").
  Bubble ditandai "balasan otomatis · tidak dihitung". GSI Orbit sendiri tidak pernah mengirim pesan otomatis;
  pengaturan auto-reply WhatsApp Business ada di HP (tugas 18, lihat OPEN-QUESTIONS).
- **Tipe kontak** (`contact_types`, per nomor): *pemasok* — tetap tersimpan, chip "Pemasok", tidak masuk "Belum dibalas"
  dan tidak dianalisa sebagai lead; *pribadi* — semua chat nomor itu dihapus dari CRM (pesan + sinyalnya) di semua nomor
  GSI, pesan berikutnya tidak disimpan (`DropPersonal`). Ditandai dari panel Chat; tercatat di audit.
- **Waktu respon per jam kerja** (kebijakan `chat.rules`, bawaan Senin–Sabtu 08.00–17.00 WIB): pesan pelanggan di jam
  kerja diukur dalam menit kerja; di luar jam dilaporkan terpisah. Ringkasan dan per sales: median + **p90** jawaban
  substantif (`wa_insights.reply_work_min/reply_off_min`). "Perlu dibalas" = menunggu ≥ 2 jam **kerja**.
  Data contoh memakai 06.00–22.00 setiap hari agar cerita demo tetap.
- **Kredensial disamarkan** di Chat, kutipan, MCP, dan sebelum LLM (`llm.MaskCredentials`: password/sandi/PIN/OTP/kode
  verifikasi → "••••••").

## Konsekuensi
- Pola tetap (bukan AI); kalimat di luar pola dihitung seperti biasa — pola diperluas lewat migrasi.
- Rekening tidak disamarkan di Chat (instruksi bayar GSI perlu terlihat sales); tetap disamarkan sebelum LLM.
