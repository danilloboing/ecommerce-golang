//go:build integration

package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/infrastructure"
	paymentdomain "github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

// reconcileFixture holds the data planted for one reconcile test case.
type reconcileFixture struct {
	userID    uuid.UUID
	variantID uuid.UUID
	orderID   uuid.UUID
	chargeID  uuid.UUID
	total     int64
}

// newReconcileEnv reuses newConfirmEnv (same test binary) to get a live pool.
func newReconcileEnv(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	return newConfirmEnv(t, ctx)
}

// seedPendingOrder inserts category→product→variant→stock, a user, an order in
// pending_payment status, a single held reservation, and a pending charge.
// available = initial available qty; reserved = the qty reserved by the order.
// couponCode may be nil.
func seedPendingOrder(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	available int32, reserved int32,
	couponCode *string,
) reconcileFixture {
	t.Helper()

	categoryID := uuid.New()
	productID := uuid.New()
	variantID := uuid.New()
	userID := uuid.New()
	orderID := uuid.New()
	chargeID := uuid.New()
	const total int64 = 9900

	_, err := pool.Exec(ctx,
		`INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, 'Cat')`,
		categoryID, "cat-"+categoryID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_products
		 (id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		 VALUES ($1, $2, 'P', 'D', 'B', $3, 9900, 'BRL', 'published')`,
		productID, "slug-"+productID.String(), categoryID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		 VALUES ($1, $2, $3, 'M', 'Red', 9900)`,
		variantID, productID, "sku-"+variantID.String())
	require.NoError(t, err)

	// available + reserved should equal total stock at rest; after reservation
	// we store them independently.
	_, err = pool.Exec(ctx,
		`INSERT INTO inventory_stock (variant_id, available, reserved, version)
		 VALUES ($1, $2, $3, 0)`,
		variantID, available, reserved)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test')`,
		userID, "u-"+userID.String()+"@test.local")
	require.NoError(t, err)

	var couponArg interface{}
	if couponCode != nil {
		couponArg = *couponCode
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO orders
		 (id, user_id, status, subtotal_cents, shipping_cents, discount_cents, total_cents,
		  coupon_code, address_snapshot, shipping_snapshot)
		 VALUES ($1, $2, 'pending_payment', $3, 0, 0, $3,
		         $4, '{}', '{}')`,
		orderID, userID, total, couponArg)
	require.NoError(t, err)

	// Seed the initial status transition.
	_, err = pool.Exec(ctx,
		`INSERT INTO order_status_transitions (order_id, from_status, to_status, reason, actor)
		 VALUES ($1, NULL, 'pending_payment', 'checkout_confirmed', 'system')`,
		orderID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO order_items (order_id, variant_id, quantity, unit_price_cents, product_snapshot)
		 VALUES ($1, $2, 1, $3, '{}')`,
		orderID, variantID, total)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO stock_reservations (id, order_id, variant_id, quantity, status, expires_at)
		 VALUES ($1, $2, $3, $4, 'held', $5)`,
		uuid.New(), orderID, variantID, reserved, time.Now().Add(15*time.Minute))
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO charges (id, order_id, provider, provider_charge_id, method, status, amount_cents)
		 VALUES ($1, $2, 'mock', $3, 'pix', 'pending', $4)`,
		chargeID, orderID, "mock_"+orderID.String(), total)
	require.NoError(t, err)

	return reconcileFixture{
		userID:    userID,
		variantID: variantID,
		orderID:   orderID,
		chargeID:  chargeID,
		total:     total,
	}
}

