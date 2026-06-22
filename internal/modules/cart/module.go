// Package cart wires the cart bounded context.
package cart

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/transport"
)

// Module wires the cart bounded context onto a chi router.
type Module struct {
	handlers       *transport.CartHandlers
	svc            *application.CartService
	sessions       sessionauth.Manager
	sessionCookie  string
	anonCookieName string
}

// Deps groups raw dependencies the cart module needs.
type Deps struct {
	Pool           *pgxpool.Pool
	Sessions       sessionauth.Manager
	SessionCookie  string
	AnonCookieName string
}

// New builds the cart Module.
func New(d Deps) *Module {
	repo := infrastructure.New(d.Pool)
	svc := application.NewCartService(repo)
	return &Module{
		handlers:       transport.NewCartHandlers(svc, d.AnonCookieName),
		svc:            svc,
		sessions:       d.Sessions,
		sessionCookie:  d.SessionCookie,
		anonCookieName: d.AnonCookieName,
	}
}

// Mount registers public cart routes wrapped with cart-identity resolution.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(public chi.Router) {
		public.Use(transport.ResolveCartIdentity(m.sessions, m.sessionCookie, m.anonCookieName))
		m.handlers.RegisterCartRoutes(public)
	})
}

// Merger returns the cart-merge callback for the identity Login handler.
// Decoupled signature (no cart types) so identity needs no cart import.
func (m *Module) Merger() func(ctx context.Context, anonID string, userID uuid.UUID) error {
	return m.svc.Merge
}

// AnonCookieName exposes the anon cookie name so identity can clear it on login.
func (m *Module) AnonCookieName() string { return m.anonCookieName }
