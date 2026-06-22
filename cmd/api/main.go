// Package main is the API server entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/health"
	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	imagex "github.com/danilloboing/marketplace-golang/internal/platform/image"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	internalredis "github.com/danilloboing/marketplace-golang/internal/platform/redis"
	"github.com/danilloboing/marketplace-golang/internal/platform/storage/r2"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func cookieName(base string, securePrefix bool) string {
	if securePrefix {
		return "__Secure-" + base
	}
	return base
}

func parseCIDRs(raw []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		out = append(out, p)
	}
	return out, nil
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

	storeClient, err := r2.New(rootCtx, cfg.Storage)
	if err != nil {
		return fmt.Errorf("connect storage: %w", err)
	}
	imageProcessor := imagex.New()

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

	catalogModule := catalog.New(pool, storeClient, imageProcessor, cfg.Admin.APIToken)
	catalogModule.Mount(router)

	emailSender, err := email.NewSenderFromConfig(email.Config{
		Provider:            cfg.Email.Provider,
		FromAddress:         cfg.Email.FromAddress,
		FromName:            cfg.Email.FromName,
		SESRegion:           cfg.Email.SESRegion,
		SESConfigurationSet: cfg.Email.SESConfigurationSet,
	}, logger)
	if err != nil {
		return fmt.Errorf("api: build email sender: %w", err)
	}

	sessions := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:        rdb,
		TTLDefault:    cfg.Session.TTLDefault,
		TTLRememberMe: cfg.Session.TTLRememberMe,
		RefreshAfter:  cfg.Session.RefreshAfter,
	})

	cookies := transport.CookieConfig{
		SessionName:  cookieName("session_id", cfg.Cookies.SecurePrefix),
		CSRFName:     cookieName("csrf_token", cfg.Cookies.SecurePrefix),
		SecurePrefix: cfg.Cookies.SecurePrefix,
	}

	csrfCfg := csrf.Config{
		AllowedOrigins: cfg.CSRF.AllowedOrigins,
		CookieName:     cookies.CSRFName,
	}

	trustedProxies, err := parseCIDRs(cfg.RateLimit.TrustedProxies)
	if err != nil {
		return fmt.Errorf("api: parse trusted proxies: %w", err)
	}

	identityModule := identity.New(identity.Deps{
		Pool:     pool,
		Redis:    rdb,
		Email:    emailSender,
		Sessions: sessions,
		Cookies:  cookies,
		CSRFCfg:  csrfCfg,
		RateLimitOpts: ratelimit.Options{
			Client:         rdb,
			TrustedProxies: trustedProxies,
		},
		Cfg: cfg,
	})
	identityModule.Mount(router)

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
