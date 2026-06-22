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
