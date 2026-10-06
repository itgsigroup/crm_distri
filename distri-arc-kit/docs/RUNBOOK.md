# Runbook — insiden umum

Alat utama: **Pengaturan → Status sistem** (CEO/admin), log JSON (`docker compose -f infra/docker-compose.prod.yml logs -f worker api`),
`arc ctl …` (di Docker: `docker compose -f infra/docker-compose.prod.yml run --rm api ctl …`). Alert otomatis masuk ke grup
WhatsApp internal (`ALERT_WA_GROUP`) sekali per insiden dan "✅ Pulih" saat selesai.

## 1. WhatsApp terputus (alert "WhatsApp … terputus")
1. Status sistem → baris WhatsApp menunjukkan nomor mana. Penyebab umum: ponsel sales mati/tanpa internet > 14 hari,
   perangkat tertaut dihapus dari ponsel, atau WhatsApp membatasi akun.
2. Minta sales membuka WhatsApp di ponsel (online). Bila status `logged_out`: Pengaturan → WhatsApp → pasangkan ulang (QR).
3. Pesan yang disetujui selama terputus tetap di outbox (`pending`) dan dikirim setelah terhubung — tidak ada yang hilang.
4. Bila akun dibatasi WhatsApp: hentikan kirim proaktif nomor itu (Matriks otonomi → follow-up ke Konfirmasi), turunkan
   `followup.rules.max_per_day_per_sales`, dan jangan pasangkan nomor lain untuk "menyiasati" (ADR 0007).

## 2. Odoo gagal sync (Status sistem: Odoo "Gagal")
1. `arc ctl odoo test` — kredensial/URL. Error 401/403: API key user read-only dicabut atau kadaluarsa → buat ulang di Odoo.
2. Timeout: Odoo sedang berat/maintenance; sync berikutnya (10 menit) melanjutkan dari kursor `write_date` (idempoten).
3. Field hilang setelah upgrade Odoo: lihat `error` per model di Status sistem; mapper ada di `internal/odoo/sync.go`.
4. SO draft gagal dibuat (toast "gagal"): saran tetap *disetujui*, worker mencoba 3×; lalu buat manual di Odoo dan beri
   catatan. Jangan ubah `ODOO_WRITE` di tengah insiden tanpa CEO.

## 3. Siklus Orchestrator gagal 2× (alert)
1. `arc ctl cycle status` — catatan error tahap. Log: `grep '"cycle_id"' | grep ERROR`.
2. LLM gagal (kuota/kunci): agen jatuh ke template, siklus tetap *partial* — bukan insiden. Bila Claude API mati lama,
   Pengaturan → Koneksi AI → Analisis via **MCP** atau isi `OPENAI_API_KEY` cadangan.
3. Siklus macet `running` > 1 jam: worker restart menandainya gagal (FailStaleCycles). `docker compose … restart worker`.
4. Jalankan manual: `arc ctl reanalyze --scope all`.

## 4. Antrean menumpuk (> 500 job)
1. `psql … -c "select kind, state, count(*) from river_job group by 1,2 order by 3 desc"`.
2. `outbox.send` menumpuk di luar 08–18 WIB itu normal (menunggu jendela kirim). `identify.number`/`odoo.sync` gagal
   berulang → lihat `errors` di `river_job`.
3. Worker mati: `docker compose … ps` → `up -d worker`. CPU penuh: kurangi siklus (jam 06–20 saja, default).

## 5. LLM mahal / biaya melonjak
Status sistem → AI: panggilan & biaya hari ini. `llm_calls` per `purpose`. Turunkan jam batch (`llm.routing.batch_hours`)
atau pindah model di Pengaturan → Kebijakan `llm.routing`.

## 6. Restore dari backup
```bash
# daftar backup (Docker: volume distri-arc_backups)
ls -lt /var/backups/distri-arc/
# 1. hentikan penulis
docker compose -f infra/docker-compose.prod.yml stop api worker
# 2. pulihkan (database dikosongkan dulu oleh --clean)
BACKUP_PASSPHRASE=… infra/restore.sh /var/backups/distri-arc/distri-arc-20261006-190000.dump.enc "$DATABASE_URL"
# 3. migrasi (bila backup lebih tua dari kode) lalu nyalakan
arc ctl migrate && docker compose -f infra/docker-compose.prod.yml start api worker
# 4. hitung ulang & sinkron
arc ctl recompute && arc ctl odoo sync
```
Data sejak backup terakhir: Odoo disinkron ulang otomatis; chat WhatsApp diisi ulang dari riwayat (backfill 30 hari)
saat nomor dipasangkan ulang. Keputusan yang dibuat di antara backup dan insiden hilang — catat di audit manual.

## 7. Rollback migrasi
Setiap migrasi punya blok `-- +goose Down`. `arc` belum mengekspos `down`; gunakan goose CLI pada database yang sama:
`goose -dir db/migrations postgres "$DATABASE_URL" down` **setelah backup**. Migrasi 0011 (partisi) bisa di-down: data
dipindah kembali ke tabel biasa. Lebih aman: restore backup sebelum upgrade (§6) lalu jalankan versi lama.

## 8. Permintaan subjek data (UU PDP)
- Akses: `arc ctl pdp export --dealer <slug> --out dealer.json` → kirim lewat kanal aman.
- Hapus orang: `arc ctl pdp delete --contact 62812… --by sam@gsi.co.id --yes` (kontak, percakapan, sinyal percakapan,
  identifikasi; order/invoice/metrik tetap; audit menyimpan hash nomor saja).

## 9. Ponsel 2FA hilang
`arc ctl user totp-reset --email sam@gsi.co.id`, login, lalu aktifkan lagi di Pengaturan → Keamanan akun.