// seedExpiredOrder seeds an order that has already been expired:
//   - order status = 'expired'
//   - reservations status = 'released' (stock released back)
//   - stock is fully available (reserved=0, available=total)
func seedExpiredOrder(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	stockAvailable int32,
) reconcileFixture {
	t.Helper()

	categoryID := uuid.New()
	productID := uuid.New()
	variantID := uuid.New()
	userID := uuid.New()
	orderID := uuid.New()
	chargeID := uuid.New()
	const total int64 = 9900

	_, err := pool.Exec(ctx,
		`INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, 'Cat')`,
		categoryID, "cat-"+categoryID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_products
		 (id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		 VALUES ($1, $2, 'P', 'D', 'B', $3, 9900, 'BRL', 'published')`,
		productID, "slug-"+productID.String(), categoryID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		 VALUES ($1, $2, $3, 'M', 'Red', 9900)`,
		variantID, productID, "sku-"+variantID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO inventory_stock (variant_id, available, reserved, version)
		 VALUES ($1, $2, 0, 0)`,
		variantID, stockAvailable)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test')`,
		userID, "u-"+userID.String()+"@test.local")
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO orders
		 (id, user_id, status, subtotal_cents, shipping_cents, discount_cents, total_cents,
		  coupon_code, address_snapshot, shipping_snapshot)
		 VALUES ($1, $2, 'expired', $3, 0, 0, $3, NULL, '{}', '{}')`,
		orderID, userID, total)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO order_items (order_id, variant_id, quantity, unit_price_cents, product_snapshot)
		 VALUES ($1, $2, 1, $3, '{}')`,
		orderID, variantID, total)
	require.NoError(t, err)

	// Reservation was released by the expiry job.
	_, err = pool.Exec(ctx,
		`INSERT INTO stock_reservations (id, order_id, variant_id, quantity, status, expires_at)
		 VALUES ($1, $2, $3, 1, 'released', $4)`,
		uuid.New(), orderID, variantID, time.Now().Add(-time.Hour))
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO charges (id, order_id, provider, provider_charge_id, method, status, amount_cents)
		 VALUES ($1, $2, 'mock', $3, 'pix', 'pending', $4)`,
		chargeID, orderID, "mock_"+orderID.String(), total)
	require.NoError(t, err)

	return reconcileFixture{
		userID:    userID,
		variantID: variantID,
		orderID:   orderID,
		chargeID:  chargeID,
		total:     total,
	}
}

// makeEvent constructs a payment domain Event for testing.
func makeEvent(id, eventType string, orderID uuid.UUID, amount int64) paymentdomain.Event {
	return paymentdomain.Event{
		ID:               id,
		Type:             eventType,
		ProviderChargeID: "mock_" + orderID.String(),
		AmountCents:      amount,
	}
}

// queryOrderStatus returns the current order status from the DB.
func queryOrderStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status))
	return status
}

// queryChargeStatus returns the current charge status.
func queryChargeStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM charges WHERE order_id = $1`, orderID).Scan(&status))
	return status
}

// queryStock returns (available, reserved) for the variant.
func queryStock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, variantID uuid.UUID) (available, reserved int32) {
	t.Helper()
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT available, reserved FROM inventory_stock WHERE variant_id = $1`, variantID).
		Scan(&available, &reserved))
	return
}

// queryReservationStatus returns the reservation status for the order.
func queryReservationStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM stock_reservations WHERE order_id = $1 LIMIT 1`, orderID).Scan(&status))
	return status
}

// queryTransitionCount returns how many transition rows exist for the order.
func queryTransitionCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID uuid.UUID) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM order_status_transitions WHERE order_id = $1`, orderID).Scan(&count))
	return count
}

// queryWebhookEventCount returns how many webhook_event rows exist for the event_id.
func queryWebhookEventCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM payment_webhook_events WHERE event_id = $1`, eventID).Scan(&count))
	return count
}

// TestReconcileRepo_PaidOnPending verifies that a "paid" event on a
// pending_payment order transitions it to paid, commits the reservation, and
// decrements reserved stock while keeping available unchanged.
func TestReconcileRepo_PaidOnPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)
	fx := seedPendingOrder(t, ctx, pool, 0, 1, nil)

	repo := infrastructure.NewReconcileRepo(pool)
	ev := makeEvent("evt-paid-1", "paid", fx.orderID, fx.total)

	require.NoError(t, repo.Apply(ctx, ev))

	assert.Equal(t, "paid", queryOrderStatus(t, ctx, pool, fx.orderID))
	assert.Equal(t, "paid", queryChargeStatus(t, ctx, pool, fx.orderID))
	assert.Equal(t, "committed", queryReservationStatus(t, ctx, pool, fx.orderID))

	// CommitReservedStock: reserved goes from 1→0 (available stays at 0 — sold).
	avail, resv := queryStock(t, ctx, pool, fx.variantID)
	assert.EqualValues(t, 0, avail, "available must not change on commit")
	assert.EqualValues(t, 0, resv, "reserved must decrement to 0")

	// Transition recorded (initial + webhook = 2).
	assert.Equal(t, 2, queryTransitionCount(t, ctx, pool, fx.orderID))
	// Webhook event deduplication row inserted.
	assert.Equal(t, 1, queryWebhookEventCount(t, ctx, pool, ev.ID))
}

