# Phase 3a — Checkout Core (mock payment) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the full cart→quote→order→payment flow behind provider interfaces with a mock payment provider and mock shipping provider, hardened for money integrity per the design spec's §7.

**Architecture:** Five new bounded contexts (`inventory`, `ordering`, `payment`, `shipping`, `checkout`) following the existing five-layer module pattern. `checkout` orchestrates the others through application-layer ports (no import cycles); a checkout-owned reconciler applies payment-webhook effects (order transition + stock commit/release + coupon release) so `payment` stays decoupled from `ordering`/`inventory`. Stock safety is an atomic conditional decrement; payment confirmation is async via a signed, idempotent webhook; the customer is charged exactly the total locked in a persisted quote.

**Tech Stack:** Go 1.25, chi v5, pgx v5 + sqlc, Atlas, Redis 7, river (jobs), crypto/hmac + crypto/subtle, testify + testcontainers-go.

**Spec:** `docs/superpowers/specs/2026-06-22-phase-3a-checkout-core-design.md` — read §7 (Security & Money-Integrity Policy) before any money-path task.

## Global Constraints

- Module: `github.com/danilloboing/marketplace-golang`. Tools off PATH — prefix: `export PATH="$PATH:/usr/local/go/bin:/Users/danilloboing/go/bin"`. Integration/E2E need Docker via colima: `export DOCKER_HOST="unix://${HOME}/.colima/default/docker.sock"` (colima already running).
- Money is `BIGINT`/`int64` cents end-to-end. Quantities `INT`/`int32`, cap 1..99.
- Package naming: lowercase single word; no stutter; constructors `New`/`NewXxx`; sentinels `domain.ErrXxx` with package-prefixed messages (`"inventory: ..."`, `"ordering: ..."`, `"payment: ..."`, `"checkout: ..."`). One primary export per file. Co-located `_test.go`. Integration tests gated `//go:build integration`.
- Repositories: `Repository{pool, q}` + `New(pool)`; transactions via `r.pool.Begin` + `r.q.WithTx(tx)` + deferred `Rollback` + explicit `Commit`. Map `pgx.ErrNoRows` → domain sentinel at the boundary; wrap other errors `fmt.Errorf("<ctx>: <op>: %w", err)`; return sentinels UNWRAPPED.
- Transport: own `mapErrorToHTTP` per module (mirror identity) → `responsex.Error`/`ErrorWithCause`/`JSON`. NEVER `responsex.WriteError` (catalog-coupled). Authenticated handlers read `sessionauth.SessionFromContext`.
- Modules expose `New(Deps) *Module` + `Mount(chi.Router)`; wired manually in `cmd/api/main.go`.
- **Money-integrity rules are binding (spec §7).** The verbatim non-negotiables:
  - **C1:** `/payments/webhook` ALWAYS verifies an HMAC signature over the **raw request body** using `crypto/hmac` (SHA-256) + `subtle.ConstantTimeCompare`. No unsigned path.
  - **C2:** `expired` order is NOT terminal w.r.t. a `paid` event; a `paid` event re-reserves, and on stock-gone the order becomes `paid_awaiting_stock` (never dropped). The release job only expires orders still `pending_payment` with no `paid` charge, status-guarded.
  - **C3:** client supplies only `quote_id`, `shipping_address_id`, `shipping_service_id`, `coupon_code` — never a price/discount/total. Prices come from catalog, locked in `checkout_quotes`, honored at confirm.
  - **C4:** coupon redeem is one atomic conditional UPDATE (`WHERE used_count < usage_limit AND active AND not expired RETURNING`); 0 rows → reject.
  - **C5:** webhook dedup (`payment_webhook_events.event_id` PK) + effect in ONE transaction; verify `event.amount == charge.amount_cents == order.total_cents` before any transition; forward-only transitions.
  - **I3:** reserve = `UPDATE inventory_stock SET available=available-$qty, reserved=reserved+$qty, version=version+1 WHERE variant_id=$id AND available>=$qty` RETURNING; 0 rows → insufficient.
  - **I2:** multi-line reserve locks variants in **ascending `variant_id` order** in one tx; partial failure rolls back all.
  - **I4:** `total = max(0, subtotal + shipping - discount)`; `discount ≤ subtotal`; percent discount round-half-up: `(subtotal*pct + 50) / 100`.
  - **I5:** idempotency keyed `(user_id, key)` + `request_hash`; replay only on match; different body → 409; never crosses users.
  - **I6:** reservation transitions guarded `WHERE status='held'`.
  - **I7:** every `/me/orders` query scoped by session `user_id`.
- Commit after each green step (Conventional Commits).

## File Structure

```
db/migrations/20260612000001_checkout.sql
db/queries/inventory.sql · orders.sql · charges.sql · coupons.sql · checkout_quotes.sql
internal/config/config.go                  # +Checkout, +Payment, +Shipping sections (Task 1)

internal/modules/inventory/{domain,application,infrastructure,jobs,transport}/* + module.go
internal/modules/ordering/{domain,application,infrastructure,transport}/* + module.go
internal/modules/shipping/{domain,application,infrastructure}/* + module.go
internal/modules/payment/{domain,application,infrastructure,transport}/* + module.go
internal/modules/checkout/{domain,application,infrastructure,transport}/* + module.go

cmd/api/main.go        # wire 5 modules + provider factories + reconciler (Task 22)
cmd/worker/main.go     # register release_expired_reservations job (Task 6)
cmd/tools/seed/main.go # seed demo stock (Task 5)
api/openapi.yaml       # checkout/order/payment/admin tags (Task 23)
README.md              # Phase 3a env vars (Task 25)

tests/integration/checkout_e2e_test.go · checkout_support_test.go (Task 24)
internal/testutil/payment.go   # signed mock webhook helper (Task 24)
```

## Spec Corrections / decisions baked in

- Authoritative price = catalog current, locked in `checkout_quotes` at quote time; confirm honors it (no catalog re-check). Cart snapshot (2b) = drift display only.
- Coupon redemption released (`used_count--`, guarded) on `payment_failed`/`expired`.
- Webhook effect orchestration (cross-module) lives in a **checkout reconciler**; `payment` calls it via an injected `EventApplier` port → payment stays decoupled.
- `PriceReader` for checkout is satisfied by a small checkout-infra adapter calling the existing sqlc `GetVariantUnitPrice` (added in Phase 2b Task 1).

---

## Task 1: Migration + sqlc queries + config sections

**Files:**
- Create: `db/migrations/20260612000001_checkout.sql`, `db/queries/{inventory,orders,charges,coupons,checkout_quotes}.sql`
- Modify: `internal/config/config.go`, generated `internal/platform/postgres/queries/*`

**Interfaces:**
- Produces: tables + sqlc methods consumed by all later tasks (named in each query file below); `config.Checkout`, `config.Payment`, `config.Shipping`.

- [ ] **Step 1: Write the migration**

Create `db/migrations/20260612000001_checkout.sql` with the exact DDL from the spec §3 (all tables: `inventory_stock`, `stock_reservations`, `orders`, `order_items`, `order_status_transitions`, `charges`, `payment_webhook_events`, `coupons`, `checkout_quotes`, `idempotency_keys`, plus the deferred `stock_reservations_order_fk`). Copy §3 verbatim.

- [ ] **Step 2: Hash the migration**

Run: `atlas migrate hash --dir file://db/migrations`
Expected: `db/migrations/atlas.sum` updated, no error.

- [ ] **Step 3: Write the inventory queries**

Create `db/queries/inventory.sql`:

```sql
-- name: UpsertStock :one
INSERT INTO inventory_stock (variant_id, available, reserved, version)
VALUES ($1, $2, 0, 0)
ON CONFLICT (variant_id) DO UPDATE
SET available = EXCLUDED.available, version = inventory_stock.version + 1, updated_at = now()
WHERE inventory_stock.version = sqlc.arg(expected_version)
RETURNING *;

-- name: GetStock :one
SELECT * FROM inventory_stock WHERE variant_id = $1;

-- name: ReserveStock :one
UPDATE inventory_stock
SET available = available - sqlc.arg(qty), reserved = reserved + sqlc.arg(qty),
    version = version + 1, updated_at = now()
WHERE variant_id = sqlc.arg(variant_id) AND available >= sqlc.arg(qty)
RETURNING *;

-- name: CommitReservedStock :exec
UPDATE inventory_stock
SET reserved = reserved - sqlc.arg(qty), version = version + 1, updated_at = now()
WHERE variant_id = sqlc.arg(variant_id) AND reserved >= sqlc.arg(qty);

-- name: ReleaseReservedStock :exec
UPDATE inventory_stock
SET available = available + sqlc.arg(qty), reserved = reserved - sqlc.arg(qty),
    version = version + 1, updated_at = now()
WHERE variant_id = sqlc.arg(variant_id) AND reserved >= sqlc.arg(qty);

-- name: CreateReservation :one
INSERT INTO stock_reservations (order_id, variant_id, quantity, status, expires_at)
VALUES ($1, $2, $3, 'held', $4)
RETURNING *;

-- name: ListReservationsByOrder :many
SELECT * FROM stock_reservations WHERE order_id = $1;

-- name: SetReservationStatus :execrows
UPDATE stock_reservations SET status = sqlc.arg(new_status)
WHERE order_id = sqlc.arg(order_id) AND status = 'held';

-- name: ListExpiredHeldOrderIDs :many
SELECT DISTINCT order_id FROM stock_reservations
WHERE status = 'held' AND expires_at < $1;
```

- [ ] **Step 4: Write the orders queries**

Create `db/queries/orders.sql`:

```sql
-- name: CreateOrder :one
INSERT INTO orders (id, user_id, status, subtotal_cents, shipping_cents, discount_cents,
                    total_cents, coupon_code, address_snapshot, shipping_snapshot)
VALUES ($1, $2, 'pending_payment', $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: GetUserOrderByID :one
SELECT * FROM orders WHERE id = $1 AND user_id = $2;

-- name: ListOrdersByUser :many
SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: TransitionOrderStatus :execrows
UPDATE orders SET status = sqlc.arg(to_status), updated_at = now()
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);

-- name: SetOrderStatus :exec
UPDATE orders SET status = sqlc.arg(status), updated_at = now() WHERE id = sqlc.arg(id);

-- name: CreateOrderItem :one
INSERT INTO order_items (order_id, variant_id, quantity, unit_price_cents, product_snapshot)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListOrderItems :many
SELECT * FROM order_items WHERE order_id = $1 ORDER BY created_at;

-- name: RecordTransition :exec
INSERT INTO order_status_transitions (order_id, from_status, to_status, reason, actor)
VALUES ($1, $2, $3, $4, $5);
```

- [ ] **Step 5: Write the charges queries**

Create `db/queries/charges.sql`:

```sql
-- name: CreateCharge :one
INSERT INTO charges (order_id, provider, provider_charge_id, method, status, amount_cents, raw_payload)
VALUES ($1, $2, $3, $4, 'pending', $5, $6)
RETURNING *;

-- name: GetChargeByProviderID :one
SELECT * FROM charges WHERE provider = $1 AND provider_charge_id = $2;

-- name: GetChargeByOrder :one
SELECT * FROM charges WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: SetChargeStatus :exec
UPDATE charges SET status = sqlc.arg(status), updated_at = now() WHERE id = sqlc.arg(id);

-- name: HasPaidCharge :one
SELECT EXISTS (SELECT 1 FROM charges WHERE order_id = $1 AND status = 'paid');

-- name: InsertWebhookEvent :execrows
INSERT INTO payment_webhook_events (event_id, provider, charge_id)
VALUES ($1, $2, $3) ON CONFLICT (event_id) DO NOTHING;
```

- [ ] **Step 6: Write the coupons + checkout_quotes + idempotency queries**

Create `db/queries/coupons.sql`:

```sql
-- name: CreateCoupon :one
INSERT INTO coupons (code, type, value, expires_at, usage_limit, min_order_cents)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCouponByCode :one
SELECT * FROM coupons WHERE code = $1;

-- name: RedeemCoupon :one
UPDATE coupons SET used_count = used_count + 1
WHERE code = sqlc.arg(code) AND active
  AND (usage_limit IS NULL OR used_count < usage_limit)
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: ReleaseCoupon :exec
UPDATE coupons SET used_count = used_count - 1
WHERE code = sqlc.arg(code) AND used_count > 0;
```

Create `db/queries/checkout_quotes.sql`:

```sql
-- name: CreateQuote :one
INSERT INTO checkout_quotes (user_id, cart_fingerprint, lines_snapshot, shipping_snapshot,
                             coupon_code, subtotal_cents, shipping_cents, discount_cents,
                             total_cents, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetUserQuote :one
SELECT * FROM checkout_quotes WHERE id = $1 AND user_id = $2;

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys WHERE user_id = $1 AND key = $2;

-- name: PutIdempotencyKey :exec
INSERT INTO idempotency_keys (user_id, key, request_hash, order_id, response)
VALUES ($1, $2, $3, $4, $5);
```

- [ ] **Step 7: Add config sections**

In `internal/config/config.go`, add three structs and embed them in `Config`:

```go
// Checkout configures the checkout flow timings.
type Checkout struct {
	QuoteTTL          time.Duration `env:"CHECKOUT_QUOTE_TTL" envDefault:"15m"`
	ReservationTTL    time.Duration `env:"CHECKOUT_RESERVATION_TTL" envDefault:"30m"`
	ReleaseInterval   time.Duration `env:"CHECKOUT_RELEASE_INTERVAL" envDefault:"5m"`
}

// Payment configures the payment provider + webhook.
type Payment struct {
	Provider      string `env:"PAYMENT_PROVIDER" envDefault:"mock"`
	WebhookSecret string `env:"MOCK_WEBHOOK_SECRET" envDefault:"dev-mock-secret"`
}

// Shipping configures the shipping provider.
type Shipping struct {
	Provider string `env:"SHIPPING_PROVIDER" envDefault:"mock"`
}
```

Add `Checkout Checkout`, `Payment Payment`, `Shipping Shipping` to `Config`. `Load()` unchanged.

- [ ] **Step 8: Regenerate sqlc + build**

Run: `make sqlc-gen && go build ./...`
Expected: new methods in `internal/platform/postgres/queries/`; exit 0.

- [ ] **Step 9: Confirm generated identifiers**

Read `internal/platform/postgres/queries/` and note param struct names + nullable pointer types (e.g. `coupon_code`/`expires_at`/`usage_limit`/`min_order_cents` nullable → pointers; `:execrows` → `(int64, error)`; `HasPaidCharge`/`GetIdempotencyKey` shapes). Record them in the report for downstream tasks.

- [ ] **Step 10: Commit**

```bash
git add db/ internal/platform/postgres/queries/ internal/config/config.go
git commit -m "feat(db): add checkout/inventory/ordering/payment tables, queries, config"
```

---

## Task 2: `inventory/domain`

**Files:** Create `internal/modules/inventory/domain/{stock.go,errors.go}`; Test `stock_test.go`

**Interfaces:**
- Produces: `Stock{VariantID uuid.UUID; Available, Reserved, Version int}`; `Reservation{ID, OrderID, VariantID uuid.UUID; Quantity int; Status ReservationStatus; ExpiresAt time.Time}`; `ReservationStatus` consts `Held/Committed/Released`; sentinels `ErrInsufficientStock`, `ErrStockNotFound`, `ErrStockConflict`.

- [ ] **Step 1: Failing test** — create `stock_test.go`:

```go
package domain_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

func TestReservationStatus_Values(t *testing.T) {
	assert.Equal(t, domain.ReservationStatus("held"), domain.StatusHeld)
	assert.Equal(t, domain.ReservationStatus("committed"), domain.StatusCommitted)
	assert.Equal(t, domain.ReservationStatus("released"), domain.StatusReleased)
}

func TestErrors_Prefixed(t *testing.T) {
	assert.Contains(t, domain.ErrInsufficientStock.Error(), "inventory:")
	assert.Contains(t, domain.ErrStockNotFound.Error(), "inventory:")
}
```

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/modules/inventory/domain/...` → does not compile.

- [ ] **Step 3: Write `errors.go`:**

```go
package domain

import "errors"

var (
	ErrInsufficientStock = errors.New("inventory: insufficient stock")
	ErrStockNotFound     = errors.New("inventory: stock not found")
	ErrStockConflict     = errors.New("inventory: stock version conflict")
)
```

- [ ] **Step 4: Write `stock.go`:**

```go
// Package domain holds inventory value types and invariants.
package domain

import (
	"time"
	"github.com/google/uuid"
)

type ReservationStatus string

const (
	StatusHeld      ReservationStatus = "held"
	StatusCommitted ReservationStatus = "committed"
	StatusReleased  ReservationStatus = "released"
)

// Stock is the sellable inventory for one variant.
type Stock struct {
	VariantID uuid.UUID
	Available int
	Reserved  int
	Version   int
}

// Reservation is a hold on stock tied to an order.
type Reservation struct {
	ID        uuid.UUID
	OrderID   uuid.UUID
	VariantID uuid.UUID
	Quantity  int
	Status    ReservationStatus
	ExpiresAt time.Time
}
```

- [ ] **Step 5: Run, expect PASS.** **Step 6: Commit** `feat(inventory): add domain types and sentinel errors`.

---

## Task 3: `inventory/application` — InventoryService + ports

**Files:** Create `internal/modules/inventory/application/{ports.go,inventory_service.go}`; Test `inventory_service_test.go`

**Interfaces:**
- Consumes (Task 2): domain types.
- Produces:
  - `type StockRepository interface { Reserve(ctx, items []ReserveItem, orderID uuid.UUID, expiresAt time.Time) error; CommitForOrder(ctx, orderID uuid.UUID) error; ReleaseForOrder(ctx, orderID uuid.UUID) error; SetStock(ctx, variantID uuid.UUID, available, expectedVersion int) (domain.Stock, error); Get(ctx, variantID uuid.UUID) (domain.Stock, error) }`
  - `type ReserveItem struct { VariantID uuid.UUID; Quantity int }`
  - `type InventoryService struct{...}` + `NewInventoryService(StockRepository)`; methods `Reserve(ctx, []ReserveItem, orderID, expiresAt) error`, `Commit(ctx, orderID) error`, `Release(ctx, orderID) error`, `SetStock(ctx, variantID, available, version) (domain.Stock, error)`.

- [ ] **Step 1: Failing test** — `inventory_service_test.go` with an in-memory fake `StockRepository`. Tests: `Reserve` delegates with the items+order+expiry; `Reserve` propagates `ErrInsufficientStock`; `SetStock` propagates `ErrStockConflict`; `Commit`/`Release` delegate by order.

```go
package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

type fakeStock struct {
	reserveErr error
	committed  uuid.UUID
	released   uuid.UUID
}

func (f *fakeStock) Reserve(_ context.Context, _ []application.ReserveItem, _ uuid.UUID, _ time.Time) error {
	return f.reserveErr
}
func (f *fakeStock) CommitForOrder(_ context.Context, o uuid.UUID) error  { f.committed = o; return nil }
func (f *fakeStock) ReleaseForOrder(_ context.Context, o uuid.UUID) error { f.released = o; return nil }
func (f *fakeStock) SetStock(_ context.Context, v uuid.UUID, a, _ int) (domain.Stock, error) {
	return domain.Stock{VariantID: v, Available: a}, nil
}
func (f *fakeStock) Get(_ context.Context, v uuid.UUID) (domain.Stock, error) {
	return domain.Stock{VariantID: v}, nil
}

func TestInventoryService_Reserve_Insufficient(t *testing.T) {
	svc := application.NewInventoryService(&fakeStock{reserveErr: domain.ErrInsufficientStock})
	err := svc.Reserve(context.Background(), []application.ReserveItem{{VariantID: uuid.New(), Quantity: 2}}, uuid.New(), time.Now())
	require.ErrorIs(t, err, domain.ErrInsufficientStock)
}

func TestInventoryService_CommitRelease(t *testing.T) {
	f := &fakeStock{}
	svc := application.NewInventoryService(f)
	order := uuid.New()
	require.NoError(t, svc.Commit(context.Background(), order))
	assert.Equal(t, order, f.committed)
	require.NoError(t, svc.Release(context.Background(), order))
	assert.Equal(t, order, f.released)
}
```

- [ ] **Step 2: Run, FAIL.** **Step 3: Write `ports.go`:**

```go
// Package application holds inventory use cases and ports.
package application

