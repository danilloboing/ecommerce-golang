# Phase 2 — Identity + Cart Design

**Date:** 2026-05-09
**Phase:** 2 (Identity + Cart)
**Builds on:** Phase 1 (Foundation + Catalog) — branches `feat/phase-1a-bootstrap`, `feat/phase-1b-catalog`, `feat/phase-1c-image-upload` merged into `main` with tags `v0.1.0-bootstrap`, `v0.2.0-catalog`, `v0.3.0-images`.
**Sub-phases:**
- **2a — Identity:** sessions, CSRF, rate limit, email provider, register, login, email verify, password reset, change password, GET /me, update profile (name)
- **2b — Commerce:** anon cart, user cart with merge-on-login, addresses CRUD, ViaCEP proxy

## 1. Scope

### In-scope (Phase 2)

- Email + password auth (register, login, logout, logout-all)
- Email verification (hard block: login requires `email_verified_at IS NOT NULL`)
- Password reset (request + confirm)
- Change password (requires current password)
- GET /me + PATCH /me (name only)
- Session management (Redis-backed, sliding window, "remember me")
- CSRF protection (double-submit cookie + Origin check)
- Rate limiting (token bucket Redis, per-endpoint granularity)
- Email provider abstraction with `LogSender` (dev/test) and `SESSender` (prod) impls
- Server-side anon cart (cookie-based identification) + user cart with merge on login
- Addresses CRUD per user (one default flag, atomic uniqueness)
- ViaCEP proxy with Redis cache (1h TTL)

### Deferred to later phases

| Feature | Phase | Reason |
|---|---|---|
| Google OAuth login | 2.5 or 3 | Schema preparado (`auth_methods.provider='google'`); impl pending |
| Wishlist / Saved-for-later | 4 | Standalone feature; doesn't block checkout |
| Update email (re-verify chain) | 4 | Complexity vs MVP |
| Delete account (LGPD) | 5 | Depends on orders (Phase 3); requires compliance review |
| Cart abandonment recovery email | 4 | Schema preparado (`carts.status='abandoned'`); job impl pending |
| Phone / WhatsApp auth | 5+ | Not requested |
| 2FA / TOTP | 4+ | Schema extensible via `auth_methods` |
| CAPTCHA on signup/login | 2.5 or 3 | Provider integration future |
| Account lockout policy | 4 | Rate limit covers ~90% of need |
| Audit log persistence | 4 | Today only slog stdout |
| Session list / "active devices" UI | 4 | Backend already supports |
| OAuth account linking | 3+ | Schema permits; UX flow pending |
| NF-e CPF/CNPJ on address | 4 | Add column when invoicing required |
| Multi-language email templates | 5 | pt-BR only Phase 2 |

## 2. Architecture

### Module layout

```
internal/
├── core/
│   ├── adminauth/         # existing — bearer admin auth
│   ├── sessionauth/       # NEW — user session middleware (Manager interface + RedisManager impl)
│   ├── csrf/              # NEW — double-submit cookie + Origin check middleware
│   ├── ratelimit/         # NEW — token bucket Redis middleware
│   ├── responsex/         # existing — extend to auto-log errors
│   ├── httpx/             # existing
│   ├── observability/     # existing
│   └── health/            # existing
├── platform/
│   ├── email/             # NEW — Sender interface + LogSender + SESSender
│   ├── viacep/            # NEW — HTTP Client with Redis cache
│   ├── passwords/         # NEW — argon2id Hash/Verify
│   ├── tokens/            # NEW — opaque token Generate/Hash
│   ├── postgres/          # existing — extend queries
│   ├── redis/             # existing
│   ├── storage/           # existing (R2)
│   ├── queue/             # existing (river) — extend with cleanup jobs
│   └── image/             # existing
├── modules/
│   ├── catalog/           # existing Phase 1
│   ├── identity/          # Phase 2a
│   │   ├── domain/        # User, AuthMethod, EmailVerifyToken, PasswordResetToken; sentinel errors
│   │   ├── application/   # IdentityService, ports (UserRepository, AuthMethodRepository, TokenRepository)
│   │   ├── infrastructure/# Postgres impls of repository ports
│   │   ├── jobs/          # cleanup_expired_tokens river job
│   │   └── transport/     # auth_handlers, me_handlers
│   ├── cart/              # Phase 2b
│   │   ├── domain/        # Cart, CartItem; sentinel errors
│   │   ├── application/   # CartService, ports (CartRepository, AnonSessionResolver)
│   │   ├── infrastructure/# Postgres impls
│   │   ├── jobs/          # cleanup_abandoned_carts river job
│   │   └── transport/     # cart_handlers
│   └── address/           # Phase 2b
│       ├── domain/        # Address; sentinel errors
│       ├── application/   # AddressService, ports
│       ├── infrastructure/# Postgres impls
│       └── transport/     # address_handlers, cep_handlers
└── config/                # extend with Phase 2 sections
```

