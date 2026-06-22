# Phase 2b — Commerce (Cart + Address + ViaCEP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a server-side cart (anonymous + user, with merge on login), per-user shipping addresses (CRUD + atomic default), and a cached ViaCEP postal-code lookup — completing Phase 2's commerce surface.

**Architecture:** Two new bounded contexts (`internal/modules/cart`, `internal/modules/address`) following the existing five-layer module pattern (domain → application → infrastructure → transport, plus jobs). A new `internal/platform/viacep` adapter wraps the ViaCEP HTTP API behind a Redis cache. Cart is reachable by anonymous visitors (cookie `cart_anon`) or logged-in users; on login the anon cart merges into the user cart. Addresses live under the authenticated `/me/addresses` surface. Wiring stays manual constructor injection in `cmd/api/main.go`; a minimal patch to the identity Login handler triggers the cart merge via an injected callback (no identity→cart type coupling).

**Tech Stack:** Go 1.25, chi v5, pgx v5 + sqlc, Atlas migrations, Redis 7 (go-redis v9), river (jobs), testify + testcontainers-go.

## Global Constraints

- Module name: `github.com/danilloboing/marketplace-golang`.
- Package naming (per `cc-skills-golang:golang-naming`): lowercase single word; no stutter (`viacep.Client`, not `viacep.ViaCEPClient`); single-method interfaces use `-er`; constructors `New` / `NewXxx`; sentinel errors `domain.ErrXxx` with package-prefixed message `"cart: ..."` / `"address: ..."`.
- One primary export per file. Co-located `_test.go`. Integration tests gated by `//go:build integration`.
- Errors propagate un-logged through layers; logged once at the transport boundary via `responsex.ErrorWithCause`. Internal `err.Error()` never appears in an HTTP body — only `{code, message}`.
- Repositories wrap with `fmt.Errorf("<repo>: <op>: %w", err)` and map `pgx.ErrNoRows` to a domain sentinel at the boundary.
- Money is in cents, type **`BIGINT`/`int64`** end-to-end (matches `catalog_variants.price_cents` and `catalog_products.base_price_cents`).
- HTTP status policy: 200 body / 201 created / 204 no-content / 400 bad payload / 401 unauth / 403 CSRF·forbidden / 404 missing (also cross-user) / 422 semantic (qty>99, default conflict) / 429 rate-limited / 500 unexpected.
- Commit after every green step using Conventional Commits.

## Spec Corrections (read before starting)

The joint design spec (`docs/superpowers/specs/2026-05-09-phase-2-identity-cart-design.md`) was written before Phase 2a code existed. Three deviations are intentional and baked into the tasks below:

1. **Variant FK** — the spec migration references `product_variants(id)`. The real table (Phase 1) is **`catalog_variants`**. `cart_items.variant_id` references `catalog_variants(id) ON DELETE RESTRICT`.
2. **`unit_price_cents` type** — spec says `INT`; this plan uses **`BIGINT`** so the price snapshot (`COALESCE(catalog_variants.price_cents, catalog_products.base_price_cents)`, both `BIGINT`) maps cleanly to `int64` with no narrowing.
3. **Error rendering** — `responsex.WriteError(w, err)` only classifies *catalog* domain errors; do **not** use it in cart/address. Use `responsex.Error` / `responsex.ErrorWithCause` with an explicit status+code, fed by a per-module `mapErrorToHTTP` mirroring `internal/modules/identity/transport/error_mapping.go`.
4. **ViaCEP config already exists** — `config.ViaCEP{BaseURL, Timeout, CacheTTL}` is already in `internal/config/config.go`. No config change needed for ViaCEP. Cart adds one small `config.Cart` section (Task 6).

## File Structure

**New files (created):**

```
db/migrations/20260611000001_commerce.sql          # addresses, carts, cart_items
db/queries/addresses.sql                           # sqlc source
db/queries/carts.sql                               # sqlc source
db/queries/cart_items.sql                          # sqlc source

internal/platform/viacep/viacep.go                 # Client + Lookup + Redis cache
internal/platform/viacep/viacep_test.go
internal/platform/viacep/fake.go                   # FakeClient for service/E2E tests

internal/modules/cart/domain/cart.go               # Cart, CartItem value types
internal/modules/cart/domain/errors.go             # sentinel errors
internal/modules/cart/domain/cart_test.go
internal/modules/cart/application/ports.go         # CartRepository, AnonSessionResolver
internal/modules/cart/application/cart_service.go   # CartService
internal/modules/cart/application/cart_service_test.go
internal/modules/cart/infrastructure/cart_repository.go
internal/modules/cart/infrastructure/mappers.go
internal/modules/cart/infrastructure/cart_repository_test.go
internal/modules/cart/jobs/cleanup_abandoned_carts.go
internal/modules/cart/jobs/cleanup_abandoned_carts_test.go
internal/modules/cart/transport/cart_handlers.go
internal/modules/cart/transport/identity_middleware.go   # ResolveCartIdentity
internal/modules/cart/transport/responses.go
internal/modules/cart/transport/error_mapping.go
internal/modules/cart/transport/cart_handlers_test.go
internal/modules/cart/module.go

internal/modules/address/domain/address.go
internal/modules/address/domain/errors.go
internal/modules/address/domain/address_test.go
internal/modules/address/application/ports.go
internal/modules/address/application/address_service.go
internal/modules/address/application/address_service_test.go
internal/modules/address/infrastructure/address_repository.go
internal/modules/address/infrastructure/mappers.go
internal/modules/address/infrastructure/address_repository_test.go
internal/modules/address/transport/address_handlers.go
internal/modules/address/transport/cep_handler.go
internal/modules/address/transport/responses.go
internal/modules/address/transport/error_mapping.go
internal/modules/address/transport/address_handlers_test.go
internal/modules/address/module.go

internal/testutil/viacep.go                        # httptest fixture server helper

tests/integration/cart_e2e_test.go
tests/integration/address_e2e_test.go
tests/integration/viacep_e2e_test.go
tests/integration/commerce_support_test.go         # shared E2E boot for cart+address
```

**Modified files:**

```
internal/config/config.go                          # + Cart section (Task 6)
internal/platform/postgres/queries/*               # regenerated by `make sqlc-gen`
internal/modules/identity/transport/auth_handlers.go  # Login: trigger cart merge (Task 12)
internal/modules/identity/module.go                # Deps: CartMerge + CartCookieName (Task 12)
cmd/api/main.go                                     # wire viacep, cart, address, cart cookie (Task 12)
cmd/worker/main.go                                  # register cleanup_abandoned_carts (Task 6)
api/openapi.yaml                                    # cart + address tags (Task 13)
README.md                                           # env vars (Task 15)
```

---

## Task 1: Commerce migration + sqlc queries

**Files:**
- Create: `db/migrations/20260611000001_commerce.sql`
- Create: `db/queries/addresses.sql`, `db/queries/carts.sql`, `db/queries/cart_items.sql`
- Modify (generated): `internal/platform/postgres/queries/*`

**Interfaces:**
- Produces: sqlc `queries.Queries` methods used by later repository tasks — `CreateAddress`, `GetAddressByID`, `ListAddressesByUser`, `UpdateAddress`, `DeleteAddress`, `ClearDefaultAddress`, `SetDefaultAddress`, `GetActiveCartByUser`, `GetActiveCartByAnon`, `CreateUserCart`, `CreateAnonCart`, `SetCartStatus`, `GetVariantUnitPrice`, `UpsertCartItem`, `ListCartItems`, `GetCartItemByID`, `UpdateCartItemQuantity`, `DeleteCartItem`, `DeleteCartItemsByCart`, `CountActiveItems`, `DeleteAbandonedCarts`.

- [ ] **Step 1: Write the migration file**

Create `db/migrations/20260611000001_commerce.sql`:

```sql
-- Commerce tables: addresses + carts + cart_items.

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

CREATE TABLE carts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,
    anon_session_id TEXT,
    status          TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','merged','abandoned','converted')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
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

CREATE TABLE cart_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id          UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id       UUID NOT NULL REFERENCES catalog_variants(id) ON DELETE RESTRICT,
    quantity         INT NOT NULL CHECK (quantity > 0 AND quantity <= 99),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents >= 0),
    added_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cart_items_cart_variant_uniq ON cart_items(cart_id, variant_id);
```

- [ ] **Step 2: Hash the migration into the Atlas sum**

Run: `atlas migrate hash --dir file://db/migrations`
Expected: `db/migrations/atlas.sum` updated with the new file; no error.

- [ ] **Step 3: Write the addresses queries**

Create `db/queries/addresses.sql`:

```sql
-- name: CreateAddress :one
INSERT INTO addresses (
    id, user_id, recipient_name, postal_code, street, number,
    complement, neighborhood, city, state, is_default
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAddressByID :one
SELECT * FROM addresses WHERE id = $1 AND user_id = $2;

-- name: ListAddressesByUser :many
SELECT * FROM addresses WHERE user_id = $1 ORDER BY is_default DESC, created_at DESC;

-- name: UpdateAddress :one
UPDATE addresses
SET recipient_name = $3, postal_code = $4, street = $5, number = $6,
    complement = $7, neighborhood = $8, city = $9, state = $10, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteAddress :execrows
DELETE FROM addresses WHERE id = $1 AND user_id = $2;

-- name: ClearDefaultAddress :exec
UPDATE addresses SET is_default = FALSE, updated_at = now()
WHERE user_id = $1 AND is_default = TRUE;

-- name: SetDefaultAddress :one
UPDATE addresses SET is_default = TRUE, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;
```

- [ ] **Step 4: Write the carts queries**

Create `db/queries/carts.sql`:

```sql
-- name: GetActiveCartByUser :one
SELECT * FROM carts WHERE user_id = $1 AND status = 'active';

-- name: GetActiveCartByAnon :one
SELECT * FROM carts WHERE anon_session_id = $1 AND status = 'active';

-- name: CreateUserCart :one
INSERT INTO carts (id, user_id, status)
VALUES (gen_random_uuid(), $1, 'active')
RETURNING *;

-- name: CreateAnonCart :one
INSERT INTO carts (id, anon_session_id, status)
VALUES (gen_random_uuid(), $1, 'active')
RETURNING *;

-- name: SetCartStatus :exec
UPDATE carts SET status = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAbandonedCarts :execrows
UPDATE carts SET status = 'abandoned', updated_at = now()
WHERE status = 'active' AND updated_at < $1 AND user_id IS NULL;
```

- [ ] **Step 5: Write the cart_items queries**

Create `db/queries/cart_items.sql`:

```sql
-- name: GetVariantUnitPrice :one
SELECT COALESCE(cv.price_cents, cp.base_price_cents)::bigint AS unit_price_cents
FROM catalog_variants cv
JOIN catalog_products cp ON cp.id = cv.product_id
WHERE cv.id = $1;

-- name: UpsertCartItem :one
INSERT INTO cart_items (id, cart_id, variant_id, quantity, unit_price_cents)
VALUES (gen_random_uuid(), $1, $2, $3, $4)
ON CONFLICT (cart_id, variant_id) DO UPDATE
SET quantity = LEAST(cart_items.quantity + EXCLUDED.quantity, 99),
    unit_price_cents = EXCLUDED.unit_price_cents,
    updated_at = now()
RETURNING *;

-- name: ListCartItems :many
SELECT * FROM cart_items WHERE cart_id = $1 ORDER BY added_at;

-- name: GetCartItemByID :one
SELECT ci.* FROM cart_items ci
JOIN carts c ON c.id = ci.cart_id
WHERE ci.id = $1 AND c.id = $2;

-- name: UpdateCartItemQuantity :one
UPDATE cart_items SET quantity = $3, updated_at = now()
WHERE id = $1 AND cart_id = $2
RETURNING *;

-- name: DeleteCartItem :execrows
DELETE FROM cart_items WHERE id = $1 AND cart_id = $2;

-- name: DeleteCartItemsByCart :exec
DELETE FROM cart_items WHERE cart_id = $1;

-- name: CountActiveItems :one
SELECT COUNT(*) FROM cart_items WHERE cart_id = $1;
```

> Note: `UpsertCartItem` clamps to 99 on conflict via `LEAST(...)`; the service still validates the requested quantity up-front so an explicit over-cap request returns 422 rather than silently clamping. The `::bigint` cast on `GetVariantUnitPrice` forces sqlc to emit `int64` (not `interface{}`).

- [ ] **Step 6: Regenerate sqlc code**

Run: `make sqlc-gen`
Expected: new methods appear in `internal/platform/postgres/queries/` (`addresses.sql.go`, `carts.sql.go`, `cart_items.sql.go`, updated `querier.go` + `models.go` with `Address`, `Cart`, `CartItem`). No error.

- [ ] **Step 7: Verify it compiles**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 8: Commit**

```bash
git add db/migrations/20260611000001_commerce.sql db/migrations/atlas.sum db/queries/ internal/platform/postgres/queries/
git commit -m "feat(db): add commerce tables, queries, and sqlc generation"
```

---

## Task 2: `internal/platform/viacep` — Client + Redis cache + FakeClient

**Files:**
- Create: `internal/platform/viacep/viacep.go`, `internal/platform/viacep/fake.go`
- Test: `internal/platform/viacep/viacep_test.go`

**Interfaces:**
- Produces:
  - `type Address struct { PostalCode, Street, Neighborhood, City, State string }`
  - `type Lookuper interface { Lookup(ctx context.Context, cep string) (Address, error) }`
  - `func NewClient(httpClient *http.Client, cache *redis.Client, baseURL string, cacheTTL time.Duration) *Client`
  - `func (c *Client) Lookup(ctx context.Context, cep string) (Address, error)`
  - sentinel `var ErrCEPNotFound = errors.New("viacep: cep not found")` and `var ErrInvalidCEP = errors.New("viacep: invalid cep")`
  - `type FakeClient struct { Responses map[string]Address; Err error; Calls map[string]int }` implementing `Lookuper`
- Consumes: `config.ViaCEP` (already exists).

- [ ] **Step 1: Write the failing test**

Create `internal/platform/viacep/viacep_test.go`:

```go
package viacep_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestClient_Lookup_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/01001000/json/", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"Praça da Sé","bairro":"Sé","localidade":"São Paulo","uf":"SP"}`))
	}))
	defer srv.Close()

	c := viacep.NewClient(srv.Client(), newTestRedis(t), srv.URL, time.Hour)
	addr, err := c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	assert.Equal(t, "01001000", addr.PostalCode)
	assert.Equal(t, "Praça da Sé", addr.Street)
	assert.Equal(t, "Sé", addr.Neighborhood)
	assert.Equal(t, "São Paulo", addr.City)
	assert.Equal(t, "SP", addr.State)
}

