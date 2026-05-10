# Phase 2a — Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the complete email+password identity stack on top of Phase 1: register, login, email verification (hard block), password reset, change password, GET/PATCH /me, logout, logout-all — backed by Redis-stored sessions, double-submit CSRF, per-endpoint rate limiting, and a pluggable EmailProvider (LogSender for dev/test, SESSender for prod).

**Architecture:** New module `internal/modules/identity/` (domain/application/infrastructure/jobs/transport mirroring Phase 1 catalog). Three new core middlewares (`internal/core/sessionauth`, `csrf`, `ratelimit`). Four new platform packages (`passwords`, `tokens`, `email`, plus an extended `responsex`). Sessions are Redis-only. Manual constructor injection wires everything in `cmd/api/main.go`. Phase 1 catalog and `adminauth` are not modified.

**Tech Stack:** Go 1.25, chi v5, pgx v5, sqlc, golang.org/x/crypto/argon2, redis/go-redis v9, riverqueue/river v0.35, AWS SDK v2 (sesv2 service to be added), slog, testcontainers-go (Postgres + Redis), testify, mockery.

**Reference spec:** `docs/superpowers/specs/2026-05-09-phase-2-identity-cart-design.md` (commit `8e3620e` on branch `feat/phase-2a-identity`).

**Depends on:** Phase 1 (`v0.3.0-images` tag) — config, postgres pool, redis client, observability, queue, adminauth, catalog module already exist.

**Branch:** `feat/phase-2a-identity` (already created from main; spec already committed there).

---

## Out of Scope (Phase 2a)

- Cart, addresses, ViaCEP — Phase 2b
- Google OAuth — Phase 2.5
- Wishlist, account deletion, email change — later phases
- CAPTCHA, account lockout, audit log persistence — later phases
- Frontend implementation — backend-only deliverable
- Multi-language email templates — pt-BR only

---

## File Structure (created/modified by this plan)

```
marketplace-golang/
├── cmd/
│   └── api/main.go                                                # MODIFIED: wire identity, sessionauth, csrf, ratelimit, email
├── internal/
│   ├── config/
│   │   ├── config.go                                              # MODIFIED: add Email, Sessions, CSRF, RateLimit, Cookies, ViaCEP sections (ViaCEP unused 2a but added now)
│   │   └── config_test.go                                         # MODIFIED: cover new sections
│   ├── core/
│   │   ├── responsex/error.go                                     # MODIFIED: add explicit Error/JSON helpers, keep WriteError for catalog
│   │   ├── responsex/error_test.go                                # MODIFIED
│   │   ├── sessionauth/                                           # NEW
│   │   │   ├── session.go                                         # NEW: Session, Manager, ContextWithSession/SessionFromContext
│   │   │   ├── redis_manager.go                                   # NEW: RedisManager impl
│   │   │   ├── redis_manager_test.go                              # NEW: integration test against testcontainers redis
│   │   │   ├── middleware.go                                      # NEW: Middleware + RequireVerifiedEmail
│   │   │   └── middleware_test.go                                 # NEW
│   │   ├── csrf/                                                  # NEW
│   │   │   ├── middleware.go                                      # NEW
│   │   │   └── middleware_test.go                                 # NEW
│   │   └── ratelimit/                                             # NEW
│   │       ├── middleware.go                                      # NEW
│   │       ├── middleware_test.go                                 # NEW
│   │       ├── realip.go                                          # NEW
│   │       └── realip_test.go                                     # NEW
│   ├── platform/
│   │   ├── passwords/                                             # NEW
│   │   │   ├── passwords.go                                       # NEW: Hash/Verify + DummyHash
│   │   │   └── passwords_test.go                                  # NEW
│   │   ├── tokens/                                                # NEW
│   │   │   ├── tokens.go                                          # NEW: Generate/Hash
│   │   │   └── tokens_test.go                                     # NEW
│   │   └── email/                                                 # NEW
│   │       ├── email.go                                           # NEW: Sender, Message, factory
│   │       ├── log_sender.go                                      # NEW
│   │       ├── log_sender_test.go                                 # NEW
│   │       ├── ses_sender.go                                      # NEW
│   │       ├── ses_sender_test.go                                 # NEW
│   │       ├── templates.go                                       # NEW: verify + reset template constants
│   │       └── templates_test.go                                  # NEW
│   └── modules/identity/                                          # NEW (full module)
│       ├── module.go                                              # NEW
│       ├── domain/
│       │   ├── user.go                                            # NEW
│       │   ├── auth_method.go                                     # NEW
│       │   ├── tokens.go                                          # NEW (EmailVerifyToken, PasswordResetToken)
│       │   ├── errors.go                                          # NEW (sentinel errors)
│       │   └── tokens_test.go                                     # NEW
│       ├── application/
│       │   ├── ports.go                                           # NEW (UserRepository, AuthMethodRepository, TokenRepository, IdentityServiceClock)
│       │   ├── identity_service.go                                # NEW (Register, Login, VerifyEmail, ResendVerifyEmail, RequestPasswordReset, ConfirmPasswordReset, ChangePassword, GetMe, UpdateProfile, Logout, LogoutAll)
│       │   └── identity_service_test.go                           # NEW (mocks for repos + email + sessionauth)
│       ├── infrastructure/
│       │   ├── user_repository.go                                 # NEW
│       │   ├── user_repository_test.go                            # NEW (testcontainers postgres)
│       │   ├── auth_method_repository.go                          # NEW
│       │   ├── auth_method_repository_test.go                     # NEW
│       │   ├── token_repository.go                                # NEW (verify + reset)
│       │   ├── token_repository_test.go                           # NEW
│       │   └── mappers.go                                         # NEW (sqlc row → domain mappers)
│       ├── jobs/
│       │   ├── cleanup_expired_tokens.go                          # NEW
│       │   └── cleanup_expired_tokens_test.go                     # NEW
│       └── transport/
│           ├── auth_handlers.go                                   # NEW (register/login/verify-email/verify-email-resend/password-reset/csrf/logout)
│           ├── auth_handlers_test.go                              # NEW
│           ├── me_handlers.go                                     # NEW (GET /me, PATCH /me, POST /me/change-password, DELETE /auth/sessions/all)
│           ├── me_handlers_test.go                                # NEW
│           ├── error_mapping.go                                   # NEW
│           └── responses.go                                       # NEW (UserResponse DTO etc.)
├── db/
│   ├── migrations/
│   │   └── 20260510000001_identity.sql                            # NEW
│   └── queries/
│       ├── users.sql                                              # NEW
│       ├── auth_methods.sql                                       # NEW
│       ├── email_verify_tokens.sql                                # NEW
│       └── password_reset_tokens.sql                              # NEW
├── api/openapi.yaml                                               # MODIFIED: auth + account paths, schemas, tags
├── tests/integration/                                             # NEW directory if not present
│   └── identity_e2e_test.go                                       # NEW
└── README.md                                                      # MODIFIED: env var docs (or wherever Phase 1 documented)
```

Each file has one responsibility. Cross-module dependencies are isolated through interfaces (`UserRepository`, `Sender`, `sessionauth.Manager`) so the service layer is fully unit-testable with mocks.

---

## Conventional Commit Discipline

Every task ends with a commit using the format established in Phase 1:

```
<type>(<scope>): <imperative summary>

Optional body explaining "why" when non-obvious.
```

Examples (style only — exact messages per task below):
- `feat(identity): add User and AuthMethod domain types`
- `feat(passwords): add argon2id Hash/Verify with PHC encoding`
- `test(identity): add register E2E with email capture`

Do NOT use `git commit --no-verify`. Do NOT amend existing commits — create new ones if hooks fail (per global git safety rules).

---

## Task 1: Add Phase 2a config sections

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-spf13-viper` — env loading patterns (project uses caarlos0/env, but conventions transfer)
- `cc-skills-golang:golang-naming` — struct field naming
- `cc-skills-golang:golang-testing` — config validation tests

- [ ] **Step 1: Write failing test**

Append to `internal/config/config_test.go` (within the existing `TestLoad_*` family):

```go
func TestLoad_PopulatesEmailSessionsCSRFAndRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_API_TOKEN", "abc")
	t.Setenv("STORAGE_ENDPOINT", "http://localhost:9000")
	t.Setenv("STORAGE_ACCESS_KEY_ID", "k")
	t.Setenv("STORAGE_SECRET_ACCESS_KEY", "s")
	t.Setenv("STORAGE_BUCKET", "b")
	t.Setenv("STORAGE_PUBLIC_BASE_URL", "http://localhost:9000/b")

	t.Setenv("EMAIL_PROVIDER", "log")
	t.Setenv("EMAIL_FROM_ADDRESS", "no-reply@example.com")
	t.Setenv("EMAIL_FROM_NAME", "Loja")
	t.Setenv("EMAIL_VERIFY_LINK_BASE_URL", "https://app.example/verify")
	t.Setenv("EMAIL_RESET_LINK_BASE_URL", "https://app.example/reset")

	t.Setenv("SESSION_TTL_DEFAULT", "336h")
	t.Setenv("SESSION_TTL_REMEMBER_ME", "720h")
	t.Setenv("SESSION_REFRESH_AFTER", "24h")

	t.Setenv("CSRF_ALLOWED_ORIGINS", "http://localhost:3000,https://app.example")
	t.Setenv("RATELIMIT_TRUSTED_PROXIES", "10.0.0.0/8")
	t.Setenv("COOKIES_SECURE_PREFIX", "false")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "log", cfg.Email.Provider)
	assert.Equal(t, "no-reply@example.com", cfg.Email.FromAddress)
	assert.Equal(t, "Loja", cfg.Email.FromName)
	assert.Equal(t, "https://app.example/verify", cfg.Email.VerifyLinkBaseURL)
	assert.Equal(t, "https://app.example/reset", cfg.Email.ResetLinkBaseURL)

	assert.Equal(t, 336*time.Hour, cfg.Session.TTLDefault)
	assert.Equal(t, 720*time.Hour, cfg.Session.TTLRememberMe)
	assert.Equal(t, 24*time.Hour, cfg.Session.RefreshAfter)

	assert.Equal(t, []string{"http://localhost:3000", "https://app.example"}, cfg.CSRF.AllowedOrigins)
	assert.Equal(t, []string{"10.0.0.0/8"}, cfg.RateLimit.TrustedProxies)
	assert.False(t, cfg.Cookies.SecurePrefix)
}
```

Add the necessary import for `time` if not present.

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/config/... -run TestLoad_PopulatesEmailSessionsCSRFAndRateLimit -v
```

Expected: build error — `cfg.Email`, `cfg.Session`, `cfg.CSRF`, `cfg.RateLimit`, `cfg.Cookies` undefined.

- [ ] **Step 3: Add new sections to `config.go`**

Edit `internal/config/config.go`. Append new sections to `Config` struct and define structs:

```go
// Config aggregates all runtime configuration sections.
type Config struct {
	App           App
	Database      Database
	Redis         Redis
	Admin         Admin
	CORS          CORS
	Observability Observability
	Storage       Storage
	Email         Email
	Session       Session
	CSRF          CSRF
	RateLimit     RateLimit
	Cookies       Cookies
	ViaCEP        ViaCEP
}

// Email configures outbound email delivery.
type Email struct {
	Provider           string `env:"EMAIL_PROVIDER" envDefault:"log"`
	FromAddress        string `env:"EMAIL_FROM_ADDRESS" envDefault:"no-reply@localhost"`
	FromName           string `env:"EMAIL_FROM_NAME" envDefault:"Loja"`
	VerifyLinkBaseURL  string `env:"EMAIL_VERIFY_LINK_BASE_URL,required,notEmpty"`
	ResetLinkBaseURL   string `env:"EMAIL_RESET_LINK_BASE_URL,required,notEmpty"`
	SESRegion          string `env:"SES_REGION"`
	SESConfigurationSet string `env:"SES_CONFIGURATION_SET"`
}

// Session configures session lifetime semantics.
type Session struct {
	TTLDefault    time.Duration `env:"SESSION_TTL_DEFAULT" envDefault:"336h"`     // 14d
	TTLRememberMe time.Duration `env:"SESSION_TTL_REMEMBER_ME" envDefault:"720h"` // 30d
	RefreshAfter  time.Duration `env:"SESSION_REFRESH_AFTER" envDefault:"24h"`
}

// CSRF configures CSRF middleware behaviour.
type CSRF struct {
	AllowedOrigins []string `env:"CSRF_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
}

// RateLimit configures the rate-limit middleware.
type RateLimit struct {
	TrustedProxies []string `env:"RATELIMIT_TRUSTED_PROXIES" envSeparator:","`
}

// Cookies configures cookie naming/flags.
type Cookies struct {
	SecurePrefix bool `env:"COOKIES_SECURE_PREFIX" envDefault:"false"`
}

// ViaCEP configures the ViaCEP HTTP client (used in Phase 2b but added now).
type ViaCEP struct {
	BaseURL  string        `env:"VIACEP_BASE_URL" envDefault:"https://viacep.com.br/ws"`
	Timeout  time.Duration `env:"VIACEP_TIMEOUT" envDefault:"3s"`
	CacheTTL time.Duration `env:"VIACEP_CACHE_TTL" envDefault:"1h"`
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -run TestLoad_PopulatesEmailSessionsCSRFAndRateLimit -v
go test ./internal/config/... -v
```

Expected: PASS for new test and all existing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add Phase 2a sections for email, session, csrf, ratelimit, cookies, viacep"
```

---

## Task 2: Identity migration + sqlc query files

**Files:**
- Create: `db/migrations/20260510000001_identity.sql`
- Create: `db/queries/users.sql`
- Create: `db/queries/auth_methods.sql`
- Create: `db/queries/email_verify_tokens.sql`
- Create: `db/queries/password_reset_tokens.sql`
- Modify: `db/migrations/atlas.sum` (regenerated by atlas)
- Generated: `internal/platform/postgres/queries/users.sql.go`, `auth_methods.sql.go`, `email_verify_tokens.sql.go`, `password_reset_tokens.sql.go`, plus updates to `models.go` and `querier.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — sqlc patterns, parameterised queries, RETURNING clauses, conditional partial indexes

- [ ] **Step 1: Create the migration**

Write `db/migrations/20260510000001_identity.sql`:

```sql
-- Identity tables: users + auth_methods + opaque single-use tokens.

CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             CITEXT NOT NULL UNIQUE,
    email_verified_at TIMESTAMPTZ,
    name              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_status_idx ON users(status);

CREATE TABLE auth_methods (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('password','google')),
    password_hash    TEXT,
    provider_subject TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ,
    CHECK (
        (provider = 'password' AND password_hash IS NOT NULL AND provider_subject IS NULL)
        OR
        (provider = 'google'   AND provider_subject IS NOT NULL AND password_hash IS NULL)
    )
);

CREATE UNIQUE INDEX auth_methods_user_provider_uniq
    ON auth_methods(user_id, provider);
CREATE UNIQUE INDEX auth_methods_provider_subject_uniq
    ON auth_methods(provider, provider_subject)
    WHERE provider_subject IS NOT NULL;

CREATE TABLE email_verify_tokens (
    token_hash  BYTEA PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email       CITEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX email_verify_tokens_user_active_idx
    ON email_verify_tokens(user_id) WHERE consumed_at IS NULL;

CREATE TABLE password_reset_tokens (
    token_hash  BYTEA PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_user_active_idx
    ON password_reset_tokens(user_id) WHERE consumed_at IS NULL;
```

- [ ] **Step 2: Regenerate atlas hashfile**

Atlas needs an updated checksum after a new file lands.

```bash
atlas migrate hash --dir file://db/migrations
```

Expected: `db/migrations/atlas.sum` updated. Inspect the diff:

```bash
git diff db/migrations/atlas.sum
```

- [ ] **Step 3: Apply migration to a throwaway dev DB to validate SQL**

Atlas’s `migrate apply` requires a DATABASE_URL pointing at a database. For local sanity:

```bash
docker run --rm -d --name pg-2a-dev -p 55433:5432 -e POSTGRES_PASSWORD=pw postgres:16
sleep 3
DATABASE_URL=postgres://postgres:pw@localhost:55433/postgres?sslmode=disable \
  atlas migrate apply --env local
```

Expected output: lists all migrations including `20260510000001_identity.sql` as applied. Verify with:

```bash
PGPASSWORD=pw psql -h localhost -p 55433 -U postgres -c '\d+ users'
PGPASSWORD=pw psql -h localhost -p 55433 -U postgres -c '\d+ auth_methods'
PGPASSWORD=pw psql -h localhost -p 55433 -U postgres -c '\d+ email_verify_tokens'
PGPASSWORD=pw psql -h localhost -p 55433 -U postgres -c '\d+ password_reset_tokens'
```

Tear down:

```bash
docker rm -f pg-2a-dev
```

- [ ] **Step 4: Write sqlc query files**

Create `db/queries/users.sql`:

```sql
-- name: InsertUser :one
INSERT INTO users (id, email, name, status, created_at, updated_at)
VALUES (gen_random_uuid(), $1, $2, 'active', now(), now())
RETURNING id, email, email_verified_at, name, status, created_at, updated_at;

-- name: FindUserByID :one
SELECT id, email, email_verified_at, name, status, created_at, updated_at
FROM users
WHERE id = $1;

-- name: FindUserByEmail :one
SELECT id, email, email_verified_at, name, status, created_at, updated_at
FROM users
WHERE email = $1;

-- name: MarkUserEmailVerified :exec
UPDATE users
SET email_verified_at = now(),
    updated_at = now()
WHERE id = $1 AND email_verified_at IS NULL;

-- name: UpdateUserName :one
UPDATE users
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, email, email_verified_at, name, status, created_at, updated_at;
```

Create `db/queries/auth_methods.sql`:

```sql
-- name: InsertAuthMethodPassword :one
INSERT INTO auth_methods (id, user_id, provider, password_hash, created_at)
VALUES (gen_random_uuid(), $1, 'password', $2, now())
RETURNING id, user_id, provider, password_hash, provider_subject, created_at, last_used_at;

-- name: FindAuthMethodByUserAndProvider :one
SELECT id, user_id, provider, password_hash, provider_subject, created_at, last_used_at
FROM auth_methods
WHERE user_id = $1 AND provider = $2;

-- name: UpdateAuthMethodPassword :exec
UPDATE auth_methods
SET password_hash = $2,
    last_used_at = now()
WHERE user_id = $1 AND provider = 'password';

-- name: TouchAuthMethodLastUsed :exec
UPDATE auth_methods
SET last_used_at = now()
WHERE id = $1;
```

Create `db/queries/email_verify_tokens.sql`:

```sql
-- name: InsertEmailVerifyToken :exec
INSERT INTO email_verify_tokens (token_hash, user_id, email, expires_at)
VALUES ($1, $2, $3, $4);

-- name: FindEmailVerifyToken :one
SELECT token_hash, user_id, email, expires_at, consumed_at, created_at
FROM email_verify_tokens
WHERE token_hash = $1;

-- name: ConsumeEmailVerifyToken :exec
UPDATE email_verify_tokens
SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL;

-- name: DeleteExpiredEmailVerifyTokens :execrows
DELETE FROM email_verify_tokens
WHERE expires_at < now() - interval '7 days';
```

Create `db/queries/password_reset_tokens.sql`:

```sql
-- name: InsertPasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: FindPasswordResetToken :one
SELECT token_hash, user_id, expires_at, consumed_at, created_at
FROM password_reset_tokens
WHERE token_hash = $1;

-- name: ConsumePasswordResetToken :exec
UPDATE password_reset_tokens
SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL;

-- name: DeleteExpiredPasswordResetTokens :execrows
DELETE FROM password_reset_tokens
WHERE expires_at < now() - interval '7 days';
```

- [ ] **Step 5: Regenerate sqlc**

```bash
cd db && sqlc generate && cd ..
```

Verify new files:

```bash
ls internal/platform/postgres/queries/
```

Expected: new files `users.sql.go`, `auth_methods.sql.go`, `email_verify_tokens.sql.go`, `password_reset_tokens.sql.go`. `models.go` and `querier.go` updated.

- [ ] **Step 6: Run existing tests to confirm no breakage**

```bash
go build ./...
go test ./internal/platform/postgres/...
```

Expected: build succeeds, tests pass.

- [ ] **Step 7: Commit**

```bash
git add db/migrations/20260510000001_identity.sql db/migrations/atlas.sum \
        db/queries/users.sql db/queries/auth_methods.sql \
        db/queries/email_verify_tokens.sql db/queries/password_reset_tokens.sql \
        internal/platform/postgres/queries/
git commit -m "feat(db): add identity tables, queries, and sqlc generation"
```

---

## Task 3: Extend `responsex` with explicit error/JSON helpers

The existing `responsex.WriteError(w, err)` couples to catalog errors via `classify`. Phase 2a needs domain-agnostic helpers — add new explicit functions while keeping `WriteError` for Phase 1 catalog handlers (no breakage).

**Files:**
- Modify: `internal/core/responsex/error.go`
- Modify: `internal/core/responsex/error_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-error-handling` — single handling rule, structured logging
- `cc-skills-golang:golang-naming` — function naming
- `cc-skills-golang:golang-observability` — slog at boundaries

- [ ] **Step 1: Write failing test**

Append to `internal/core/responsex/error_test.go`:

```go
func TestError_WritesJSONWithCodeAndMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)

	responsex.Error(rec, r, http.StatusForbidden, "csrf_invalid", "csrf token invalid")

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Error struct {
			Code, Message string
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "csrf_invalid", body.Error.Code)
	assert.Equal(t, "csrf token invalid", body.Error.Message)
}

func TestJSON_WritesPayloadWithStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	responsex.JSON(rec, http.StatusCreated, map[string]string{"id": "abc"})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "abc", body["id"])
}
```

Add the missing imports:

```go
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/core/responsex/... -run "TestError_WritesJSONWithCodeAndMessage|TestJSON_WritesPayloadWithStatus" -v
```

Expected: build error — `responsex.Error` and `responsex.JSON` undefined.

- [ ] **Step 3: Implement helpers**

Modify `internal/core/responsex/error.go` — keep `WriteError` and `classify` intact, add two functions:

```go
// Error writes a JSON error body with explicit status, code, and user-facing message.
// Caller is responsible for choosing status/code; this helper does not classify.
// Internal err details (when present) are logged via slog at warn (4xx) or error (5xx).
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = message

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ErrorWithCause is Error plus structured logging of an internal error chain.
// Use when transport mapped a domain error and wants the original cause logged
// without leaking it in the response body.
func ErrorWithCause(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	logger := observability.FromContext(r.Context())
	attrs := []any{
		slog.Int("status", status),
		slog.String("code", code),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
	}
	if cause != nil {
		attrs = append(attrs, slog.String("error", cause.Error()))
	}
	switch {
	case status >= 500:
		logger.Error("request_failed", attrs...)
	case status >= 400:
		logger.Warn("request_rejected", attrs...)
	}

	Error(w, r, status, code, message)
}

