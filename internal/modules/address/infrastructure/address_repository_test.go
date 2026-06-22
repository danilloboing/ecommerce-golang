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
