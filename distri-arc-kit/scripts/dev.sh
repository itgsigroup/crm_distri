#!/usr/bin/env bash
# make dev: postgres (Compose when Docker is available, else the native server in DATABASE_URL) → migrate →
# seed when empty → api + worker + web. Ctrl-C stops everything.
set -euo pipefail
cd "$(dirname "$0")/.."
if [ -f .env ]; then set -a; . ./.env; set +a; fi

if command -v docker >/dev/null 2>&1 && [ "${DEV_DB:-compose}" = "compose" ]; then
  docker compose -f infra/docker-compose.yml up -d postgres
fi
for i in $(seq 1 30); do
  if go run ./cmd/arc ctl counts >/dev/null 2>&1 || go run ./cmd/arc ctl migrate >/dev/null 2>&1; then break; fi
  echo "waiting for postgres ($DATABASE_URL)…"; sleep 1
done
go run ./cmd/arc ctl migrate
go run ./cmd/arc ctl seed --if-empty
go build -o bin/arc ./cmd/arc
bin/arc ctl agents run --all --if-empty   # first proposals for the Keputusan queue (Orchestrator from stage 06)

pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; wait 2>/dev/null || true; }
trap cleanup EXIT INT TERM
bin/arc api    2>&1 | sed -u 's/^/[api]    /' & pids+=($!)
bin/arc worker 2>&1 | sed -u 's/^/[worker] /' & pids+=($!)
(cd web && npm run dev -- --host 127.0.0.1) 2>&1 | sed -u 's/^/[web]    /' & pids+=($!)
wait