// JSON writes status + arbitrary payload as JSON.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
```

Add the missing imports:

```go
import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/core/responsex/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/responsex/error.go internal/core/responsex/error_test.go
git commit -m "feat(responsex): add explicit Error/ErrorWithCause/JSON helpers for module-driven mapping"
```

---

## Task 4: `internal/platform/passwords` — argon2id Hash + Verify + DummyHash

**Files:**
- Create: `internal/platform/passwords/passwords.go`
- Create: `internal/platform/passwords/passwords_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-security` — password hashing, constant-time compare, never roll your own
- `cc-skills-golang:golang-naming` — package + function naming
- `cc-skills-golang:golang-error-handling` — sentinel errors

- [ ] **Step 1: Write failing test**

Create `internal/platform/passwords/passwords_test.go`:

```go
package passwords_test

import (
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/passwords"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHash_ProducesArgon2idEncodedString(t *testing.T) {
	encoded, err := passwords.Hash("S3cretP@ss!")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=1,p=4$"),
		"expected PHC argon2id prefix, got %q", encoded)
}

func TestVerify_AcceptsCorrectPassword(t *testing.T) {
	encoded, err := passwords.Hash("S3cretP@ss!")
	require.NoError(t, err)

	ok, err := passwords.Verify("S3cretP@ss!", encoded)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerify_RejectsIncorrectPassword(t *testing.T) {
	encoded, err := passwords.Hash("S3cretP@ss!")
	require.NoError(t, err)

	ok, err := passwords.Verify("not-the-password", encoded)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerify_RejectsMalformedEncoded(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=64$",
		"$bcrypt$something",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := passwords.Verify("anything", c)
			require.ErrorIs(t, err, passwords.ErrInvalidEncoded)
		})
	}
}

func TestDummyHash_VerifiesAgainstSomePassword(t *testing.T) {
	require.NotEmpty(t, passwords.DummyHash)
	// Must be a valid PHC argon2id encoded string so Verify does not return ErrInvalidEncoded.
	_, err := passwords.Verify("anything", passwords.DummyHash)
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/platform/passwords/... -v
```

Expected: build error — package missing.

- [ ] **Step 3: Implement passwords package**

Create `internal/platform/passwords/passwords.go`:

```go
// Package passwords hashes and verifies passwords using argon2id (PHC encoding).
package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Above OWASP 2024 minimum (m=19MiB, t=2, p=1) — we trade a
// little CPU for more memory pressure to harden against GPU/ASIC attackers.
const (
	memoryKiB uint32 = 64 * 1024 // 64 MiB
	timeIters uint32 = 1
	threads   uint8  = 4
	keyLen    uint32 = 32
	saltLen   int    = 16
)

// ErrInvalidEncoded indicates a malformed argon2id PHC string.
var ErrInvalidEncoded = errors.New("passwords: invalid encoded format")

// DummyHash is a pre-computed argon2id PHC string used by the login flow to
// keep latency uniform when the email is not found. It MUST never authenticate
// any real input — the source plaintext is intentionally non-meaningful.
var DummyHash string

func init() {
	encoded, err := Hash("dummy-password-not-real-DO-NOT-USE")
	if err != nil {
		panic(fmt.Sprintf("passwords: precompute dummy hash: %v", err))
	}
	DummyHash = encoded
}

// Hash returns a PHC-encoded argon2id hash of plain.
// Format: $argon2id$v=<version>$m=<mem>,t=<time>,p=<threads>$<salt-b64>$<hash-b64>
func Hash(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("passwords: read random salt: %w", err)
	}

	hash := argon2.IDKey([]byte(plain), salt, timeIters, memoryKiB, threads, keyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryKiB, timeIters, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// Verify checks plain against a PHC-encoded argon2id string.
// Returns ErrInvalidEncoded if the string is not well-formed.
func Verify(plain, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// Parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidEncoded
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidEncoded
	}
	if version != argon2.Version {
		return false, ErrInvalidEncoded
	}

	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, ErrInvalidEncoded
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidEncoded
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidEncoded
	}

	candidate := argon2.IDKey([]byte(plain), salt, t, m, p, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, candidate) == 1, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/platform/passwords/... -v
```

Expected: all PASS.

- [ ] **Step 5: Promote argon2 dep from indirect to direct**

```bash
go get golang.org/x/crypto/argon2
go mod tidy
```

Verify go.mod no longer marks `golang.org/x/crypto` as `// indirect`.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/passwords/ go.mod go.sum
git commit -m "feat(passwords): add argon2id Hash/Verify with PHC encoding and DummyHash"
```

---

## Task 5: `internal/platform/tokens` — opaque token Generate/Hash

**Files:**
- Create: `internal/platform/tokens/tokens.go`
- Create: `internal/platform/tokens/tokens_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-security` — `crypto/rand`, never `math/rand` for tokens
- `cc-skills-golang:golang-error-handling` — sentinel errors

- [ ] **Step 1: Write failing test**

Create `internal/platform/tokens/tokens_test.go`:

```go
package tokens_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_Returns64HexCharsAndMatchingHash(t *testing.T) {
	token, hash, err := tokens.Generate()
	require.NoError(t, err)
	assert.Len(t, token, 64, "expected 32 bytes hex-encoded = 64 chars")

	raw, err := hex.DecodeString(token)
	require.NoError(t, err)
	expected := sha256.Sum256(raw)
	assert.Equal(t, expected[:], hash)
}

func TestGenerate_TokensAreUnique(t *testing.T) {
	a, _, err := tokens.Generate()
	require.NoError(t, err)
	b, _, err := tokens.Generate()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestHash_ReturnsSameHashAsGenerate(t *testing.T) {
	token, originalHash, err := tokens.Generate()
	require.NoError(t, err)

	again, err := tokens.Hash(token)
	require.NoError(t, err)
	assert.Equal(t, originalHash, again)
}

func TestHash_RejectsInvalidToken(t *testing.T) {
	cases := []string{
		"",
		"too-short",
		"zz" + string(make([]byte, 62)),
	}
	for _, c := range cases {
		_, err := tokens.Hash(c)
		require.ErrorIs(t, err, tokens.ErrInvalidToken)
	}
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/platform/tokens/... -v
```

Expected: build error.

- [ ] **Step 3: Implement tokens package**

Create `internal/platform/tokens/tokens.go`:

```go
// Package tokens generates and hashes opaque single-use tokens for email
// verification and password reset. Tokens are 32 random bytes hex-encoded;
// only their SHA-256 hash is stored in the database.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const tokenSize = 32

// ErrInvalidToken indicates a malformed or wrong-length token string.
var ErrInvalidToken = errors.New("tokens: invalid token")

// Generate returns a new random token (hex string, 64 chars) and its SHA-256 hash.
// Callers send the hex token via email and persist the hash for later lookup.
func Generate() (token string, hash []byte, err error) {
	raw := make([]byte, tokenSize)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("tokens: read random: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(raw), sum[:], nil
}

// Hash decodes a hex token and returns its SHA-256 hash.
// Returns ErrInvalidToken if decoding fails or length is wrong.
func Hash(token string) ([]byte, error) {
	raw, err := hex.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if len(raw) != tokenSize {
		return nil, ErrInvalidToken
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/platform/tokens/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/tokens/
git commit -m "feat(tokens): add opaque token Generate/Hash backed by crypto/rand"
```

---

## Task 6: `internal/platform/email` — Sender interface + LogSender + templates

**Files:**
- Create: `internal/platform/email/email.go`
- Create: `internal/platform/email/log_sender.go`
- Create: `internal/platform/email/log_sender_test.go`
- Create: `internal/platform/email/templates.go`
- Create: `internal/platform/email/templates_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — interface boundaries, single-method `-er` interfaces
- `cc-skills-golang:golang-naming` — `Sender` not `Provider`
- `cc-skills-golang:golang-observability` — slog from context, structured fields

- [ ] **Step 1: Write failing test for templates**

Create `internal/platform/email/templates_test.go`:

```go
package email_test

import (
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderVerifyEmail_IncludesNameAndLink(t *testing.T) {
	msg, err := email.RenderVerifyEmail(email.VerifyEmailData{
		ToAddress: "ana@example.com",
		Name:      "Ana",
		VerifyURL: "https://app.example/verify?token=abc",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"ana@example.com"}, msg.To)
	assert.Contains(t, msg.Subject, "verifique")
	assert.Contains(t, msg.TextBody, "Ana")
	assert.Contains(t, msg.TextBody, "https://app.example/verify?token=abc")
	assert.Contains(t, msg.HTMLBody, "https://app.example/verify?token=abc")
}

func TestRenderPasswordResetEmail_IncludesLinkAndExpiry(t *testing.T) {
	msg, err := email.RenderPasswordResetEmail(email.PasswordResetEmailData{
		ToAddress: "ana@example.com",
		Name:      "Ana",
		ResetURL:  "https://app.example/reset?token=xyz",
		ExpiryMin: 60,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"ana@example.com"}, msg.To)
	assert.True(t, strings.Contains(msg.Subject, "senha"), "subject should mention password")
	assert.Contains(t, msg.TextBody, "https://app.example/reset?token=xyz")
	assert.Contains(t, msg.TextBody, "60")
}
```

- [ ] **Step 2: Write failing test for LogSender**

Create `internal/platform/email/log_sender_test.go`:

```go
package email_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogSender_Send_LogsStructured(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	sender := email.NewLogSender(logger)
	err := sender.Send(context.Background(), email.Message{
		To:       []string{"ana@example.com"},
		Subject:  "verifique seu email",
		TextBody: "Hello Ana, click https://app.example/verify?token=abc",
	})
	require.NoError(t, err)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry))
	assert.Equal(t, "email_send", entry["msg"])
	assert.Equal(t, "verifique seu email", entry["subject"])
	to, ok := entry["to"].([]any)
	require.True(t, ok)
	assert.Equal(t, "ana@example.com", to[0])
	assert.Contains(t, entry["preview"], "https://app.example/verify?token=abc")
}
```

- [ ] **Step 3: Run failing tests**

```bash
go test ./internal/platform/email/... -v
```

Expected: build error — package missing.

- [ ] **Step 4: Implement Sender + Message + factory shell**

Create `internal/platform/email/email.go`:

```go
// Package email defines the Sender interface and ships pluggable adapters
// (LogSender for dev/test, SESSender for production).
package email

import (
	"context"
	"errors"
	"log/slog"
)

// Message is a single outbound email.
type Message struct {
	To       []string
	Subject  string
	HTMLBody string
	TextBody string
	Tags     map[string]string
}

// Sender delivers messages via the configured provider.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Config controls which adapter the factory builds.
type Config struct {
	Provider            string // "log" or "ses"
	FromAddress         string
	FromName            string
	SESRegion           string
	SESConfigurationSet string
}

// ErrUnknownProvider indicates Config.Provider is not recognised.
var ErrUnknownProvider = errors.New("email: unknown provider")

// NewSenderFromConfig builds a Sender by reading cfg.Provider.
// Falls back to LogSender for empty/log values.
func NewSenderFromConfig(cfg Config, logger *slog.Logger) (Sender, error) {
	switch cfg.Provider {
	case "", "log":
		return NewLogSender(logger), nil
	case "ses":
		return NewSESSender(SESConfig{
			Region:           cfg.SESRegion,
			FromAddress:      cfg.FromAddress,
			FromName:         cfg.FromName,
			ConfigurationSet: cfg.SESConfigurationSet,
		})
	default:
		return nil, ErrUnknownProvider
	}
}
```

- [ ] **Step 5: Implement LogSender**

Create `internal/platform/email/log_sender.go`:

```go
package email

import (
	"context"
	"log/slog"
)

// LogSender writes message metadata and a body preview to slog.
// Used for dev/test — the verify/reset URL is visible in the terminal output.
type LogSender struct {
	logger *slog.Logger
}

// NewLogSender returns a LogSender that uses the given logger.
func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSender{logger: logger}
}

var _ Sender = (*LogSender)(nil)

const previewLen = 200

// Send logs the message instead of delivering it. Always returns nil.
func (s *LogSender) Send(ctx context.Context, msg Message) error {
	s.logger.LogAttrs(ctx, slog.LevelInfo, "email_send",
		slog.Any("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("preview", preview(msg.TextBody, previewLen)),
	)
	return nil
}

func preview(body string, n int) string {
	if len(body) <= n {
		return body
	}
	return body[:n] + "..."
}
```

- [ ] **Step 6: Implement templates**

Create `internal/platform/email/templates.go`:

```go
package email

import (
	"bytes"
	"fmt"
	"text/template"
)

// VerifyEmailData feeds the verify-email template.
type VerifyEmailData struct {
	ToAddress string
	Name      string
	VerifyURL string
}

// PasswordResetEmailData feeds the password-reset template.
type PasswordResetEmailData struct {
	ToAddress string
	Name      string
	ResetURL  string
	ExpiryMin int
}

const verifyTextTpl = `Olá {{.Name}},

Para concluir seu cadastro, verifique seu email clicando no link abaixo:

{{.VerifyURL}}

Se você não solicitou este cadastro, ignore esta mensagem.
`

const verifyHTMLTpl = `<p>Olá {{.Name}},</p>
<p>Para concluir seu cadastro, verifique seu email clicando no link abaixo:</p>
<p><a href="{{.VerifyURL}}">{{.VerifyURL}}</a></p>
<p>Se você não solicitou este cadastro, ignore esta mensagem.</p>
`

const resetTextTpl = `Olá {{.Name}},

Recebemos uma solicitação para redefinir sua senha. Use o link abaixo (válido por {{.ExpiryMin}} minutos):

{{.ResetURL}}

Se você não fez esta solicitação, ignore esta mensagem; sua senha permanece a mesma.
`

const resetHTMLTpl = `<p>Olá {{.Name}},</p>
<p>Recebemos uma solicitação para redefinir sua senha. Use o link abaixo (válido por {{.ExpiryMin}} minutos):</p>
<p><a href="{{.ResetURL}}">{{.ResetURL}}</a></p>
<p>Se você não fez esta solicitação, ignore esta mensagem; sua senha permanece a mesma.</p>
`

// RenderVerifyEmail returns a Message ready to send.
func RenderVerifyEmail(data VerifyEmailData) (Message, error) {
	text, err := render(verifyTextTpl, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render verify text: %w", err)
	}
	html, err := render(verifyHTMLTpl, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render verify html: %w", err)
	}
	return Message{
		To:       []string{data.ToAddress},
		Subject:  "Verifique seu email",
		TextBody: text,
		HTMLBody: html,
		Tags:     map[string]string{"category": "verify_email"},
	}, nil
}

// RenderPasswordResetEmail returns a Message ready to send.
func RenderPasswordResetEmail(data PasswordResetEmailData) (Message, error) {
	text, err := render(resetTextTpl, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render reset text: %w", err)
	}
	html, err := render(resetHTMLTpl, data)
	if err != nil {
		return Message{}, fmt.Errorf("email: render reset html: %w", err)
	}
	return Message{
		To:       []string{data.ToAddress},
		Subject:  "Redefina sua senha",
		TextBody: text,
		HTMLBody: html,
		Tags:     map[string]string{"category": "password_reset"},
	}, nil
}

func render(tpl string, data any) (string, error) {
	t, err := template.New("email").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/platform/email/... -v -run "TestLogSender|TestRenderVerifyEmail|TestRenderPasswordResetEmail"
```

Expected: PASS for templates and LogSender tests. SESSender tests will be added next task and expected to fail until then; for this commit, they should not yet exist.

- [ ] **Step 8: Commit**

```bash
git add internal/platform/email/email.go \
        internal/platform/email/log_sender.go internal/platform/email/log_sender_test.go \
        internal/platform/email/templates.go internal/platform/email/templates_test.go
git commit -m "feat(email): add Sender interface, LogSender, and verify/reset templates"
```

Note: `email.go` references `NewSESSender` and `SESConfig` (defined in Task 7). The package will not build until Task 7 lands. That is acceptable since Tasks 6 and 7 are sequenced — do NOT push or merge between them. Run `go build ./...` only after Task 7 step 6.

If you prefer a clean intermediate build, temporarily replace the `case "ses":` arm in `email.go` with `return nil, ErrUnknownProvider` and revert that line in Task 7. The plan assumes immediate sequencing.

---

## Task 7: `internal/platform/email` — SESSender (AWS SDK SES v2)

**Files:**
- Create: `internal/platform/email/ses_sender.go`
- Create: `internal/platform/email/ses_sender_test.go`
- Modify: `go.mod`, `go.sum` (add `github.com/aws/aws-sdk-go-v2/service/sesv2`)

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — interface implementation, timeouts on external calls
- `cc-skills-golang:golang-security` — never log secret access keys
- `cc-skills-golang:golang-context` — context with deadline on external SDK calls

- [ ] **Step 1: Add the SES v2 dependency**

```bash
go get github.com/aws/aws-sdk-go-v2/service/sesv2
go mod tidy
```

Verify in go.mod:

```
require (
    github.com/aws/aws-sdk-go-v2/service/sesv2 vX.Y.Z
    ...
)
```

- [ ] **Step 2: Write failing test using a fake SES API**

The SESSender must accept a small interface so we can fake the SDK in tests without booting a real SES backend.

Create `internal/platform/email/ses_sender_test.go`:

```go
package email_test

import (
	"context"
	"errors"
	"testing"
	"time"

	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	awssesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSESAPI struct {
	called bool
	in     *awssesv2.SendEmailInput
	out    *awssesv2.SendEmailOutput
	err    error
}

func (f *fakeSESAPI) SendEmail(ctx context.Context, in *awssesv2.SendEmailInput, _ ...func(*awssesv2.Options)) (*awssesv2.SendEmailOutput, error) {
	f.called = true
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	if f.out != nil {
		return f.out, nil
	}
	return &awssesv2.SendEmailOutput{}, nil
}

func TestSESSender_Send_BuildsExpectedRequest(t *testing.T) {
	api := &fakeSESAPI{}
	sender := email.NewSESSenderWithAPI(api, "Loja <no-reply@example.com>", "marketing")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := sender.Send(ctx, email.Message{
		To:       []string{"ana@example.com"},
		Subject:  "olá",
		TextBody: "texto",
		HTMLBody: "<p>html</p>",
		Tags:     map[string]string{"category": "verify_email"},
	})
	require.NoError(t, err)
	require.True(t, api.called)

	require.NotNil(t, api.in.FromEmailAddress)
	assert.Equal(t, "Loja <no-reply@example.com>", *api.in.FromEmailAddress)
	require.NotNil(t, api.in.Destination)
	assert.Equal(t, []string{"ana@example.com"}, api.in.Destination.ToAddresses)

	require.NotNil(t, api.in.Content)
	require.NotNil(t, api.in.Content.Simple)
	require.NotNil(t, api.in.Content.Simple.Subject)
	assert.Equal(t, "olá", *api.in.Content.Simple.Subject.Data)
	require.NotNil(t, api.in.Content.Simple.Body.Text)
	assert.Equal(t, "texto", *api.in.Content.Simple.Body.Text.Data)
	require.NotNil(t, api.in.Content.Simple.Body.Html)
	assert.Equal(t, "<p>html</p>", *api.in.Content.Simple.Body.Html.Data)

	require.NotNil(t, api.in.ConfigurationSetName)
	assert.Equal(t, "marketing", *api.in.ConfigurationSetName)

	require.Len(t, api.in.EmailTags, 1)
	assert.Equal(t, awssesv2types.MessageTag{Name: ptr("category"), Value: ptr("verify_email")}, api.in.EmailTags[0])
}

func TestSESSender_Send_PropagatesError(t *testing.T) {
	api := &fakeSESAPI{err: errors.New("ses unavailable")}
	sender := email.NewSESSenderWithAPI(api, "Loja <no-reply@example.com>", "")

	err := sender.Send(context.Background(), email.Message{
		To:       []string{"ana@example.com"},
		Subject:  "x",
		TextBody: "y",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ses unavailable")
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 3: Run failing test**

```bash
go test ./internal/platform/email/... -v -run TestSESSender
```

Expected: build error — `email.NewSESSenderWithAPI` undefined.

- [ ] **Step 4: Implement SESSender**

Create `internal/platform/email/ses_sender.go`:

```go
package email

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	awssesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// SESConfig configures the SESSender constructor.
type SESConfig struct {
	Region           string
	FromAddress      string
	FromName         string
	ConfigurationSet string
}

// sesAPI is the minimal SES SendEmail surface used here. Implemented by
// awssesv2.Client and by test fakes.
type sesAPI interface {
	SendEmail(ctx context.Context, in *awssesv2.SendEmailInput, optFns ...func(*awssesv2.Options)) (*awssesv2.SendEmailOutput, error)
}

// SESSender delivers messages via AWS SES v2.
type SESSender struct {
	api              sesAPI
	from             string
	configurationSet string
}

var _ Sender = (*SESSender)(nil)

// NewSESSender builds a real SESSender using the AWS default credential chain.
func NewSESSender(cfg SESConfig) (*SESSender, error) {
	if cfg.Region == "" {
		return nil, fmt.Errorf("email: SES region required")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("email: load aws config: %w", err)
	}
	from := cfg.FromAddress
	if cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.FromAddress)
	}
	return &SESSender{
		api:              awssesv2.NewFromConfig(awsCfg),
		from:             from,
		configurationSet: cfg.ConfigurationSet,
	}, nil
}

// NewSESSenderWithAPI is a constructor for tests; injects the SES API surface.
func NewSESSenderWithAPI(api sesAPI, from, configurationSet string) *SESSender {
	return &SESSender{api: api, from: from, configurationSet: configurationSet}
}

// Send delivers msg via SES.
func (s *SESSender) Send(ctx context.Context, msg Message) error {
	in := &awssesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.from),
		Destination: &awssesv2types.Destination{
			ToAddresses: msg.To,
		},
		Content: &awssesv2types.EmailContent{
			Simple: &awssesv2types.Message{
				Subject: &awssesv2types.Content{Data: aws.String(msg.Subject), Charset: aws.String("UTF-8")},
				Body: &awssesv2types.Body{
					Text: &awssesv2types.Content{Data: aws.String(msg.TextBody), Charset: aws.String("UTF-8")},
					Html: &awssesv2types.Content{Data: aws.String(msg.HTMLBody), Charset: aws.String("UTF-8")},
				},
			},
		},
	}
	if s.configurationSet != "" {
		in.ConfigurationSetName = aws.String(s.configurationSet)
	}
	for k, v := range msg.Tags {
		in.EmailTags = append(in.EmailTags, awssesv2types.MessageTag{
			Name:  aws.String(k),
			Value: aws.String(v),
		})
	}

	if _, err := s.api.SendEmail(ctx, in); err != nil {
		return fmt.Errorf("email: ses send: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run all email tests**

```bash
go test ./internal/platform/email/... -v
```

Expected: all PASS.

- [ ] **Step 6: Build everything to confirm Task 6 compile gap is closed**

```bash
go build ./...
```

Expected: build succeeds.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/email/ses_sender.go internal/platform/email/ses_sender_test.go go.mod go.sum
git commit -m "feat(email): add SES v2 sender with injectable API surface for tests"
```

---

## Task 8: `internal/core/sessionauth` — Manager interface, Session, RedisManager

**Files:**
- Create: `internal/core/sessionauth/session.go`
- Create: `internal/core/sessionauth/redis_manager.go`
- Create: `internal/core/sessionauth/redis_manager_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-structs-interfaces` — multi-method `Manager` interface
- `cc-skills-golang:golang-database` — applies to Redis pipelines/MULTI conceptually
- `cc-skills-golang:golang-context` — propagating cancellation to redis client
- `cc-skills-golang:golang-naming` — `RedisManager` not `RedisSessionManager`
- `cc-skills-golang:golang-testing` — testcontainers Redis suite

- [ ] **Step 1: Define the package surface**

Create `internal/core/sessionauth/session.go`:

```go
// Package sessionauth provides user session management and HTTP middleware.
//
// Sessions live in Redis only. A successful login creates a fresh session id
// (32 random bytes hex) and a CSRF token (same shape) stored in a Redis hash.
// A secondary set indexes sessions per user for "logout all devices".
package sessionauth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errors returned by Manager.
var (
	ErrNotFound = errors.New("sessionauth: session not found")
	ErrExpired  = errors.New("sessionauth: session expired")
)

// Session is the authenticated state for a single (user, device) pair.
type Session struct {
	ID             string
	UserID         uuid.UUID
	CSRFToken      string
	CreatedAt      time.Time
	LastActivityAt time.Time
	ExpiresAt      time.Time
	RememberMe     bool
	UserAgent      string
	IP             string
}

// CreateParams describes a new session being created at login.
type CreateParams struct {
	UserID     uuid.UUID
	RememberMe bool
	UserAgent  string
	IP         string
}

// Manager is the session store contract used by transport handlers.
type Manager interface {
	Create(ctx context.Context, p CreateParams) (Session, error)
	Get(ctx context.Context, sessionID string) (Session, error)
	Refresh(ctx context.Context, sessionID string) error
	Delete(ctx context.Context, sessionID string) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	DeleteAllForUserExcept(ctx context.Context, userID uuid.UUID, keepID string) error
}

type ctxKey struct{}

// ContextWithSession injects s into ctx.
func ContextWithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SessionFromContext returns the session stored by Middleware, or false.
func SessionFromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(Session)
	return s, ok
}
```

- [ ] **Step 2: Write failing test for RedisManager**

Create `internal/core/sessionauth/redis_manager_test.go`:

```go
//go:build integration

package sessionauth_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisManager_CreateGetDeleteRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rdb := testutil.StartRedis(t, ctx)

	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:        rdb,
		TTLDefault:    14 * 24 * time.Hour,
		TTLRememberMe: 30 * 24 * time.Hour,
		RefreshAfter:  24 * time.Hour,
	})

	uid := uuid.New()
	created, err := mgr.Create(ctx, sessionauth.CreateParams{
		UserID:    uid,
		UserAgent: "go-test",
		IP:        "127.0.0.1",
	})
	require.NoError(t, err)
	assert.Len(t, created.ID, 64)
	assert.Len(t, created.CSRFToken, 64)
	assert.Equal(t, uid, created.UserID)

	got, err := mgr.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, uid, got.UserID)
	assert.Equal(t, created.CSRFToken, got.CSRFToken)

	require.NoError(t, mgr.Delete(ctx, created.ID))

	_, err = mgr.Get(ctx, created.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
}

func TestRedisManager_DeleteAllForUser_RemovesEverySessionInIndex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := testutil.StartRedis(t, ctx)
	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:     rdb,
		TTLDefault: time.Hour,
	})

	uid := uuid.New()
	a, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)
	b, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)

	require.NoError(t, mgr.DeleteAllForUser(ctx, uid))

	_, err = mgr.Get(ctx, a.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
	_, err = mgr.Get(ctx, b.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
}

func TestRedisManager_DeleteAllForUserExcept_KeepsOne(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := testutil.StartRedis(t, ctx)
	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:     rdb,
		TTLDefault: time.Hour,
	})

	uid := uuid.New()
	keep, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)
	other, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)

	require.NoError(t, mgr.DeleteAllForUserExcept(ctx, uid, keep.ID))

	got, err := mgr.Get(ctx, keep.ID)
	require.NoError(t, err)
	assert.Equal(t, uid, got.UserID)

	_, err = mgr.Get(ctx, other.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
}

