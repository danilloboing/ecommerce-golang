// Package main is the API server entry point.
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

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/health"
	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	internalredis "github.com/danilloboing/marketplace-golang/internal/platform/redis"
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
		Service: cfg.Observability.OTELServiceName,
		Env:     cfg.App.Env,
	})
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	slog.SetDefault(logger)

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	flushSentry, err := observability.SetupSentry(observability.SentryOptions{
		DSN:     cfg.Observability.SentryDSN,
		Service: cfg.Observability.OTELServiceName,
		Env:     cfg.App.Env,
	})
	if err != nil {
		return fmt.Errorf("init sentry: %w", err)
	}
	defer flushSentry()

	shutdownTracing, err := observability.SetupTracing(rootCtx, observability.TracingOptions{
		ServiceName:  cfg.Observability.OTELServiceName,
		Env:          cfg.App.Env,
		Endpoint:     cfg.Observability.OTELExporterEndpoint,
		SamplerRatio: cfg.Observability.OTELTracesSamplerRatio,
		Insecure:     true,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

	pool, err := internalpostgres.NewPool(rootCtx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	rdb, err := internalredis.NewClient(rootCtx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()

	metrics := observability.NewMetrics()

	healthHandler := health.NewHandler(map[string]health.Checker{
		"postgres": health.CheckerFunc(func(ctx context.Context) error { return pool.Ping(ctx) }),
		"redis":    health.CheckerFunc(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
	})

	router := chi.NewRouter()
	router.Use(httpx.RequestID())
	router.Use(httpx.RequestLogger(logger))
	router.Use(httpx.Recover(logger))
	router.Use(httpx.SecurityHeaders())
	router.Use(httpx.CORS(cfg.CORS.AllowedOrigins))

	router.Get("/health", healthHandler.Liveness)
	router.Get("/ready", healthHandler.Readiness)
	router.Method("GET", "/metrics", metrics.Handler())

	catalogModule := catalog.New(pool, cfg.Admin.APIToken)
	catalogModule.Mount(router)

	srv := httpx.NewServer(httpx.ServerOptions{
		Addr:            fmt.Sprintf(":%d", cfg.App.Port),
		Handler:         router,
		ShutdownTimeout: cfg.App.ShutdownTimeout,
	})

	logger.Info("api starting",
		slog.Int("port", cfg.App.Port),
		slog.String("env", cfg.App.Env))

	g, gCtx := errgroup.WithContext(rootCtx)
	g.Go(func() error {
		if err := srv.Start(); err != nil {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		<-gCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	logger.Info("api shutdown complete")
	return nil
}