func TestClient_Lookup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"erro": true}`))
	}))
	defer srv.Close()

	c := viacep.NewClient(srv.Client(), newTestRedis(t), srv.URL, time.Hour)
	_, err := c.Lookup(context.Background(), "00000000")
	require.ErrorIs(t, err, viacep.ErrCEPNotFound)
}

func TestClient_Lookup_InvalidCEP(t *testing.T) {
	c := viacep.NewClient(http.DefaultClient, newTestRedis(t), "http://unused", time.Hour)
	_, err := c.Lookup(context.Background(), "123")
	require.ErrorIs(t, err, viacep.ErrInvalidCEP)
}

func TestClient_Lookup_CachesResult(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"X","bairro":"Y","localidade":"Z","uf":"SP"}`))
	}))
	defer srv.Close()

	c := viacep.NewClient(srv.Client(), newTestRedis(t), srv.URL, time.Hour)
	_, err := c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	_, err = c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	assert.Equal(t, 1, hits, "second lookup must be served from cache")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/viacep/...`
Expected: FAIL — package `viacep` does not compile (Client undefined). (If `miniredis` is missing: `go get github.com/alicebob/miniredis/v2@latest` then `go mod tidy`.)

- [ ] **Step 3: Write the implementation**

Create `internal/platform/viacep/viacep.go`:

```go
// Package viacep wraps the ViaCEP postal-code API behind a Redis cache.
package viacep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

// Sentinel errors.
var (
	ErrCEPNotFound = errors.New("viacep: cep not found")
	ErrInvalidCEP  = errors.New("viacep: invalid cep")
)

var cepPattern = regexp.MustCompile(`^[0-9]{8}$`)

// Address is a resolved postal address (subset of the ViaCEP payload).
type Address struct {
	PostalCode   string
	Street       string
	Neighborhood string
	City         string
	State        string
}

// Lookuper resolves a CEP to an Address. Implemented by *Client and *FakeClient.
type Lookuper interface {
	Lookup(ctx context.Context, cep string) (Address, error)
}

// Client is the cached ViaCEP HTTP client.
type Client struct {
	httpClient *http.Client
	cache      *redis.Client
	baseURL    string
	cacheTTL   time.Duration
}

var _ Lookuper = (*Client)(nil)

// NewClient builds a Client. baseURL has no trailing slash (e.g. https://viacep.com.br/ws).
func NewClient(httpClient *http.Client, cache *redis.Client, baseURL string, cacheTTL time.Duration) *Client {
	return &Client{httpClient: httpClient, cache: cache, baseURL: baseURL, cacheTTL: cacheTTL}
}

type viacepResponse struct {
	Cep         string `json:"cep"`
	Logradouro  string `json:"logradouro"`
	Bairro      string `json:"bairro"`
	Localidade  string `json:"localidade"`
	UF          string `json:"uf"`
	Erro        any    `json:"erro"`
}

// Lookup resolves cep (8 digits, no mask). Cache hit short-circuits the HTTP call.
func (c *Client) Lookup(ctx context.Context, cep string) (Address, error) {
	if !cepPattern.MatchString(cep) {
		return Address{}, ErrInvalidCEP
	}

	cacheKey := "viacep:" + cep
	if raw, err := c.cache.Get(ctx, cacheKey).Bytes(); err == nil {
		var cached Address
		if json.Unmarshal(raw, &cached) == nil {
			return cached, nil
		}
	}

	url := fmt.Sprintf("%s/%s/json/", c.baseURL, cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Address{}, fmt.Errorf("viacep: build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Address{}, fmt.Errorf("viacep: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Address{}, fmt.Errorf("viacep: unexpected status %d", resp.StatusCode)
	}

	var body viacepResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Address{}, fmt.Errorf("viacep: decode: %w", err)
	}
	if body.Erro != nil && body.Erro != false {
		return Address{}, ErrCEPNotFound
	}

	addr := Address{
		PostalCode:   cep,
		Street:       body.Logradouro,
		Neighborhood: body.Bairro,
		City:         body.Localidade,
		State:        body.UF,
	}

	if encoded, err := json.Marshal(addr); err == nil {
		_ = c.cache.Set(ctx, cacheKey, encoded, c.cacheTTL).Err()
	}

	return addr, nil
}
```

- [ ] **Step 4: Write the FakeClient**

Create `internal/platform/viacep/fake.go`:

```go
package viacep

import "context"

// FakeClient is a test double for Lookuper.
type FakeClient struct {
	Responses map[string]Address
	Err       error
	Calls     map[string]int
}

var _ Lookuper = (*FakeClient)(nil)

// NewFakeClient builds an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{Responses: map[string]Address{}, Calls: map[string]int{}}
}

// Lookup returns a canned Address or Err.
func (f *FakeClient) Lookup(_ context.Context, cep string) (Address, error) {
	if f.Calls == nil {
		f.Calls = map[string]int{}
	}
	f.Calls[cep]++
	if f.Err != nil {
		return Address{}, f.Err
	}
	addr, ok := f.Responses[cep]
	if !ok {
		return Address{}, ErrCEPNotFound
	}
	return addr, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/platform/viacep/...`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/platform/viacep/ go.mod go.sum
git commit -m "feat(viacep): add cached ViaCEP client with Lookuper interface and fake"
```

---

## Task 3: `internal/modules/cart/domain` — Cart, CartItem, sentinel errors

**Files:**
- Create: `internal/modules/cart/domain/cart.go`, `internal/modules/cart/domain/errors.go`
- Test: `internal/modules/cart/domain/cart_test.go`

**Interfaces:**
- Produces:
  - `type CartItem struct { ID, VariantID uuid.UUID; Quantity int; UnitPriceCents int64 }`
  - `type Cart struct { ID uuid.UUID; UserID *uuid.UUID; AnonSessionID *string; Status Status; Items []CartItem }`
  - `func (c Cart) SubtotalCents() int64`
  - `type Status string` with consts `StatusActive/StatusMerged/StatusAbandoned/StatusConverted`
  - `const MaxItemQuantity = 99`
  - `func ValidateQuantity(q int) error`
  - sentinels: `ErrCartNotFound`, `ErrItemNotFound`, `ErrInvalidQuantity`, `ErrVariantNotFound`

- [ ] **Step 1: Write the failing test**

Create `internal/modules/cart/domain/cart_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

func TestCart_SubtotalCents(t *testing.T) {
	c := domain.Cart{Items: []domain.CartItem{
		{ID: uuid.New(), VariantID: uuid.New(), Quantity: 2, UnitPriceCents: 1500},
		{ID: uuid.New(), VariantID: uuid.New(), Quantity: 3, UnitPriceCents: 1000},
	}}
	assert.Equal(t, int64(6000), c.SubtotalCents())
}

func TestCart_SubtotalCents_Empty(t *testing.T) {
	assert.Equal(t, int64(0), domain.Cart{}.SubtotalCents())
}

func TestValidateQuantity(t *testing.T) {
	require.NoError(t, domain.ValidateQuantity(1))
	require.NoError(t, domain.ValidateQuantity(99))
	require.ErrorIs(t, domain.ValidateQuantity(0), domain.ErrInvalidQuantity)
	require.ErrorIs(t, domain.ValidateQuantity(-1), domain.ErrInvalidQuantity)
	require.ErrorIs(t, domain.ValidateQuantity(100), domain.ErrInvalidQuantity)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modules/cart/domain/...`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write the errors file**

Create `internal/modules/cart/domain/errors.go`:

```go
package domain

import "errors"

// Sentinel errors for the cart bounded context.
var (
	ErrCartNotFound    = errors.New("cart: not found")
	ErrItemNotFound    = errors.New("cart: item not found")
	ErrInvalidQuantity = errors.New("cart: invalid quantity")
	ErrVariantNotFound = errors.New("cart: variant not found")
)
```

- [ ] **Step 4: Write the domain types**

Create `internal/modules/cart/domain/cart.go`:

```go
// Package domain holds cart value types and invariants.
package domain

import "github.com/google/uuid"

// MaxItemQuantity is the hard per-line quantity cap (schema enforces the same).
const MaxItemQuantity = 99

// Status is a cart lifecycle state.
type Status string

// Cart lifecycle states.
const (
	StatusActive    Status = "active"
	StatusMerged    Status = "merged"
	StatusAbandoned Status = "abandoned"
	StatusConverted Status = "converted"
)

// CartItem is a single line in a cart with a price snapshot.
type CartItem struct {
	ID             uuid.UUID
	VariantID      uuid.UUID
	Quantity       int
	UnitPriceCents int64
}

// Cart is an active shopping cart owned by a user OR an anonymous session.
type Cart struct {
	ID            uuid.UUID
	UserID        *uuid.UUID
	AnonSessionID *string
	Status        Status
	Items         []CartItem
}

// SubtotalCents sums line totals (unit price × quantity) across all items.
func (c Cart) SubtotalCents() int64 {
	var total int64
	for _, it := range c.Items {
		total += it.UnitPriceCents * int64(it.Quantity)
	}
	return total
}

// ValidateQuantity enforces 1..MaxItemQuantity.
func ValidateQuantity(q int) error {
	if q < 1 || q > MaxItemQuantity {
		return ErrInvalidQuantity
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/modules/cart/domain/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/modules/cart/domain/
git commit -m "feat(cart): add domain types, subtotal, quantity rule, and sentinel errors"
```

---
## Task 4: `internal/modules/cart/application` — CartService + ports

**Files:**
- Create: `internal/modules/cart/application/ports.go`, `internal/modules/cart/application/cart_service.go`
- Test: `internal/modules/cart/application/cart_service_test.go`

**Interfaces:**
- Consumes (Task 3): `domain.Cart`, `domain.CartItem`, `domain.Owner`, `domain.ValidateQuantity`, sentinels.
- Produces:
  - `type Owner = domain.Owner` (re-export not needed; service takes `domain.Owner`)
  - `type CartRepository interface { FindActive(ctx, domain.Owner)(domain.Cart,error); EnsureActive(ctx, domain.Owner)(domain.Cart,error); VariantUnitPrice(ctx, uuid.UUID)(int64,error); UpsertItem(ctx, cartID, variantID uuid.UUID, qty int, unitPrice int64) error; UpdateItemQuantity(ctx, cartID, itemID uuid.UUID, qty int) error; DeleteItem(ctx, cartID, itemID uuid.UUID) error; ClearItems(ctx, cartID uuid.UUID) error; Merge(ctx, anonID string, userID uuid.UUID) error }`
  - `type CartService struct{...}`, `func NewCartService(CartRepository) *CartService`
  - methods: `Get(ctx, domain.Owner)(domain.Cart,error)`, `AddItem(ctx, domain.Owner, variantID uuid.UUID, qty int)(domain.Cart,error)`, `UpdateItem(ctx, domain.Owner, itemID uuid.UUID, qty int)(domain.Cart,error)`, `RemoveItem(ctx, domain.Owner, itemID uuid.UUID)(domain.Cart,error)`, `Clear(ctx, domain.Owner) error`, `Merge(ctx, anonID string, userID uuid.UUID) error`

- [ ] **Step 1: Add `Owner` to the domain package**

Append to `internal/modules/cart/domain/cart.go`:

```go
// Owner identifies who a cart belongs to: exactly one of UserID or AnonID is set.
type Owner struct {
	UserID *uuid.UUID
	AnonID *string
}

// Valid reports whether exactly one identity is present.
func (o Owner) Valid() bool {
	return (o.UserID != nil) != (o.AnonID != nil)
}
```

- [ ] **Step 2: Write the failing service test**

Create `internal/modules/cart/application/cart_service_test.go`:

```go
package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// fakeRepo is an in-memory CartRepository for service unit tests.
type fakeRepo struct {
	cart        domain.Cart
	hasCart     bool
	prices      map[uuid.UUID]int64
	priceErr    error
	mergeCalled bool
}

func (f *fakeRepo) FindActive(_ context.Context, _ domain.Owner) (domain.Cart, error) {
	if !f.hasCart {
		return domain.Cart{}, domain.ErrCartNotFound
	}
	return f.cart, nil
}

func (f *fakeRepo) EnsureActive(_ context.Context, owner domain.Owner) (domain.Cart, error) {
	if !f.hasCart {
		f.cart = domain.Cart{ID: uuid.New(), UserID: owner.UserID, AnonSessionID: owner.AnonID, Status: domain.StatusActive}
		f.hasCart = true
	}
	return f.cart, nil
}

func (f *fakeRepo) VariantUnitPrice(_ context.Context, id uuid.UUID) (int64, error) {
	if f.priceErr != nil {
		return 0, f.priceErr
	}
	p, ok := f.prices[id]
	if !ok {
		return 0, domain.ErrVariantNotFound
	}
	return p, nil
}

func (f *fakeRepo) UpsertItem(_ context.Context, cartID, variantID uuid.UUID, qty int, unitPrice int64) error {
	f.cart.Items = append(f.cart.Items, domain.CartItem{ID: uuid.New(), VariantID: variantID, Quantity: qty, UnitPriceCents: unitPrice})
	return nil
}

func (f *fakeRepo) UpdateItemQuantity(_ context.Context, cartID, itemID uuid.UUID, qty int) error {
	for i := range f.cart.Items {
		if f.cart.Items[i].ID == itemID {
			f.cart.Items[i].Quantity = qty
			return nil
		}
	}
	return domain.ErrItemNotFound
}

func (f *fakeRepo) DeleteItem(_ context.Context, cartID, itemID uuid.UUID) error {
	for i := range f.cart.Items {
		if f.cart.Items[i].ID == itemID {
			f.cart.Items = append(f.cart.Items[:i], f.cart.Items[i+1:]...)
			return nil
		}
	}
	return domain.ErrItemNotFound
}

func (f *fakeRepo) ClearItems(_ context.Context, _ uuid.UUID) error { f.cart.Items = nil; return nil }
func (f *fakeRepo) Merge(_ context.Context, _ string, _ uuid.UUID) error {
	f.mergeCalled = true
	return nil
}

func anonOwner() domain.Owner { id := "anon123"; return domain.Owner{AnonID: &id} }

func TestCartService_AddItem_Success(t *testing.T) {
	variant := uuid.New()
	repo := &fakeRepo{prices: map[uuid.UUID]int64{variant: 2500}}
	svc := application.NewCartService(repo)

	cart, err := svc.AddItem(context.Background(), anonOwner(), variant, 2)
	require.NoError(t, err)
	require.Len(t, cart.Items, 1)
	assert.Equal(t, int64(2500), cart.Items[0].UnitPriceCents)
	assert.Equal(t, int64(5000), cart.SubtotalCents())
}

func TestCartService_AddItem_QuantityOverCap(t *testing.T) {
	repo := &fakeRepo{prices: map[uuid.UUID]int64{}}
	svc := application.NewCartService(repo)
	_, err := svc.AddItem(context.Background(), anonOwner(), uuid.New(), 200)
	require.ErrorIs(t, err, domain.ErrInvalidQuantity)
}

func TestCartService_AddItem_UnknownVariant(t *testing.T) {
	repo := &fakeRepo{prices: map[uuid.UUID]int64{}}
	svc := application.NewCartService(repo)
	_, err := svc.AddItem(context.Background(), anonOwner(), uuid.New(), 1)
	require.ErrorIs(t, err, domain.ErrVariantNotFound)
}

func TestCartService_Get_NoCartReturnsEmpty(t *testing.T) {
	svc := application.NewCartService(&fakeRepo{})
	cart, err := svc.Get(context.Background(), anonOwner())
	require.NoError(t, err)
	assert.Empty(t, cart.Items)
}

func TestCartService_UpdateItem_NotFound(t *testing.T) {
	repo := &fakeRepo{hasCart: true, cart: domain.Cart{ID: uuid.New(), Status: domain.StatusActive}}
	svc := application.NewCartService(repo)
	_, err := svc.UpdateItem(context.Background(), anonOwner(), uuid.New(), 3)
	require.ErrorIs(t, err, domain.ErrItemNotFound)
}

func TestCartService_Merge_Delegates(t *testing.T) {
	repo := &fakeRepo{}
	svc := application.NewCartService(repo)
	require.NoError(t, svc.Merge(context.Background(), "anon123", uuid.New()))
	assert.True(t, repo.mergeCalled)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/modules/cart/application/...`
Expected: FAIL — package does not compile.

- [ ] **Step 4: Write ports**

Create `internal/modules/cart/application/ports.go`:

```go
// Package application contains cart use cases and ports.
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// CartRepository is the persistence contract for carts and their items.
// FindActive/EnsureActive return a Cart with Items loaded. EnsureActive
// lazily creates an active cart for the owner if none exists.
type CartRepository interface {
	FindActive(ctx context.Context, owner domain.Owner) (domain.Cart, error)
	EnsureActive(ctx context.Context, owner domain.Owner) (domain.Cart, error)
	VariantUnitPrice(ctx context.Context, variantID uuid.UUID) (int64, error)
	UpsertItem(ctx context.Context, cartID, variantID uuid.UUID, qty int, unitPrice int64) error
	UpdateItemQuantity(ctx context.Context, cartID, itemID uuid.UUID, qty int) error
	DeleteItem(ctx context.Context, cartID, itemID uuid.UUID) error
	ClearItems(ctx context.Context, cartID uuid.UUID) error
	Merge(ctx context.Context, anonID string, userID uuid.UUID) error
}
```

- [ ] **Step 5: Write the service**

Create `internal/modules/cart/application/cart_service.go`:

```go
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// CartService orchestrates cart read/write flows.
type CartService struct {
	repo CartRepository
}

// NewCartService builds a CartService.
func NewCartService(repo CartRepository) *CartService {
	return &CartService{repo: repo}
}

// Get returns the owner's active cart, or an empty cart when none exists.
func (s *CartService) Get(ctx context.Context, owner domain.Owner) (domain.Cart, error) {
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		if err == domain.ErrCartNotFound {
			return domain.Cart{}, nil
		}
		return domain.Cart{}, err
	}
	return cart, nil
}

// AddItem validates quantity, snapshots the variant price, lazily creates the
// cart, and upserts the line (summing on conflict).
func (s *CartService) AddItem(ctx context.Context, owner domain.Owner, variantID uuid.UUID, qty int) (domain.Cart, error) {
	if err := domain.ValidateQuantity(qty); err != nil {
		return domain.Cart{}, err
	}
	price, err := s.repo.VariantUnitPrice(ctx, variantID)
	if err != nil {
		return domain.Cart{}, err
	}
	cart, err := s.repo.EnsureActive(ctx, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := s.repo.UpsertItem(ctx, cart.ID, variantID, qty, price); err != nil {
		return domain.Cart{}, err
	}
	return s.repo.FindActive(ctx, owner)
}

// UpdateItem sets a line quantity. Cross-cart item IDs surface as ErrItemNotFound.
func (s *CartService) UpdateItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID, qty int) (domain.Cart, error) {
	if err := domain.ValidateQuantity(qty); err != nil {
		return domain.Cart{}, err
	}
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := s.repo.UpdateItemQuantity(ctx, cart.ID, itemID, qty); err != nil {
		return domain.Cart{}, err
	}
	return s.repo.FindActive(ctx, owner)
}

// RemoveItem drops a line from the cart.
func (s *CartService) RemoveItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID) (domain.Cart, error) {
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := s.repo.DeleteItem(ctx, cart.ID, itemID); err != nil {
		return domain.Cart{}, err
	}
	return s.repo.FindActive(ctx, owner)
}

// Clear removes all items from the owner's cart. No-op when no cart exists.
func (s *CartService) Clear(ctx context.Context, owner domain.Owner) error {
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		if err == domain.ErrCartNotFound {
			return nil
		}
		return err
	}
	return s.repo.ClearItems(ctx, cart.ID)
}

// Merge folds an anonymous cart into the user's active cart (summing lines)
// and marks the anon cart merged. No-op when the anon cart is empty/absent.
func (s *CartService) Merge(ctx context.Context, anonID string, userID uuid.UUID) error {
	return s.repo.Merge(ctx, anonID, userID)
}
```

> The `err == domain.ErrCartNotFound` comparisons are intentional direct equality: the repository returns the sentinel unwrapped at its boundary (Task 5). Use `errors.Is` instead if the repo ever wraps it.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/modules/cart/application/...`
Expected: PASS (6 tests).

- [ ] **Step 7: Commit**

```bash
git add internal/modules/cart/domain/cart.go internal/modules/cart/application/
git commit -m "feat(cart): add CartService, ports, and Owner identity"
```

---

## Task 5: `internal/modules/cart/infrastructure` — Postgres CartRepository

**Files:**
- Create: `internal/modules/cart/infrastructure/cart_repository.go`, `internal/modules/cart/infrastructure/mappers.go`
- Test: `internal/modules/cart/infrastructure/cart_repository_test.go` (integration)

**Interfaces:**
- Consumes (Task 1): sqlc `queries.*` for carts/cart_items; (Task 4) implements `application.CartRepository`.
- Produces: `func New(pool *pgxpool.Pool) *Repository`.

- [ ] **Step 1: Write the failing integration test**

Create `internal/modules/cart/infrastructure/cart_repository_test.go`:

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func newRepo(t *testing.T, ctx context.Context) (*infrastructure.Repository, *pgxIDs) {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ids := seedCatalog(t, ctx, pool) // inserts a user, product, variant; returns their IDs
	return infrastructure.New(pool), ids
}

func TestCartRepository_AnonAddAndFind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newRepo(t, ctx)

	anon := "anon-" + uuid.NewString()
	owner := domain.Owner{AnonID: &anon}

	price, err := repo.VariantUnitPrice(ctx, ids.variantID)
	require.NoError(t, err)
	assert.Equal(t, int64(9900), price)

	cart, err := repo.EnsureActive(ctx, owner)
	require.NoError(t, err)
	require.NoError(t, repo.UpsertItem(ctx, cart.ID, ids.variantID, 2, price))

	got, err := repo.FindActive(ctx, owner)
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	assert.Equal(t, 2, got.Items[0].Quantity)

	// upsert same variant sums quantity, clamped at 99
	require.NoError(t, repo.UpsertItem(ctx, cart.ID, ids.variantID, 100, price))
	got, err = repo.FindActive(ctx, owner)
	require.NoError(t, err)
	assert.Equal(t, 99, got.Items[0].Quantity)
}

func TestCartRepository_MergeAnonIntoUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newRepo(t, ctx)

	anon := "anon-" + uuid.NewString()
	anonOwner := domain.Owner{AnonID: &anon}
	price, err := repo.VariantUnitPrice(ctx, ids.variantID)
	require.NoError(t, err)
	anonCart, err := repo.EnsureActive(ctx, anonOwner)
	require.NoError(t, err)
	require.NoError(t, repo.UpsertItem(ctx, anonCart.ID, ids.variantID, 3, price))

	require.NoError(t, repo.Merge(ctx, anon, ids.userID))

	userOwner := domain.Owner{UserID: &ids.userID}
	userCart, err := repo.FindActive(ctx, userOwner)
	require.NoError(t, err)
	require.Len(t, userCart.Items, 1)
	assert.Equal(t, 3, userCart.Items[0].Quantity)

	// anon cart no longer active
	_, err = repo.FindActive(ctx, anonOwner)
	require.ErrorIs(t, err, domain.ErrCartNotFound)
}

func TestCartRepository_FindActive_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, _ := newRepo(t, ctx)
	anon := "missing"
	_, err := repo.FindActive(ctx, domain.Owner{AnonID: &anon})
	require.ErrorIs(t, err, domain.ErrCartNotFound)
}
```

Create the seed helper in the same package (`internal/modules/cart/infrastructure/helpers_test.go`):

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type pgxIDs struct {
	userID    uuid.UUID
	productID uuid.UUID
	variantID uuid.UUID
}

// seedCatalog inserts the minimal rows cart tests depend on: a user, a product,
// and a variant with an explicit price_cents of 9900.
func seedCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *pgxIDs {
	t.Helper()
	ids := &pgxIDs{userID: uuid.New(), productID: uuid.New(), variantID: uuid.New()}

	_, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test')`,
		ids.userID, "u-"+ids.userID.String()+"@test.local")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_products
		(id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		VALUES ($1, $2, 'P', 'D', 'B', $3, 5000, 'BRL', 'published')`,
		ids.productID, "slug-"+ids.productID.String(), uuid.New())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		VALUES ($1, $2, $3, 'M', 'Red', 9900)`,
		ids.variantID, ids.productID, "sku-"+ids.variantID.String())
	require.NoError(t, err)

	return ids
}
```

> The product insert sets `category_id` to a random UUID. The Phase 1 `catalog_products.category_id` has no FK (or is nullable) per the existing schema; if the integration run reports a FK violation here, first insert a `catalog_categories` row and use its ID. Verify against `db/migrations/20260508120000_catalog.sql` before running.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration -run TestCartRepository ./internal/modules/cart/infrastructure/...`
Expected: FAIL — `infrastructure.New` undefined.