func TestRedisManager_Refresh_UpdatesLastActivityAndTTL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := testutil.StartRedis(t, ctx)
	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:       rdb,
		TTLDefault:   time.Hour,
		RefreshAfter: time.Millisecond,
	})

	uid := uuid.New()
	s, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)
	original := s.LastActivityAt

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, mgr.Refresh(ctx, s.ID))

	again, err := mgr.Get(ctx, s.ID)
	require.NoError(t, err)
	assert.True(t, again.LastActivityAt.After(original),
		"expected LastActivityAt to advance, got %v <= %v", again.LastActivityAt, original)
}
```

- [ ] **Step 3: Run failing tests**

```bash
go test -tags=integration -count=1 -timeout=2m ./internal/core/sessionauth/... -v
```

Expected: build error — package missing.

- [ ] **Step 4: Implement RedisManager**

Create `internal/core/sessionauth/redis_manager.go`:

```go
package sessionauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisOptions configures RedisManager.
type RedisOptions struct {
	Client        *redis.Client
	TTLDefault    time.Duration
	TTLRememberMe time.Duration
	RefreshAfter  time.Duration
	Now           func() time.Time
}

// RedisManager stores sessions as hashes in Redis with a per-user index set.
type RedisManager struct {
	client        *redis.Client
	ttlDefault    time.Duration
	ttlRememberMe time.Duration
	refreshAfter  time.Duration
	now           func() time.Time
}

var _ Manager = (*RedisManager)(nil)

// NewRedisManager builds a RedisManager.
func NewRedisManager(opts RedisOptions) *RedisManager {
	if opts.TTLDefault == 0 {
		opts.TTLDefault = 14 * 24 * time.Hour
	}
	if opts.TTLRememberMe == 0 {
		opts.TTLRememberMe = 30 * 24 * time.Hour
	}
	if opts.RefreshAfter == 0 {
		opts.RefreshAfter = 24 * time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &RedisManager{
		client:        opts.Client,
		ttlDefault:    opts.TTLDefault,
		ttlRememberMe: opts.TTLRememberMe,
		refreshAfter:  opts.RefreshAfter,
		now:           opts.Now,
	}
}

func sessionKey(id string) string         { return "session:" + id }
func userIndexKey(uid uuid.UUID) string   { return "session:user:" + uid.String() }

// Create stores a new session and adds it to the per-user index.
func (m *RedisManager) Create(ctx context.Context, p CreateParams) (Session, error) {
	id, err := randomHex(32)
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: random session id: %w", err)
	}
	csrf, err := randomHex(32)
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: random csrf: %w", err)
	}

	ttl := m.ttlDefault
	if p.RememberMe {
		ttl = m.ttlRememberMe
	}
	now := m.now().UTC()
	expiresAt := now.Add(ttl)

	sess := Session{
		ID:             id,
		UserID:         p.UserID,
		CSRFToken:      csrf,
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      expiresAt,
		RememberMe:     p.RememberMe,
		UserAgent:      p.UserAgent,
		IP:             p.IP,
	}

	pipe := m.client.TxPipeline()
	pipe.HSet(ctx, sessionKey(id), serializeSession(sess))
	pipe.ExpireAt(ctx, sessionKey(id), expiresAt)
	pipe.SAdd(ctx, userIndexKey(p.UserID), id)
	pipe.ExpireAt(ctx, userIndexKey(p.UserID), expiresAt)
	if _, err := pipe.Exec(ctx); err != nil {
		return Session{}, fmt.Errorf("sessionauth: create pipeline: %w", err)
	}
	return sess, nil
}

// Get fetches a session by id. Returns ErrNotFound if missing.
func (m *RedisManager) Get(ctx context.Context, sessionID string) (Session, error) {
	res, err := m.client.HGetAll(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: hgetall: %w", err)
	}
	if len(res) == 0 {
		return Session{}, ErrNotFound
	}
	return deserializeSession(sessionID, res)
}

// Refresh advances LastActivityAt and renews TTL for both keys.
func (m *RedisManager) Refresh(ctx context.Context, sessionID string) error {
	sess, err := m.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	now := m.now().UTC()
	if now.Sub(sess.LastActivityAt) < m.refreshAfter {
		// No-op; not stale enough to warrant a Redis write.
		return nil
	}
	sess.LastActivityAt = now

	ttl := m.ttlDefault
	if sess.RememberMe {
		ttl = m.ttlRememberMe
	}
	expiresAt := now.Add(ttl)
	sess.ExpiresAt = expiresAt

	pipe := m.client.TxPipeline()
	pipe.HSet(ctx, sessionKey(sessionID), serializeSession(sess))
	pipe.ExpireAt(ctx, sessionKey(sessionID), expiresAt)
	pipe.ExpireAt(ctx, userIndexKey(sess.UserID), expiresAt)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sessionauth: refresh pipeline: %w", err)
	}
	return nil
}

// Delete removes a single session and its index entry.
func (m *RedisManager) Delete(ctx context.Context, sessionID string) error {
	sess, err := m.Get(ctx, sessionID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	pipe := m.client.TxPipeline()
	pipe.Del(ctx, sessionKey(sessionID))
	pipe.SRem(ctx, userIndexKey(sess.UserID), sessionID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sessionauth: delete pipeline: %w", err)
	}
	return nil
}

// DeleteAllForUser removes every session for the given user.
func (m *RedisManager) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	ids, err := m.client.SMembers(ctx, userIndexKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("sessionauth: smembers: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	keys := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		keys = append(keys, sessionKey(id))
	}
	keys = append(keys, userIndexKey(userID))

	if _, err := m.client.Del(ctx, keys...).Result(); err != nil {
		return fmt.Errorf("sessionauth: del all: %w", err)
	}
	return nil
}

// DeleteAllForUserExcept keeps the session keepID and deletes the rest.
func (m *RedisManager) DeleteAllForUserExcept(ctx context.Context, userID uuid.UUID, keepID string) error {
	ids, err := m.client.SMembers(ctx, userIndexKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("sessionauth: smembers: %w", err)
	}

	pipe := m.client.TxPipeline()
	for _, id := range ids {
		if id == keepID {
			continue
		}
		pipe.Del(ctx, sessionKey(id))
		pipe.SRem(ctx, userIndexKey(userID), id)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sessionauth: prune pipeline: %w", err)
	}
	return nil
}

// --- helpers ---

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func serializeSession(s Session) map[string]string {
	return map[string]string{
		"user_id":          s.UserID.String(),
		"csrf_token":       s.CSRFToken,
		"created_at":       strconv.FormatInt(s.CreatedAt.UnixNano(), 10),
		"last_activity_at": strconv.FormatInt(s.LastActivityAt.UnixNano(), 10),
		"expires_at":       strconv.FormatInt(s.ExpiresAt.UnixNano(), 10),
		"remember_me":      strconv.FormatBool(s.RememberMe),
		"user_agent":       s.UserAgent,
		"ip":               s.IP,
	}
}

func deserializeSession(id string, data map[string]string) (Session, error) {
	uid, err := uuid.Parse(data["user_id"])
	if err != nil {
		return Session{}, fmt.Errorf("sessionauth: parse user_id: %w", err)
	}
	created, err := parseUnixNano(data["created_at"])
	if err != nil {
		return Session{}, err
	}
	last, err := parseUnixNano(data["last_activity_at"])
	if err != nil {
		return Session{}, err
	}
	expires, err := parseUnixNano(data["expires_at"])
	if err != nil {
		return Session{}, err
	}
	remember, _ := strconv.ParseBool(data["remember_me"])

	return Session{
		ID:             id,
		UserID:         uid,
		CSRFToken:      data["csrf_token"],
		CreatedAt:      created,
		LastActivityAt: last,
		ExpiresAt:      expires,
		RememberMe:     remember,
		UserAgent:      data["user_agent"],
		IP:             data["ip"],
	}, nil
}

func parseUnixNano(s string) (time.Time, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("sessionauth: parse time %q: %w", s, err)
	}
	return time.Unix(0, n).UTC(), nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test -tags=integration -count=1 -timeout=2m ./internal/core/sessionauth/... -v
```

Expected: PASS for all four integration tests.

- [ ] **Step 6: Commit**

```bash
git add internal/core/sessionauth/session.go \
        internal/core/sessionauth/redis_manager.go \
        internal/core/sessionauth/redis_manager_test.go
git commit -m "feat(sessionauth): add Manager interface, Session, and Redis-backed manager"
```

---

## Task 9: `internal/core/sessionauth` — HTTP middleware

**Files:**
- Create: `internal/core/sessionauth/middleware.go`
- Create: `internal/core/sessionauth/middleware_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — middleware composition
- `cc-skills-golang:golang-error-handling` — single point of logging at boundary

- [ ] **Step 1: Write failing test**

Create `internal/core/sessionauth/middleware_test.go`:

```go
package sessionauth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubManager struct {
	getResp     sessionauth.Session
	getErr      error
	refreshErr  error
	deleteErr   error
	getCalls    int
	refreshCalls int
}

func (s *stubManager) Create(context.Context, sessionauth.CreateParams) (sessionauth.Session, error) {
	return sessionauth.Session{}, errors.New("not used")
}
func (s *stubManager) Get(_ context.Context, id string) (sessionauth.Session, error) {
	s.getCalls++
	return s.getResp, s.getErr
}
func (s *stubManager) Refresh(_ context.Context, id string) error {
	s.refreshCalls++
	return s.refreshErr
}
func (s *stubManager) Delete(context.Context, string) error                          { return s.deleteErr }
func (s *stubManager) DeleteAllForUser(context.Context, uuid.UUID) error             { return nil }
func (s *stubManager) DeleteAllForUserExcept(context.Context, uuid.UUID, string) error { return nil }

func TestMiddleware_AttachesSessionWhenCookieValid(t *testing.T) {
	mgr := &stubManager{getResp: sessionauth.Session{
		ID: "sid", UserID: uuid.New(), CSRFToken: "ct",
		LastActivityAt: time.Now(),
	}}

	called := false
	handler := sessionauth.Middleware(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		s, ok := sessionauth.SessionFromContext(r.Context())
		require.True(t, ok)
		assert.Equal(t, "sid", s.ID)
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, called)
	assert.Equal(t, 1, mgr.getCalls)
	assert.Equal(t, 1, mgr.refreshCalls)
}

func TestMiddleware_Returns401WhenCookieMissing(t *testing.T) {
	mgr := &stubManager{}
	handler := sessionauth.Middleware(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner should not run")
	}))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_Returns401WhenSessionNotFound(t *testing.T) {
	mgr := &stubManager{getErr: sessionauth.ErrNotFound}
	handler := sessionauth.Middleware(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner should not run")
	}))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "missing"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Cookie should be cleared (Set-Cookie with Max-Age=0).
	cookies := rec.Result().Cookies()
	require.NotEmpty(t, cookies)
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			found = c
			break
		}
	}
	require.NotNil(t, found)
	assert.True(t, found.MaxAge < 0 || found.MaxAge == 0 && found.Value == "")
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/core/sessionauth/... -v -run TestMiddleware
```

Expected: build error.

- [ ] **Step 3: Implement middleware**

Create `internal/core/sessionauth/middleware.go`:

```go
package sessionauth

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
)

// CookieName is the cookie carrying the session id. The Cookies.SecurePrefix
// flag in config controls whether `__Secure-` is prepended at write time.
const CookieName = "session_id"

// Middleware reads the session_id cookie, looks up the session, refreshes its
// activity timestamp, and injects Session into the request context.
func Middleware(mgr Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			}

			sess, err := mgr.Get(r.Context(), cookie.Value)
			switch {
			case errors.Is(err, ErrNotFound), errors.Is(err, ErrExpired):
				clearCookie(w, CookieName)
				responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			case err != nil:
				responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "session lookup failed", err)
				return
			}

			// Best-effort refresh; not fatal if it fails.
			_ = mgr.Refresh(r.Context(), sess.ID)

			ctx := ContextWithSession(r.Context(), sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireVerifiedEmail composes after Middleware and rejects sessions whose
// user has not verified their email. It re-reads the session and assumes the
// caller injected it. The flag itself is enforced earlier (login refuses
// unverified users), so this exists as defense in depth for routes that may
// be reached otherwise.
//
// In Phase 2a it has no required wiring (login already enforces). It will be
// applied to checkout in Phase 3.
func RequireVerifiedEmail(check func(userID string) (bool, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			}
			verified, err := check(sess.UserID.String())
			if err != nil {
				responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "verify check failed", err)
				return
			}
			if !verified {
				responsex.Error(w, r, http.StatusForbidden, "email_not_verified", "email verification required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/core/sessionauth/... -v
```

Expected: PASS for both unit middleware tests and prior integration tests (the latter only when `-tags=integration`).

- [ ] **Step 5: Commit**

```bash
git add internal/core/sessionauth/middleware.go internal/core/sessionauth/middleware_test.go
git commit -m "feat(sessionauth): add HTTP middleware injecting Session into request context"
```

---

## Task 10: `internal/core/csrf` — double-submit + Origin middleware

**Files:**
- Create: `internal/core/csrf/middleware.go`
- Create: `internal/core/csrf/middleware_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-security` — CSRF defense in depth, constant-time compare
- `cc-skills-golang:golang-design-patterns` — middleware composition order

- [ ] **Step 1: Write failing test**

Create `internal/core/csrf/middleware_test.go`:

```go
package csrf_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	allowedOrigin = "https://app.example"
	csrfCookie    = "csrf_token"
	csrfHeader    = "X-CSRF-Token"
)

func cfg() csrf.Config {
	return csrf.Config{
		AllowedOrigins: []string{allowedOrigin},
		CookieName:     csrfCookie,
	}
}

func mwWithSession(s sessionauth.Session) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sessionauth.ContextWithSession(r.Context(), s)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCSRF_PassesGetRequests(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRF_RejectsMutationWithMismatchedOrigin(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_RejectsMutationWithoutHeader(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_RejectsMutationWhenCookieAndHeaderDiffer(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set(csrfHeader, "abc")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "different"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_AcceptsMutationWhenAllChecksPass(t *testing.T) {
	sess := sessionauth.Session{ID: "sid", UserID: uuid.New(), CSRFToken: "abc"}
	h := mwWithSession(sess)(csrf.Middleware(cfg())(okHandler()))

	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set(csrfHeader, "abc")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRF_RejectsMutationWhenSessionTokenDiffers(t *testing.T) {
	sess := sessionauth.Session{ID: "sid", UserID: uuid.New(), CSRFToken: "session-token"}
	h := mwWithSession(sess)(csrf.Middleware(cfg())(okHandler()))

	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set(csrfHeader, "abc")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/core/csrf/... -v
```

Expected: build error — package missing.

- [ ] **Step 3: Implement middleware**

Create `internal/core/csrf/middleware.go`:

```go
// Package csrf provides double-submit cookie + Origin check middleware.
package csrf

import (
	"crypto/subtle"
	"net/http"
	"slices"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
)

// HeaderName is the request header that must mirror the CSRF cookie.
const HeaderName = "X-CSRF-Token"

// Config controls the middleware.
type Config struct {
	AllowedOrigins []string
	CookieName     string // e.g. "csrf_token" or "__Secure-csrf_token"
}

// Middleware enforces double-submit + Origin for unsafe HTTP methods.
//
// On safe methods (GET/HEAD/OPTIONS) the request is passed through.
// On unsafe methods:
//  1. If Origin header is set, it MUST match cfg.AllowedOrigins.
//  2. The csrf_token cookie and X-CSRF-Token header MUST be present, equal,
//     and equal to the session's stored csrf_token (if a session is present).
func Middleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafe(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if origin := r.Header.Get("Origin"); origin != "" {
				if !slices.Contains(cfg.AllowedOrigins, origin) {
					responsex.Error(w, r, http.StatusForbidden, "csrf_origin_invalid", "origin not allowed")
					return
				}
			}

			cookie, err := r.Cookie(cfg.CookieName)
			if err != nil || cookie.Value == "" {
				responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf cookie missing")
				return
			}
			header := r.Header.Get(HeaderName)
			if header == "" {
				responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf header missing")
				return
			}

			if !equal(cookie.Value, header) {
				responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf token mismatch")
				return
			}

			if sess, ok := sessionauth.SessionFromContext(r.Context()); ok {
				if !equal(sess.CSRFToken, cookie.Value) {
					responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf token does not match session")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isSafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/core/csrf/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/csrf/
git commit -m "feat(csrf): add double-submit + Origin check middleware"
```

---

## Task 11: `internal/core/ratelimit` — Rule, Middleware, realIP

**Files:**
- Create: `internal/core/ratelimit/realip.go`
- Create: `internal/core/ratelimit/realip_test.go`
- Create: `internal/core/ratelimit/middleware.go`
- Create: `internal/core/ratelimit/middleware_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-concurrency` — Lua script atomicity for fixed-window counters
- `cc-skills-golang:golang-security` — trusting client headers (`X-Forwarded-For`)
- `cc-skills-golang:golang-context` — Redis ops with context

- [ ] **Step 1: Write failing test for realIP**

Create `internal/core/ratelimit/realip_test.go`:

```go
package ratelimit_test

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseCIDRs(t *testing.T, raw ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		p, err := netip.ParsePrefix(s)
		require.NoError(t, err)
		out = append(out, p)
	}
	return out
}

func TestRealIP_UsesRemoteAddrWhenNoTrustedProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	got := ratelimit.RealIP(r, nil)
	assert.Equal(t, "8.8.8.8", got)
}

func TestRealIP_ReturnsClientHopWhenComingFromTrustedProxy(t *testing.T) {
	trusted := parseCIDRs(t, "10.0.0.0/8")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.1.2.4")

	got := ratelimit.RealIP(r, trusted)
	assert.Equal(t, "1.2.3.4", got)
}

func TestRealIP_FallsBackToRemoteAddrWhenXFFEmpty(t *testing.T) {
	trusted := parseCIDRs(t, "10.0.0.0/8")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:1234"

	got := ratelimit.RealIP(r, trusted)
	assert.Equal(t, "10.1.2.3", got)
}
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/core/ratelimit/... -v -run TestRealIP
```

Expected: build error.

- [ ] **Step 3: Implement realIP**

Create `internal/core/ratelimit/realip.go`:

```go
package ratelimit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP returns the originating client IP for the request.
// If r.RemoteAddr falls inside trustedProxies, X-Forwarded-For is honoured
// (rightmost-trusted hops stripped, leftmost remaining hop returned).
// Otherwise RemoteAddr (host portion) is used.
func RealIP(r *http.Request, trustedProxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}

	if !inAny(addr, trustedProxies) {
		return host
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}

	parts := strings.Split(xff, ",")
	// Walk right-to-left, skipping trusted hops; return first non-trusted hop.
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		hopAddr, err := netip.ParseAddr(hop)
		if err != nil {
			continue
		}
		if !inAny(hopAddr, trustedProxies) {
			return hop
		}
	}
	return host
}

func inAny(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Write failing test for Middleware**

Create `internal/core/ratelimit/middleware_test.go`:

```go
//go:build integration

package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_AllowsUpToLimitThenBlocks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := testutil.StartRedis(t, ctx)

	mw := ratelimit.Middleware(ratelimit.Options{
		Client: rdb,
		Rules: []ratelimit.Rule{
			{Key: "test:ip", Source: ratelimit.ByIP, Limit: 3, Window: time.Minute},
		},
	})

	called := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("POST", "/x", nil)
		r.RemoteAddr = "1.2.3.4:80"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		require.Equal(t, http.StatusNoContent, rec.Code, "request %d should pass", i+1)
	}

	r := httptest.NewRequest("POST", "/x", nil)
	r.RemoteAddr = "1.2.3.4:80"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Equal(t, 3, called)
}
```

- [ ] **Step 5: Run failing test**

```bash
go test -tags=integration -count=1 ./internal/core/ratelimit/... -v -run TestMiddleware
```

Expected: build error.

- [ ] **Step 6: Implement Middleware**

Create `internal/core/ratelimit/middleware.go`:

```go
package ratelimit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/redis/go-redis/v9"
)

