// Package jobs holds catalog background workers.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// CleanupOrphansArgs declares the periodic job payload (no fields).
type CleanupOrphansArgs struct{}

// Kind returns the job type identifier.
func (CleanupOrphansArgs) Kind() string { return "catalog.cleanup_orphans" }

// CleanupOrphansWorker deletes catalog_images rows whose product no longer exists
// and whose creation time is older than 24h. Runs periodically.
type CleanupOrphansWorker struct {
	river.WorkerDefaults[CleanupOrphansArgs]
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Work executes the deletion query.
func (w *CleanupOrphansWorker) Work(ctx context.Context, _ *river.Job[CleanupOrphansArgs]) error {
	if w.Pool == nil {
		return errors.New("cleanup_orphans: nil pool")
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	tag, err := w.Pool.Exec(ctx, `
        DELETE FROM catalog_images
        WHERE created_at < $1
          AND product_id NOT IN (SELECT id FROM catalog_products)
    `, cutoff)
	if err != nil {
		return fmt.Errorf("cleanup_orphans: delete: %w", err)
	}
	if w.Logger != nil {
		w.Logger.Info("cleanup_orphans completed",
			slog.Int64("deletedCount", tag.RowsAffected()),
		)
	}
	return nil
}
