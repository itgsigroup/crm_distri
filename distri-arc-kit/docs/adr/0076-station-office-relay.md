# ADR 0076 — Stasiun Orbit menjadi kantor kru AI dengan estafet hasil

Status: diterima · 2026-10-28 · Sam

## Konteks
Sam memberi contoh video "kantor virtual 3D untuk AI agent". Permintaannya: dashboard astronot dibuat seperti 3D office
agent, untuk ranah CRM. Astronot 1 menganalisa, lalu berjalan ke astronot lain memberikan hasil analisanya, dan
seterusnya sampai astronot 10. Latar yang dipilih: kantor di dalam stasiun luar angkasa. Videonya sendiri tidak bisa
ditonton Claude. Yang terbaca hanya judulnya.

## Keputusan
- Sepuluh ruangan berkubah dan cincin stasiun diganti **satu dek kantor bundar** di dalam stasiun. Isinya lantai panel
  logam, jalur jalan bercahaya, dinding rendah, dan kaca panorama ke luar angkasa. Inti AI (Orchestrator) melayang di
  atas alasnya di tengah. Sepuluh **meja kerja** mengelilinginya, masing-masing dengan astronot, layar, hologram, tiang
  lampu, dan cincin status di lantai.
- **Estafet**: begitu astronot N selesai, ia mengambil kapsul hasil (bercahaya), memutari mejanya ke jalur, berjalan
  menyusuri jalur ke depan meja N+1, menyerahkan kapsul (hasilnya terbang ke hologram meja itu), lalu kembali ke mejanya.
  Jejak percikan berwarna meja asal menandai jalannya. Yang menggerakkan estafet adalah status siklus sungguhan
  (`crewStatus`, sama seperti sebelumnya). Dalam siklus nyata, beberapa astronot bisa berjalan bersamaan karena agen
  4–9 bekerja paralel.
- **Putar alur kerja**: tombol baru yang memutar estafet 1 → 10 secara berurutan sebagai tayangan. Astronot N
  menganalisa sekitar 2,4 detik, lalu berjalan. Astronot N+1 baru mulai setelah kapsul diterima. Tayangan ini tidak
  menjalankan analisis dan tidak memanggil AI. Meja yang selesai menampilkan baris yang sudah dihitung dari data (sama
  seperti kartu "Hasil overview AI"). Siklus sungguhan yang dimulai di tengah tayangan langsung mengambil alih.
- Tetap tanpa data pelanggan di scene. Klik meja membuka halamannya, dan klik inti AI membuka Orchestrator, sama seperti
  sebelumnya. Dengan `prefers-reduced-motion`, astronot tidak berjalan dan kapsul langsung diserahkan.

## Akibat
- Alur kerja Orchestrator terlihat sebagai kerja tim yang berurutan. Tayangan bisa diputar kapan saja tanpa biaya AI.
- Diverifikasi di Chromium headless (SwiftShader): dek, meja, label, estafet berjalan, penyerahan, dan kembali ke meja.