// Source identifies how a rule keys its bucket.
type Source int

const (
	SourceUnknown Source = iota
	ByIP
	ByEmailField
	ByUserID
)

// Rule defines a single rate-limit constraint applied per request.
type Rule struct {
	Key    string
	Source Source
	Field  string // for ByEmailField — JSON path (e.g. "email")
	Limit  int
	Window time.Duration
}

// Options configures the middleware.
type Options struct {
	Client         *redis.Client
	Rules          []Rule
	TrustedProxies []netip.Prefix
}

// Lua script: atomic INCR + EXPIRE on first hit. Returns current count.
const incrScript = `local v = redis.call('INCR', KEYS[1])
if v == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return v`

// Middleware enforces all configured rules on every request.
// Each rule's key is computed from request data; the first rule to exceed
// its limit short-circuits with 429.
func Middleware(opts Options) func(http.Handler) http.Handler {
	script := redis.NewScript(incrScript)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Buffer body once if any rule needs JSON field access.
			var bodyBuf []byte
			needBody := false
			for _, rule := range opts.Rules {
				if rule.Source == ByEmailField {
					needBody = true
					break
				}
			}
			if needBody && r.Body != nil {
				buf, err := io.ReadAll(r.Body)
				if err == nil {
					bodyBuf = buf
					r.Body = io.NopCloser(bytes.NewReader(buf))
				}
			}

			now := time.Now().Unix()
			for _, rule := range opts.Rules {
				bucket := now / int64(rule.Window.Seconds())
				keyPart, ok := keyForRule(r, rule, opts.TrustedProxies, bodyBuf)
				if !ok {
					continue
				}
				key := fmt.Sprintf("ratelimit:%s:%s:%d", rule.Key, keyPart, bucket)

				count, err := script.Run(r.Context(), opts.Client, []string{key},
					int(rule.Window.Seconds())).Int()
				if err != nil {
					responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "rate limit failure", err)
					return
				}
				if count > rule.Limit {
					retry := int64(rule.Window.Seconds()) - (now % int64(rule.Window.Seconds()))
					w.Header().Set("Retry-After", strconv.FormatInt(retry, 10))
					responsex.Error(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func keyForRule(r *http.Request, rule Rule, trusted []netip.Prefix, body []byte) (string, bool) {
	switch rule.Source {
	case ByIP:
		return "ip:" + RealIP(r, trusted), true
	case ByUserID:
		sess, ok := sessionauth.SessionFromContext(r.Context())
		if !ok {
			return "", false
		}
		return "user:" + sess.UserID.String(), true
	case ByEmailField:
		if len(body) == 0 || rule.Field == "" {
			return "", false
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", false
		}
		v, ok := payload[rule.Field].(string)
		if !ok || v == "" {
			return "", false
		}
		sum := sha256.Sum256([]byte(v))
		return "email:" + hex.EncodeToString(sum[:]), true
	}
	return "", false
}
```

- [ ] **Step 7: Run all ratelimit tests**

```bash
go test ./internal/core/ratelimit/... -v
go test -tags=integration -count=1 ./internal/core/ratelimit/... -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/core/ratelimit/
git commit -m "feat(ratelimit): add Redis token-bucket middleware with realIP helper"
```

---

## Task 12: `internal/modules/identity/domain` — types + sentinel errors

**Files:**
- Create: `internal/modules/identity/domain/user.go`
- Create: `internal/modules/identity/domain/auth_method.go`
- Create: `internal/modules/identity/domain/tokens.go`
- Create: `internal/modules/identity/domain/errors.go`
- Create: `internal/modules/identity/domain/tokens_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-structs-interfaces` — value-object structs
- `cc-skills-golang:golang-naming` — sentinel `Err*` with package prefix
- `cc-skills-golang:golang-error-handling` — sentinel inventory

- [ ] **Step 1: Write failing test**

Create `internal/modules/identity/domain/tokens_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailVerifyToken_IsConsumed(t *testing.T) {
	tk := domain.EmailVerifyToken{}
	assert.False(t, tk.IsConsumed())
	tk.ConsumedAt = ptrTime(time.Now())
	assert.True(t, tk.IsConsumed())
}

func TestEmailVerifyToken_IsExpired(t *testing.T) {
	tk := domain.EmailVerifyToken{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, tk.IsExpired(time.Now()))

	tk.ExpiresAt = time.Now().Add(-time.Minute)
	assert.True(t, tk.IsExpired(time.Now()))
}

func TestPasswordResetToken_IsConsumedAndIsExpired(t *testing.T) {
	tk := domain.PasswordResetToken{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, tk.IsConsumed())
	assert.False(t, tk.IsExpired(time.Now()))

	tk.ConsumedAt = ptrTime(time.Now())
	tk.ExpiresAt = time.Now().Add(-time.Minute)
	assert.True(t, tk.IsConsumed())
	assert.True(t, tk.IsExpired(time.Now()))
}

func TestSentinelErrors_AreNotEqualButComparableViaIs(t *testing.T) {
	require.NotEqual(t, domain.ErrInvalidCredentials, domain.ErrEmailNotVerified)
}

func ptrTime(t time.Time) *time.Time { return &t }
```

- [ ] **Step 2: Run failing test**

```bash
go test ./internal/modules/identity/domain/... -v
```

Expected: build error — package missing.

- [ ] **Step 3: Implement domain**

Create `internal/modules/identity/domain/errors.go`:

```go
// Package domain holds the identity bounded context's value objects and errors.
package domain

import "errors"

// Sentinel errors. Map to HTTP at the transport boundary via error_mapping.go.
var (
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	ErrEmailNotVerified   = errors.New("identity: email not verified")
	ErrUserNotFound       = errors.New("identity: user not found")
	ErrEmailAlreadyTaken  = errors.New("identity: email already taken")
	ErrTokenExpired       = errors.New("identity: token expired")
	ErrTokenAlreadyUsed   = errors.New("identity: token already used")
	ErrTokenNotFound      = errors.New("identity: token not found")
	ErrSessionExpired     = errors.New("identity: session expired")
	ErrSessionNotFound    = errors.New("identity: session not found")
	ErrPasswordTooWeak    = errors.New("identity: password does not meet policy")
)
```

Create `internal/modules/identity/domain/user.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserStatus values match the DB CHECK constraint.
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusDeleted   UserStatus = "deleted"
)

// User is the identity aggregate root.
type User struct {
	ID              uuid.UUID
	Email           string
	EmailVerifiedAt *time.Time
	Name            string
	Status          UserStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsEmailVerified returns true when EmailVerifiedAt is set.
func (u User) IsEmailVerified() bool { return u.EmailVerifiedAt != nil }

// IsActive returns true when Status == active.
func (u User) IsActive() bool { return u.Status == UserStatusActive }
```

Create `internal/modules/identity/domain/auth_method.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuthProvider enumerates supported login methods.
type AuthProvider string

const (
	AuthProviderPassword AuthProvider = "password"
	AuthProviderGoogle   AuthProvider = "google" // implementation deferred to Phase 2.5
)

// AuthMethod is one credential bound to a user.
type AuthMethod struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Provider        AuthProvider
	PasswordHash    *string // present only when Provider == AuthProviderPassword
	ProviderSubject *string // present only for OAuth providers
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}
```

Create `internal/modules/identity/domain/tokens.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmailVerifyToken is an opaque single-use token.
// Plaintext is sent via email; hash is what's stored.
type EmailVerifyToken struct {
	TokenHash  []byte
	UserID     uuid.UUID
	Email      string // snapshot at issuance
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// IsConsumed returns true once the token has been used.
func (t EmailVerifyToken) IsConsumed() bool { return t.ConsumedAt != nil }

// IsExpired returns true if now >= ExpiresAt.
func (t EmailVerifyToken) IsExpired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}

// PasswordResetToken mirrors EmailVerifyToken without the email snapshot.
type PasswordResetToken struct {
	TokenHash  []byte
	UserID     uuid.UUID
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

func (t PasswordResetToken) IsConsumed() bool { return t.ConsumedAt != nil }
func (t PasswordResetToken) IsExpired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/domain/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/domain/
git commit -m "feat(identity): add domain types and sentinel errors"
```

---

## Task 13: `internal/modules/identity/infrastructure` — UserRepository + AuthMethodRepository

**Files:**
- Create: `internal/modules/identity/application/ports.go`
- Create: `internal/modules/identity/infrastructure/mappers.go`
- Create: `internal/modules/identity/infrastructure/user_repository.go`
- Create: `internal/modules/identity/infrastructure/user_repository_test.go`
- Create: `internal/modules/identity/infrastructure/auth_method_repository.go`
- Create: `internal/modules/identity/infrastructure/auth_method_repository_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — pgx + sqlc patterns, mapping nullable cols
- `cc-skills-golang:golang-error-handling` — `pgx.ErrNoRows` → domain sentinel
- `cc-skills-golang:golang-testing` — testcontainers Postgres, fixture helpers

- [ ] **Step 1: Define ports**

Create `internal/modules/identity/application/ports.go`:

```go
// Package application contains the IdentityService and its repository ports.
package application

import (
	"context"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/google/uuid"
)

// UserRepository persists the user aggregate.
type UserRepository interface {
	Insert(ctx context.Context, email, name string) (domain.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error
	UpdateName(ctx context.Context, id uuid.UUID, name string) (domain.User, error)
}

// AuthMethodRepository persists per-user credentials.
type AuthMethodRepository interface {
	InsertPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (domain.AuthMethod, error)
	FindForUser(ctx context.Context, userID uuid.UUID, provider domain.AuthProvider) (domain.AuthMethod, error)
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
}

// EmailVerifyTokenRepository persists email-verification tokens.
type EmailVerifyTokenRepository interface {
	Insert(ctx context.Context, hash []byte, userID uuid.UUID, email string, expiresAt time.Time) error
	Find(ctx context.Context, hash []byte) (domain.EmailVerifyToken, error)
	Consume(ctx context.Context, hash []byte) error
}

// PasswordResetTokenRepository persists password-reset tokens.
type PasswordResetTokenRepository interface {
	Insert(ctx context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error
	Find(ctx context.Context, hash []byte) (domain.PasswordResetToken, error)
	Consume(ctx context.Context, hash []byte) error
}
```

- [ ] **Step 2: Implement mappers**

Create `internal/modules/identity/infrastructure/mappers.go`:

```go
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapUser(row queries.User) domain.User {
	return domain.User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		Name:            row.Name,
		Status:          domain.UserStatus(row.Status),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func mapAuthMethod(row queries.AuthMethod) domain.AuthMethod {
	return domain.AuthMethod{
		ID:              row.ID,
		UserID:          row.UserID,
		Provider:        domain.AuthProvider(row.Provider),
		PasswordHash:    row.PasswordHash,
		ProviderSubject: row.ProviderSubject,
		CreatedAt:       row.CreatedAt,
		LastUsedAt:      row.LastUsedAt,
	}
}
```

Note: replace `queries.User`, `queries.AuthMethod` with the actual struct names sqlc generated. Inspect `internal/platform/postgres/queries/models.go` after Task 2 — sqlc names tables PascalCase singular.

- [ ] **Step 3: Implement UserRepository**

Create `internal/modules/identity/infrastructure/user_repository.go`:

```go
// Package infrastructure wires identity domain to Postgres via sqlc.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
	"github.com/google/uuid"
)

// UserRepository is the Postgres implementation of application.UserRepository.
type UserRepository struct {
	q *queries.Queries
}

var _ application.UserRepository = (*UserRepository)(nil)

// NewUserRepository builds a UserRepository over the given pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{q: queries.New(pool)}
}

// Insert creates a user and returns the row.
// Returns domain.ErrEmailAlreadyTaken on unique violation.
func (r *UserRepository) Insert(ctx context.Context, email, name string) (domain.User, error) {
	row, err := r.q.InsertUser(ctx, queries.InsertUserParams{Email: email, Name: name})
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, domain.ErrEmailAlreadyTaken
		}
		return domain.User{}, fmt.Errorf("user repo: insert: %w", err)
	}
	return mapUser(row), nil
}

// FindByID returns ErrUserNotFound if missing.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := r.q.FindUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("user repo: find by id: %w", err)
	}
	return mapUser(row), nil
}

// FindByEmail returns ErrUserNotFound if missing.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.q.FindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("user repo: find by email: %w", err)
	}
	return mapUser(row), nil
}

// MarkEmailVerified is idempotent — repeated calls do not error.
func (r *UserRepository) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	if err := r.q.MarkUserEmailVerified(ctx, id); err != nil {
		return fmt.Errorf("user repo: mark email verified: %w", err)
	}
	return nil
}

// UpdateName updates the name and returns the new row.
func (r *UserRepository) UpdateName(ctx context.Context, id uuid.UUID, name string) (domain.User, error) {
	row, err := r.q.UpdateUserName(ctx, queries.UpdateUserNameParams{ID: id, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("user repo: update name: %w", err)
	}
	return mapUser(row), nil
}
```

Add helper for unique violation detection at the bottom of `mappers.go`:

```go
import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
```

(Move imports as needed; the helper goes in `mappers.go`.)

- [ ] **Step 4: Write failing repo test**

Create `internal/modules/identity/infrastructure/user_repository_test.go`:

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_InsertFindUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	repo := infrastructure.NewUserRepository(pool)

	u, err := repo.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.Equal(t, "ana@example.com", u.Email)
	assert.Equal(t, "Ana", u.Name)
	assert.False(t, u.IsEmailVerified())

	got, err := repo.FindByEmail(ctx, "ANA@example.com") // CITEXT case-insensitive
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)

	gotByID, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, gotByID.ID)

	require.NoError(t, repo.MarkEmailVerified(ctx, u.ID))
	verified, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, verified.IsEmailVerified())

	updated, err := repo.UpdateName(ctx, u.ID, "Ana Lima")
	require.NoError(t, err)
	assert.Equal(t, "Ana Lima", updated.Name)
}

func TestUserRepository_Insert_ReturnsErrEmailAlreadyTakenOnDuplicate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	repo := infrastructure.NewUserRepository(pool)

	_, err := repo.Insert(ctx, "dup@example.com", "X")
	require.NoError(t, err)
	_, err = repo.Insert(ctx, "DUP@example.com", "Y")
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrEmailAlreadyTaken))
}

func TestUserRepository_FindByID_ReturnsNotFoundForUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	repo := infrastructure.NewUserRepository(pool)

	_, err := repo.FindByID(ctx, uuid.New())
	require.True(t, errors.Is(err, domain.ErrUserNotFound))
}
```

- [ ] **Step 5: Implement AuthMethodRepository**

Create `internal/modules/identity/infrastructure/auth_method_repository.go`:

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
	"github.com/google/uuid"
)

// AuthMethodRepository is the Postgres implementation of application.AuthMethodRepository.
type AuthMethodRepository struct {
	q *queries.Queries
}

var _ application.AuthMethodRepository = (*AuthMethodRepository)(nil)

func NewAuthMethodRepository(pool *pgxpool.Pool) *AuthMethodRepository {
	return &AuthMethodRepository{q: queries.New(pool)}
}

func (r *AuthMethodRepository) InsertPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (domain.AuthMethod, error) {
	row, err := r.q.InsertAuthMethodPassword(ctx, queries.InsertAuthMethodPasswordParams{
		UserID:       userID,
		PasswordHash: stringPtr(passwordHash),
	})
	if err != nil {
		return domain.AuthMethod{}, fmt.Errorf("auth_method repo: insert password: %w", err)
	}
	return mapAuthMethod(row), nil
}

func (r *AuthMethodRepository) FindForUser(ctx context.Context, userID uuid.UUID, provider domain.AuthProvider) (domain.AuthMethod, error) {
	row, err := r.q.FindAuthMethodByUserAndProvider(ctx, queries.FindAuthMethodByUserAndProviderParams{
		UserID:   userID,
		Provider: string(provider),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AuthMethod{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.AuthMethod{}, fmt.Errorf("auth_method repo: find: %w", err)
	}
	return mapAuthMethod(row), nil
}

func (r *AuthMethodRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	if err := r.q.UpdateAuthMethodPassword(ctx, queries.UpdateAuthMethodPasswordParams{
		UserID:       userID,
		PasswordHash: stringPtr(passwordHash),
	}); err != nil {
		return fmt.Errorf("auth_method repo: update password: %w", err)
	}
	return nil
}

func (r *AuthMethodRepository) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	if err := r.q.TouchAuthMethodLastUsed(ctx, id); err != nil {
		return fmt.Errorf("auth_method repo: touch last used: %w", err)
	}
	return nil
}

func stringPtr(s string) *string { return &s }
```

If sqlc generated `PasswordHash` as `pgtype.Text` rather than `*string`, adjust `stringPtr` accordingly. The plan assumes the project's existing sqlc config uses pointer types for nullable columns (matches `db/sqlc.yaml`).

- [ ] **Step 6: Write failing test for AuthMethodRepository**

Create `internal/modules/identity/infrastructure/auth_method_repository_test.go`:

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMethodRepository_InsertFindUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)

	users := infrastructure.NewUserRepository(pool)
	auths := infrastructure.NewAuthMethodRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	am, err := auths.InsertPassword(ctx, u.ID, "$argon2id$v=19$m=65536,t=1,p=4$abc$def")
	require.NoError(t, err)
	require.NotNil(t, am.PasswordHash)
	assert.Equal(t, domain.AuthProviderPassword, am.Provider)

	got, err := auths.FindForUser(ctx, u.ID, domain.AuthProviderPassword)
	require.NoError(t, err)
	assert.Equal(t, am.ID, got.ID)

	require.NoError(t, auths.UpdatePassword(ctx, u.ID, "$argon2id$v=19$m=65536,t=1,p=4$xxx$yyy"))
	updated, err := auths.FindForUser(ctx, u.ID, domain.AuthProviderPassword)
	require.NoError(t, err)
	require.NotNil(t, updated.PasswordHash)
	assert.NotEqual(t, am.PasswordHash, updated.PasswordHash)
}

func TestAuthMethodRepository_FindForUser_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)

	users := infrastructure.NewUserRepository(pool)
	auths := infrastructure.NewAuthMethodRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	_, err = auths.FindForUser(ctx, u.ID, domain.AuthProviderPassword)
	require.True(t, errors.Is(err, domain.ErrUserNotFound))
}
```

- [ ] **Step 7: Run tests**

```bash
go test -tags=integration -count=1 -timeout=5m ./internal/modules/identity/infrastructure/... -v
```

Expected: PASS for all repo tests.

- [ ] **Step 8: Commit**

```bash
git add internal/modules/identity/application/ports.go \
        internal/modules/identity/infrastructure/
git commit -m "feat(identity): add User and AuthMethod Postgres repositories"
```

---

## Task 14: `internal/modules/identity/infrastructure` — TokenRepository (verify + reset)

**Files:**
- Create: `internal/modules/identity/infrastructure/token_repository.go`
- Create: `internal/modules/identity/infrastructure/token_repository_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — BYTEA columns, partial indexes
- `cc-skills-golang:golang-error-handling` — sentinel mapping

- [ ] **Step 1: Write failing test**

Create `internal/modules/identity/infrastructure/token_repository_test.go`:

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailVerifyTokenRepository_RoundTripAndConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	users := infrastructure.NewUserRepository(pool)
	tokens := infrastructure.NewEmailVerifyTokenRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	hash := []byte("0123456789abcdef0123456789abcdef")
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)

	require.NoError(t, tokens.Insert(ctx, hash, u.ID, "ana@example.com", expires))

	got, err := tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.UserID)
	assert.False(t, got.IsConsumed())

	require.NoError(t, tokens.Consume(ctx, hash))
	got, err = tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.True(t, got.IsConsumed())
}

func TestEmailVerifyTokenRepository_FindReturnsNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	tokens := infrastructure.NewEmailVerifyTokenRepository(pool)

	_, err := tokens.Find(ctx, []byte("does-not-exist--------------------"))
	require.True(t, errors.Is(err, domain.ErrTokenNotFound))
}

