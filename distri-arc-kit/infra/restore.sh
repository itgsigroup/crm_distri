#!/usr/bin/env bash
# Restore an encrypted backup into a database (RUNBOOK §6). The target is emptied first (--clean).
#   infra/restore.sh <backup file> [target DATABASE_URL]   (default target: $DATABASE_URL)
# Needs the same BACKUP_PASSPHRASE / age identity (BACKUP_AGE_IDENTITY file) that made the backup.
set -euo pipefail

file="${1:?usage: infra/restore.sh <backup file> [target database url]}"
target="${2:-${DATABASE_URL:?DATABASE_URL or a target url is required}}"
PG_BIN="${PG_BIN:-}"
restore="${PG_BIN:+$PG_BIN/}pg_restore"

if [[ -f "$file.sha256" ]]; then
  (cd "$(dirname "$file")" && (sha256sum -c "$(basename "$file").sha256" 2>/dev/null || shasum -a 256 -c "$(basename "$file").sha256")) >/dev/null \
    || { echo "restore: checksum mismatch for $file" >&2; exit 3; }
fi

decrypt() {
  case "$file" in
    *.age) age -d -i "${BACKUP_AGE_IDENTITY:?BACKUP_AGE_IDENTITY (age key file) is required}" "$file" ;;
    *.enc) "${OPENSSL:-openssl}" enc -d -aes-256-cbc -pbkdf2 -iter 200000 -pass env:BACKUP_PASSPHRASE -in "$file" ;;
    *) cat "$file" ;;
  esac
}

decrypt | "$restore" --clean --if-exists --no-owner --no-privileges --exit-on-error -d "$target"
echo "restored $file → $target"
