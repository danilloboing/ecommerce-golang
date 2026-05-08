# Phase 1a — Bootstrap Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bootstrap the marketplace-golang backend with a running HTTP server, observability instrumentation (logs, metrics, traces, errors), Postgres + Redis connections, and migration/query tooling. Output is a working `cmd/api` that responds to `/health`, `/ready`, `/metrics` and shuts down gracefully — ready to host domain modules in Plan 1b.

**Architecture:** Go monolito modular (DDD-light layout per spec). All infrastructure lives in `internal/platform/`, cross-cutting concerns in `internal/core/`, configuration in `internal/config/`, entry points in `cmd/`. Vendor-neutral observability — instrumentation stays in code, backends configured via env. 12-factor compliance throughout.

**Tech Stack:** Go 1.23+, `go-chi/chi` v5, `jackc/pgx/v5`, `redis/go-redis/v9`, `prometheus/client_golang`, `go.opentelemetry.io/otel`, `getsentry/sentry-go`, `caarlos0/env/v11`, `google/uuid`, `stretchr/testify`, `testcontainers/testcontainers-go`, Atlas (HCL declarative), sqlc, `log/slog` (stdlib), `unrolled/secure`, `go-resty/resty/v2`.

**Reference:** Design spec at `docs/superpowers/specs/2026-05-08-marketplace-golang-design.md`.

---

## File Structure (created by this plan)

```
marketplace-golang/
├── cmd/
│   └── api/
│       └── main.go                              # HTTP server entry point
├── internal/
│   ├── config/
│   │   ├── config.go                            # env-based config struct
│   │   └── config_test.go
│   ├── core/
│   │   ├── httpx/
│   │   │   ├── middleware.go                    # logger, recover, cors, security headers
│   │   │   ├── middleware_test.go
│   │   │   ├── server.go                        # http.Server wrapper + graceful shutdown
│   │   │   └── server_test.go
│   │   ├── observability/
│   │   │   ├── logger.go                        # slog setup
│   │   │   ├── logger_test.go
│   │   │   ├── metrics.go                       # Prometheus registry + handler
│   │   │   ├── metrics_test.go
│   │   │   ├── tracing.go                       # OpenTelemetry SDK setup
│   │   │   ├── tracing_test.go
│   │   │   ├── sentry.go                        # Sentry init + scrubber
│   │   │   └── sentry_test.go
│   │   └── health/
│   │       ├── handler.go                       # /health and /ready handlers
│   │       └── handler_test.go
│   ├── platform/
│   │   ├── postgres/
│   │   │   ├── pool.go                          # pgx pool factory + ping
│   │   │   └── pool_test.go
│   │   └── redis/
│   │       ├── client.go                        # redis client factory + ping
│   │       └── client_test.go
│   └── testutil/
│       ├── postgres.go                          # NewTestPostgres (testcontainers)
│       └── redis.go                             # NewTestRedis (testcontainers)
├── db/
│   ├── migrations/
│   │   └── 20260508000001_init.sql              # extensions: pg_trgm, citext
│   └── sqlc.yaml                                # sqlc config (skeleton)
├── deployments/
│   ├── Dockerfile                               # multi-stage build
│   └── docker-compose.yml                       # postgres + redis for dev
├── api/
│   └── openapi.yaml                             # OpenAPI 3.1 skeleton (only health endpoints)
├── .editorconfig
├── .golangci.yml
├── .env.example
├── atlas.hcl
├── Makefile
├── go.mod
├── go.sum
├── README.md
└── (already exists) .gitignore, docs/
```

Each file has one clear responsibility. `internal/core/observability/` holds the four signals as separate files (logs, metrics, traces, errors) so each can be reasoned about independently.

---

## Task 1: Initialize Go module and project skeleton

**Files:**
- Create: `go.mod`
- Create: `.golangci.yml`
- Create: `.editorconfig`
- Create: `.env.example`

**Skills to consult:**
- `cc-skills-golang:golang-project-layout` — confirms cmd/internal layout
- `cc-skills-golang:golang-lint` — golangci-lint config
- `cc-skills-golang:golang-naming` — module path and package naming rules
- `cc-skills-golang:golang-modernize` — Go version selection

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/danilloboing/Documents/danillo/projects/marketplace-golang
go mod init github.com/danilloboing/marketplace-golang
```

Verify: `cat go.mod` shows `module github.com/danilloboing/marketplace-golang` and `go 1.23` (or current).

- [ ] **Step 2: Create `.editorconfig`**

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = tab
indent_size = 4

[*.{yml,yaml,json,md}]
indent_style = space
indent_size = 2

[Makefile]
indent_style = tab
```

- [ ] **Step 3: Create `.golangci.yml`**

```yaml
run:
  timeout: 5m
  go: "1.23"
linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gocyclo
    - gosec
    - goconst
    - gofumpt
    - goimports
    - revive
    - sqlclosecheck
    - nilerr
    - errorlint
    - bodyclose
    - noctx
    - prealloc
linters-settings:
  gocyclo:
    min-complexity: 15
  goimports:
    local-prefixes: github.com/danilloboing/marketplace-golang
  revive:
    rules:
      - name: exported
        severity: warning
      - name: package-comments
        severity: warning
issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec
        - errcheck
```

- [ ] **Step 4: Create `.env.example`**

```bash
# Server
APP_ENV=development
APP_PORT=8080
APP_LOG_LEVEL=info
APP_SHUTDOWN_TIMEOUT=30s

# Database
DATABASE_URL=postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=5
DATABASE_CONN_MAX_LIFETIME=30m

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# Observability (optional in development)
SENTRY_DSN=
OTEL_EXPORTER_OTLP_ENDPOINT=
OTEL_SERVICE_NAME=marketplace-golang-api
OTEL_TRACES_SAMPLER_RATIO=0.1

# Admin bootstrap (Phase 1 only — replaced in Phase 2)
ADMIN_API_TOKEN=change-me-in-production

# CORS
CORS_ALLOWED_ORIGINS=http://localhost:3000
```

- [ ] **Step 5: Verify formatting and commit**

Run:
```bash
gofmt -s -w .
git add go.mod .golangci.yml .editorconfig .env.example
git commit -m "chore: initialize go module and editor configs"
```

Expected: clean commit, no errors.

---

## Task 2: Add Makefile and Dockerfile + docker-compose

**Files:**
- Create: `Makefile`
- Create: `deployments/Dockerfile`
- Create: `deployments/docker-compose.yml`