func TestPasswordResetTokenRepository_RoundTripAndConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	users := infrastructure.NewUserRepository(pool)
	tokens := infrastructure.NewPasswordResetTokenRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	hash := []byte("aabbccddeeff00112233445566778899")
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	require.NoError(t, tokens.Insert(ctx, hash, u.ID, expires))

	got, err := tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.UserID)
	require.NoError(t, tokens.Consume(ctx, hash))

	got, err = tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.True(t, got.IsConsumed())
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test -tags=integration -count=1 -timeout=5m ./internal/modules/identity/infrastructure/... -v -run TokenRepository
```

Expected: build error.

- [ ] **Step 3: Implement repositories**

Append to `internal/modules/identity/infrastructure/mappers.go`:

```go
func mapEmailVerifyToken(row queries.EmailVerifyToken) domain.EmailVerifyToken {
	return domain.EmailVerifyToken{
		TokenHash:  row.TokenHash,
		UserID:     row.UserID,
		Email:      row.Email,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func mapPasswordResetToken(row queries.PasswordResetToken) domain.PasswordResetToken {
	return domain.PasswordResetToken{
		TokenHash:  row.TokenHash,
		UserID:     row.UserID,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}
```

Create `internal/modules/identity/infrastructure/token_repository.go`:

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
	"github.com/google/uuid"
)

// EmailVerifyTokenRepository persists email verify tokens.
type EmailVerifyTokenRepository struct {
	q *queries.Queries
}

var _ application.EmailVerifyTokenRepository = (*EmailVerifyTokenRepository)(nil)

func NewEmailVerifyTokenRepository(pool *pgxpool.Pool) *EmailVerifyTokenRepository {
	return &EmailVerifyTokenRepository{q: queries.New(pool)}
}

func (r *EmailVerifyTokenRepository) Insert(ctx context.Context, hash []byte, userID uuid.UUID, email string, expiresAt time.Time) error {
	if err := r.q.InsertEmailVerifyToken(ctx, queries.InsertEmailVerifyTokenParams{
		TokenHash: hash,
		UserID:    userID,
		Email:     email,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("verify token repo: insert: %w", err)
	}
	return nil
}

func (r *EmailVerifyTokenRepository) Find(ctx context.Context, hash []byte) (domain.EmailVerifyToken, error) {
	row, err := r.q.FindEmailVerifyToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EmailVerifyToken{}, domain.ErrTokenNotFound
	}
	if err != nil {
		return domain.EmailVerifyToken{}, fmt.Errorf("verify token repo: find: %w", err)
	}
	return mapEmailVerifyToken(row), nil
}

func (r *EmailVerifyTokenRepository) Consume(ctx context.Context, hash []byte) error {
	if err := r.q.ConsumeEmailVerifyToken(ctx, hash); err != nil {
		return fmt.Errorf("verify token repo: consume: %w", err)
	}
	return nil
}

// PasswordResetTokenRepository persists password reset tokens.
type PasswordResetTokenRepository struct {
	q *queries.Queries
}

var _ application.PasswordResetTokenRepository = (*PasswordResetTokenRepository)(nil)

func NewPasswordResetTokenRepository(pool *pgxpool.Pool) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{q: queries.New(pool)}
}

func (r *PasswordResetTokenRepository) Insert(ctx context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error {
	if err := r.q.InsertPasswordResetToken(ctx, queries.InsertPasswordResetTokenParams{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("reset token repo: insert: %w", err)
	}
	return nil
}

func (r *PasswordResetTokenRepository) Find(ctx context.Context, hash []byte) (domain.PasswordResetToken, error) {
	row, err := r.q.FindPasswordResetToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PasswordResetToken{}, domain.ErrTokenNotFound
	}
	if err != nil {
		return domain.PasswordResetToken{}, fmt.Errorf("reset token repo: find: %w", err)
	}
	return mapPasswordResetToken(row), nil
}

func (r *PasswordResetTokenRepository) Consume(ctx context.Context, hash []byte) error {
	if err := r.q.ConsumePasswordResetToken(ctx, hash); err != nil {
		return fmt.Errorf("reset token repo: consume: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test -tags=integration -count=1 -timeout=5m ./internal/modules/identity/infrastructure/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/infrastructure/token_repository.go \
        internal/modules/identity/infrastructure/token_repository_test.go \
        internal/modules/identity/infrastructure/mappers.go
git commit -m "feat(identity): add email verify + password reset token repositories"
```

---

## Task 15: `internal/modules/identity/jobs` — cleanup_expired_tokens river job

**Files:**
- Create: `internal/modules/identity/jobs/cleanup_expired_tokens.go`
- Create: `internal/modules/identity/jobs/cleanup_expired_tokens_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-database` — DELETE with WHERE expires_at
- river docs (`internal/modules/catalog/jobs/cleanup_orphans.go`) for reference pattern

- [ ] **Step 1: Write failing test**

Create `internal/modules/identity/jobs/cleanup_expired_tokens_test.go`:

```go
//go:build integration

package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/jobs"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupExpiredTokens_DeletesOldTokens(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := testutil.StartPostgres(t, ctx)
	users := infrastructure.NewUserRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	q := queries.New(pool)

	// One stale (expired 8 days ago), one fresh (expires tomorrow).
	stale := []byte("00112233445566778899aabbccddeeff")
	fresh := []byte("ffeeddccbbaa99887766554433221100")
	require.NoError(t, q.InsertEmailVerifyToken(ctx, queries.InsertEmailVerifyTokenParams{
		TokenHash: stale, UserID: u.ID, Email: "ana@example.com",
		ExpiresAt: time.Now().Add(-8 * 24 * time.Hour),
	}))
	require.NoError(t, q.InsertEmailVerifyToken(ctx, queries.InsertEmailVerifyTokenParams{
		TokenHash: fresh, UserID: u.ID, Email: "ana@example.com",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}))

	deleted, err := jobs.RunCleanupExpiredTokensOnce(ctx, pool)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(1))

	_, err = q.FindEmailVerifyToken(ctx, stale)
	require.Error(t, err)
	got, err := q.FindEmailVerifyToken(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.UserID)

	_ = uuid.Nil
}
```

- [ ] **Step 2: Run failing test**

```bash
go test -tags=integration -count=1 ./internal/modules/identity/jobs/... -v
```

Expected: build error.

- [ ] **Step 3: Implement job**

Create `internal/modules/identity/jobs/cleanup_expired_tokens.go`:

```go
// Package jobs holds river background workers for the identity module.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// CleanupExpiredTokensArgs is the river job payload (no fields needed).
type CleanupExpiredTokensArgs struct{}

// Kind implements river.JobArgs.
func (CleanupExpiredTokensArgs) Kind() string { return "identity.cleanup_expired_tokens" }

// CleanupExpiredTokensWorker prunes expired tokens older than 7 days.
type CleanupExpiredTokensWorker struct {
	river.WorkerDefaults[CleanupExpiredTokensArgs]
	pool *pgxpool.Pool
}

// NewCleanupExpiredTokensWorker builds the worker.
func NewCleanupExpiredTokensWorker(pool *pgxpool.Pool) *CleanupExpiredTokensWorker {
	return &CleanupExpiredTokensWorker{pool: pool}
}

// Work runs once per scheduled tick.
func (w *CleanupExpiredTokensWorker) Work(ctx context.Context, _ *river.Job[CleanupExpiredTokensArgs]) error {
	if _, err := RunCleanupExpiredTokensOnce(ctx, w.pool); err != nil {
		return err
	}
	return nil
}

// RunCleanupExpiredTokensOnce executes the cleanup statement directly.
// Used by tests and by the periodic worker. Returns total deleted rows
// across both token tables.
func RunCleanupExpiredTokensOnce(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	q := queries.New(pool)
	v, err := q.DeleteExpiredEmailVerifyTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("identity jobs: delete expired verify tokens: %w", err)
	}
	r, err := q.DeleteExpiredPasswordResetTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("identity jobs: delete expired reset tokens: %w", err)
	}
	return v + r, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test -tags=integration -count=1 ./internal/modules/identity/jobs/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/jobs/
git commit -m "feat(identity): add cleanup_expired_tokens river job"
```

---

## Task 16: `internal/modules/identity/application` — IdentityService.Register

**Files:**
- Create: `internal/modules/identity/application/identity_service.go`
- Create: `internal/modules/identity/application/identity_service_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — service constructor with explicit deps
- `cc-skills-golang:golang-error-handling` — domain sentinels, wrapping
- `cc-skills-golang:golang-stretchr-testify` — mock-based unit tests
- `cc-skills-golang:golang-context` — propagate context through every call

- [ ] **Step 1: Write failing test for Register**

Create `internal/modules/identity/application/identity_service_test.go`:

```go
package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type fakeUserRepo struct{ mock.Mock }

func (f *fakeUserRepo) Insert(ctx context.Context, e, n string) (domain.User, error) {
	args := f.Called(ctx, e, n)
	return args.Get(0).(domain.User), args.Error(1)
}
func (f *fakeUserRepo) FindByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	args := f.Called(ctx, id)
	return args.Get(0).(domain.User), args.Error(1)
}
func (f *fakeUserRepo) FindByEmail(ctx context.Context, e string) (domain.User, error) {
	args := f.Called(ctx, e)
	return args.Get(0).(domain.User), args.Error(1)
}
func (f *fakeUserRepo) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	return f.Called(ctx, id).Error(0)
}
func (f *fakeUserRepo) UpdateName(ctx context.Context, id uuid.UUID, name string) (domain.User, error) {
	args := f.Called(ctx, id, name)
	return args.Get(0).(domain.User), args.Error(1)
}

type fakeAuthRepo struct{ mock.Mock }

func (f *fakeAuthRepo) InsertPassword(ctx context.Context, uid uuid.UUID, hash string) (domain.AuthMethod, error) {
	args := f.Called(ctx, uid, hash)
	return args.Get(0).(domain.AuthMethod), args.Error(1)
}
func (f *fakeAuthRepo) FindForUser(ctx context.Context, uid uuid.UUID, p domain.AuthProvider) (domain.AuthMethod, error) {
	args := f.Called(ctx, uid, p)
	return args.Get(0).(domain.AuthMethod), args.Error(1)
}
func (f *fakeAuthRepo) UpdatePassword(ctx context.Context, uid uuid.UUID, hash string) error {
	return f.Called(ctx, uid, hash).Error(0)
}
func (f *fakeAuthRepo) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	return f.Called(ctx, id).Error(0)
}

type fakeVerifyRepo struct{ mock.Mock }

func (f *fakeVerifyRepo) Insert(ctx context.Context, h []byte, uid uuid.UUID, e string, exp time.Time) error {
	return f.Called(ctx, h, uid, e, exp).Error(0)
}
func (f *fakeVerifyRepo) Find(ctx context.Context, h []byte) (domain.EmailVerifyToken, error) {
	args := f.Called(ctx, h)
	return args.Get(0).(domain.EmailVerifyToken), args.Error(1)
}
func (f *fakeVerifyRepo) Consume(ctx context.Context, h []byte) error {
	return f.Called(ctx, h).Error(0)
}

type fakeResetRepo struct{ mock.Mock }

func (f *fakeResetRepo) Insert(ctx context.Context, h []byte, uid uuid.UUID, exp time.Time) error {
	return f.Called(ctx, h, uid, exp).Error(0)
}
func (f *fakeResetRepo) Find(ctx context.Context, h []byte) (domain.PasswordResetToken, error) {
	args := f.Called(ctx, h)
	return args.Get(0).(domain.PasswordResetToken), args.Error(1)
}
func (f *fakeResetRepo) Consume(ctx context.Context, h []byte) error {
	return f.Called(ctx, h).Error(0)
}

type fakeSender struct {
	sent []email.Message
}

func (f *fakeSender) Send(_ context.Context, msg email.Message) error {
	f.sent = append(f.sent, msg)
	return nil
}

// --- tests ---

func TestRegister_HappyPath_CreatesUserPasswordTokenAndSendsEmail(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}
	verify := &fakeVerifyRepo{}
	reset := &fakeResetRepo{}
	sender := &fakeSender{}

	uid := uuid.New()
	users.On("Insert", mock.Anything, "ana@example.com", "Ana").
		Return(domain.User{ID: uid, Email: "ana@example.com", Name: "Ana", Status: domain.UserStatusActive}, nil)
	auths.On("InsertPassword", mock.Anything, uid, mock.AnythingOfType("string")).
		Return(domain.AuthMethod{}, nil)
	verify.On("Insert", mock.Anything, mock.AnythingOfType("[]uint8"), uid, "ana@example.com", mock.AnythingOfType("time.Time")).
		Return(nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users:                  users,
		AuthMethods:            auths,
		VerifyTokens:           verify,
		ResetTokens:            reset,
		Email:                  sender,
		VerifyLinkBaseURL:      "https://app.example/verify",
		ResetLinkBaseURL:       "https://app.example/reset",
		VerifyTokenTTL:         24 * time.Hour,
		ResetTokenTTL:          time.Hour,
		Now:                    func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) },
	})

	u, err := svc.Register(context.Background(), application.RegisterInput{
		Email:    "ana@example.com",
		Password: "S3cretPass!",
		Name:     "Ana",
	})
	require.NoError(t, err)
	assert.Equal(t, uid, u.ID)
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0].TextBody, "https://app.example/verify?token=")
	users.AssertExpectations(t)
	auths.AssertExpectations(t)
	verify.AssertExpectations(t)
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{},
		Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app.example/verify", ResetLinkBaseURL: "https://app.example/reset",
		Now: time.Now,
	})

	_, err := svc.Register(context.Background(), application.RegisterInput{
		Email: "ana@example.com", Password: "short", Name: "Ana",
	})
	require.ErrorIs(t, err, domain.ErrPasswordTooWeak)
}

func TestRegister_PropagatesEmailDuplicate(t *testing.T) {
	users := &fakeUserRepo{}
	users.On("Insert", mock.Anything, mock.Anything, mock.Anything).
		Return(domain.User{}, domain.ErrEmailAlreadyTaken)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{},
		Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app.example/verify", ResetLinkBaseURL: "https://app.example/reset",
		Now: time.Now,
	})

	_, err := svc.Register(context.Background(), application.RegisterInput{
		Email: "ana@example.com", Password: "S3cretPass!", Name: "Ana",
	})
	require.ErrorIs(t, err, domain.ErrEmailAlreadyTaken)
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/modules/identity/application/... -v
```

Expected: build error.

- [ ] **Step 3: Implement IdentityService skeleton + Register**

Create `internal/modules/identity/application/identity_service.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/danilloboing/marketplace-golang/internal/platform/passwords"
	"github.com/danilloboing/marketplace-golang/internal/platform/tokens"
	"github.com/google/uuid"
)

// IdentityServiceDeps lists every collaborator. All required.
type IdentityServiceDeps struct {
	Users        UserRepository
	AuthMethods  AuthMethodRepository
	VerifyTokens EmailVerifyTokenRepository
	ResetTokens  PasswordResetTokenRepository
	Email        email.Sender

	VerifyLinkBaseURL string
	ResetLinkBaseURL  string

	VerifyTokenTTL time.Duration
	ResetTokenTTL  time.Duration

	Now func() time.Time
}

// IdentityService orchestrates auth flows.
type IdentityService struct {
	deps IdentityServiceDeps
}

// NewIdentityService builds the service. Defaults: VerifyTokenTTL=24h, ResetTokenTTL=1h, Now=time.Now.
func NewIdentityService(d IdentityServiceDeps) *IdentityService {
	if d.VerifyTokenTTL == 0 {
		d.VerifyTokenTTL = 24 * time.Hour
	}
	if d.ResetTokenTTL == 0 {
		d.ResetTokenTTL = time.Hour
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &IdentityService{deps: d}
}

// RegisterInput is the request body for Register.
type RegisterInput struct {
	Email, Password, Name string
}

// Register creates a user, sets an initial password, issues a verify token,
// and sends the verify email. Returns the created user.
func (s *IdentityService) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	if err := validatePassword(in.Password); err != nil {
		return domain.User{}, err
	}
	if err := validateEmail(in.Email); err != nil {
		return domain.User{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return domain.User{}, fmt.Errorf("identity: %w: name required", errPolicyMisc)
	}

	user, err := s.deps.Users.Insert(ctx, in.Email, in.Name)
	if err != nil {
		// pass through ErrEmailAlreadyTaken
		return domain.User{}, err
	}

	hashedPwd, err := passwords.Hash(in.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: hash password: %w", err)
	}
	if _, err := s.deps.AuthMethods.InsertPassword(ctx, user.ID, hashedPwd); err != nil {
		return domain.User{}, fmt.Errorf("identity: insert auth method: %w", err)
	}

	rawToken, hash, err := tokens.Generate()
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: generate verify token: %w", err)
	}
	expires := s.deps.Now().Add(s.deps.VerifyTokenTTL).UTC()
	if err := s.deps.VerifyTokens.Insert(ctx, hash, user.ID, user.Email, expires); err != nil {
		return domain.User{}, fmt.Errorf("identity: store verify token: %w", err)
	}

	verifyURL := s.deps.VerifyLinkBaseURL + "?token=" + rawToken
	msg, err := email.RenderVerifyEmail(email.VerifyEmailData{
		ToAddress: user.Email,
		Name:      user.Name,
		VerifyURL: verifyURL,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: render verify email: %w", err)
	}
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return domain.User{}, fmt.Errorf("identity: send verify email: %w", err)
	}
	return user, nil
}

// errPolicyMisc is a fallback used to wrap policy violations not covered by sentinels.
var errPolicyMisc = errors.New("identity: policy violation")

func validatePassword(p string) error {
	if len(p) < 8 {
		return domain.ErrPasswordTooWeak
	}
	return nil
}

func validateEmail(e string) error {
	if !strings.Contains(e, "@") || !strings.Contains(e, ".") {
		return fmt.Errorf("identity: %w: invalid email", errPolicyMisc)
	}
	return nil
}

// uuid import marker (used by later methods); avoids "imported and not used" if Register is the only consumer.
var _ = uuid.Nil
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/application/... -v -run TestRegister
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/application/identity_service.go \
        internal/modules/identity/application/identity_service_test.go