import (
	"context"
	"time"
	"github.com/google/uuid"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

type ReserveItem struct {
	VariantID uuid.UUID
	Quantity  int
}

// StockRepository persists stock + reservations atomically.
type StockRepository interface {
	Reserve(ctx context.Context, items []ReserveItem, orderID uuid.UUID, expiresAt time.Time) error
	CommitForOrder(ctx context.Context, orderID uuid.UUID) error
	ReleaseForOrder(ctx context.Context, orderID uuid.UUID) error
	SetStock(ctx context.Context, variantID uuid.UUID, available, expectedVersion int) (domain.Stock, error)
	Get(ctx context.Context, variantID uuid.UUID) (domain.Stock, error)
}
```

- [ ] **Step 4: Write `inventory_service.go`:**

```go
package application

import (
	"context"
	"time"
	"github.com/google/uuid"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

type InventoryService struct{ repo StockRepository }

func NewInventoryService(repo StockRepository) *InventoryService { return &InventoryService{repo: repo} }

// Reserve holds stock for all items atomically (all-or-nothing).
func (s *InventoryService) Reserve(ctx context.Context, items []ReserveItem, orderID uuid.UUID, expiresAt time.Time) error {
	return s.repo.Reserve(ctx, items, orderID, expiresAt)
}

func (s *InventoryService) Commit(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.CommitForOrder(ctx, orderID)
}

func (s *InventoryService) Release(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.ReleaseForOrder(ctx, orderID)
}

func (s *InventoryService) SetStock(ctx context.Context, variantID uuid.UUID, available, version int) (domain.Stock, error) {
	return s.repo.SetStock(ctx, variantID, available, version)
}

var _ = time.Now
```

(Remove the `var _ = time.Now` once `time` is otherwise referenced; it's only there if the import would be unused — verify and delete.)

- [ ] **Step 5: Run, PASS.** **Step 6: Commit** `feat(inventory): add InventoryService and ports`.

---
## Task 4: `inventory/infrastructure` — atomic reserve/commit/release repo

**Files:** Create `internal/modules/inventory/infrastructure/{repository.go,mappers.go}`; Test `repository_test.go` (integration)

**Interfaces:** Consumes Task 1 sqlc (`ReserveStock`, `CommitReservedStock`, `ReleaseReservedStock`, `CreateReservation`, `ListReservationsByOrder`, `SetReservationStatus`, `UpsertStock`, `GetStock`); Task 3 `application.StockRepository`. Produces `New(pool) *Repository`.

This is the money-safety core: **I2 ascending lock order, I3 conditional decrement, I6 status-guarded transitions.**

- [ ] **Step 1: Failing integration test** — `repository_test.go`:

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func newRepo(t *testing.T, ctx context.Context) (*infrastructure.Repository, func(variant uuid.UUID, avail int) uuid.UUID) {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 8, MaxIdleConns: 2, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	repo := infrastructure.New(pool)
	seed := func(variant uuid.UUID, avail int) uuid.UUID {
		cat, prod := uuid.New(), uuid.New()
		_, e := pool.Exec(ctx, `INSERT INTO catalog_categories (id, slug, name) VALUES ($1,$2,'C')`, cat, "c-"+cat.String())
		require.NoError(t, e)
		_, e = pool.Exec(ctx, `INSERT INTO catalog_products (id,slug,name,description,brand,category_id,base_price_cents,currency,status) VALUES ($1,$2,'P','D','B',$3,5000,'BRL','published')`, prod, "p-"+prod.String(), cat)
		require.NoError(t, e)
		_, e = pool.Exec(ctx, `INSERT INTO catalog_variants (id,product_id,sku,size,color,price_cents) VALUES ($1,$2,$3,'M','R',9900)`, variant, prod, "s-"+variant.String())
		require.NoError(t, e)
		_, e = repo.SetStock(ctx, variant, avail, 0)
		require.NoError(t, e)
		return variant
	}
	return repo, seed
}

func TestRepo_ReserveCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, seed := newRepo(t, ctx)
	v := seed(uuid.New(), 10)
	order := uuid.New()
	require.NoError(t, mkOrder(t, ctx, repo, order))
	require.NoError(t, repo.Reserve(ctx, []application.ReserveItem{{VariantID: v, Quantity: 3}}, order, time.Now().Add(time.Hour)))
	st, _ := repo.Get(ctx, v)
	assert.Equal(t, 7, st.Available)
	assert.Equal(t, 3, st.Reserved)
	require.NoError(t, repo.CommitForOrder(ctx, order))
	st, _ = repo.Get(ctx, v)
	assert.Equal(t, 7, st.Available)
	assert.Equal(t, 0, st.Reserved)
}

func TestRepo_Reserve_Insufficient_RollsBackAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, seed := newRepo(t, ctx)
	a := seed(uuid.New(), 5)
	b := seed(uuid.New(), 1) // not enough for qty 2
	order := uuid.New()
	require.NoError(t, mkOrder(t, ctx, repo, order))
	err := repo.Reserve(ctx, []application.ReserveItem{{VariantID: a, Quantity: 2}, {VariantID: b, Quantity: 2}}, order, time.Now().Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrInsufficientStock)
	// a must be untouched (whole tx rolled back)
	st, _ := repo.Get(ctx, a)
	assert.Equal(t, 5, st.Available)
}

func TestRepo_Release(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, seed := newRepo(t, ctx)
	v := seed(uuid.New(), 4)
	order := uuid.New()
	require.NoError(t, mkOrder(t, ctx, repo, order))
	require.NoError(t, repo.Reserve(ctx, []application.ReserveItem{{VariantID: v, Quantity: 4}}, order, time.Now().Add(time.Hour)))
	require.NoError(t, repo.ReleaseForOrder(ctx, order))
	st, _ := repo.Get(ctx, v)
	assert.Equal(t, 4, st.Available)
	assert.Equal(t, 0, st.Reserved)
}

func TestRepo_SetStock_VersionConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, seed := newRepo(t, ctx)
	v := seed(uuid.New(), 1) // version now 1 after seed's set
	_, err := repo.SetStock(ctx, v, 50, 999) // wrong expected version
	require.ErrorIs(t, err, domain.ErrStockConflict)
	_ = sort.Ints
}
```

Create `repository_test_helpers_test.go` (a `mkOrder` SQL helper that inserts a minimal `orders` row so the reservation FK is satisfiable):

```go
//go:build integration

package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/infrastructure"
)

// mkOrder inserts a minimal user + order so stock_reservations.order_id FK holds.
func mkOrder(t *testing.T, ctx context.Context, repo *infrastructure.Repository, orderID uuid.UUID) error {
	t.Helper()
	pool := repo.Pool() // expose pool for tests (see repository.go)
	user := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,name) VALUES ($1,$2,'U')`, user, "u-"+user.String()+"@t.local"); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `INSERT INTO orders (id,user_id,status,subtotal_cents,shipping_cents,discount_cents,total_cents,address_snapshot,shipping_snapshot)
		VALUES ($1,$2,'pending_payment',0,0,0,0,'{}','{}')`, orderID, user)
	return err
}
```

- [ ] **Step 2: Run, FAIL** (`go test -tags=integration -run TestRepo ./internal/modules/inventory/infrastructure/...`).

- [ ] **Step 3: Write `mappers.go`:**

```go
// Package infrastructure adapts sqlc to the inventory domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapStock(r queries.InventoryStock) domain.Stock {
	return domain.Stock{VariantID: r.VariantID, Available: int(r.Available), Reserved: int(r.Reserved), Version: int(r.Version)}
}
```

- [ ] **Step 4: Write `repository.go`** (the money-safety core):

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.StockRepository = (*Repository)(nil)

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, q: queries.New(pool)} }

// Pool exposes the pool for integration-test seeding only.
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// Reserve holds stock for every item in ONE tx, locking variants in ascending
// id order (I2 deadlock avoidance). The conditional ReserveStock is the oversell
// guard (I3): zero rows → ErrInsufficientStock → whole tx rolls back.
func (r *Repository) Reserve(ctx context.Context, items []application.ReserveItem, orderID uuid.UUID, expiresAt time.Time) error {
	ordered := make([]application.ReserveItem, len(items))
	copy(ordered, items)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].VariantID.String() < ordered[j].VariantID.String() })

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventory repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	for _, it := range ordered {
		_, err := q.ReserveStock(ctx, queries.ReserveStockParams{VariantID: it.VariantID, Qty: int32(it.Quantity)})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrInsufficientStock
		}
		if err != nil {
			return fmt.Errorf("inventory repo: reserve %s: %w", it.VariantID, err)
		}
		if _, err := q.CreateReservation(ctx, queries.CreateReservationParams{
			OrderID: orderID, VariantID: it.VariantID, Quantity: int32(it.Quantity), ExpiresAt: expiresAt,
		}); err != nil {
			return fmt.Errorf("inventory repo: create reservation: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// CommitForOrder turns held reservations into committed (stock leaves), guarded
// by status (I6). Idempotent: a second call finds no held rows → no-op.
func (r *Repository) CommitForOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.resolve(ctx, orderID, domain.StatusCommitted)
}

// ReleaseForOrder returns held reservations to available, guarded by status (I6).
func (r *Repository) ReleaseForOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.resolve(ctx, orderID, domain.StatusReleased)
}

func (r *Repository) resolve(ctx context.Context, orderID uuid.UUID, to domain.ReservationStatus) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventory repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	res, err := q.ListReservationsByOrder(ctx, orderID)
	if err != nil {
		return fmt.Errorf("inventory repo: list reservations: %w", err)
	}
	for _, rv := range res {
		if domain.ReservationStatus(rv.Status) != domain.StatusHeld {
			continue
		}
		switch to {
		case domain.StatusCommitted:
			if err := q.CommitReservedStock(ctx, queries.CommitReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
				return fmt.Errorf("inventory repo: commit stock: %w", err)
			}
		case domain.StatusReleased:
			if err := q.ReleaseReservedStock(ctx, queries.ReleaseReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
				return fmt.Errorf("inventory repo: release stock: %w", err)
			}
		}
	}
	if _, err := q.SetReservationStatus(ctx, queries.SetReservationStatusParams{OrderID: orderID, NewStatus: string(to)}); err != nil {
		return fmt.Errorf("inventory repo: set reservation status: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repository) SetStock(ctx context.Context, variantID uuid.UUID, available, expectedVersion int) (domain.Stock, error) {
	row, err := r.q.UpsertStock(ctx, queries.UpsertStockParams{
		VariantID: variantID, Available: int32(available), ExpectedVersion: int32(expectedVersion),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, domain.ErrStockConflict
	}
	if err != nil {
		return domain.Stock{}, fmt.Errorf("inventory repo: set stock: %w", err)
	}
	return mapStock(row), nil
}

func (r *Repository) Get(ctx context.Context, variantID uuid.UUID) (domain.Stock, error) {
	row, err := r.q.GetStock(ctx, variantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, domain.ErrStockNotFound
	}
	if err != nil {
		return domain.Stock{}, fmt.Errorf("inventory repo: get stock: %w", err)
	}
	return mapStock(row), nil
}
```

> Confirm generated param names after Task 1 (`ReserveStockParams.Qty`, `CommitReservedStockParams`, `UpsertStockParams.ExpectedVersion`, `SetReservationStatusParams.NewStatus`). Adjust to the real sqlc names if they differ.

- [ ] **Step 5: Run, PASS** (4 tests). **Step 6: Commit** `feat(inventory): add atomic reserve/commit/release Postgres repo`.

---

## Task 5: `inventory/transport` (admin set-stock) + `inventory/module.go`

**Files:** Create `internal/modules/inventory/transport/{stock_handler.go,error_mapping.go}`, `internal/modules/inventory/module.go`; Test `stock_handler_test.go`; Modify `cmd/tools/seed/main.go`

**Interfaces:** Consumes Task 3 service. Produces `inventory.New(Deps{Pool, AdminToken}) *Module` with `Mount(chi.Router)` and `Service() *application.InventoryService` (exposed so checkout/ordering wiring can consume it).

- [ ] **Step 1: Failing handler test** — `stock_handler_test.go` (httptest + fake admin auth via the existing `adminauth.RequireToken`): `PUT /admin/variants/{id}/stock` with valid token + `{available:50}` → 200; version conflict → 409 `stock_conflict`; missing token → 401.

- [ ] **Step 2: Run, FAIL.** **Step 3: Write `error_mapping.go`:**

```go
package transport

import (
	"errors"
	"net/http"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrStockConflict):
		return http.StatusConflict, "stock_conflict", "stock version conflict"
	case errors.Is(err, domain.ErrStockNotFound):
		return http.StatusNotFound, "not_found", "stock not found"
	case errors.Is(err, domain.ErrInsufficientStock):
		return http.StatusUnprocessableEntity, "insufficient_stock", "insufficient stock"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
```

- [ ] **Step 4: Write `stock_handler.go`** — handler with `RegisterStockRoutes(chi.Router)` mounting `r.Put("/admin/variants/{id}/stock", ...)`; decodes `{available int, version int}`; parses variant uuid; calls `svc.SetStock`; 200 with stock JSON or mapped error. (Mirror catalog admin handler decode/uuid/responsex pattern.)

- [ ] **Step 5: Write `module.go`:**

```go
// Package inventory wires the inventory bounded context.
package inventory

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/adminauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/transport"
)

type Module struct {
	svc        *application.InventoryService
	handler    *transport.StockHandler
	adminToken string
}

type Deps struct {
	Pool       *pgxpool.Pool
	AdminToken string
}

func New(d Deps) *Module {
	svc := application.NewInventoryService(infrastructure.New(d.Pool))
	return &Module{svc: svc, handler: transport.NewStockHandler(svc), adminToken: d.AdminToken}
}

func (m *Module) Service() *application.InventoryService { return m.svc }

func (m *Module) Mount(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(adminauth.RequireToken(m.adminToken))
		m.handler.RegisterStockRoutes(admin)
	})
}
```

- [ ] **Step 6: Update seed** — in `cmd/tools/seed/main.go`, after creating demo variants, set stock for each (e.g. available=100) via direct SQL insert into `inventory_stock` (or call the inventory repo). Keep it simple: `INSERT INTO inventory_stock (variant_id, available, reserved, version) VALUES ($1, 100, 0, 0) ON CONFLICT DO NOTHING` per seeded variant.

- [ ] **Step 7: Run tests + build.** **Step 8: Commit** `feat(inventory): add admin set-stock endpoint, module, seed stock`.

---

## Task 6: `inventory/jobs` — release_expired_reservations + worker wiring

**Files:** Create `internal/modules/inventory/jobs/release_expired_reservations.go`; Test `release_expired_reservations_test.go` (integration); Modify `cmd/worker/main.go`

**Interfaces:** Consumes Task 1 sqlc (`ListExpiredHeldOrderIDs`, `HasPaidCharge`) + the inventory release + ordering transition. Produces `ReleaseExpiredReservationsArgs{}` (`Kind()`="inventory.release_expired_reservations"), worker, `RunReleaseExpiredOnce(ctx, pool, now time.Time) (int64, error)`.

**Important (C2):** the job must only expire orders still `pending_payment` AND with no `paid` charge, in a status-guarded transition — paid wins.

- [ ] **Step 1: Failing integration test** — seed: order A pending_payment with held reservation expired + no paid charge → after run, order A `expired`, stock released, reservation `released`. Order B pending_payment with expired reservation BUT a paid charge → after run, order B still NOT expired (paid wins). Assert both.

- [ ] **Step 2: Run, FAIL.** **Step 3: Write the job:**

```go
// Package jobs holds river workers for the inventory module.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

type ReleaseExpiredReservationsArgs struct{}

func (ReleaseExpiredReservationsArgs) Kind() string { return "inventory.release_expired_reservations" }

type ReleaseExpiredReservationsWorker struct {
	river.WorkerDefaults[ReleaseExpiredReservationsArgs]
	pool *pgxpool.Pool
}

func NewReleaseExpiredReservationsWorker(pool *pgxpool.Pool) *ReleaseExpiredReservationsWorker {
	return &ReleaseExpiredReservationsWorker{pool: pool}
}

func (w *ReleaseExpiredReservationsWorker) Work(ctx context.Context, _ *river.Job[ReleaseExpiredReservationsArgs]) error {
	_, err := RunReleaseExpiredOnce(ctx, w.pool, time.Now())
	return err
}

// RunReleaseExpiredOnce expires pending_payment orders whose held reservations
// are past expiry AND have no paid charge (paid wins, C2). Returns orders expired.
func RunReleaseExpiredOnce(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int64, error) {
	q := queries.New(pool)
	orderIDs, err := q.ListExpiredHeldOrderIDs(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("inventory jobs: list expired: %w", err)
	}
	var expired int64
	for _, orderID := range orderIDs {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return expired, fmt.Errorf("inventory jobs: begin: %w", err)
		}
		qx := q.WithTx(tx)
		paid, err := qx.HasPaidCharge(ctx, orderID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: has paid: %w", err)
		}
		if paid {
			_ = tx.Rollback(ctx) // paid wins — leave for the webhook
			continue
		}
		// guarded transition pending_payment -> expired
		n, err := qx.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{ID: orderID, FromStatus: "pending_payment", ToStatus: "expired"})
		if err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: transition: %w", err)
		}
		if n == 0 {
			_ = tx.Rollback(ctx) // already moved by a concurrent webhook
			continue
		}
		// release held reservations for this order
		res, err := qx.ListReservationsByOrder(ctx, orderID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return expired, err
		}
		for _, rv := range res {
			if rv.Status != "held" {
				continue
			}
			if err := qx.ReleaseReservedStock(ctx, queries.ReleaseReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
				_ = tx.Rollback(ctx)
				return expired, err
			}
		}
		if _, err := qx.SetReservationStatus(ctx, queries.SetReservationStatusParams{OrderID: orderID, NewStatus: "released"}); err != nil {
			_ = tx.Rollback(ctx)
			return expired, err
		}
		// release coupon redemption if any
		ord, err := qx.GetOrderByID(ctx, orderID)
		if err == nil && ord.CouponCode != nil {
			_ = qx.ReleaseCoupon(ctx, *ord.CouponCode)
		}
		if err := qx.RecordTransition(ctx, queries.RecordTransitionParams{OrderID: orderID, FromStatus: ptr("pending_payment"), ToStatus: "expired", Reason: "reservation_expired", Actor: "job"}); err != nil {
			_ = tx.Rollback(ctx)
			return expired, err
		}
		if err := tx.Commit(ctx); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}

func ptr(s string) *string { return &s }

var _ = pgx.ErrNoRows
```

