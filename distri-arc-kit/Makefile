# Distri ARC Orbit — developer commands. Requires Go 1.26+, Node 20+, PostgreSQL 16+ (Docker optional).
SHELL := /bin/bash
GO    ?= go
WEB   := web
# Export .env so make targets see DATABASE_URL / DATABASE_URL_TEST.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: help dev api worker web setup check test test-go test-web lint typecheck migrate seed reset sqlc seedgen build e2e db-up

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-10s %s\n", $$1, $$2}'

dev: ## postgres → migrate → seed (if empty) → api :8080 + worker + web :5173
	@scripts/dev.sh

db-up: ## Start postgres in Docker Compose (skip when using a native PostgreSQL)
	docker compose -f infra/docker-compose.yml up -d postgres

api: ## Run only the API
	$(GO) run ./cmd/arc api

worker: ## Run only the worker
	$(GO) run ./cmd/arc worker

web: ## Run only the web dev server
	cd $(WEB) && npm run dev

setup: ## Install web dependencies
	cd $(WEB) && npm ci

check: lint test typecheck ## go vet + golangci-lint + go test + web typecheck + web test

test: test-go test-web ## All unit + integration tests

test-go: ## Go tests (integration tests use DATABASE_URL_TEST)
	$(GO) test ./...

test-web:
	cd $(WEB) && npm test

lint: ## go vet + golangci-lint + oxlint
	$(GO) vet ./...
	golangci-lint run ./...
	cd $(WEB) && npm run lint

typecheck:
	cd $(WEB) && npm run typecheck

migrate: ## Apply migrations (goose)
	$(GO) run ./cmd/arc ctl migrate

seed: ## Load db/seed (idempotent)
	$(GO) run ./cmd/arc ctl seed

reset: ## Drop everything, migrate, seed (dev only)
	$(GO) run ./cmd/arc ctl reset

sqlc: ## Regenerate internal/store/gen from db/queries
	sqlc generate

seedgen: ## Regenerate db/seed from the mockup sample data
	node tools/seedgen/extract_mockup.mjs
	$(GO) run ./tools/seedgen

build: ## bin/arc + web/dist
	$(GO) build -o bin/arc ./cmd/arc
	cd $(WEB) && npm run build

e2e: ## Playwright (needs `make dev` running)
	cd $(WEB) && npm run e2e
