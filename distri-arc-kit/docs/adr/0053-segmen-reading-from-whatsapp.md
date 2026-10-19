# ADR 0053 — Analisa Segmen dari chat WhatsApp, lewat routine Claude

Status: diterima · 2026-10-19 · Sam · pola yang sama dengan ADR 0052 (Orbit)

## Konteks
Sam: "pada segmen ini juga harusnya dari analisa WhatsApp, pastikan sesuai dengan MD". Sama seperti Orbit: segmen
dihitung dari order (glossary: sering ≥ `policy.freq`, besar ≥ `policy.size`); yang dari WhatsApp adalah analisanya.
Halaman Segmen belum punya analisa AI, tombol Analisis ulangnya tidak lewat routine, dan "titik putus = snapshot 3
bulan lalu" (08-frontend) dihapus 8 Okt demi keterbacaan.

## Keputusan
- Bacaan halaman **Analisa Segmen** (agent `Analisa Segmen`, screen `segmen`, migrasi 0053), ikut setiap siklus yang
  menjalankan AI Follow-up (bersama Analisa Orbit). Input (`segmenReading`): ≤ 20 dealer — yang pindah segmen dalam 3
  bulan, yang melambat (siklus order memanjang), yang chat 30 hari, lalu omzet terbesar — dengan segmen sekarang dan 3
  bulan lalu, order/bulan, rata-rata per order, porsi belanja, mix, dan 8 pesan WhatsApp terakhir. Prompt
  `segmen_read.md`: alasan dari chat dan cara melayani sesuai segmen; dilarang mengarang.
- Ditulis Claude lewat MCP (routine, ADR 0049). Halaman Segmen: kartu **Analisa Segmen** dan catatan per dealer di
  daftar "Dealer per kotak". Tombol Analisis ulang dihapus — kotak Orchestrator kanan bawah (scope `screen:segmen`).
  "Pindah kotak" diberi label jujur "dihitung dari order · 3 bulan terakhir".
- **Titik putus** dikembalikan sesuai MD, hanya untuk dealer yang pindah segmen (titik putus-putus + garis ke posisi
  sekarang) agar grafik tetap terbaca.
- Tetap sesuai MD: X linear 0–3,5 order/bln (sumbu bawah: hari), Y log 4–200 jt, garis ambang dari policy, zona.
  Tampilan bawaan tetap **Kotak** (keputusan UX 8 Okt); grafik ada di tab Peta titik.