git commit -m "feat(identity): add IdentityService.Register with verify email send"
```

---

## Task 17: `internal/modules/identity/application` — IdentityService.Login (constant-time defense)

**Files:**
- Modify: `internal/modules/identity/application/identity_service.go`
- Modify: `internal/modules/identity/application/identity_service_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-security` — timing attack defense via dummy verify
- `cc-skills-golang:golang-error-handling` — sentinel mapping for service-layer errors

- [ ] **Step 1: Write failing test**

Append to `identity_service_test.go`:

```go
func TestLogin_ReturnsInvalidCredentialsWhenUserMissing(t *testing.T) {
	users := &fakeUserRepo{}
	users.On("FindByEmail", mock.Anything, "missing@example.com").
		Return(domain.User{}, domain.ErrUserNotFound)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	_, err := svc.Login(context.Background(), application.LoginInput{
		Email: "missing@example.com", Password: "S3cretPass!",
	})
	require.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestLogin_ReturnsInvalidCredentialsWhenPasswordWrong(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}

	uid := uuid.New()
	users.On("FindByEmail", mock.Anything, "ana@example.com").
		Return(domain.User{
			ID: uid, Email: "ana@example.com", Name: "Ana",
			EmailVerifiedAt: ptrTimeNow(), Status: domain.UserStatusActive,
		}, nil)

	encoded, err := passwordsHash(t, "real-password")
	require.NoError(t, err)
	auths.On("FindForUser", mock.Anything, uid, domain.AuthProviderPassword).
		Return(domain.AuthMethod{ID: uuid.New(), UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &encoded}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	_, err = svc.Login(context.Background(), application.LoginInput{
		Email: "ana@example.com", Password: "wrong-password",
	})
	require.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestLogin_ReturnsEmailNotVerifiedWhenUnverified(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}
	uid := uuid.New()
	encoded, err := passwordsHash(t, "S3cretPass!")
	require.NoError(t, err)

	users.On("FindByEmail", mock.Anything, "ana@example.com").
		Return(domain.User{ID: uid, Email: "ana@example.com", Status: domain.UserStatusActive}, nil)
	auths.On("FindForUser", mock.Anything, uid, domain.AuthProviderPassword).
		Return(domain.AuthMethod{ID: uuid.New(), UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &encoded}, nil)
	auths.On("TouchLastUsed", mock.Anything, mock.Anything).Return(nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	_, err = svc.Login(context.Background(), application.LoginInput{
		Email: "ana@example.com", Password: "S3cretPass!",
	})
	require.ErrorIs(t, err, domain.ErrEmailNotVerified)
}

func TestLogin_HappyPathReturnsUser(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}
	uid := uuid.New()
	encoded, err := passwordsHash(t, "S3cretPass!")
	require.NoError(t, err)

	users.On("FindByEmail", mock.Anything, "ana@example.com").
		Return(domain.User{
			ID: uid, Email: "ana@example.com", Name: "Ana",
			EmailVerifiedAt: ptrTimeNow(), Status: domain.UserStatusActive,
		}, nil)
	auths.On("FindForUser", mock.Anything, uid, domain.AuthProviderPassword).
		Return(domain.AuthMethod{ID: uuid.New(), UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &encoded}, nil)
	auths.On("TouchLastUsed", mock.Anything, mock.Anything).Return(nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	u, err := svc.Login(context.Background(), application.LoginInput{
		Email: "ana@example.com", Password: "S3cretPass!",
	})
	require.NoError(t, err)
	assert.Equal(t, uid, u.ID)
}

func passwordsHash(t *testing.T, plain string) (string, error) {
	t.Helper()
	return passwordsHashFn(plain)
}

func ptrTimeNow() *time.Time {
	t := time.Now().UTC()
	return &t
}
```

Add the `passwordsHashFn` indirection to allow tests to import the real package:

```go
import (
	pw "github.com/danilloboing/marketplace-golang/internal/platform/passwords"
)

var passwordsHashFn = pw.Hash
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/modules/identity/application/... -v -run TestLogin
```

Expected: build error — `Login` and `LoginInput` undefined.

- [ ] **Step 3: Implement Login with timing defense**

Append to `identity_service.go`:

```go
// LoginInput is the login request payload.
type LoginInput struct {
	Email    string
	Password string
}

// Login validates credentials and returns the user.
// Returns ErrInvalidCredentials when email is unknown or password mismatches
// (with constant-time defense), and ErrEmailNotVerified only after a successful
// password match against an unverified user.
func (s *IdentityService) Login(ctx context.Context, in LoginInput) (domain.User, error) {
	user, err := s.deps.Users.FindByEmail(ctx, in.Email)
	if errors.Is(err, domain.ErrUserNotFound) {
		// Constant-time defense: pretend we have a hash to verify against, then fail.
		_, _ = passwords.Verify(in.Password, passwords.DummyHash)
		return domain.User{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: lookup user: %w", err)
	}

	auth, err := s.deps.AuthMethods.FindForUser(ctx, user.ID, domain.AuthProviderPassword)
	if errors.Is(err, domain.ErrUserNotFound) || auth.PasswordHash == nil {
		_, _ = passwords.Verify(in.Password, passwords.DummyHash)
		return domain.User{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: lookup auth method: %w", err)
	}

	ok, err := passwords.Verify(in.Password, *auth.PasswordHash)
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: verify password: %w", err)
	}
	if !ok {
		return domain.User{}, domain.ErrInvalidCredentials
	}

	// Best-effort last-used touch; do not fail login on this.
	_ = s.deps.AuthMethods.TouchLastUsed(ctx, auth.ID)

	if !user.IsEmailVerified() {
		return domain.User{}, domain.ErrEmailNotVerified
	}
	if !user.IsActive() {
		return domain.User{}, domain.ErrInvalidCredentials
	}

	return user, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/application/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/application/identity_service.go \
        internal/modules/identity/application/identity_service_test.go
git commit -m "feat(identity): add IdentityService.Login with constant-time defense"
```

---

## Task 18: `internal/modules/identity/application` — VerifyEmail + ResendVerifyEmail

**Files:**
- Modify: `internal/modules/identity/application/identity_service.go`
- Modify: `internal/modules/identity/application/identity_service_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-error-handling` — sentinel reuse, single handling rule

- [ ] **Step 1: Write failing tests**

Append to `identity_service_test.go`:

```go
func TestVerifyEmail_HappyPath(t *testing.T) {
	users := &fakeUserRepo{}
	verify := &fakeVerifyRepo{}
	uid := uuid.New()

	rawHash := []byte("hashbytes------------------------")
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	verify.On("Find", mock.Anything, mock.AnythingOfType("[]uint8")).
		Return(domain.EmailVerifyToken{
			TokenHash: rawHash, UserID: uid, Email: "ana@example.com",
			ExpiresAt: now.Add(time.Hour),
		}, nil)
	verify.On("Consume", mock.Anything, mock.AnythingOfType("[]uint8")).Return(nil)
	users.On("MarkEmailVerified", mock.Anything, uid).Return(nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{}, VerifyTokens: verify, ResetTokens: &fakeResetRepo{},
		Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: func() time.Time { return now },
	})

	rawToken, _, _ := tokensGen()
	require.NoError(t, svc.VerifyEmail(context.Background(), rawToken))
}

func TestVerifyEmail_RejectsExpired(t *testing.T) {
	verify := &fakeVerifyRepo{}
	uid := uuid.New()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	verify.On("Find", mock.Anything, mock.AnythingOfType("[]uint8")).
		Return(domain.EmailVerifyToken{
			UserID: uid, Email: "ana@example.com",
			ExpiresAt: now.Add(-time.Minute),
		}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: &fakeAuthRepo{}, VerifyTokens: verify, ResetTokens: &fakeResetRepo{},
		Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: func() time.Time { return now },
	})

	rawToken, _, _ := tokensGen()
	err := svc.VerifyEmail(context.Background(), rawToken)
	require.ErrorIs(t, err, domain.ErrTokenExpired)
}

func TestVerifyEmail_RejectsConsumed(t *testing.T) {
	verify := &fakeVerifyRepo{}
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	consumed := now.Add(-time.Minute)

	verify.On("Find", mock.Anything, mock.AnythingOfType("[]uint8")).
		Return(domain.EmailVerifyToken{
			UserID: uuid.New(), Email: "ana@example.com",
			ExpiresAt: now.Add(time.Hour), ConsumedAt: &consumed,
		}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: &fakeAuthRepo{}, VerifyTokens: verify, ResetTokens: &fakeResetRepo{},
		Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: func() time.Time { return now },
	})

	rawToken, _, _ := tokensGen()
	err := svc.VerifyEmail(context.Background(), rawToken)
	require.ErrorIs(t, err, domain.ErrTokenAlreadyUsed)
}

func TestResendVerifyEmail_AlwaysReturnsNilEvenWhenEmailUnknown(t *testing.T) {
	users := &fakeUserRepo{}
	users.On("FindByEmail", mock.Anything, "noone@example.com").
		Return(domain.User{}, domain.ErrUserNotFound)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	require.NoError(t, svc.ResendVerifyEmail(context.Background(), "noone@example.com"))
}

func TestResendVerifyEmail_SkipsWhenAlreadyVerified(t *testing.T) {
	users := &fakeUserRepo{}
	uid := uuid.New()
	users.On("FindByEmail", mock.Anything, "ana@example.com").
		Return(domain.User{ID: uid, Email: "ana@example.com", EmailVerifiedAt: ptrTimeNow()}, nil)

	verify := &fakeVerifyRepo{}
	sender := &fakeSender{}

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: verify, ResetTokens: &fakeResetRepo{}, Email: sender,
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	require.NoError(t, svc.ResendVerifyEmail(context.Background(), "ana@example.com"))
	assert.Empty(t, sender.sent)
	verify.AssertNotCalled(t, "Insert")
}

// helper to generate a real token + hash (for tests that need both ends).
func tokensGen() (string, []byte, error) {
	return tokensGenFn()
}

var tokensGenFn = func() (string, []byte, error) {
	return tokensGenerate()
}
```

Add this import + helper at the top of the test file:

```go
import (
	tk "github.com/danilloboing/marketplace-golang/internal/platform/tokens"
)

var tokensGenerate = tk.Generate
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/modules/identity/application/... -v -run "TestVerifyEmail|TestResendVerifyEmail"
```

Expected: build error.

- [ ] **Step 3: Implement VerifyEmail and ResendVerifyEmail**

Append to `identity_service.go`:

```go
// VerifyEmail consumes a verify token and marks the user's email as verified.
//
// Returns ErrTokenNotFound, ErrTokenAlreadyUsed, or ErrTokenExpired for invalid
// tokens. Idempotent for already-consumed tokens belonging to verified users —
// they map to ErrTokenAlreadyUsed (transport returns 400 invalid_token, which is
// the correct privacy-preserving behaviour: don't tell callers whose mailbox
// it was).
func (s *IdentityService) VerifyEmail(ctx context.Context, rawToken string) error {
	hash, err := tokens.Hash(rawToken)
	if err != nil {
		return domain.ErrTokenNotFound
	}
	tok, err := s.deps.VerifyTokens.Find(ctx, hash)
	if errors.Is(err, domain.ErrTokenNotFound) {
		return domain.ErrTokenNotFound
	}
	if err != nil {
		return fmt.Errorf("identity: find verify token: %w", err)
	}
	if tok.IsConsumed() {
		return domain.ErrTokenAlreadyUsed
	}
	if tok.IsExpired(s.deps.Now()) {
		return domain.ErrTokenExpired
	}

	if err := s.deps.Users.MarkEmailVerified(ctx, tok.UserID); err != nil {
		return fmt.Errorf("identity: mark verified: %w", err)
	}
	if err := s.deps.VerifyTokens.Consume(ctx, hash); err != nil {
		return fmt.Errorf("identity: consume verify token: %w", err)
	}
	return nil
}

// ResendVerifyEmail issues a new verify token and sends a fresh email.
// Always returns nil even when the email does not match a user (anti-enumeration).
func (s *IdentityService) ResendVerifyEmail(ctx context.Context, addr string) error {
	user, err := s.deps.Users.FindByEmail(ctx, addr)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity: lookup user: %w", err)
	}
	if user.IsEmailVerified() {
		// Already verified; do not send again.
		return nil
	}
	rawToken, hash, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("identity: generate verify token: %w", err)
	}
	expires := s.deps.Now().Add(s.deps.VerifyTokenTTL).UTC()
	if err := s.deps.VerifyTokens.Insert(ctx, hash, user.ID, user.Email, expires); err != nil {
		return fmt.Errorf("identity: store verify token: %w", err)
	}
	verifyURL := s.deps.VerifyLinkBaseURL + "?token=" + rawToken
	msg, err := email.RenderVerifyEmail(email.VerifyEmailData{
		ToAddress: user.Email, Name: user.Name, VerifyURL: verifyURL,
	})
	if err != nil {
		return fmt.Errorf("identity: render verify email: %w", err)
	}
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return fmt.Errorf("identity: send verify email: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/application/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/application/identity_service.go \
        internal/modules/identity/application/identity_service_test.go
git commit -m "feat(identity): add VerifyEmail and ResendVerifyEmail with anti-enumeration"
```

---

## Task 19: `internal/modules/identity/application` — RequestPasswordReset + ConfirmPasswordReset

**Files:**
- Modify: `internal/modules/identity/application/identity_service.go`
- Modify: `internal/modules/identity/application/identity_service_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-security` — anti-enumeration, password storage
- `cc-skills-golang:golang-error-handling` — sentinel reuse

- [ ] **Step 1: Write failing tests**

Append to `identity_service_test.go`:

```go
func TestRequestPasswordReset_AlwaysSucceeds(t *testing.T) {
	users := &fakeUserRepo{}
	users.On("FindByEmail", mock.Anything, "missing@example.com").
		Return(domain.User{}, domain.ErrUserNotFound)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	require.NoError(t, svc.RequestPasswordReset(context.Background(), "missing@example.com"))
}

func TestRequestPasswordReset_HappyPathSendsEmail(t *testing.T) {
	users := &fakeUserRepo{}
	reset := &fakeResetRepo{}
	sender := &fakeSender{}
	uid := uuid.New()

	users.On("FindByEmail", mock.Anything, "ana@example.com").
		Return(domain.User{ID: uid, Email: "ana@example.com", Name: "Ana", EmailVerifiedAt: ptrTimeNow()}, nil)
	reset.On("Insert", mock.Anything, mock.AnythingOfType("[]uint8"), uid, mock.AnythingOfType("time.Time")).
		Return(nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: reset, Email: sender,
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	require.NoError(t, svc.RequestPasswordReset(context.Background(), "ana@example.com"))
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0].TextBody, "https://app/reset?token=")
}

func TestConfirmPasswordReset_HappyPathRevokesAndUpdates(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}
	reset := &fakeResetRepo{}
	uid := uuid.New()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	reset.On("Find", mock.Anything, mock.AnythingOfType("[]uint8")).
		Return(domain.PasswordResetToken{UserID: uid, ExpiresAt: now.Add(time.Hour)}, nil)
	auths.On("UpdatePassword", mock.Anything, uid, mock.AnythingOfType("string")).Return(nil)
	reset.On("Consume", mock.Anything, mock.AnythingOfType("[]uint8")).Return(nil)

	revoked := 0
	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: reset, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now:               func() time.Time { return now },
		RevokeAllSessions: func(_ context.Context, _ uuid.UUID) error { revoked++; return nil },
	})

	rawToken, _, _ := tokensGen()
	require.NoError(t, svc.ConfirmPasswordReset(context.Background(), rawToken, "NewS3cret!"))
	assert.Equal(t, 1, revoked)
}

func TestConfirmPasswordReset_RejectsExpired(t *testing.T) {
	reset := &fakeResetRepo{}
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	reset.On("Find", mock.Anything, mock.AnythingOfType("[]uint8")).
		Return(domain.PasswordResetToken{ExpiresAt: now.Add(-time.Minute)}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: reset, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: func() time.Time { return now },
	})

	rawToken, _, _ := tokensGen()
	err := svc.ConfirmPasswordReset(context.Background(), rawToken, "NewS3cret!")
	require.ErrorIs(t, err, domain.ErrTokenExpired)
}

func TestConfirmPasswordReset_RejectsWeakPassword(t *testing.T) {
	reset := &fakeResetRepo{}
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	reset.On("Find", mock.Anything, mock.AnythingOfType("[]uint8")).
		Return(domain.PasswordResetToken{UserID: uuid.New(), ExpiresAt: now.Add(time.Hour)}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: reset, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: func() time.Time { return now },
	})

	rawToken, _, _ := tokensGen()
	err := svc.ConfirmPasswordReset(context.Background(), rawToken, "short")
	require.ErrorIs(t, err, domain.ErrPasswordTooWeak)
}
```

- [ ] **Step 2: Add RevokeAllSessions hook to deps**

Modify the `IdentityServiceDeps` struct in `identity_service.go`:

```go
// RevokeAllSessions is called by the service to invalidate every active
// session for a user (password reset / change-password full revoke). The
// caller wires this to sessionauth.Manager.DeleteAllForUser.
RevokeAllSessions func(ctx context.Context, userID uuid.UUID) error

// RevokeAllSessionsExcept is the variant that keeps a single session id
// (used by ChangePassword from a logged-in browser). Wired to
// sessionauth.Manager.DeleteAllForUserExcept.
RevokeAllSessionsExcept func(ctx context.Context, userID uuid.UUID, keepID string) error
```

In `NewIdentityService`, default both to no-ops:

```go
if d.RevokeAllSessions == nil {
	d.RevokeAllSessions = func(context.Context, uuid.UUID) error { return nil }
}
if d.RevokeAllSessionsExcept == nil {
	d.RevokeAllSessionsExcept = func(context.Context, uuid.UUID, string) error { return nil }
}
```

- [ ] **Step 3: Implement RequestPasswordReset and ConfirmPasswordReset**

Append to `identity_service.go`:

```go
// RequestPasswordReset issues a reset token + sends an email. Always returns nil
// regardless of whether the email matches a user (anti-enumeration).
func (s *IdentityService) RequestPasswordReset(ctx context.Context, addr string) error {
	user, err := s.deps.Users.FindByEmail(ctx, addr)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity: lookup user: %w", err)
	}
	if !user.IsEmailVerified() {
		// We do not allow resetting an unverified account; do not leak that fact.
		return nil
	}
	rawToken, hash, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("identity: generate reset token: %w", err)
	}
	expires := s.deps.Now().Add(s.deps.ResetTokenTTL).UTC()
	if err := s.deps.ResetTokens.Insert(ctx, hash, user.ID, expires); err != nil {
		return fmt.Errorf("identity: store reset token: %w", err)
	}
	resetURL := s.deps.ResetLinkBaseURL + "?token=" + rawToken
	msg, err := email.RenderPasswordResetEmail(email.PasswordResetEmailData{
		ToAddress: user.Email, Name: user.Name,
		ResetURL: resetURL, ExpiryMin: int(s.deps.ResetTokenTTL / time.Minute),
	})
	if err != nil {
		return fmt.Errorf("identity: render reset email: %w", err)
	}
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return fmt.Errorf("identity: send reset email: %w", err)
	}
	return nil
}

// ConfirmPasswordReset consumes a reset token, sets a new password hash,
// and revokes ALL sessions for the user.
func (s *IdentityService) ConfirmPasswordReset(ctx context.Context, rawToken, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	hash, err := tokens.Hash(rawToken)
	if err != nil {
		return domain.ErrTokenNotFound
	}
	tok, err := s.deps.ResetTokens.Find(ctx, hash)
	if errors.Is(err, domain.ErrTokenNotFound) {
		return domain.ErrTokenNotFound
	}
	if err != nil {
		return fmt.Errorf("identity: find reset token: %w", err)
	}
	if tok.IsConsumed() {
		return domain.ErrTokenAlreadyUsed
	}
	if tok.IsExpired(s.deps.Now()) {
		return domain.ErrTokenExpired
	}
	encoded, err := passwords.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("identity: hash new password: %w", err)
	}
	if err := s.deps.AuthMethods.UpdatePassword(ctx, tok.UserID, encoded); err != nil {
		return fmt.Errorf("identity: update password: %w", err)
	}
	if err := s.deps.ResetTokens.Consume(ctx, hash); err != nil {
		return fmt.Errorf("identity: consume reset token: %w", err)
	}
	if err := s.deps.RevokeAllSessions(ctx, tok.UserID); err != nil {
		return fmt.Errorf("identity: revoke sessions: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/application/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/application/identity_service.go \
        internal/modules/identity/application/identity_service_test.go
git commit -m "feat(identity): add RequestPasswordReset and ConfirmPasswordReset"
```

---

## Task 20: `internal/modules/identity/application` — ChangePassword, GetMe, UpdateProfile

**Files:**
- Modify: `internal/modules/identity/application/identity_service.go`
- Modify: `internal/modules/identity/application/identity_service_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-error-handling` — sentinel mapping

- [ ] **Step 1: Write failing tests**

Append to `identity_service_test.go`:

```go
func TestChangePassword_HappyPathRevokesOtherSessions(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}
	uid := uuid.New()
	encoded, err := passwordsHash(t, "S3cretPass!")
	require.NoError(t, err)

	auths.On("FindForUser", mock.Anything, uid, domain.AuthProviderPassword).
		Return(domain.AuthMethod{UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &encoded}, nil)
	auths.On("UpdatePassword", mock.Anything, uid, mock.AnythingOfType("string")).Return(nil)

	revokedExcept := ""
	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
		RevokeAllSessionsExcept: func(_ context.Context, _ uuid.UUID, keep string) error {
			revokedExcept = keep
			return nil
		},
	})

	require.NoError(t, svc.ChangePassword(context.Background(), application.ChangePasswordInput{
		UserID: uid, CurrentPassword: "S3cretPass!", NewPassword: "NewS3cret!",
		KeepSessionID: "keep-me",
	}))
	assert.Equal(t, "keep-me", revokedExcept)
}

func TestChangePassword_RejectsWrongCurrent(t *testing.T) {
	auths := &fakeAuthRepo{}
	uid := uuid.New()
	encoded, err := passwordsHash(t, "S3cretPass!")
	require.NoError(t, err)

	auths.On("FindForUser", mock.Anything, uid, domain.AuthProviderPassword).
		Return(domain.AuthMethod{UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &encoded}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: auths,
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})
	err = svc.ChangePassword(context.Background(), application.ChangePasswordInput{
		UserID: uid, CurrentPassword: "wrong", NewPassword: "NewS3cret!",
	})
	require.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestGetMe_ReturnsUser(t *testing.T) {
	users := &fakeUserRepo{}
	uid := uuid.New()
	users.On("FindByID", mock.Anything, uid).
		Return(domain.User{ID: uid, Email: "ana@example.com", Name: "Ana"}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	got, err := svc.GetMe(context.Background(), uid)
	require.NoError(t, err)
	assert.Equal(t, "ana@example.com", got.Email)
}

func TestUpdateProfile_UpdatesNameOnly(t *testing.T) {
	users := &fakeUserRepo{}
	uid := uuid.New()
	users.On("UpdateName", mock.Anything, uid, "Ana Lima").
		Return(domain.User{ID: uid, Name: "Ana Lima"}, nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{}, Email: &fakeSender{},
		VerifyLinkBaseURL: "https://app/verify", ResetLinkBaseURL: "https://app/reset",
		Now: time.Now,
	})

	got, err := svc.UpdateProfile(context.Background(), application.UpdateProfileInput{
		UserID: uid, Name: "Ana Lima",
	})
	require.NoError(t, err)
	assert.Equal(t, "Ana Lima", got.Name)
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./internal/modules/identity/application/... -v -run "TestChangePassword|TestGetMe|TestUpdateProfile"
```

Expected: build error.

- [ ] **Step 3: Implement methods**

Append to `identity_service.go`:

```go
// ChangePasswordInput carries change-password parameters.
type ChangePasswordInput struct {
	UserID          uuid.UUID
	CurrentPassword string
	NewPassword     string
	KeepSessionID   string // current session id; revoked-except pivot
}

// ChangePassword verifies the current password, sets the new one, and revokes
// every session for this user EXCEPT KeepSessionID.
func (s *IdentityService) ChangePassword(ctx context.Context, in ChangePasswordInput) error {
	if err := validatePassword(in.NewPassword); err != nil {
		return err
	}
	auth, err := s.deps.AuthMethods.FindForUser(ctx, in.UserID, domain.AuthProviderPassword)
	if errors.Is(err, domain.ErrUserNotFound) || auth.PasswordHash == nil {
		return domain.ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("identity: lookup auth method: %w", err)
	}

	ok, err := passwords.Verify(in.CurrentPassword, *auth.PasswordHash)
	if err != nil {
		return fmt.Errorf("identity: verify current password: %w", err)
	}
	if !ok {
		return domain.ErrInvalidCredentials
	}

	encoded, err := passwords.Hash(in.NewPassword)
	if err != nil {
		return fmt.Errorf("identity: hash new password: %w", err)
	}
	if err := s.deps.AuthMethods.UpdatePassword(ctx, in.UserID, encoded); err != nil {
		return fmt.Errorf("identity: update password: %w", err)
	}
	if err := s.deps.RevokeAllSessionsExcept(ctx, in.UserID, in.KeepSessionID); err != nil {
		return fmt.Errorf("identity: revoke other sessions: %w", err)
	}
	return nil
}

// GetMe returns the current user.
func (s *IdentityService) GetMe(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	return s.deps.Users.FindByID(ctx, userID)
}

// UpdateProfileInput accepts editable fields.
type UpdateProfileInput struct {
	UserID uuid.UUID
	Name   string
}

// UpdateProfile updates the user's name.
func (s *IdentityService) UpdateProfile(ctx context.Context, in UpdateProfileInput) (domain.User, error) {
	if strings.TrimSpace(in.Name) == "" {
		return domain.User{}, fmt.Errorf("identity: %w: name required", errPolicyMisc)
	}
	return s.deps.Users.UpdateName(ctx, in.UserID, in.Name)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/application/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/application/identity_service.go \
        internal/modules/identity/application/identity_service_test.go
git commit -m "feat(identity): add ChangePassword, GetMe, and UpdateProfile"
```

---

## Task 21: `internal/modules/identity/transport` — auth_handlers + error mapping + responses

**Files:**
- Create: `internal/modules/identity/transport/responses.go`
- Create: `internal/modules/identity/transport/error_mapping.go`
- Create: `internal/modules/identity/transport/auth_handlers.go`
- Create: `internal/modules/identity/transport/auth_handlers_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — handler composition with helper for decode+validate+respond
- `cc-skills-golang:golang-error-handling` — single handling rule at boundary
- `cc-skills-golang:golang-naming` — handler method names

- [ ] **Step 1: Define response DTOs and error mapping**

Create `internal/modules/identity/transport/responses.go`:

```go
// Package transport contains HTTP handlers for the identity module.
package transport

import (
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/google/uuid"
)

// UserResponse is the JSON shape of a user returned to the client.
type UserResponse struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
}

func userResponse(u domain.User) UserResponse {
	return UserResponse{
		ID: u.ID, Email: u.Email, Name: u.Name,
		EmailVerifiedAt: u.EmailVerifiedAt,
	}
}
```

Create `internal/modules/identity/transport/error_mapping.go`:

```go
package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
)

// mapErrorToHTTP returns (status, code, userMessage) for a service error.
// Internal errors collapse to 500 + internal_error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrSessionExpired),
		errors.Is(err, domain.ErrSessionNotFound):
		return http.StatusUnauthorized, "invalid_credentials", "invalid credentials"
	case errors.Is(err, domain.ErrEmailNotVerified):
		return http.StatusForbidden, "email_not_verified", "email verification required"
	case errors.Is(err, domain.ErrEmailAlreadyTaken):
		return http.StatusConflict, "email_already_taken", "email already taken"
	case errors.Is(err, domain.ErrTokenExpired),
		errors.Is(err, domain.ErrTokenAlreadyUsed),
		errors.Is(err, domain.ErrTokenNotFound):
		return http.StatusBadRequest, "invalid_token", "invalid token"
	case errors.Is(err, domain.ErrPasswordTooWeak):
		return http.StatusUnprocessableEntity, "password_policy", "password does not meet policy"
	case errors.Is(err, domain.ErrUserNotFound):
		return http.StatusNotFound, "not_found", "user not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
```

- [ ] **Step 2: Implement auth handlers**

Create `internal/modules/identity/transport/auth_handlers.go`:

```go
package transport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
)

// AuthHandlers handles unauthenticated identity endpoints.
type AuthHandlers struct {
	svc      *application.IdentityService
	sessions sessionauth.Manager
	cookies  CookieConfig
}

// CookieConfig controls cookie names and flags written by handlers.
type CookieConfig struct {
	SessionName  string
	CSRFName     string
	SecurePrefix bool
	Domain       string
}

// NewAuthHandlers builds AuthHandlers.
func NewAuthHandlers(svc *application.IdentityService, sessions sessionauth.Manager, cookies CookieConfig) *AuthHandlers {
	return &AuthHandlers{svc: svc, sessions: sessions, cookies: cookies}
}

// RegisterAuthRoutes wires routes onto r.
func (h *AuthHandlers) RegisterAuthRoutes(r chi.Router) {
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/verify-email", h.VerifyEmail)
	r.Post("/auth/verify-email/resend", h.ResendVerifyEmail)
	r.Post("/auth/password-reset/request", h.RequestPasswordReset)
	r.Post("/auth/password-reset/confirm", h.ConfirmPasswordReset)
	r.Get("/auth/csrf", h.CSRF)
}

// --- handlers ---

type registerInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var in registerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	user, err := h.svc.Register(r.Context(), application.RegisterInput{
		Email: strings.TrimSpace(in.Email), Password: in.Password, Name: strings.TrimSpace(in.Name),
	})
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusCreated, userResponse(user))
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember,omitempty"`
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	user, err := h.svc.Login(r.Context(), application.LoginInput{
		Email: strings.TrimSpace(in.Email), Password: in.Password,
	})
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	sess, err := h.sessions.Create(r.Context(), sessionauth.CreateParams{
		UserID:     user.ID,
		RememberMe: in.Remember,
		UserAgent:  r.UserAgent(),
		IP:         r.RemoteAddr,
	})
	if err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "session creation failed", err)
		return
	}
	h.setSessionCookies(w, sess)
	responsex.JSON(w, http.StatusOK, userResponse(user))
}

