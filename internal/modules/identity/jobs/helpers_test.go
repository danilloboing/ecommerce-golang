//go:build integration

package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

// newTestPool spins up a fresh Postgres testcontainer, applies migrations,
// and returns a connected pool. The pool is closed at test cleanup.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)

	cfg := config.Database{
		URL:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	}
	pool, err := internalpostgres.NewPool(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}
