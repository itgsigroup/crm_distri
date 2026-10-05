# 09 · Kebijakan, Privasi, Keamanan

## Kebijakan (tabel `policies`, JSONB, berversi; default di `db/seed/policies.json`)
```json
{
 "orbit.thresholds":   {"drift": 1.2, "churn": 2.0, "key_account": {"sow_min": 50, "on_time_min": 85}},
 "segment.thresholds": {"freq_per_month": 1.5, "size_idr": 20000000, "new_dealer_wait_orders": 2},
 "credit.rules":       {"room_min": 0.40, "pay_max_days": 35, "default_limit": {"A": 250000000, "B": 150000000, "C": 0, "new": 25000000},
                        "limit_up": {"on_time_min": 90, "tight_months_min": 3}, "limit_down": {"late_invoices": 2, "late_days": 14},
                        "release_over_limit": "ceo_approve", "sop_sec_001_required": true},
 "followup.rules":     {"gap_days": 14, "h_minus": 1, "max_per_day_per_sales": 12, "second_followup": "approve"},
 "margin.floor":       {"pct": 9},
 "stock.rules":        {"aging_days": 90, "bundle_max_discount_pct": 8},
 "autonomy.matrix":    { "...": "lihat 05-agents.md" },
 "mcp.permissions":    {"allow_reanalyze": true, "allow_plan_update_proposal": true, "allow_send": false, "mask_pii_in_read": true, "max_cycles_per_hour": 6},
 "llm.routing":        {"mode": "both", "provider": "anthropic", "model": "claude-sonnet-4-5", "fallback": "openai:gpt-4.1", "batch_hours": [6,20], "timezone": "Asia/Jakarta"},
 "retention":          {"chat_days": 90, "signals_months": 24, "llm_calls_days": 180}
}
```
Perubahan lewat `PUT /policies/{key}` (ceo), divalidasi schema (JSON Schema per key di `internal/policy/schema/`), versi naik, history disimpan, `audit_log` dicatat, Orchestrator memuat ulang di siklus berikutnya (toast UI: "berlaku di run berikutnya").

## SOP-SEC-001 (pencegahan social engineering, tidak bisa dimatikan)
Sebelum proposal `credit_release`/`so_draft` di atas Rp 25 jt untuk dealer dengan perubahan alamat/PIC baru: (1) PO/permintaan diverifikasi via telepon ke nomor terdaftar dealer, (2) alamat kirim konsisten dengan ≥ 3 pengiriman terakhir, (3) nomor pengirim WA terdaftar sebagai PIC. Hasil dicatat sebagai sinyal `manual` oleh sales/admin; AI Kredit menampilkan checklist dan **menolak** opsi rilis bila belum lengkap.

## Privasi (UU PDP)
- **Nomor internal**: tabel `internal_numbers`; pesan DM antar nomor internal **tidak disimpan** (filter di `wa` parser sebelum DB). Grup internal (`wa_groups.kind='internal'`): hanya diekstrak untuk stok, surat jalan, jadwal, tugas; tidak memengaruhi metrik dealer.
- **Identifikasi nomor** hanya untuk nomor inbound yang tidak dikenal; sumber: profil WA Business, Truecaller (API resmi bila ada), Getcontact **impor manual** (CSV dari aplikasi; tidak ada scraping). Hasil `identifications` disimpan 90 hari bila tidak menjadi dealer.
- **Retensi**: `chat_messages` 90 hari (job harian `retention.purge`), `signals` 24 bulan, `llm_calls` 180 hari; dealer/transaksi tidak dihapus.
- **Masking ke LLM eksternal**: nomor telepon, email, NIK, rekening diganti placeholder sebelum request; mapping placeholder hanya di memori proses; `llm_calls` menyimpan hash input bukan isi.
- **Hak subjek data**: `arc ctl pdp export --dealer <id>` dan `pdp delete --contact <wa>` (menghapus kontak + pesan, menyisakan agregat).

## Keamanan
- Sesi JWT HttpOnly + SameSite Strict; password argon2id; login rate limit; 2FA TOTP opsional untuk ceo/admin (Stage 13).
- RBAC di handler (`internal/api/authz.go`) dan di query (filter sales).
- Rahasia hanya dari env (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `ODOO_*`, `WA_*`, `MCP_*`, `DATABASE_URL`, `SESSION_SECRET`); `.env` tidak di-commit; `arc ctl check-env` memvalidasi.
- WhatsApp linked-device (`whatsmeow`): sesi disimpan terenkripsi (`WA_SESSION_KEY`); hanya nomor sales terdaftar yang dipasangkan; tidak ada broadcast; maksimal `followup.rules.max_per_day_per_sales` kirim/hari; jeda acak 20–90 dtk antar kirim; semua kirim lewat outbox yang disetujui manusia. Cloud API untuk nomor bisnis resmi (tanpa grup).
- MCP: token per klien, scope, rate limit, audit, `allow_send=false` di kode.
- Backup harian `pg_dump` terenkripsi; restore diuji di Stage 13.
- Header keamanan via Caddy (HSTS, CSP untuk SPA, no-sniff). Tidak ada CORS lintas origin (SPA dilayani dari host yang sama).