### Wiring (cmd/api/main.go) — order

```
config → logger → postgres pool → redis client → storage (R2) →
email sender (factory: log|ses) → viacep client → river client →
session manager → rate limiter →
repositories (catalog/identity/cart/address) →
services (catalog/identity/cart/address) →
handlers → router → server
```

### Naming conventions (per `golang-naming` skill)

- Package names: lowercase single word (`sessionauth`, `csrf`, `ratelimit`, `viacep`, `passwords`, `tokens`, `email`)
- No stutter: `email.Sender` (not `email.EmailSender`), `sessionauth.Manager` (not `SessionManager`), `viacep.Client`
- Single-method interfaces use `-er` suffix: `email.Sender`
- Multi-method interfaces are nouns: `sessionauth.Manager`
- `passwords` and `tokens` packages expose plain functions (no interface) — they are stateless utilities
- Constructors: `New()` for single primary type per package; `NewRedisManager()` when package has multiple constructible types
- Sentinel errors: `domain.ErrInvalidCredentials` with package-prefixed message `"identity: invalid credentials"`
- Compile-time interface checks: `var _ email.Sender = (*email.LogSender)(nil)` in each impl file

## 3. Data Model

### Phase 2a — Identity migrations

**File:** `db/migrations/20260510000001_identity.sql`

```sql
CREATE EXTENSION IF NOT EXISTS citext;

-- users: identity (email, profile, status)
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

-- auth_methods: credentials per user, multi-provider ready
CREATE TABLE auth_methods (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider          TEXT NOT NULL CHECK (provider IN ('password','google')),
  password_hash     TEXT,
  provider_subject  TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at      TIMESTAMPTZ,
  CHECK (
    (provider = 'password' AND password_hash IS NOT NULL AND provider_subject IS NULL)
    OR
    (provider = 'google'   AND provider_subject IS NOT NULL AND password_hash IS NULL)
  )
);
CREATE UNIQUE INDEX auth_methods_user_provider_uniq ON auth_methods(user_id, provider);
CREATE UNIQUE INDEX auth_methods_provider_subject_uniq
  ON auth_methods(provider, provider_subject)
  WHERE provider_subject IS NOT NULL;

-- email_verify_tokens: opaque single-use tokens
CREATE TABLE email_verify_tokens (
  token_hash    BYTEA PRIMARY KEY,
  user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email         CITEXT NOT NULL,
  expires_at    TIMESTAMPTZ NOT NULL,
  consumed_at   TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_verify_tokens_user_active_idx
  ON email_verify_tokens(user_id) WHERE consumed_at IS NULL;

-- password_reset_tokens: opaque single-use tokens
CREATE TABLE password_reset_tokens (
  token_hash    BYTEA PRIMARY KEY,
  user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at    TIMESTAMPTZ NOT NULL,
  consumed_at   TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_tokens_user_active_idx
  ON password_reset_tokens(user_id) WHERE consumed_at IS NULL;
```

### Phase 2b — Commerce migrations

**File:** `db/migrations/20260511000001_commerce.sql`

```sql
-- addresses: shipping addresses per user
CREATE TABLE addresses (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_name  TEXT NOT NULL,
  postal_code     TEXT NOT NULL CHECK (postal_code ~ '^[0-9]{8}$'),
  street          TEXT NOT NULL,
  number          TEXT NOT NULL,
  complement      TEXT,
  neighborhood    TEXT NOT NULL,
  city            TEXT NOT NULL,
  state           CHAR(2) NOT NULL,
  is_default      BOOLEAN NOT NULL DEFAULT FALSE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX addresses_user_idx ON addresses(user_id);
CREATE UNIQUE INDEX addresses_user_default_uniq
  ON addresses(user_id) WHERE is_default = TRUE;

-- carts: one active cart per user OR per anon session
CREATE TABLE carts (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID REFERENCES users(id) ON DELETE CASCADE,
  anon_session_id   TEXT,
  status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','merged','abandoned','converted')),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (user_id IS NOT NULL AND anon_session_id IS NULL)
    OR
    (user_id IS NULL AND anon_session_id IS NOT NULL)
  )
);
CREATE UNIQUE INDEX carts_user_active_uniq
  ON carts(user_id) WHERE status = 'active' AND user_id IS NOT NULL;
CREATE UNIQUE INDEX carts_anon_active_uniq
  ON carts(anon_session_id) WHERE status = 'active' AND anon_session_id IS NOT NULL;

-- cart_items
CREATE TABLE cart_items (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cart_id           UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
  variant_id        UUID NOT NULL REFERENCES product_variants(id) ON DELETE RESTRICT,
  quantity          INT NOT NULL CHECK (quantity > 0 AND quantity <= 99),
  unit_price_cents  INT NOT NULL CHECK (unit_price_cents >= 0),
  added_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX cart_items_cart_variant_uniq ON cart_items(cart_id, variant_id);
```

