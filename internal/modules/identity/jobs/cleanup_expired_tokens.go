// Package jobs holds river background workers for the identity module.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// CleanupExpiredTokensArgs is the river job payload (no fields needed).
type CleanupExpiredTokensArgs struct{}

// Kind implements river.JobArgs.
func (CleanupExpiredTokensArgs) Kind() string { return "identity.cleanup_expired_tokens" }

// CleanupExpiredTokensWorker prunes expired tokens older than 7 days.
type CleanupExpiredTokensWorker struct {
	river.WorkerDefaults[CleanupExpiredTokensArgs]
	pool *pgxpool.Pool
}

// NewCleanupExpiredTokensWorker builds the worker.
func NewCleanupExpiredTokensWorker(pool *pgxpool.Pool) *CleanupExpiredTokensWorker {
	return &CleanupExpiredTokensWorker{pool: pool}
}

// Work runs once per scheduled tick.
func (w *CleanupExpiredTokensWorker) Work(ctx context.Context, _ *river.Job[CleanupExpiredTokensArgs]) error {
	if _, err := RunCleanupExpiredTokensOnce(ctx, w.pool); err != nil {
		return err
	}
	return nil
}

// RunCleanupExpiredTokensOnce executes the cleanup statement directly.
// Used by tests and by the periodic worker. Returns total deleted rows
// across both token tables.
func RunCleanupExpiredTokensOnce(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	q := queries.New(pool)
	v, err := q.DeleteExpiredEmailVerifyTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("identity jobs: delete expired verify tokens: %w", err)
	}
	r, err := q.DeleteExpiredPasswordResetTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("identity jobs: delete expired reset tokens: %w", err)
	}
	return v + r, nil
}
