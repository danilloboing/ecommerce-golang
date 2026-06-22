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