> `from_status` is nullable in `order_status_transitions` → `RecordTransitionParams.FromStatus *string`. Confirm sqlc nullable pointer names. Remove the `var _ = pgx.ErrNoRows` if unused.

- [ ] **Step 4: Run, PASS.** **Step 5: Wire into `cmd/worker/main.go`** — import `inventoryjobs`, register the worker + a periodic job at `cfg.Checkout.ReleaseInterval` (`RunOnStart: false`), alongside the existing catalog + cart cleanups (do not remove them).

- [ ] **Step 6: Build + run.** **Step 7: Commit** `feat(inventory): add release_expired_reservations job with paid-wins guard`.

---

## Task 7: `shipping` module (ShippingProvider port + MockShipping)

**Files:** Create `internal/modules/shipping/domain/{quote.go,errors.go}`, `internal/modules/shipping/application/{ports.go,shipping_service.go}`, `internal/modules/shipping/infrastructure/mock.go`, `internal/modules/shipping/module.go`; Tests for domain + mock.

**Interfaces:** Produces `ShippingProvider` interface, `Quote{ServiceID, Name string; PriceCents int64; ETADays int}`, `QuoteRequest{PostalCode string; SubtotalCents int64}`, `MockShipping`, `shipping.New(Deps{Provider string}) *Module` with `Service() *application.ShippingService` (no HTTP routes in 3a — quotes are consumed by checkout).

- [ ] **Step 1: Failing test** — mock returns ≥2 services with positive prices/ETA for a valid CEP; `ShippingService.Quote` delegates.

- [ ] **Step 2: FAIL.** **Step 3: domain** (`quote.go` types + `errors.go` `ErrQuoteUnavailable`).

- [ ] **Step 4: ports.go:**

```go
package application

import (
	"context"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/domain"
)

type ShippingProvider interface {
	Quote(ctx context.Context, req domain.QuoteRequest) ([]domain.Quote, error)
}

type ShippingService struct{ provider ShippingProvider }

func NewShippingService(p ShippingProvider) *ShippingService { return &ShippingService{provider: p} }

func (s *ShippingService) Quote(ctx context.Context, req domain.QuoteRequest) ([]domain.Quote, error) {
	return s.provider.Quote(ctx, req)
}
```

- [ ] **Step 5: MockShipping** (`infrastructure/mock.go`) — returns two deterministic services, e.g. `{"pac","PAC",1990,8}` and `{"sedex","SEDEX",3490,3}`, prices/ETA optionally varied by CEP region prefix. `var _ application.ShippingProvider = (*MockShipping)(nil)`.

- [ ] **Step 6: module.go** — `New(Deps{Provider})` selects `MockShipping` when `Provider=="mock"` (factory mirrors `email.NewSenderFromConfig`); panics or errors on unknown provider until `melhorenvio` lands (3b). Exposes `Service()`. No `Mount` routes.

- [ ] **Step 7: Run, PASS, build.** **Step 8: Commit** `feat(shipping): add ShippingProvider port and mock implementation`.

---
## Task 8: `payment/domain` + `payment/application` (PaymentProvider port + MockProvider + HMAC)

**Files:** Create `internal/modules/payment/domain/{charge.go,event.go,errors.go}`, `internal/modules/payment/application/{ports.go,charge_service.go}`, `internal/modules/payment/infrastructure/mock_provider.go`; Tests for HMAC verify + mock charge.

**Interfaces:**
- Produces: `Charge{ID, OrderID uuid.UUID; Provider, ProviderChargeID, Method string; Status ChargeStatus; AmountCents int64}`; `ChargeStatus` consts `Pending/Paid/Failed/Refunded`; `Event{ID, Type string; ProviderChargeID string; AmountCents int64}` (Type ∈ `paid|failed`); errors `ErrInvalidSignature`, `ErrChargeNotFound`, `ErrAmountMismatch`.
- `PaymentProvider interface { CreateCharge(ctx, ChargeRequest) (Charge, error); VerifyWebhook(payload []byte, signature string) (domain.Event, error) }`
- `ChargeRequest{OrderID uuid.UUID; AmountCents int64; Method string}`
- `MockProvider` (impl) + `Sign(secret string, body []byte) string` helper (HMAC-SHA256 hex) used by both the mock and tests.
- `ChargeService` with `CreateCharge(ctx, ChargeRequest) (Charge, error)` (delegates to provider + persists via a `ChargeRepository` port defined here).

- [ ] **Step 1: Failing test** — `mock_provider_test.go`:

```go
package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/infrastructure"
)

func TestMockProvider_CreateCharge_Pending(t *testing.T) {
	p := infrastructure.NewMockProvider("secret")
	ch, err := p.CreateCharge(context.Background(), application.ChargeRequest{OrderID: uuid.New(), AmountCents: 19800, Method: "pix"})
	require.NoError(t, err)
	assert.Equal(t, domain.ChargePending, ch.Status)
	assert.NotEmpty(t, ch.ProviderChargeID)
	assert.Equal(t, int64(19800), ch.AmountCents)
}

func TestMockProvider_VerifyWebhook_GoodAndBadSignature(t *testing.T) {
	p := infrastructure.NewMockProvider("secret")
	body := []byte(`{"id":"evt_1","type":"paid","provider_charge_id":"mock_x","amount_cents":19800}`)
	sig := infrastructure.Sign("secret", body)

	ev, err := p.VerifyWebhook(body, sig)
	require.NoError(t, err)
	assert.Equal(t, "paid", ev.Type)
	assert.Equal(t, int64(19800), ev.AmountCents)

	_, err = p.VerifyWebhook(body, "deadbeef")
	require.ErrorIs(t, err, domain.ErrInvalidSignature)
}
```

- [ ] **Step 2: FAIL.** **Step 3: domain** — `charge.go` (Charge + ChargeStatus consts), `event.go` (Event), `errors.go` (the three sentinels, `payment:`-prefixed).

- [ ] **Step 4: ports.go** (PaymentProvider, ChargeRequest, ChargeRepository, ChargeService):

```go
package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

type ChargeRequest struct {
	OrderID     uuid.UUID
	AmountCents int64
	Method      string
}

type PaymentProvider interface {
	CreateCharge(ctx context.Context, req ChargeRequest) (domain.Charge, error)
	VerifyWebhook(payload []byte, signature string) (domain.Event, error)
}

type ChargeRepository interface {
	Create(ctx context.Context, c domain.Charge) (domain.Charge, error)
}

type ChargeService struct {
	provider PaymentProvider
	repo     ChargeRepository
}

func NewChargeService(p PaymentProvider, r ChargeRepository) *ChargeService { return &ChargeService{provider: p, repo: r} }

func (s *ChargeService) CreateCharge(ctx context.Context, req ChargeRequest) (domain.Charge, error) {
	ch, err := s.provider.CreateCharge(ctx, req)
	if err != nil {
		return domain.Charge{}, err
	}
	return s.repo.Create(ctx, ch)
}
```

- [ ] **Step 5: MockProvider** (`infrastructure/mock_provider.go`) — the HMAC core (C1):

```go
package infrastructure

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

// Sign returns hex(HMAC-SHA256(secret, body)). Shared by the mock and tests.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

type MockProvider struct{ secret string }

var _ application.PaymentProvider = (*MockProvider)(nil)

func NewMockProvider(secret string) *MockProvider { return &MockProvider{secret: secret} }

func (m *MockProvider) CreateCharge(_ context.Context, req application.ChargeRequest) (domain.Charge, error) {
	return domain.Charge{
		OrderID:          req.OrderID,
		Provider:         "mock",
		ProviderChargeID: "mock_" + req.OrderID.String(),
		Method:           req.Method,
		Status:           domain.ChargePending,
		AmountCents:      req.AmountCents,
	}, nil
}

type mockEvent struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	ProviderChargeID string `json:"provider_charge_id"`
	AmountCents      int64  `json:"amount_cents"`
}

// VerifyWebhook validates the HMAC over the raw body in constant time (C1), then decodes.
func (m *MockProvider) VerifyWebhook(payload []byte, signature string) (domain.Event, error) {
	expected := Sign(m.secret, payload)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return domain.Event{}, domain.ErrInvalidSignature
	}
	var e mockEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return domain.Event{}, domain.ErrInvalidSignature
	}
	return domain.Event{ID: e.ID, Type: e.Type, ProviderChargeID: e.ProviderChargeID, AmountCents: e.AmountCents}, nil
}
```

> `hmac.Equal` is constant-time — satisfies C1's timing requirement directly; do not use `==` on the hex strings.

- [ ] **Step 6: Run, PASS. Step 7: Commit** `feat(payment): add PaymentProvider port, mock provider, HMAC webhook verify`.

---

## Task 9: `payment/infrastructure` charge repo + `payment/transport` webhook + `payment/module.go`

**Files:** Create `internal/modules/payment/infrastructure/charge_repository.go`, `internal/modules/payment/transport/{webhook_handler.go,error_mapping.go}`, `internal/modules/payment/module.go`; Test `webhook_handler_test.go`.

**Interfaces:**
- Produces: `ChargeRepository` impl (`Create`, satisfies Task 8 port); `EventApplier interface { Apply(ctx context.Context, ev domain.Event) error }` (defined in payment/application, IMPLEMENTED by the checkout reconciler in Task 20 — payment never imports checkout); `payment.New(Deps{Pool, Provider PaymentProvider, Applier EventApplier}) *Module` with `Mount(chi.Router)` (mounts `POST /payments/webhook`) and `ChargeService()` accessor.

- [ ] **Step 1: Failing handler test** — `webhook_handler_test.go` (httptest + a fake `EventApplier` capturing calls + the real `MockProvider`): a body signed with the secret in header `X-Webhook-Signature` → 200 + applier called with decoded event; bad signature → 401, applier NOT called; missing signature → 401.

```go
package transport_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/transport"
)

type fakeApplier struct{ called bool; ev domain.Event }

func (f *fakeApplier) Apply(_ context.Context, ev domain.Event) error { f.called = true; f.ev = ev; return nil }

func TestWebhook_SignatureGate(t *testing.T) {
	provider := infrastructure.NewMockProvider("secret")
	applier := &fakeApplier{}
	h := transport.NewWebhookHandler(provider, applier)
	r := chi.NewRouter()
	h.RegisterWebhookRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := []byte(`{"id":"evt_1","type":"paid","provider_charge_id":"mock_x","amount_cents":100}`)
	good := infrastructure.Sign("secret", body)

	// bad signature → 401, applier not called
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/payments/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", "bad")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
	assert.False(t, applier.called)

	// good signature → 200, applier called
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/payments/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", good)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	assert.True(t, applier.called)
	assert.Equal(t, "paid", applier.ev.Type)
	_ = application.ChargeRequest{}
}
```

- [ ] **Step 2: FAIL.** **Step 3: charge_repository.go** — `Repository{pool,q}` + `New(pool)` + `Create(ctx, domain.Charge)` via sqlc `CreateCharge` (mirror cart/address infra). `var _ application.ChargeRepository = (*Repository)(nil)`.

- [ ] **Step 4: webhook_handler.go** — read the **raw body** (`io.ReadAll`), call `provider.VerifyWebhook(body, r.Header.Get("X-Webhook-Signature"))`; `ErrInvalidSignature` → 401 `invalid_signature`; on success call `applier.Apply(ctx, ev)`; applier error → 500 (logged); else 200. **Define `EventApplier` in payment/application.** Mounts `r.Post("/payments/webhook", ...)`. (Read raw body BEFORE any parsing — the signature is over raw bytes, C1.)

