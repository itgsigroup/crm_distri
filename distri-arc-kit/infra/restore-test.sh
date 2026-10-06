#!/usr/bin/env bash
# Restore test (stage 13 acceptance): a database restored from an encrypted backup gives identical metrics.
#   1. migrate + seed + recompute the source DB, fingerprint the metrics (arc ctl fingerprint)
#   2. backup.sh → restore.sh into a fresh database
#   3. recompute in the restored DB with the same clock → the fingerprint must match
# Uses DATABASE_URL_TEST's server; creates and drops two scratch databases. Run: make restore-test
set -euo pipefail
cd "$(dirname "$0")/.."

base="${DATABASE_URL_TEST:?DATABASE_URL_TEST is required}"
PG_BIN="${PG_BIN:-}"
psql="${PG_BIN:+$PG_BIN/}psql"
arc="${ARC_BIN:-bin/arc}"
export APP_ENV=dev ARC_NOW="${ARC_NOW:-2026-10-05T06:45:00+07:00}" BACKUP_PASSPHRASE="${BACKUP_PASSPHRASE:-restore-test-$$}"
src_db="arc_restore_src_$$"
dst_db="arc_restore_dst_$$"
url_for() { echo "$base" | sed -E "s#/[^/?]+(\?|$)#/$1\1#"; }
admin="$(url_for postgres)"
tmp="$(mktemp -d)"
cleanup() {
  "$psql" "$admin" -qc "drop database if exists $src_db" -c "drop database if exists $dst_db" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

"$psql" "$admin" -qc "create database $src_db" -c "create database $dst_db"
src="$(url_for "$src_db")"
dst="$(url_for "$dst_db")"

DATABASE_URL="$src" "$arc" ctl migrate >/dev/null
DATABASE_URL="$src" "$arc" ctl seed >/dev/null
DATABASE_URL="$src" "$arc" ctl recompute >/dev/null
before="$(DATABASE_URL="$src" "$arc" ctl fingerprint)"

file="$(DATABASE_URL="$src" BACKUP_DIR="$tmp" infra/backup.sh)"
[[ "$file" == *.enc ]] || { echo "backup not encrypted: $file" >&2; exit 1; }
if head -c 5 "$file" | grep -q PGDMP; then echo "backup is plain pg_dump" >&2; exit 1; fi

infra/restore.sh "$file" "$dst" >/dev/null
DATABASE_URL="$dst" "$arc" ctl recompute >/dev/null
after="$(DATABASE_URL="$dst" "$arc" ctl fingerprint)"
counts_src="$(DATABASE_URL="$src" "$arc" ctl counts)"
counts_dst="$(DATABASE_URL="$dst" "$arc" ctl counts)"

echo "metrics before restore: $before"
echo "metrics after restore:  $after"
[[ "$before" == "$after" ]] || { echo "FAIL: metrics differ after restore" >&2; exit 1; }
[[ "$counts_src" == "$counts_dst" ]] || { echo "FAIL: row counts differ: $counts_src vs $counts_dst" >&2; exit 1; }
echo "OK: restore gives identical metrics and row counts ($counts_dst)"
