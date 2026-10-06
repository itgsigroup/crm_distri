#!/usr/bin/env bash
# Daily encrypted backup of the Distri ARC database (docs/DEPLOY.md, systemd timer 02:00 WIB).
#   pg_dump (custom format) → encrypted with age (BACKUP_AGE_RECIPIENT) or openssl AES-256 (BACKUP_PASSPHRASE)
#   → $BACKUP_DIR/distri-arc-YYYYmmdd-HHMMSS.dump.{age|enc}; files older than BACKUP_KEEP_DAYS (30) are removed.
# Optional BACKUP_RCLONE_REMOTE (e.g. "b2:gsi-backup/distri-arc") copies the file off the host.
set -euo pipefail

DATABASE_URL="${DATABASE_URL:?DATABASE_URL is required}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/distri-arc}"
KEEP="${BACKUP_KEEP_DAYS:-30}"
PG_BIN="${PG_BIN:-}"
dump="${PG_BIN:+$PG_BIN/}pg_dump"

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
stamp="$(date -u +%Y%m%d-%H%M%S)"
base="$BACKUP_DIR/distri-arc-$stamp.dump"

if [[ -n "${BACKUP_AGE_RECIPIENT:-}" ]]; then
  out="$base.age"
  "$dump" --format=custom --no-owner --no-privileges "$DATABASE_URL" | age -r "$BACKUP_AGE_RECIPIENT" -o "$out.part"
elif [[ -n "${BACKUP_PASSPHRASE:-}" ]]; then
  out="$base.enc"
  "$dump" --format=custom --no-owner --no-privileges "$DATABASE_URL" \
    | "${OPENSSL:-openssl}" enc -aes-256-cbc -pbkdf2 -iter 200000 -salt -pass env:BACKUP_PASSPHRASE -out "$out.part"
else
  echo "backup: set BACKUP_AGE_RECIPIENT or BACKUP_PASSPHRASE — unencrypted backups are not allowed" >&2
  exit 2
fi
mv "$out.part" "$out"
chmod 600 "$out"
sha256sum "$out" 2>/dev/null > "$out.sha256" || shasum -a 256 "$out" > "$out.sha256"

if [[ -n "${BACKUP_RCLONE_REMOTE:-}" ]]; then
  rclone copy "$out" "$BACKUP_RCLONE_REMOTE" && rclone copy "$out.sha256" "$BACKUP_RCLONE_REMOTE"
fi

find "$BACKUP_DIR" -maxdepth 1 -name 'distri-arc-*.dump.*' -type f -mtime +"$KEEP" -delete
echo "$out"
