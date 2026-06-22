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
	addressinfra "github.com/danilloboing/marketplace-golang/internal/modules/address/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout"
	checkoutinfra "github.com/danilloboing/marketplace-golang/internal/modules/checkout/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment"
	payinfra "github.com/danilloboing/marketplace-golang/internal/modules/payment/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

// checkoutEnv is the wired-up environment for checkout E2E tests. It boots the
// full commerce surface (identity + cart + address + inventory + ordering +
// shipping + payment + checkout) against testcontainers, with mock providers
// and the reconciler wired into payment exactly like cmd/api/main.go.
type checkoutEnv struct {
	srv       *httptest.Server
	emails    emailCapture
	pool      *pgxpool.Pool
	variantID uuid.UUID
	secret    string
}

// checkoutWebhookSecret is the shared HMAC secret for the mock payment provider
// in checkout E2E tests. It mirrors MOCK_WEBHOOK_SECRET in cmd/api.
const checkoutWebhookSecret = "test-secret"

// startCheckoutAPI boots the full commerce API against testcontainers and
// returns a wired checkoutEnv. The admin token is "admin-token" so admin
// endpoints (set-stock, create-coupon) can be exercised with a bearer header.
func startCheckoutAPI(t *testing.T, ctx context.Context) checkoutEnv {
	t.Helper()

	const adminToken = "admin-token"

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
	inventoryModule := inventory.New(inventory.Deps{Pool: pool, AdminToken: adminToken})
	orderingModule := ordering.New(ordering.Deps{Pool: pool, Sessions: sessions, SessionCookie: "session_id"})
	shippingModule := shipping.New(shipping.Deps{Provider: "mock"})

	// Payment + checkout wiring mirrors cmd/api/main.go (Task 21): the checkout
	// charger wraps the mock provider so provider_charge_id is replay-stable, and
	// the reconciler is injected into payment via SetApplier after construction.
	paymentProvider := payinfra.NewMockProvider(checkoutWebhookSecret)
	paymentModule := payment.New(payment.Deps{Pool: pool, Provider: paymentProvider})

	checkoutModule := checkout.New(checkout.Deps{
		Pool:          pool,
		Sessions:      sessions,
		SessionCookie: "session_id",
		CSRFCfg:       csrfCfg,
		AdminToken:    adminToken,
		Address:       addressinfra.New(pool),
		Shipping:      shippingModule.Service(),
		Charger:       checkoutinfra.NewProviderCharger(paymentProvider),
		Cfg:           config.Checkout{QuoteTTL: 15 * time.Minute, ReservationTTL: 30 * time.Minute},
	})

	paymentModule.SetApplier(checkoutModule.Reconciler())

	router := chi.NewRouter()
	identityModule.Mount(router)
	cartModule.Mount(router)
	addressModule.Mount(router)
	inventoryModule.Mount(router)
	orderingModule.Mount(router)
	paymentModule.Mount(router)
	checkoutModule.Mount(router)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return checkoutEnv{srv: srv, emails: sender, pool: pool, variantID: variantID, secret: checkoutWebhookSecret}
}

// seedStock sets the available stock for the env's seeded variant via a direct
// upsert (version 0 on first insert). Reserved/version start at 0.
func (e checkoutEnv) seedStock(t *testing.T, ctx context.Context, variantID uuid.UUID, available int) {
	t.Helper()
	_, err := e.pool.Exec(ctx, `INSERT INTO inventory_stock (variant_id, available, reserved, version)
		VALUES ($1, $2, 0, 0)
		ON CONFLICT (variant_id) DO UPDATE SET available = EXCLUDED.available, reserved = 0, updated_at = now()`,
		variantID, available)
	require.NoError(t, err)
}

// stockRow is the projection of an inventory_stock row used by assertions.
type stockRow struct {
	Available int
	Reserved  int
}

// readStock returns the current available/reserved for a variant.
func (e checkoutEnv) readStock(t *testing.T, ctx context.Context, variantID uuid.UUID) stockRow {
	t.Helper()
	var s stockRow
	err := e.pool.QueryRow(ctx, `SELECT available, reserved FROM inventory_stock WHERE variant_id = $1`, variantID).
		Scan(&s.Available, &s.Reserved)
	require.NoError(t, err)
	return s
}