**Skills to consult:**
- `cc-skills-golang:golang-project-layout` — Makefile patterns
- `cc-skills-golang:golang-continuous-integration` — build commands
- `cc-skills-golang:golang-modernize` — Docker base images for Go 1.23+

- [ ] **Step 1: Create `Makefile`**

```makefile
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
```

- [ ] **Step 2: Create `deployments/Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1.7

FROM golang:1.23-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGET=api
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/app ./cmd/${TARGET}

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /out/app /app/app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/app"]
```

- [ ] **Step 3: Create `deployments/docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: marketplace
      POSTGRES_USER: marketplace
      POSTGRES_PASSWORD: marketplace
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U marketplace"]
      interval: 5s
      timeout: 5s
      retries: 5
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    command: ["redis-server", "--appendonly", "yes"]
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

- [ ] **Step 4: Verify Docker compose comes up**

Run:
```bash
docker compose -f deployments/docker-compose.yml up -d
docker compose -f deployments/docker-compose.yml ps
```

Expected: both `postgres` and `redis` services healthy.

Tear down: `docker compose -f deployments/docker-compose.yml down`

- [ ] **Step 5: Commit**

```bash
git add Makefile deployments/
git commit -m "chore: add Makefile, Dockerfile, and docker-compose for dev"
```

---

## Task 3: Implement config struct with env parsing

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-naming` — config naming conventions
- `cc-skills-golang:golang-error-handling` — error wrapping
- `cc-skills-golang:golang-code-style` — struct field tags
- `cc-skills-golang:golang-testing` — table-driven tests
- `cc-skills-golang:golang-stretchr-testify` — assertions

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/caarlos0/env/v11
go get github.com/stretchr/testify
go get github.com/joho/godotenv
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ParsesAllFieldsFromEnv(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_LOG_LEVEL", "debug")
	t.Setenv("APP_SHUTDOWN_TIMEOUT", "15s")
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db?sslmode=disable")
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "30")
	t.Setenv("DATABASE_MAX_IDLE_CONNS", "10")
	t.Setenv("DATABASE_CONN_MAX_LIFETIME", "1h")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("ADMIN_API_TOKEN", "abc123")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://a.com,http://b.com")
	t.Setenv("OTEL_SERVICE_NAME", "test-svc")
	t.Setenv("OTEL_TRACES_SAMPLER_RATIO", "0.5")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "test", cfg.App.Env)
	assert.Equal(t, 9090, cfg.App.Port)
	assert.Equal(t, "debug", cfg.App.LogLevel)
	assert.Equal(t, 15*time.Second, cfg.App.ShutdownTimeout)
	assert.Equal(t, "postgres://u:p@h:5432/db?sslmode=disable", cfg.Database.URL)
	assert.Equal(t, 30, cfg.Database.MaxOpenConns)
	assert.Equal(t, 10, cfg.Database.MaxIdleConns)
	assert.Equal(t, time.Hour, cfg.Database.ConnMaxLifetime)
	assert.Equal(t, "redis:6379", cfg.Redis.Addr)
	assert.Equal(t, "secret", cfg.Redis.Password)
	assert.Equal(t, 2, cfg.Redis.DB)
	assert.Equal(t, "abc123", cfg.Admin.APIToken)
	assert.Equal(t, []string{"http://a.com", "http://b.com"}, cfg.CORS.AllowedOrigins)
	assert.Equal(t, "test-svc", cfg.Observability.OTELServiceName)
	assert.InDelta(t, 0.5, cfg.Observability.OTELTracesSamplerRatio, 0.0001)
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_API_TOKEN", "x")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_RequiresAdminAPIToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_API_TOKEN")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "x")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "development", cfg.App.Env)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Equal(t, "info", cfg.App.LogLevel)
	assert.Equal(t, 30*time.Second, cfg.App.ShutdownTimeout)
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/config/... -v
```

Expected: build error (`config.Load undefined`).

- [ ] **Step 4: Implement `internal/config/config.go`**

```go
// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config aggregates all runtime configuration sections.
type Config struct {
	App           App
	Database      Database
	Redis         Redis
	Admin         Admin
	CORS          CORS
	Observability Observability
}

// App holds general application settings.
type App struct {
	Env             string        `env:"APP_ENV" envDefault:"development"`
	Port            int           `env:"APP_PORT" envDefault:"8080"`
	LogLevel        string        `env:"APP_LOG_LEVEL" envDefault:"info"`
	ShutdownTimeout time.Duration `env:"APP_SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// Database holds Postgres connection settings.
type Database struct {
	URL             string        `env:"DATABASE_URL,required,notEmpty"`
	MaxOpenConns    int           `env:"DATABASE_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"DATABASE_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"DATABASE_CONN_MAX_LIFETIME" envDefault:"30m"`
}

// Redis holds Redis connection settings.
type Redis struct {
	Addr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`
}

// Admin holds bootstrap admin auth (Phase 1 only).
type Admin struct {
	APIToken string `env:"ADMIN_API_TOKEN,required,notEmpty"`
}

// CORS holds allowed origins.
type CORS struct {
	AllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
}

// Observability holds tracing and error reporting endpoints.
type Observability struct {
	SentryDSN              string  `env:"SENTRY_DSN"`
	OTELExporterEndpoint   string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELServiceName        string  `env:"OTEL_SERVICE_NAME" envDefault:"marketplace-golang-api"`
	OTELTracesSamplerRatio float64 `env:"OTEL_TRACES_SAMPLER_RATIO" envDefault:"0.1"`
}

// Load parses configuration from environment variables.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse env: %w", err)
	}
	return cfg, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/config/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): add env-based configuration with required validation"
```

---

## Task 4: Implement structured logger (slog)

**Files:**
- Create: `internal/core/observability/logger.go`
- Create: `internal/core/observability/logger_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-observability` — slog patterns, levels, attributes
- `cc-skills-golang:golang-naming` — logger naming
- `cc-skills-golang:golang-testing` — capturing slog output for assertions

- [ ] **Step 1: Write the failing test**

Create `internal/core/observability/logger_test.go`:

```go
package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_EmitsJSON(t *testing.T) {
	var buf bytes.Buffer

	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:    "info",
		Output:   &buf,
		Service:  "test-svc",
		Env:      "test",
		AddSource: false,
	})
	require.NoError(t, err)

	logger.Info("hello world", slog.String("orderID", "abc"))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello world", entry["msg"])
	assert.Equal(t, "abc", entry["orderID"])
	assert.Equal(t, "test-svc", entry["service"])
	assert.Equal(t, "test", entry["env"])
}

