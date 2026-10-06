# ADR 0013 — Kebijakan berversi, login, peran
**Status**: diterima · 2026-10-06 · melengkapi `09-policies-security.md`

## Keputusan
1. **Kebijakan**: JSON Schema per key di `internal/policy/schema/` (tertanam; divalidasi dengan `github.com/google/jsonschema-go`,
   sudah ada lewat SDK MCP). Aturan terkunci di schema/kode: `sop_sec_001_required = true`, `release_over_limit = ceo_approve`,
   `mcp.permissions.allow_send = false`, `credit_release`/`credit_limit` tidak pernah otonom, churn > lewat jadwal.
   `policy.Save` = validasi → versi naik → `policy_history` → `audit_log` → NOTIFY `policy_changed`. Hanya CEO.
2. **Berlaku di siklus berikutnya**: Orchestrator membaca kebijakan dari DB tiap siklus; bila ambang yang memengaruhi metrik
   berubah sejak siklus terakhir, tahap Ingest menghitung ulang semua dealer. Tidak ada cache kebijakan di proses API.
3. **Login**: `users.password_hash` argon2id (parameter OWASP, dipakai juga untuk token MCP); sesi = JWT HS256 (stdlib) di
   cookie `arc_session` HttpOnly, SameSite=Strict, Secure di balik HTTPS, 12 jam; `SESSION_SECRET` ≥ 32 karakter wajib di luar
   dev. Rate limit 10 gagal / 15 menit per IP+email. `X-Dev-User` hanya di `APP_ENV=dev`, dan cookie sesi didahulukan.
   `ARC_DEMO_PASSWORD` (dev saja) memberi akun contoh kata sandi saat seed/reset.
4. **Peran** (`ceo`, `admin`, `finance`, `sales`, `warehouse`): menu dari `/me.screens`; keputusan per jenis
   (`proposals.RolesFor`): rilis kredit CEO; limit CEO/finance; penagihan CEO/admin/finance/sales; transfer & PO
   CEO/admin/gudang; bundle, harga khusus, perubahan rencana CEO/admin; lainnya CEO/admin/sales. Sales hanya dealer miliknya
   (atau nomor baru yang menulis ke nomornya) dan daftar dealer difilter server; halaman satu dealer tetap terbuka.
   Pengguna dikelola CEO/admin (CEO baru hanya oleh CEO).
5. **Panduan**: isi layar "Konsep orbit" mockup disalin apa adanya (`web/src/features/guide/panduan.html`, uji sinkron) dan
   tombolnya menjadi navigasi aplikasi.
