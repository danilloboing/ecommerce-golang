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
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

type seedIDs struct {
	userID    uuid.UUID
	variantID uuid.UUID
}

func newOrderRepo(t *testing.T, ctx context.Context) (*infrastructure.Repository, *seedIDs) {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ids := seedOrderFixtures(t, ctx, pool)
	return infrastructure.New(pool), ids
}

// seedOrderFixtures inserts the minimal rows order tests depend on.
func seedOrderFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *seedIDs {
	t.Helper()
	ids := &seedIDs{userID: uuid.New(), variantID: uuid.New()}
	categoryID := uuid.New()
	productID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test')`,
		ids.userID, "u-"+ids.userID.String()+"@test.local",
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, 'Cat')`,
		categoryID, "cat-"+categoryID.String(),
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_products
			(id, slug, name, description, brand, category_id, base_price_cents, currency, status)
			VALUES ($1, $2, 'P', 'D', 'B', $3, 5000, 'BRL', 'published')`,
		productID, "slug-"+productID.String(), categoryID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
			VALUES ($1, $2, $3, 'M', 'Red', 9900)`,
		ids.variantID, productID, "sku-"+ids.variantID.String(),
	)
	require.NoError(t, err)

	return ids
}

func newOrderInput(userID uuid.UUID) application.NewOrder {
	return application.NewOrder{
		UserID:           userID,
		Subtotal:         9900,
		Shipping:         1500,
		Discount:         0,
		Total:            11400,
		CouponCode:       nil,
		AddressSnapshot:  json.RawMessage(`{"street":"Av Paulista","city":"SP"}`),
		ShippingSnapshot: json.RawMessage(`{"method":"PAC","days":5}`),
	}
}

func newItemInput(variantID uuid.UUID) application.NewOrderItem {
	return application.NewOrderItem{
		VariantID:       variantID,
		Quantity:        2,
		UnitPriceCents:  9900,
		ProductSnapshot: json.RawMessage(`{"name":"Shirt","sku":"SKU-01"}`),
	}
}

func TestOrderRepository_Create(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newOrderRepo(t, ctx)

	order, err := repo.Create(ctx, newOrderInput(ids.userID))
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, order.ID)
	assert.Equal(t, ids.userID, order.UserID)
	assert.Equal(t, domain.PendingPayment, order.Status)
	assert.Equal(t, int64(9900), order.Subtotal)
	assert.Equal(t, int64(11400), order.Total)
	assert.NotNil(t, order.AddressSnapshot)
	assert.NotNil(t, order.ShippingSnapshot)
}

func TestOrderRepository_GetByID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newOrderRepo(t, ctx)

	created, err := repo.Create(ctx, newOrderInput(ids.userID))
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, ids.userID, got.UserID)

	_, err = repo.GetByID(ctx, uuid.New())
	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestOrderRepository_GetUserOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newOrderRepo(t, ctx)

	created, err := repo.Create(ctx, newOrderInput(ids.userID))
	require.NoError(t, err)

	got, err := repo.GetUserOrder(ctx, created.ID, ids.userID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	// Cross-user access must return ErrOrderNotFound.
	_, err = repo.GetUserOrder(ctx, created.ID, uuid.New())
	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestOrderRepository_ListByUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newOrderRepo(t, ctx)

	first, err := repo.Create(ctx, newOrderInput(ids.userID))
	require.NoError(t, err)
	second, err := repo.Create(ctx, newOrderInput(ids.userID))
	require.NoError(t, err)

	list, err := repo.ListByUser(ctx, ids.userID, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)
	// ListByUser returns most-recent first (DESC by created_at).
	assert.Equal(t, second.ID, list[0].ID)
	assert.Equal(t, first.ID, list[1].ID)

	// Different user gets empty list.
	other, err := repo.ListByUser(ctx, uuid.New(), 10)
	require.NoError(t, err)
	assert.Empty(t, other)
}

func TestOrderRepository_ListItems(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, ids := newOrderRepo(t, ctx)

	order, err := repo.Create(ctx, newOrderInput(ids.userID))
	require.NoError(t, err)

	item, err := repo.CreateItem(ctx, order.ID, newItemInput(ids.variantID))
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, item.ID)
	assert.Equal(t, order.ID, item.OrderID)
	assert.Equal(t, ids.variantID, item.VariantID)
	assert.Equal(t, int32(2), item.Quantity)
	assert.Equal(t, int64(9900), item.UnitPrice)

	items, err := repo.ListItems(ctx, order.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, item.ID, items[0].ID)

	// Empty for unknown order.
	empty, err := repo.ListItems(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, empty)
}

var _ application.OrderRepository = (*infrastructure.Repository)(nil)