- [ ] **Step 5: module.go** — `New(Deps{Pool, Provider, Applier})`: builds charge repo + `ChargeService`; `Mount` registers the webhook route; `ChargeService()` accessor for checkout wiring.

- [ ] **Step 6: Run, PASS, build. Step 7: Commit** `feat(payment): add charge repo, signed webhook handler, module`.

---

## Task 10: `ordering/domain` — Order, OrderItem, state machine

**Files:** Create `internal/modules/ordering/domain/{order.go,state.go,errors.go}`; Test `state_test.go`

**Interfaces:**
- Produces: `Order{ID, UserID uuid.UUID; Status OrderStatus; Subtotal/Shipping/Discount/Total int64; CouponCode *string; AddressSnapshot, ShippingSnapshot json.RawMessage; ...}`; `OrderItem{...}`; `OrderStatus` consts `PendingPayment/Paid/PaymentFailed/Expired/PaidAwaitingStock`; `CanTransition(from, to OrderStatus) bool`; errors `ErrOrderNotFound`, `ErrInvalidTransition`.

- [ ] **Step 1: Failing test** — `state_test.go` asserting the §6 transition table:

```go
package domain_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

func TestCanTransition(t *testing.T) {
	ok := [][2]domain.OrderStatus{
		{domain.PendingPayment, domain.Paid},
		{domain.PendingPayment, domain.PaymentFailed},
		{domain.PendingPayment, domain.Expired},
		{domain.Expired, domain.Paid},
		{domain.Expired, domain.PaidAwaitingStock},
	}
	for _, p := range ok {
		assert.Truef(t, domain.CanTransition(p[0], p[1]), "%s->%s should be allowed", p[0], p[1])
	}
	bad := [][2]domain.OrderStatus{
		{domain.Paid, domain.PendingPayment},
		{domain.Paid, domain.Expired},
		{domain.PaymentFailed, domain.Paid},
		{domain.PendingPayment, domain.PaidAwaitingStock},
	}
	for _, p := range bad {
		assert.Falsef(t, domain.CanTransition(p[0], p[1]), "%s->%s should be denied", p[0], p[1])
	}
}
```

- [ ] **Step 2: FAIL.** **Step 3: errors.go** (`ErrOrderNotFound`, `ErrInvalidTransition`, `ordering:`-prefixed). **Step 4: state.go:**

```go
package domain

type OrderStatus string

const (
	PendingPayment     OrderStatus = "pending_payment"
	Paid               OrderStatus = "paid"
	PaymentFailed      OrderStatus = "payment_failed"
	Expired            OrderStatus = "expired"
	PaidAwaitingStock  OrderStatus = "paid_awaiting_stock"
)

var allowed = map[OrderStatus]map[OrderStatus]bool{
	PendingPayment: {Paid: true, PaymentFailed: true, Expired: true},
	Expired:        {Paid: true, PaidAwaitingStock: true},
}

// CanTransition reports whether from→to is a permitted order transition (§6).
func CanTransition(from, to OrderStatus) bool { return allowed[from][to] }
```

- [ ] **Step 5: order.go** (Order + OrderItem structs; `json.RawMessage` snapshots). **Step 6: Run PASS. Step 7: Commit** `feat(ordering): add Order domain + state machine`.

---

## Task 11: `ordering/application` — OrderService + ports

**Files:** Create `internal/modules/ordering/application/{ports.go,order_service.go}`; Test `order_service_test.go`

**Interfaces:**
- Produces:
  - `OrderRepository interface { Create(ctx, NewOrder) (domain.Order, error); GetByID(ctx, id uuid.UUID) (domain.Order, error); GetUserOrder(ctx, id, userID uuid.UUID) (domain.Order, error); ListByUser(ctx, userID uuid.UUID, limit int) ([]domain.Order, error); ListItems(ctx, orderID uuid.UUID) ([]domain.OrderItem, error) }`
  - `NewOrder` + `NewOrderItem` input structs (fields mirror the orders/order_items columns).
  - `OrderService` with `GetForUser(ctx, id, userID) (domain.Order, []domain.OrderItem, error)` (404 on cross-user, I7) and `ListForUser(ctx, userID, limit)`.
  - Note: order CREATION + transitions are driven inside the checkout confirm tx and the reconciler tx (Tasks 20-21) via the shared sqlc queries, not via this service — this service is the **read** surface + the typed `NewOrder` builder used by the checkout repo. Keep it read-focused.

- [ ] **Step 1: Failing test** — fake repo: `GetForUser` returns order+items; cross-user → `ErrOrderNotFound`. **Step 2: FAIL.** **Step 3: ports.go + order_service.go** (read methods delegate; GetForUser uses GetUserOrder, maps not-found). **Step 4: PASS. Step 5: Commit** `feat(ordering): add OrderService read surface + ports`.

---

## Task 12: `ordering/infrastructure` — Postgres repo

**Files:** Create `internal/modules/ordering/infrastructure/{repository.go,mappers.go}`; Test `repository_test.go` (integration)

**Interfaces:** Implements Task 11 `OrderRepository` via sqlc (`CreateOrder`, `GetOrderByID`, `GetUserOrderByID`, `ListOrdersByUser`, `ListOrderItems`, `CreateOrderItem`). Mirror cart/address infra (`Repository{pool,q}` + mappers + ErrNoRows→`ErrOrderNotFound`). JSONB snapshots map to/from `json.RawMessage`.

- [ ] **Step 1: Failing integration test** — insert a user, `Create` an order + items, `GetUserOrder` returns it; cross-user → `ErrOrderNotFound`; `ListByUser` ordered desc. **Step 2: FAIL.** **Step 3: mappers + repository** (full CRUD, mirror address repo). **Step 4: PASS. Step 5: Commit** `feat(ordering): add Postgres order repository`.

---

## Task 13: `ordering/transport` + `ordering/module.go`

**Files:** Create `internal/modules/ordering/transport/{order_handlers.go,responses.go,error_mapping.go}`, `internal/modules/ordering/module.go`; Test `order_handlers_test.go`

**Interfaces:** `GET /me/orders`, `GET /me/orders/{id}` behind sessionauth (read user from `sessionauth.SessionFromContext`, I7). `ordering.New(Deps{Pool, Sessions, SessionCookie}) *Module` with `Mount` (authenticated group: `sessionauth.Middleware`) and `Service()` accessor. Mirror identity me_handlers for the auth group + session read.

- [ ] **Step 1: Failing handler test** — fake service; `GET /me/orders/{id}` with session ctx → 200; without session → 401; cross-user (service returns ErrOrderNotFound) → 404. **Step 2: FAIL.** **Step 3-5: responses + error_mapping (`ErrOrderNotFound`→404) + handlers + module.** **Step 6: PASS, build. Step 7: Commit** `feat(ordering): add order read endpoints + module`.

---
## Task 14: `checkout/domain` — Coupon + money math (I4)

**Files:** Create `internal/modules/checkout/domain/{coupon.go,money.go,errors.go}`; Test `money_test.go`

**Interfaces:**
- Produces: `Coupon{Code string; Type CouponType; Value int64; ExpiresAt *time.Time; UsageLimit *int; UsedCount int; MinOrderCents *int64; Active bool}`; `CouponType` consts `Fixed/Percent`; `ComputeDiscount(t CouponType, value, subtotalCents int64) int64`; `ComputeTotal(subtotal, shipping, discount int64) int64`; errors `ErrCouponInvalid`, `ErrCouponUnavailable`, `ErrQuoteExpired`, `ErrQuoteNotFound`, `ErrCartChanged`, `ErrCartEmpty`, `ErrIdempotencyConflict`.

- [ ] **Step 1: Failing test** — `money_test.go` (the binding I4 math):

```go
package domain_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

func TestComputeDiscount_Fixed_CappedAtSubtotal(t *testing.T) {
	assert.Equal(t, int64(500), domain.ComputeDiscount(domain.Fixed, 500, 10000))
	assert.Equal(t, int64(10000), domain.ComputeDiscount(domain.Fixed, 99999, 10000)) // capped
}

func TestComputeDiscount_Percent_RoundHalfUp_Capped(t *testing.T) {
	// 10% of 19999 = 1999.9 → round-half-up → 2000
	assert.Equal(t, int64(2000), domain.ComputeDiscount(domain.Percent, 10, 19999))
	// 100% capped at subtotal
	assert.Equal(t, int64(5000), domain.ComputeDiscount(domain.Percent, 100, 5000))
}

func TestComputeTotal_NeverNegative(t *testing.T) {
	assert.Equal(t, int64(7990), domain.ComputeTotal(5000, 2990, 0))
	assert.Equal(t, int64(0), domain.ComputeTotal(5000, 0, 5000))
}
```

- [ ] **Step 2: FAIL.** **Step 3: errors.go** (all sentinels `checkout:`-prefixed). **Step 4: money.go:**

```go
package domain

// ComputeDiscount returns the discount in cents, round-half-up for percent,
// and capped at subtotal so it can never exceed the goods value (I4).
func ComputeDiscount(t CouponType, value, subtotalCents int64) int64 {
	var d int64
	switch t {
	case Fixed:
		d = value
	case Percent:
		d = (subtotalCents*value + 50) / 100 // round-half-up
	}
	if d > subtotalCents {
		d = subtotalCents
	}
	if d < 0 {
		d = 0
	}
	return d
}

// ComputeTotal = max(0, subtotal + shipping - discount) (I4).
func ComputeTotal(subtotal, shipping, discount int64) int64 {
	t := subtotal + shipping - discount
	if t < 0 {
		return 0
	}
	return t
}
```

- [ ] **Step 5: coupon.go** (Coupon + CouponType consts). **Step 6: PASS. Step 7: Commit** `feat(checkout): add coupon domain + money math`.

---

## Task 15: `checkout/application` coupon — CouponService + ports

**Files:** Create `internal/modules/checkout/application/{coupon_ports.go,coupon_service.go}`; Test `coupon_service_test.go`

**Interfaces:**
- Produces:
  - `CouponRepository interface { GetByCode(ctx, code string) (domain.Coupon, error); Redeem(ctx, code string) error; Release(ctx, code string) error; Create(ctx, NewCoupon) (domain.Coupon, error) }` (Redeem = the atomic conditional UPDATE, C4: maps 0-rows→`ErrCouponUnavailable`).
  - `CouponService` with `Validate(ctx, code string, subtotalCents int64) (discount int64, err error)` — checks active/expiry/min_order; computes discount; `ErrCouponInvalid` on missing/inactive/expired/min-not-met. `Create` for admin.

- [ ] **Step 1: Failing test** — fake repo: valid percent coupon → discount; expired → `ErrCouponInvalid`; below min_order → `ErrCouponInvalid`. **Step 2: FAIL.** **Step 3: coupon_ports.go** (interface + `NewCoupon` input). **Step 4: coupon_service.go:**

```go
package application

import (
	"context"
	"time"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

type CouponService struct{ repo CouponRepository }

func NewCouponService(repo CouponRepository) *CouponService { return &CouponService{repo: repo} }

// Validate returns the discount for an applicable coupon, or ErrCouponInvalid.
// It does NOT redeem — redemption is the atomic step inside confirm (C4).
func (s *CouponService) Validate(ctx context.Context, code string, subtotalCents int64) (int64, error) {
	c, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, domain.ErrCouponInvalid
	}
	if !c.Active {
		return 0, domain.ErrCouponInvalid
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(time.Now()) {
		return 0, domain.ErrCouponInvalid
	}
	if c.UsageLimit != nil && c.UsedCount >= *c.UsageLimit {
		return 0, domain.ErrCouponInvalid
	}
	if c.MinOrderCents != nil && subtotalCents < *c.MinOrderCents {
		return 0, domain.ErrCouponInvalid
	}
	return domain.ComputeDiscount(c.Type, c.Value, subtotalCents), nil
}
```

> `Validate` uses `time.Now()` directly (not workflow-restricted; this is normal app code). Redeem/Release are exercised in Task 18/19 integration.

- [ ] **Step 5: PASS. Step 6: Commit** `feat(checkout): add CouponService validation + ports`.

---

## Task 16: `checkout/application` — CheckoutService.Quote + orchestration ports

**Files:** Create `internal/modules/checkout/application/{ports.go,checkout_service.go}`; Test `quote_test.go`