// TestReconcileRepo_DuplicateEvent verifies that applying the same event_id
// twice is a no-op: the order stays paid and stock is not double-committed.
func TestReconcileRepo_DuplicateEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)
	fx := seedPendingOrder(t, ctx, pool, 0, 1, nil)

	repo := infrastructure.NewReconcileRepo(pool)
	ev := makeEvent("evt-dup-1", "paid", fx.orderID, fx.total)

	// First apply succeeds.
	require.NoError(t, repo.Apply(ctx, ev))
	assert.Equal(t, "paid", queryOrderStatus(t, ctx, pool, fx.orderID))

	_, resv1 := queryStock(t, ctx, pool, fx.variantID)

	// Second apply with same event_id: must be a no-op.
	require.NoError(t, repo.Apply(ctx, ev))

	assert.Equal(t, "paid", queryOrderStatus(t, ctx, pool, fx.orderID))
	_, resv2 := queryStock(t, ctx, pool, fx.variantID)
	assert.Equal(t, resv1, resv2, "reserved must not change on duplicate apply")

	// Still only one webhook event row.
	assert.Equal(t, 1, queryWebhookEventCount(t, ctx, pool, ev.ID))
}

// TestReconcileRepo_AmountMismatch verifies that when the event amount does not
// match the order total the order stays in pending_payment.
func TestReconcileRepo_AmountMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)
	fx := seedPendingOrder(t, ctx, pool, 0, 1, nil)

	repo := infrastructure.NewReconcileRepo(pool)
	// Mismatched amount: order total is fx.total but event says +1.
	ev := makeEvent("evt-mismatch-1", "paid", fx.orderID, fx.total+1)

	require.NoError(t, repo.Apply(ctx, ev))

	// Order must stay in pending_payment.
	assert.Equal(t, "pending_payment", queryOrderStatus(t, ctx, pool, fx.orderID))
	// Charge must still be pending.
	assert.Equal(t, "pending", queryChargeStatus(t, ctx, pool, fx.orderID))

	// Webhook event row was recorded (anti-replay) even though nothing was applied.
	assert.Equal(t, 1, queryWebhookEventCount(t, ctx, pool, ev.ID))
}

