//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPool_ConnectsAndPings(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)

	cfg := config.Database{
		URL:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	}

	pool, err := postgres.NewPool(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, pool.Ping(context.Background()))

	var got int
	err = pool.QueryRow(context.Background(), "SELECT 1").Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, 1, got)
}

func TestNewPool_InvalidDSNReturnsError(t *testing.T) {
	cfg := config.Database{URL: "not-a-dsn"}

	_, err := postgres.NewPool(context.Background(), cfg)

	require.Error(t, err)
}
