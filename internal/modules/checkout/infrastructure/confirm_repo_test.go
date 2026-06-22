//go:build integration

package infrastructure_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

// confirmFixtures holds the ids and the persisted quote a confirm test needs.
type confirmFixtures struct {
	userID    uuid.UUID
	variantID uuid.UUID
	quote     domain.Quote
}

// newConfirmEnv spins up Postgres, applies migrations, and returns a live pool.
func newConfirmEnv(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{
		URL: dsn, MaxOpenConns: 8, MaxIdleConns: 2, ConnMaxLifetime: 30 * time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// seedCatalogAndStock inserts category → product → variant (price 9900) plus an
// inventory_stock row with the given available quantity (reserved 0).
func seedCatalogAndStock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, available int) (productID, variantID uuid.UUID) {
	t.Helper()
	categoryID := uuid.New()
	productID = uuid.New()
	variantID = uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, 'Cat')`,
		categoryID, "cat-"+categoryID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_products
		(id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		VALUES ($1, $2, 'P', 'D', 'B', $3, 5000, 'BRL', 'published')`,
		productID, "slug-"+productID.String(), categoryID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		VALUES ($1, $2, $3, 'M', 'Red', 9900)`,
		variantID, productID, "sku-"+variantID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO inventory_stock (variant_id, available, reserved, version)
		VALUES ($1, $2, 0, 0)`, variantID, available)
	require.NoError(t, err)

	return productID, variantID
}

