# Deploy — VPS Ubuntu 24.04 (single-tenant)

Ukuran: **2 vCPU · 4 GB RAM · 40 GB SSD** cukup untuk ≤ 50 pengguna (Postgres ± 1 GB, api + worker ± 300 MB).
Satu domain, mis. `distri.gsi.co.id`, A record ke IP VPS. Port publik hanya 80/443 (Caddy, TLS otomatis).

## A. Docker Compose (disarankan)
```bash
# 1. Server
sudo apt update && sudo apt install -y docker.io docker-compose-v2 ufw
sudo ufw allow OpenSSH && sudo ufw allow 80,443/tcp && sudo ufw allow 443/udp && sudo ufw enable
sudo adduser --disabled-password arc && sudo usermod -aG docker arc

# 2. Kode + konfigurasi (sebagai arc)
git clone <repo> /opt/distri-arc && cd /opt/distri-arc/distri-arc-kit
cp .env.example .env && chmod 600 .env
#   APP_ENV=prod, DOMAIN=distri.gsi.co.id, PUBLIC_URL=https://distri.gsi.co.id,
#   POSTGRES_PASSWORD=$(openssl rand -hex 24), SESSION_SECRET=$(openssl rand -hex 32), METRICS_TOKEN=$(openssl rand -hex 24),
#   BACKUP_PASSPHRASE=$(openssl rand -hex 32)  ← simpan juga di password manager GSI (tanpa ini backup tidak bisa dibuka)
#   WA_TRANSPORT=whatsmeow, WA_SEND_HOURS=08-18, ODOO_MODE=rpc + ODOO_*, ODOO_WRITE=false (true setelah uji di Odoo test),
#   LLM_PROVIDER=anthropic + ANTHROPIC_API_KEY (+ OPENAI_API_KEY cadangan), ALERT_WA_GROUP=<jid grup internal>
#   kosongkan ARC_NOW dan ARC_DEMO_PASSWORD

# 3. Jalankan
docker compose -f infra/docker-compose.prod.yml --env-file .env up -d --build
docker compose -f infra/docker-compose.prod.yml run --rm api ctl check-env        # harus tanpa FAIL
docker compose -f infra/docker-compose.prod.yml run --rm api ctl seed --policies-only
docker compose -f infra/docker-compose.prod.yml run --rm api ctl user add --email sam@gsi.co.id --name "Sam Setiadi" --role ceo --password '…'
docker compose -f infra/docker-compose.prod.yml run --rm api ctl odoo sync --full
```
Lalu login di `https://distri.gsi.co.id`, aktifkan **2FA** (Pengaturan → Keamanan akun), pasangkan nomor WhatsApp
sales (Pengaturan → WhatsApp → QR), dan tandai grup gudang sebagai internal.

Pembaruan: `git pull && docker compose -f infra/docker-compose.prod.yml up -d --build` (migrasi jalan otomatis lewat
service `migrate` sebelum api/worker).

## B. Tanpa Docker (systemd)
```bash
sudo apt install -y postgresql-16 caddy openssl
sudo -u postgres createuser arc && sudo -u postgres createdb -O arc distri_arc
sudo useradd --system --home /var/lib/distri-arc --create-home arc
make build && sudo install -D bin/arc /opt/distri-arc/bin/arc && sudo cp -r web/dist /opt/distri-arc/web && sudo cp -r infra /opt/distri-arc/
sudo install -D -m 600 -o arc .env /etc/distri-arc/env            # DATABASE_URL=postgres:///distri_arc?host=/var/run/postgresql
sudo cp infra/systemd/*.service infra/systemd/*.timer /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now distri-arc-api distri-arc-worker distri-arc-backup.timer
# Caddy: DOMAIN, API_UPSTREAM=127.0.0.1:8080, WEB_ROOT=/opt/distri-arc/web di /etc/default/caddy; Caddyfile = infra/Caddyfile
```

## Yang diperiksa setelah deploy (acceptance Stage 13)
1. `curl -I https://distri.gsi.co.id` → `strict-transport-security`, `content-security-policy`, `x-content-type-options`.
2. Login CEO dengan 2FA aktif meminta kode 6 digit.
3. `docker compose … logs worker --since 24h | grep -c '"level":"ERROR"'` → 0; Pengaturan → Status sistem: siklus per jam
   06.00–20.00 berjalan, antrean normal, WhatsApp terhubung.
4. Backup: `ls /var/lib/docker/volumes/distri-arc_backups/_data` berisi `distri-arc-*.dump.enc`; uji pulih bulanan
   (RUNBOOK §6). CI menjalankan `restore-test` tiap Senin.
5. Monitoring opsional: Prometheus men-scrape `http://api:8080/metrics` dengan `Authorization: Bearer $METRICS_TOKEN`
   (Caddy menolak `/metrics` dari luar).
