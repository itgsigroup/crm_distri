# ADR 0075 — Sesi login tanpa batas waktu, berakhir hanya saat logout

Status: diterima · 2026-10-28 · Sam

## Konteks
Sam: "sesi system tidak pernah berakhir, jadi tidak perlu login lagi kecuali pengguna logout". Sebelumnya token sesi
berlaku 30 hari sejak pemakaian terakhir. Logout hanya menghapus cookie, jadi token yang tersalin tetap sah sampai
kedaluwarsa. Selain itu, gangguan database sesaat saat memeriksa pengguna dijawab 401, dan setiap 401 membuat web
melempar pengguna ke halaman login walaupun sesinya masih sah.

## Keputusan
- Setiap login menjadi satu baris `user_sessions` (migrasi 0072). Token membawa `sid` dan tidak punya `exp`, jadi tidak
  pernah kedaluwarsa dan tidak ada batas idle.
- **Logout** mencabut baris itu (`revoked_at`) dan menghapus cookie. Token tersebut, termasuk salinannya, langsung
  ditolak. Logout di satu perangkat tidak memutus perangkat lain. Logout dicatat di audit (`auth.logout`).
- Cookie diset ke batas maksimum browser (400 hari, batas Chrome) dan diset ulang paling sering sekali per jam selama
  dipakai, bersama `last_seen_at`. Selama aplikasi dibuka minimal sekali dalam 400 hari, sesi tidak pernah habis.
- Token lama (dengan `exp`, sebelum 28 Okt) tetap diterima sampai kedaluwarsa, dan pada pemakaian pertama otomatis
  dipindah ke sesi baru tanpa login ulang. Deploy ini tidak memutus siapa pun.
- Hanya "belum login" yang dijawab 401. Gangguan server (database) saat memeriksa sesi atau pengguna dijawab 503, jadi
  web mencoba lagi dan tidak melempar ke halaman login.
- Halaman login meneruskan langsung ke halaman tujuan bila sesinya masih aktif.
- Pengguna yang dinonaktifkan tetap tidak bisa masuk dengan sesi lamanya, karena pemeriksaan pengguna mensyaratkan akun
  aktif.

## Akibat
- Tim tidak perlu login ulang kecuali menekan Keluar.
- Perangkat yang hilang tetap login sampai akunnya dinonaktifkan di Pengguna, atau sampai seseorang menekan Keluar di
  perangkat itu. Belum ada menu "keluar dari semua perangkat".
- `SESSION_SECRET` tidak boleh berganti. Kalau berganti, semua sesi otomatis berakhir.