func TestNewLogger_RejectsInvalidLevel(t *testing.T) {
	_, err := observability.NewLogger(observability.LoggerOptions{
		Level:   "shout",
		Output:  &bytes.Buffer{},
		Service: "x",
		Env:     "x",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestNewLogger_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer

	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:   "warn",
		Output:  &buf,
		Service: "test",
		Env:     "test",
	})
	require.NoError(t, err)

	logger.Info("filtered out")
	assert.Empty(t, buf.String())

	logger.Warn("kept")
	assert.Contains(t, buf.String(), "kept")
}

func TestFromContext_UsesContextLogger(t *testing.T) {
	var buf bytes.Buffer
	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:   "info",
		Output:  &buf,
		Service: "x",
		Env:     "x",
	})
	require.NoError(t, err)

	ctx := observability.WithLogger(context.Background(), logger)
	got := observability.FromContext(ctx)

	got.Info("from ctx")
	assert.Contains(t, buf.String(), "from ctx")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/core/observability/... -v
```

Expected: build error.

- [ ] **Step 3: Implement `internal/core/observability/logger.go`**

```go
// Package observability provides structured logging, metrics, tracing,
// and error reporting primitives shared across modules.
package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// LoggerOptions configures a slog.Logger.
type LoggerOptions struct {
	Level     string
	Output    io.Writer
	Service   string
	Env       string
	AddSource bool
}

type loggerKey struct{}

// NewLogger builds a JSON slog.Logger with service/env attributes attached.
func NewLogger(opts LoggerOptions) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.AddSource,
	})

	logger := slog.New(handler).With(
		slog.String("service", opts.Service),
		slog.String("env", opts.Env),
	)
	return logger, nil
}

// WithLogger returns a context carrying the given logger.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext extracts a logger from context, falling back to slog.Default.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("observability: invalid log level %q", s)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/core/observability/... -v -run TestNewLogger
go test ./internal/core/observability/... -v -run TestFromContext
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/observability/logger.go internal/core/observability/logger_test.go
git commit -m "feat(observability): add slog logger factory and context propagation"
```

---

## Task 5: Implement Postgres pool factory

**Files:**
- Create: `internal/platform/postgres/pool.go`
- Create: `internal/platform/postgres/pool_test.go`
- Create: `internal/testutil/postgres.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — pgx pool config, parameterized queries, ctx propagation
- `cc-skills-golang:golang-safety` — defer Close patterns, nil-safe ctx
- `cc-skills-golang:golang-error-handling` — wrapping db errors
- `cc-skills-golang:golang-testing` — testcontainers-go usage

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/jackc/pgx/v5 github.com/jackc/pgx/v5/pgxpool
go get github.com/testcontainers/testcontainers-go github.com/testcontainers/testcontainers-go/modules/postgres
go mod tidy
```

- [ ] **Step 2: Write `internal/testutil/postgres.go`**

```go
// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewTestPostgresURL spins up a fresh Postgres container and returns its DSN.
// The container is automatically terminated at test cleanup.
func NewTestPostgresURL(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("marketplace_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	return dsn
}
```

- [ ] **Step 3: Write the failing pool test**

Create `internal/platform/postgres/pool_test.go`:

```go
//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPool_ConnectsAndPings(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)

	cfg := config.Database{
		URL:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	}

	pool, err := postgres.NewPool(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, pool.Ping(context.Background()))

	var got int
	err = pool.QueryRow(context.Background(), "SELECT 1").Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, 1, got)
}

func TestNewPool_InvalidDSNReturnsError(t *testing.T) {
	cfg := config.Database{URL: "not-a-dsn"}

	_, err := postgres.NewPool(context.Background(), cfg)

	require.Error(t, err)
}
```

- [ ] **Step 4: Run integration test to verify it fails**

```bash
go test -tags=integration ./internal/platform/postgres/... -v
```

Expected: build error (`postgres.NewPool undefined`).

- [ ] **Step 5: Implement `internal/platform/postgres/pool.go`**

```go
// Package postgres provides a pgx-backed connection pool factory.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/config"
)

