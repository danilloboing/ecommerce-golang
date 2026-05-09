//go:build integration

package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/jobs"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupOrphansWorker_DeletesOrphansOlderThan24h(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)

	cfg := config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: time.Minute}
	pool, err := internalpostgres.NewPool(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ctx := context.Background()

	categoryID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, $3)`,
		categoryID, "vestidos", "Vestidos")
	require.NoError(t, err)

	productID := uuid.New()
	_, err = pool.Exec(ctx, `
        INSERT INTO catalog_products (id, slug, name, category_id, base_price_cents, currency, status)
        VALUES ($1, 'p', 'P', $2, 100, 'BRL', 'published')
    `, productID, categoryID)
	require.NoError(t, err)

	// Drop FK temporarily so we can insert an orphan image (real-world orphans
	// arise from race conditions or manual deletion; the cleanup job exists
	// precisely for these cases).
	_, err = pool.Exec(ctx, `ALTER TABLE catalog_images DROP CONSTRAINT catalog_images_product_id_fkey`)
	require.NoError(t, err)

	// Old orphan (older than 24h, parent missing) — should be deleted.
	orphanID := uuid.New()
	phantomProductID := uuid.New()
	_, err = pool.Exec(ctx, `
        INSERT INTO catalog_images (id, product_id, url, alt_text, position, created_at)
        VALUES ($1, $2, 'https://x', '', 0, NOW() - INTERVAL '48 hours')
    `, orphanID, phantomProductID)
	require.NoError(t, err)

	// Recent orphan (younger than 24h) — should be retained.
	youngOrphanID := uuid.New()
	_, err = pool.Exec(ctx, `
        INSERT INTO catalog_images (id, product_id, url, alt_text, position)
        VALUES ($1, $2, 'https://x', '', 0)
    `, youngOrphanID, phantomProductID)
	require.NoError(t, err)

	// Old non-orphan (parent exists) — should be retained.
	keepID := uuid.New()
	_, err = pool.Exec(ctx, `
        INSERT INTO catalog_images (id, product_id, url, alt_text, position, created_at)
        VALUES ($1, $2, 'https://x', '', 0, NOW() - INTERVAL '48 hours')
    `, keepID, productID)
	require.NoError(t, err)

	worker := &jobs.CleanupOrphansWorker{Pool: pool}
	err = worker.Work(ctx, &river.Job[jobs.CleanupOrphansArgs]{Args: jobs.CleanupOrphansArgs{}})
	require.NoError(t, err)

	var count int

	err = pool.QueryRow(ctx, `SELECT count(*) FROM catalog_images WHERE id = $1`, orphanID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "old orphan should be deleted")

	err = pool.QueryRow(ctx, `SELECT count(*) FROM catalog_images WHERE id = $1`, youngOrphanID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "young orphan should be retained")

	err = pool.QueryRow(ctx, `SELECT count(*) FROM catalog_images WHERE id = $1`, keepID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "non-orphan with existing parent should be retained")
}

func TestCleanupOrphansWorker_NilPoolReturnsError(t *testing.T) {
	worker := &jobs.CleanupOrphansWorker{}
	err := worker.Work(context.Background(), &river.Job[jobs.CleanupOrphansArgs]{})
	require.Error(t, err)
}
