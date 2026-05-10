//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

// emailCapture exposes captured emails for E2E assertions.
type emailCapture interface {
	messages() []email.Message
}

// fakeSender implements email.Sender and records every Send call so tests
// can extract verify / reset tokens from the rendered message bodies.
type fakeSender struct {
	mu  sync.Mutex
	msg []email.Message
}

func (f *fakeSender) Send(_ context.Context, m email.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msg = append(f.msg, m)
	return nil
}

func (f *fakeSender) messages() []email.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]email.Message, len(f.msg))
	copy(out, f.msg)
	return out
}

// startAPIForIdentity boots a chi router with the identity module wired
// against testcontainers Postgres + Redis and a fakeSender. The returned
// httptest.Server exposes the full identity API surface.
//
// Mirrors cmd/api/main.go's identity wiring with two deviations:
//   - email.Sender is replaced by *fakeSender (so tests can read tokens)
//   - cookies use plain "session_id" / "csrf_token" names (no __Secure- prefix)
//     because httptest serves over plaintext.
func startAPIForIdentity(t *testing.T, ctx context.Context) (*httptest.Server, emailCapture) {
	t.Helper()

	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	addr := testutil.NewTestRedisAddr(t)

	pool, err := internalpostgres.NewPool(ctx, config.Database{
		URL:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Minute,
	})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx).Err())

	sender := &fakeSender{}

	sessions := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:        rdb,
		TTLDefault:    time.Hour,
		TTLRememberMe: 2 * time.Hour,
		RefreshAfter:  30 * time.Minute,
	})

	cookies := transport.CookieConfig{
		SessionName:  "session_id",
		CSRFName:     "csrf_token",
		SecurePrefix: false,
	}

	// Empty AllowedOrigins means: no Origin header is allowed. The test client
	// does not set an Origin header, so the Origin check is bypassed (the
	// middleware only enforces it when present). The double-submit cookie+header
	// match still runs.
	csrfCfg := csrf.Config{
		AllowedOrigins: []string{},
		CookieName:     cookies.CSRFName,
	}

	cfg := config.Config{
		Email: config.Email{
			Provider:          "log",
			FromAddress:       "no-reply@test.local",
			FromName:          "Test Loja",
			VerifyLinkBaseURL: "http://test.local/verify",
			ResetLinkBaseURL:  "http://test.local/reset",
		},
		Session: config.Session{
			TTLDefault:    time.Hour,
			TTLRememberMe: 2 * time.Hour,
			RefreshAfter:  30 * time.Minute,
		},
	}

	module := identity.New(identity.Deps{
		Pool:          pool,
		Redis:         rdb,
		Email:         sender,
		Sessions:      sessions,
		Cookies:       cookies,
		CSRFCfg:       csrfCfg,
		RateLimitOpts: ratelimit.Options{Client: rdb},
		Cfg:           cfg,
	})

	router := chi.NewRouter()
	module.Mount(router)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return srv, sender
}