- [ ] **Step 3: Write the mappers**

Create `internal/modules/cart/infrastructure/mappers.go`:

```go
// Package infrastructure adapts sqlc queries to the cart domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapCart(row queries.Cart, items []queries.CartItem) domain.Cart {
	c := domain.Cart{
		ID:            row.ID,
		UserID:        row.UserID,
		AnonSessionID: row.AnonSessionID,
		Status:        domain.Status(row.Status),
		Items:         make([]domain.CartItem, 0, len(items)),
	}
	for _, it := range items {
		c.Items = append(c.Items, domain.CartItem{
			ID:             it.ID,
			VariantID:      it.VariantID,
			Quantity:       int(it.Quantity),
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return c
}
```

- [ ] **Step 4: Write the repository**

Create `internal/modules/cart/infrastructure/cart_repository.go`:

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed cart store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.CartRepository = (*Repository)(nil)

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// FindActive returns the owner's active cart with items loaded.
func (r *Repository) FindActive(ctx context.Context, owner domain.Owner) (domain.Cart, error) {
	row, err := r.activeCartRow(ctx, r.q, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	items, err := r.q.ListCartItems(ctx, row.ID)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("cart repo: list items: %w", err)
	}
	return mapCart(row, items), nil
}

// EnsureActive returns the owner's active cart, creating an empty one if absent.
func (r *Repository) EnsureActive(ctx context.Context, owner domain.Owner) (domain.Cart, error) {
	cart, err := r.FindActive(ctx, owner)
	if err == nil {
		return cart, nil
	}
	if !errors.Is(err, domain.ErrCartNotFound) {
		return domain.Cart{}, err
	}
	var row queries.Cart
	switch {
	case owner.UserID != nil:
		row, err = r.q.CreateUserCart(ctx, owner.UserID)
	case owner.AnonID != nil:
		row, err = r.q.CreateAnonCart(ctx, owner.AnonID)
	default:
		return domain.Cart{}, fmt.Errorf("cart repo: invalid owner")
	}
	if err != nil {
		return domain.Cart{}, fmt.Errorf("cart repo: create cart: %w", err)
	}
	return mapCart(row, nil), nil
}

// VariantUnitPrice returns the effective unit price for a variant.
func (r *Repository) VariantUnitPrice(ctx context.Context, variantID uuid.UUID) (int64, error) {
	price, err := r.q.GetVariantUnitPrice(ctx, variantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrVariantNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cart repo: variant price: %w", err)
	}
	return price, nil
}

// UpsertItem inserts or sums a cart line (clamped to 99 by the query).
func (r *Repository) UpsertItem(ctx context.Context, cartID, variantID uuid.UUID, qty int, unitPrice int64) error {
	_, err := r.q.UpsertCartItem(ctx, queries.UpsertCartItemParams{
		CartID:         cartID,
		VariantID:      variantID,
		Quantity:       int32(qty),
		UnitPriceCents: unitPrice,
	})
	if err != nil {
		return fmt.Errorf("cart repo: upsert item: %w", err)
	}
	return nil
}

