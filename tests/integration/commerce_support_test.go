//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/address"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

type commerceEnv struct {
	srv       *httptest.Server
	emails    emailCapture
	pool      *pgxpool.Pool
	variantID uuid.UUID
	viacepHit func() int64
}

// startCommerceAPI boots identity + cart + address modules against testcontainers,
// using a fake email sender and a real ViaCEP client pointed at an httptest fixture.
func startCommerceAPI(t *testing.T, ctx context.Context) commerceEnv {
	t.Helper()

	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	addr := testutil.NewTestRedisAddr(t)

	pool, err := internalpostgres.NewPool(ctx, config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: 30 * time.Minute})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.Ping(ctx).Err())

	variantID := seedVariant(t, ctx, pool)

	sender := &fakeSender{}
	sessions := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client: rdb, TTLDefault: time.Hour, TTLRememberMe: 2 * time.Hour, RefreshAfter: 30 * time.Minute,
	})
	cookies := transport.CookieConfig{SessionName: "session_id", CSRFName: "csrf_token"}
	csrfCfg := csrf.Config{AllowedOrigins: []string{}, CookieName: cookies.CSRFName}

	fixture := testutil.NewViaCEPFixture(t)
	viacepClient := viacep.NewClient(fixture.Server.Client(), rdb, fixture.Server.URL, time.Hour)

	cfg := config.Config{
		Email:   config.Email{Provider: "log", VerifyLinkBaseURL: "http://t/verify", ResetLinkBaseURL: "http://t/reset"},
		Session: config.Session{TTLDefault: time.Hour, TTLRememberMe: 2 * time.Hour, RefreshAfter: 30 * time.Minute},
	}

	cartModule := cart.New(cart.Deps{Pool: pool, Sessions: sessions, SessionCookie: "session_id", AnonCookieName: "cart_anon"})
	identityModule := identity.New(identity.Deps{
		Pool: pool, Redis: rdb, Email: sender, Sessions: sessions, Cookies: cookies, CSRFCfg: csrfCfg,
		RateLimitOpts: ratelimit.Options{Client: rdb}, Cfg: cfg,
		CartMerge: cartModule.Merger(), CartCookieName: cartModule.AnonCookieName(),
	})
	addressModule := address.New(address.Deps{Pool: pool, Sessions: sessions, SessionCookie: "session_id", CSRFCfg: csrfCfg, ViaCEP: viacepClient})

	router := chi.NewRouter()
	identityModule.Mount(router)
	cartModule.Mount(router)
	addressModule.Mount(router)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return commerceEnv{srv: srv, emails: sender, pool: pool, variantID: variantID, viacepHit: fixture.Hits}
}

// seedVariant inserts a category + product + variant (price 9900) and returns the variant ID.
func seedVariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	productID := uuid.New()
	variantID := uuid.New()
	categoryID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, 'Cat')`,
		categoryID, "cat-"+categoryID.String())
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO catalog_products
		(id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		VALUES ($1, $2, 'P', 'D', 'B', $3, 5000, 'BRL', 'published')`,
		productID, "slug-"+productID.String(), categoryID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		VALUES ($1, $2, $3, 'M', 'Red', 9900)`, variantID, productID, "sku-"+variantID.String())
	require.NoError(t, err)
	return variantID
}
