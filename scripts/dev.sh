#!/usr/bin/env bash
# Starts API, wa-bridge and the Vite dev server; Ctrl-C stops all three.
set -euo pipefail
cd "$(dirname "$0")/.."
# Share .env with the bridge (the API also reads it itself).
if [ -f .env ]; then set -a; . ./.env; set +a; fi
pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; wait 2>/dev/null || true; }
trap cleanup EXIT INT TERM

go run ./apps/api/cmd/arc serve 2>&1 | sed -u 's/^/[api]    /' & pids+=($!)
(cd apps/wa-bridge && npm run dev) 2>&1 | sed -u 's/^/[bridge] /' & pids+=($!)
(cd apps/web && npm run dev -- --host 127.0.0.1) 2>&1 | sed -u 's/^/[web]    /' & pids+=($!)
wait
