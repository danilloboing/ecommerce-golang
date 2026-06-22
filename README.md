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

## Environment variables

Configuration is loaded from environment variables (see `.env.example`). The
table below documents variables introduced per phase.

### Phase 2a — Identity

| Variable | Default | Required | Description |
|---|---|---|---|
| `EMAIL_PROVIDER` | `log` | no | `log` (dev/test, writes to slog) or `ses` (AWS SES). |
| `EMAIL_FROM_ADDRESS` | `no-reply@localhost` | only when `EMAIL_PROVIDER=ses` | Envelope From address. |
| `EMAIL_FROM_NAME` | `Loja` | no | Display name in From header. |
| `EMAIL_VERIFY_LINK_BASE_URL` | — | yes | Frontend URL embedded in verify emails (e.g. `https://app.example/verify`). |
| `EMAIL_RESET_LINK_BASE_URL` | — | yes | Frontend URL embedded in password reset emails. |
| `SES_REGION` | — | only when `EMAIL_PROVIDER=ses` | AWS region. |
| `SES_CONFIGURATION_SET` | — | no | Optional SES configuration set name. |
| `SESSION_TTL_DEFAULT` | `336h` (14d) | no | Default session lifetime. |
| `SESSION_TTL_REMEMBER_ME` | `720h` (30d) | no | Lifetime when `remember=true` at login. |
| `SESSION_REFRESH_AFTER` | `24h` | no | Sliding window threshold to write last_activity. |
| `CSRF_ALLOWED_ORIGINS` | `http://localhost:3000` | no | Comma-separated list of allowed Origin headers on mutations. |
| `RATELIMIT_TRUSTED_PROXIES` | (empty) | no | Comma-separated CIDRs whose `X-Forwarded-For` is trusted. |
| `COOKIES_SECURE_PREFIX` | `false` | no | Set to `true` in production behind HTTPS to enable `__Secure-` cookie name prefix. |

### Phase 2b — Commerce

| Variable | Default | Required | Description |
|---|---|---|---|
| `VIACEP_BASE_URL` | `https://viacep.com.br/ws` | no | ViaCEP API base (no trailing slash). |
| `VIACEP_TIMEOUT` | `3s` | no | Per-request timeout for CEP lookups. |
| `VIACEP_CACHE_TTL` | `1h` | no | Redis cache TTL for resolved CEPs. |
| `CART_ABANDONED_AFTER` | `168h` (7d) | no | Idle period after which an anonymous cart is marked abandoned. |
| `CART_CLEANUP_INTERVAL` | `6h` | no | How often the worker runs the abandoned-cart sweep. |

The `cart_anon` cookie (HttpOnly, 30d) identifies anonymous carts and is cleared on login when its contents merge into the user cart.

## Project structure

See `docs/superpowers/specs/2026-05-08-marketplace-golang-design.md` section 3.