// UpdateItemQuantity sets a line quantity scoped to the cart.
func (r *Repository) UpdateItemQuantity(ctx context.Context, cartID, itemID uuid.UUID, qty int) error {
	_, err := r.q.UpdateCartItemQuantity(ctx, queries.UpdateCartItemQuantityParams{
		ID:       itemID,
		CartID:   cartID,
		Quantity: int32(qty),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrItemNotFound
	}
	if err != nil {
		return fmt.Errorf("cart repo: update item: %w", err)
	}
	return nil
}

// DeleteItem removes a line scoped to the cart.
func (r *Repository) DeleteItem(ctx context.Context, cartID, itemID uuid.UUID) error {
	n, err := r.q.DeleteCartItem(ctx, queries.DeleteCartItemParams{ID: itemID, CartID: cartID})
	if err != nil {
		return fmt.Errorf("cart repo: delete item: %w", err)
	}
	if n == 0 {
		return domain.ErrItemNotFound
	}
	return nil
}

// ClearItems removes all lines from a cart.
func (r *Repository) ClearItems(ctx context.Context, cartID uuid.UUID) error {
	if err := r.q.DeleteCartItemsByCart(ctx, cartID); err != nil {
		return fmt.Errorf("cart repo: clear items: %w", err)
	}
	return nil
}

// Merge folds the anon cart into the user's active cart in one transaction.
func (r *Repository) Merge(ctx context.Context, anonID string, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cart repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	anonCart, err := q.GetActiveCartByAnon(ctx, &anonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // nothing to merge
	}
	if err != nil {
		return fmt.Errorf("cart repo: merge get anon: %w", err)
	}

	userCart, err := q.GetActiveCartByUser(ctx, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		userCart, err = q.CreateUserCart(ctx, &userID)
	}
	if err != nil {
		return fmt.Errorf("cart repo: merge ensure user cart: %w", err)
	}

	items, err := q.ListCartItems(ctx, anonCart.ID)
	if err != nil {
		return fmt.Errorf("cart repo: merge list items: %w", err)
	}
	for _, it := range items {
		if _, err := q.UpsertCartItem(ctx, queries.UpsertCartItemParams{
			CartID:         userCart.ID,
			VariantID:      it.VariantID,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		}); err != nil {
			return fmt.Errorf("cart repo: merge upsert: %w", err)
		}
	}

	if err := q.SetCartStatus(ctx, queries.SetCartStatusParams{ID: anonCart.ID, Status: string(domain.StatusMerged)}); err != nil {
		return fmt.Errorf("cart repo: merge mark status: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repository) activeCartRow(ctx context.Context, q *queries.Queries, owner domain.Owner) (queries.Cart, error) {
	var row queries.Cart
	var err error
	switch {
	case owner.UserID != nil:
		row, err = q.GetActiveCartByUser(ctx, owner.UserID)
	case owner.AnonID != nil:
		row, err = q.GetActiveCartByAnon(ctx, owner.AnonID)
	default:
		return queries.Cart{}, fmt.Errorf("cart repo: invalid owner")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return queries.Cart{}, domain.ErrCartNotFound
	}
	if err != nil {
		return queries.Cart{}, fmt.Errorf("cart repo: get active cart: %w", err)
	}
	return row, nil
}
```

> sqlc emits pointer params for nullable columns (`emit_pointers_for_null_types: true`), so `GetActiveCartByUser` takes `*uuid.UUID` and `GetActiveCartByAnon`/`CreateAnonCart` take `*string`. The owner already holds pointers — pass them directly. Confirm the generated param names (`SetCartStatusParams`, `UpsertCartItemParams`, etc.) after `make sqlc-gen`; adjust if sqlc named them differently.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags=integration -run TestCartRepository ./internal/modules/cart/infrastructure/...`
Expected: PASS (3 tests). Requires Docker (colima) running — see `reference_local_dev_colima` memory: `colima start`, `export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock`.

- [ ] **Step 6: Commit**

```bash
git add internal/modules/cart/infrastructure/
git commit -m "feat(cart): add Postgres cart repository with merge transaction"
```

---
## Task 6: `internal/modules/cart/jobs` — cleanup_abandoned_carts + config + worker wiring

**Files:**
- Create: `internal/modules/cart/jobs/cleanup_abandoned_carts.go`
- Test: `internal/modules/cart/jobs/cleanup_abandoned_carts_test.go` (integration)
- Modify: `internal/config/config.go` (+ `Cart` section), `cmd/worker/main.go` (register worker + periodic job)

**Interfaces:**
- Consumes (Task 1): `queries.DeleteAbandonedCarts`.
- Produces: `type CleanupAbandonedCartsArgs struct{}` (`Kind()` = `"cart.cleanup_abandoned_carts"`), `type CleanupAbandonedCartsWorker struct{...}`, `func NewCleanupAbandonedCartsWorker(pool, abandonedAfter time.Duration) *CleanupAbandonedCartsWorker`, `func RunCleanupAbandonedCartsOnce(ctx, pool, cutoff time.Time) (int64, error)`.

- [ ] **Step 1: Add the `Cart` config section**

In `internal/config/config.go`, add the struct (after the `ViaCEP` struct) and embed it in `Config`:

```go
// Cart configures cart background maintenance.
type Cart struct {
	AbandonedAfter  time.Duration `env:"CART_ABANDONED_AFTER" envDefault:"168h"` // 7d
	CleanupInterval time.Duration `env:"CART_CLEANUP_INTERVAL" envDefault:"6h"`
}
```

Add the field to `Config`:

```go
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
	Cart          Cart
}
```

(`Load()` is zero-touch — `env.Parse` recurses into the new section automatically.)

- [ ] **Step 2: Write the failing integration test**

Create `internal/modules/cart/jobs/cleanup_abandoned_carts_test.go`:

```go
//go:build integration

package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/jobs"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func TestRunCleanupAbandonedCartsOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Old anon cart (updated 10 days ago) → should be abandoned.
	oldID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO carts (id, anon_session_id, status, updated_at)
		VALUES ($1, $2, 'active', now() - interval '10 days')`, oldID, "old-anon")
	require.NoError(t, err)

	// Fresh anon cart → should stay active.
	freshID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO carts (id, anon_session_id, status) VALUES ($1, $2, 'active')`, freshID, "fresh-anon")
	require.NoError(t, err)

	n, err := jobs.RunCleanupAbandonedCartsOnce(ctx, pool, time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM carts WHERE id = $1`, oldID).Scan(&status))
	assert.Equal(t, "abandoned", status)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM carts WHERE id = $1`, freshID).Scan(&status))
	assert.Equal(t, "active", status)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -tags=integration -run TestRunCleanupAbandonedCartsOnce ./internal/modules/cart/jobs/...`
Expected: FAIL — `jobs` package undefined.

- [ ] **Step 4: Write the job**

Create `internal/modules/cart/jobs/cleanup_abandoned_carts.go`:

```go
// Package jobs holds river background workers for the cart module.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// CleanupAbandonedCartsArgs is the river job payload.
type CleanupAbandonedCartsArgs struct{}

// Kind implements river.JobArgs.
func (CleanupAbandonedCartsArgs) Kind() string { return "cart.cleanup_abandoned_carts" }

// CleanupAbandonedCartsWorker marks stale anonymous carts as abandoned.
type CleanupAbandonedCartsWorker struct {
	river.WorkerDefaults[CleanupAbandonedCartsArgs]
	pool           *pgxpool.Pool
	abandonedAfter time.Duration
}

// NewCleanupAbandonedCartsWorker builds the worker.
func NewCleanupAbandonedCartsWorker(pool *pgxpool.Pool, abandonedAfter time.Duration) *CleanupAbandonedCartsWorker {
	return &CleanupAbandonedCartsWorker{pool: pool, abandonedAfter: abandonedAfter}
}

// Work runs once per scheduled tick.
func (w *CleanupAbandonedCartsWorker) Work(ctx context.Context, _ *river.Job[CleanupAbandonedCartsArgs]) error {
	_, err := RunCleanupAbandonedCartsOnce(ctx, w.pool, time.Now().Add(-w.abandonedAfter))
	return err
}

// RunCleanupAbandonedCartsOnce marks active anon carts older than cutoff as abandoned.
// Returns the number of rows updated. Used by the worker and by tests.
func RunCleanupAbandonedCartsOnce(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time) (int64, error) {
	q := queries.New(pool)
	n, err := q.DeleteAbandonedCarts(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cart jobs: abandon carts: %w", err)
	}
	return n, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -tags=integration -run TestRunCleanupAbandonedCartsOnce ./internal/modules/cart/jobs/...`
Expected: PASS.

- [ ] **Step 6: Wire the worker into `cmd/worker/main.go`**

Add the import:

```go
	cartjobs "github.com/danilloboing/marketplace-golang/internal/modules/cart/jobs"
```

Register the worker alongside the existing catalog cleanup (after `river.AddWorkerSafely(workers, cleanup)`):

```go
	cartCleanup := cartjobs.NewCleanupAbandonedCartsWorker(pool, cfg.Cart.AbandonedAfter)
	if err := river.AddWorkerSafely(workers, cartCleanup); err != nil {
		return fmt.Errorf("register cart cleanup worker: %w", err)
	}
```

Add a periodic entry to the `periodic` slice:

```go
		river.NewPeriodicJob(
			river.PeriodicInterval(cfg.Cart.CleanupInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return cartjobs.CleanupAbandonedCartsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
```

- [ ] **Step 7: Verify build**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/modules/cart/jobs/ cmd/worker/main.go
git commit -m "feat(cart): add cleanup_abandoned_carts job, config, and worker wiring"
```

---

## Task 7: `internal/modules/cart/transport` — handlers, identity middleware, module

**Files:**
- Create: `internal/modules/cart/transport/identity_middleware.go`, `cart_handlers.go`, `responses.go`, `error_mapping.go`, `internal/modules/cart/module.go`
- Test: `internal/modules/cart/transport/cart_handlers_test.go`

**Interfaces:**
- Consumes: `application.CartService` (via local `CartUseCase` interface), `sessionauth.Manager`, `domain.Owner`.
- Produces:
  - `func ContextWithOwner(ctx, domain.Owner) context.Context`, `func OwnerFromContext(ctx) (domain.Owner, bool)`
  - `func ResolveCartIdentity(sessions sessionauth.Manager, sessionCookieName, anonCookieName string) func(http.Handler) http.Handler`
  - `type CartHandlers struct{...}`, `func NewCartHandlers(svc CartUseCase, anonCookieName string) *CartHandlers`, `RegisterCartRoutes(chi.Router)`
  - `func cart.New(Deps) *Module` with `Mount(chi.Router)` and `Merger() func(ctx, anonID string, userID uuid.UUID) error`

- [ ] **Step 1: Write the identity middleware**

Create `internal/modules/cart/transport/identity_middleware.go`:

```go
// Package transport adapts cart use cases to HTTP.
package transport

import (
	"context"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

type ownerCtxKey struct{}

// ContextWithOwner injects a resolved cart owner.
func ContextWithOwner(ctx context.Context, owner domain.Owner) context.Context {
	return context.WithValue(ctx, ownerCtxKey{}, owner)
}

// OwnerFromContext returns the resolved owner, or false if none was resolved.
func OwnerFromContext(ctx context.Context) (domain.Owner, bool) {
	o, ok := ctx.Value(ownerCtxKey{}).(domain.Owner)
	return o, ok
}

// ResolveCartIdentity resolves the cart owner without requiring authentication.
// Preference: a valid user session, else an existing cart_anon cookie. When
// neither is present no owner is injected (handlers decide whether to mint one).
func ResolveCartIdentity(sessions sessionauth.Manager, sessionCookieName, anonCookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
				if sess, err := sessions.Get(r.Context(), c.Value); err == nil {
					uid := sess.UserID
					ctx := ContextWithOwner(r.Context(), domain.Owner{UserID: &uid})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if c, err := r.Cookie(anonCookieName); err == nil && c.Value != "" {
				id := c.Value
				ctx := ContextWithOwner(r.Context(), domain.Owner{AnonID: &id})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2: Write responses + error mapping**

Create `internal/modules/cart/transport/responses.go`:

```go
package transport

import "github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"

// CartResponse is the JSON shape returned by cart endpoints.
type CartResponse struct {
	Items         []CartItemResponse `json:"items"`
	SubtotalCents int64              `json:"subtotal_cents"`
}

// CartItemResponse is a single cart line.
type CartItemResponse struct {
	ID             string `json:"id"`
	VariantID      string `json:"variant_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

func toCartResponse(c domain.Cart) CartResponse {
	items := make([]CartItemResponse, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, CartItemResponse{
			ID:             it.ID.String(),
			VariantID:      it.VariantID.String(),
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return CartResponse{Items: items, SubtotalCents: c.SubtotalCents()}
}
```

Create `internal/modules/cart/transport/error_mapping.go`:

```go
package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// mapErrorToHTTP returns (status, code, message) for a cart error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidQuantity):
		return http.StatusUnprocessableEntity, "invalid_quantity", "quantity must be between 1 and 99"
	case errors.Is(err, domain.ErrVariantNotFound):
		return http.StatusNotFound, "variant_not_found", "variant not found"
	case errors.Is(err, domain.ErrItemNotFound):
		return http.StatusNotFound, "item_not_found", "cart item not found"
	case errors.Is(err, domain.ErrCartNotFound):
		return http.StatusNotFound, "cart_not_found", "cart not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
```

- [ ] **Step 3: Write the failing handler test**

Create `internal/modules/cart/transport/cart_handlers_test.go`:

```go
package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/transport"
)

type fakeUseCase struct {
	cart    domain.Cart
	addErr  error
	lastQty int
}

func (f *fakeUseCase) Get(_ context.Context, _ domain.Owner) (domain.Cart, error) { return f.cart, nil }
func (f *fakeUseCase) AddItem(_ context.Context, _ domain.Owner, _ uuid.UUID, qty int) (domain.Cart, error) {
	f.lastQty = qty
	if f.addErr != nil {
		return domain.Cart{}, f.addErr
	}
	f.cart.Items = append(f.cart.Items, domain.CartItem{ID: uuid.New(), Quantity: qty, UnitPriceCents: 1000})
	return f.cart, nil
}
func (f *fakeUseCase) UpdateItem(_ context.Context, _ domain.Owner, _ uuid.UUID, _ int) (domain.Cart, error) {
	return f.cart, nil
}
func (f *fakeUseCase) RemoveItem(_ context.Context, _ domain.Owner, _ uuid.UUID) (domain.Cart, error) {
	return f.cart, nil
}
func (f *fakeUseCase) Clear(_ context.Context, _ domain.Owner) error { return nil }

func router(uc transport.CartUseCase) chi.Router {
	h := transport.NewCartHandlers(uc, "cart_anon")
	r := chi.NewRouter()
	h.RegisterCartRoutes(r)
	return r
}

func TestAddItem_NoIdentity_MintsAnonCookie(t *testing.T) {
	uc := &fakeUseCase{}
	srv := httptest.NewServer(router(uc))
	defer srv.Close()

	body := strings.NewReader(`{"variant_id":"` + uuid.NewString() + `","quantity":2}`)
	resp, err := http.Post(srv.URL+"/cart/items", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "cart_anon" && c.Value != "" {
			found = true
		}
	}
	assert.True(t, found, "expected cart_anon cookie to be set")
	assert.Equal(t, 2, uc.lastQty)
}

func TestAddItem_OverCap_422(t *testing.T) {
	uc := &fakeUseCase{addErr: domain.ErrInvalidQuantity}
	srv := httptest.NewServer(router(uc))
	defer srv.Close()

	body := strings.NewReader(`{"variant_id":"` + uuid.NewString() + `","quantity":200}`)
	resp, err := http.Post(srv.URL+"/cart/items", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestGetCart_NoIdentity_EmptyCart(t *testing.T) {
	srv := httptest.NewServer(router(&fakeUseCase{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/cart")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var out transport.CartResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Empty(t, out.Items)
	assert.Equal(t, int64(0), out.SubtotalCents)
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/modules/cart/transport/...`
Expected: FAIL — `transport.NewCartHandlers` / `CartUseCase` undefined.

- [ ] **Step 5: Write the handlers**

Create `internal/modules/cart/transport/cart_handlers.go`:

```go
package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// CartUseCase is the slice of CartService consumed by handlers.
type CartUseCase interface {
	Get(ctx context.Context, owner domain.Owner) (domain.Cart, error)
	AddItem(ctx context.Context, owner domain.Owner, variantID uuid.UUID, qty int) (domain.Cart, error)
	UpdateItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID, qty int) (domain.Cart, error)
	RemoveItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID) (domain.Cart, error)
	Clear(ctx context.Context, owner domain.Owner) error
}

// CartHandlers exposes cart endpoints to anon + user visitors.
type CartHandlers struct {
	svc            CartUseCase
	anonCookieName string
}

// NewCartHandlers builds CartHandlers.
func NewCartHandlers(svc CartUseCase, anonCookieName string) *CartHandlers {
	return &CartHandlers{svc: svc, anonCookieName: anonCookieName}
}

// RegisterCartRoutes mounts cart routes (caller wraps with ResolveCartIdentity).
func (h *CartHandlers) RegisterCartRoutes(r chi.Router) {
	r.Get("/cart", h.get)
	r.Post("/cart/items", h.addItem)
	r.Patch("/cart/items/{id}", h.updateItem)
	r.Delete("/cart/items/{id}", h.removeItem)
	r.Delete("/cart", h.clear)
}

func (h *CartHandlers) get(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		responsex.JSON(w, http.StatusOK, CartResponse{Items: []CartItemResponse{}})
		return
	}
	cart, err := h.svc.Get(r.Context(), owner)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

type addItemInput struct {
	VariantID string `json:"variant_id"`
	Quantity  int    `json:"quantity"`
}

func (h *CartHandlers) addItem(w http.ResponseWriter, r *http.Request) {
	var in addItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	variantID, err := uuid.Parse(in.VariantID)
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid variant id")
		return
	}
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		anon, err := newAnonID()
		if err != nil {
			responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "cart id gen failed", err)
			return
		}
		h.setAnonCookie(w, anon)
		owner = domain.Owner{AnonID: &anon}
	}
	cart, err := h.svc.AddItem(r.Context(), owner, variantID, in.Quantity)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

type updateItemInput struct {
	Quantity int `json:"quantity"`
}

func (h *CartHandlers) updateItem(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusNotFound, "cart_not_found", "cart not found")
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid item id")
		return
	}
	var in updateItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	cart, err := h.svc.UpdateItem(r.Context(), owner, itemID, in.Quantity)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *CartHandlers) removeItem(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusNotFound, "cart_not_found", "cart not found")
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid item id")
		return
	}
	cart, err := h.svc.RemoveItem(r.Context(), owner, itemID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *CartHandlers) clear(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.svc.Clear(r.Context(), owner); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CartHandlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := mapErrorToHTTP(err)
	responsex.ErrorWithCause(w, r, status, code, msg, err)
}

func (h *CartHandlers) setAnonCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.anonCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

func newAnonID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
```

- [ ] **Step 6: Write the module**

Create `internal/modules/cart/module.go`:

```go
// Package cart wires the cart bounded context.
package cart

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/transport"
)

// Module wires the cart bounded context onto a chi router.
type Module struct {
	handlers       *transport.CartHandlers
	svc            *application.CartService
	sessions       sessionauth.Manager
	sessionCookie  string
	anonCookieName string
}

// Deps groups raw dependencies the cart module needs.
type Deps struct {
	Pool           *pgxpool.Pool
	Sessions       sessionauth.Manager
	SessionCookie  string
	AnonCookieName string
}

// New builds the cart Module.
func New(d Deps) *Module {
	repo := infrastructure.New(d.Pool)
	svc := application.NewCartService(repo)
	return &Module{
		handlers:       transport.NewCartHandlers(svc, d.AnonCookieName),
		svc:            svc,
		sessions:       d.Sessions,
		sessionCookie:  d.SessionCookie,
		anonCookieName: d.AnonCookieName,
	}
}

// Mount registers public cart routes wrapped with cart-identity resolution.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(public chi.Router) {
		public.Use(transport.ResolveCartIdentity(m.sessions, m.sessionCookie, m.anonCookieName))
		m.handlers.RegisterCartRoutes(public)
	})
}

