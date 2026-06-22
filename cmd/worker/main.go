// Package main is the river worker entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	cartjobs "github.com/danilloboing/marketplace-golang/internal/modules/cart/jobs"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/jobs"
	inventoryjobs "github.com/danilloboing/marketplace-golang/internal/modules/inventory/jobs"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/platform/queue"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:   cfg.App.LogLevel,
		Output:  os.Stdout,
		Service: cfg.Observability.OTELServiceName + "-worker",
		Env:     cfg.App.Env,
	})
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	slog.SetDefault(logger)

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := internalpostgres.NewPool(rootCtx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	if err := queue.Migrate(rootCtx, pool); err != nil {
		return fmt.Errorf("migrate river schema: %w", err)
	}

	workers := river.NewWorkers()
	cleanup := &jobs.CleanupOrphansWorker{Pool: pool, Logger: logger}
	if err := river.AddWorkerSafely(workers, cleanup); err != nil {
		return fmt.Errorf("register cleanup worker: %w", err)
	}

	cartCleanup := cartjobs.NewCleanupAbandonedCartsWorker(pool, cfg.Cart.AbandonedAfter)
	if err := river.AddWorkerSafely(workers, cartCleanup); err != nil {
		return fmt.Errorf("register cart cleanup worker: %w", err)
	}

	releaseExpired := inventoryjobs.NewReleaseExpiredReservationsWorker(pool)
	if err := river.AddWorkerSafely(workers, releaseExpired); err != nil {
		return fmt.Errorf("register release expired reservations worker: %w", err)
	}

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return jobs.CleanupOrphansArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(cfg.Cart.CleanupInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return cartjobs.CleanupAbandonedCartsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(cfg.Checkout.ReleaseInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return inventoryjobs.ReleaseExpiredReservationsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
	}

	server, err := queue.NewServer(pool, workers, periodic)
	if err != nil {
		return err
	}

	if err := server.Start(rootCtx); err != nil {
		return fmt.Errorf("queue: start: %w", err)
	}

	logger.Info("worker started")

	<-rootCtx.Done()
	logger.Info("worker shutdown initiated")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer stopCancel()
	if err := server.Stop(stopCtx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("queue: stop: %w", err)
	}

	logger.Info("worker shutdown complete")
	return nil
}
