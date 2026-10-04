#!/usr/bin/env bash
# Restore a dump made by backup.sh into DATABASE_URL (or RESTORE_DATABASE_URL).
# The target database must exist and should be empty (e.g. a fresh machine).
# PG_BIN may point at the client binaries matching the server version.
set -euo pipefail
cd "$(dirname "$0")/.."
file="${1:?usage: scripts/restore.sh backups/arc-YYYYmmdd-HHMMSS.dump}"
[ -s "$file" ] || { echo "restore: $file is missing or empty" >&2; exit 1; }
[ -f .env ] && set -a && . ./.env && set +a
target="${RESTORE_DATABASE_URL:-${DATABASE_URL:?DATABASE_URL not set}}"
"${PG_BIN:+$PG_BIN/}pg_restore" --no-owner --clean --if-exists --exit-on-error --dbname "$target" "$file"
echo "restored $file → $target"
