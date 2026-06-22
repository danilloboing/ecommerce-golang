// Package checkout wires the checkout bounded context.
package checkout

import (
	"context"
	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/adminauth"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	addrdomain "github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/transport"
	shippingdomain "github.com/danilloboing/marketplace-golang/internal/modules/shipping/domain"
)

// Compile-time assertion: *application.CheckoutService must satisfy transport.CheckoutUseCase.
var _ transport.CheckoutUseCase = (*application.CheckoutService)(nil)

// Compile-time assertion: *application.CouponService must satisfy transport.CouponUseCase.
var _ transport.CouponUseCase = (*application.CouponService)(nil)

// Module wires the checkout bounded context onto a chi router.
type Module struct {
	checkout      *transport.CheckoutHandlers
	coupon        *transport.CouponHandler
	reconcileRepo *infrastructure.ReconcileRepo
	reconciler    *application.Reconciler
	sessions      sessionauth.Manager
	sessionCookie string
	csrfCfg       csrf.Config
	adminToken    string
}

// AddressGetter is the narrow interface checkout needs from the address module.
// Satisfied by *address/infrastructure.Repository.
type AddressGetter interface {
	GetByID(ctx context.Context, id, userID uuid.UUID) (addrdomain.Address, error)
}

// ShippingQuoter is the narrow interface checkout needs from the shipping module.
// Satisfied by *shipping/application.ShippingService.
type ShippingQuoter interface {
	Quote(ctx context.Context, req shippingdomain.QuoteRequest) ([]shippingdomain.Quote, error)
}

// Charger is the checkout-visible payment charge port (from application/ports.go).
// Carried as a raw application.Charger so the module can inject the mock charger
// without depending on payment infrastructure.
type Charger = application.Charger

// Deps groups all raw dependencies the checkout module needs.
type Deps struct {
	Pool          *pgxpool.Pool
	Sessions      sessionauth.Manager
	SessionCookie string
	CSRFCfg       csrf.Config
	AdminToken    string
	// Address is used to build the AddressReaderAdapter (needs GetByID).
	Address       AddressGetter
	// Shipping is used to build the ShippingQuoterAdapter.
	Shipping      ShippingQuoter
	// Charger is the payment charge adapter (Phase 3a uses MockCharger).
	Charger       application.Charger
	// Cfg carries checkout-specific timing configuration.
	Cfg           config.Checkout
}

// New builds the checkout Module.
func New(d Deps) *Module {
	// --- infrastructure repos ---
	quoteRepo := infrastructure.NewQuoteRepo(d.Pool)
	couponRepo := infrastructure.NewCouponRepo(d.Pool)
	confirmRepo := infrastructure.NewConfirmRepo(d.Pool)
	idemRepo := infrastructure.NewIdempotencyRepo(d.Pool)
	priceReader := infrastructure.NewPriceReader(d.Pool)
	reconcileRepo := infrastructure.NewReconcileRepo(d.Pool)
	cartReader := infrastructure.NewCartReaderAdapter(d.Pool)

	// --- cross-module adapters ---
	addrAdapter := infrastructure.NewAddressReaderAdapter(func(ctx context.Context, addressID, userID uuid.UUID) (application.AddressView, error) {
		a, err := d.Address.GetByID(ctx, addressID, userID)
		if err != nil {
			return application.AddressView{}, err
		}
		snapshot, err := json.Marshal(a)
		if err != nil {
			return application.AddressView{}, err
		}
		return application.AddressView{
			PostalCode: a.PostalCode,
			Snapshot:   snapshot,
		}, nil
	})

	shippingAdapter := infrastructure.NewShippingQuoterAdapter(func(ctx context.Context, postalCode string, subtotalCents int64) ([]application.ShippingOption, error) {
		quotes, err := d.Shipping.Quote(ctx, shippingdomain.QuoteRequest{
			PostalCode:    postalCode,
			SubtotalCents: subtotalCents,
		})
		if err != nil {
			return nil, err
		}
		opts := make([]application.ShippingOption, 0, len(quotes))
		for _, q := range quotes {
			opts = append(opts, application.ShippingOption{
				ServiceID:  q.ServiceID,
				Name:       q.Name,
				PriceCents: q.PriceCents,
				ETADays:    q.ETADays,
			})
		}
		return opts, nil
	})

	charger := d.Charger
	if charger == nil {
		charger = infrastructure.NewMockCharger()
	}

	// --- services ---
	checkoutSvc := application.NewCheckoutService(application.CheckoutDeps{
		Carts:       cartReader,
		Prices:      priceReader,
		Shipping:    shippingAdapter,
		Addresses:   addrAdapter,
		Quotes:      quoteRepo,
		Coupons:     application.NewCouponService(couponRepo),
		ConfirmRepo: confirmRepo,
		Idempotency: idemRepo,
		Charger:     charger,
	},
		application.WithQuoteTTL(d.Cfg.QuoteTTL),
		application.WithReservationTTL(d.Cfg.ReservationTTL),
	)

	couponSvc := application.NewCouponService(couponRepo)
	reconciler := application.NewReconciler(reconcileRepo)

	return &Module{
		checkout:      transport.NewCheckoutHandlers(checkoutSvc),
		coupon:        transport.NewCouponHandler(couponSvc),
		reconcileRepo: reconcileRepo,
		reconciler:    reconciler,
		sessions:      d.Sessions,
		sessionCookie: d.SessionCookie,
		csrfCfg:       d.CSRFCfg,
		adminToken:    d.AdminToken,
	}
}

// Mount registers /checkout/* (session+csrf) and /admin/coupons (admin bearer).
func (m *Module) Mount(r chi.Router) {
	// Session + CSRF protected checkout routes.
	r.Group(func(auth chi.Router) {
		auth.Use(sessionauth.Middleware(m.sessions, m.sessionCookie))
		auth.Use(csrf.Middleware(m.csrfCfg))
		m.checkout.RegisterCheckoutRoutes(auth)
	})

	// Admin-bearer protected coupon management routes.
	r.Group(func(admin chi.Router) {
		admin.Use(adminauth.RequireToken(m.adminToken))
		m.coupon.RegisterCouponRoutes(admin)
	})
}

// Reconciler returns the application-layer reconciler satisfying
// payment/application.EventApplier. The payment module wires this in cmd/api
// (Task 21) to process webhook events.
func (m *Module) Reconciler() *application.Reconciler { return m.reconciler }