// NewPool builds a pgxpool.Pool from configuration and validates connectivity.
// Caller is responsible for invoking Close on the returned pool at shutdown.
func NewPool(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return pool, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test -tags=integration ./internal/platform/postgres/... -v
```

Expected: PASS (Docker required).

- [ ] **Step 7: Commit**

```bash
git add internal/platform/postgres/ internal/testutil/postgres.go go.mod go.sum
git commit -m "feat(platform): add Postgres pool factory with pgx and integration test"
```

---

## Task 6: Implement Redis client factory

**Files:**
- Create: `internal/platform/redis/client.go`
- Create: `internal/platform/redis/client_test.go`
- Create: `internal/testutil/redis.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — connection lifecycle
- `cc-skills-golang:golang-safety` — Close patterns
- `cc-skills-golang:golang-testing` — testcontainers Redis module
- `cc-skills-golang:golang-error-handling` — wrapping client errors

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/redis/go-redis/v9
go get github.com/testcontainers/testcontainers-go/modules/redis
go mod tidy
```

- [ ] **Step 2: Write `internal/testutil/redis.go`**

```go
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// NewTestRedisAddr spins up a fresh Redis container and returns its host:port.
func NewTestRedisAddr(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	return uri[len("redis://"):]
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/platform/redis/client_test.go`:

```go
//go:build integration

package redis_test

import (
	"context"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/config"
	internalredis "github.com/danilloboing/marketplace-golang/internal/platform/redis"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_ConnectsAndPings(t *testing.T) {
	addr := testutil.NewTestRedisAddr(t)

	cfg := config.Redis{Addr: addr}

	client, err := internalredis.NewClient(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(context.Background()).Err())

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	got, err := client.Get(context.Background(), "k").Result()
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test -tags=integration ./internal/platform/redis/... -v
```

Expected: build error.

- [ ] **Step 5: Implement `internal/platform/redis/client.go`**

```go
// Package redis provides a redis client factory.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/danilloboing/marketplace-golang/internal/config"
)

// NewClient builds a *redis.Client and validates connectivity.
// Caller must Close at shutdown.
func NewClient(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return client, nil
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test -tags=integration ./internal/platform/redis/... -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/redis/ internal/testutil/redis.go go.mod go.sum
git commit -m "feat(platform): add Redis client factory with integration test"
```

---

## Task 7: Set up Atlas migrations + initial extensions migration

**Files:**
- Create: `atlas.hcl`
- Create: `db/migrations/20260508000001_init.sql`

**Skills to consult:**
- `cc-skills-golang:golang-database` — migration tooling decision rationale
- `cc-skills-golang:golang-security` — least-privilege DB user notes

- [ ] **Step 1: Install Atlas CLI (developer task — document in README)**

Add to README a setup section. Run locally to verify:

```bash
curl -sSf https://atlasgo.sh | sh
atlas version
```

Expected: Atlas version printed.

- [ ] **Step 2: Create `atlas.hcl`**

```hcl
env "local" {
  src = "file://db/migrations"
  url = getenv("DATABASE_URL")
  dev = "docker://postgres/16/dev?search_path=public"

  migration {
    dir = "file://db/migrations"
  }
}
```

- [ ] **Step 3: Create `db/migrations/20260508000001_init.sql`**

```sql
-- Enable required extensions for the marketplace.
-- pg_trgm: trigram similarity for fuzzy product search (Phase 1b).
-- citext:  case-insensitive text type for emails and similar fields.
-- pgcrypto: gen_random_uuid + crypto helpers (used for legacy uuid v4 if needed).
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

- [ ] **Step 4: Compute integrity hash and validate**

```bash
atlas migrate hash --dir file://db/migrations
atlas migrate validate --dir file://db/migrations --dev-url "docker://postgres/16/dev?search_path=public"
```

Expected: validation passes, `atlas.sum` created.

- [ ] **Step 5: Apply against the local Docker Postgres**

```bash
docker compose -f deployments/docker-compose.yml up -d postgres
DATABASE_URL="postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable" \
  atlas migrate apply --env local
```

Expected: migration applied, extensions present.

Verify:
```bash
docker exec -it $(docker compose -f deployments/docker-compose.yml ps -q postgres) \
  psql -U marketplace -d marketplace -c "SELECT extname FROM pg_extension;"
```

Expected output includes `pg_trgm`, `citext`, `pgcrypto`.

- [ ] **Step 6: Commit**

```bash
git add atlas.hcl db/
git commit -m "feat(db): add Atlas tooling and initial extensions migration"
```

---

## Task 8: Set up sqlc skeleton

**Files:**
- Create: `db/sqlc.yaml`
- Create: `db/queries/health.sql`
- Create: `internal/platform/postgres/queries/.gitkeep` (output dir)

**Skills to consult:**
- `cc-skills-golang:golang-database` — sqlc patterns, parameterized queries
- `cc-skills-golang:golang-safety` — generated nullable handling
- `cc-skills-golang:golang-naming` — sqlc query naming conventions

- [ ] **Step 1: Create `db/sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: postgresql
    queries: "db/queries"
    schema: "db/migrations"
    gen:
      go:
        package: "queries"
        out: "internal/platform/postgres/queries"
        sql_package: "pgx/v5"
        emit_interface: true
        emit_exact_table_names: false
        emit_json_tags: false
        emit_pointers_for_null_types: true
        omit_unused_structs: true
```

- [ ] **Step 2: Create a smoke query `db/queries/health.sql`**

```sql
-- name: HealthCheck :one
SELECT 1::int AS ok;
```

- [ ] **Step 3: Generate code**

```bash
sqlc generate -f db/sqlc.yaml
```

Expected: files created under `internal/platform/postgres/queries/`.

If sqlc is not installed, install:
```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

- [ ] **Step 4: Verify generated code compiles**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add db/sqlc.yaml db/queries/ internal/platform/postgres/queries/
git commit -m "feat(db): add sqlc tooling and smoke health query"
```

---

## Task 9: Implement health/ready handlers

**Files:**
- Create: `internal/core/health/handler.go`
- Create: `internal/core/health/handler_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-context` — ctx with timeout
- `cc-skills-golang:golang-error-handling` — graceful degradation
- `cc-skills-golang:golang-testing` — net/http/httptest patterns
- `cc-skills-golang:golang-stretchr-testify` — JSON response assertions
- `cc-skills-golang:golang-naming` — handler naming

- [ ] **Step 1: Write the failing test**

Create `internal/core/health/handler_test.go`:

```go
package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChecker struct{ err error }

func (f fakeChecker) Check(_ context.Context) error { return f.err }

func TestLiveness_AlwaysReturns200(t *testing.T) {
	h := health.NewHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Liveness(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadiness_AllHealthyReturns200(t *testing.T) {
	checks := map[string]health.Checker{
		"postgres": fakeChecker{},
		"redis":    fakeChecker{},
	}
	h := health.NewHandler(checks)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"postgres":"ok"`)
	assert.Contains(t, rec.Body.String(), `"redis":"ok"`)
}

func TestReadiness_AnyUnhealthyReturns503(t *testing.T) {
	checks := map[string]health.Checker{
		"postgres": fakeChecker{},
		"redis":    fakeChecker{err: errors.New("conn refused")},
	}
	h := health.NewHandler(checks)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"redis":"conn refused"`)
}

func TestReadiness_RespectsTimeout(t *testing.T) {
	slow := slowChecker{}
	h := health.NewHandlerWithTimeout(map[string]health.Checker{"slow": slow}, 1)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

type slowChecker struct{}

func (slowChecker) Check(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReadiness_NoCheckersReturns200(t *testing.T) {
	h := health.NewHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/core/health/... -v
```

Expected: build error.

- [ ] **Step 3: Implement `internal/core/health/handler.go`**

```go
// Package health provides liveness and readiness HTTP handlers.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker reports component health.
type Checker interface {
	Check(ctx context.Context) error
}

// CheckerFunc adapts a function to Checker.
type CheckerFunc func(ctx context.Context) error

// Check implements Checker.
func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

// Handler exposes /health (liveness) and /ready (readiness).
type Handler struct {
	checkers map[string]Checker
	timeout  time.Duration
}

const defaultReadinessTimeout = 2 * time.Second

// NewHandler builds a handler with the default readiness timeout.
func NewHandler(checkers map[string]Checker) *Handler {
	return NewHandlerWithTimeout(checkers, defaultReadinessTimeout)
}

// NewHandlerWithTimeout overrides the per-checker timeout.
func NewHandlerWithTimeout(checkers map[string]Checker, timeout time.Duration) *Handler {
	return &Handler{checkers: checkers, timeout: timeout}
}

// Liveness always returns 200; the process is up.
func (h *Handler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness verifies all registered components and returns 503 on any failure.
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	results := make(map[string]string, len(h.checkers))
	healthy := true

	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, checker := range h.checkers {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
			defer cancel()

			if err := checker.Check(ctx); err != nil {
				mu.Lock()
				results[name] = err.Error()
				healthy = false
				mu.Unlock()
				return
			}

			mu.Lock()
			results[name] = "ok"
			mu.Unlock()
		}(name, checker)
	}
	wg.Wait()

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, results)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/core/health/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/health/
git commit -m "feat(health): add liveness and readiness handlers with per-component timeouts"
```

---

## Task 10: Implement HTTP middleware (request logger, recover, security headers, CORS)

**Files:**
- Create: `internal/core/httpx/middleware.go`
- Create: `internal/core/httpx/middleware_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-observability` — request logging, slog with HTTP
- `cc-skills-golang:golang-error-handling` — recover patterns
- `cc-skills-golang:golang-security` — security headers, CSP, HSTS
- `cc-skills-golang:golang-context` — request-scoped values
- `cc-skills-golang:golang-naming` — middleware naming
- `cc-skills-golang:golang-testing` — httptest middleware verification

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/go-chi/chi/v5
go get github.com/go-chi/cors
go get github.com/unrolled/secure
go get github.com/google/uuid
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/core/httpx/middleware_test.go`:

```go
package httpx_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_GeneratesAndPropagates(t *testing.T) {
	captured := ""

	handler := httpx.RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = httpx.RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, captured)
	assert.Equal(t, captured, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_RespectsExistingHeader(t *testing.T) {
	captured := ""

	handler := httpx.RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = httpx.RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "abc-123", captured)
	assert.Equal(t, "abc-123", rec.Header().Get("X-Request-ID"))
}

func TestRequestLogger_LogsMethodPathAndStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := httpx.RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "tea")
	}))

	req := httptest.NewRequest(http.MethodGet, "/teapot", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTeapot, rec.Code)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "GET", entry["method"])
	assert.Equal(t, "/teapot", entry["path"])
	assert.EqualValues(t, http.StatusTeapot, entry["status"])
}