// TestReconcileRepo_Failed verifies that a "failed" event on a pending_payment
// order transitions it to payment_failed, releases reservations, restores stock,
// and releases the coupon.
func TestReconcileRepo_Failed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)

	// Insert a coupon with used_count=1 (it was redeemed at confirm time).
	couponCode := "SAVE10-FAIL"
	_, err := pool.Exec(ctx,
		`INSERT INTO coupons (id, code, type, value, usage_limit, used_count, active)
		 VALUES ($1, $2, 'fixed', 1000, 10, 1, TRUE)`,
		uuid.New(), couponCode)
	require.NoError(t, err)

	fx := seedPendingOrder(t, ctx, pool, 0, 1, &couponCode)

	repo := infrastructure.NewReconcileRepo(pool)
	ev := makeEvent("evt-fail-1", "failed", fx.orderID, fx.total)

	require.NoError(t, repo.Apply(ctx, ev))

	assert.Equal(t, "payment_failed", queryOrderStatus(t, ctx, pool, fx.orderID))
	assert.Equal(t, "failed", queryChargeStatus(t, ctx, pool, fx.orderID))
	assert.Equal(t, "released", queryReservationStatus(t, ctx, pool, fx.orderID))

	// ReleaseReservedStock: available goes up, reserved goes down.
	avail, resv := queryStock(t, ctx, pool, fx.variantID)
	assert.EqualValues(t, 1, avail, "available must be restored")
	assert.EqualValues(t, 0, resv, "reserved must be 0 after release")

	// Coupon used_count must be decremented back.
	var usedCount int32
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT used_count FROM coupons WHERE code = $1`, couponCode).Scan(&usedCount))
	assert.EqualValues(t, 0, usedCount, "coupon used_count must be released")

	assert.Equal(t, 2, queryTransitionCount(t, ctx, pool, fx.orderID))
	assert.Equal(t, 1, queryWebhookEventCount(t, ctx, pool, ev.ID))
}

// TestReconcileRepo_PaidAfterExpiry_StockAvailable verifies that when a "paid"
// event arrives for an expired order and stock is available the order becomes
// paid (C2).
func TestReconcileRepo_PaidAfterExpiry_StockAvailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)
	// stockAvailable=5 so re-reserve of qty=1 succeeds.
	fx := seedExpiredOrder(t, ctx, pool, 5)

	repo := infrastructure.NewReconcileRepo(pool)
	ev := makeEvent("evt-expiry-ok", "paid", fx.orderID, fx.total)

	require.NoError(t, repo.Apply(ctx, ev))

	assert.Equal(t, "paid", queryOrderStatus(t, ctx, pool, fx.orderID))
	assert.Equal(t, "paid", queryChargeStatus(t, ctx, pool, fx.orderID))

	// Stock: reserved 1 unit, then immediately committed → available reduced by 1.
	avail, resv := queryStock(t, ctx, pool, fx.variantID)
	assert.EqualValues(t, 4, avail, "available decremented by committed qty")
	assert.EqualValues(t, 0, resv, "reserved is 0 after immediate commit")

	assert.Equal(t, 1, queryWebhookEventCount(t, ctx, pool, ev.ID))
}

// TestReconcileRepo_PaidAfterExpiry_StockGone verifies that when a "paid" event
// arrives for an expired order but stock is exhausted the order becomes
// paid_awaiting_stock (C2 — never drop the payment).
func TestReconcileRepo_PaidAfterExpiry_StockGone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)
	// stockAvailable=0 so re-reserve of qty=1 fails.
	fx := seedExpiredOrder(t, ctx, pool, 0)

	repo := infrastructure.NewReconcileRepo(pool)
	ev := makeEvent("evt-expiry-nostock", "paid", fx.orderID, fx.total)

	require.NoError(t, repo.Apply(ctx, ev))

	assert.Equal(t, "paid_awaiting_stock", queryOrderStatus(t, ctx, pool, fx.orderID))
	assert.Equal(t, "paid", queryChargeStatus(t, ctx, pool, fx.orderID))

	// Stock should be untouched (savepoint rolled back).
	avail, resv := queryStock(t, ctx, pool, fx.variantID)
	assert.EqualValues(t, 0, avail, "available must be unchanged")
	assert.EqualValues(t, 0, resv, "reserved must be unchanged")

	assert.Equal(t, 1, queryWebhookEventCount(t, ctx, pool, ev.ID))
}

// TestReconcileRepo_ForwardOnly verifies that applying a "paid" event to an
// already-paid order is a no-op (forward-only constraint).
func TestReconcileRepo_ForwardOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newReconcileEnv(t, ctx)
	fx := seedPendingOrder(t, ctx, pool, 0, 1, nil)

	repo := infrastructure.NewReconcileRepo(pool)
	ev1 := makeEvent("evt-fwd-1", "paid", fx.orderID, fx.total)
	ev2 := makeEvent("evt-fwd-2", "paid", fx.orderID, fx.total)

	require.NoError(t, repo.Apply(ctx, ev1))
	assert.Equal(t, "paid", queryOrderStatus(t, ctx, pool, fx.orderID))

	// Second distinct event on an already-paid order must be a no-op.
	require.NoError(t, repo.Apply(ctx, ev2))
	assert.Equal(t, "paid", queryOrderStatus(t, ctx, pool, fx.orderID))

	// Only 2 transitions: initial + first webhook.
	assert.Equal(t, 2, queryTransitionCount(t, ctx, pool, fx.orderID))
}