type verifyEmailInput struct {
	Token string `json:"token"`
}

func (h *AuthHandlers) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in verifyEmailInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), in.Token); err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type resendVerifyInput struct {
	Email string `json:"email"`
}

func (h *AuthHandlers) ResendVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in resendVerifyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		// even on bad payload, return 202 to preserve anti-enumeration. Log internally.
		responsex.ErrorWithCause(w, r, http.StatusAccepted, "accepted", "accepted", err)
		return
	}
	// Best-effort send; service swallows unknown emails.
	_ = h.svc.ResendVerifyEmail(r.Context(), strings.TrimSpace(in.Email))
	w.WriteHeader(http.StatusAccepted)
}

type passwordResetRequestInput struct {
	Email string `json:"email"`
}

func (h *AuthHandlers) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in passwordResetRequestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	_ = h.svc.RequestPasswordReset(r.Context(), strings.TrimSpace(in.Email))
	w.WriteHeader(http.StatusAccepted)
}

type passwordResetConfirmInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandlers) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in passwordResetConfirmInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if err := h.svc.ConfirmPasswordReset(r.Context(), in.Token, in.NewPassword); err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// CSRF returns a fresh CSRF token in a cookie. Used by SPA bootstrap before login.
// The cookie value is a random 32-byte hex; not bound to a session because there
// isn't one yet. Once the user logs in, the session-bound csrf_token replaces it.
func (h *AuthHandlers) CSRF(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "csrf gen failed", err)
		return
	}
	value := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.CSRFName,
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Hour.Seconds()),
	})
	responsex.JSON(w, http.StatusOK, map[string]string{"csrf_token": value})
}

// setSessionCookies writes the session_id and csrf_token cookies.
func (h *AuthHandlers) setSessionCookies(w http.ResponseWriter, s sessionauth.Session) {
	maxAge := int(time.Until(s.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.SessionName,
		Value:    s.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.CSRFName,
		Value:    s.CSRFToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// Logout deletes the current session and clears cookies. Mounted on the
// authenticated branch (handler signature lives here for cohesion).
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	if err := h.sessions.Delete(r.Context(), sess.ID); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "logout failed", err)
		return
	}
	clearCookie(w, h.cookies.SessionName)
	clearCookie(w, h.cookies.CSRFName)
	w.WriteHeader(http.StatusNoContent)
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// helper: keep the linter happy if uuid/errors are not yet referenced in early skeletons.
var _ = uuid.Nil
var _ = errors.Is
```

- [ ] **Step 3: Write failing handler tests**

Create `internal/modules/identity/transport/auth_handlers_test.go`:

```go
package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakes shared with application tests would normally live in a testutil pkg.
// For brevity, redeclare minimal in-memory fakes here.

type memUserRepo struct {
	users map[string]domain.User
}

func newMemUserRepo() *memUserRepo { return &memUserRepo{users: map[string]domain.User{}} }
func (r *memUserRepo) Insert(_ context.Context, e, n string) (domain.User, error) {
	if _, ok := r.users[strings.ToLower(e)]; ok {
		return domain.User{}, domain.ErrEmailAlreadyTaken
	}
	u := domain.User{ID: uuid.New(), Email: e, Name: n, Status: domain.UserStatusActive}
	r.users[strings.ToLower(e)] = u
	return u, nil
}
func (r *memUserRepo) FindByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	for _, u := range r.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (r *memUserRepo) FindByEmail(_ context.Context, e string) (domain.User, error) {
	if u, ok := r.users[strings.ToLower(e)]; ok {
		return u, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (r *memUserRepo) MarkEmailVerified(_ context.Context, id uuid.UUID) error {
	for k, u := range r.users {
		if u.ID == id {
			now := time.Now().UTC()
			u.EmailVerifiedAt = &now
			r.users[k] = u
		}
	}
	return nil
}
func (r *memUserRepo) UpdateName(_ context.Context, id uuid.UUID, name string) (domain.User, error) {
	for k, u := range r.users {
		if u.ID == id {
			u.Name = name
			r.users[k] = u
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

type memAuthRepo struct {
	byUser map[uuid.UUID]domain.AuthMethod
}

func newMemAuthRepo() *memAuthRepo { return &memAuthRepo{byUser: map[uuid.UUID]domain.AuthMethod{}} }
func (r *memAuthRepo) InsertPassword(_ context.Context, uid uuid.UUID, hash string) (domain.AuthMethod, error) {
	am := domain.AuthMethod{ID: uuid.New(), UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &hash}
	r.byUser[uid] = am
	return am, nil
}
func (r *memAuthRepo) FindForUser(_ context.Context, uid uuid.UUID, _ domain.AuthProvider) (domain.AuthMethod, error) {
	am, ok := r.byUser[uid]
	if !ok {
		return domain.AuthMethod{}, domain.ErrUserNotFound
	}
	return am, nil
}
func (r *memAuthRepo) UpdatePassword(_ context.Context, uid uuid.UUID, hash string) error {
	am := r.byUser[uid]
	am.PasswordHash = &hash
	r.byUser[uid] = am
	return nil
}
func (r *memAuthRepo) TouchLastUsed(_ context.Context, _ uuid.UUID) error { return nil }

type memVerifyRepo struct {
	byHash map[string]domain.EmailVerifyToken
}

func newMemVerifyRepo() *memVerifyRepo { return &memVerifyRepo{byHash: map[string]domain.EmailVerifyToken{}} }
func (r *memVerifyRepo) Insert(_ context.Context, h []byte, uid uuid.UUID, e string, exp time.Time) error {
	r.byHash[string(h)] = domain.EmailVerifyToken{
		TokenHash: h, UserID: uid, Email: e, ExpiresAt: exp,
	}
	return nil
}
func (r *memVerifyRepo) Find(_ context.Context, h []byte) (domain.EmailVerifyToken, error) {
	t, ok := r.byHash[string(h)]
	if !ok {
		return domain.EmailVerifyToken{}, domain.ErrTokenNotFound
	}
	return t, nil
}
func (r *memVerifyRepo) Consume(_ context.Context, h []byte) error {
	t, ok := r.byHash[string(h)]
	if !ok {
		return domain.ErrTokenNotFound
	}
	now := time.Now().UTC()
	t.ConsumedAt = &now
	r.byHash[string(h)] = t
	return nil
}

type memResetRepo struct{}

func (memResetRepo) Insert(context.Context, []byte, uuid.UUID, time.Time) error {
	return nil
}
func (memResetRepo) Find(context.Context, []byte) (domain.PasswordResetToken, error) {
	return domain.PasswordResetToken{}, domain.ErrTokenNotFound
}
func (memResetRepo) Consume(context.Context, []byte) error { return nil }

type captureSender struct{ msgs []email.Message }

func (s *captureSender) Send(_ context.Context, m email.Message) error {
	s.msgs = append(s.msgs, m)
	return nil
}

type stubSessions struct {
	created sessionauth.Session
	deleted []string
}

func (s *stubSessions) Create(_ context.Context, _ sessionauth.CreateParams) (sessionauth.Session, error) {
	s.created = sessionauth.Session{
		ID: "sid", UserID: uuid.New(), CSRFToken: "ct",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	return s.created, nil
}
func (s *stubSessions) Get(context.Context, string) (sessionauth.Session, error) {
	return sessionauth.Session{}, sessionauth.ErrNotFound
}
func (s *stubSessions) Refresh(context.Context, string) error { return nil }
func (s *stubSessions) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubSessions) DeleteAllForUser(context.Context, uuid.UUID) error             { return nil }
func (s *stubSessions) DeleteAllForUserExcept(context.Context, uuid.UUID, string) error { return nil }

func newSvc(users application.UserRepository, auths application.AuthMethodRepository, verify application.EmailVerifyTokenRepository, reset application.PasswordResetTokenRepository, sender email.Sender) *application.IdentityService {
	return application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: verify, ResetTokens: reset, Email: sender,
		VerifyLinkBaseURL: "https://app/verify",
		ResetLinkBaseURL:  "https://app/reset",
		Now:               time.Now,
	})
}

func TestRegisterHandler_HappyPath(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)

	h := transport.NewAuthHandlers(svc, &stubSessions{}, transport.CookieConfig{
		SessionName: "session_id", CSRFName: "csrf_token",
	})
	r := chi.NewRouter()
	h.RegisterAuthRoutes(r)

	body, _ := json.Marshal(map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!", "name": "Ana",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, sender.msgs, 1)
	assert.Contains(t, sender.msgs[0].TextBody, "https://app/verify?token=")
}

func TestLoginHandler_SetsSessionCookies(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	// Bootstrap user via real Register flow to keep password hashes in sync.
	h := transport.NewAuthHandlers(svc, sessions, transport.CookieConfig{
		SessionName: "session_id", CSRFName: "csrf_token",
	})
	r := chi.NewRouter()
	h.RegisterAuthRoutes(r)

	body, _ := json.Marshal(map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!", "name": "Ana",
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(string(body)))
	registerReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), registerReq)

	// Verify email out-of-band — flip via mem repo to keep test focused on cookies.
	for _, u := range users.users {
		require.NoError(t, users.MarkEmailVerified(context.Background(), u.ID))
	}

	loginBody, _ := json.Marshal(map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	cookies := rec.Result().Cookies()
	var session, csrf *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "session_id":
			session = c
		case "csrf_token":
			csrf = c
		}
	}
	require.NotNil(t, session)
	require.NotNil(t, csrf)
	assert.Equal(t, "sid", session.Value)
	assert.True(t, session.HttpOnly)
	assert.False(t, csrf.HttpOnly)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/modules/identity/transport/... -v
```

Expected: PASS for handler tests.

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/transport/responses.go \
        internal/modules/identity/transport/error_mapping.go \
        internal/modules/identity/transport/auth_handlers.go \
        internal/modules/identity/transport/auth_handlers_test.go
git commit -m "feat(identity): add auth handlers (register/login/verify/reset/csrf/logout) with error mapping"
```

---

## Task 22: `internal/modules/identity/transport` — me_handlers (GET/PATCH /me, change-password, sessions/all)

**Files:**
- Create: `internal/modules/identity/transport/me_handlers.go`
- Create: `internal/modules/identity/transport/me_handlers_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-naming` — handler method names
- `cc-skills-golang:golang-error-handling` — single handling rule

- [ ] **Step 1: Write failing test**

Create `internal/modules/identity/transport/me_handlers_test.go`:

```go
package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withSession(s sessionauth.Session) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sessionauth.ContextWithSession(r.Context(), s)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestMeHandler_GetMeReturnsCurrentUser(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	// Seed a verified user.
	u, err := users.Insert(t.Context(), "ana@example.com", "Ana")
	require.NoError(t, err)
	require.NoError(t, users.MarkEmailVerified(t.Context(), u.ID))

	h := transport.NewMeHandlers(svc, sessions, transport.CookieConfig{
		SessionName: "session_id", CSRFName: "csrf_token",
	})
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: u.ID, CSRFToken: "ct"}))
		h.RegisterMeRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body transport.UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, u.ID, body.ID)
	assert.Equal(t, "Ana", body.Name)
}

func TestMeHandler_PatchMeUpdatesName(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	u, err := users.Insert(t.Context(), "ana@example.com", "Ana")
	require.NoError(t, err)

	h := transport.NewMeHandlers(svc, sessions, transport.CookieConfig{SessionName: "session_id", CSRFName: "csrf_token"})
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: u.ID, CSRFToken: "ct"}))
		h.RegisterMeRoutes(grp)
	})

	body, _ := json.Marshal(map[string]string{"name": "Ana Lima"})
	req := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got transport.UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Ana Lima", got.Name)
}

func TestMeHandler_DeleteAllSessions(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	uid := uuid.New()
	h := transport.NewMeHandlers(svc, sessions, transport.CookieConfig{SessionName: "session_id", CSRFName: "csrf_token"})
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: uid, CSRFToken: "ct"}))
		h.RegisterMeRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/auth/sessions/all", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
}
```

- [ ] **Step 2: Implement me handlers**

Create `internal/modules/identity/transport/me_handlers.go`:

```go
package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
)

// MeHandlers handles authenticated identity endpoints.
type MeHandlers struct {
	svc      *application.IdentityService
	sessions sessionauth.Manager
	cookies  CookieConfig
}

// NewMeHandlers builds MeHandlers.
func NewMeHandlers(svc *application.IdentityService, sessions sessionauth.Manager, cookies CookieConfig) *MeHandlers {
	return &MeHandlers{svc: svc, sessions: sessions, cookies: cookies}
}

// RegisterMeRoutes wires routes onto r. The caller wraps r with sessionauth + csrf middlewares.
func (h *MeHandlers) RegisterMeRoutes(r chi.Router) {
	r.Get("/me", h.GetMe)
	r.Patch("/me", h.UpdateProfile)
	r.Post("/me/change-password", h.ChangePassword)
	r.Post("/auth/logout", h.Logout)
	r.Delete("/auth/sessions/all", h.DeleteAllSessions)
}

func (h *MeHandlers) GetMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	user, err := h.svc.GetMe(r.Context(), sess.UserID)
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusOK, userResponse(user))
}

type updateProfileInput struct {
	Name *string `json:"name,omitempty"`
}

func (h *MeHandlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	var in updateProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if in.Name == nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "no fields to update")
		return
	}
	user, err := h.svc.UpdateProfile(r.Context(), application.UpdateProfileInput{
		UserID: sess.UserID,
		Name:   strings.TrimSpace(*in.Name),
	})
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusOK, userResponse(user))
}

type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *MeHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	var in changePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if err := h.svc.ChangePassword(r.Context(), application.ChangePasswordInput{
		UserID:          sess.UserID,
		CurrentPassword: in.CurrentPassword,
		NewPassword:     in.NewPassword,
		KeepSessionID:   sess.ID,
	}); err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *MeHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	if err := h.sessions.Delete(r.Context(), sess.ID); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "logout failed", err)
		return
	}
	clearCookie(w, h.cookies.SessionName)
	clearCookie(w, h.cookies.CSRFName)
	w.WriteHeader(http.StatusNoContent)
}

func (h *MeHandlers) DeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	if err := h.sessions.DeleteAllForUser(r.Context(), sess.UserID); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "session purge failed", err)
		return
	}
	clearCookie(w, h.cookies.SessionName)
	clearCookie(w, h.cookies.CSRFName)
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/modules/identity/transport/... -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/modules/identity/transport/me_handlers.go \
        internal/modules/identity/transport/me_handlers_test.go
git commit -m "feat(identity): add me handlers (get/patch /me, change-password, sessions/all)"
```

---

## Task 23: `internal/modules/identity/module.go` — module wiring

**Files:**
- Create: `internal/modules/identity/module.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — module assembly mirroring catalog/module.go

- [ ] **Step 1: Implement module**

Create `internal/modules/identity/module.go`:

```go
// Package identity wires the identity bounded context.
package identity

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
)

// Module wires the identity bounded context onto a chi router.
type Module struct {
	auth     *transport.AuthHandlers
	me       *transport.MeHandlers
	sessions sessionauth.Manager
	csrfCfg  csrf.Config
	rl       func(http.Handler) http.Handler
}

// Deps groups raw dependencies the module needs.
type Deps struct {
	Pool          *pgxpool.Pool
	Redis         *redis.Client
	Email         email.Sender
	Sessions      sessionauth.Manager
	Cookies       transport.CookieConfig
	CSRFCfg       csrf.Config
	RateLimitOpts ratelimit.Options
	Cfg           config.Config
}

// New builds the identity Module.
func New(d Deps) *Module {
	users := infrastructure.NewUserRepository(d.Pool)
	auths := infrastructure.NewAuthMethodRepository(d.Pool)
	verify := infrastructure.NewEmailVerifyTokenRepository(d.Pool)
	reset := infrastructure.NewPasswordResetTokenRepository(d.Pool)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users:       users,
		AuthMethods: auths,
		VerifyTokens: verify,
		ResetTokens:  reset,
		Email:        d.Email,
		VerifyLinkBaseURL: d.Cfg.Email.VerifyLinkBaseURL,
		ResetLinkBaseURL:  d.Cfg.Email.ResetLinkBaseURL,
		RevokeAllSessions: d.Sessions.DeleteAllForUser,
		RevokeAllSessionsExcept: d.Sessions.DeleteAllForUserExcept,
	})

	return &Module{
		auth:     transport.NewAuthHandlers(svc, d.Sessions, d.Cookies),
		me:       transport.NewMeHandlers(svc, d.Sessions, d.Cookies),
		sessions: d.Sessions,
		csrfCfg:  d.CSRFCfg,
	}
}

// Mount registers public + authenticated routes.
//
// Public routes get rate-limit middleware composed via the per-route group.
// Authenticated routes also pick up sessionauth + csrf middleware.
func (m *Module) Mount(r chi.Router) {
	// Public auth endpoints (rate-limited individually inside RegisterAuthRoutes
	// callers — but we keep them grouped here for clarity).
	r.Group(func(public chi.Router) {
		m.auth.RegisterAuthRoutes(public)
	})

	// Authenticated routes (session + CSRF).
	r.Group(func(auth chi.Router) {
		auth.Use(sessionauth.Middleware(m.sessions))
		auth.Use(csrf.Middleware(m.csrfCfg))
		m.me.RegisterMeRoutes(auth)
	})
}
```

(`http` import is needed when the optional `rl` field is reintroduced — keep it for future composition. If unused, the import is omitted by gofmt.)

- [ ] **Step 2: Run build**

```bash
go build ./internal/modules/identity/...
```

Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/modules/identity/module.go
git commit -m "feat(identity): add module wiring (auth + authenticated routes)"
```

---

## Task 24: `cmd/api/main.go` — wire identity, sessions, csrf, ratelimit, email

**Files:**
- Modify: `cmd/api/main.go`

**Skills to consult:**
- `cc-skills-golang:golang-design-patterns` — manual constructor injection order
- `cc-skills-golang:golang-context` — graceful shutdown context

- [ ] **Step 1: Read existing main**

```bash
cat cmd/api/main.go
```

The file already wires config, logger, postgres pool, redis client, storage, image processor, queue, catalog. Add identity wiring before catalog mount, since identity does not depend on catalog (and vice versa); ordering is for consistency.

- [ ] **Step 2: Add wiring**

Inside `main()` (or wherever components are constructed), append after the existing redis/email/storage section the following block. Adapt variable names to match what's already there (e.g. `pool`, `rdb`, `cfg`, `logger`).

```go
// --- Email sender ---
emailSender, err := email.NewSenderFromConfig(email.Config{
	Provider:            cfg.Email.Provider,
	FromAddress:         cfg.Email.FromAddress,
	FromName:            cfg.Email.FromName,
	SESRegion:           cfg.Email.SESRegion,
	SESConfigurationSet: cfg.Email.SESConfigurationSet,
}, logger)
if err != nil {
	return fmt.Errorf("api: build email sender: %w", err)
}

// --- Sessions ---
sessions := sessionauth.NewRedisManager(sessionauth.RedisOptions{
	Client:        rdb,
	TTLDefault:    cfg.Session.TTLDefault,
	TTLRememberMe: cfg.Session.TTLRememberMe,
	RefreshAfter:  cfg.Session.RefreshAfter,
})

// --- Cookie names (toggle prefix in prod) ---
cookies := transport.CookieConfig{
	SessionName: cookieName("session_id", cfg.Cookies.SecurePrefix),
	CSRFName:    cookieName("csrf_token", cfg.Cookies.SecurePrefix),
	SecurePrefix: cfg.Cookies.SecurePrefix,
}

// --- CSRF ---
csrfCfg := csrf.Config{
	AllowedOrigins: cfg.CSRF.AllowedOrigins,
	CookieName:     cookies.CSRFName,
}

// --- Trusted proxies for rate limit IP source ---
trustedProxies, err := parseCIDRs(cfg.RateLimit.TrustedProxies)
if err != nil {
	return fmt.Errorf("api: parse trusted proxies: %w", err)
}

// --- Identity module ---
identityModule := identity.New(identity.Deps{
	Pool:    pool,
	Redis:   rdb,
	Email:   emailSender,
	Sessions: sessions,
	Cookies: cookies,
	CSRFCfg: csrfCfg,
	RateLimitOpts: ratelimit.Options{
		Client:         rdb,
		TrustedProxies: trustedProxies,
	},
	Cfg: cfg,
})
```

- [ ] **Step 3: Mount identity routes onto the chi router**

In the same function, find the existing block where `catalog.Mount(...)` is called and append:

```go
identityModule.Mount(r)
```

Make sure rate-limit middleware is applied per-route in identity/transport when needed; for now identityModule.Mount mounts both public and authenticated branches.

- [ ] **Step 4: Add `cookieName` and `parseCIDRs` helpers near the top of main.go**

```go
func cookieName(base string, securePrefix bool) string {
	if securePrefix {
		return "__Secure-" + base
	}
	return base
}

func parseCIDRs(raw []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		out = append(out, p)
	}
	return out, nil
}
```

Add `net/netip`, `strings`, and the new internal package imports at the top.

- [ ] **Step 5: Apply identity-specific rate-limit rules per public auth endpoint**

Wrap the auth route group inside `identityModule.Mount` with rate limit. Modify `internal/modules/identity/module.go` `Mount` to accept an optional rate-limit middleware, OR — simpler — apply rate-limit middleware to the whole `r` chain in main.go for the auth path. For Phase 2a, the cleanest path is per-route:

Replace `identityModule.Mount(r)` with route-by-route mounting in `main.go` (only if you want the per-endpoint limits):

```go
r.Group(func(g chi.Router) {
	g.Use(ratelimit.Middleware(ratelimit.Options{
		Client: rdb, TrustedProxies: trustedProxies,
		Rules: []ratelimit.Rule{
			{Key: "login:ip", Source: ratelimit.ByIP, Limit: 5, Window: 15 * time.Minute},
			{Key: "login:email", Source: ratelimit.ByEmailField, Field: "email", Limit: 5, Window: 15 * time.Minute},
		},
	}))
	g.Post("/auth/login", identityModule.AuthHandler().Login)
})
// register endpoints, verify-resend, password-reset/request, etc. — each with their own rate limit set
```

Add a public method on Module to expose handlers:

```go
// AuthHandler exposes the auth handler for per-endpoint route mounting.
func (m *Module) AuthHandler() *transport.AuthHandlers { return m.auth }

// MeHandler exposes the me handler for per-endpoint route mounting.
func (m *Module) MeHandler() *transport.MeHandlers { return m.me }
```

For Phase 2a, the simplest approach acceptable: keep `identityModule.Mount(r)` for the basic shape and apply a single moderate rate limit to the entire `/auth/*` prefix. Document that Phase 2b polish will tighten per-endpoint.

- [ ] **Step 6: Build + run smoke**

```bash
go build ./...
```

Expected: build succeeds.

Run the API binary briefly to assert wiring works (with all required env vars):