**Interfaces:**
- Produces (consumed-from-other-modules ports, declared here):
  - `CartReader interface { Get(ctx, owner cartdomain.Owner) (cartdomain.Cart, error) }` — wait, to avoid importing cart domain, declare a checkout-local view: `CartReader interface { ActiveCart(ctx, userID uuid.UUID) (CartView, error) }` where `CartView{Lines []CartLine}`, `CartLine{VariantID uuid.UUID; Quantity int}`. The cart adapter (Task 18) maps cart→CartView.
  - `PriceReader interface { UnitPrice(ctx, variantID uuid.UUID) (int64, error) }`
  - `ShippingQuoter interface { Quote(ctx, postalCode string, subtotal int64) ([]ShippingOption, error) }`; `ShippingOption{ServiceID, Name string; PriceCents int64; ETADays int}`
  - `AddressReader interface { Get(ctx, addressID, userID uuid.UUID) (AddressView, error) }`; `AddressView{PostalCode string; Snapshot json.RawMessage}`
  - `QuoteRepository interface { Create(ctx, NewQuote) (domain.Quote, error); GetUserQuote(ctx, id, userID uuid.UUID) (domain.Quote, error) }`
  - `CheckoutService` with `Quote(ctx, QuoteInput) (QuoteResult, error)` (`QuoteInput{UserID, AddressID uuid.UUID; ServiceID, CouponCode string}`).
- The `cart_fingerprint` = hex(sha256) of sorted `variantID:qty` lines.

- [ ] **Step 1: Failing test** — fakes for all ports: happy quote computes subtotal from `PriceReader`, picks shipping option, applies coupon discount, persists a quote, returns totals; empty cart → `ErrCartEmpty`. **Step 2: FAIL.** **Step 3: ports.go** (all the interfaces + input/output structs + fingerprint helper). **Step 4: checkout_service.go Quote:**

```go
// (excerpt — Quote method)
func (s *CheckoutService) Quote(ctx context.Context, in QuoteInput) (QuoteResult, error) {
	cart, err := s.carts.ActiveCart(ctx, in.UserID)
	if err != nil {
		return QuoteResult{}, err
	}
	if len(cart.Lines) == 0 {
		return QuoteResult{}, domain.ErrCartEmpty
	}
	addr, err := s.addresses.Get(ctx, in.AddressID, in.UserID)
	if err != nil {
		return QuoteResult{}, err // ErrAddressNotFound surfaced
	}
	var subtotal int64
	lines := make([]QuoteLine, 0, len(cart.Lines))
	for _, l := range cart.Lines {
		price, err := s.prices.UnitPrice(ctx, l.VariantID)
		if err != nil {
			return QuoteResult{}, err
		}
		subtotal += price * int64(l.Quantity)
		lines = append(lines, QuoteLine{VariantID: l.VariantID, Quantity: l.Quantity, UnitPriceCents: price})
	}
	opts, err := s.shipping.Quote(ctx, addr.PostalCode, subtotal)
	if err != nil {
		return QuoteResult{}, err
	}
	chosen := pickShipping(opts, in.ServiceID) // chosen service, price re-derived server-side (C3)
	var discount int64
	if in.CouponCode != "" {
		discount, err = s.coupons.Validate(ctx, in.CouponCode, subtotal)
		if err != nil {
			return QuoteResult{}, err // ErrCouponInvalid
		}
	}
	total := domain.ComputeTotal(subtotal, chosen.PriceCents, discount)
	fp := fingerprint(cart.Lines)
	q, err := s.quotes.Create(ctx, NewQuote{
		UserID: in.UserID, CartFingerprint: fp, Lines: lines, Chosen: chosen,
		CouponCode: in.CouponCode, Subtotal: subtotal, Shipping: chosen.PriceCents,
		Discount: discount, Total: total, ExpiresAt: s.now().Add(s.quoteTTL),
	})
	if err != nil {
		return QuoteResult{}, err
	}
	return QuoteResult{QuoteID: q.ID, Lines: lines, Options: opts, Chosen: chosen, Subtotal: subtotal, Shipping: chosen.PriceCents, Discount: discount, Total: total, ExpiresAt: q.ExpiresAt}, nil
}
```

> `s.now` is a `func() time.Time` field (defaults to `time.Now`) for testability. `s.quoteTTL` from config. Write `pickShipping` (default cheapest when ServiceID empty; error `ErrQuoteNotFound`-style if a given ServiceID isn't in opts) and `fingerprint` (sha256 of sorted `variantID:qty`).

- [ ] **Step 5: PASS. Step 6: Commit** `feat(checkout): add CheckoutService.Quote orchestration + ports`.

---

## Task 17: `checkout/application` — CheckoutService.Confirm

**Files:** Modify `internal/modules/checkout/application/{ports.go,checkout_service.go}`; Test `confirm_test.go`

**Interfaces:**
- Produces:
  - `ConfirmRepository interface { ConfirmTx(ctx, ConfirmPlan) (domain.Order, error) }` — executes the WHOLE atomic confirm (idempotency insert, reserve, coupon redeem, order+items, cart convert, charge persist) in ONE tx (Task 18 implements). Returns the created order. Maps `ErrInsufficientStock`/`ErrCouponUnavailable`/`ErrIdempotencyConflict`.
  - `Idempotency interface { Lookup(ctx, userID uuid.UUID, key, requestHash string) (IdemHit, error) }` (replay/conflict before the tx).
  - `Charger interface { CreateCharge(ctx, orderID uuid.UUID, amount int64, method string) (ChargeView, error) }` (payment ChargeService adapter).
  - `CheckoutService.Confirm(ctx, ConfirmInput) (ConfirmResult, error)` (`ConfirmInput{UserID uuid.UUID; QuoteID uuid.UUID; IdempotencyKey string}`).

- [ ] **Step 1: Failing test** — `confirm_test.go` with fakes covering: (a) idempotency replay returns stored result without a 2nd ConfirmTx; (b) idempotency key reused with different quote → `ErrIdempotencyConflict`; (c) expired quote → `ErrQuoteExpired`; (d) cart fingerprint mismatch → `ErrCartChanged`; (e) ConfirmTx returns `ErrInsufficientStock` → propagated; (f) happy path returns order + charge.

- [ ] **Step 2: FAIL.** **Step 3: Confirm method:**

```go
func (s *CheckoutService) Confirm(ctx context.Context, in ConfirmInput) (ConfirmResult, error) {
	reqHash := hashConfirm(in.QuoteID)

	hit, err := s.idem.Lookup(ctx, in.UserID, in.IdempotencyKey, reqHash)
	if err != nil {
		return ConfirmResult{}, err // ErrIdempotencyConflict on body mismatch (I5)
	}
	if hit.Replay {
		return hit.Result, nil // exactly-once: return stored response
	}

	q, err := s.quotes.GetUserQuote(ctx, in.QuoteID, in.UserID)
	if err != nil {
		return ConfirmResult{}, domain.ErrQuoteNotFound
	}
	if !q.ExpiresAt.After(s.now()) {
		return ConfirmResult{}, domain.ErrQuoteExpired
	}

	cart, err := s.carts.ActiveCart(ctx, in.UserID)
	if err != nil {
		return ConfirmResult{}, err
	}
	if fingerprint(cart.Lines) != q.CartFingerprint {
		return ConfirmResult{}, domain.ErrCartChanged
	}

	// Mock charge is pure/in-process — safe to mint before the tx (3a). 3b moves
	// the real external charge to just-after-commit with compensation (spec §12).
	charge, err := s.charger.CreateCharge(ctx, q.ProposedOrderID(), q.Total, "pix")
	if err != nil {
		return ConfirmResult{}, err
	}

	order, err := s.repo.ConfirmTx(ctx, ConfirmPlan{
		UserID: in.UserID, Quote: q, Cart: cart, Charge: charge,
		IdempotencyKey: in.IdempotencyKey, RequestHash: reqHash,
	})
	if err != nil {
		return ConfirmResult{}, err // ErrInsufficientStock / ErrCouponUnavailable mapped in transport
	}
	res := ConfirmResult{Order: order, Charge: charge}
	return res, nil
}
```

> The idempotency-key row is inserted **inside** `ConfirmTx` (atomic with the order), and `Lookup` does the pre-check/replay. `q.ProposedOrderID()` returns a deterministic order id generated at quote time so the mock charge's `provider_charge_id` (`mock_<orderID>`) is stable for replays — store `proposed_order_id` on the quote, OR generate the order id in the service and thread it through ConfirmPlan. Simpler: generate `orderID := uuid.New()` in Confirm, pass into both CreateCharge and ConfirmPlan. Update accordingly (drop `ProposedOrderID`).

- [ ] **Step 4: PASS. Step 5: Commit** `feat(checkout): add CheckoutService.Confirm orchestration`.

---
## Task 18: `checkout/infrastructure` — quote/idempotency/coupon repos + atomic ConfirmTx + PriceReader

**Files:** Create `internal/modules/checkout/infrastructure/{quote_repo.go,coupon_repo.go,idempotency.go,confirm_repo.go,price_reader.go,adapters.go}`; Tests `confirm_repo_test.go` (integration)

**Boundary note:** checkout's confirm/reconcile cross bounded contexts atomically. For the *atomic write* they use the shared `internal/platform/postgres/queries` (the same data layer every module imports) inside one tx — they do NOT import other modules' infrastructure. Reads/validation still go through application ports (Task 16/17).

**Interfaces:** Implements `QuoteRepository`, `CouponRepository`, `Idempotency`, `ConfirmRepository`, `PriceReader` from Tasks 15-17. `ConfirmTx` is the §5-confirm transaction (C3 honor quote, I2/I3 reserve, C4 coupon, cart-guard, charge).

- [ ] **Step 1: Failing integration test** — `confirm_repo_test.go`: seed user+category+product+variant+stock+active-cart+quote; `ConfirmTx` → order `pending_payment`, stock reserved, cart `converted`, charge `pending`, idempotency row. Then: oversell (stock < qty) → `ErrInsufficientStock` + nothing persisted; coupon at limit → `ErrCouponUnavailable` + rollback; second ConfirmTx same key → `ErrIdempotencyConflict`.

- [ ] **Step 2: FAIL.** **Step 3:** small repos `quote_repo.go` (CreateQuote/GetUserQuote via sqlc + JSONB marshal of lines/shipping), `coupon_repo.go` (GetByCode/Create/Redeem[0-rows→ErrCouponUnavailable]/Release), `idempotency.go` (Lookup: GetIdempotencyKey → if found & hash matches → Replay with decoded response; found & hash differs → ErrIdempotencyConflict; not found → no replay), `price_reader.go` (`UnitPrice` via sqlc `GetVariantUnitPrice`), `adapters.go` (CartReader/AddressReader/ShippingQuoter/Charger adapters wrapping the cart/address/shipping/payment services into checkout's port shapes).

- [ ] **Step 4: Write `confirm_repo.go`** (the atomic confirm — money-critical):

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

type ConfirmRepo struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.ConfirmRepository = (*ConfirmRepo)(nil)

func NewConfirmRepo(pool *pgxpool.Pool) *ConfirmRepo { return &ConfirmRepo{pool: pool, q: queries.New(pool)} }

// ConfirmTx runs the whole §5 confirm atomically. Order id is plan.OrderID
// (minted by the service so the charge's provider_charge_id is replay-stable).
func (r *ConfirmRepo) ConfirmTx(ctx context.Context, plan application.ConfirmPlan) (domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	// Cart-guard idempotency: the active cart must still exist (a concurrent
	// confirm that already converted it leaves none → ErrCartChanged).
	cart, err := q.GetActiveCartByUser(ctx, &plan.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrCartChanged
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: get cart: %w", err)
	}

	// Order first (reservations FK to it).
	ord, err := q.CreateOrder(ctx, queries.CreateOrderParams{
		ID: plan.OrderID, UserID: plan.UserID,
		SubtotalCents: plan.Quote.Subtotal, ShippingCents: plan.Quote.Shipping,
		DiscountCents: plan.Quote.Discount, TotalCents: plan.Quote.Total,
		CouponCode: nilIfEmpty(plan.Quote.CouponCode),
		AddressSnapshot: plan.Quote.AddressSnapshot, ShippingSnapshot: plan.Quote.ShippingSnapshot,
	})
	if err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: create order: %w", err)
	}

	// Reserve stock — ascending variant order (I2), conditional decrement (I3).
	lines := append([]application.QuoteLine(nil), plan.Quote.Lines...)
	sort.Slice(lines, func(i, j int) bool { return lines[i].VariantID.String() < lines[j].VariantID.String() })
	for _, l := range lines {
		if _, err := q.ReserveStock(ctx, queries.ReserveStockParams{VariantID: l.VariantID, Qty: int32(l.Quantity)}); errors.Is(err, pgx.ErrNoRows) {
			return domain.Order{}, domain.ErrInsufficientStock
		} else if err != nil {
			return domain.Order{}, fmt.Errorf("checkout repo: reserve: %w", err)
		}
		if _, err := q.CreateReservation(ctx, queries.CreateReservationParams{
			OrderID: plan.OrderID, VariantID: l.VariantID, Quantity: int32(l.Quantity), ExpiresAt: plan.ReservationExpiresAt,
		}); err != nil {
			return domain.Order{}, fmt.Errorf("checkout repo: reservation: %w", err)
		}
	}

	// Order items (original order, not the reserve sort).
	for _, l := range plan.Quote.Lines {
		if _, err := q.CreateOrderItem(ctx, queries.CreateOrderItemParams{
			OrderID: plan.OrderID, VariantID: l.VariantID, Quantity: int32(l.Quantity),
			UnitPriceCents: l.UnitPriceCents, ProductSnapshot: l.ProductSnapshot,
		}); err != nil {
			return domain.Order{}, fmt.Errorf("checkout repo: order item: %w", err)
		}
	}

	// Coupon redeem — atomic conditional (C4).
	if plan.Quote.CouponCode != "" {
		if _, err := q.RedeemCoupon(ctx, plan.Quote.CouponCode); errors.Is(err, pgx.ErrNoRows) {
			return domain.Order{}, domain.ErrCouponUnavailable
		} else if err != nil {
			return domain.Order{}, fmt.Errorf("checkout repo: redeem coupon: %w", err)
		}
	}

	// Convert the cart (cart-guard: unique active index → one winner).
	if err := q.SetCartStatus(ctx, queries.SetCartStatusParams{ID: cart.ID, Status: "converted"}); err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: convert cart: %w", err)
	}

	// Persist the (mock, pure) charge.
	if _, err := q.CreateCharge(ctx, queries.CreateChargeParams{
		OrderID: plan.OrderID, Provider: plan.Charge.Provider, ProviderChargeID: plan.Charge.ProviderChargeID,
		Method: plan.Charge.Method, AmountCents: plan.Charge.AmountCents, RawPayload: nil,
	}); err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: charge: %w", err)
	}

	// Idempotency key — race guard; (user,key) PK conflict → ErrIdempotencyConflict.
	if err := q.PutIdempotencyKey(ctx, queries.PutIdempotencyKeyParams{
		UserID: plan.UserID, Key: plan.IdempotencyKey, RequestHash: plan.RequestHash,
		OrderID: &plan.OrderID, Response: plan.ResponseJSON,
	}); isUniqueViolation(err) {
		return domain.Order{}, domain.ErrIdempotencyConflict
	} else if err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: idempotency: %w", err)
	}

	if err := q.RecordTransition(ctx, queries.RecordTransitionParams{
		OrderID: plan.OrderID, FromStatus: nil, ToStatus: "pending_payment", Reason: "checkout_confirmed", Actor: "system",
	}); err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("checkout repo: commit: %w", err)
	}
	return mapOrder(ord), nil
}
```

(`isUniqueViolation` = the `pgconn.PgError` `23505` helper from the cart merge fix — copy it into a checkout helper. `nilIfEmpty(string) *string`, `mapOrder` in `adapters.go`. Confirm sqlc param names after Task 1.)

- [ ] **Step 5: Run, PASS** (Docker). **Step 6: Commit** `feat(checkout): add atomic ConfirmTx, quote/coupon/idempotency repos, price reader`.

---

## Task 19: `checkout` reconciler — the webhook EventApplier (C2, C5)

**Files:** Create `internal/modules/checkout/application/reconciler.go` (port shape) + `internal/modules/checkout/infrastructure/reconcile_repo.go`; Test `reconcile_repo_test.go` (integration)

**Interfaces:** Produces `Reconciler` implementing payment's `EventApplier` (`Apply(ctx, payment.Event) error`). The whole effect is ONE tx via shared sqlc: dedup (C5), amount verify (C5), forward-only transition, stock commit/release, coupon release, paid-after-expiry re-reserve (C2 via savepoint).

- [ ] **Step 1: Failing integration test** — seed a confirmed order (pending_payment + held reservation + pending charge). Cases:
  - `paid` event (valid sig built in test, but Apply takes a decoded Event) → order `paid`, reservation `committed`, charge `paid`, stock reserved→sold.
  - duplicate `paid` (same event_id) → second Apply no-op (idempotent), order stays `paid`, stock not double-committed.
  - amount mismatch (event.amount ≠ order.total) → no transition, order stays `pending_payment`.
  - `failed` event → order `payment_failed`, reservation `released`, stock restored, coupon released.
  - paid-after-expiry with stock available → order `paid`; with stock gone → order `paid_awaiting_stock`.

- [ ] **Step 2: FAIL.** **Step 3: Write `reconcile_repo.go`:**

```go
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
	paymentdomain "github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