// Merger returns the cart-merge callback for the identity Login handler.
// Decoupled signature (no cart types) so identity needs no cart import.
func (m *Module) Merger() func(ctx context.Context, anonID string, userID uuid.UUID) error {
	return m.svc.Merge
}

// AnonCookieName exposes the anon cookie name so identity can clear it on login.
func (m *Module) AnonCookieName() string { return m.anonCookieName }
```

- [ ] **Step 7: Run tests + build**

Run: `go test ./internal/modules/cart/... && go build ./...`
Expected: PASS + exit 0.

- [ ] **Step 8: Commit**

```bash
git add internal/modules/cart/transport/ internal/modules/cart/module.go
git commit -m "feat(cart): add HTTP handlers, identity middleware, and module wiring"
```

---
## Task 8: `internal/modules/address/domain` — Address + validation + errors

**Files:**
- Create: `internal/modules/address/domain/address.go`, `internal/modules/address/domain/errors.go`
- Test: `internal/modules/address/domain/address_test.go`

**Interfaces:**
- Produces:
  - `type Address struct { ID, UserID uuid.UUID; RecipientName, PostalCode, Street, Number string; Complement *string; Neighborhood, City, State string; IsDefault bool; CreatedAt, UpdatedAt time.Time }`
  - `func Validate(a Address) error`
  - sentinels: `ErrAddressNotFound`, `ErrInvalidAddress`, `ErrInvalidCEP`, `ErrCEPNotFound`

- [ ] **Step 1: Write the failing test**

Create `internal/modules/address/domain/address_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

func valid() domain.Address {
	return domain.Address{
		ID: uuid.New(), UserID: uuid.New(),
		RecipientName: "Ana", PostalCode: "01001000", Street: "Praça da Sé",
		Number: "100", Neighborhood: "Sé", City: "São Paulo", State: "SP",
	}
}

func TestValidate_OK(t *testing.T) {
	require.NoError(t, domain.Validate(valid()))
}

func TestValidate_BadPostalCode(t *testing.T) {
	a := valid()
	a.PostalCode = "1234"
	require.ErrorIs(t, domain.Validate(a), domain.ErrInvalidAddress)
}

func TestValidate_BadState(t *testing.T) {
	a := valid()
	a.State = "SAO"
	require.ErrorIs(t, domain.Validate(a), domain.ErrInvalidAddress)
}

func TestValidate_MissingRequired(t *testing.T) {
	a := valid()
	a.City = "  "
	require.ErrorIs(t, domain.Validate(a), domain.ErrInvalidAddress)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modules/address/domain/...`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write errors**

Create `internal/modules/address/domain/errors.go`:

```go
package domain

import "errors"

// Sentinel errors for the address bounded context.
var (
	ErrAddressNotFound = errors.New("address: not found")
	ErrInvalidAddress  = errors.New("address: invalid address")
	ErrInvalidCEP      = errors.New("address: invalid cep")
	ErrCEPNotFound     = errors.New("address: cep not found")
)
```

- [ ] **Step 4: Write the domain type + validation**

Create `internal/modules/address/domain/address.go`:

```go
// Package domain holds address value types and invariants.
package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var postalPattern = regexp.MustCompile(`^[0-9]{8}$`)

// Address is a user's shipping address.
type Address struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	RecipientName string
	PostalCode    string
	Street        string
	Number        string
	Complement    *string
	Neighborhood  string
	City          string
	State         string
	IsDefault     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Validate enforces required fields, 8-digit postal code, and 2-letter state.
func Validate(a Address) error {
	if blank(a.RecipientName) || blank(a.Street) || blank(a.Number) ||
		blank(a.Neighborhood) || blank(a.City) {
		return ErrInvalidAddress
	}
	if !postalPattern.MatchString(a.PostalCode) {
		return ErrInvalidAddress
	}
	if len(a.State) != 2 {
		return ErrInvalidAddress
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/modules/address/domain/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/modules/address/domain/
git commit -m "feat(address): add domain type, validation, and sentinel errors"
```

---

## Task 9: `internal/modules/address/application` — AddressService + ports

**Files:**
- Create: `internal/modules/address/application/ports.go`, `internal/modules/address/application/address_service.go`
- Test: `internal/modules/address/application/address_service_test.go`

**Interfaces:**
- Produces:
  - `type AddressRepository interface { Create(ctx, domain.Address)(domain.Address,error); GetByID(ctx, id, userID uuid.UUID)(domain.Address,error); List(ctx, userID uuid.UUID)([]domain.Address,error); Update(ctx, domain.Address)(domain.Address,error); Delete(ctx, id, userID uuid.UUID) error; SetDefault(ctx, id, userID uuid.UUID)(domain.Address,error) }`
  - `type CreateInput struct{...}`, `type UpdateInput struct{...}` (pointer fields for partial)
  - `type AddressService struct{...}`, `NewAddressService(AddressRepository)`, methods `List/Create/Update/Delete/SetDefault`

- [ ] **Step 1: Write the failing test**

Create `internal/modules/address/application/address_service_test.go`:

```go
package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

type fakeRepo struct {
	store map[uuid.UUID]domain.Address
}

func newFake() *fakeRepo { return &fakeRepo{store: map[uuid.UUID]domain.Address{}} }

func (f *fakeRepo) Create(_ context.Context, a domain.Address) (domain.Address, error) {
	f.store[a.ID] = a
	return a, nil
}
func (f *fakeRepo) GetByID(_ context.Context, id, userID uuid.UUID) (domain.Address, error) {
	a, ok := f.store[id]
	if !ok || a.UserID != userID {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	return a, nil
}
func (f *fakeRepo) List(_ context.Context, userID uuid.UUID) ([]domain.Address, error) {
	var out []domain.Address
	for _, a := range f.store {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) Update(_ context.Context, a domain.Address) (domain.Address, error) {
	f.store[a.ID] = a
	return a, nil
}
func (f *fakeRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	a, ok := f.store[id]
	if !ok || a.UserID != userID {
		return domain.ErrAddressNotFound
	}
	delete(f.store, id)
	return nil
}
func (f *fakeRepo) SetDefault(_ context.Context, id, userID uuid.UUID) (domain.Address, error) {
	return f.GetByID(context.Background(), id, userID)
}

func TestAddressService_Create_Valid(t *testing.T) {
	svc := application.NewAddressService(newFake())
	user := uuid.New()
	a, err := svc.Create(context.Background(), application.CreateInput{
		UserID: user, RecipientName: "Ana", PostalCode: "01001000",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP",
	})
	require.NoError(t, err)
	assert.Equal(t, user, a.UserID)
	assert.NotEqual(t, uuid.Nil, a.ID)
}

func TestAddressService_Create_Invalid(t *testing.T) {
	svc := application.NewAddressService(newFake())
	_, err := svc.Create(context.Background(), application.CreateInput{
		UserID: uuid.New(), RecipientName: "Ana", PostalCode: "bad",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP",
	})
	require.ErrorIs(t, err, domain.ErrInvalidAddress)
}

func TestAddressService_Update_PartialAndCrossUser(t *testing.T) {
	repo := newFake()
	svc := application.NewAddressService(repo)
	owner := uuid.New()
	created, err := svc.Create(context.Background(), application.CreateInput{
		UserID: owner, RecipientName: "Ana", PostalCode: "01001000",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP",
	})
	require.NoError(t, err)

	newName := "Ana Maria"
	updated, err := svc.Update(context.Background(), application.UpdateInput{
		UserID: owner, ID: created.ID, RecipientName: &newName,
	})
	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", updated.RecipientName)
	assert.Equal(t, "Sé", updated.Street) // unchanged

	_, err = svc.Update(context.Background(), application.UpdateInput{
		UserID: uuid.New(), ID: created.ID, RecipientName: &newName,
	})
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/modules/address/application/...`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write ports**

Create `internal/modules/address/application/ports.go`:

```go
// Package application contains address use cases and ports.
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// AddressRepository is the persistence contract for addresses.
// Create and SetDefault maintain the single-default invariant atomically.
type AddressRepository interface {
	Create(ctx context.Context, a domain.Address) (domain.Address, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (domain.Address, error)
	List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error)
	Update(ctx context.Context, a domain.Address) (domain.Address, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error)
}
```

- [ ] **Step 4: Write the service**

Create `internal/modules/address/application/address_service.go`:

```go
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// AddressService orchestrates address use cases.
type AddressService struct {
	repo AddressRepository
}

// NewAddressService builds an AddressService.
func NewAddressService(repo AddressRepository) *AddressService {
	return &AddressService{repo: repo}
}

// CreateInput is the full create payload.
type CreateInput struct {
	UserID        uuid.UUID
	RecipientName string
	PostalCode    string
	Street        string
	Number        string
	Complement    *string
	Neighborhood  string
	City          string
	State         string
	IsDefault     bool
}

// UpdateInput is a partial update; nil fields are left unchanged.
type UpdateInput struct {
	UserID        uuid.UUID
	ID            uuid.UUID
	RecipientName *string
	PostalCode    *string
	Street        *string
	Number        *string
	Complement    *string
	Neighborhood  *string
	City          *string
	State         *string
}

// List returns the user's addresses (default first).
func (s *AddressService) List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error) {
	return s.repo.List(ctx, userID)
}

// Create validates and persists a new address.
func (s *AddressService) Create(ctx context.Context, in CreateInput) (domain.Address, error) {
	a := domain.Address{
		ID:            uuid.New(),
		UserID:        in.UserID,
		RecipientName: in.RecipientName,
		PostalCode:    in.PostalCode,
		Street:        in.Street,
		Number:        in.Number,
		Complement:    in.Complement,
		Neighborhood:  in.Neighborhood,
		City:          in.City,
		State:         in.State,
		IsDefault:     in.IsDefault,
	}
	if err := domain.Validate(a); err != nil {
		return domain.Address{}, err
	}
	return s.repo.Create(ctx, a)
}

// Update fetches, applies provided fields, validates, and persists.
func (s *AddressService) Update(ctx context.Context, in UpdateInput) (domain.Address, error) {
	a, err := s.repo.GetByID(ctx, in.ID, in.UserID)
	if err != nil {
		return domain.Address{}, err
	}
	applyString(&a.RecipientName, in.RecipientName)
	applyString(&a.PostalCode, in.PostalCode)
	applyString(&a.Street, in.Street)
	applyString(&a.Number, in.Number)
	applyString(&a.Neighborhood, in.Neighborhood)
	applyString(&a.City, in.City)
	applyString(&a.State, in.State)
	if in.Complement != nil {
		a.Complement = in.Complement
	}
	if err := domain.Validate(a); err != nil {
		return domain.Address{}, err
	}
	return s.repo.Update(ctx, a)
}

// Delete removes an address scoped to the user.
func (s *AddressService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}

// SetDefault makes one address the user's default (atomic in the repo).
func (s *AddressService) SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error) {
	return s.repo.SetDefault(ctx, id, userID)
}

func applyString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/modules/address/application/...`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/modules/address/application/
git commit -m "feat(address): add AddressService with partial update and ports"
```

---

## Task 10: `internal/modules/address/infrastructure` — Postgres AddressRepository

**Files:**
- Create: `internal/modules/address/infrastructure/address_repository.go`, `internal/modules/address/infrastructure/mappers.go`
- Test: `internal/modules/address/infrastructure/address_repository_test.go` (integration)

**Interfaces:**
- Consumes (Task 1): sqlc address queries. Implements `application.AddressRepository`.
- Produces: `func New(pool *pgxpool.Pool) *Repository`.

- [ ] **Step 1: Write the failing integration test**

Create `internal/modules/address/infrastructure/address_repository_test.go`:

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func newRepo(t *testing.T, ctx context.Context) (*infrastructure.Repository, uuid.UUID) {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	userID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'U')`, userID, "u-"+userID.String()+"@t.local")
	require.NoError(t, err)
	return infrastructure.New(pool), userID
}

func addr(userID uuid.UUID, def bool) domain.Address {
	return domain.Address{
		ID: uuid.New(), UserID: userID, RecipientName: "Ana", PostalCode: "01001000",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP", IsDefault: def,
	}
}

func TestAddressRepository_CRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, userID := newRepo(t, ctx)

	created, err := repo.Create(ctx, addr(userID, false))
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, created.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, "Ana", got.RecipientName)

	_, err = repo.GetByID(ctx, created.ID, uuid.New()) // cross-user
	require.ErrorIs(t, err, domain.ErrAddressNotFound)

	list, err := repo.List(ctx, userID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, repo.Delete(ctx, created.ID, userID))
	require.ErrorIs(t, repo.Delete(ctx, created.ID, userID), domain.ErrAddressNotFound)
}

func TestAddressRepository_DefaultUniqueness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, userID := newRepo(t, ctx)

	first, err := repo.Create(ctx, addr(userID, true))
	require.NoError(t, err)
	second, err := repo.Create(ctx, addr(userID, true))
	require.NoError(t, err)

	// Only the second remains default (partial unique index respected via tx).
	got2, err := repo.GetByID(ctx, second.ID, userID)
	require.NoError(t, err)
	assert.True(t, got2.IsDefault)
	got1, err := repo.GetByID(ctx, first.ID, userID)
	require.NoError(t, err)
	assert.False(t, got1.IsDefault)

	// SetDefault flips it back to the first.
	_, err = repo.SetDefault(ctx, first.ID, userID)
	require.NoError(t, err)
	got1, _ = repo.GetByID(ctx, first.ID, userID)
	got2, _ = repo.GetByID(ctx, second.ID, userID)
	assert.True(t, got1.IsDefault)
	assert.False(t, got2.IsDefault)

	_, err = repo.SetDefault(ctx, uuid.New(), userID)
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}

var _ application.AddressRepository = (*infrastructure.Repository)(nil)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration -run TestAddressRepository ./internal/modules/address/infrastructure/...`
Expected: FAIL — `infrastructure.New` undefined.

- [ ] **Step 3: Write the mappers**

Create `internal/modules/address/infrastructure/mappers.go`:

```go
// Package infrastructure adapts sqlc queries to the address domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapAddress(row queries.Address) domain.Address {
	return domain.Address{
		ID:            row.ID,
		UserID:        row.UserID,
		RecipientName: row.RecipientName,
		PostalCode:    row.PostalCode,
		Street:        row.Street,
		Number:        row.Number,
		Complement:    row.Complement,
		Neighborhood:  row.Neighborhood,
		City:          row.City,
		State:         row.State,
		IsDefault:     row.IsDefault,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
```

> `state` is `CHAR(2)` → sqlc may emit `string` (pgx returns it trimmed/padded). If the generated `queries.Address.State` is not `string`, adjust `mapAddress` accordingly after `make sqlc-gen`.

- [ ] **Step 4: Write the repository**

Create `internal/modules/address/infrastructure/address_repository.go`:

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed address store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// Create persists a new address; when IsDefault it clears the prior default in one tx.
func (r *Repository) Create(ctx context.Context, a domain.Address) (domain.Address, error) {
	if !a.IsDefault {
		row, err := r.q.CreateAddress(ctx, createParams(a))
		if err != nil {
			return domain.Address{}, fmt.Errorf("address repo: create: %w", err)
		}
		return mapAddress(row), nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if err := q.ClearDefaultAddress(ctx, a.UserID); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: clear default: %w", err)
	}
	row, err := q.CreateAddress(ctx, createParams(a))
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: create default: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: commit: %w", err)
	}
	return mapAddress(row), nil
}

// GetByID returns an address scoped to the user.
func (r *Repository) GetByID(ctx context.Context, id, userID uuid.UUID) (domain.Address, error) {
	row, err := r.q.GetAddressByID(ctx, queries.GetAddressByIDParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: get: %w", err)
	}
	return mapAddress(row), nil
}

// List returns the user's addresses.
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error) {
	rows, err := r.q.ListAddressesByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("address repo: list: %w", err)
	}
	out := make([]domain.Address, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapAddress(row))
	}
	return out, nil
}

