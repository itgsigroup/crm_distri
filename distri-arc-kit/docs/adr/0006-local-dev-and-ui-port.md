# ADR 0006 — Penyesuaian implementasi: dev tanpa Docker, port CSS mockup, seed dari mockup
**Status**: diterima · 2026-10-05 · melengkapi ADR 0001 dan 0005 (tidak menggantikan)

## Konteks
Mesin pengembangan GSI tidak punya Docker; PostgreSQL 17 berjalan native. Sam meminta tampilan **persis** mockup
(`reference/distri-arc-orbit-v2-mockup.html`). Kit lama (ARC v1) masih ada di repo induk `crm_distri`.

## Keputusan
1. **Database dev**: `DATABASE_URL` menunjuk PostgreSQL mana pun ≥ 16 (native atau `make db-up` via Compose).
   `scripts/dev.sh` memakai Compose bila Docker ada dan `DEV_DB=compose`, selain itu server native.
2. **Uji integrasi**: `DATABASE_URL_TEST` + skema sekali pakai per test (`internal/testdb`) menggantikan
   testcontainers-go; ekstensi `pgcrypto`/`pg_trgm` dipasang di skema `public`. CI memakai service Postgres 16.
3. **Toolchain**: Go 1.27 (dependensi river/pgx butuh ≥ 1.26; kit menyebut minimal 1.23). React 19, react-router 8
   (penerus 7, API data router sama), Vite 8, TypeScript 6.
4. **Styling**: blok `:root` mockup disalin ke `web/src/styles/tokens.css` dan sisa CSS mockup **verbatim** ke
   `web/src/styles/mockup.css` (global, nama kelas sama). Komponen React memakai nama kelas mockup. Ini mengganti
   "CSS Modules" di ADR 0001 karena tujuan utamanya — visual mockup dipindah apa adanya — lebih terjamin dengan
   CSS yang identik byte per byte. Font di-self-host lewat `@fontsource`. Ikon: sprite dari mockup
   (`web/src/icons/sprite.svg`).
5. **Seed**: `tools/seedgen/extract_mockup.mjs` mengekstrak data contoh mockup (18 dealer, chat, aksi) ke
   `tools/seedgen/mockup.json`; `go run ./tools/seedgen` membangun order/invoice/pembayaran sintetis 12 bulan
   sehingga rumus glossary menghasilkan angka mockup (median siklus, rata-rata order, exposure, pola bayar, tepat
   waktu, product mix). Record seed memakai `source_system='odoo'` dan id berbentuk Odoo agar sync Odoo nanti
   meng-upsert record yang sama.
6. **Jam**: `ARC_NOW` (hanya `APP_ENV=dev`) memulai jam aplikasi pada 5 Okt 2026 06.45 WIB (anchor data contoh)
   dan tetap berdetak.
7. **Lokasi proyek**: dibangun di subfolder `distri-arc-kit/` pada branch `distri-arc-orbit` repo `crm_distri`;
   kode ARC v1 tidak disentuh sampai Sam memutuskan dihapus/dipindah.

## Konsekuensi
- Visual mockup bisa dibandingkan langsung (kelas dan token sama); perubahan desain dilakukan di mockup lalu disalin.
- Data contoh punya beberapa angka mockup yang saling bertentangan (mis. teks "Order terakhir Rp 22 jt" vs exposure
  Rp 162 jt Mitra Jaya). Rumus menang; teks seed disesuaikan dengan angka hasil hitung.