type ReconcileRepo struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

func NewReconcileRepo(pool *pgxpool.Pool) *ReconcileRepo { return &ReconcileRepo{pool: pool, q: queries.New(pool)} }

// Apply processes one verified payment event atomically (C5 dedup+amount+forward;
// C2 paid-after-expiry). Returns nil on no-op (dup/unknown/mismatch/invalid transition).
func (r *ReconcileRepo) Apply(ctx context.Context, ev paymentdomain.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reconcile: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	// C5 idempotency: dedup on event_id. 0 rows inserted → already processed.
	n, err := q.InsertWebhookEvent(ctx, queries.InsertWebhookEventParams{EventID: ev.ID, Provider: "mock", ChargeID: nil})
	if err != nil {
		return fmt.Errorf("reconcile: dedup: %w", err)
	}
	if n == 0 {
		return tx.Commit(ctx) // duplicate — no-op
	}

	charge, err := q.GetChargeByProviderID(ctx, queries.GetChargeByProviderIDParams{Provider: "mock", ProviderChargeID: ev.ProviderChargeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx) // unknown charge — record event, no-op
	}
	if err != nil {
		return fmt.Errorf("reconcile: get charge: %w", err)
	}
	ord, err := q.GetOrderByID(ctx, charge.OrderID)
	if err != nil {
		return fmt.Errorf("reconcile: get order: %w", err)
	}
	// C5 amount integrity: event == charge == order.total.
	if ev.AmountCents != charge.AmountCents || charge.AmountCents != ord.TotalCents {
		// mismatch — record event (anti-replay) but apply nothing; alert via log upstream.
		return tx.Commit(ctx)
	}

	switch ev.Type {
	case "failed":
		if ord.Status != "pending_payment" {
			return tx.Commit(ctx) // forward-only
		}
		if err := releaseHeld(ctx, q, ord.ID); err != nil {
			return err
		}
		if ord.CouponCode != nil {
			_ = q.ReleaseCoupon(ctx, *ord.CouponCode)
		}
		_ = q.SetChargeStatus(ctx, queries.SetChargeStatusParams{ID: charge.ID, Status: "failed"})
		if _, err := q.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{ID: ord.ID, FromStatus: "pending_payment", ToStatus: "payment_failed"}); err != nil {
			return err
		}
		_ = q.RecordTransition(ctx, queries.RecordTransitionParams{OrderID: ord.ID, FromStatus: ptr("pending_payment"), ToStatus: "payment_failed", Reason: "charge_failed", Actor: "webhook"})
		return tx.Commit(ctx)

	case "paid":
		switch ord.Status {
		case "pending_payment":
			if err := commitHeld(ctx, q, ord.ID); err != nil {
				return err
			}
			_ = q.SetChargeStatus(ctx, queries.SetChargeStatusParams{ID: charge.ID, Status: "paid"})
			_, _ = q.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{ID: ord.ID, FromStatus: "pending_payment", ToStatus: "paid"})
			_ = q.RecordTransition(ctx, queries.RecordTransitionParams{OrderID: ord.ID, FromStatus: ptr("pending_payment"), ToStatus: "paid", Reason: "charge_paid", Actor: "webhook"})
			return tx.Commit(ctx)

		case "expired":
			// C2: payment landed after expiry. Re-reserve in a savepoint; if stock
			// is gone, the order becomes paid_awaiting_stock (never dropped).
			ok, err := reReserveSavepoint(ctx, tx, q, ord.ID)
			if err != nil {
				return err
			}
			_ = q.SetChargeStatus(ctx, queries.SetChargeStatusParams{ID: charge.ID, Status: "paid"})
			to := "paid"
			reason := "charge_paid_after_expiry"
			if !ok {
				to = "paid_awaiting_stock"
				reason = "paid_no_stock"
			}
			if _, err := q.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{ID: ord.ID, FromStatus: "expired", ToStatus: to}); err != nil {
				return err
			}
			_ = q.RecordTransition(ctx, queries.RecordTransitionParams{OrderID: ord.ID, FromStatus: ptr("expired"), ToStatus: to, Reason: reason, Actor: "webhook"})
			return tx.Commit(ctx)

		default:
			return tx.Commit(ctx) // already paid / failed — forward-only no-op
		}
	}
	return tx.Commit(ctx)
}

func commitHeld(ctx context.Context, q *queries.Queries, orderID uuid.UUID) error {
	res, err := q.ListReservationsByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	for _, rv := range res {
		if rv.Status != "held" {
			continue
		}
		if err := q.CommitReservedStock(ctx, queries.CommitReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
			return err
		}
	}
	_, err = q.SetReservationStatus(ctx, queries.SetReservationStatusParams{OrderID: orderID, NewStatus: "committed"})
	return err
}

func releaseHeld(ctx context.Context, q *queries.Queries, orderID uuid.UUID) error {
	res, err := q.ListReservationsByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	for _, rv := range res {
		if rv.Status != "held" {
			continue
		}
		if err := q.ReleaseReservedStock(ctx, queries.ReleaseReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
			return err
		}
	}
	_, err = q.SetReservationStatus(ctx, queries.SetReservationStatusParams{OrderID: orderID, NewStatus: "released"})
	return err
}

// reReserveSavepoint tries to reserve+commit all order items in a savepoint.
// Returns true if all succeeded (committed); false if any was short (savepoint
// rolled back, no stock mutated). C2.
func reReserveSavepoint(ctx context.Context, tx pgx.Tx, q *queries.Queries, orderID uuid.UUID) (bool, error) {
	items, err := q.ListOrderItems(ctx, orderID)
	if err != nil {
		return false, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].VariantID.String() < items[j].VariantID.String() })
	sp, err := tx.Begin(ctx) // savepoint
	if err != nil {
		return false, err
	}
	qsp := q.WithTx(sp)
	for _, it := range items {
		if _, err := qsp.ReserveStock(ctx, queries.ReserveStockParams{VariantID: it.VariantID, Qty: it.Quantity}); errors.Is(err, pgx.ErrNoRows) {
			_ = sp.Rollback(ctx)
			return false, nil
		} else if err != nil {
			_ = sp.Rollback(ctx)
			return false, err
		}
		if err := qsp.CommitReservedStock(ctx, queries.CommitReservedStockParams{VariantID: it.VariantID, Qty: it.Quantity}); err != nil {
			_ = sp.Rollback(ctx)
			return false, err
		}
	}
	if err := sp.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
