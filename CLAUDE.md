# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Single-vendor e-commerce backend (women's clothing, Brazil-first), built greenfield in Go as a **modular monolith** (DDD-light). Budget-conscious MVP delivered in phases; quality bar is high (strong tests, strong security, swappable external providers).

## Commands

| Command | Purpose |
|---|---|
| `make dev` | Start Docker deps + run API (`cmd/api`) on `:8080` |
| `make build` | Build `bin/api` |
| `make test` | Unit tests with `-race` + coverage (alias of `test-unit`) |
| `make test-integration` | Integration tests — build tag `integration`, **needs Docker** (testcontainers spins up Postgres/Redis/MinIO) |
| `make lint` | `golangci-lint run ./...` |
| `make fmt` | `gofumpt -w .` + `goimports -local <module>` (not plain gofmt) |
| `make migrate` | Apply Atlas migrations (`--env local`, reads `DATABASE_URL`) |
| `make sqlc-gen` | Regenerate sqlc code from `db/queries/` |
| `make docker-up` / `docker-down` | Start/stop deps (`deployments/docker-compose.yml`) |

Run a single test: `go test -run TestName ./internal/path/...`
Single integration test: `go test -tags=integration -run TestName ./tests/integration/...`

First-time setup: `cp .env.example .env && make docker-up`, then `atlas migrate apply --env local` (with `DATABASE_URL`), then `make sqlc-gen && make dev`.

**Caveat:** golangci-lint may break under a Go 1.26 toolchain. If `make lint` fails, fall back to `go vet ./...` + `make fmt` for verification.

## Architecture

Three binaries in `cmd/`: **`api`** (HTTP server, composition root), **`worker`** (river background jobs — periodic cleanup, registers its own river schema migration), **`tools/seed`** (50 demo products).

Code is organized in three top-level groups under `internal/`:

- **`internal/modules/<context>/`** — bounded contexts (`catalog`, `identity`). Each is a vertical slice with **five layers**:
  - `domain/` — pure types + sentinel errors, no outward deps
  - `application/` — services (business logic) + `ports.go` (interfaces it depends on)
  - `infrastructure/` — Postgres repositories implementing the application ports
  - `transport/` — chi HTTP handlers, request/response DTOs, error mapping
  - `jobs/` — river job workers
- **`internal/core/`** — cross-cutting HTTP machinery, framework-agnostic of any domain: `httpx` (server, request-id, logger, recover, security headers, CORS), `observability` (slog/Prometheus/OTel/Sentry), `health`, `responsex` (error/JSON helpers), `adminauth` (bearer token), `sessionauth` (Redis sessions + middleware), `csrf` (double-submit + Origin check), `ratelimit` (Redis token-bucket).
- **`internal/platform/`** — adapters for external providers, each behind an interface so the provider is swappable: `postgres` (pgx pool + sqlc-generated `queries/`), `redis`, `storage/r2` (S3-compatible), `image` (variants via `disintegration/imaging`, pure Go), `email` (`Sender` interface + `LogSender` + `SESSender`), `passwords` (argon2id), `tokens` (opaque crypto/rand).

**Module wiring pattern** (mirror this when adding a context): each module exposes `New(Deps) *Module` (wires repos→services→handlers via manual constructor injection — **no DI framework**) and `Mount(chi.Router)` (registers route groups, composing per-group middleware). `cmd/api/main.go` is the single composition root: it loads config, builds platform clients (pool, redis, r2, email sender, session manager), then constructs each module's `Deps` and calls `Mount`. Graceful shutdown via `errgroup` + `signal.NotifyContext`.

**Provider abstraction is a hard rule.** Anything external (payment, shipping, search, storage, email) sits behind an interface in `application/ports.go` or `internal/platform/`. Never call a vendor SDK directly from a service — Pagar.me↔Stripe, R2↔S3, Postgres-FTS↔Meilisearch must be swappable without touching business logic.

## Database workflow

- **Migrations**: hand-written SQL in `db/migrations/`, applied with **Atlas** (`atlas.hcl`, env `local`). Never edit applied migrations; add a new timestamped file.
- **Queries**: write SQL in `db/queries/*.sql`, then `make sqlc-gen` (config `db/sqlc.yaml`) regenerates typed Go into `internal/platform/postgres/queries/`. **Never hand-edit generated `*.sql.go` files** — change the `.sql` source and regenerate.

## Conventions (non-obvious)

- **Config**: all via env vars, parsed in `internal/config/config.go` with `caarlos0/env` struct tags, grouped into sectioned sub-structs (App, Database, Redis, Admin, Email, Session, CSRF, RateLimit, Cookies, ViaCEP, …). Required vars use `,required,notEmpty`. `.env` auto-loaded in dev via godotenv.
- **Email provider** chosen at runtime by `EMAIL_PROVIDER=log|ses` via `email.NewSenderFromConfig`.
- **Cookie names** get a `__Secure-` prefix in production when `COOKIES_SECURE_PREFIX=true` — pass the resolved name through, don't hardcode.
- **Identity security policy** (already implemented, follow when extending): argon2id password hashing + `DummyHash` for constant-time login on unknown email; opaque tokens stored SHA-256-hashed; email-verify hard-blocks login; CSRF double-submit + Origin; granular per-endpoint rate limits.
- **Logging**: structured slog only — message first, then key/value attrs.
- One primary export per file; constructor named `New` / `NewXxx`.

## Docs (source of truth for design & plans)

- `docs/superpowers/specs/` — design specs. `2026-05-08-marketplace-golang-design.md` is the full system design (overview, stack, structure §3, domain model, cross-cutting). `2026-05-09-phase-2-identity-cart-design.md` covers Identity+Cart.
- `docs/superpowers/plans/` — per-phase TDD implementation plans (`phase-1a` … `phase-2a`). Read the relevant plan before implementing a phase.
- `docs/adr/` — Architecture Decision Records (currently empty; add ADRs here).
- `api/openapi.yaml` — spec-first OpenAPI; extend it per phase when adding endpoints.

**Phase status**: Phase 1 (catalog + images) merged to `main`. Phase 2a (Identity) is code-complete on `feat/phase-2a-identity` (auth, sessions, CSRF, rate limit, email). Next: Phase 2b (cart + addresses + ViaCEP), then 3 (checkout/payment), 4 (post-sale), 5 (growth).