// Update persists mutable fields scoped to the user.
func (r *Repository) Update(ctx context.Context, a domain.Address) (domain.Address, error) {
	row, err := r.q.UpdateAddress(ctx, queries.UpdateAddressParams{
		ID:            a.ID,
		UserID:        a.UserID,
		RecipientName: a.RecipientName,
		PostalCode:    a.PostalCode,
		Street:        a.Street,
		Number:        a.Number,
		Complement:    a.Complement,
		Neighborhood:  a.Neighborhood,
		City:          a.City,
		State:         a.State,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: update: %w", err)
	}
	return mapAddress(row), nil
}

// Delete removes an address scoped to the user.
func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	n, err := r.q.DeleteAddress(ctx, queries.DeleteAddressParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("address repo: delete: %w", err)
	}
	if n == 0 {
		return domain.ErrAddressNotFound
	}
	return nil
}

// SetDefault clears the current default and marks id default, in one tx.
func (r *Repository) SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if err := q.ClearDefaultAddress(ctx, userID); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: clear default: %w", err)
	}
	row, err := q.SetDefaultAddress(ctx, queries.SetDefaultAddressParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: set default: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: commit: %w", err)
	}
	return mapAddress(row), nil
}

func createParams(a domain.Address) queries.CreateAddressParams {
	return queries.CreateAddressParams{
		ID:            a.ID,
		UserID:        a.UserID,
		RecipientName: a.RecipientName,
		PostalCode:    a.PostalCode,
		Street:        a.Street,
		Number:        a.Number,
		Complement:    a.Complement,
		Neighborhood:  a.Neighborhood,
		City:          a.City,
		State:         a.State,
		IsDefault:     a.IsDefault,
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags=integration -run TestAddressRepository ./internal/modules/address/infrastructure/...`
Expected: PASS (2 tests). Requires Docker (colima).

- [ ] **Step 6: Commit**

```bash
git add internal/modules/address/infrastructure/
git commit -m "feat(address): add Postgres repository with atomic default handling"
```

---

## Task 11: `internal/modules/address/transport` — handlers, cep handler, module

**Files:**
- Create: `internal/modules/address/transport/address_handlers.go`, `cep_handler.go`, `responses.go`, `error_mapping.go`, `internal/modules/address/module.go`
- Test: `internal/modules/address/transport/address_handlers_test.go`

**Interfaces:**
- Consumes: `application.AddressService` (via local `AddressUseCase` interface), `viacep.Lookuper`, `sessionauth.Manager`, `csrf.Config`.
- Produces: `func address.New(Deps) *Module` with `Mount(chi.Router)`.

- [ ] **Step 1: Write responses + error mapping**

Create `internal/modules/address/transport/responses.go`:

```go
package transport

import "github.com/danilloboing/marketplace-golang/internal/modules/address/domain"

// AddressResponse is the JSON shape of an address.
type AddressResponse struct {
	ID            string  `json:"id"`
	RecipientName string  `json:"recipient_name"`
	PostalCode    string  `json:"postal_code"`
	Street        string  `json:"street"`
	Number        string  `json:"number"`
	Complement    *string `json:"complement,omitempty"`
	Neighborhood  string  `json:"neighborhood"`
	City          string  `json:"city"`
	State         string  `json:"state"`
	IsDefault     bool    `json:"is_default"`
}

// CEPResponse is the JSON shape of a ViaCEP lookup.
type CEPResponse struct {
	PostalCode   string `json:"postal_code"`
	Street       string `json:"street"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
}

func toAddressResponse(a domain.Address) AddressResponse {
	return AddressResponse{
		ID:            a.ID.String(),
		RecipientName: a.RecipientName,
		PostalCode:    a.PostalCode,
		Street:        a.Street,
		Number:        a.Number,
		Complement:    a.Complement,
		Neighborhood:  a.Neighborhood,
		City:          a.City,
		State:         a.State,
		IsDefault:     a.IsDefault,
	}
}
```

Create `internal/modules/address/transport/error_mapping.go`:

```go
package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidAddress):
		return http.StatusUnprocessableEntity, "invalid_address", "invalid address data"
	case errors.Is(err, domain.ErrAddressNotFound):
		return http.StatusNotFound, "not_found", "address not found"
	case errors.Is(err, domain.ErrInvalidCEP):
		return http.StatusBadRequest, "invalid_cep", "invalid cep"
	case errors.Is(err, domain.ErrCEPNotFound):
		return http.StatusNotFound, "cep_not_found", "cep not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
```

- [ ] **Step 2: Write the failing handler test**

Create `internal/modules/address/transport/address_handlers_test.go`:

```go
package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

func TestCEPHandler_Success(t *testing.T) {
	fake := viacep.NewFakeClient()
	fake.Responses["01001000"] = viacep.Address{PostalCode: "01001000", Street: "Sé", Neighborhood: "Sé", City: "São Paulo", State: "SP"}

	h := transport.NewCEPHandler(fake)
	r := chi.NewRouter()
	h.RegisterCEPRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/address/cep/01001000")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCEPHandler_NotFound(t *testing.T) {
	fake := viacep.NewFakeClient()
	h := transport.NewCEPHandler(fake)
	r := chi.NewRouter()
	h.RegisterCEPRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/address/cep/99999999")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

var _ transport.AddressUseCase = (*noopUseCase)(nil)

type noopUseCase struct{}

func (noopUseCase) List(context.Context, [16]byte) ([]domain.Address, error) { return nil, nil }
```

> The `noopUseCase` compile assertion intentionally references the interface to lock its shape; flesh out a full fake only if you add handler-level tests beyond the CEP path (the CRUD paths are covered by the E2E suite in Task 14). Remove the assertion if it drifts from the final `AddressUseCase` signature.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/modules/address/transport/...`
Expected: FAIL — undefined symbols.

- [ ] **Step 4: Write the CEP handler**

Create `internal/modules/address/transport/cep_handler.go`:

```go
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

// CEPHandler exposes the public postal-code lookup.
type CEPHandler struct {
	lookuper viacep.Lookuper
}

// NewCEPHandler builds a CEPHandler.
func NewCEPHandler(l viacep.Lookuper) *CEPHandler {
	return &CEPHandler{lookuper: l}
}

// RegisterCEPRoutes mounts the public CEP route.
func (h *CEPHandler) RegisterCEPRoutes(r chi.Router) {
	r.Get("/address/cep/{cep}", h.lookup)
}

func (h *CEPHandler) lookup(w http.ResponseWriter, r *http.Request) {
	cep := chi.URLParam(r, "cep")
	addr, err := h.lookuper.Lookup(r.Context(), cep)
	if err != nil {
		switch err {
		case viacep.ErrInvalidCEP:
			responsex.Error(w, r, http.StatusBadRequest, "invalid_cep", "invalid cep")
		case viacep.ErrCEPNotFound:
			responsex.Error(w, r, http.StatusNotFound, "cep_not_found", "cep not found")
		default:
			responsex.ErrorWithCause(w, r, http.StatusBadGateway, "cep_lookup_failed", "cep lookup failed", err)
		}
		return
	}
	responsex.JSON(w, http.StatusOK, CEPResponse{
		PostalCode:   addr.PostalCode,
		Street:       addr.Street,
		Neighborhood: addr.Neighborhood,
		City:         addr.City,
		State:        addr.State,
	})
}
```

- [ ] **Step 5: Write the address handlers**

Create `internal/modules/address/transport/address_handlers.go`:

```go
package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// AddressUseCase is the slice of AddressService consumed by handlers.
type AddressUseCase interface {
	List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error)
}

// AddressWriter adds the mutating use cases.
type AddressWriter interface {
	AddressUseCase
	Create(ctx context.Context, in application.CreateInput) (domain.Address, error)
	Update(ctx context.Context, in application.UpdateInput) (domain.Address, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error)
}

// AddressHandlers exposes the authenticated /me/addresses surface.
type AddressHandlers struct {
	svc AddressWriter
}

// NewAddressHandlers builds AddressHandlers.
func NewAddressHandlers(svc AddressWriter) *AddressHandlers {
	return &AddressHandlers{svc: svc}
}

// RegisterAddressRoutes mounts routes (caller wraps with sessionauth + csrf).
func (h *AddressHandlers) RegisterAddressRoutes(r chi.Router) {
	r.Get("/me/addresses", h.list)
	r.Post("/me/addresses", h.create)
	r.Patch("/me/addresses/{id}", h.update)
	r.Delete("/me/addresses/{id}", h.delete)
	r.Post("/me/addresses/{id}/default", h.setDefault)
}

func (h *AddressHandlers) userID(r *http.Request) (uuid.UUID, bool) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		return uuid.Nil, false
	}
	return sess.UserID, true
}

func (h *AddressHandlers) list(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	items, err := h.svc.List(r.Context(), uid)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	out := make([]AddressResponse, 0, len(items))
	for _, a := range items {
		out = append(out, toAddressResponse(a))
	}
	responsex.JSON(w, http.StatusOK, out)
}

type addressBody struct {
	RecipientName *string `json:"recipient_name"`
	PostalCode    *string `json:"postal_code"`
	Street        *string `json:"street"`
	Number        *string `json:"number"`
	Complement    *string `json:"complement"`
	Neighborhood  *string `json:"neighborhood"`
	City          *string `json:"city"`
	State         *string `json:"state"`
	IsDefault     bool    `json:"is_default"`
}

func (h *AddressHandlers) create(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	var b addressBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	a, err := h.svc.Create(r.Context(), application.CreateInput{
		UserID:        uid,
		RecipientName: deref(b.RecipientName),
		PostalCode:    deref(b.PostalCode),
		Street:        deref(b.Street),
		Number:        deref(b.Number),
		Complement:    b.Complement,
		Neighborhood:  deref(b.Neighborhood),
		City:          deref(b.City),
		State:         deref(b.State),
		IsDefault:     b.IsDefault,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusCreated, toAddressResponse(a))
}

func (h *AddressHandlers) update(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid address id")
		return
	}
	var b addressBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	a, err := h.svc.Update(r.Context(), application.UpdateInput{
		UserID: uid, ID: id,
		RecipientName: b.RecipientName, PostalCode: b.PostalCode, Street: b.Street,
		Number: b.Number, Complement: b.Complement, Neighborhood: b.Neighborhood,
		City: b.City, State: b.State,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toAddressResponse(a))
}

func (h *AddressHandlers) delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid address id")
		return
	}
	if err := h.svc.Delete(r.Context(), id, uid); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AddressHandlers) setDefault(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid address id")
		return
	}
	a, err := h.svc.SetDefault(r.Context(), id, uid)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toAddressResponse(a))
}