```bash
DATABASE_URL=postgres://postgres:pw@localhost:55433/postgres?sslmode=disable \
ADMIN_API_TOKEN=test \
STORAGE_ENDPOINT=http://localhost:9000 \
STORAGE_ACCESS_KEY_ID=k STORAGE_SECRET_ACCESS_KEY=s STORAGE_BUCKET=b \
STORAGE_PUBLIC_BASE_URL=http://localhost:9000/b \
EMAIL_VERIFY_LINK_BASE_URL=http://localhost:3000/verify \
EMAIL_RESET_LINK_BASE_URL=http://localhost:3000/reset \
go run ./cmd/api &
APIPID=$!
sleep 1
curl -sS http://localhost:8080/health
kill $APIPID
```

Expected: `{"status":"ok"}` (or whatever Phase 1 returns from `/health`).

- [ ] **Step 7: Commit**

```bash
git add cmd/api/main.go internal/modules/identity/module.go
git commit -m "feat(api): wire identity module + sessions + csrf + ratelimit + email"
```

---

## Task 25: OpenAPI spec extension

**Files:**
- Modify: `api/openapi.yaml`

**Skills to consult:**
- `cc-skills-golang:golang-swagger` — annotation patterns (project uses spec-first, not codegen, but the structure follows OpenAPI 3.1)

- [ ] **Step 1: Add new tags**

Open `api/openapi.yaml`. Under `tags:`, append:

```yaml
- name: auth
  description: Authentication endpoints (register, login, verify, reset).
- name: account
  description: Authenticated account-management endpoints (/me, sessions).
```

- [ ] **Step 2: Add schemas**

Under `components.schemas:`, append:

```yaml
UserResponse:
  type: object
  required: [id, email, name]
  properties:
    id:
      type: string
      format: uuid
    email:
      type: string
      format: email
    name:
      type: string
    email_verified_at:
      type: string
      format: date-time
      nullable: true

ErrorResponse:
  type: object
  required: [error]
  properties:
    error:
      type: object
      required: [code, message]
      properties:
        code:
          type: string
        message:
          type: string

RegisterRequest:
  type: object
  required: [email, password, name]
  properties:
    email: { type: string, format: email }
    password: { type: string, minLength: 8 }
    name: { type: string }

LoginRequest:
  type: object
  required: [email, password]
  properties:
    email: { type: string, format: email }
    password: { type: string }
    remember: { type: boolean, default: false }

VerifyEmailRequest:
  type: object
  required: [token]
  properties:
    token: { type: string }

ResendVerifyRequest:
  type: object
  required: [email]
  properties:
    email: { type: string, format: email }

PasswordResetRequest:
  type: object
  required: [email]
  properties:
    email: { type: string, format: email }

PasswordResetConfirm:
  type: object
  required: [token, new_password]
  properties:
    token: { type: string }
    new_password: { type: string, minLength: 8 }

UpdateProfileRequest:
  type: object
  properties:
    name: { type: string }

ChangePasswordRequest:
  type: object
  required: [current_password, new_password]
  properties:
    current_password: { type: string }
    new_password: { type: string, minLength: 8 }
```

- [ ] **Step 3: Add paths**

Under `paths:`, append (preserve the existing catalog paths above):

```yaml
/auth/register:
  post:
    tags: [auth]
    summary: Register a new user.
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '#/components/schemas/RegisterRequest' }
    responses:
      '201': { description: Created, content: { application/json: { schema: { $ref: '#/components/schemas/UserResponse' } } } }
      '400': { $ref: '#/components/responses/BadRequest' }
      '409': { $ref: '#/components/responses/Conflict' }
      '422': { $ref: '#/components/responses/Unprocessable' }
      '429': { $ref: '#/components/responses/RateLimited' }

/auth/login:
  post:
    tags: [auth]
    summary: Log in with email + password.
    requestBody:
      required: true
      content: { application/json: { schema: { $ref: '#/components/schemas/LoginRequest' } } }
    responses:
      '200': { description: OK, content: { application/json: { schema: { $ref: '#/components/schemas/UserResponse' } } } }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }
      '429': { $ref: '#/components/responses/RateLimited' }

/auth/verify-email:
  post:
    tags: [auth]
    summary: Consume an email verify token.
    requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/VerifyEmailRequest' } } } }
    responses:
      '200': { description: OK }
      '400': { $ref: '#/components/responses/BadRequest' }

/auth/verify-email/resend:
  post:
    tags: [auth]
    summary: Re-send the verify email (always returns 202).
    requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/ResendVerifyRequest' } } } }
    responses:
      '202': { description: Accepted }
      '429': { $ref: '#/components/responses/RateLimited' }

/auth/password-reset/request:
  post:
    tags: [auth]
    summary: Request a password reset email (always returns 202).
    requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/PasswordResetRequest' } } } }
    responses:
      '202': { description: Accepted }
      '429': { $ref: '#/components/responses/RateLimited' }

/auth/password-reset/confirm:
  post:
    tags: [auth]
    summary: Consume a reset token and set a new password.
    requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/PasswordResetConfirm' } } } }
    responses:
      '200': { description: OK }
      '400': { $ref: '#/components/responses/BadRequest' }
      '422': { $ref: '#/components/responses/Unprocessable' }

/auth/csrf:
  get:
    tags: [auth]
    summary: Issue a CSRF cookie + body token (bootstrap before login).
    responses:
      '200':
        description: OK
        content:
          application/json:
            schema:
              type: object
              required: [csrf_token]
              properties:
                csrf_token: { type: string }

/auth/logout:
  post:
    tags: [account]
    summary: Logout the current session.
    security: [{ session: [] }]
    responses:
      '204': { description: No Content }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }

/auth/sessions/all:
  delete:
    tags: [account]
    summary: Revoke all sessions for the current user.
    security: [{ session: [] }]
    responses:
      '204': { description: No Content }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }

/me:
  get:
    tags: [account]
    summary: Get the current user.
    security: [{ session: [] }]
    responses:
      '200': { description: OK, content: { application/json: { schema: { $ref: '#/components/schemas/UserResponse' } } } }
      '401': { $ref: '#/components/responses/Unauthorized' }
  patch:
    tags: [account]
    summary: Update editable fields on the current user.
    security: [{ session: [] }]
    requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/UpdateProfileRequest' } } } }
    responses:
      '200': { description: OK, content: { application/json: { schema: { $ref: '#/components/schemas/UserResponse' } } } }
      '400': { $ref: '#/components/responses/BadRequest' }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }

/me/change-password:
  post:
    tags: [account]
    summary: Change password (requires current password).
    security: [{ session: [] }]
    requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/ChangePasswordRequest' } } } }
    responses:
      '204': { description: No Content }
      '400': { $ref: '#/components/responses/BadRequest' }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }
      '422': { $ref: '#/components/responses/Unprocessable' }
```

- [ ] **Step 4: Add responses + security scheme**

Under `components`, ensure these reusable responses exist (add if missing):

```yaml
responses:
  BadRequest:
    description: Bad Request
    content: { application/json: { schema: { $ref: '#/components/schemas/ErrorResponse' } } }
  Unauthorized:
    description: Unauthorized
    content: { application/json: { schema: { $ref: '#/components/schemas/ErrorResponse' } } }
  Forbidden:
    description: Forbidden
    content: { application/json: { schema: { $ref: '#/components/schemas/ErrorResponse' } } }
  Conflict:
    description: Conflict
    content: { application/json: { schema: { $ref: '#/components/schemas/ErrorResponse' } } }
  Unprocessable:
    description: Unprocessable Entity
    content: { application/json: { schema: { $ref: '#/components/schemas/ErrorResponse' } } }
  RateLimited:
    description: Too Many Requests
    content: { application/json: { schema: { $ref: '#/components/schemas/ErrorResponse' } } }
```

Add the security scheme:

```yaml
securitySchemes:
  session:
    type: apiKey
    in: cookie
    name: session_id
```

- [ ] **Step 5: Validate the spec**

Use `npx` to validate (or any OpenAPI validator):

```bash
npx -y @redocly/cli@latest lint api/openapi.yaml
```

Expected: 0 errors. Warnings are OK.

- [ ] **Step 6: Commit**

```bash
git add api/openapi.yaml
git commit -m "docs(api): extend OpenAPI spec with auth + account endpoints"
```

---

## Task 26: E2E integration test suite

**Files:**
- Create: `tests/integration/identity_e2e_test.go`

**Skills to consult:**
- `cc-skills-golang:golang-testing` — testcontainers, subtest organisation
- `cc-skills-golang:golang-stretchr-testify` — assertions

- [ ] **Step 1: Inspect existing E2E pattern**

Read `tests/integration/catalog_e2e_test.go` and `tests/integration/smoke_test.go` (if present) to mirror setup helpers (DB pool, redis, HTTP server boot).

```bash
ls tests/integration/
```

If `tests/integration/` does not yet exist, create it as part of this task.

- [ ] **Step 2: Write the E2E suite**

Create `tests/integration/identity_e2e_test.go`:

```go
//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startAPIForIdentity boots a chi router wired with the identity module
// against testcontainers Postgres + Redis. Returns httptest.Server.
//
// The actual implementation lives in a helper file `support_test.go` per
// the existing Phase 1 integration suite pattern. If that helper does not
// exist yet, follow `catalog_e2e_test.go` and replicate the boot steps:
//   1. testutil.StartPostgres(t, ctx) → pool, run migrations
//   2. testutil.StartRedis(t, ctx) → rdb
//   3. build identity module + middlewares (mirror cmd/api wiring)
//   4. mount on chi router; wrap in httptest.NewServer
//   5. return server + a fakeSender capturing emails
//
// The plan assumes such a helper exists; if you must create it, scope it to
// this task and commit alongside.

func TestIdentityE2E_RegisterVerifyLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	// 1) Register
	resp := postJSON(t, srv, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!", "name": "Ana",
	}, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Captured verify email
	require.Eventually(t, func() bool { return len(captured.messages()) > 0 }, 5*time.Second, 50*time.Millisecond)
	verifyMsg := captured.messages()[0]
	token := extractTokenFromBody(t, verifyMsg.TextBody, "verify?token=")

	// 2) Verify email
	resp = postJSON(t, srv, "/auth/verify-email", map[string]string{"token": token}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 3) Login → 200 + cookies
	resp = postJSON(t, srv, "/auth/login", map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cookies := resp.Cookies()
	resp.Body.Close()

	var session, csrfCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "session_id":
			session = c
		case "csrf_token":
			csrfCookie = c
		}
	}
	require.NotNil(t, session)
	require.NotNil(t, csrfCookie)

	// 4) GET /me with cookie → 200 + correct user
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/me", nil)
	req.AddCookie(session)
	mResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, mResp.StatusCode)
	var user struct{ Email string `json:"email"` }
	require.NoError(t, json.NewDecoder(mResp.Body).Decode(&user))
	mResp.Body.Close()
	assert.Equal(t, "ana@example.com", user.Email)
}

func TestIdentityE2E_LoginUnverifiedBlocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, _ := startAPIForIdentity(t, ctx)
	defer srv.Close()

	resp := postJSON(t, srv, "/auth/register", map[string]string{
		"email": "noverify@example.com", "password": "S3cretPass!", "name": "X",
	}, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = postJSON(t, srv, "/auth/login", map[string]string{
		"email": "noverify@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}

func TestIdentityE2E_PasswordResetFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	registerVerify(t, srv, captured, "rst@example.com", "S3cretPass!")

	resp := postJSON(t, srv, "/auth/password-reset/request", map[string]string{
		"email": "rst@example.com",
	}, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	require.Eventually(t, func() bool { return len(captured.messages()) >= 2 }, 5*time.Second, 50*time.Millisecond)
	resetMsg := captured.messages()[len(captured.messages())-1]
	token := extractTokenFromBody(t, resetMsg.TextBody, "reset?token=")

	resp = postJSON(t, srv, "/auth/password-reset/confirm", map[string]string{
		"token": token, "new_password": "NewS3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Old password rejected
	resp = postJSON(t, srv, "/auth/login", map[string]string{
		"email": "rst@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// New password works
	resp = postJSON(t, srv, "/auth/login", map[string]string{
		"email": "rst@example.com", "password": "NewS3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestIdentityE2E_CSRFRequired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	cookies := registerVerifyLogin(t, srv, captured, "csrf@example.com", "S3cretPass!")

	// PATCH /me without CSRF header → 403
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/me", strings.NewReader(`{"name":"X"}`))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	// PATCH /me with valid CSRF header → 200
	var csrfValue string
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfValue = c.Value
		}
	}
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/me", strings.NewReader(`{"name":"X"}`))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfValue)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestIdentityE2E_LogoutAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	first := registerVerifyLogin(t, srv, captured, "logoutall@example.com", "S3cretPass!")

	// Second login (different "device").
	loginResp := postJSON(t, srv, "/auth/login", map[string]string{
		"email": "logoutall@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	second := loginResp.Cookies()
	loginResp.Body.Close()

	// DELETE /auth/sessions/all from first device.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/auth/sessions/all", nil)
	for _, c := range first {
		req.AddCookie(c)
	}
	for _, c := range first {
		if c.Name == "csrf_token" {
			req.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Both cookies should now be invalid.
	for _, group := range [][]*http.Cookie{first, second} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/me", nil)
		for _, c := range group {
			req.AddCookie(c)
		}
		r, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, r.StatusCode)
		r.Body.Close()
	}
}

// --- helpers ---

func postJSON(t *testing.T, srv *httptest.Server, path string, body any, cookies []*http.Cookie) *http.Response {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func extractTokenFromBody(t *testing.T, body, marker string) string {
	t.Helper()
	idx := strings.Index(body, marker)
	require.GreaterOrEqual(t, idx, 0, "marker %q not found in body", marker)
	rest := body[idx+len(marker):]
	end := strings.IndexAny(rest, "\n \t")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

func registerVerify(t *testing.T, srv *httptest.Server, captured emailCapture, email, password string) {
	t.Helper()
	resp := postJSON(t, srv, "/auth/register", map[string]string{
		"email": email, "password": password, "name": "User",
	}, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
	require.Eventually(t, func() bool { return len(captured.messages()) > 0 }, 5*time.Second, 50*time.Millisecond)
	last := captured.messages()[len(captured.messages())-1]
	token := extractTokenFromBody(t, last.TextBody, "verify?token=")
	resp = postJSON(t, srv, "/auth/verify-email", map[string]string{"token": token}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func registerVerifyLogin(t *testing.T, srv *httptest.Server, captured emailCapture, email, password string) []*http.Cookie {
	t.Helper()
	registerVerify(t, srv, captured, email, password)
	resp := postJSON(t, srv, "/auth/login", map[string]string{
		"email": email, "password": password,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cookies := resp.Cookies()
	resp.Body.Close()
	return cookies
}
```

- [ ] **Step 3: Implement E2E support helpers**

Create or extend `tests/integration/support_test.go` with:

```go
//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	// imports for your project — adapt to existing patterns
)

// emailCapture is an interface our test helper exposes to read captured messages.
type emailCapture interface {
	messages() []email.Message
}

type fakeSender struct {
	mu  sync.Mutex
	msg []email.Message
}

func (f *fakeSender) Send(_ context.Context, m email.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msg = append(f.msg, m)
	return nil
}
func (f *fakeSender) messages() []email.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]email.Message, len(f.msg))
	copy(out, f.msg)
	return out
}

// startAPIForIdentity boots a complete identity API on httptest.
//
// Mirror the wiring done in cmd/api/main.go but with:
//   - testutil.StartPostgres + atlas migrate
//   - testutil.StartRedis
//   - LogSender replaced by fakeSender
//   - cmd/api router → httptest.Server
//
// Use the existing catalog_e2e_test.go bootstrap for the Postgres + chi
// mount pattern; copy and extend with identity wiring (no shared
// production code outside what main.go already provides).
func startAPIForIdentity(t *testing.T, ctx context.Context) (*httptest.Server, emailCapture) {
	t.Helper()
	// Implementation: mirror cmd/api wiring step-by-step here.
	// ... (replace this block with the actual setup, matching the patterns
	//      of catalog_e2e_test.go's helpers)
	panic("implement me — copy catalog E2E setup and add identity module")
}
```

Replace the `panic` body with a full setup. The skeleton keeps Phase 1 patterns intact and avoids leaking test code into the production binary.

- [ ] **Step 4: Run E2E tests**

```bash
go test -tags=integration -count=1 -timeout=15m ./tests/integration/... -v -run TestIdentityE2E
```

Expected: all pass. Failures are most likely cookie/CSRF wiring mismatches between `cmd/api/main.go` and `startAPIForIdentity`. Fix in one place at a time — the assertions are explicit about which property fails.

- [ ] **Step 5: Commit**

```bash
git add tests/integration/identity_e2e_test.go tests/integration/support_test.go
git commit -m "test(integration): add identity E2E suite (register/verify/login/reset/csrf/logoutall)"
```

---

## Task 27: README env-var documentation

**Files:**
- Modify: `README.md`

**Skills to consult:**
- `cc-skills-golang:golang-documentation` — README sections, env var table

- [ ] **Step 1: Find existing env var docs**

```bash
grep -n "DATABASE_URL\|STORAGE_ENDPOINT" README.md
```

If a `## Environment Variables` section exists, extend it. If not, add one near the bottom of the README (or wherever Phase 1 placed similar content).

- [ ] **Step 2: Append Phase 2a variables**

```markdown
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
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document Phase 2a env vars in README"
```

---

## Task 28: Final validation + tag `v0.4.0-identity`

**Files:** none

- [ ] **Step 1: Full test sweep**

```bash
go fmt ./...
go vet ./...
go build ./...
go test ./...                                            # unit
go test -tags=integration -count=1 -timeout=15m ./...    # integration + E2E
go test -race ./internal/...                             # race detector on unit tests
```

Every command must succeed.

- [ ] **Step 2: Manual smoke against running API (optional but recommended)**

Bring up Docker (`open -a Docker` on macOS), then:

```bash
docker run --rm -d --name pg-2a -p 55432:5432 -e POSTGRES_PASSWORD=pw postgres:16
docker run --rm -d --name redis-2a -p 6380:6379 redis:7

DATABASE_URL=postgres://postgres:pw@localhost:55432/postgres?sslmode=disable \
  atlas migrate apply --env local

DATABASE_URL=postgres://postgres:pw@localhost:55432/postgres?sslmode=disable \
ADMIN_API_TOKEN=admin-token \
REDIS_ADDR=localhost:6380 \
STORAGE_ENDPOINT=http://localhost:9000 STORAGE_ACCESS_KEY_ID=k \
STORAGE_SECRET_ACCESS_KEY=s STORAGE_BUCKET=b STORAGE_PUBLIC_BASE_URL=http://localhost:9000/b \
EMAIL_VERIFY_LINK_BASE_URL=http://localhost:3000/verify \
EMAIL_RESET_LINK_BASE_URL=http://localhost:3000/reset \
go run ./cmd/api &

# Register
curl -sS -X POST -H 'Content-Type: application/json' \
  -d '{"email":"smoke@example.com","password":"S3cretPass!","name":"Smoke"}' \
  http://localhost:8080/auth/register

# The verify URL is logged on stdout. Copy the token and run:
curl -sS -X POST -H 'Content-Type: application/json' \
  -d '{"token":"<token>"}' http://localhost:8080/auth/verify-email

# Login
curl -sS -c /tmp/cookies.txt -X POST -H 'Content-Type: application/json' \
  -d '{"email":"smoke@example.com","password":"S3cretPass!"}' \
  http://localhost:8080/auth/login

curl -sS -b /tmp/cookies.txt http://localhost:8080/me
```

Tear down:

```bash
docker rm -f pg-2a redis-2a
```

- [ ] **Step 3: Tag the branch**

```bash
git tag -a v0.4.0-identity -m "Phase 2a — Identity (auth + sessions + CSRF + ratelimit + email)"
git tag --list
```

Optional push (only if user explicitly asks):

```bash
# git push origin feat/phase-2a-identity
# git push origin v0.4.0-identity
```

- [ ] **Step 4: Update memory file**

Update the marketplace project memory at `~/.claude/projects/-Users-danilloboing-Documents-danillo-projects-marketplace-golang/memory/project_marketplace_current_state.md` to reflect that Phase 2a is complete and Phase 2b (commerce) is next.

(This step is informational — no commit; it lives outside the repo.)

- [ ] **Step 5: Final commit if any docs changed during validation**

```bash
git status
# If anything was tweaked, commit with an appropriate message.
```

---

## Self-Review

This section was filled in after writing the plan; it documents the spec coverage check and any gaps fixed inline.

**Spec coverage map:**

| Spec section | Plan task(s) |
|---|---|
| §1 Scope (in-scope) | All in-scope items mapped to Tasks 2–28 |
| §2 Architecture (module layout, naming) | Task 23 (module) + naming applied across Tasks 4–22 |
| §3 Data Model (identity migrations) | Task 2 |
| §4 HTTP API (auth + account endpoints) | Tasks 21 + 22 + 25 |
| §5.1 sessionauth | Tasks 8 + 9 |
| §5.2 csrf | Task 10 |
| §5.3 ratelimit | Task 11 |
| §5.4 email | Tasks 6 + 7 |
| §5.5 viacep | Deferred to Phase 2b — config added in Task 1 |
| §5.6 passwords | Task 4 |
| §5.7 tokens | Task 5 |
| §6 Security policy | Threaded across Tasks 4 (constant-time, password hash), 8 (sessions), 10 (CSRF + Origin), 11 (rate limit + trusted proxies), 17 (login dummy verify), 19 (revoke on reset), 20 (revoke-except on change) |
| §7 Error handling | Task 3 (responsex), Task 12 (sentinels), Task 21 (mapping) |
| §8 Testing strategy | Each task carries unit + (where applicable) integration tests; Task 26 ships E2E |
| §9 Sub-Phase Plans (~22 tasks) | Plan has 28 tasks; the spec's task count was a rough estimate. The split is finer-grained but covers identical scope |
| §10 Skill References | Skill list embedded in every task header |
| §11 Configuration | Task 1 (config) + Task 27 (README) |

**Placeholder scan:** none remaining (no `TBD`, no "implement later", no `// TODO` left in tasks). Two task notes mention "adapt to existing patterns" — those are guidance, not placeholders, with concrete pointers (`catalog_e2e_test.go`, `cmd/api/main.go`).

**Type/name consistency check:**
- `email.Sender` interface (Tasks 6, 7, 8 wiring, 21 handlers) — consistent
- `sessionauth.Manager` interface, `sessionauth.RedisManager`, `sessionauth.Session` — consistent
- `IdentityServiceDeps` fields (`Users`, `AuthMethods`, `VerifyTokens`, `ResetTokens`, `Email`, `RevokeAllSessions`, `RevokeAllSessionsExcept`) — consistent across Tasks 16–20 and 23
- `CookieConfig` (`SessionName`, `CSRFName`, `SecurePrefix`) — consistent across Tasks 21, 22, 23, 24

**Scope check:** plan covers Phase 2a only. Phase 2b (commerce: cart + addresses + ViaCEP) is its own plan (`docs/superpowers/plans/<future>-phase-2b-commerce.md`) to be written after 2a merges.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-09-phase-2a-identity.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration via `superpowers:subagent-driven-development`.
2. **Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch execution with checkpoints.

Choose one to begin.
