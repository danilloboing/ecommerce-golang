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
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/jobs"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func TestRunReleaseExpiredOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{
		URL:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Shared FK dependencies: user, category, product, variant, stock.
	userID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, email, name, status)
		VALUES ($1, $2, 'Test User', 'active')`,
		userID, "testrel@example.com")
	require.NoError(t, err)

	catID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO catalog_categories (id, slug, name)
		VALUES ($1, 'cat-rel', 'Category Rel')`,
		catID)
	require.NoError(t, err)

	prodID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO catalog_products (id, category_id, slug, name, description, base_price_cents, currency, status)
		VALUES ($1, $2, 'prod-rel', 'Prod Rel', 'desc', 1000, 'BRL', 'published')`,
		prodID, catID)
	require.NoError(t, err)

	variantID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO catalog_variants (id, product_id, sku, size, color)
		VALUES ($1, $2, 'SKU-REL-1', 'M', 'red')`,
		variantID, prodID)
	require.NoError(t, err)

	// Stock: 0 available, 2 reserved (for both orders).
	_, err = pool.Exec(ctx, `
		INSERT INTO inventory_stock (variant_id, available, reserved)
		VALUES ($1, 0, 2)`,
		variantID)
	require.NoError(t, err)

	addrSnap := []byte(`{}`)
	shipSnap := []byte(`{}`)

	// ----- Order A: pending_payment + expired reservation + NO paid charge -----
	orderAID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO orders (id, user_id, status, subtotal_cents, shipping_cents, discount_cents,
		                    total_cents, address_snapshot, shipping_snapshot)
		VALUES ($1, $2, 'pending_payment', 1000, 0, 0, 1000, $3, $4)`,
		orderAID, userID, addrSnap, shipSnap)
	require.NoError(t, err)

	// Expired reservation (expires 5 minutes ago).
	_, err = pool.Exec(ctx, `
		INSERT INTO stock_reservations (order_id, variant_id, quantity, status, expires_at)
		VALUES ($1, $2, 1, 'held', now() - interval '5 minutes')`,
		orderAID, variantID)
	require.NoError(t, err)

	// ----- Order B: pending_payment + expired reservation + PAID charge (paid wins) -----
	orderBID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO orders (id, user_id, status, subtotal_cents, shipping_cents, discount_cents,
		                    total_cents, address_snapshot, shipping_snapshot)
		VALUES ($1, $2, 'pending_payment', 1000, 0, 0, 1000, $3, $4)`,
		orderBID, userID, addrSnap, shipSnap)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO stock_reservations (order_id, variant_id, quantity, status, expires_at)
		VALUES ($1, $2, 1, 'held', now() - interval '5 minutes')`,
		orderBID, variantID)
	require.NoError(t, err)

	// Insert a paid charge for order B.
	_, err = pool.Exec(ctx, `
		INSERT INTO charges (order_id, provider, provider_charge_id, method, status, amount_cents, raw_payload)
		VALUES ($1, 'stripe', 'ch_test_paid', 'card', 'paid', 1000, '{}')`,
		orderBID)
	require.NoError(t, err)

	// Run the job.
	n, err := jobs.RunReleaseExpiredOnce(ctx, pool, time.Now())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "only order A should be expired")

	q := queries.New(pool)

	// Order A must be expired.
	orderA, err := q.GetOrderByID(ctx, orderAID)
	require.NoError(t, err)
	assert.Equal(t, "expired", orderA.Status, "order A should be expired")

	// Order A reservations must be released.
	resA, err := q.ListReservationsByOrder(ctx, orderAID)
	require.NoError(t, err)
	for _, r := range resA {
		assert.Equal(t, "released", r.Status, "order A reservation should be released")
	}

	// Order B must remain pending_payment (paid wins).
	orderB, err := q.GetOrderByID(ctx, orderBID)
	require.NoError(t, err)
	assert.Equal(t, "pending_payment", orderB.Status, "order B should remain pending_payment")

	// Stock for order A's release: reserved should have decreased by 1.
	stock, err := q.GetStock(ctx, variantID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), stock.Reserved, "reserved should decrease by 1 after order A release")
	assert.Equal(t, int32(1), stock.Available, "available should increase by 1 after order A release")
}
