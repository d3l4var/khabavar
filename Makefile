# KHABAVAR — Makefile (single binary, KH_ROLE dispatch)
GO ?= go
MIGRATE_BIN ?= migrate
PG_URL ?= postgres://app_runtime:app_runtime@localhost:5432/khabavar?sslmode=disable

.PHONY: help build test test-race cover lint migrate-up migrate-down seed run-api run-identity run-worker fmt vet tidy web-build

help:
	@echo "KHABAVAR targets: build test lint migrate-up seed run-api run-identity run-worker web-build"

build: web-build
	$(GO) build -o bin/khabavar ./cmd/khabavar

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-pg:
	KH_TEST_PG_DSN="$(PG_URL)" $(GO) test -count=1 -run TestPG ./internal/modules/identity ./internal/modules/worker

cover:
	$(GO) test -coverprofile=cover.out ./... && $(GO) tool cover -func=cover.out | tail -1

lint:
	$(GO) vet ./...
	@golangci-lint run ./...

migrate-up:
	$(MIGRATE_BIN) -path migrations -database "$(PG_URL)" up

migrate-down:
	@echo "forward-only — فقط با emergency_rollback_migration (CI §۴)"
	$(MIGRATE_BIN) -path migrations -database "$(PG_URL)" down 1

seed:
	PGPASSWORD=app_runtime psql "$(PG_URL)" -f seed/001_seed_baseline.sql

run-api:	build && KH_ROLE=api        ./bin/khabavar
run-identity:	build && KH_ROLE=identity   ./bin/khabavar
run-worker:	build && KH_ROLE=worker     ./bin/khabavar

fmt:
	$(GO) fmt ./...
vet:
	$(GO) vet ./...
tidy:
	$(GO) mod tidy

web-build:
	@for app in main shop magazine admin; do \
		( cd web/$$app && bun install && bun run build ) ; \
	done
