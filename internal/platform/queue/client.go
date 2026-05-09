// Package queue wires the river job queue.
package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// NewServer builds a river server (worker) backed by Postgres.
func NewServer(pool *pgxpool.Pool, workers *river.Workers, periodicJobs []*river.PeriodicJob) (*river.Client[pgx.Tx], error) {
	cli, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers:      workers,
		PeriodicJobs: periodicJobs,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: build server: %w", err)
	}
	return cli, nil
}

// NewInsertOnly builds a client used only to enqueue jobs from the API.
func NewInsertOnly(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	cli, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("queue: build insert-only client: %w", err)
	}
	return cli, nil
}

// Migrate applies river's own schema migrations against the given pool.
// Idempotent: safe to call on every worker start.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("queue: build migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("queue: migrate up: %w", err)
	}
	return nil
}
