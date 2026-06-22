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
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func newRepo(t *testing.T, ctx context.Context) (*infrastructure.Repository, *pgxIDs) {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ids := seedCatalog(t, ctx, pool)
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
	// Pass 98: existing(2)+new(98)=100, LEAST(100,99)=99.
	// Cannot pass 100: INSERT check constraint rejects qty>99 before conflict resolution.
	require.NoError(t, repo.UpsertItem(ctx, cart.ID, ids.variantID, 98, price))
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
