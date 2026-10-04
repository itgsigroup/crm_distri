# ARC — developer commands. Requires Go ≥ 1.26, Node ≥ 20, PostgreSQL ≥ 15.
SHELL := /bin/bash
GO    ?= go
NPM   ?= npm
WEB   := apps/web
BRIDGE := apps/wa-bridge

.PHONY: help dev setup api bridge web build test test-go test-bridge test-web e2e perf lint fmt seed reset db-upgrade eval brief mcp-inspect backup restore compose-up compose-config

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

setup: ## Install Node dependencies (web + bridge)
	cd $(WEB) && $(NPM) ci
	cd $(BRIDGE) && $(NPM) ci

dev: ## API :8000 + wa-bridge :3001 + web :5173 (Ctrl-C stops all)
	@scripts/dev.sh

api: ## Run only the API
	$(GO) run ./apps/api/cmd/arc serve

bridge: ## Run only the WhatsApp bridge (Baileys)
	cd $(BRIDGE) && $(NPM) run dev

web: ## Run only the web dev server
	cd $(WEB) && $(NPM) run dev

build: ## Build bin/arc, the bridge (apps/wa-bridge/dist) and the web bundle
	$(GO) build -o bin/arc ./apps/api/cmd/arc
	cd $(BRIDGE) && $(NPM) run build
	cd $(WEB) && $(NPM) run build

test: test-go test-bridge test-web ## All unit + integration tests

test-go: ## Backend tests (uses ARC_TEST_DATABASE_URL, default arc_test)
	$(GO) test ./...

test-bridge:
	cd $(BRIDGE) && $(NPM) test

test-web:
	cd $(WEB) && npx vitest run

e2e: ## Playwright smoke (needs `make dev` running and a seeded DB)
	cd $(WEB) && npx playwright test

perf: ## Load check: 500 accounts, 10k interactions, p95 < 300 ms (uses ARC_PERF_DATABASE_URL, default arc_perf)
	$(GO) run ./apps/api/cmd/arcperf

lint: ## go vet + gofmt check + tsc (bridge) + oxlint + tsc (web)
	$(GO) vet ./...
	cd $(BRIDGE) && $(NPM) run lint
	@out=$$(gofmt -l apps packages); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	cd $(WEB) && npx oxlint --deny-warnings src && npx tsc -b

fmt: ## Format Go code
	gofmt -w apps packages

db-upgrade: ## Apply embedded SQL migrations
	$(GO) run ./apps/api/cmd/arc migrate

seed: ## Load tests/fixtures (idempotent)
	$(GO) run ./apps/api/cmd/arc seed

reset: ## Drop all data (refused in production) then reseed
	$(GO) run ./apps/api/cmd/arc reset
	$(GO) run ./apps/api/cmd/arc seed

eval: ## Capture evaluation (writes docs/eval/capture-<provider>.md)
	$(GO) run ./apps/api/cmd/arc eval

brief: ## Generate today's brief now (no send)
	$(GO) run ./apps/api/cmd/arc brief

mcp-inspect: ## Open MCP Inspector against the local server
	npx @modelcontextprotocol/inspector --transport http --server-url http://localhost:8000/mcp

backup: ## pg_dump to backups/arc-<timestamp>.dump
	@scripts/backup.sh

restore: ## Restore a dump: make restore FILE=backups/arc-....dump
	@scripts/restore.sh "$(FILE)"

compose-config: ## Validate docker compose
	docker compose -f infra/docker-compose.yml --env-file .env config -q

compose-up: ## Production-like stack (postgres, api, bridge, caddy)
	docker compose -f infra/docker-compose.yml --env-file .env up -d --build