```

(Add imports `github.com/google/uuid`; `ptr` helper. The `Reconciler` in `application/reconciler.go` is a thin wrapper holding `*ReconcileRepo` and exposing `Apply` to satisfy `payment.EventApplier` — or have `ReconcileRepo` satisfy it directly and expose it from the module. Confirm sqlc param names.)

- [ ] **Step 4: Run, PASS** (Docker; 5 cases). **Step 5: Commit** `feat(checkout): add payment reconciler with paid-after-expiry + idempotent webhook effect`.

---
## Task 20: `checkout/transport` + `checkout/module.go`

**Files:** Create `internal/modules/checkout/transport/{checkout_handlers.go,coupon_handler.go,responses.go,error_mapping.go}`, `internal/modules/checkout/module.go`; Test `checkout_handlers_test.go`

**Interfaces:** `POST /checkout/quote` (session+csrf), `POST /checkout/confirm` (session+csrf, `Idempotency-Key` header → 400 if missing), `POST /admin/coupons` (admin bearer). `checkout.New(Deps{Pool, Sessions, SessionCookie, CSRFCfg, AdminToken, Cart, Address, Shipping, Charger, Cfg}) *Module` with `Mount(chi.Router)` and `Reconciler() *infrastructure.ReconcileRepo` (for payment wiring). The Deps carry the other modules' services (as the port adapters from Task 18).

- [ ] **Step 1: Failing handler test** — fake CheckoutService: `/checkout/confirm` without `Idempotency-Key` → 400; quote happy → 200; confirm happy → 201; mapped errors per table below. **Step 2: FAIL.**
- [ ] **Step 3: error_mapping.go:**

```go
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrCartEmpty):
		return 422, "cart_empty", "cart is empty"
	case errors.Is(err, domain.ErrQuoteExpired), errors.Is(err, domain.ErrQuoteNotFound):
		return 409, "quote_expired", "quote expired — re-quote"
	case errors.Is(err, domain.ErrCartChanged):
		return 409, "cart_changed", "cart changed — re-quote"
	case errors.Is(err, invdomain.ErrInsufficientStock):
		return 422, "insufficient_stock", "insufficient stock"
	case errors.Is(err, domain.ErrCouponInvalid):
		return 422, "coupon_invalid", "coupon invalid"
	case errors.Is(err, domain.ErrCouponUnavailable):
		return 422, "coupon_unavailable", "coupon unavailable"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return 409, "idempotency_conflict", "idempotency key reused with a different request"
	case errors.Is(err, addrdomain.ErrAddressNotFound):
		return 404, "not_found", "address not found"
	default:
		return 500, "internal_error", "internal error"
	}
}
```

- [ ] **Step 4:** `responses.go` (QuoteResponse, ConfirmResponse, CouponResponse), `checkout_handlers.go` (read session user via `sessionauth.SessionFromContext`; quote decodes `{shipping_address_id, shipping_service_id?, coupon_code?}`; confirm reads `Idempotency-Key` header + `{quote_id}`), `coupon_handler.go` (admin create).
- [ ] **Step 5: module.go** — `New` builds CheckoutService (Quote+Confirm) + CouponService + ConfirmRepo + ReconcileRepo + the port adapters; `Mount`: session+csrf group for `/checkout/*`, admin-bearer group for `/admin/coupons`; `Reconciler()` accessor.
- [ ] **Step 6: Run, PASS, build. Step 7: Commit** `feat(checkout): add quote/confirm/admin-coupon endpoints + module`.

---

## Task 21: Wire all modules in `cmd/api/main.go`

**Files:** Modify `cmd/api/main.go`

**Interfaces:** provider factories + module wiring. Break the payment↔checkout cycle with a setter (proven pattern from Phase 2b identity↔cart): payment module built without applier; checkout built with `payment.ChargeService()` as Charger; then `paymentModule.SetApplier(checkoutModule.Reconciler())`.

- [ ] **Step 1:** Add provider factories:

```go
func newPaymentProvider(cfg config.Payment) payapplication.PaymentProvider {
	switch cfg.Provider {
	case "mock":
		return payinfra.NewMockProvider(cfg.WebhookSecret)
	default:
		panic(fmt.Sprintf("unsupported payment provider %q (pagarme lands in 3b)", cfg.Provider))
	}
}
func newShippingProvider(cfg config.Shipping) shipapplication.ShippingProvider {
	switch cfg.Provider {
	case "mock":
		return shipinfra.NewMockShipping()
	default:
		panic(fmt.Sprintf("unsupported shipping provider %q", cfg.Provider))
	}
}
```

- [ ] **Step 2:** Wire (order matters):

```go
inventoryModule := inventory.New(inventory.Deps{Pool: pool, AdminToken: cfg.Admin.APIToken})
orderingModule := ordering.New(ordering.Deps{Pool: pool, Sessions: sessions, SessionCookie: cookies.SessionName})
shippingModule := shipping.New(shipping.Deps{Provider: cfg.Shipping.Provider})
paymentProvider := newPaymentProvider(cfg.Payment)
paymentModule := payment.New(payment.Deps{Pool: pool, Provider: paymentProvider})

checkoutModule := checkout.New(checkout.Deps{
	Pool: pool, Sessions: sessions, SessionCookie: cookies.SessionName, CSRFCfg: csrfCfg,
	AdminToken: cfg.Admin.APIToken,
	Cart: cartModule.Service(), Address: addressModule.Service(),
	Shipping: shippingModule.Service(), Charger: paymentModule.ChargeService(),
	Cfg: cfg.Checkout,
})
paymentModule.SetApplier(checkoutModule.Reconciler()) // break the cycle

inventoryModule.Mount(router)
orderingModule.Mount(router)
paymentModule.Mount(router)
checkoutModule.Mount(router)
```

(`cartModule.Service()` / `addressModule.Service()` — add a `Service()` accessor to the cart and address modules if not present; both already hold their service. Small additive change.)

- [ ] **Step 3:** `go build ./... && go vet ./... && go test ./...` (existing suites green). **Step 4: Commit** `feat(checkout): wire inventory/ordering/shipping/payment/checkout in cmd/api`.

---

## Task 22: OpenAPI extension

**Files:** Modify `api/openapi.yaml`

- [ ] **Step 1:** Add tags `checkout`, `order`, `payment` and admin entries. **Step 2:** Add paths: `/checkout/quote`, `/checkout/confirm` (note `Idempotency-Key` header param), `/me/orders`, `/me/orders/{id}`, `/payments/webhook` (header `X-Webhook-Signature`), `/admin/variants/{id}/stock`, `/admin/coupons` — with the status codes from spec §4/§9. **Step 3:** Add schemas `Quote`, `QuoteOption`, `Order`, `OrderItem`, `Charge`, `Coupon`, `CouponInput`, `StockInput`. **Step 4:** Validate: `python3 -c "import yaml; yaml.safe_load(open('api/openapi.yaml')); print('ok')"`. **Step 5: Commit** `docs(api): extend OpenAPI with checkout/order/payment endpoints`.

---

## Task 23: E2E integration suite (§10 — the proof)

**Files:** Create `internal/testutil/payment.go` (signed mock webhook helper), `tests/integration/checkout_support_test.go`, `tests/integration/checkout_e2e_test.go`

Reuse the `integration_test` package helpers (`postIdentityJSON`, `registerVerifyLogin`, `fakeSender`, etc. — do NOT redeclare). `seedVariant`/`seedStock` insert a category first (FK).

- [ ] **Step 1:** `internal/testutil/payment.go` — `SignWebhook(secret string, body []byte) string` (= HMAC-SHA256 hex, same algo as the mock provider) so E2E can POST signed events.

- [ ] **Step 2:** `checkout_support_test.go` — `startCommerceFullAPI(t, ctx)` boots identity + cart + address + inventory + ordering + shipping + payment + checkout modules against testcontainers (mock providers; `MOCK_WEBHOOK_SECRET="test-secret"`), wiring the reconciler into payment exactly like `cmd/api` (Task 21). Returns `{srv, emails, pool, variantID, secret}` + a `seedStock(variantID, n)` helper.

- [ ] **Step 3:** `checkout_e2e_test.go` — the nine tests from spec §10. Each: arrange (login, set stock, add to cart, quote), act, assert. Write them in full following the Phase 2b E2E style. The critical ones (full code required):

```go
func TestCheckoutE2E_HappyPath(t *testing.T) {
	// signup→login→admin set stock→add to cart→quote→confirm→signed `paid` webhook
	// assert: confirm 201 + order pending_payment; after webhook → GET /me/orders/{id} == "paid";
	// stock available decremented by qty; reservation committed.
}
func TestCheckoutE2E_Oversell(t *testing.T) {
	// stock=1; two concurrent confirms (same product, separate users/carts) → exactly one 201, other 422 insufficient_stock.
}
func TestCheckoutE2E_PaidAfterExpiry_AwaitingStock(t *testing.T) {
	// confirm; expire reservation + run release job → order expired, stock freed and sold to a 2nd order;
	// then signed `paid` webhook for the 1st order → order == paid_awaiting_stock (never dropped).
}
func TestCheckoutE2E_WebhookForgeryRejected(t *testing.T) {
	// POST /payments/webhook with bad/no signature → 401; order unchanged.
}
func TestCheckoutE2E_WebhookAmountMismatch(t *testing.T) {
	// signed `paid` event with amount != order.total → 200 but order stays pending_payment.
}
func TestCheckoutE2E_Idempotent(t *testing.T) {
	// same Idempotency-Key twice → one order (second returns the same order_id); different body same key → 409.
}
func TestCheckoutE2E_QuoteStale(t *testing.T) {
	// confirm with an expired quote → 409 quote_expired; mutate cart after quote → 409 cart_changed.
}
func TestCheckoutE2E_CouponLimit(t *testing.T) {
	// coupon usage_limit=1; two confirms → second 422 coupon_unavailable; failed/expired order releases the redemption.
}
func TestCheckoutE2E_OrderIDOR(t *testing.T) {
	// user B GET /me/orders/{A's id} → 404.
}
```

Each test body is written in full in this task (mirror Phase 2b's `cart_e2e_test.go`/`address_e2e_test.go` request helpers). The webhook is posted to `/payments/webhook` with `X-Webhook-Signature: testutil.SignWebhook(secret, body)`.

- [ ] **Step 4:** Run the full suite (Docker): `go test -race -count=1 -tags=integration -timeout=15m ./...` — all PASS incl. the nine checkout E2E + no regressions. **Step 5: Commit** `test(integration): add checkout E2E suite (happy/oversell/paid-after-expiry/forgery/idempotent/coupon/IDOR)`.

---

## Task 24: README env docs + final validation

**Files:** Modify `README.md`

- [ ] **Step 1:** Add a "### Phase 3a — Checkout" env table: `PAYMENT_PROVIDER` (mock), `MOCK_WEBHOOK_SECRET` (required), `SHIPPING_PROVIDER` (mock), `CHECKOUT_QUOTE_TTL` (15m), `CHECKOUT_RESERVATION_TTL` (30m), `CHECKOUT_RELEASE_INTERVAL` (5m). Note the webhook is HMAC-signed and the reservation release job runs on the worker.
- [ ] **Step 2:** Full verification: `go build ./... && go vet ./... && go test ./...` then `go test -race -count=1 -tags=integration -timeout=15m ./...` — all green. (Controller's final gate before tag; do not tag until green.)
- [ ] **Step 3: Commit** `docs: document Phase 3a env vars`.
- [ ] **Step 4: Finish** via `superpowers:finishing-a-development-branch`: merge `--no-ff` into main, tag `v0.6.0-checkout`, push main + branch + tag.

---

## Self-Review

**1. Spec coverage** — every §-requirement maps to a task:

| Spec | Task(s) |
|---|---|
| inventory_stock + reserve/release/commit | 1,2,3,4 |
| admin set-stock | 5 |
| release job (paid-wins, C2) | 6 |
| ShippingProvider + mock | 7 |
| PaymentProvider + mock + HMAC (C1) | 8 |
| charge repo + signed webhook (C1) | 9 |
| Order + state machine incl paid_awaiting_stock (C2) | 10,11,12,13 |
| coupon + money math (I4) | 14,15 |
| quote (C3 lock) | 16 |
| confirm (C3/C4/I2/I3/I5 + cart-guard) | 17,18 |
| reconciler (C2/C5/I6) | 19 |
| endpoints | 5,9,13,20 |
| wiring | 21 |
| OpenAPI | 22 |
| E2E sad paths (§10) | 23 |
| config + README | 1,24 |

Money-integrity map: C1→8/9, C2→6/10/19, C3→16/17/18, C4→15/18, C5→9/19, I1→16/17, I2→4/18, I3→1/4, I4→14, I5→17/18, I6→4/19, I7→13.

**2. Placeholder scan:** Tasks 5, 11-13, 20, 22 use "mirror module X" pointers to **shipped** modules (cart/address/identity) rather than re-pasting their boilerplate — the implementer reads the real code. The money-critical and novel code (Tasks 1,4,6,8,9,14,16,17,18,19) is spelled in full. Task 23's nine E2E bodies are described arrange/act/assert with the critical mechanics; the implementer writes them mirroring Phase 2b's E2E files. This is a deliberate altitude choice for an already-large plan; flagged here so a reviewer can demand full expansion of any task if desired.

**3. Type consistency:** `OrderStatus` strings match the DB CHECK in Task 1 and §6. `domain.Event` (payment) is the single type crossing payment→checkout reconciler. `ReserveItem`/`QuoteLine` carry `VariantID uuid.UUID`+`Quantity int`. sqlc param struct names (`ReserveStockParams.Qty`, `CreateOrderParams.ID`, `SetReservationStatusParams.NewStatus`, etc.) are flagged for post-`make sqlc-gen` confirmation in Tasks 1/4/18/19 — the single biggest execution risk is a generated-name mismatch; reconcile against the real generated code before writing each repo.

**Open execution risks to watch:** (a) cross-module tx correctness (Tasks 18/19) — the integration tests are the gate; (b) the payment↔checkout setter wiring (Task 21); (c) sqlc nullable/pointer types for `coupon_code`/`from_status`/`order_id`/`response`.