func (h *AddressHandlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := mapErrorToHTTP(err)
	responsex.ErrorWithCause(w, r, status, code, msg, err)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

- [ ] **Step 6: Write the module**

Create `internal/modules/address/module.go`:

```go
// Package address wires the address bounded context.
package address

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

// Module wires the address bounded context onto a chi router.
type Module struct {
	addresses     *transport.AddressHandlers
	cep           *transport.CEPHandler
	sessions      sessionauth.Manager
	sessionCookie string
	csrfCfg       csrf.Config
}

// Deps groups raw dependencies the address module needs.
type Deps struct {
	Pool          *pgxpool.Pool
	Sessions      sessionauth.Manager
	SessionCookie string
	CSRFCfg       csrf.Config
	ViaCEP        viacep.Lookuper
}

// New builds the address Module.
func New(d Deps) *Module {
	repo := infrastructure.New(d.Pool)
	svc := application.NewAddressService(repo)
	return &Module{
		addresses:     transport.NewAddressHandlers(svc),
		cep:           transport.NewCEPHandler(d.ViaCEP),
		sessions:      d.Sessions,
		sessionCookie: d.SessionCookie,
		csrfCfg:       d.CSRFCfg,
	}
}

// Mount registers the public CEP route and the authenticated address routes.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(public chi.Router) {
		m.cep.RegisterCEPRoutes(public)
	})
	r.Group(func(auth chi.Router) {
		auth.Use(sessionauth.Middleware(m.sessions, m.sessionCookie))
		auth.Use(csrf.Middleware(m.csrfCfg))
		m.addresses.RegisterAddressRoutes(auth)
	})
}
```

- [ ] **Step 7: Run tests + build**

Run: `go test ./internal/modules/address/... && go build ./...`
Expected: PASS + exit 0.

- [ ] **Step 8: Commit**

```bash
git add internal/modules/address/transport/ internal/modules/address/module.go
git commit -m "feat(address): add HTTP handlers, CEP lookup, and module wiring"
```

---
## Task 12: Wire merge into identity Login + wire all modules in `cmd/api`

**Files:**
- Modify: `internal/modules/identity/transport/auth_handlers.go` (merge hook), `internal/modules/identity/module.go` (Deps), `cmd/api/main.go` (build viacep/cart/address, wire merge)

**Interfaces:**
- Consumes: `cartModule.Merger()`, `cartModule.AnonCookieName()`, `viacep.NewClient`.

- [ ] **Step 1: Add the merge hook to identity AuthHandlers**

In `internal/modules/identity/transport/auth_handlers.go`, add imports `context` and `log/slog`, add fields to `AuthHandlers`, a setter, and a helper. Add to the struct:

```go
type AuthHandlers struct {
	svc            *application.IdentityService
	sessions       sessionauth.Manager
	cookies        CookieConfig
	cartMerge      func(ctx context.Context, anonID string, userID uuid.UUID) error
	cartCookieName string
}
```

Add the setter (after `NewAuthHandlers`):

```go
// SetCartMerge installs the optional cart-merge hook invoked on login.
// Passing a nil fn (or empty cookie name) disables merging.
func (h *AuthHandlers) SetCartMerge(fn func(ctx context.Context, anonID string, userID uuid.UUID) error, cookieName string) {
	h.cartMerge = fn
	h.cartCookieName = cookieName
}

// maybeMergeCart folds an anon cart into the just-authenticated user's cart.
// Best-effort: a merge failure never blocks login.
func (h *AuthHandlers) maybeMergeCart(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	if h.cartMerge == nil || h.cartCookieName == "" {
		return
	}
	c, err := r.Cookie(h.cartCookieName)
	if err != nil || c.Value == "" {
		return
	}
	if err := h.cartMerge(r.Context(), c.Value, userID); err != nil {
		slog.Default().Warn("cart merge on login failed", slog.String("error", err.Error()))
		return
	}
	clearCookie(w, h.cartCookieName)
}
```

In the `Login` handler, insert the merge call between `h.setSessionCookies(w, sess)` and the final `responsex.JSON(...)`:

```go
	h.setSessionCookies(w, sess)
	h.maybeMergeCart(w, r, user.ID)
	responsex.JSON(w, http.StatusOK, userResponse(user))
```

- [ ] **Step 2: Extend identity `Deps` + `New`**

In `internal/modules/identity/module.go`, add imports `context` and `github.com/google/uuid`, then add to `Deps`:

```go
	CartMerge      func(ctx context.Context, anonID string, userID uuid.UUID) error
	CartCookieName string
```

In `New`, build the auth handler, install the hook, then assemble the module:

```go
func New(d Deps) *Module {
	users := infrastructure.NewUserRepository(d.Pool)
	auths := infrastructure.NewAuthMethodRepository(d.Pool)
	verify := infrastructure.NewEmailVerifyTokenRepository(d.Pool)
	reset := infrastructure.NewPasswordResetTokenRepository(d.Pool)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users:                   users,
		AuthMethods:             auths,
		VerifyTokens:            verify,
		ResetTokens:             reset,
		Email:                   d.Email,
		VerifyLinkBaseURL:       d.Cfg.Email.VerifyLinkBaseURL,
		ResetLinkBaseURL:        d.Cfg.Email.ResetLinkBaseURL,
		RevokeAllSessions:       d.Sessions.DeleteAllForUser,
		RevokeAllSessionsExcept: d.Sessions.DeleteAllForUserExcept,
	})

	auth := transport.NewAuthHandlers(svc, d.Sessions, d.Cookies)
	auth.SetCartMerge(d.CartMerge, d.CartCookieName)

	return &Module{
		auth:     auth,
		me:       transport.NewMeHandlers(svc, d.Sessions, d.Cookies),
		sessions: d.Sessions,
		cookies:  d.Cookies,
		csrfCfg:  d.CSRFCfg,
	}
}
```

(Existing callers that omit `CartMerge`/`CartCookieName` — e.g. the Phase 2a E2E `support_test.go` — keep compiling; merge is simply disabled there.)

- [ ] **Step 3: Wire viacep + cart + address into `cmd/api/main.go`**

Add imports:

```go
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/address"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
```

After the existing `identityModule` block is removed/relocated, wire in this order (cart before identity so its merger is available; resolve the `cart_anon` cookie name with the secure prefix):

```go
	viacepClient := viacep.NewClient(
		&http.Client{Timeout: cfg.ViaCEP.Timeout},
		rdb,
		cfg.ViaCEP.BaseURL,
		cfg.ViaCEP.CacheTTL,
	)

	cartAnonCookie := cookieName("cart_anon", cfg.Cookies.SecurePrefix)

	cartModule := cart.New(cart.Deps{
		Pool:           pool,
		Sessions:       sessions,
		SessionCookie:  cookies.SessionName,
		AnonCookieName: cartAnonCookie,
	})

	identityModule := identity.New(identity.Deps{
		Pool:     pool,
		Redis:    rdb,
		Email:    emailSender,
		Sessions: sessions,
		Cookies:  cookies,
		CSRFCfg:  csrfCfg,
		RateLimitOpts: ratelimit.Options{
			Client:         rdb,
			TrustedProxies: trustedProxies,
		},
		Cfg:            cfg,
		CartMerge:      cartModule.Merger(),
		CartCookieName: cartModule.AnonCookieName(),
	})

	addressModule := address.New(address.Deps{
		Pool:          pool,
		Sessions:      sessions,
		SessionCookie: cookies.SessionName,
		CSRFCfg:       csrfCfg,
		ViaCEP:        viacepClient,
	})

	identityModule.Mount(router)
	cartModule.Mount(router)
	addressModule.Mount(router)
```

> Ensure the previous single `identityModule.Mount(router)` call is replaced by the three-module mount block above (don't double-mount identity).

- [ ] **Step 4: Verify build + Phase 2a tests still pass**

Run: `go build ./... && go test ./...`
Expected: exit 0; all unit tests PASS (the identity packages must remain green).

- [ ] **Step 5: Commit**

```bash
git add internal/modules/identity/ cmd/api/main.go
git commit -m "feat(commerce): wire cart/address/viacep and merge anon cart on login"
```

---

## Task 13: OpenAPI spec extension

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Add the tags**

Under the top-level `tags:` list, add:

```yaml
  - name: cart
    description: Shopping cart (anonymous or user)
  - name: address
    description: User shipping addresses and CEP lookup
```

- [ ] **Step 2: Add the cart + address paths**

Under `paths:`, add (schemas referenced inline for brevity):

```yaml
  /cart:
    get:
      tags: [cart]
      summary: Get current cart
      responses:
        '200': { description: Cart, content: { application/json: { schema: { $ref: '#/components/schemas/Cart' } } } }
    delete:
      tags: [cart]
      summary: Clear cart
      responses: { '204': { description: Cleared } }
  /cart/items:
    post:
      tags: [cart]
      summary: Add item (sets cart_anon cookie if anonymous)
      requestBody:
        required: true
        content: { application/json: { schema: { type: object, required: [variant_id, quantity], properties: { variant_id: { type: string, format: uuid }, quantity: { type: integer, minimum: 1, maximum: 99 } } } } }
      responses:
        '200': { description: Cart, content: { application/json: { schema: { $ref: '#/components/schemas/Cart' } } } }
        '404': { description: Variant not found }
        '422': { description: Invalid quantity }
  /cart/items/{id}:
    patch:
      tags: [cart]
      summary: Update item quantity
      parameters: [{ name: id, in: path, required: true, schema: { type: string, format: uuid } }]
      requestBody:
        required: true
        content: { application/json: { schema: { type: object, required: [quantity], properties: { quantity: { type: integer, minimum: 1, maximum: 99 } } } } }
      responses:
        '200': { description: Cart, content: { application/json: { schema: { $ref: '#/components/schemas/Cart' } } } }
        '404': { description: Item not found }
    delete:
      tags: [cart]
      summary: Remove item
      parameters: [{ name: id, in: path, required: true, schema: { type: string, format: uuid } }]
      responses:
        '200': { description: Cart, content: { application/json: { schema: { $ref: '#/components/schemas/Cart' } } } }
        '404': { description: Item not found }
  /address/cep/{cep}:
    get:
      tags: [address]
      summary: Lookup address by CEP (cached)
      parameters: [{ name: cep, in: path, required: true, schema: { type: string, pattern: '^[0-9]{8}$' } }]
      responses:
        '200': { description: Resolved address, content: { application/json: { schema: { $ref: '#/components/schemas/CEPResult' } } } }
        '400': { description: Invalid CEP }
        '404': { description: CEP not found }
  /me/addresses:
    get:
      tags: [address]
      summary: List addresses
      responses: { '200': { description: Addresses, content: { application/json: { schema: { type: array, items: { $ref: '#/components/schemas/Address' } } } } } }
    post:
      tags: [address]
      summary: Create address
      requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/AddressInput' } } } }
      responses: { '201': { description: Created, content: { application/json: { schema: { $ref: '#/components/schemas/Address' } } } }, '422': { description: Invalid } }
  /me/addresses/{id}:
    patch:
      tags: [address]
      summary: Update address (partial)
      parameters: [{ name: id, in: path, required: true, schema: { type: string, format: uuid } }]
      requestBody: { required: true, content: { application/json: { schema: { $ref: '#/components/schemas/AddressInput' } } } }
      responses: { '200': { description: Updated, content: { application/json: { schema: { $ref: '#/components/schemas/Address' } } } }, '404': { description: Not found } }
    delete:
      tags: [address]
      summary: Delete address
      parameters: [{ name: id, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { '204': { description: Deleted }, '404': { description: Not found } }
  /me/addresses/{id}/default:
    post:
      tags: [address]
      summary: Set address as default
      parameters: [{ name: id, in: path, required: true, schema: { type: string, format: uuid } }]
      responses: { '200': { description: Updated, content: { application/json: { schema: { $ref: '#/components/schemas/Address' } } } }, '404': { description: Not found } }
```

- [ ] **Step 3: Add the schemas**

Under `components.schemas`, add `Cart`, `CartItem`, `Address`, `AddressInput`, `CEPResult`:

```yaml
    CartItem:
      type: object
      properties:
        id: { type: string, format: uuid }
        variant_id: { type: string, format: uuid }
        quantity: { type: integer }
        unit_price_cents: { type: integer, format: int64 }
    Cart:
      type: object
      properties:
        items: { type: array, items: { $ref: '#/components/schemas/CartItem' } }
        subtotal_cents: { type: integer, format: int64 }
    Address:
      type: object
      properties:
        id: { type: string, format: uuid }
        recipient_name: { type: string }
        postal_code: { type: string }
        street: { type: string }
        number: { type: string }
        complement: { type: string, nullable: true }
        neighborhood: { type: string }
        city: { type: string }
        state: { type: string }
        is_default: { type: boolean }
    AddressInput:
      type: object
      properties:
        recipient_name: { type: string }
        postal_code: { type: string, pattern: '^[0-9]{8}$' }
        street: { type: string }
        number: { type: string }
        complement: { type: string, nullable: true }
        neighborhood: { type: string }
        city: { type: string }
        state: { type: string, minLength: 2, maxLength: 2 }
        is_default: { type: boolean }
    CEPResult:
      type: object
      properties:
        postal_code: { type: string }
        street: { type: string }
        neighborhood: { type: string }
        city: { type: string }
        state: { type: string }
```

- [ ] **Step 4: Validate YAML parses**

Run: `go run ./cmd/tools/seed --help 2>/dev/null; python3 -c "import yaml,sys; yaml.safe_load(open('api/openapi.yaml')); print('ok')"`
Expected: `ok` (no YAML parse error). (If python3/pyyaml unavailable, skip — the file is doc-only and not compiled.)

- [ ] **Step 5: Commit**

```bash
git add api/openapi.yaml
git commit -m "docs(api): extend OpenAPI spec with cart + address endpoints"
```

---

## Task 14: E2E integration suite

**Files:**
- Create: `internal/testutil/viacep.go`, `tests/integration/commerce_support_test.go`, `tests/integration/cart_e2e_test.go`, `tests/integration/address_e2e_test.go`, `tests/integration/viacep_e2e_test.go`

**Interfaces:**
- Consumes: all modules; reuses `fakeSender`, `emailCapture`, `postIdentityJSON`, `registerVerify`, `registerVerifyLogin`, `extractTokenFromBody` from the existing `tests/integration/support_test.go` (same `integration_test` package).

- [ ] **Step 1: Add the ViaCEP fixture helper**

Create `internal/testutil/viacep.go`:

```go
package testutil

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ViaCEPFixture is an httptest ViaCEP server with a hit counter.
type ViaCEPFixture struct {
	Server *httptest.Server
	hits   atomic.Int64
}

// Hits returns how many times the fixture was called.
func (f *ViaCEPFixture) Hits() int64 { return f.hits.Load() }

// NewViaCEPFixture serves a fixed payload for any /<cep>/json/ path and counts hits.
func NewViaCEPFixture(t *testing.T) *ViaCEPFixture {
	t.Helper()
	f := &ViaCEPFixture{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"Praça da Sé","bairro":"Sé","localidade":"São Paulo","uf":"SP"}`))
	}))
	t.Cleanup(f.Server.Close)
	return f
}
```

- [ ] **Step 2: Write the commerce E2E harness**

Create `tests/integration/commerce_support_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/address"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

