# ADR 0064 — Balasan live di Chat tidak ditahan batas volume anti-blokir

Status: diterima · 2026-10-24 · Sam

## Konteks
Sam (24 Okt): "saat chat grup tidak bisa terkirim, harusnya bisa terkirim seperti chat pada umumnya". Pesan hanya
menampilkan "mengirim…" dan tidak sampai ke HP. Data produksi: nomor 6289506401111 mengirim ±22 balasan antara 09.48
dan 10.09 WIB (grup dan chat pribadi), lalu penjaga anti-blokir bridge (ADR 0004/0017) menolak sementara setiap kiriman
berikutnya dengan *"Batas 20 pesan/jam untuk nomor ini tercapai"*. Sejak 10.10 ada 13 pesan dari nomor itu yang
tertahan di status `pending`, termasuk ke grup. Batas itu dibuat untuk follow-up dan proposal (pola bulk), tetapi ikut
menghitung balasan yang diketik orang di percakapan yang sedang berjalan. Batas 6 pesan/chat/jam ("tunggu balasan
dulu") dan jam tenang 21.00–07.00 menahan percakapan grup yang ramai dengan cara yang sama. Di layar Chat alasan
penahanan tidak tampil, sehingga terlihat seperti pesan gagal tanpa sebab.

## Keputusan
- Balasan yang diketik orang di Chat (`outbox` kind `reply`, termasuk saran AI Trigger yang dipakai lalu dikirim)
  dikirim ke bridge dengan `live: true` (`wa.WithLive`). Kiriman lain (follow-up, proposal yang di-approve) tetap
  non-live.
- Untuk kiriman live, penjaga **tidak** menerapkan batas per jam/hari, pemanasan perangkat baru, batas per-chat per
  jam, dan jam tenang. Yang **tetap** berlaku: opt-out, kontak dingin (belum pernah menyapa), pola broadcast untuk teks
  ≥ 30 karakter (teks pendek seperti "ok" / "terima kasih" di banyak chat adalah balasan, bukan broadcast), jeda
  20 detik ke chat yang sama, jeda acak, dan "mengetik…" seperti manusia. Ditambah plafon pengaman
  `BRIDGE_MAX_LIVE_PER_HOUR` (default 100 pesan/jam per nomor, menghitung semua kiriman) untuk menghentikan kiriman
  yang lepas kendali.
- Batas volume lama (20/jam, 120/hari, 6/chat/jam) hanya menghitung kiriman **non-live**
  (`wa_bridge.arc_sends.live`), sehingga orang yang sedang aktif chat tidak menghabiskan jatah follow-up.
- Pesan yang tertahan menampilkan alasannya di bubble ("tertahan · Batas 20 pesan/jam … · terkirim otomatis begitu
  diizinkan"), diambil dari `outbox.error`, menggantikan "mengirim…" tanpa keterangan.

## Akibat
- Orang bisa chat di grup dan percakapan pribadi dengan kecepatan wajar tanpa pesan menumpuk. Risiko blokir tetap
  dijaga untuk pola bulk, sedangkan aktivitas live tetap terlihat seperti manusia karena ada jeda dan "mengetik…".
- Balasan pada malam hari kini terkirim. Bila ingin kembali menahan, turunkan `BRIDGE_MAX_LIVE_PER_HOUR` atau
  ganti ADR ini.
- Kolom `live` ditambahkan ke skema bridge (`ALTER TABLE … ADD COLUMN IF NOT EXISTS`, pola DDL bridge yang sudah ada).
  Kiriman lama terhitung non-live.