// seedUserAndCart inserts a user and an active cart owned by that user.
func seedUserAndCart(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	userID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test')`,
		userID, "u-"+userID.String()+"@test.local")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO carts (id, user_id, status) VALUES ($1, $2, 'active')`,
		uuid.New(), userID)
	require.NoError(t, err)

	return userID
}

// seedQuote persists a checkout_quotes row through the real QuoteRepository so
// the confirm under test reads exactly what production would have written.
func seedQuote(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, variantID uuid.UUID, couponCode string) domain.Quote {
	t.Helper()
	repo := infrastructure.NewQuoteRepo(pool)
	quote, err := repo.Create(ctx, application.NewQuote{
		UserID:          userID,
		CartFingerprint: "fp-" + uuid.New().String(),
		Lines: []domain.QuoteLine{{
			VariantID:       variantID,
			Quantity:        2,
			UnitPriceCents:  9900,
			ProductSnapshot: json.RawMessage(`{}`),
		}},
		Chosen:     application.ShippingOption{ServiceID: "sedex", Name: "Sedex", PriceCents: 1000, ETADays: 3},
		CouponCode: couponCode,
		Subtotal:   19800,
		Shipping:   1000,
		Discount:   0,
		Total:      20800,
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	return quote
}

// makePlan assembles a ConfirmPlan from a persisted quote, mirroring what the
// service builds after a successful idempotency pre-check and charge mint.
func makePlan(quote domain.Quote, userID uuid.UUID, idemKey, requestHash string) application.ConfirmPlan {
	orderID := uuid.New()
	return application.ConfirmPlan{
		UserID:  userID,
		OrderID: orderID,
		Quote:   quote,
		Charge: application.ChargeView{
			ChargeID: uuid.New(),
			OrderID:  orderID,
			Amount:   quote.Total,
			Method:   "pix",
			Status:   "pending",
		},
		IdempotencyKey:       idemKey,
		RequestHash:          requestHash,
		ReservationExpiresAt: time.Now().Add(15 * time.Minute),
	}
}

// TestConfirmTx_HappyPath confirms a valid quote and asserts every side effect
// of the §5 transaction landed: order, reservation, cart conversion, charge,
// idempotency row (decodable as ConfirmResult), and status transition.
func TestConfirmTx_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newConfirmEnv(t, ctx)
	_, variantID := seedCatalogAndStock(t, ctx, pool, 5)
	userID := seedUserAndCart(t, ctx, pool)
	quote := seedQuote(t, ctx, pool, userID, variantID, "")

	repo := infrastructure.NewConfirmRepo(pool)
	plan := makePlan(quote, userID, "key-happy", "hash-happy")

	order, err := repo.ConfirmTx(ctx, plan)
	require.NoError(t, err)
	assert.Equal(t, plan.OrderID, order.ID)
	assert.EqualValues(t, "pending_payment", order.Status)

	// orders row exists with pending_payment status.
	var orderStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, plan.OrderID).Scan(&orderStatus))
	assert.Equal(t, "pending_payment", orderStatus)

	// stock_reservations row exists for the order/variant, status 'held'.
	var resvStatus string
	var resvQty int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status, quantity FROM stock_reservations WHERE order_id = $1 AND variant_id = $2`,
		plan.OrderID, variantID).Scan(&resvStatus, &resvQty))
	assert.Equal(t, "held", resvStatus)
	assert.Equal(t, 2, resvQty)

	// stock decremented: available 5 → 3, reserved 0 → 2.
	var available, reserved int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT available, reserved FROM inventory_stock WHERE variant_id = $1`, variantID).Scan(&available, &reserved))
	assert.Equal(t, 3, available)
	assert.Equal(t, 2, reserved)

	// cart converted.
	var cartStatus string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM carts WHERE user_id = $1`, userID).Scan(&cartStatus))
	assert.Equal(t, "converted", cartStatus)

	// charge row exists, status pending, mock provider.
	var chargeStatus, provider string
	var chargeAmount int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status, provider, amount_cents FROM charges WHERE order_id = $1`,
		plan.OrderID).Scan(&chargeStatus, &provider, &chargeAmount))
	assert.Equal(t, "pending", chargeStatus)
	assert.Equal(t, "mock", provider)
	assert.Equal(t, int64(20800), chargeAmount)

	// idempotency row exists and decodes to the full ConfirmResult.
	var response []byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT response FROM idempotency_keys WHERE user_id = $1 AND key = $2`,
		userID, "key-happy").Scan(&response))
	var stored application.ConfirmResult
	require.NoError(t, json.Unmarshal(response, &stored))
	assert.Equal(t, plan.OrderID, stored.Order.ID)
	assert.Equal(t, plan.Charge.ChargeID, stored.Charge.ChargeID)

	// status transition recorded.
	var toStatus string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT to_status FROM order_status_transitions WHERE order_id = $1`,
		plan.OrderID).Scan(&toStatus))
	assert.Equal(t, "pending_payment", toStatus)
}

// TestConfirmTx_OversellRollback asserts that reserving more than is available
// fails with ErrInsufficientStock and rolls the whole transaction back.
func TestConfirmTx_OversellRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newConfirmEnv(t, ctx)
	_, variantID := seedCatalogAndStock(t, ctx, pool, 5)
	userID := seedUserAndCart(t, ctx, pool)

	// A quote that wants 10 units while only 5 are in stock.
	repo := infrastructure.NewQuoteRepo(pool)
	quote, err := repo.Create(ctx, application.NewQuote{
		UserID:          userID,
		CartFingerprint: "fp-oversell",
		Lines: []domain.QuoteLine{{
			VariantID:       variantID,
			Quantity:        10,
			UnitPriceCents:  9900,
			ProductSnapshot: json.RawMessage(`{}`),
		}},
		Chosen:    application.ShippingOption{ServiceID: "sedex", Name: "Sedex", PriceCents: 1000, ETADays: 3},
		Subtotal:  99000,
		Shipping:  1000,
		Total:     100000,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	confirmRepo := infrastructure.NewConfirmRepo(pool)
	plan := makePlan(quote, userID, "key-oversell", "hash-oversell")

	_, err = confirmRepo.ConfirmTx(ctx, plan)
	require.ErrorIs(t, err, domain.ErrInsufficientStock)

	// Nothing persisted: no order row.
	var orderCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE id = $1`, plan.OrderID).Scan(&orderCount))
	assert.Equal(t, 0, orderCount)

	// Stock untouched.
	var available, reserved int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT available, reserved FROM inventory_stock WHERE variant_id = $1`, variantID).Scan(&available, &reserved))
	assert.Equal(t, 5, available)
	assert.Equal(t, 0, reserved)

	// Cart still active.
	var cartStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM carts WHERE user_id = $1`, userID).Scan(&cartStatus))
	assert.Equal(t, "active", cartStatus)
}

