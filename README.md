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