func TestRecover_RecoversAndReturns500(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() { handler.ServeHTTP(rec, req) })

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, buf.String(), "boom")
}

func TestSecurityHeaders_SetsExpected(t *testing.T) {
	handler := httpx.SecurityHeaders()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = nil
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, rec.Header().Get("Referrer-Policy"))
}

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	handler := httpx.CORS([]string{"http://allowed.test"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/x", strings.NewReader(""))
	req.Header.Set("Origin", "http://allowed.test")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "http://allowed.test", rec.Header().Get("Access-Control-Allow-Origin"))
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/core/httpx/... -v
```

Expected: build error.

- [ ] **Step 4: Implement `internal/core/httpx/middleware.go`**

```go
// Package httpx provides HTTP middleware shared across modules.
package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/unrolled/secure"
)

type requestIDKey struct{}

const requestIDHeader = "X-Request-ID"

// RequestID assigns each request an ID, surfaces it via header and context.
func RequestID() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(requestIDHeader)
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set(requestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext returns the request id, or "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestLogger logs each request with method, path, status, duration, and request ID.
func RequestLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Duration("durationMs", time.Since(start)),
				slog.String("requestID", RequestIDFromContext(r.Context())),
				slog.String("remoteAddr", r.RemoteAddr),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Recover converts panics to 500s and logs the stack.
func Recover(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", rec),
						slog.String("requestID", RequestIDFromContext(r.Context())),
					)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders applies sane defaults via unrolled/secure.
func SecurityHeaders() func(next http.Handler) http.Handler {
	sm := secure.New(secure.Options{
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		STSSeconds:            31536000,
		STSIncludeSubdomains:  true,
		ContentSecurityPolicy: "default-src 'self'",
	})
	return sm.Handler
}

// CORS configures cross-origin sharing for the given allowed origins.
func CORS(allowed []string) func(next http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowed,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/core/httpx/... -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/httpx/ go.mod go.sum
git commit -m "feat(httpx): add request id, logger, recover, CORS, and security headers middleware"
```

---

## Task 11: Implement Prometheus /metrics endpoint

**Files:**
- Create: `internal/core/observability/metrics.go`
- Create: `internal/core/observability/metrics_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-observability` — Prometheus client patterns, RED metrics
- `cc-skills-golang:golang-naming` — metric naming conventions
- `cc-skills-golang:golang-testing` — promhttp test helpers

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
go get github.com/prometheus/client_golang/prometheus/collectors
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/core/observability/metrics_test.go`:

```go
package observability_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics_HandlerReturnsRegisteredCollectors(t *testing.T) {
	m := observability.NewMetrics()

	m.HTTPRequests.WithLabelValues("GET", "/x", "200").Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "marketplace_http_requests_total")
	assert.Contains(t, body, `method="GET"`)
	assert.Contains(t, body, "go_goroutines")
	assert.Contains(t, body, "process_cpu_seconds_total")
}

func TestNewMetrics_DurationHistogramRegistered(t *testing.T) {
	m := observability.NewMetrics()

	m.HTTPDurationSeconds.WithLabelValues("GET", "/x", "200").Observe(0.123)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "marketplace_http_request_duration_seconds")
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/core/observability/... -v -run Metrics
```

Expected: build error.

- [ ] **Step 4: Implement `internal/core/observability/metrics.go`**

```go
package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the application's Prometheus registry and core metrics.
type Metrics struct {
	registry            *prometheus.Registry
	HTTPRequests        *prometheus.CounterVec
	HTTPDurationSeconds *prometheus.HistogramVec
}

// NewMetrics builds a fresh registry and registers default collectors plus HTTP RED metrics.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "marketplace",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests handled, labelled by method, route, and status.",
	}, []string{"method", "route", "status"})

	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "marketplace",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds, labelled by method, route, and status.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
	)

	return &Metrics{
		registry:            reg,
		HTTPRequests:        httpRequests,
		HTTPDurationSeconds: httpDuration,
	}
}

// Handler returns the HTTP handler that serves Prometheus metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry:          m.registry,
		EnableOpenMetrics: true,
	})
}

// Registry exposes the underlying registry for additional registrations.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/core/observability/... -v -run Metrics
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/observability/metrics.go internal/core/observability/metrics_test.go go.mod go.sum
git commit -m "feat(observability): add Prometheus registry with HTTP RED metrics"
```

---

## Task 12: Implement OpenTelemetry tracing setup

**Files:**
- Create: `internal/core/observability/tracing.go`
- Create: `internal/core/observability/tracing_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-observability` — OTel SDK setup, sampling, OTLP
- `cc-skills-golang:golang-context` — span propagation
- `cc-skills-golang:golang-error-handling` — exporter init failures

- [ ] **Step 1: Add dependencies**

```bash
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk/trace
go get go.opentelemetry.io/otel/sdk/resource
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go get go.opentelemetry.io/otel/semconv/v1.26.0
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/core/observability/tracing_test.go`:

```go
package observability_test

import (
	"context"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestSetupTracing_NoEndpointReturnsNoop(t *testing.T) {
	shutdown, err := observability.SetupTracing(context.Background(), observability.TracingOptions{
		ServiceName: "x",
		Env:         "test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "noop-span")
	defer span.End()

	assert.NotNil(t, span)
}

func TestSetupTracing_AcceptsValidEndpoint(t *testing.T) {
	shutdown, err := observability.SetupTracing(context.Background(), observability.TracingOptions{
		ServiceName:    "x",
		Env:            "test",
		Endpoint:       "localhost:4318",
		SamplerRatio:   0.5,
		Insecure:       true,
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() { _ = shutdown(context.Background()) })
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/core/observability/... -v -run Tracing
```

Expected: build error.

- [ ] **Step 4: Implement `internal/core/observability/tracing.go`**

```go
package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// TracingOptions configures OpenTelemetry tracing.
type TracingOptions struct {
	ServiceName  string
	Env          string
	Endpoint     string
	SamplerRatio float64
	Insecure     bool
}

// ShutdownFunc cleans up tracer providers.
type ShutdownFunc func(context.Context) error

// SetupTracing wires the global tracer provider. If Endpoint is empty,
// a no-op provider is installed so callers can still call otel.Tracer.
func SetupTracing(ctx context.Context, opts TracingOptions) (ShutdownFunc, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(opts.ServiceName),
			semconv.DeploymentEnvironment(opts.Env),
		),
		resource.WithProcess(),
		resource.WithOS(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	if opts.Endpoint == "" {
		tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.TraceContext{})
		return tp.Shutdown, nil
	}

	exporterOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(opts.Endpoint),
	}
	if opts.Insecure {
		exporterOpts = append(exporterOpts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptrace.New(ctx, otlptracehttp.NewClient(exporterOpts...))
	if err != nil {
		return nil, fmt.Errorf("observability: create OTLP trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(opts.SamplerRatio))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/core/observability/... -v -run Tracing
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/observability/tracing.go internal/core/observability/tracing_test.go go.mod go.sum
git commit -m "feat(observability): add OpenTelemetry tracing setup with OTLP exporter"
```

---

## Task 13: Implement Sentry initialization

**Files:**
- Create: `internal/core/observability/sentry.go`
- Create: `internal/core/observability/sentry_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-observability` — error reporting patterns
- `cc-skills-golang:golang-security` — PII scrubbing, request body filtering
- `cc-skills-golang:golang-error-handling` — capture and propagation

- [ ] **Step 1: Add dependency**

```bash
go get github.com/getsentry/sentry-go
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/core/observability/sentry_test.go`:

```go
package observability_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupSentry_NoDSNIsNoop(t *testing.T) {
	flush, err := observability.SetupSentry(observability.SentryOptions{
		Service: "x",
		Env:     "test",
	})
	require.NoError(t, err)
	require.NotNil(t, flush)

	flush()
}

func TestSetupSentry_InvalidDSNReturnsError(t *testing.T) {
	_, err := observability.SetupSentry(observability.SentryOptions{
		DSN:     "not-a-dsn",
		Service: "x",
		Env:     "test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sentry")
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/core/observability/... -v -run Sentry
```

Expected: build error.

- [ ] **Step 4: Implement `internal/core/observability/sentry.go`**

```go
package observability

import (
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// SentryOptions configures Sentry initialization.
type SentryOptions struct {
	DSN              string
	Service          string
	Env              string
	Release          string
	TracesSampleRate float64
}

// FlushFunc waits for in-flight events to be delivered.
type FlushFunc func()

// SetupSentry initializes Sentry. With an empty DSN it returns a no-op flusher.
func SetupSentry(opts SentryOptions) (FlushFunc, error) {
	if opts.DSN == "" {
		return func() {}, nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              opts.DSN,
		Environment:      opts.Env,
		Release:          opts.Release,
		ServerName:       opts.Service,
		AttachStacktrace: true,
		EnableTracing:    opts.TracesSampleRate > 0,
		TracesSampleRate: opts.TracesSampleRate,
		BeforeSend:       scrubPII,
	})
	if err != nil {
		return nil, fmt.Errorf("sentry: init: %w", err)
	}

	return func() { sentry.Flush(2 * time.Second) }, nil
}

func scrubPII(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Request == nil {
		return event
	}

	for _, key := range []string{"Authorization", "Cookie", "Set-Cookie"} {
		delete(event.Request.Headers, key)
	}

	if event.Request.Data != "" && (strings.Contains(strings.ToLower(event.Request.Data), "password") ||
		strings.Contains(strings.ToLower(event.Request.Data), "cpf") ||
		strings.Contains(strings.ToLower(event.Request.Data), "token")) {
		event.Request.Data = "[REDACTED]"
	}

	return event
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/core/observability/... -v -run Sentry
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/observability/sentry.go internal/core/observability/sentry_test.go go.mod go.sum
git commit -m "feat(observability): add Sentry init with PII scrubber"
```

---

## Task 14: Implement HTTP server wrapper with graceful shutdown

**Files:**
- Create: `internal/core/httpx/server.go`
- Create: `internal/core/httpx/server_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — graceful shutdown patterns
- `cc-skills-golang:golang-context` — shutdown ctx propagation
- `cc-skills-golang:golang-concurrency` — signal handling, errgroup
- `cc-skills-golang:golang-error-handling` — listener errors

- [ ] **Step 1: Add dependency**

```bash
go get golang.org/x/sync/errgroup
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/core/httpx/server_test.go`:

```go
package httpx_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_StartAndShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})

	srv := httpx.NewServer(httpx.ServerOptions{
		Addr:            "127.0.0.1:0",
		Handler:         mux,
		ShutdownTimeout: 2 * time.Second,
	})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	resp, err := http.Get("http://" + srv.Addr() + "/x")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	require.NoError(t, srv.Shutdown(context.Background()))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop within deadline")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/core/httpx/... -v -run Server