> **Stock check deferred:** Phase 2 enforces only the hard quantity cap (`<= 99`) at the schema level. Real inventory availability check (variant stock vs requested qty) is **Phase 3** (checkout). Add-to-cart in Phase 2 always succeeds if cap is respected, even if stock would be insufficient at checkout time. Cart price snapshot (`unit_price_cents`) lets the checkout step detect price drift between add and purchase.

### Sessions — Redis-only schema

```
session:<sessionID>           HASH {
  user_id, csrf_token, created_at, last_activity_at,
  expires_at, remember_me, user_agent, ip
}                             TTL: 14d (default) or 30d (remember_me)

session:user:<userID>         SET <sessionID, sessionID, ...>
                              TTL: max of session TTLs
```

`sessionID` and `csrf_token` formats: `crypto/rand` 32 bytes → hex (64 chars).

Loss on Redis restart = mass logout, accepted for MVP. Phase 4+ may add Postgres mirror.

### Anon cart cookie

`cart_anon` cookie, value = `crypto/rand` 32 bytes hex. HttpOnly, Secure, SameSite=Lax, Max-Age=2592000 (30 days), Path=/. Resolved by `ResolveCartIdentity` middleware before cart handlers.

## 4. HTTP API

### Phase 2a — Auth + Account endpoints

**Public (no session required):**

| Method | Path | Body | Response | Notes |
|---|---|---|---|---|
| POST | /auth/register | `{email, password, name}` | 201 `{user}` | sends verify email |
| POST | /auth/login | `{email, password, remember?}` | 200 `{user}` + Set-Cookie session/csrf | hard block if not verified |
| POST | /auth/verify-email | `{token}` | 200 on success / 400 if token invalid/expired/consumed | single-use; consumed tokens cannot be replayed |
| POST | /auth/verify-email/resend | `{email}` | 202 | always 202; rate-limited |
| POST | /auth/password-reset/request | `{email}` | 202 | always 202; no enumeration |
| POST | /auth/password-reset/confirm | `{token, new_password}` | 200 | revokes all sessions |
| GET | /auth/csrf | — | 200 + Set-Cookie csrf_token | bootstrap when no session |

**Authenticated (session + CSRF):**

| Method | Path | Body | Response | Notes |
|---|---|---|---|---|
| POST | /auth/logout | — | 204 | deletes current session |
| DELETE | /auth/sessions/all | — | 204 | deletes all sessions for user |
| GET | /me | — | 200 `{user}` | |
| PATCH | /me | `{name?}` | 200 `{user}` | |
| POST | /me/change-password | `{current_password, new_password}` | 204 | revokes other sessions |

### Phase 2b — Cart + Address + ViaCEP endpoints

**Public (anon session OR user session):**

| Method | Path | Body | Response | Notes |
|---|---|---|---|---|
| GET | /cart | — | 200 `{items[], subtotal_cents}` | lazy-create on add only; `subtotal_cents` computed at read as `sum(item.unit_price_cents * item.quantity)` |
| POST | /cart/items | `{variant_id, quantity}` | 200 `{cart}` | sets `cart_anon` cookie if no session |
| PATCH | /cart/items/:id | `{quantity}` | 200 `{cart}` | |
| DELETE | /cart/items/:id | — | 200 `{cart}` | |
| DELETE | /cart | — | 204 | clears all items |
| GET | /address/cep/:cep | — | 200 `{postal_code, street, neighborhood, city, state}` or 404 | Redis cache 1h |

**Authenticated (session + CSRF):**

| Method | Path | Body | Response | Notes |
|---|---|---|---|---|
| GET | /me/addresses | — | 200 `[address...]` | |
| POST | /me/addresses | `{recipient_name, postal_code, ...}` | 201 `{address}` | |
| PATCH | /me/addresses/:id | partial | 200 `{address}` | |
| DELETE | /me/addresses/:id | — | 204 | |
| POST | /me/addresses/:id/default | — | 200 `{address}` | atomic via tx |

### Status code policy

