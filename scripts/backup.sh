#!/usr/bin/env bash
# Nightly backup: custom-format pg_dump of the ARC database (includes schema wa_bridge).
# Keeps the newest 14 dumps. Cron example: 30 1 * * * cd /opt/arc && scripts/backup.sh
# PG_BIN may point at the client binaries matching the server version (e.g. /usr/lib/postgresql/17/bin).
set -euo pipefail
cd "$(dirname "$0")/.."
[ -f .env ] && set -a && . ./.env && set +a
PG_DUMP="${PG_BIN:+$PG_BIN/}pg_dump"
mkdir -p backups
out="backups/arc-$(date +%Y%m%d-%H%M%S).dump"
tmp="$out.partial"
trap 'rm -f "$tmp"' EXIT
"$PG_DUMP" --format=custom --no-owner --file="$tmp" "${DATABASE_URL:?DATABASE_URL not set}"
mv "$tmp" "$out"
ls -1t backups/arc-*.dump | tail -n +15 | while read -r old; do rm -f -- "$old"; done
echo "backup: $out ($(du -h "$out" | cut -f1))"
