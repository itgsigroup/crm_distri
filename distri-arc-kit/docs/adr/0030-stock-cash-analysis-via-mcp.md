# ADR 0030 — Analisa Stok dan Analisa Kas lewat MCP

Status: diterima · 2026-10-13 · mengikuti pola ADR 0026 (Analisa WhatsApp)

## Konteks
Permintaan: analisa di halaman **Push stok** dan **Kredit · kas** dibuat Claude lewat MCP "seperti yang lain", bukan
analisa GSI Orbit sendiri. Sebelumnya tombol *Analisis ulang* di kedua halaman menjalankan siklus lewat API, dan isi
halaman yang tampak seperti saran AI berasal dari aturan tetap: "Usul: diskon atau retur ke supplier" / "Usul: bundle
untuk dealer tier C" (umur stok > 180 hari) dan "Pengingat H-3 otomatis" — padahal GSI Orbit tidak pernah mengirim
pesan otomatis.

## Keputusan
- **Dua pembacaan baru**, sejajar dengan Analisa WhatsApp: agen baca `Analisa Stok` (stok menua dengan nilai terbesar,
  kandidat dealernya, SKU kritis) dan `Analisa Kas` (piutang terbuka terbesar, pola bayar, prediksi kas 30 hari). Angka
  tetap dihitung GSI Orbit (`views`, rumus glossary); Claude hanya menulis `overview` (≤ 800 karakter) dan satu `note`
  per item (≤ 300 karakter, `confidence` 0–1).
- **MCP**: `orchestrator_input_get {cycle_id, agent: "Analisa Stok" | "Analisa Kas"}` memberi masukan yang dimasking
  (nama sales → `<PIC_n>`) dan menyimpannya di `cycle_inputs`; `orchestrator_submit {…, overview, lines: [{key, note,
  confidence}]}` divalidasi terhadap masukan itu (key harus ada), di-unmask, disimpan di `screen_analyses` (migrasi
  0026) dengan `source = ai`. Prompt *Analisis ulang* meminta pembacaan halaman yang agennya ikut di siklus (AI Stok →
  Analisa Stok; AI Kredit/AI Penagihan → Analisa Kas).
- **Tombol Analisis ulang** di Push stok dan Kredit · kas selalu lewat MCP (`via: mcp`): membuka lembar "Claude
  menganalisis lewat MCP" (atau Claude di server bila kunci API tersimpan).
- Dengan model di server dan siklus bukan lewat MCP, Belajar meminta model menulis pembacaan yang sama (seperti
  Analisa WhatsApp). Hanya jawaban model yang disimpan; tanpa jawaban AI halaman menulis "Belum dianalisis AI".
- **UI**: kartu ringkasan AI dengan asal-usul (ditulis AI · siklus · waktu) dan catatan per baris (SKU per cabang,
  dealer). Saran aturan tetap dan "Pengingat H-3 otomatis" dihapus.
- `GET /screen-analysis/{stock|credit}` memberi pembacaan terakhir; catatan Kas dipetakan ke id dealer halaman dan hanya
  dealer yang boleh dilihat pengguna.

## Konsekuensi
- Isi AI hanya muncul setelah Claude mengirim lewat MCP; catatan per baris berlaku untuk item yang ada di masukan
  (maks. 20 terbesar).
- Usulan aksi (push stok, tagih, rilis kredit) tetap proposal agen AI Stok / AI Kredit / AI Penagihan yang diputuskan
  manusia.
