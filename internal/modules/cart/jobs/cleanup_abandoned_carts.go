// Package jobs holds river background workers for the cart module.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// CleanupAbandonedCartsArgs is the river job payload.
type CleanupAbandonedCartsArgs struct{}

// Kind implements river.JobArgs.
func (CleanupAbandonedCartsArgs) Kind() string { return "cart.cleanup_abandoned_carts" }

// CleanupAbandonedCartsWorker marks stale anonymous carts as abandoned.
type CleanupAbandonedCartsWorker struct {
	river.WorkerDefaults[CleanupAbandonedCartsArgs]
	pool           *pgxpool.Pool
	abandonedAfter time.Duration
}

// NewCleanupAbandonedCartsWorker builds the worker.
func NewCleanupAbandonedCartsWorker(pool *pgxpool.Pool, abandonedAfter time.Duration) *CleanupAbandonedCartsWorker {
	return &CleanupAbandonedCartsWorker{pool: pool, abandonedAfter: abandonedAfter}
}

// Work runs once per scheduled tick.
func (w *CleanupAbandonedCartsWorker) Work(ctx context.Context, _ *river.Job[CleanupAbandonedCartsArgs]) error {
	_, err := RunCleanupAbandonedCartsOnce(ctx, w.pool, time.Now().Add(-w.abandonedAfter))
	return err
}

// RunCleanupAbandonedCartsOnce marks active anon carts older than cutoff as abandoned.
// Returns the number of rows updated. Used by the worker and by tests.
func RunCleanupAbandonedCartsOnce(ctx context.Context, pool *pgxpool.Pool, cutoff time.Time) (int64, error) {
	q := queries.New(pool)
	n, err := q.DeleteAbandonedCarts(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cart jobs: abandon carts: %w", err)
	}
	return n, nil
}