// TestConfirmTx_CouponUnavailable asserts that redeeming a coupon already at its
// usage limit fails with ErrCouponUnavailable and rolls the transaction back.
func TestConfirmTx_CouponUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newConfirmEnv(t, ctx)
	_, variantID := seedCatalogAndStock(t, ctx, pool, 5)
	userID := seedUserAndCart(t, ctx, pool)

	// Coupon already exhausted: usage_limit 1, used_count 1.
	couponCode := "SAVE10"
	_, err := pool.Exec(ctx,
		`INSERT INTO coupons (id, code, type, value, usage_limit, used_count, active)
		 VALUES ($1, $2, 'fixed', 1000, 1, 1, TRUE)`,
		uuid.New(), couponCode)
	require.NoError(t, err)

	quote := seedQuote(t, ctx, pool, userID, variantID, couponCode)

	confirmRepo := infrastructure.NewConfirmRepo(pool)
	plan := makePlan(quote, userID, "key-coupon", "hash-coupon")

	_, err = confirmRepo.ConfirmTx(ctx, plan)
	require.ErrorIs(t, err, domain.ErrCouponUnavailable)

	// Nothing persisted: no order row.
	var orderCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE id = $1`, plan.OrderID).Scan(&orderCount))
	assert.Equal(t, 0, orderCount)

	// Coupon used_count unchanged (rolled back).
	var usedCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT used_count FROM coupons WHERE code = $1`, couponCode).Scan(&usedCount))
	assert.Equal(t, 1, usedCount)

	// Cart still active.
	var cartStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM carts WHERE user_id = $1`, userID).Scan(&cartStatus))
	assert.Equal(t, "active", cartStatus)
}

// TestConfirmTx_IdempotencyConflict asserts that a second confirm reusing the
// same (userID, key) with a different request hash returns ErrIdempotencyConflict.
func TestConfirmTx_IdempotencyConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newConfirmEnv(t, ctx)
	_, variantID := seedCatalogAndStock(t, ctx, pool, 10)
	userID := seedUserAndCart(t, ctx, pool)
	confirmRepo := infrastructure.NewConfirmRepo(pool)

	// First confirm succeeds with its own quote.
	quote1 := seedQuote(t, ctx, pool, userID, variantID, "")
	plan1 := makePlan(quote1, userID, "key-shared", "hash-first")
	_, err := confirmRepo.ConfirmTx(ctx, plan1)
	require.NoError(t, err)

	// A new active cart so the cart-guard passes for the second attempt; the
	// idempotency-key conflict is what must trip, not the cart guard.
	_, err = pool.Exec(ctx, `INSERT INTO carts (id, user_id, status) VALUES ($1, $2, 'active')`, uuid.New(), userID)
	require.NoError(t, err)

	// Second confirm: same (userID, key) but a DIFFERENT request hash.
	quote2 := seedQuote(t, ctx, pool, userID, variantID, "")
	plan2 := makePlan(quote2, userID, "key-shared", "hash-second")
	_, err = confirmRepo.ConfirmTx(ctx, plan2)
	require.ErrorIs(t, err, domain.ErrIdempotencyConflict)

	// The second order was rolled back.
	var orderCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE id = $1`, plan2.OrderID).Scan(&orderCount))
	assert.Equal(t, 0, orderCount)
}
