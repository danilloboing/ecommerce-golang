.PHONY: help dev build test test-unit test-integration lint fmt tidy migrate sqlc-gen docker-up docker-down clean

GOFLAGS := -trimpath
LDFLAGS := -s -w

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## Start docker dependencies and run API with live reload
	docker compose -f deployments/docker-compose.yml up -d
	go run ./cmd/api

build: ## Build api binary
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/api ./cmd/api

test: test-unit ## Run unit tests

test-unit: ## Run unit tests only
	go test -race -count=1 -coverprofile=cover.out ./...

test-integration: ## Run integration tests (requires Docker)
	go test -race -count=1 -tags=integration -timeout=10m ./...

lint: ## Run linters
	golangci-lint run ./...

fmt: ## Format code
	gofumpt -w .
	goimports -w -local github.com/danilloboing/marketplace-golang .

tidy: ## Tidy go modules
	go mod tidy

migrate: ## Apply migrations (requires Atlas CLI installed)
	atlas migrate apply --env local

sqlc-gen: ## Regenerate sqlc code
	sqlc generate -f db/sqlc.yaml

docker-up: ## Start docker dependencies
	docker compose -f deployments/docker-compose.yml up -d

docker-down: ## Stop docker dependencies
	docker compose -f deployments/docker-compose.yml down

clean: ## Remove build artifacts
	rm -rf bin cover.out