type commerceEnv struct {
	srv       *httptest.Server
	emails    emailCapture
	pool      *pgxpool.Pool
	variantID uuid.UUID
	viacepHit func() int64
}

// startCommerceAPI boots identity + cart + address modules against testcontainers,
// using a fake email sender and a real ViaCEP client pointed at an httptest fixture.
func startCommerceAPI(t *testing.T, ctx context.Context) commerceEnv {
	t.Helper()

	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	addr := testutil.NewTestRedisAddr(t)

	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx).Err())

	variantID := seedVariant(t, ctx, pool)

	sender := &fakeSender{}
	sessions := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client: rdb, TTLDefault: time.Hour, TTLRememberMe: 2 * time.Hour, RefreshAfter: 30 * time.Minute,
	})
	cookies := transport.CookieConfig{SessionName: "session_id", CSRFName: "csrf_token"}
	csrfCfg := csrf.Config{AllowedOrigins: []string{}, CookieName: cookies.CSRFName}

	fixture := testutil.NewViaCEPFixture(t)
	viacepClient := viacep.NewClient(fixture.Server.Client(), rdb, fixture.Server.URL, time.Hour)

	cfg := config.Config{
		Email:   config.Email{Provider: "log", VerifyLinkBaseURL: "http://t/verify", ResetLinkBaseURL: "http://t/reset"},
		Session: config.Session{TTLDefault: time.Hour, TTLRememberMe: 2 * time.Hour, RefreshAfter: 30 * time.Minute},
	}

	cartModule := cart.New(cart.Deps{Pool: pool, Sessions: sessions, SessionCookie: "session_id", AnonCookieName: "cart_anon"})
	identityModule := identity.New(identity.Deps{
		Pool: pool, Redis: rdb, Email: sender, Sessions: sessions, Cookies: cookies, CSRFCfg: csrfCfg,
		RateLimitOpts: ratelimit.Options{Client: rdb}, Cfg: cfg,
		CartMerge: cartModule.Merger(), CartCookieName: cartModule.AnonCookieName(),
	})
	addressModule := address.New(address.Deps{Pool: pool, Sessions: sessions, SessionCookie: "session_id", CSRFCfg: csrfCfg, ViaCEP: viacepClient})

	router := chi.NewRouter()
	identityModule.Mount(router)
	cartModule.Mount(router)
	addressModule.Mount(router)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return commerceEnv{srv: srv, emails: sender, pool: pool, variantID: variantID, viacepHit: fixture.Hits}
}

// seedVariant inserts a product + variant (price 9900) and returns the variant ID.
func seedVariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	productID := uuid.New()
	variantID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO catalog_products
		(id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		VALUES ($1, $2, 'P', 'D', 'B', $3, 5000, 'BRL', 'published')`,
		productID, "slug-"+productID.String(), uuid.New())
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		VALUES ($1, $2, $3, 'M', 'Red', 9900)`, variantID, productID, "sku-"+variantID.String())
	require.NoError(t, err)
	return variantID
}
```

> Reuses `fakeSender`/`emailCapture` from `support_test.go`. If `seedVariant`'s product insert fails on a `category_id` FK, insert a `catalog_categories` row first (see Task 5 note).

- [ ] **Step 3: Write the cart E2E tests**

Create `tests/integration/cart_e2e_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCartE2E_AnonAddRemove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 2,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var anon *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "cart_anon" {
			anon = c
		}
	}
	var cart struct {
		Items         []struct{ ID string `json:"id"` } `json:"items"`
		SubtotalCents int64                              `json:"subtotal_cents"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cart))
	resp.Body.Close()
	require.NotNil(t, anon, "cart_anon cookie must be set")
	require.Len(t, cart.Items, 1)
	assert.Equal(t, int64(19800), cart.SubtotalCents)

	// remove the item using the anon cookie
	itemID := cart.Items[0].ID
	req, err := http.NewRequest(http.MethodDelete, env.srv.URL+"/cart/items/"+itemID, nil)
	require.NoError(t, err)
	req.AddCookie(anon)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, delResp.StatusCode)
	delResp.Body.Close()
}

func TestCartE2E_QtyClamp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 200,
	}, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	resp.Body.Close()
}

func TestCartE2E_MergeOnLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	// 1) anon adds an item
	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 3,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var anon *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "cart_anon" {
			anon = c
		}
	}
	resp.Body.Close()
	require.NotNil(t, anon)

	// 2) register + verify
	registerVerify(t, env.srv, env.emails, "merge@example.com", "S3cretPass!")

	// 3) login WITH the anon cart cookie → merge fires
	loginResp := postIdentityJSON(t, env.srv, "/auth/login", map[string]string{
		"email": "merge@example.com", "password": "S3cretPass!",
	}, []*http.Cookie{anon})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	var session, clearedAnon *http.Cookie
	for _, c := range loginResp.Cookies() {
		switch c.Name {
		case "session_id":
			session = c
		case "cart_anon":
			clearedAnon = c
		}
	}
	loginResp.Body.Close()
	require.NotNil(t, session)
	if clearedAnon != nil {
		assert.True(t, clearedAnon.MaxAge < 0, "cart_anon should be cleared on login")
	}

	// 4) user cart has the merged item
	req, err := http.NewRequest(http.MethodGet, env.srv.URL+"/cart", nil)
	require.NoError(t, err)
	req.AddCookie(session)
	getResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var cart struct {
		Items []struct{ Quantity int `json:"quantity"` } `json:"items"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&cart))
	getResp.Body.Close()
	require.Len(t, cart.Items, 1)
	assert.Equal(t, 3, cart.Items[0].Quantity)
}

func TestCartE2E_VariantDeleteBlockedByFK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 1,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Hard-deleting a referenced variant must fail with FK violation (23503).
	_, err := env.pool.Exec(ctx, `DELETE FROM catalog_variants WHERE id = $1`, env.variantID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "23503")
	_ = uuid.Nil
}
```

- [ ] **Step 4: Write the address E2E tests**

Create `tests/integration/address_e2e_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authedAddressReq builds an authenticated, CSRF-bearing request.
func authedAddressReq(t *testing.T, method, url, body string, cookies []*http.Cookie) *http.Request {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, url, nil)
	} else {
		r, err = http.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	require.NoError(t, err)
	for _, c := range cookies {
		r.AddCookie(c)
		if c.Name == "csrf_token" {
			r.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	return r
}

func TestAddressE2E_CRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)
	cookies := registerVerifyLogin(t, env.srv, env.emails, "addr@example.com", "S3cretPass!")

	// create
	body := `{"recipient_name":"Ana","postal_code":"01001000","street":"Sé","number":"1","neighborhood":"Sé","city":"São Paulo","state":"SP"}`
	resp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodPost, env.srv.URL+"/me/addresses", body, cookies))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created struct{ ID string `json:"id"` }
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()

	// list
	listResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodGet, env.srv.URL+"/me/addresses", "", cookies))
	require.NoError(t, err)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	listResp.Body.Close()
	assert.Len(t, list, 1)

	// patch
	patchResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodPatch, env.srv.URL+"/me/addresses/"+created.ID, `{"number":"222"}`, cookies))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, patchResp.StatusCode)
	patchResp.Body.Close()

	// delete
	delResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodDelete, env.srv.URL+"/me/addresses/"+created.ID, "", cookies))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()
}

func TestAddressE2E_DefaultUnique(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)
	cookies := registerVerifyLogin(t, env.srv, env.emails, "def@example.com", "S3cretPass!")

	mk := func(def bool) {
		body := `{"recipient_name":"Ana","postal_code":"01001000","street":"Sé","number":"1","neighborhood":"Sé","city":"SP","state":"SP","is_default":` + boolStr(def) + `}`
		resp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodPost, env.srv.URL+"/me/addresses", body, cookies))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}
	mk(true)
	mk(true)

	listResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodGet, env.srv.URL+"/me/addresses", "", cookies))
	require.NoError(t, err)
	var list []struct {
		IsDefault bool `json:"is_default"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	listResp.Body.Close()

	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
		}
	}
	assert.Equal(t, 1, defaults, "exactly one default address")
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
```

- [ ] **Step 5: Write the ViaCEP E2E test**

Create `tests/integration/viacep_e2e_test.go`:

```go
//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViaCEPE2E_LookupAndCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	for i := 0; i < 2; i++ {
		resp, err := http.Get(env.srv.URL + "/address/cep/01001000")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}

	assert.Equal(t, int64(1), env.viacepHit(), "second lookup must be served from Redis cache")
}
```

- [ ] **Step 6: Run the full integration suite**

Run (Docker/colima up — see `reference_local_dev_colima`):
```bash
export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE="/var/run/docker.sock"
go test -race -count=1 -tags=integration -timeout=15m ./...
```
Expected: all PASS, including `tests/integration` (identity + cart + address + viacep) and the new repository/job integration tests. Zero FAIL.

- [ ] **Step 7: Commit**

```bash
git add internal/testutil/viacep.go tests/integration/
git commit -m "test(integration): add commerce E2E suite (cart/merge/address/viacep)"
```

---

## Task 15: README env documentation + final validation + tag

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document the new env vars**

In `README.md`, under the "Environment variables" section, add a Phase 2b subsection:

```markdown
### Phase 2b — Commerce

| Variable | Default | Required | Description |
|---|---|---|---|
| `VIACEP_BASE_URL` | `https://viacep.com.br/ws` | no | ViaCEP API base (no trailing slash). |
| `VIACEP_TIMEOUT` | `3s` | no | Per-request timeout for CEP lookups. |
| `VIACEP_CACHE_TTL` | `1h` | no | Redis cache TTL for resolved CEPs. |
| `CART_ABANDONED_AFTER` | `168h` (7d) | no | Idle period after which an anonymous cart is marked abandoned. |
| `CART_CLEANUP_INTERVAL` | `6h` | no | How often the worker runs the abandoned-cart sweep. |

The `cart_anon` cookie (HttpOnly, 30d) identifies anonymous carts and is cleared on login when its contents merge into the user cart.
```

- [ ] **Step 2: Full verification**

Run:
```bash
go build ./...
go vet ./...
go test ./...                       # unit
export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"
go test -race -count=1 -tags=integration -timeout=15m ./...   # integration + E2E
```
Expected: all green, zero FAIL. Do not proceed to the tag until this passes (per `superpowers:verification-before-completion`).

- [ ] **Step 3: Commit the docs**

```bash
git add README.md
git commit -m "docs: document Phase 2b env vars in README"
```

- [ ] **Step 4: Finish the branch**

Use `superpowers:finishing-a-development-branch`. Following the Phase 1/2a flow (merge-local + tag):

```bash
git checkout main
git pull --ff-only
git merge --no-ff feat/phase-2b-commerce -m "merge: phase 2b commerce into main"
git tag -a v0.5.0-commerce -m "Phase 2b: Commerce (cart, addresses, ViaCEP)"
git push origin main
git push origin feat/phase-2b-commerce
git push origin v0.5.0-commerce
```

---

## Self-Review

**1. Spec coverage** — every in-scope Phase 2b item maps to a task:

| Spec item | Task(s) |
|---|---|
| Server-side anon cart + cookie | 1 (schema), 5 (repo), 7 (handlers/middleware/cookie) |
| User cart | 1, 5, 7 |
| Merge on login | 5 (repo tx), 12 (Login hook) |
| Cart item add/update/remove/clear | 4 (service), 5, 7 |
| Quantity cap (≤99) | 3 (domain), 1 (schema CHECK + LEAST), 14 (E2E) |
| Price snapshot | 1 (`GetVariantUnitPrice`), 4, 5 |
| Abandoned-cart cleanup job | 6 |
| Addresses CRUD | 8–11 |
| Atomic default uniqueness | 1 (partial index), 10 (tx), 14 (E2E) |
| ViaCEP proxy + Redis cache (1h) | 2, 11 (handler), 14 (cache E2E) |
| OpenAPI tags cart/address | 13 |
| E2E suites (spec §8 list) | 14 (AnonAddRemove, MergeOnLogin, QtyClamp, VariantDeleteBlockedByFK, AddressCRUD, DefaultUnique, ViaCEPLookupAndCache) |
| Config | 6 (Cart), ViaCEP already present |
| README env docs | 15 |

**2. Placeholder scan** — no "TBD"/"implement later"/"add error handling"/"similar to Task N". Every code step carries complete code. The one compile-assertion stub in Task 11 (`noopUseCase`) is explicitly bounded and optional.

**3. Type consistency:**
- `unit_price_cents` is `BIGINT`/`int64` everywhere (migration, `GetVariantUnitPrice` `::bigint` cast, `domain.CartItem.UnitPriceCents int64`, mapper, response).
- `domain.Owner` defined in Task 4 Step 1, consumed identically by service (Task 4), repo (Task 5), middleware + handlers (Task 7).
- `CartUseCase` (Task 7) method set matches `CartService` (Task 4) methods used by handlers.
- `viacep.Lookuper` (Task 2) implemented by `*Client` and `*FakeClient`; consumed by `CEPHandler` (Task 11) and address module Deps.
- Cart merge callback signature `func(ctx, anonID string, userID uuid.UUID) error` is identical in `cart.Module.Merger()` (Task 7), `identity.Deps.CartMerge` (Task 12), and `AuthHandlers.cartMerge` (Task 12).
- sqlc param/return names (`UpsertCartItemParams`, `GetAddressByIDParams`, pointer types for nullable `user_id`/`anon_session_id`) are flagged for post-`make sqlc-gen` confirmation in Tasks 5 and 10.

**Open risk to confirm during execution:** the exact sqlc-generated identifiers (param struct names, nullable pointer types, `state CHAR(2)` Go type) depend on the generator run in Task 1. Each repository task notes where to reconcile. Run `make sqlc-gen` and read the generated signatures before writing the repository code.