| Status | Meaning |
|---|---|
| 200 OK | success with body |
| 201 Created | resource created |
| 202 Accepted | async / privacy-preserving (no enumeration) |
| 204 No Content | success without body |
| 400 Bad Request | malformed payload, invalid token shape |
| 401 Unauthorized | not authenticated / invalid credentials |
| 403 Forbidden | authenticated but lacking permission (CSRF mismatch, email_not_verified) |
| 404 Not Found | resource missing (also for cross-user access — don't leak existence) |
| 409 Conflict | email already taken |
| 422 Unprocessable Entity | semantic violation (qty > 99, default address conflict, etc) |
| 429 Too Many Requests | rate limit exceeded; `Retry-After` header |
| 500 Internal Server Error | unexpected; logged with stack |

### Cookies

| Name | Flags | TTL | Purpose |
|---|---|---|---|
| `session_id` (or `__Secure-session_id` in prod) | HttpOnly, Secure, SameSite=Lax, Path=/ | 14d / 30d | user session |
| `csrf_token` (or `__Secure-csrf_token`) | Secure, SameSite=Lax, Path=/ — NOT HttpOnly | matches session | double-submit token (read by JS) |
| `cart_anon` | HttpOnly, Secure, SameSite=Lax, Path=/ | 30d | anon cart identity (cleared on login) |

Prefix `__Secure-` enabled via `COOKIES_SECURE_PREFIX=true` env in production. Disabled in dev (no TLS).

### OpenAPI spec

Extend `api/openapi.yaml` with tags `auth`, `account`, `cart`, `address`. Existing catalog endpoints unchanged.

## 5. Cross-Cutting

### `internal/core/sessionauth`

```go
type Manager interface {
    Create(ctx context.Context, params CreateParams) (Session, error)
    Get(ctx context.Context, sessionID string) (Session, error)
    Refresh(ctx context.Context, sessionID string) error
    Delete(ctx context.Context, sessionID string) error
    DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
    DeleteAllForUserExcept(ctx context.Context, userID uuid.UUID, keepID string) error
}

type Session struct {
    ID, CSRFToken         string
    UserID                uuid.UUID
    CreatedAt, LastActivityAt, ExpiresAt time.Time
    RememberMe            bool
    UserAgent, IP         string
}

type RedisManager struct{ client *redis.Client; clock func() time.Time }

func NewRedisManager(client *redis.Client) *RedisManager
func Middleware(mgr Manager) func(http.Handler) http.Handler
func RequireVerifiedEmail(next http.Handler) http.Handler  // composable extra check
func SessionFromContext(ctx context.Context) (Session, bool)

var _ Manager = (*RedisManager)(nil)
```

`Middleware` reads `session_id` cookie, calls `mgr.Get`, refreshes if `last_activity_at > 24h`, injects `Session` into context. Failure → clears cookie and returns 401.

### `internal/core/csrf`

```go
type Config struct {
    AllowedOrigins []string
    CookieName     string  // "csrf_token" or "__Secure-csrf_token"
}

func Middleware(cfg Config) func(http.Handler) http.Handler
```

Logic for mutation methods (POST/PUT/PATCH/DELETE):
1. Validate `Origin` header in `AllowedOrigins` → 403 on mismatch.
2. Read `csrf_token` cookie + `X-CSRF-Token` header.
3. Compare via `subtle.ConstantTimeCompare`.
4. Read session from context (if present); compare cookie token with session's `csrf_token`.
5. Failure → 403 `{"code":"csrf_invalid"}`.

GET/HEAD/OPTIONS exempt. Public mutation routes (login, register, password-reset/*, verify-email/*) skip CSRF — defense via SameSite=Lax + Origin check + rate limit.

**Cart endpoints have dual mode:** when called with user session → CSRF required (full checks). When called anonymously (only `cart_anon` cookie present) → CSRF skipped, defended by SameSite=Lax + Origin check + rate limit + the fact that `cart_anon` is HttpOnly so attacker JS cannot read or forge it cross-site. The middleware composes accordingly: cart routes use a `csrf.IfAuthenticated()` variant that branches on presence of session in context.

### `internal/core/ratelimit`

```go
type Source int
const (
    SourceUnknown Source = iota
    ByIP
    ByEmailField    // requires JSON body field name
    ByUserID
)

type Rule struct {
    Key    string
    Source Source
    Field  string         // for ByEmailField
    Limit  int
    Window time.Duration
}

func Middleware(client *redis.Client, trustedProxies []netip.Prefix, rules ...Rule) func(http.Handler) http.Handler
```

Strategy: fixed window via Redis Lua atomic `INCR + EXPIRE` (key includes window bucket).

Key generation:
- `realIP(r, trustedProxies)` — returns `r.RemoteAddr` if not from trusted proxy; else parses `X-Forwarded-For` right-to-left skipping trusted CIDRs.
- `ratelimit:<rule.Key>:ip:<ip>:<bucket>` for `ByIP`.
- `ratelimit:<rule.Key>:email:<sha256(email)>:<bucket>` for `ByEmailField` (peeks JSON body — must replay reader).
- `ratelimit:<rule.Key>:user:<userID>:<bucket>` for `ByUserID`.

Exceeded → 429 with `Retry-After: <seconds>` header.

### `internal/platform/email`

```go
type Message struct {
    To       []string
    Subject  string
    HTMLBody string
    TextBody string
    Tags     map[string]string  // for SES configuration sets
}

type Sender interface {
    Send(ctx context.Context, msg Message) error
}

type LogSender struct{ logger *slog.Logger }
func NewLogSender(logger *slog.Logger) *LogSender
var _ Sender = (*LogSender)(nil)

type SESSender struct{ /* ... */ }
func NewSESSender(cfg SESConfig) (*SESSender, error)
var _ Sender = (*SESSender)(nil)

func NewSenderFromConfig(cfg Config, logger *slog.Logger) (Sender, error)  // factory
```

Templates: inline `const` strings per email type (verify, reset). `text/template` for substitution. Phase 2 has 2 templates (verify, reset). Migrate to `html/template` files when count grows.

### `internal/platform/viacep`

```go
type Address struct {
    PostalCode, Street, Neighborhood, City, State string
}

type Client struct {
    httpClient *http.Client
    cache      *redis.Client
    baseURL    string
}

func NewClient(httpClient *http.Client, cache *redis.Client, baseURL string) *Client
func (c *Client) Lookup(ctx context.Context, cep string) (Address, error)
```

- Endpoint: `https://viacep.com.br/ws/<cep>/json/`
- Timeout 3s via `http.Client.Timeout`; no retry.
- Cache key `viacep:<cep>` TTL 1h.
- `{"erro": true}` → `domain.ErrCEPNotFound`.

### `internal/platform/passwords`

```go
const (
    argon2Memory  uint32 = 64 * 1024  // 64 MiB
    argon2Time    uint32 = 1
    argon2Threads uint8  = 4
    argon2KeyLen  uint32 = 32
    saltLen       int    = 16
)

func Hash(plain string) (encoded string, err error)
func Verify(plain, encoded string) (ok bool, err error)
```

Encoded format PHC: `$argon2id$v=19$m=65536,t=1,p=4$<base64-salt>$<base64-hash>`.

`DummyHash` constant exposed for constant-time login defense.

### `internal/platform/tokens`

```go
const opaqueTokenSize = 32

func Generate() (token string, hash []byte, err error)  // hex(32B random), sha256(rawBytes)
func Hash(token string) ([]byte, error)
```

## 6. Security Policy

### Password lifecycle

- Hash with argon2id (`golang.org/x/crypto/argon2`), params m=64MiB, t=1, p=4. Above OWASP 2024 minimum.
- Verify with `subtle.ConstantTimeCompare` after recompute.
- **Login constant-time defense:** when email not found, run `passwords.Verify("dummy", DummyHash)` to keep latency uniform (~150–300ms). Prevents timing-based account enumeration.
- Change password requires current password; revokes all other sessions on success (current preserved).
- Password reset confirms via single-use token; revokes all sessions on success (forces re-login on all devices).

### Token lifecycle (verify + reset)

- Generate via `crypto/rand` 32 bytes → hex (64 chars).
- Store SHA-256 hash as PRIMARY KEY in DB; never store plaintext.
- Single-use: `consumed_at` set at first valid use; subsequent attempts → `ErrTokenAlreadyUsed`.
- Expiry: verify 24h, reset 1h.
- Cleanup: river job `cleanup_expired_tokens` every 6h deletes tokens older than 7 days past expiry.

### Session lifecycle

- Created on login: new `sessionID`, new `csrf_token`, both `crypto/rand` 32B hex.
- TTL 14d default, 30d if `remember_me=true`.
- Sliding window refresh: middleware updates `last_activity_at` and renews TTL when activity > 24h ago.
- Logout: deletes Redis hash + removes ID from user index set; clears cookies.
- Logout-all: enumerates user index set, bulk delete; clears cookies.
- Forced revoke on password change/reset (with current-session preservation for change).

### CSRF

Double-submit cookie validates that `csrf_token` cookie matches `X-CSRF-Token` header. Additional defense: validates header token matches session-stored CSRF token (defeats cookie-only attacker if SameSite breaks).

`Origin` header check against `ALLOWED_ORIGINS` env list as defense-in-depth.

Public mutations (login, register, password-reset/*, verify-email, verify-email/resend): skip CSRF (no session yet); rely on SameSite=Lax + Origin check + rate limit.

### Rate limiting

Token bucket fixed window via Redis Lua atomic. Per-endpoint rules:

| Endpoint | Limits |
|---|---|
| POST /auth/login | 5/15min/IP + 5/15min/email |
| POST /auth/register | 3/hour/IP |
| POST /auth/verify-email/resend | 3/hour/email + 5/hour/IP |
| POST /auth/password-reset/request | 3/hour/email + 5/hour/IP |
| POST /auth/password-reset/confirm | 5/hour/IP |
| GET /auth/csrf | 60/min/IP |
| Authenticated routes (default cap) | 60/min/user |

`X-Forwarded-For` parsed only when request comes from `RATELIMIT_TRUSTED_PROXIES` (CIDR list).

### Cookies

- All cookies: `Secure` + `SameSite=Lax`.
- Session + cart_anon: `HttpOnly`. CSRF token: NOT HttpOnly (read by JS).
- Production: prefix `__Secure-` via `COOKIES_SECURE_PREFIX=true`.

### Anti-enumeration

- POST /auth/login: same status code (401 `invalid_credentials`) and latency for unknown email vs wrong password vs unverified. Email-not-verified surfaces only after correct password match.
- POST /auth/password-reset/request: always 202 regardless of email existence.
- POST /auth/verify-email/resend: always 202 regardless.
- POST /auth/register: 409 on email duplicate (accepted leak; rate limit + future CAPTCHA mitigates).

### Logging policy

Logged auth events (level + structured fields):

| Event | Level | Fields |
|---|---|---|
| login_success | info | user_id, ip, user_agent, session_id_hash |
| login_failed | warn | email, ip, user_agent, reason |
| login_rate_limited | warn | rule, key |
| register_success | info | user_id, email |
| register_duplicate | warn | email, ip |
| password_reset_requested | info | email, ip |
| password_reset_consumed | info | user_id, ip |
| password_changed | info | user_id, ip |
| email_verified | info | user_id, email |
| logout | info | user_id, session_id_hash |
| logout_all | info | user_id, session_count |
| csrf_failed | warn | path, reason |

Never logged: raw passwords, raw verify/reset tokens, full session_id (only first 8 chars + hash), full CSRF token.

## 7. Error Handling

### Sentinel errors per domain

```go
// internal/modules/identity/domain/errors.go
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

// internal/modules/cart/domain/errors.go
var (
    ErrCartNotFound    = errors.New("cart: not found")
    ErrItemNotFound    = errors.New("cart: item not found")
    ErrInvalidQuantity = errors.New("cart: invalid quantity")
    ErrVariantNotFound = errors.New("cart: variant not found")
)

// internal/modules/address/domain/errors.go
var (
    ErrAddressNotFound = errors.New("address: not found")
    ErrInvalidCEP      = errors.New("address: invalid cep")
    ErrCEPNotFound     = errors.New("address: cep not found")
)
```

### Wrapping convention

Repositories wrap with `fmt.Errorf("...: %w", err)`. Map `pgx.ErrNoRows` to domain sentinels at the boundary.

```go
func (r *PostgresUserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
    row, err := r.queries.FindUserByEmail(ctx, email)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, domain.ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("user repository: find by email: %w", err)
    }
    return mapUser(row), nil
}
```

### HTTP error mapping (transport boundary)

```go
func mapToHTTPStatus(err error) (status int, code string) {
    switch {
    case errors.Is(err, domain.ErrInvalidCredentials),
         errors.Is(err, domain.ErrSessionExpired),
         errors.Is(err, domain.ErrSessionNotFound):
        return 401, "invalid_credentials"
    case errors.Is(err, domain.ErrEmailNotVerified):
        return 403, "email_not_verified"
    case errors.Is(err, domain.ErrEmailAlreadyTaken):
        return 409, "email_already_taken"
    case errors.Is(err, domain.ErrTokenExpired),
         errors.Is(err, domain.ErrTokenAlreadyUsed),
         errors.Is(err, domain.ErrTokenNotFound):
        return 400, "invalid_token"
    case errors.Is(err, domain.ErrPasswordTooWeak):
        return 422, "password_policy"
    default:
        return 500, "internal_error"
    }
}
```

### Single handling rule

Errors propagate through layers (repo → service → handler) without logging. Logged once at transport boundary via `responsex.Error` extension. Internal `err.Error()` never exposed in HTTP response — only machine-readable `code` + user-friendly `message`.

5xx logs at `error` level with stack via `runtime.Stack`. 4xx logs at `warn` level.

### Panic policy

Panic only for:
- Constructor validation of mandatory config (e.g. `adminauth.RequireToken` panics if empty)
- `MustParse` in package init for compile-time constants

All other errors: returned. HTTP recovery middleware (`chi.Recoverer` already wired) captures runtime panics, returns 500, logs stack.

## 8. Testing Strategy

### Layer coverage

| Layer | Type | Tools | Target |
|---|---|---|---|
| Domain (errors, value objects) | Unit | testify | 100% |
| Application (services) | Unit | testify + mockery mocks | 80%+ |
| Infrastructure (repos) | Integration | testify + testcontainers Postgres | 90%+ |
| Platform (sessions, viacep, email) | Integration | testcontainers Redis + httptest | 90% |
| Transport (handlers) | HTTP integration | httptest + service fakes | 80% |
| End-to-end | Full stack | testcontainers (Postgres+Redis) | golden + 2 sad paths per feature |

### Test conventions

- Co-located `_test.go` files (Phase 1 pattern).
- Build tag `//go:build integration` for testcontainers tests.
- Commands:
  ```
  go test ./...                                         # unit
  go test -tags=integration -count=1 -timeout=15m ./... # integration + E2E
  go test -race ./...                                   # race detector
  ```

### Test helpers (`internal/testutil/`)

Existing: `postgres.go`, `redis.go`, `minio.go`. New for Phase 2:
- `email.go` — `FakeSender` capturing `Message` slice for assertions.
- `viacep.go` — `httptest.NewServer` returning fixture responses.
- `auth.go` — helpers `RegisterAndVerify(t, api)`, `Login(t, api, email, password)`, `WithCSRF(req, csrf)`.

### E2E suites

**Phase 2a — `TestIdentityE2E_*`:**
- `RegisterVerifyLogin` — register → assert email captured → verify → login → GET /me succeeds
- `LoginRateLimited` — 6 login attempts → 429 with `Retry-After`
- `PasswordResetFlow` — register+verify → request reset → consume token → old password fails (401) → new password works → all sessions revoked
- `LoginUnverifiedBlocked` — register without verify → login → 403 `email_not_verified`
- `ChangePassword` — login from 2 cookies → change-password on one → other 401 → current still valid
- `LogoutAll` — login from 2 cookies → DELETE /auth/sessions/all → both 401
- `CSRFRequired` — login → mutation without CSRF → 403; with valid CSRF → 200
- `LoginConstantTime` — measure login latency for unknown vs wrong password (within 2x of each other)

**Phase 2b — `TestCart`/`TestAddress`/`TestViaCEP` E2E:**
- `CartE2E_AnonAddRemove` — anon: add → assert cart_anon cookie + DB row → patch qty → delete
- `CartE2E_MergeOnLogin` — anon adds → register+verify → login → user cart has merged items → anon cart status=merged → cart_anon cookie cleared
- `CartE2E_QtyClamp` — qty=200 → 422
- `CartE2E_VariantDeleteBlockedByFK` (sad path) — admin attempts DELETE on `product_variants` row referenced by a `cart_items` row → expects PG `23503 foreign_key_violation`. Documents Phase 2 limitation: hard delete blocked. Phase 4 ships variant soft-delete to unblock admin while preserving historical cart context.
- `AddressE2E_CRUD` — login → POST → list → patch → delete → list empty
- `AddressE2E_DefaultUnique` — POST 2 with is_default=true → only latest is default (atomic tx)
- `ViaCEPE2E_LookupAndCache` — GET cep → GET cep again → mock server hit count == 1

### Mocks/fakes

- **Mockery** generates mocks for `UserRepository`, `Sender`, `sessionauth.Manager`, `viacep.Client`.
- Hand-written fakes: `email.FakeSender` (captures messages), `viacep.FakeClient` (returns fixtures).
- Service unit tests use mock repos + fake sender.

## 9. Sub-Phase Plans

### Phase 2a — Identity (~22 tasks)

Branch: `feat/phase-2a-identity` → tag `v0.4.0-identity` after E2E pass.

Order:
1. Migrations + sqlc queries for `users`, `auth_methods`, `email_verify_tokens`, `password_reset_tokens`
2. `internal/platform/passwords` — argon2id Hash/Verify + DummyHash
3. `internal/platform/tokens` — Generate/Hash
4. `internal/platform/email` — Sender interface + LogSender + SESSender + factory + templates
5. `internal/core/sessionauth` — Manager interface + RedisManager + Middleware + helpers
6. `internal/core/csrf` — double-submit + Origin check Middleware
7. `internal/core/ratelimit` — Rule + Middleware + realIP helper + Lua atomic INCR
8. `internal/modules/identity/domain` — User, AuthMethod, EmailVerifyToken, PasswordResetToken, sentinel errors
9. `internal/modules/identity/application` — IdentityService + ports
10. `internal/modules/identity/infrastructure` — Postgres repository impls
11. `internal/modules/identity/jobs` — `cleanup_expired_tokens` river job
12. `internal/modules/identity/transport` — auth_handlers (register, login, verify, password-reset, csrf, logout) + me_handlers (get/patch/change-password, sessions/all)
13. Wire into `cmd/api` — config extension, dependency wiring, route registration
14. OpenAPI spec extension (`api/openapi.yaml`) — auth + account tags
15. Unit tests per package
16. Integration tests for repositories + sessions
17. E2E suites listed in §8
18. `responsex.Error` extension for slog logging at transport boundary
19. Logging instrumentation per §6 logging policy
20. Smoke test endpoint `/admin/test` extended (existing pattern from Phase 1)
21. README env var documentation
22. Tag `v0.4.0-identity`

### Phase 2b — Commerce (~15 tasks)

Branch: `feat/phase-2b-commerce` → tag `v0.5.0-commerce` after E2E pass.

Order:
1. Migrations + sqlc queries for `addresses`, `carts`, `cart_items`
2. `internal/platform/viacep` — Client + Redis cache
3. `internal/modules/cart/domain` — Cart, CartItem, sentinel errors
4. `internal/modules/cart/application` — CartService + ports + AnonSessionResolver
5. `internal/modules/cart/infrastructure` — Postgres repository
6. `internal/modules/cart/jobs` — `cleanup_abandoned_carts` river job
7. `internal/modules/cart/transport` — cart_handlers + ResolveCartIdentity middleware
8. `internal/modules/address/domain` — Address, sentinel errors
9. `internal/modules/address/application` — AddressService + ports
10. `internal/modules/address/infrastructure` — Postgres repository
11. `internal/modules/address/transport` — address_handlers + cep_handler (via viacep)
12. Wire cart merge into Login handler (Phase 2a) — minimal patch
13. OpenAPI spec extension
14. Unit + integration + E2E tests
15. Tag `v0.5.0-commerce`

## 10. Skill References (for implementation tasks)

Per task type, consult skills before implementing:

| Task | Skills |
|---|---|
| Module structure / package layout | `cc-skills-golang:golang-project-layout`, `golang-naming` |
| Interfaces, structs, manual DI | `golang-structs-interfaces`, `golang-design-patterns` |
| Migrations, sqlc, transactions | `golang-database` |
| argon2 / crypto/rand / subtle / cookies | `golang-security` |
| Sentinel errors, wrap, slog | `golang-error-handling`, `golang-observability` |
| Context with timeouts on external calls | `golang-context` |
| Redis Lua atomic, locks, concurrency safety | `golang-concurrency` |
| Tests with testify + testcontainers | `golang-testing`, `golang-stretchr-testify` |
| chi handlers + middleware composition | `golang-design-patterns`, `golang-naming` |
| Defensive code (nil/index/numeric conversion) | `golang-safety` |
| HTTP client (ViaCEP) — timeout + SSRF | `golang-design-patterns`, `golang-security` |
| OpenAPI annotations (if maintained) | `golang-swagger` |

## 11. Configuration

New env vars (extend `internal/config/config.go`):

```
# Email
EMAIL_PROVIDER=log|ses                  # default: log
EMAIL_FROM_ADDRESS=...                  # required if provider=ses; LogSender uses placeholder
EMAIL_FROM_NAME=...                     # default: "Loja"
EMAIL_VERIFY_LINK_BASE_URL=...          # required (frontend URL used in template)
EMAIL_RESET_LINK_BASE_URL=...           # required

# SES (only if provider=ses)
SES_REGION=us-east-1                    # required if provider=ses
SES_CONFIGURATION_SET=                  # optional

# Sessions
SESSION_TTL_DEFAULT=336h                # 14d
SESSION_TTL_REMEMBER_ME=720h            # 30d
SESSION_REFRESH_AFTER=24h

# CSRF
CSRF_ALLOWED_ORIGINS=http://localhost:3000

# Rate limit
RATELIMIT_TRUSTED_PROXIES=              # CIDR list, comma-separated

# Cookies
COOKIES_SECURE_PREFIX=false             # true in production

# ViaCEP
VIACEP_BASE_URL=https://viacep.com.br/ws
VIACEP_TIMEOUT=3s
VIACEP_CACHE_TTL=1h
```

Existing env vars (`DATABASE_URL`, `REDIS_ADDR`, `STORAGE_*`, `ADMIN_API_TOKEN`, etc.) unchanged.