```

Expected: build error.

- [ ] **Step 4: Implement `internal/core/httpx/server.go`**

```go
package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// ServerOptions configures Server.
type ServerOptions struct {
	Addr            string
	Handler         http.Handler
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Server wraps http.Server with graceful shutdown helpers.
type Server struct {
	http     *http.Server
	listener net.Listener
	addr     string
	shutdown time.Duration
}

// NewServer constructs a Server with sensible defaults.
func NewServer(opts ServerOptions) *Server {
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 10 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 30 * time.Second
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = 90 * time.Second
	}
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = 30 * time.Second
	}

	return &Server{
		http: &http.Server{
			Addr:         opts.Addr,
			Handler:      opts.Handler,
			ReadTimeout:  opts.ReadTimeout,
			WriteTimeout: opts.WriteTimeout,
			IdleTimeout:  opts.IdleTimeout,
		},
		shutdown: opts.ShutdownTimeout,
	}
}

// Start binds the listener and serves HTTP. Returns nil on graceful shutdown.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.addr = ln.Addr().String()

	if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Addr returns the actual bound address (resolves :0 ports).
func (s *Server) Addr() string { return s.addr }

// Shutdown gracefully drains in-flight requests within the configured timeout.
func (s *Server) Shutdown(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, s.shutdown)
	defer cancel()
	return s.http.Shutdown(ctx)
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/core/httpx/... -v -run Server
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/core/httpx/server.go internal/core/httpx/server_test.go go.mod go.sum
git commit -m "feat(httpx): add HTTP server wrapper with graceful shutdown"
```

---

## Task 15: Wire `cmd/api/main.go` and add OpenAPI skeleton

**Files:**
- Create: `cmd/api/main.go`
- Create: `api/openapi.yaml`
- Create: `README.md`

**Skills to consult:**
- `cc-skills-golang:golang-cli` — main entry pattern
- `cc-skills-golang:golang-design-patterns` — composition root, signal handling
- `cc-skills-golang:golang-error-handling` — startup error reporting
- `cc-skills-golang:golang-context` — root context cancellation
- `cc-skills-golang:golang-concurrency` — errgroup for parallel start

- [ ] **Step 1: Create `api/openapi.yaml`**

```yaml
openapi: 3.1.0
info:
  title: Marketplace Golang API
  description: |
    REST API for the marketplace-golang single-vendor e-commerce backend.
    Phase 1a exposes only liveness, readiness, and metrics; domain endpoints
    are added in subsequent phases.
  version: 0.1.0
