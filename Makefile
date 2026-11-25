SHELL := /bin/zsh
.PHONY: setup dev agent test test-go test-web lint build seed-demo stop backup-sqlite test-restore-sqlite clean

GO := $(shell command -v go 2>/dev/null || echo "$(HOME)/.local/go/bin/go")

setup:
	@echo "Checking required tools..."
	@command -v $(GO) >/dev/null || (echo "Go is required" && exit 1)
	@command -v node >/dev/null || (echo "Node.js is required" && exit 1)
	@command -v pnpm >/dev/null || (echo "pnpm is required" && exit 1)
	$(GO) mod tidy
	pnpm install
	@mkdir -p data .infraos/logs .infraos/pids backups

dev:
	./scripts/dev.sh

agent:
	./scripts/run-agent.sh

test: test-go test-web

test-go:
	$(GO) test -race -count=1 ./...

test-web:
	pnpm --filter @infraos/web test

lint:
	@test -z "$$($(GO)fmt -l cmd internal pkg)" || (echo "Run gofmt on Go files" && $(GO)fmt -l cmd internal pkg && exit 1)
	$(GO) vet ./...
	pnpm --filter @infraos/web lint
	pnpm --filter @infraos/web typecheck

build:
	@mkdir -p bin
	$(GO) build -o bin/infraos-api ./cmd/api
	$(GO) build -o bin/infraos-agent ./cmd/agent
	$(GO) build -o bin/infraos-worker ./cmd/worker
	pnpm --filter @infraos/web build

seed-demo:
	$(GO) run ./cmd/api --seed-demo-only

stop:
	./scripts/stop.sh

backup-sqlite:
	./scripts/backup-sqlite.sh

test-restore-sqlite:
	./scripts/test-restore-sqlite.sh

clean:
	rm -rf bin coverage.out apps/web/.next