servers:
  - url: http://localhost:8080
    description: Local development
paths:
  /health:
    get:
      summary: Liveness probe
      description: Returns 200 if the process is running.
      operationId: getHealth
      responses:
        "200":
          description: Process alive
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
                    enum: [ok]
                required: [status]
  /ready:
    get:
      summary: Readiness probe
      description: Returns 200 if all backing services are reachable.
      operationId: getReady
      responses:
        "200":
          description: All checks healthy
          content:
            application/json:
              schema:
                type: object
                additionalProperties:
                  type: string
        "503":
          description: One or more checks failed
          content:
            application/json:
              schema:
                type: object
                additionalProperties:
                  type: string
  /metrics:
    get:
      summary: Prometheus metrics
      description: Prometheus exposition format.
      operationId: getMetrics
      responses:
        "200":
          description: Metrics text
          content:
            text/plain:
              schema:
                type: string
```

- [ ] **Step 2: Create `cmd/api/main.go`**

```go
// Package main is the API server entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/health"
	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	internalredis "github.com/danilloboing/marketplace-golang/internal/platform/redis"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:   cfg.App.LogLevel,
		Output:  os.Stdout,
		Service: cfg.Observability.OTELServiceName,
		Env:     cfg.App.Env,
	})
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	slog.SetDefault(logger)

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	flushSentry, err := observability.SetupSentry(observability.SentryOptions{
		DSN:     cfg.Observability.SentryDSN,
		Service: cfg.Observability.OTELServiceName,
		Env:     cfg.App.Env,
	})
	if err != nil {
		return fmt.Errorf("init sentry: %w", err)
	}
	defer flushSentry()

	shutdownTracing, err := observability.SetupTracing(rootCtx, observability.TracingOptions{
		ServiceName:  cfg.Observability.OTELServiceName,
		Env:          cfg.App.Env,
		Endpoint:     cfg.Observability.OTELExporterEndpoint,
		SamplerRatio: cfg.Observability.OTELTracesSamplerRatio,
		Insecure:     true,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

	pool, err := internalpostgres.NewPool(rootCtx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	rdb, err := internalredis.NewClient(rootCtx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	metrics := observability.NewMetrics()

	healthHandler := health.NewHandler(map[string]health.Checker{
		"postgres": health.CheckerFunc(func(ctx context.Context) error { return pool.Ping(ctx) }),
		"redis":    health.CheckerFunc(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
	})

	router := chi.NewRouter()
	router.Use(httpx.RequestID())
	router.Use(httpx.RequestLogger(logger))
	router.Use(httpx.Recover(logger))
	router.Use(httpx.SecurityHeaders())
	router.Use(httpx.CORS(cfg.CORS.AllowedOrigins))

	router.Get("/health", healthHandler.Liveness)
	router.Get("/ready", healthHandler.Readiness)
	router.Method("GET", "/metrics", metrics.Handler())

	srv := httpx.NewServer(httpx.ServerOptions{
		Addr:            fmt.Sprintf(":%d", cfg.App.Port),
		Handler:         router,
		ShutdownTimeout: cfg.App.ShutdownTimeout,
	})

	logger.Info("api starting",
		slog.Int("port", cfg.App.Port),
		slog.String("env", cfg.App.Env))

	g, gCtx := errgroup.WithContext(rootCtx)
	g.Go(func() error {
		if err := srv.Start(); err != nil {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		<-gCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	logger.Info("api shutdown complete")
	return nil
}
```

- [ ] **Step 3: Add `joho/godotenv` dependency (used by main)**

```bash
go mod tidy
```

- [ ] **Step 4: Build and smoke test the binary**

```bash
docker compose -f deployments/docker-compose.yml up -d
DATABASE_URL="postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable" \
ADMIN_API_TOKEN="dev" \
go run ./cmd/api &
SERVER_PID=$!
sleep 2

curl -fsS http://localhost:8080/health | jq .
curl -fsS http://localhost:8080/ready | jq .
curl -fsS http://localhost:8080/metrics | head -20

kill $SERVER_PID
```

Expected: `/health` returns `{"status":"ok"}`. `/ready` returns 200 with `postgres: ok` and `redis: ok`. `/metrics` exposes `marketplace_*` and `go_*` metrics.

- [ ] **Step 5: Create `README.md`**

```markdown
# marketplace-golang

Single-vendor e-commerce backend (women's clothing/accessories), Brazil first.

## Stack

- Go 1.23+, chi, pgx, sqlc, Atlas, Redis, river, Cloudflare R2
- Observability: slog + Prometheus + OpenTelemetry + Sentry
- Tests: testing + testify + testcontainers-go

See `docs/superpowers/specs/2026-05-08-marketplace-golang-design.md` for full design.

## Local development

### Prerequisites

- Go 1.23+
- Docker (for Postgres, Redis, and integration tests)
- [Atlas CLI](https://atlasgo.io/) for migrations
- [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) for query code generation

### Setup

```bash
cp .env.example .env
make docker-up
DATABASE_URL="postgres://marketplace:marketplace@localhost:5432/marketplace?sslmode=disable" \
  atlas migrate apply --env local
make sqlc-gen
make dev
```

API listens on `http://localhost:8080`. Try `/health`, `/ready`, `/metrics`.

### Common commands

| Command | Purpose |
|---|---|
| `make dev` | Start dependencies and run API |
| `make build` | Build `bin/api` |
| `make test` | Run unit tests with race detector |
| `make test-integration` | Run integration tests (requires Docker) |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code |
| `make migrate` | Apply Atlas migrations |
| `make sqlc-gen` | Regenerate sqlc code |

## Project structure

See `docs/superpowers/specs/2026-05-08-marketplace-golang-design.md` section 3.
```

- [ ] **Step 6: Run linter and tests across the project**

```bash
make fmt
make lint
make test
```

Expected: lint clean, all unit tests pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/api/main.go api/openapi.yaml README.md go.mod go.sum
git commit -m "feat(api): wire cmd/api with full observability + health endpoints"
```

---

## Task 16: Add integration smoke test for the running server

**Files:**
- Create: `tests/integration/api_smoke_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-testing` — integration test layout, build tags
- `cc-skills-golang:golang-context` — ctx with timeout for HTTP calls
- `cc-skills-golang:golang-stretchr-testify` — JSON matchers

- [ ] **Step 1: Write the integration smoke test**

Create `tests/integration/api_smoke_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_Smoke_HealthReadyMetrics(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)
	addr := testutil.NewTestRedisAddr(t)

	port := "18081"

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/api")
	cmd.Env = append(os.Environ(),
		"APP_PORT="+port,
		"APP_ENV=test",
		"APP_LOG_LEVEL=warn",
		"DATABASE_URL="+dsn,
		"REDIS_ADDR="+addr,
		"ADMIN_API_TOKEN=test-token",
		"CORS_ALLOWED_ORIGINS=http://test.local",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	base := "http://127.0.0.1:" + port
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 200*time.Millisecond, "server did not start")

	t.Run("health", func(t *testing.T) {
		resp, err := http.Get(base + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var got map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, "ok", got["status"])
	})

	t.Run("ready", func(t *testing.T) {
		resp, err := http.Get(base + "/ready")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		s := string(body)
		assert.Contains(t, s, `"postgres":"ok"`)
		assert.Contains(t, s, `"redis":"ok"`)
	})

	t.Run("metrics", func(t *testing.T) {
		resp, err := http.Get(base + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		s := string(body)
		assert.True(t, strings.Contains(s, "marketplace_http_requests_total") || strings.Contains(s, "go_goroutines"),
			"expected Prometheus metrics in body, got: %s", s[:min(200, len(s))])
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Run the integration test**

```bash
make test-integration
```

Expected: PASS (Docker required; spins up Postgres + Redis containers, starts the API, hits all three endpoints).

- [ ] **Step 3: Commit**

```bash
git add tests/integration/
git commit -m "test(integration): add smoke test for running api with health/ready/metrics"
```

---

## Task 17: Final lint pass + tag bootstrap milestone

**Files:**
- Modify: any files needed for lint clean-up

**Skills to consult:**
- `cc-skills-golang:golang-lint` — interpreting golangci-lint output, nolint usage
- `cc-skills-golang:golang-modernize` — modern Go idioms
- `cc-skills-golang:golang-code-style` — formatting, imports
- `cc-skills-golang:golang-documentation` — godoc comments at package level

- [ ] **Step 1: Run full lint and fix any reported issues**

```bash
make lint
```

If issues are reported, fix them. Common fixes:
- Add package-level doc comments where missing.
- Replace `interface{}` with `any`.
- Reorder imports per `goimports -local` rules.
- Add `_ =` to ignored returns.

- [ ] **Step 2: Run full unit and integration test suite**

```bash
make test
make test-integration
```

Expected: every test passes.

- [ ] **Step 3: Tag the bootstrap milestone**

```bash
git tag -a v0.1.0-bootstrap -m "Phase 1a: bootstrap foundation complete (HTTP server + observability + DB + Redis)"
```

- [ ] **Step 4: Final verification**

Run:
```bash
git status
git log --oneline -20
```

Expected: clean working tree, commits visible per task.

---

## Spec Coverage Self-Review

| Spec section | Covered by tasks |
|---|---|
| 2.1 Stack core (Go, chi, pgx, sqlc, Atlas, Redis, slog, Prometheus, OTel, Sentry, validator, env, uuid) | T1-T15 |
| 2.9 Observability (slog, /metrics, OTel, Sentry, /health, /ready) | T4, T9, T11, T12, T13, T15 |
| 3.0 Project structure (cmd/, internal/{config,core,platform,testutil}, db/, deployments/, api/, docs/) | T1, T2, T5, T6, T7, T8, T15 |
| 6.1 Security (env config, TLS-ready, security headers, CORS, no-PII logs, recover) | T2, T3, T10, T13 |
| 6.5 Error handling (recover, wrapped errors) | T10, T15 |
| 6.6 12-factor (env config, stdout JSON logs, /health, /ready, signals, graceful shutdown) | T1-T15 |
| Phase 1 Definition of Done — `docker compose` brings stack up, `/health` and `/ready` ok | T2, T15, T16 |
| Phase 1 DoD — coverage targets enforced for catalog domain | **Deferred to Plan 1b** (no domain code in 1a) |
| Phase 1 DoD — image upload + variants generated | **Deferred to Plan 1b** |
| Phase 1 DoD — seed script populates 50 demo products | **Deferred to Plan 1b** |
| Phase 1 DoD — OpenAPI spec render limpo | T15 (skeleton only) |

**Out-of-scope for Plan 1a (lands in Plan 1b):**
- Catalog domain types, repositories, handlers
- Image upload + variants (R2 + libvips)
- Postgres FTS triggers and search query
- Seed script with demo products
- Admin auth middleware (static API token)
- Catalog admin CRUD endpoints

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-08-phase-1a-bootstrap.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
